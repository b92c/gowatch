package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// untrustedSettings lists the settings the working-directory file may not set,
// because a repository you merely cloned could otherwise use them to redirect
// AWS calls or to start the trace receiver, possibly on a network address set
// in another layer. They are still accepted from ~/.gowatch/config.yaml and
// from --config. An explicit tracing.otlp_receiver.enabled: false is harmless
// and stays allowed. aws.profile and aws.accounts are honored from every layer:
// they only choose among the user's own credentials and the accounts of their
// own SSO login, and cannot redirect traffic or expose anything.
func (c *Config) untrustedSettings(path string) error {
	const advice = "move it to ~/.gowatch/config.yaml or pass the file with --config"

	var errs []error
	if c.AWS.EndpointURL != "" {
		errs = append(errs, fmt.Errorf("%s: aws.endpoint_url is not honored from the working-directory gowatch.yaml (it could come from an untrusted checkout); %s", path, advice))
	}
	if c.Tracing.OTLPReceiver.Enabled {
		errs = append(errs, fmt.Errorf("%s: tracing.otlp_receiver.enabled: true is not honored from the working-directory gowatch.yaml (it could come from an untrusted checkout); %s", path, advice))
	}
	if addr := c.Tracing.OTLPReceiver.Address; addr != "" && !isLoopbackAddress(addr) {
		errs = append(errs, fmt.Errorf("%s: tracing.otlp_receiver.address %q is not honored from the working-directory gowatch.yaml unless it is 127.0.0.1, [::1] or localhost; %s", path, addr, advice))
	}
	return errors.Join(errs...)
}

// isLoopbackAddress reports whether a host:port address names only the local
// machine. Anything it cannot positively identify, such as an empty host, a
// wildcard address or a malformed value, counts as not loopback.
func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}
