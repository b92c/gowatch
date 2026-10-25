package aws

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

// fallbackRegion is used when neither the environment, the options nor the
// profile name a region.
const fallbackRegion = "us-east-1"

// enableMode is the aws.enabled setting.
type enableMode int

const (
	// modeAuto starts the integration only when something configures AWS.
	modeAuto enableMode = iota
	// modeOn always starts it and lets the SDK try its whole credential chain.
	modeOn
	// modeOff never starts it.
	modeOff
)

// signalVariables are the environment variables that make modeAuto start the
// integration, in the order they are checked.
var signalVariables = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_PROFILE",
	"AWS_REGION",
	"AWS_DEFAULT_REGION",
	"AWS_ENDPOINT_URL",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
	"AWS_CONTAINER_CREDENTIALS_FULL_URI",
}

// settings is what Options and the environment resolve to. It is computed once,
// when the manager is built, from environment variables and file metadata only.
//
// The profile, region and endpoint fields hold the value that wins. The
// ...Option fields hold the Options value only when the environment has no
// value of its own: those are the ones handed to the SDK, which then reads the
// environment and the shared files itself.
type settings struct {
	profile           string // AWS_PROFILE, else Options.Profile, else "default"
	profileOption     string // Options.Profile when AWS_PROFILE is empty, else ""
	region            string // provisional: AWS_REGION, AWS_DEFAULT_REGION, Options.Region, else "us-east-1"
	regionOption      string // Options.Region when AWS_REGION and AWS_DEFAULT_REGION are empty, else ""
	endpointURL       string // AWS_ENDPOINT_URL, else Options.EndpointURL
	endpointOption    string // Options.EndpointURL when AWS_ENDPOINT_URL is empty, else ""
	configFile        string // AWS_CONFIG_FILE, else <home>/.aws/config, "" when neither is known
	credentialsFile   string // AWS_SHARED_CREDENTIALS_FILE, else <home>/.aws/credentials, "" when neither is known
	enabledReason     string
	accounts          []string // a copy of Options.Accounts
	refreshInterval   time.Duration
	discoveryInterval time.Duration
	mode              enableMode
	enabled           bool
}

// resolveSettings applies the precedence rules to o and the environment:
// AWS_PROFILE, then Options.Profile, then "default"; AWS_REGION, then
// AWS_DEFAULT_REGION, then Options.Region, then (once the SDK has read the
// profile) the profile's region, then us-east-1; AWS_ENDPOINT_URL, then
// Options.EndpointURL.
func resolveSettings(o Options) settings {
	s := settings{
		profile:           "default",
		region:            fallbackRegion,
		accounts:          slices.Clone(o.Accounts),
		refreshInterval:   o.RefreshInterval,
		discoveryInterval: o.DiscoveryInterval,
	}
	if s.refreshInterval <= 0 {
		s.refreshInterval = DefaultRefreshInterval
	}
	if s.discoveryInterval <= 0 {
		s.discoveryInterval = DefaultDiscoveryInterval
	}

	if env := os.Getenv("AWS_PROFILE"); env != "" {
		s.profile = env
	} else if o.Profile != "" {
		s.profile, s.profileOption = o.Profile, o.Profile
	}

	switch {
	case os.Getenv("AWS_REGION") != "":
		s.region = os.Getenv("AWS_REGION")
	case os.Getenv("AWS_DEFAULT_REGION") != "":
		s.region = os.Getenv("AWS_DEFAULT_REGION")
	case o.Region != "":
		s.region, s.regionOption = o.Region, o.Region
	}

	if env := os.Getenv("AWS_ENDPOINT_URL"); env != "" {
		s.endpointURL = env
	} else if o.EndpointURL != "" {
		s.endpointURL, s.endpointOption = o.EndpointURL, o.EndpointURL
	}

	s.configFile, s.credentialsFile = sharedFiles()

	switch {
	case o.Enabled == nil:
		s.mode = modeAuto
		reason, found := detectSignal(o, s.configFile, s.credentialsFile)
		s.enabled = found
		s.enabledReason = reason
		if !found {
			s.enabledReason = "no AWS configuration found"
		}
	case *o.Enabled:
		s.mode, s.enabled, s.enabledReason = modeOn, true, "aws.enabled: true"
	default:
		s.mode, s.enabled, s.enabledReason = modeOff, false, "aws.enabled: false"
	}
	return s
}

// sharedFiles returns the shared config and credentials files: the paths in
// AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE, otherwise the standard ones
// under the home directory. A file whose variable is empty and whose home is
// unknown is "", never a relative path.
func sharedFiles() (configFile, credentialsFile string) {
	configFile = os.Getenv("AWS_CONFIG_FILE")
	credentialsFile = os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if configFile != "" && credentialsFile != "" {
		return configFile, credentialsFile
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return configFile, credentialsFile
	}
	if configFile == "" {
		configFile = filepath.Join(home, ".aws", "config")
	}
	if credentialsFile == "" {
		credentialsFile = filepath.Join(home, ".aws", "credentials")
	}
	return configFile, credentialsFile
}

// detectSignal reports whether something configures AWS, and what: one of the
// signal variables, an aws.* option, or an existing shared config or
// credentials file. It does nothing beyond reading the environment and two
// os.Stat calls.
func detectSignal(o Options, configFile, credentialsFile string) (reason string, found bool) {
	for _, name := range signalVariables {
		if os.Getenv(name) != "" {
			return name + " is set", true
		}
	}
	switch {
	case o.Profile != "":
		return "aws.profile is set", true
	case o.Region != "":
		return "aws.region is set", true
	case o.EndpointURL != "":
		return "aws.endpoint_url is set", true
	case len(o.Accounts) > 0:
		return "aws.accounts is set", true
	}
	if isRegularFile(configFile) {
		return "shared config file found", true
	}
	if isRegularFile(credentialsFile) {
		return "shared credentials file found", true
	}
	return "", false
}

// isRegularFile reports whether path names an existing file (not a directory).
func isRegularFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// filesOf returns the file list to hand to the SDK. The result is never nil:
// the SDK reads a nil list as "use the default location", and an empty one as
// "there is no file".
func filesOf(path string) []string {
	if path == "" {
		return []string{}
	}
	return []string{path}
}

// loadOptions returns the options for config.LoadDefaultConfig. The shared
// files are always explicit: the SDK computes its own default locations from
// HOME once, when the program starts, and its profile loader ignores
// AWS_CONFIG_FILE. Instance metadata is switched off in modeAuto so a weak
// signal, such as a config file that only holds a region, never makes the
// program probe 169.254.169.254. The SDK does not retry: the poller does, on
// its next pass.
func (s settings) loadOptions() []func(*config.LoadOptions) error {
	opts := []func(*config.LoadOptions) error{
		config.WithSharedConfigFiles(filesOf(s.configFile)),
		config.WithSharedCredentialsFiles(filesOf(s.credentialsFile)),
	}
	if s.profileOption != "" {
		opts = append(opts, config.WithSharedConfigProfile(s.profileOption))
	}
	if s.regionOption != "" {
		opts = append(opts, config.WithRegion(s.regionOption))
	}
	opts = append(opts, config.WithDefaultRegion(fallbackRegion))
	if s.endpointOption != "" {
		opts = append(opts, config.WithBaseEndpoint(s.endpointOption))
	}
	if s.mode == modeAuto {
		opts = append(opts, config.WithEC2IMDSClientEnableState(imds.ClientDisabled))
	}
	return append(opts, config.WithRetryMaxAttempts(1))
}
