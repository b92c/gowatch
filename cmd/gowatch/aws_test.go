package main

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/aws"
	"github.com/b92c/gowatch/internal/config"
)

func TestAWSOptionsFromConfig(t *testing.T) {
	forcedOff, forcedOn := false, true
	tests := []struct {
		name string
		in   config.AWSConfig
		want aws.Options
	}{
		{
			name: "every setting is passed on",
			in: config.AWSConfig{
				Region: "eu-west-1", EndpointURL: "http://localhost:4566", Enabled: "false", Profile: "dev",
				RefreshInterval: 30 * time.Second, DiscoveryInterval: 2 * time.Minute,
			},
			want: aws.Options{
				Enabled: &forcedOff, Region: "eu-west-1", EndpointURL: "http://localhost:4566", Profile: "dev",
				RefreshInterval: 30 * time.Second, DiscoveryInterval: 2 * time.Minute,
			},
		},
		{
			name: "the accounts are passed on",
			in:   config.AWSConfig{Enabled: "auto", Accounts: []string{"123456789012", "210987654321/Admin"}},
			want: aws.Options{Accounts: []string{"123456789012", "210987654321/Admin"}},
		},
		{name: "auto passes nothing", in: config.AWSConfig{Enabled: "auto"}, want: aws.Options{}},
		{name: "true forces the integration on", in: config.AWSConfig{Enabled: "true"}, want: aws.Options{Enabled: &forcedOn}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := awsOptions(tt.in)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("awsOptions(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
			if tt.want.IsZero() && !got.IsZero() {
				t.Errorf("awsOptions(%+v) must be a zero aws.Options", tt.in)
			}
		})
	}
}

// The options own their account list: a change to the slice of the config
// afterwards must not reach them.
func TestAWSOptionsCopiesTheAccounts(t *testing.T) {
	in := config.AWSConfig{Accounts: []string{"123456789012"}}

	got := awsOptions(in)
	in.Accounts[0] = "changed"

	if want := []string{"123456789012"}; !reflect.DeepEqual(got.Accounts, want) {
		t.Errorf("awsOptions().Accounts = %q, want %q", got.Accounts, want)
	}
}

// The intervals the example configuration spells out are the ones the aws
// package uses when the config leaves them out.
func TestPrintedDefaultConfigMatchesAWSDefaults(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--print-default-config"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}

	path := writeConfig(t, t.TempDir(), "printed.yaml", stdout.String())
	cfg, _, err := config.Load(config.LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("the printed configuration does not load: %v", err)
	}

	if cfg.AWS.RefreshInterval != aws.DefaultRefreshInterval {
		t.Errorf("printed aws.refresh_interval = %s, want %s", cfg.AWS.RefreshInterval, aws.DefaultRefreshInterval)
	}
	if cfg.AWS.DiscoveryInterval != aws.DefaultDiscoveryInterval {
		t.Errorf("printed aws.discovery_interval = %s, want %s", cfg.AWS.DiscoveryInterval, aws.DefaultDiscoveryInterval)
	}
}
