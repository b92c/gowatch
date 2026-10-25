package aws

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func TestNewAWSClientManagerWithOptions(t *testing.T) {
	tests := []struct {
		env           map[string]string
		name          string
		configContent string // written to <home>/.aws/config when not empty
		credsContent  string // written to <home>/.aws/credentials when not empty
		wantRegion    string
		wantEndpoint  string
		wantProfile   string
		wantConn      Conn
		wantReason    string
		opts          Options
		wantEnabled   bool
	}{
		// Zero Options must behave exactly like the environment-only constructor.
		{name: "no env, no options", wantRegion: "us-east-1", wantProfile: "default", wantConn: ConnNotConfigured, wantReason: "no AWS configuration found"},
		{name: "AWS_REGION enables", env: map[string]string{"AWS_REGION": "eu-west-1"}, wantRegion: "eu-west-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_REGION is set"},
		{name: "AWS_DEFAULT_REGION is a signal too", env: map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"}, wantRegion: "sa-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_DEFAULT_REGION is set"},
		{name: "AWS_ACCESS_KEY_ID enables", env: map[string]string{"AWS_ACCESS_KEY_ID": "key"}, wantRegion: "us-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_ACCESS_KEY_ID is set"},
		{name: "AWS_PROFILE enables", env: map[string]string{"AWS_PROFILE": "dev"}, wantRegion: "us-east-1", wantProfile: "dev", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_PROFILE is set"},
		{name: "AWS_ENDPOINT_URL enables", env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://localhost:4566", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_ENDPOINT_URL is set"},

		// Options fill in what the environment leaves out.
		{name: "config region alone enables in auto mode", opts: Options{Region: "ap-south-1"}, wantRegion: "ap-south-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "aws.region is set"},
		{name: "config endpoint alone enables in auto mode", opts: Options{EndpointURL: "http://localstack:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://localstack:4566", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "aws.endpoint_url is set"},
		{name: "config profile alone enables in auto mode", opts: Options{Profile: "ops"}, wantRegion: "us-east-1", wantProfile: "ops", wantEnabled: true, wantConn: ConnConnecting, wantReason: "aws.profile is set"},
		{name: "config region, endpoint and profile", opts: Options{Region: "ap-south-1", EndpointURL: "http://localstack:4566", Profile: "ops"}, wantRegion: "ap-south-1", wantEndpoint: "http://localstack:4566", wantProfile: "ops", wantEnabled: true, wantConn: ConnConnecting, wantReason: "aws.profile is set"},

		// The environment wins over the config file.
		{name: "AWS_REGION beats config region", env: map[string]string{"AWS_REGION": "eu-west-1"}, opts: Options{Region: "ap-south-1"}, wantRegion: "eu-west-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_REGION is set"},
		{name: "AWS_DEFAULT_REGION beats config region", env: map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"}, opts: Options{Region: "ap-south-1"}, wantRegion: "sa-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_DEFAULT_REGION is set"},
		{name: "AWS_REGION beats AWS_DEFAULT_REGION", env: map[string]string{"AWS_REGION": "eu-west-1", "AWS_DEFAULT_REGION": "sa-east-1"}, wantRegion: "eu-west-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_REGION is set"},
		{name: "AWS_ENDPOINT_URL beats config endpoint", env: map[string]string{"AWS_ENDPOINT_URL": "http://env:4566"}, opts: Options{EndpointURL: "http://cfg:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://env:4566", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_ENDPOINT_URL is set"},
		{name: "AWS_PROFILE beats config profile", env: map[string]string{"AWS_PROFILE": "dev"}, opts: Options{Profile: "ops"}, wantRegion: "us-east-1", wantProfile: "dev", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_PROFILE is set"},

		// An explicit enabled always wins over detection.
		{name: "enabled true without any AWS setting", opts: Options{Enabled: boolPtr(true)}, wantRegion: "us-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "aws.enabled: true"},
		{name: "enabled false overrides credentials in the env", env: map[string]string{"AWS_ACCESS_KEY_ID": "key", "AWS_REGION": "eu-west-1"}, opts: Options{Enabled: boolPtr(false)}, wantRegion: "eu-west-1", wantProfile: "default", wantConn: ConnDisabled, wantReason: "aws.enabled: false"},
		{name: "enabled false overrides config region", opts: Options{Enabled: boolPtr(false), Region: "ap-south-1"}, wantRegion: "ap-south-1", wantProfile: "default", wantConn: ConnDisabled, wantReason: "aws.enabled: false"},
		{name: "enabled nil keeps auto detection", env: map[string]string{"AWS_PROFILE": "dev"}, opts: Options{Enabled: nil}, wantRegion: "us-east-1", wantProfile: "dev", wantEnabled: true, wantConn: ConnConnecting, wantReason: "AWS_PROFILE is set"},

		// The shared files of the user are a signal.
		{name: "an existing shared config file enables", configContent: "[default]\nregion = eu-west-1\n", wantRegion: "us-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "shared config file found"},
		{name: "an existing shared credentials file enables", credsContent: "[default]\n", wantRegion: "us-east-1", wantProfile: "default", wantEnabled: true, wantConn: ConnConnecting, wantReason: "shared credentials file found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateAWSEnv(t)
			if tt.configContent != "" {
				writeTestFile(t, filepath.Join(home, ".aws", "config"), tt.configContent)
			}
			if tt.credsContent != "" {
				writeTestFile(t, filepath.Join(home, ".aws", "credentials"), tt.credsContent)
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			mgr := NewAWSClientManagerWithOptions(tt.opts)

			if got := mgr.Region(); got != tt.wantRegion {
				t.Errorf("Region() = %q, want %q", got, tt.wantRegion)
			}
			if got := mgr.EndpointURL(); got != tt.wantEndpoint {
				t.Errorf("EndpointURL() = %q, want %q", got, tt.wantEndpoint)
			}
			state := mgr.GetState()
			if state.Region != tt.wantRegion || state.EndpointURL != tt.wantEndpoint || state.Profile != tt.wantProfile {
				t.Errorf("state = %+v, want region %q, endpoint %q and profile %q", state, tt.wantRegion, tt.wantEndpoint, tt.wantProfile)
			}
			if state.Enabled != tt.wantEnabled || state.Conn != tt.wantConn || state.EnabledReason != tt.wantReason {
				t.Errorf("state = enabled %t, conn %q, reason %q; want enabled %t, conn %q, reason %q",
					state.Enabled, state.Conn, state.EnabledReason, tt.wantEnabled, tt.wantConn, tt.wantReason)
			}
			if state.Connected || state.LastError != (Problem{}) || len(state.Accounts) != 0 {
				t.Errorf("a new manager must start without a connection, an error or accounts, got %+v", state)
			}
		})
	}
}

func TestNewAWSClientManagerMatchesZeroOptions(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ENDPOINT_URL", "http://localhost:4566")

	legacy, withOptions := NewAWSClientManager().GetState(), NewAWSClientManagerWithOptions(Options{}).GetState()
	if legacy.Region != withOptions.Region || legacy.EndpointURL != withOptions.EndpointURL || legacy.Enabled != withOptions.Enabled ||
		legacy.Conn != withOptions.Conn || legacy.EnabledReason != withOptions.EnabledReason {
		t.Errorf("NewAWSClientManager() state %+v differs from NewAWSClientManagerWithOptions(Options{}) state %+v", legacy, withOptions)
	}
}

func TestOptionsIsZero(t *testing.T) {
	if !(Options{}).IsZero() {
		t.Error("Options{} must be zero")
	}
	if !(Options{Accounts: []string{}}).IsZero() {
		t.Error("Options with an empty account list must be zero")
	}
	tests := []struct {
		name string
		opts Options
	}{
		{name: "Enabled false", opts: Options{Enabled: boolPtr(false)}},
		{name: "Enabled true", opts: Options{Enabled: boolPtr(true)}},
		{name: "Region", opts: Options{Region: "eu-west-1"}},
		{name: "EndpointURL", opts: Options{EndpointURL: "http://localhost:4566"}},
		{name: "Profile", opts: Options{Profile: "dev"}},
		{name: "RefreshInterval", opts: Options{RefreshInterval: time.Second}},
		{name: "DiscoveryInterval", opts: Options{DiscoveryInterval: time.Second}},
		{name: "Accounts", opts: Options{Accounts: []string{"123456789012"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.IsZero() {
				t.Errorf("%+v must not be zero", tt.opts)
			}
		})
	}
}

func TestGetStateIsADeepCopy(t *testing.T) {
	isolateAWSEnv(t)
	mgr := NewAWSClientManagerWithOptions(Options{})
	accounts := []Account{{ID: "123456789012", Name: "dev", Status: StatusMonitored, Roles: []string{"ReadOnly", "Admin"}}}
	mgr.publishState(AWSState{Accounts: accounts})

	accounts[0].Name = "changed after publishing"
	accounts[0].Roles[0] = "changed after publishing"
	snapshot := mgr.GetState()
	if snapshot.Accounts[0].Name != "dev" {
		t.Fatalf("publishState kept the caller's slice: name = %q, want dev", snapshot.Accounts[0].Name)
	}
	if snapshot.Accounts[0].Roles[0] != "ReadOnly" {
		t.Fatalf("publishState kept the caller's roles: role = %q, want ReadOnly", snapshot.Accounts[0].Roles[0])
	}

	snapshot.Accounts[0].Name = "changed"
	snapshot.Accounts[0].Roles[0] = "changed"
	if got := mgr.GetState().Accounts[0].Name; got != "dev" {
		t.Errorf("GetState() shares its slice with the manager: name = %q, want dev", got)
	}
	if got := mgr.GetState().Accounts[0].Roles[0]; got != "ReadOnly" {
		t.Errorf("GetState() shares the roles with the manager: role = %q, want ReadOnly", got)
	}
}

// publishState and GetState run on different goroutines in the application;
// the race detector checks the locking.
func TestGetStateIsSafeAlongsidePublish(t *testing.T) {
	isolateAWSEnv(t)
	mgr := NewAWSClientManagerWithOptions(Options{})

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := range 500 {
				mgr.publishState(AWSState{Region: "eu-west-1", Accounts: []Account{{ID: "123456789012", Name: "dev", Status: StatusMonitored, Roles: []string{"ReadOnly"}, UpdatedAt: time.Unix(int64(i), 0)}}})
			}
		}()
		go func() {
			defer wg.Done()
			for range 500 {
				st := mgr.GetState()
				if len(st.Accounts) > 0 {
					st.Accounts[0].Name = "reader"
					st.Accounts[0].Roles[0] = "reader"
				}
			}
		}()
	}
	wg.Wait()
}
