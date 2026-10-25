package aws

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

func TestSharedFiles(t *testing.T) {
	t.Run("the variables name both files", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONFIG_FILE", "/x/config")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/x/creds")

		configFile, credentialsFile := sharedFiles()

		if configFile != "/x/config" || credentialsFile != "/x/creds" {
			t.Errorf("sharedFiles() = (%q, %q), want (/x/config, /x/creds)", configFile, credentialsFile)
		}
	})

	t.Run("blank variables fall back to the home directory", func(t *testing.T) {
		home := isolateAWSEnv(t)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")

		configFile, credentialsFile := sharedFiles()

		wantConfig, wantCredentials := filepath.Join(home, ".aws", "config"), filepath.Join(home, ".aws", "credentials")
		if configFile != wantConfig || credentialsFile != wantCredentials {
			t.Errorf("sharedFiles() = (%q, %q), want (%q, %q)", configFile, credentialsFile, wantConfig, wantCredentials)
		}
	})

	t.Run("an unknown home gives no path, never a relative one", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("AWS_CONFIG_FILE", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		t.Setenv("HOME", "")

		configFile, credentialsFile := sharedFiles()

		if configFile != "" || credentialsFile != "" {
			t.Errorf("sharedFiles() = (%q, %q), want empty paths", configFile, credentialsFile)
		}
	})
}

func TestResolveSettingsPrecedence(t *testing.T) {
	type resolved struct {
		profile        string
		profileOption  string
		region         string
		regionOption   string
		endpointURL    string
		endpointOption string
	}
	tests := []struct {
		env  map[string]string
		want resolved
		name string
		opts Options
	}{
		{name: "nothing", want: resolved{profile: "default", region: "us-east-1"}},

		{name: "AWS_PROFILE beats the config profile", env: map[string]string{"AWS_PROFILE": "dev"}, opts: Options{Profile: "ops"},
			want: resolved{profile: "dev", region: "us-east-1"}},
		{name: "the config profile", opts: Options{Profile: "ops"},
			want: resolved{profile: "ops", profileOption: "ops", region: "us-east-1"}},

		{name: "AWS_REGION beats the config region", env: map[string]string{"AWS_REGION": "eu-west-1"}, opts: Options{Region: "ap-south-1"},
			want: resolved{profile: "default", region: "eu-west-1"}},
		{name: "AWS_DEFAULT_REGION beats the config region", env: map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"}, opts: Options{Region: "ap-south-1"},
			want: resolved{profile: "default", region: "sa-east-1"}},
		{name: "AWS_REGION beats AWS_DEFAULT_REGION", env: map[string]string{"AWS_REGION": "eu-west-1", "AWS_DEFAULT_REGION": "sa-east-1"},
			want: resolved{profile: "default", region: "eu-west-1"}},
		{name: "the config region", opts: Options{Region: "ap-south-1"},
			want: resolved{profile: "default", region: "ap-south-1", regionOption: "ap-south-1"}},

		{name: "AWS_ENDPOINT_URL beats the config endpoint", env: map[string]string{"AWS_ENDPOINT_URL": "http://env:4566"}, opts: Options{EndpointURL: "http://cfg:4566"},
			want: resolved{profile: "default", region: "us-east-1", endpointURL: "http://env:4566"}},
		{name: "the config endpoint", opts: Options{EndpointURL: "http://cfg:4566"},
			want: resolved{profile: "default", region: "us-east-1", endpointURL: "http://cfg:4566", endpointOption: "http://cfg:4566"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSEnv(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			s := resolveSettings(tt.opts)

			got := resolved{
				profile:        s.profile,
				profileOption:  s.profileOption,
				region:         s.region,
				regionOption:   s.regionOption,
				endpointURL:    s.endpointURL,
				endpointOption: s.endpointOption,
			}
			if got != tt.want {
				t.Errorf("resolveSettings(%+v) = %+v, want %+v", tt.opts, got, tt.want)
			}
		})
	}

	intervals := []struct {
		name          string
		opts          Options
		wantRefresh   time.Duration
		wantDiscovery time.Duration
	}{
		{name: "unset intervals use the defaults", wantRefresh: 15 * time.Second, wantDiscovery: 60 * time.Second},
		{name: "negative intervals use the defaults", opts: Options{RefreshInterval: -time.Second, DiscoveryInterval: -time.Second},
			wantRefresh: 15 * time.Second, wantDiscovery: 60 * time.Second},
		{name: "set intervals are kept", opts: Options{RefreshInterval: 30 * time.Second, DiscoveryInterval: 2 * time.Minute},
			wantRefresh: 30 * time.Second, wantDiscovery: 2 * time.Minute},
	}
	for _, tt := range intervals {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSEnv(t)

			s := resolveSettings(tt.opts)

			if s.refreshInterval != tt.wantRefresh || s.discoveryInterval != tt.wantDiscovery {
				t.Errorf("intervals = %s and %s, want %s and %s", s.refreshInterval, s.discoveryInterval, tt.wantRefresh, tt.wantDiscovery)
			}
		})
	}
}

func TestDetectSignal(t *testing.T) {
	type signalCase struct {
		setup      func(t *testing.T, home string)
		name       string
		wantReason string
		opts       Options
		wantFound  bool
	}
	tests := []signalCase{
		{name: "an option profile", opts: Options{Profile: "p"}, wantReason: "aws.profile is set", wantFound: true},
		{name: "an option region", opts: Options{Region: "r"}, wantReason: "aws.region is set", wantFound: true},
		{name: "an option endpoint", opts: Options{EndpointURL: "e"}, wantReason: "aws.endpoint_url is set", wantFound: true},
		{
			name: "the shared config file",
			setup: func(t *testing.T, home string) {
				writeTestFile(t, filepath.Join(home, ".aws", "config"), "[default]\nregion = eu-west-1\n")
			},
			wantReason: "shared config file found", wantFound: true,
		},
		{
			name: "the shared credentials file",
			setup: func(t *testing.T, home string) {
				writeTestFile(t, filepath.Join(home, ".aws", "credentials"), "[default]\n")
			},
			wantReason: "shared credentials file found", wantFound: true,
		},
		{
			name: "a directory is not a config file",
			setup: func(t *testing.T, _ string) {
				t.Setenv("AWS_CONFIG_FILE", t.TempDir())
			},
		},
		{name: "nothing"},
	}
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_PROFILE",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_ENDPOINT_URL",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	} {
		tests = append(tests, signalCase{
			name:       name,
			setup:      func(t *testing.T, _ string) { t.Setenv(name, "x") },
			wantReason: name + " is set",
			wantFound:  true,
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateAWSEnv(t)
			if tt.setup != nil {
				tt.setup(t, home)
			}
			configFile, credentialsFile := sharedFiles()

			reason, found := detectSignal(tt.opts, configFile, credentialsFile)

			if reason != tt.wantReason || found != tt.wantFound {
				t.Errorf("detectSignal() = (%q, %t), want (%q, %t)", reason, found, tt.wantReason, tt.wantFound)
			}
		})
	}
}

func TestEnablementModes(t *testing.T) {
	tests := []struct {
		env         map[string]string
		name        string
		wantReason  string
		opts        Options
		wantMode    enableMode
		wantEnabled bool
	}{
		{name: "auto without a signal", wantMode: modeAuto, wantReason: "no AWS configuration found"},
		{name: "auto with a signal", env: map[string]string{"AWS_PROFILE": "dev"}, wantMode: modeAuto, wantEnabled: true, wantReason: "AWS_PROFILE is set"},
		{name: "forced on", opts: Options{Enabled: boolPtr(true)}, wantMode: modeOn, wantEnabled: true, wantReason: "aws.enabled: true"},
		{name: "forced off", env: map[string]string{"AWS_ACCESS_KEY_ID": "x"}, opts: Options{Enabled: boolPtr(false)}, wantMode: modeOff, wantReason: "aws.enabled: false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSEnv(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			s := resolveSettings(tt.opts)

			if s.mode != tt.wantMode || s.enabled != tt.wantEnabled || s.enabledReason != tt.wantReason {
				t.Errorf("resolveSettings() = mode %d, enabled %t, reason %q; want mode %d, enabled %t, reason %q",
					s.mode, s.enabled, s.enabledReason, tt.wantMode, tt.wantEnabled, tt.wantReason)
			}
		})
	}
}

// applyLoadOptions applies the options of s to an empty SDK option value.
func applyLoadOptions(t *testing.T, s settings) config.LoadOptions {
	t.Helper()
	var lo config.LoadOptions
	for _, apply := range s.loadOptions() {
		if err := apply(&lo); err != nil {
			t.Fatalf("applying a load option: %v", err)
		}
	}
	return lo
}

func TestLoadOptions(t *testing.T) {
	t.Run("instance metadata is off only in auto mode", func(t *testing.T) {
		if got := applyLoadOptions(t, settings{mode: modeAuto}).EC2IMDSClientEnableState; got != imds.ClientDisabled {
			t.Errorf("auto mode: IMDS state = %v, want disabled", got)
		}
		if got := applyLoadOptions(t, settings{mode: modeOn}).EC2IMDSClientEnableState; got != imds.ClientDefaultEnableState {
			t.Errorf("mode on: IMDS state = %v, want the SDK default", got)
		}
	})

	t.Run("the profile is passed only when it comes from the options", func(t *testing.T) {
		if got := applyLoadOptions(t, settings{profileOption: "ops"}).SharedConfigProfile; got != "ops" {
			t.Errorf("SharedConfigProfile = %q, want ops", got)
		}
		if got := applyLoadOptions(t, settings{profile: "dev"}).SharedConfigProfile; got != "" {
			t.Errorf("SharedConfigProfile = %q, want empty so AWS_PROFILE stays in charge", got)
		}
	})

	t.Run("the region is passed only when it comes from the options", func(t *testing.T) {
		lo := applyLoadOptions(t, settings{regionOption: "ap-south-1"})
		if lo.Region != "ap-south-1" {
			t.Errorf("Region = %q, want ap-south-1", lo.Region)
		}
		if lo.DefaultRegion != "us-east-1" {
			t.Errorf("DefaultRegion = %q, want us-east-1", lo.DefaultRegion)
		}

		lo = applyLoadOptions(t, settings{region: "eu-west-1"})
		if lo.Region != "" {
			t.Errorf("Region = %q, want empty so the environment and the profile stay in charge", lo.Region)
		}
		if lo.DefaultRegion != "us-east-1" {
			t.Errorf("DefaultRegion = %q, want us-east-1", lo.DefaultRegion)
		}
	})

	t.Run("the endpoint is passed only when it comes from the options", func(t *testing.T) {
		if got := applyLoadOptions(t, settings{endpointOption: "http://cfg:4566"}).BaseEndpoint; got != "http://cfg:4566" {
			t.Errorf("BaseEndpoint = %q, want http://cfg:4566", got)
		}
		if got := applyLoadOptions(t, settings{endpointURL: "http://env:4566"}).BaseEndpoint; got != "" {
			t.Errorf("BaseEndpoint = %q, want empty so AWS_ENDPOINT_URL stays in charge", got)
		}
	})

	t.Run("the shared files are explicit", func(t *testing.T) {
		lo := applyLoadOptions(t, settings{configFile: "/x/config", credentialsFile: "/x/creds"})
		if !reflect.DeepEqual(lo.SharedConfigFiles, []string{"/x/config"}) || !reflect.DeepEqual(lo.SharedCredentialsFiles, []string{"/x/creds"}) {
			t.Errorf("shared files = %v and %v, want [/x/config] and [/x/creds]", lo.SharedConfigFiles, lo.SharedCredentialsFiles)
		}

		// A nil slice would send the SDK to its own default location; an empty
		// one tells it there is no file.
		lo = applyLoadOptions(t, settings{})
		if lo.SharedConfigFiles == nil || len(lo.SharedConfigFiles) != 0 || lo.SharedCredentialsFiles == nil || len(lo.SharedCredentialsFiles) != 0 {
			t.Errorf("shared files = %#v and %#v, want non-nil empty slices", lo.SharedConfigFiles, lo.SharedCredentialsFiles)
		}
	})

	t.Run("the SDK does not retry", func(t *testing.T) {
		if got := applyLoadOptions(t, settings{}).RetryMaxAttempts; got != 1 {
			t.Errorf("RetryMaxAttempts = %d, want 1", got)
		}
	})
}

// This test runs the real SDK loader over files in a temporary home. It makes
// no network call: the credentials come from static keys.
func TestLoadDefaultConfigReadsOnlyTheIsolatedFiles(t *testing.T) {
	const (
		legacySSOProfile = "[default]\nsso_start_url = https://example.awsapps.com/start\nsso_region = us-east-1\nsso_account_id = 123456789012\nsso_role_name = ReadOnly\nregion = sa-east-1\n"
		fakeKeys         = "[default]\naws_access_key_id = AKIDFAKEEXAMPLE\naws_secret_access_key = fake-secret-for-tests\n"
	)
	tests := []struct {
		env        map[string]string
		name       string
		wantRegion string
		opts       Options
	}{
		{name: "the region of the profile", wantRegion: "sa-east-1"},
		{name: "the config region beats the profile", opts: Options{Region: "ap-south-1"}, wantRegion: "ap-south-1"},
		{name: "the environment beats the config region", env: map[string]string{"AWS_REGION": "eu-west-1"}, opts: Options{Region: "ap-south-1"}, wantRegion: "eu-west-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateAWSEnv(t)
			writeTestFile(t, filepath.Join(home, ".aws", "config"), legacySSOProfile)
			writeTestFile(t, filepath.Join(home, ".aws", "credentials"), fakeKeys)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			ctx := context.Background()

			cfg, err := config.LoadDefaultConfig(ctx, resolveSettings(tt.opts).loadOptions()...)
			if err != nil {
				t.Fatalf("LoadDefaultConfig() error = %v", err)
			}

			if cfg.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", cfg.Region, tt.wantRegion)
			}
			creds, err := cfg.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatalf("Credentials.Retrieve() error = %v", err)
			}
			if creds.AccessKeyID != "AKIDFAKEEXAMPLE" {
				t.Errorf("AccessKeyID = %q, want the static key of the credentials file", creds.AccessKeyID)
			}
		})
	}
}
