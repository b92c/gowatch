package aws

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// The clients of the SSO portal and of its token service carry the access token
// of the login, so no endpoint override reaches them: not aws.endpoint_url, not
// AWS_ENDPOINT_URL, not the variables of the two services, and they call AWS in the
// region they are given. STS keeps the override. The clients are only built, never
// called, and loading the config with an isolated home and a region reads no file
// and makes no call.
//
// isolateAWSEnv blanks AWS_ENDPOINT_URL, which counts as set but empty: the SDK
// ignores an empty value for the base endpoint, and for the clients of the two
// services the variable of the service is what decides.
func TestSSOClientsIgnoreTheEndpointOverride(t *testing.T) {
	isolateAWSEnv(t)
	const (
		stsEndpoint  = "http://127.0.0.1:1"
		ssoEndpoint  = "http://127.0.0.1:2"
		oidcEndpoint = "http://127.0.0.1:3"
	)
	tests := []struct {
		env  map[string]string
		name string
		opts []func(*config.LoadOptions) error
	}{
		{
			name: "from the config",
			opts: []func(*config.LoadOptions) error{config.WithRegion("us-east-1"), config.WithBaseEndpoint(stsEndpoint)},
		},
		{
			name: "from the environment",
			env: map[string]string{
				"AWS_ENDPOINT_URL":          stsEndpoint,
				"AWS_ENDPOINT_URL_SSO":      ssoEndpoint,
				"AWS_ENDPOINT_URL_SSO_OIDC": oidcEndpoint,
			},
			opts: []func(*config.LoadOptions) error{config.WithRegion("us-east-1")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			cfg, err := config.LoadDefaultConfig(context.Background(), tt.opts...)
			if err != nil {
				t.Fatalf("LoadDefaultConfig() error = %v", err)
			}
			services := sdkServices{}

			ssoClient, ok := services.SSO(cfg).(*sso.Client)
			if !ok {
				t.Fatalf("SSO() is a %T, want the SDK's SSO client", services.SSO(cfg))
			}
			oidcClient, ok := services.OIDC(cfg).(*ssooidc.Client)
			if !ok {
				t.Fatalf("OIDC() is a %T, want the SDK's SSO OIDC client", services.OIDC(cfg))
			}
			stsClient, ok := services.STS(cfg).(*sts.Client)
			if !ok {
				t.Fatalf("STS() is a %T, want the SDK's STS client", services.STS(cfg))
			}

			if got := ssoClient.Options().BaseEndpoint; got != nil {
				t.Errorf("the SSO client has the endpoint %q, want none", *got)
			}
			if got := oidcClient.Options().BaseEndpoint; got != nil {
				t.Errorf("the OIDC client has the endpoint %q, want none", *got)
			}
			if got := stsClient.Options().BaseEndpoint; awssdk.ToString(got) != stsEndpoint {
				t.Errorf("the STS client has the endpoint %q, want %q", awssdk.ToString(got), stsEndpoint)
			}
			if got := ssoClient.Options().Region; got != "us-east-1" {
				t.Errorf("the SSO client has the region %q, want the one it was given, us-east-1", got)
			}
			if got := oidcClient.Options().Region; got != "us-east-1" {
				t.Errorf("the OIDC client has the region %q, want the one it was given, us-east-1", got)
			}
		})
	}
}

// wireRequest is where a request was going.
type wireRequest struct {
	host string
	path string
}

// recordingTransport is the transport of an HTTP client that never dials: it
// keeps the host and the path of every request, and nothing else (no header and no
// body, so no token can end up in a log), and answers 500 by itself.
type recordingTransport struct {
	requests []wireRequest
	mu       sync.Mutex
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, wireRequest{host: req.URL.Host, path: req.URL.Path})
	r.mu.Unlock()
	return &http.Response{
		Status:     "500 Internal Server Error",
		StatusCode: http.StatusInternalServerError,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func (r *recordingTransport) recorded() []wireRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]wireRequest(nil), r.requests...)
}

// The calls that carry the access token of the login reach AWS in the SSO region
// of the profile, whatever endpoint override is set, and the host on the wire is
// the proof: GoWatch's own clients, and the clients that the SDK builds for itself
// while it loads the config of an SSO profile with no exported keys (its
// credential provider for a legacy profile, its token provider for an sso_session
// profile). The config is loaded with the real option list; the HTTP client is
// the one above, so nothing is dialed, and every call is expected to fail.
func TestSSOCallsReachAWSOnTheWire(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ENDPOINT_URL_SSO", "http://127.0.0.1:2")
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", "http://127.0.0.1:3")

	load := func(t *testing.T, o Options, rec *recordingTransport) awssdk.Config {
		t.Helper()
		opts := append(resolveSettings(o).loadOptions(), config.WithHTTPClient(&http.Client{Transport: rec}))
		cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
		if err != nil {
			t.Fatalf("LoadDefaultConfig() error = %v", err)
		}
		return cfg
	}
	// reachedAWS checks that something was sent, that all of it went to AWS and
	// that the first request went where the profile says.
	reachedAWS := func(t *testing.T, rec *recordingTransport, first wireRequest) {
		t.Helper()
		got := rec.recorded()
		if len(got) == 0 {
			t.Fatal("no request was made")
		}
		for _, r := range got {
			if !strings.HasSuffix(r.host, ".amazonaws.com") || strings.HasPrefix(r.host, "127.0.0.1") {
				t.Errorf("a request went to %s%s, want every request on amazonaws.com", r.host, r.path)
			}
		}
		if got[0] != first {
			t.Errorf("the first request went to %s%s, want %s%s", got[0].host, got[0].path, first.host, first.path)
		}
	}

	t.Run("GoWatch's SSO client", func(t *testing.T) {
		rec := &recordingTransport{}
		cfg := load(t, Options{}, rec)
		services := sdkServices{}

		_, err := services.SSO(cfg).ListAccounts(context.Background(), &sso.ListAccountsInput{AccessToken: awssdk.String("fake")})
		if err == nil {
			t.Error("ListAccounts() succeeded against a transport that answers 500")
		}
		portal := rec.recorded()
		reachedAWS(t, rec, wireRequest{host: "portal.sso.us-east-1.amazonaws.com", path: "/assignment/accounts"})

		_, err = services.OIDC(cfg).CreateToken(context.Background(), &ssooidc.CreateTokenInput{
			ClientId: awssdk.String("fake"), ClientSecret: awssdk.String("fake"),
			GrantType: awssdk.String("refresh_token"), RefreshToken: awssdk.String("fake"),
		})
		if err == nil {
			t.Error("CreateToken() succeeded against a transport that answers 500")
		}
		got := rec.recorded()
		if len(got) != len(portal)+1 || got[len(portal)] != (wireRequest{host: "oidc.us-east-1.amazonaws.com", path: "/token"}) {
			t.Errorf("requests = %v, want the token call to go to oidc.us-east-1.amazonaws.com/token after the portal call", got)
		}
	})

	t.Run("the SDK's credential provider, legacy profile", func(t *testing.T) {
		writeTestFile(t, os.Getenv("AWS_CONFIG_FILE"), ownerConfig) // no credentials file: the profile is the only source
		writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(time.Hour), false)
		rec := &recordingTransport{}
		cfg := load(t, Options{}, rec)

		if _, err := cfg.Credentials.Retrieve(context.Background()); err == nil {
			t.Error("Retrieve() succeeded against a transport that answers 500")
		}

		reachedAWS(t, rec, wireRequest{host: "portal.sso.us-east-1.amazonaws.com", path: "/federation/credentials"})
	})

	t.Run("the SDK's token provider, sso_session profile", func(t *testing.T) {
		writeTestFile(t, os.Getenv("AWS_CONFIG_FILE"), sessionConfig)
		writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), true) // expired, with what a refresh needs
		rec := &recordingTransport{}
		cfg := load(t, Options{Profile: "dev"}, rec)

		if _, err := cfg.Credentials.Retrieve(context.Background()); err == nil {
			t.Error("Retrieve() succeeded against a transport that answers 500")
		}

		reachedAWS(t, rec, wireRequest{host: "oidc.eu-central-1.amazonaws.com", path: "/token"})
	})
}
