package aws

import (
	"context"
	"testing"
)

func TestAWSClientManager(t *testing.T) {
	mgr := NewAWSClientManager()

	if mgr.Region() == "" {
		t.Errorf("expected default region, got empty")
	}

	state := mgr.GetState()
	if state.Region != mgr.Region() {
		t.Errorf("expected state region %s, got %s", mgr.Region(), state.Region)
	}

	mgr.SetConnected(true, "")
	if !mgr.GetState().Connected {
		t.Errorf("expected state connected = true")
	}

	resources := []AWSResource{
		{ID: "test-id", Name: "test-resource", Type: "Lambda"},
	}
	mgr.SetResources(resources)

	if len(mgr.GetState().Resources) != 1 {
		t.Fatalf("expected 1 resource in state, got %d", len(mgr.GetState().Resources))
	}
}

func TestAWSCollectors(t *testing.T) {
	ctx := context.Background()
	mgr := NewAWSClientManager()

	cw := NewCloudWatchCollector(mgr)
	res := cw.CollectResource(ctx, "gowatch-logs")
	if res.Type != "CloudWatchGroup" || res.Name != "gowatch-logs" {
		t.Errorf("unexpected cloudwatch resource: %+v", res)
	}

	lambda := NewLambdaCollector(mgr)
	funcs, err := lambda.ListFunctions(ctx)
	if err != nil || len(funcs) == 0 {
		t.Errorf("failed to list lambda functions: %v", err)
	}

	xray := NewXRayCollector(mgr)
	services, err := xray.FetchServiceGraph(ctx)
	if err != nil || len(services) == 0 {
		t.Errorf("failed to fetch xray service graph: %v", err)
	}

	cf := NewCloudFormationCollector(mgr)
	stacks, err := cf.ListStacks(ctx)
	if err != nil || len(stacks) == 0 {
		t.Errorf("failed to list cloudformation stacks: %v", err)
	}
}

// awsEnvKeys are every variable the manager reads. Tests blank them all first
// so the host environment cannot leak in.
var awsEnvKeys = []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ENDPOINT_URL", "AWS_ACCESS_KEY_ID", "AWS_PROFILE"}

func boolPtr(v bool) *bool { return &v }

func TestNewAWSClientManagerWithOptions(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		opts         Options
		wantRegion   string
		wantEndpoint string
		wantEnabled  bool
	}{
		// Zero Options must behave exactly like the environment-only constructor.
		{name: "no env, no options", wantRegion: "us-east-1"},
		{name: "AWS_REGION enables", env: map[string]string{"AWS_REGION": "eu-west-1"}, wantRegion: "eu-west-1", wantEnabled: true},
		{name: "AWS_DEFAULT_REGION sets the region but does not enable", env: map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"}, wantRegion: "sa-east-1"},
		{name: "AWS_ACCESS_KEY_ID enables", env: map[string]string{"AWS_ACCESS_KEY_ID": "key"}, wantRegion: "us-east-1", wantEnabled: true},
		{name: "AWS_PROFILE enables", env: map[string]string{"AWS_PROFILE": "dev"}, wantRegion: "us-east-1", wantEnabled: true},
		{name: "AWS_ENDPOINT_URL enables", env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://localhost:4566", wantEnabled: true},

		// Options fill in what the environment leaves out.
		{name: "config region alone enables in auto mode", opts: Options{Region: "ap-south-1"}, wantRegion: "ap-south-1", wantEnabled: true},
		{name: "config endpoint alone enables in auto mode", opts: Options{EndpointURL: "http://localstack:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://localstack:4566", wantEnabled: true},
		{name: "config region and endpoint", opts: Options{Region: "ap-south-1", EndpointURL: "http://localstack:4566"}, wantRegion: "ap-south-1", wantEndpoint: "http://localstack:4566", wantEnabled: true},

		// The environment wins over the config file.
		{name: "AWS_REGION beats config region", env: map[string]string{"AWS_REGION": "eu-west-1"}, opts: Options{Region: "ap-south-1"}, wantRegion: "eu-west-1", wantEnabled: true},
		{name: "AWS_DEFAULT_REGION beats config region", env: map[string]string{"AWS_DEFAULT_REGION": "sa-east-1"}, opts: Options{Region: "ap-south-1"}, wantRegion: "sa-east-1", wantEnabled: true},
		{name: "AWS_REGION beats AWS_DEFAULT_REGION", env: map[string]string{"AWS_REGION": "eu-west-1", "AWS_DEFAULT_REGION": "sa-east-1"}, wantRegion: "eu-west-1", wantEnabled: true},
		{name: "AWS_ENDPOINT_URL beats config endpoint", env: map[string]string{"AWS_ENDPOINT_URL": "http://env:4566"}, opts: Options{EndpointURL: "http://cfg:4566"}, wantRegion: "us-east-1", wantEndpoint: "http://env:4566", wantEnabled: true},

		// An explicit enabled always wins over detection.
		{name: "enabled true without any AWS setting", opts: Options{Enabled: boolPtr(true)}, wantRegion: "us-east-1", wantEnabled: true},
		{name: "enabled false overrides credentials in the env", env: map[string]string{"AWS_ACCESS_KEY_ID": "key", "AWS_REGION": "eu-west-1"}, opts: Options{Enabled: boolPtr(false)}, wantRegion: "eu-west-1"},
		{name: "enabled false overrides config region", opts: Options{Enabled: boolPtr(false), Region: "ap-south-1"}, wantRegion: "ap-south-1"},
		{name: "enabled nil keeps auto detection", env: map[string]string{"AWS_PROFILE": "dev"}, opts: Options{Enabled: nil}, wantRegion: "us-east-1", wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range awsEnvKeys {
				t.Setenv(key, "")
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
			if state.Enabled != tt.wantEnabled {
				t.Errorf("state.Enabled = %v, want %v", state.Enabled, tt.wantEnabled)
			}
			if state.Region != tt.wantRegion || state.EndpointURL != tt.wantEndpoint {
				t.Errorf("state = %+v, want region %q and endpoint %q", state, tt.wantRegion, tt.wantEndpoint)
			}
		})
	}
}

func TestNewAWSClientManagerMatchesZeroOptions(t *testing.T) {
	for _, key := range awsEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_ENDPOINT_URL", "http://localhost:4566")

	legacy, withOptions := NewAWSClientManager().GetState(), NewAWSClientManagerWithOptions(Options{}).GetState()
	if legacy.Region != withOptions.Region || legacy.EndpointURL != withOptions.EndpointURL || legacy.Enabled != withOptions.Enabled {
		t.Errorf("NewAWSClientManager() state %+v differs from NewAWSClientManagerWithOptions(Options{}) state %+v", legacy, withOptions)
	}
}
