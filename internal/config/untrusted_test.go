package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ./gowatch.yaml may come from a cloned, untrusted repository, so it cannot
// set aws.endpoint_url, a non-loopback OTLP address or turn the OTLP receiver
// on. The same values are fine in ~/.gowatch/config.yaml and in a --config
// file.
func TestLoadRestrictsWorkingDirectoryLayer(t *testing.T) {
	const (
		endpoint = "aws:\n  endpoint_url: http://169.254.169.254:4566\n"
		hint1    = "~/.gowatch/config.yaml"
		hint2    = "--config"
	)
	// otlp sets only the address, so the address rule is exercised on its own;
	// otlpOn also turns the receiver on, which the working-directory layer
	// rejects by itself.
	otlp := func(addr string) string {
		return "tracing:\n  otlp_receiver:\n    address: \"" + addr + "\"\n"
	}
	otlpOn := func(addr string) string {
		return "tracing:\n  otlp_receiver:\n    enabled: true\n    address: \"" + addr + "\"\n"
	}
	enabled := func(value string) string {
		return "tracing:\n  otlp_receiver:\n    enabled: " + value + "\n"
	}

	// layer is "home", "work" or "explicit". An empty wantAddress means the
	// built-in default address; an empty wantErr means the file is accepted, in
	// which case wantEnabled is the expected tracing.otlp_receiver.enabled.
	tests := []struct {
		name        string
		layer       string
		content     string
		wantAddress string
		wantErr     []string
		wantEnabled bool
	}{
		{name: "endpoint from the working directory is rejected", layer: "work", content: endpoint,
			wantErr: []string{"aws.endpoint_url", hint1, hint2}},
		{name: "endpoint from the home config is accepted", layer: "home", content: endpoint},
		{name: "endpoint from --config is accepted", layer: "explicit", content: endpoint},
		{name: "an empty endpoint in the working directory is harmless", layer: "work", content: "aws:\n  endpoint_url: \"\"\n"},

		{name: "wildcard address from the working directory is rejected", layer: "work", content: otlp("0.0.0.0:4318"),
			wantErr: []string{"tracing.otlp_receiver.address", "0.0.0.0:4318", hint1, hint2}},
		{name: "empty host from the working directory is rejected", layer: "work", content: otlp(":4318"),
			wantErr: []string{"tracing.otlp_receiver.address", hint1}},
		{name: "LAN address from the working directory is rejected", layer: "work", content: otlp("192.168.1.5:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "hostname from the working directory is rejected", layer: "work", content: otlp("collector.example.com:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "other 127.x addresses are not on the allow list", layer: "work", content: otlp("127.0.0.2:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "an address without a port is rejected", layer: "work", content: otlp("127.0.0.1"),
			wantErr: []string{"tracing.otlp_receiver.address"}},
		{name: "IPv6 wildcard from the working directory is rejected", layer: "work", content: otlp("[::]:4318"),
			wantErr: []string{"tracing.otlp_receiver.address"}},

		{name: "127.0.0.1 from the working directory is accepted", layer: "work", content: otlp("127.0.0.1:4319"), wantAddress: "127.0.0.1:4319"},
		{name: "IPv6 loopback from the working directory is accepted", layer: "work", content: otlp("[::1]:4318"), wantAddress: "[::1]:4318"},
		{name: "localhost from the working directory is accepted", layer: "work", content: otlp("localhost:4318"), wantAddress: "localhost:4318"},
		{name: "localhost is case-insensitive", layer: "work", content: otlp("LocalHost:4318"), wantAddress: "LocalHost:4318"},
		{name: "non-loopback address from the home config is accepted", layer: "home", content: otlpOn("0.0.0.0:4318"), wantAddress: "0.0.0.0:4318", wantEnabled: true},
		{name: "non-loopback address from --config is accepted", layer: "explicit", content: otlpOn("0.0.0.0:4318"), wantAddress: "0.0.0.0:4318", wantEnabled: true},

		{name: "enabling the receiver from the working directory is rejected", layer: "work", content: enabled("true"),
			wantErr: []string{"tracing.otlp_receiver.enabled", hint1, hint2}},
		{name: "enabling the receiver on a loopback address from the working directory is rejected", layer: "work", content: otlpOn("127.0.0.1:4318"),
			wantErr: []string{"tracing.otlp_receiver.enabled", hint1, hint2}},
		{name: "the alternative boolean spelling of true is rejected too", layer: "work", content: enabled("yes"),
			wantErr: []string{"tracing.otlp_receiver.enabled"}},
		{name: "disabling the receiver from the working directory is harmless", layer: "work", content: enabled("false")},
		{name: "enabling the receiver from the home config is accepted", layer: "home", content: enabled("true"), wantEnabled: true},
		{name: "enabling the receiver from --config is accepted", layer: "explicit", content: enabled("true"), wantEnabled: true},

		{name: "both violations are reported together", layer: "work", content: endpoint + otlp("0.0.0.0:4318"),
			wantErr: []string{"aws.endpoint_url", "tracing.otlp_receiver.address"}},
		{name: "enabling the receiver on a public address reports the enabled key and the address", layer: "work", content: otlpOn("0.0.0.0:4318"),
			wantErr: []string{"tracing.otlp_receiver.enabled", "tracing.otlp_receiver.address", "0.0.0.0:4318"}},
		{name: "other settings in the working directory still apply", layer: "work", content: "logs:\n  tail_lines: 9\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			opts := LoadOptions{HomeDir: home, WorkDir: work}
			var path string
			switch tt.layer {
			case "home":
				path = globalConfigPath(home)
			case "work":
				path = localConfigPath(work)
			case "explicit":
				path = filepath.Join(t.TempDir(), "custom.yaml")
				opts.ExplicitPath = path
			}
			writeFile(t, path, tt.content)

			cfg, _, err := Load(opts)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load() = %+v, want an error", *cfg)
				}
				for _, want := range append(tt.wantErr, path) {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			wantAddress := tt.wantAddress
			if wantAddress == "" {
				wantAddress = Default().Tracing.OTLPReceiver.Address
			}
			if got := cfg.Tracing.OTLPReceiver.Address; got != wantAddress {
				t.Errorf("OTLP address = %q, want %q", got, wantAddress)
			}
			if got := cfg.Tracing.OTLPReceiver.Enabled; got != tt.wantEnabled {
				t.Errorf("OTLP enabled = %t, want %t", got, tt.wantEnabled)
			}
			if strings.Contains(tt.content, "endpoint_url: http") && cfg.AWS.EndpointURL == "" {
				t.Error("an accepted endpoint_url must end up in the config")
			}
		})
	}
}

// aws.profile only chooses among the user's own credentials, so the
// working-directory file may set it.
func TestWorkingDirectoryLayerHonorsAWSProfile(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeFile(t, localConfigPath(work), "aws:\n  profile: dev\n")

	cfg, _, err := Load(LoadOptions{HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AWS.Profile != "dev" {
		t.Errorf("AWS.Profile = %q, want dev", cfg.AWS.Profile)
	}
}

// aws.accounts only chooses among the accounts of the user's own SSO login, so
// the working-directory file may set it.
func TestWorkingDirectoryLayerHonorsAWSAccounts(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeFile(t, localConfigPath(work), "aws:\n  accounts: [\"123456789012\"]\n")

	cfg, _, err := Load(LoadOptions{HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := []string{"123456789012"}; !reflect.DeepEqual(cfg.AWS.Accounts, want) {
		t.Errorf("AWS.Accounts = %q, want %q", cfg.AWS.Accounts, want)
	}
}

// The restriction looks at what the working-directory file sets itself, not at
// the merged result: a trusted layer's endpoint survives, and a working
// directory file that leaves the key out is fine.
func TestWorkingDirectoryLayerKeepsTrustedValues(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeFile(t, globalConfigPath(home), "aws:\n  endpoint_url: http://localstack:4566\ntracing:\n  otlp_receiver:\n    enabled: true\n    address: \"0.0.0.0:4318\"\n")
	writeFile(t, localConfigPath(work), "logs:\n  tail_lines: 9\n")

	cfg, _, err := Load(LoadOptions{HomeDir: home, WorkDir: work})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AWS.EndpointURL != "http://localstack:4566" || !cfg.Tracing.OTLPReceiver.Enabled || cfg.Tracing.OTLPReceiver.Address != "0.0.0.0:4318" || cfg.Logs.TailLines != 9 {
		t.Errorf("merged config = %+v", *cfg)
	}
}
