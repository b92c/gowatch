package aws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
)

func TestDetectSSO(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		profile string // AWS_PROFILE, when not empty
		want    ssoProfile
		wantSSO bool
	}{
		{
			name: "a legacy profile", config: ownerConfig,
			want:    ssoProfile{startURL: ownerStartURL, region: "us-east-1", accountID: testAccount, roleName: "ReadOnly"},
			wantSSO: true,
		},
		{
			name: "an sso_session profile", config: sessionConfig, profile: "dev",
			want: ssoProfile{
				startURL: sessionStartURL, region: "eu-central-1", sessionName: "corp",
				accountID: testAccount, roleName: "ReadOnly",
			},
			wantSSO: true,
		},
		{name: "a profile without SSO", config: "[default]\nregion = eu-west-1\n"},
		{name: "a profile that does not exist", config: ownerConfig, profile: "missing"},
		{name: "an sso_session without its section", config: "[profile dev]\nsso_session = gone\n", profile: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateAWSEnv(t)
			writeTestFile(t, filepath.Join(home, ".aws", "config"), tt.config)
			if tt.profile != "" {
				t.Setenv("AWS_PROFILE", tt.profile)
			}

			got, ok := detectSSO(context.Background(), resolveSettings(Options{}))

			if got != tt.want || ok != tt.wantSSO {
				t.Errorf("detectSSO() = (%+v, %t), want (%+v, %t)", got, ok, tt.want, tt.wantSSO)
			}
		})
	}
}

// The profile comes from aws.profile when AWS_PROFILE is not set, and the keys
// that sit in the credentials file next to the SSO settings do not hide them.
func TestDetectSSOReadsTheProfileOfTheOptions(t *testing.T) {
	home := isolateAWSEnv(t)
	writeTestFile(t, filepath.Join(home, ".aws", "config"), sessionConfig)
	writeTestFile(t, filepath.Join(home, ".aws", "credentials"), sessionCredentials)

	got, ok := detectSSO(context.Background(), resolveSettings(Options{Profile: "dev"}))

	if want := (ssoProfile{startURL: sessionStartURL, region: "eu-central-1", sessionName: "corp", accountID: testAccount, roleName: "ReadOnly"}); !ok || got != want {
		t.Errorf("detectSSO() = (%+v, %t), want (%+v, true)", got, ok, want)
	}
}

func TestReadLegacyToken(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		content string // the token file; "" means no file
		want    string
		wantErr bool
	}{
		{name: "a valid token", content: `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T18:00:00Z"}`, want: "fake-access-token"},
		{
			name:    "the fields of the AWS CLI around the token are ignored",
			content: `{"startUrl":"https://example.awsapps.com/start","region":"us-east-1","accessToken":"fake-access-token","expiresAt":"2026-10-25T18:00:00Z","clientId":"x","clientSecret":"y","registrationExpiresAt":"2027-01-01T00:00:00Z"}`,
			want:    "fake-access-token",
		},
		{name: "an expiry with an offset", content: `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T15:00:00-03:00"}`, want: "fake-access-token"},
		{name: "expired a second ago", content: `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T11:59:59Z"}`, wantErr: true},
		{name: "expiring right now", content: `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T12:00:00Z"}`, wantErr: true},
		{name: "no file", wantErr: true},
		{name: "not JSON", content: "not json", wantErr: true},
		{name: "no access token", content: `{"expiresAt":"2026-10-25T18:00:00Z"}`, wantErr: true},
		{name: "an empty access token", content: `{"accessToken":"","expiresAt":"2026-10-25T18:00:00Z"}`, wantErr: true},
		{name: "no expiry", content: `{"accessToken":"fake-access-token"}`, wantErr: true},
		{name: "an expiry that is not a time", content: `{"accessToken":"fake-access-token","expiresAt":"tomorrow"}`, wantErr: true},
		{name: "an access token that is not a string", content: `{"accessToken":12,"expiresAt":"2026-10-25T18:00:00Z"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.json")
			if tt.content != "" {
				writeTestFile(t, path, tt.content)
			}

			got, err := readLegacyToken(path, now)

			if tt.wantErr {
				if !errors.Is(err, errSSOTokenExpired) || got != "" {
					t.Errorf("readLegacyToken() = (%q, %v), want an errSSOTokenExpired error and no token", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("readLegacyToken() = (%q, %v), want (%q, nil)", got, err, tt.want)
			}
		})
	}

	// Only the file at the path counts: a valid token next to it is never found.
	t.Run("another file of the directory is never read", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "valid.json"), `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T18:00:00Z"}`)
		writeTestFile(t, filepath.Join(dir, "expired.json"), `{"accessToken":"fake-access-token","expiresAt":"2026-10-25T08:00:00Z"}`)

		if got, err := readLegacyToken(filepath.Join(dir, "expired.json"), now); !errors.Is(err, errSSOTokenExpired) || got != "" {
			t.Errorf("readLegacyToken() = (%q, %v), want an errSSOTokenExpired error and no token", got, err)
		}
	})
}

func TestBearerTokenLegacyReadsTheComputedFile(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	ctx := context.Background()
	m.pollOnce(ctx, true)
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(time.Hour), false)
	p, ok := detectSSO(ctx, m.settings)
	if !ok {
		t.Fatal("detectSSO() did not find the SSO profile")
	}

	got, err := m.bearerToken(ctx, p)

	if err != nil || got != "fake-access-token" {
		t.Errorf("bearerToken() = (%q, %v), want (fake-access-token, nil)", got, err)
	}
	if got := f.oidcCalls; got != 0 {
		t.Errorf("CreateToken was called %d times, want none for a legacy profile", got)
	}
	if m.work.ssoTokenProvider != nil {
		t.Error("a legacy profile must not build an SDK token provider")
	}
	if want := filepath.Join(os.Getenv("HOME"), ".aws", "sso", "cache"); filepath.Dir(m.work.ssoTokenPath) != want {
		t.Errorf("ssoTokenPath = %q, want a file in %q", m.work.ssoTokenPath, want)
	}
}

func TestBearerTokenSSOSessionUsesTheSDKProvider(t *testing.T) {
	m, f := newTestManager(t, Options{Profile: "dev"}, sessionConfig, sessionCredentials)
	ctx := context.Background()
	m.pollOnce(ctx, true)
	p, ok := detectSSO(ctx, m.settings)
	if !ok {
		t.Fatal("detectSSO() did not find the SSO profile")
	}

	t.Run("a fresh token is read from the cache", func(t *testing.T) {
		writeSSOToken(t, "corp", "fake-access-token", time.Now().Add(time.Hour), true)
		before := f.oidcCalls

		got, err := m.bearerToken(ctx, p)

		if err != nil || got != "fake-access-token" {
			t.Errorf("bearerToken() = (%q, %v), want (fake-access-token, nil)", got, err)
		}
		if f.oidcCalls != before {
			t.Errorf("CreateToken was called %d times for a fresh token, want none", f.oidcCalls-before)
		}
		if m.work.ssoTokenProvider == nil || m.work.ssoTokenPath == "" {
			t.Errorf("an sso_session profile must record its token provider and path, got %v and %q", m.work.ssoTokenProvider, m.work.ssoTokenPath)
		}
	})

	t.Run("an expired token is refreshed through CreateToken", func(t *testing.T) {
		writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), true)
		f.oidcToken = "fake-new-access-token"
		before := f.oidcCalls

		got, err := m.bearerToken(ctx, p)

		if err != nil || got != "fake-new-access-token" {
			t.Errorf("bearerToken() = (%q, %v), want (fake-new-access-token, nil)", got, err)
		}
		if f.oidcCalls != before+1 {
			t.Errorf("CreateToken was called %d times, want 1", f.oidcCalls-before)
		}
	})

	t.Run("an expired token that cannot be refreshed", func(t *testing.T) {
		writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), false)
		before := f.oidcCalls

		got, err := m.bearerToken(ctx, p)

		if !errors.Is(err, errSSOTokenExpired) || got != "" {
			t.Errorf("bearerToken() = (%q, %v), want an errSSOTokenExpired error and no token", got, err)
		}
		if f.oidcCalls != before {
			t.Errorf("CreateToken was called %d times without refresh fields, want none", f.oidcCalls-before)
		}
	})

	t.Run("no token file", func(t *testing.T) {
		path := m.work.ssoTokenPath
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove the token file: %v", err)
		}

		if got, err := m.bearerToken(ctx, p); !errors.Is(err, errSSOTokenExpired) || got != "" {
			t.Errorf("bearerToken() = (%q, %v), want an errSSOTokenExpired error and no token", got, err)
		}
	})
}

// A refresh that never answers is cut by the same call timeout as every other
// call, and says so instead of blaming the login.
func TestBearerTokenRefreshIsBoundedByTheCallTimeout(t *testing.T) {
	m, f := newTestManager(t, Options{Profile: "dev"}, sessionConfig, sessionCredentials)
	ctx := context.Background()
	m.pollOnce(ctx, true)
	p, _ := detectSSO(ctx, m.settings)
	writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), true)
	f.hang(t, "CreateToken")
	m.callTimeout = 100 * time.Millisecond

	_, err := m.bearerToken(ctx, p)

	if !errors.Is(err, errSSOTokenExpired) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bearerToken() error = %v, want errSSOTokenExpired wrapping the deadline", err)
	}
	if got, _ := classifyError(opSSOAccess, err, nil); got != (Problem{Text: prefixUnreachable + "timeout"}) {
		t.Errorf("classifyError() = %+v, want the timeout problem", got)
	}
}

func TestClassifySSOErrors(t *testing.T) {
	expired := Problem{Text: textSSOExpired, Hint: hintSSOExpired}
	tests := []struct {
		err  error
		want Problem
		name string
		op   string
	}{
		{name: "an unusable token file", op: opSSOAccess, err: fmt.Errorf("%w: %w", errSSOTokenExpired, errors.New("x")), want: expired},
		{name: "a bare expired token", op: opSSOAccess, err: errSSOTokenExpired, want: expired},
		{
			name: "the portal rejects the token", op: opListAccounts,
			err:  &ssotypes.UnauthorizedException{Message: awssdk.String("Session token not found or invalid")},
			want: expired,
		},
		{name: "the portal rejects the token for the roles", op: opListAccountRoles, err: &ssotypes.UnauthorizedException{}, want: expired},
		{name: "the portal rejects the token for the keys", op: opRoleAccess, err: &ssotypes.UnauthorizedException{}, want: expired},
		{name: "the portal rejects the token on a call wrapped in an operation error", op: opListAccounts, err: fmt.Errorf("x: %w", &ssotypes.UnauthorizedException{}), want: expired},
		{
			// A network failure under the expired-token error is still a network
			// failure: the login is not the problem.
			name: "a DNS failure while refreshing", op: opSSOAccess,
			err:  fmt.Errorf("%w: %w", errSSOTokenExpired, &net.DNSError{Err: "no such host"}),
			want: Problem{Text: prefixUnreachable + "DNS lookup failed"},
		},
		{
			// UnauthorizedException only means an expired login for the SSO calls.
			name: "the same code on another call is not an expired login", op: opGetCallerIdentity,
			err:  &ssotypes.UnauthorizedException{Message: awssdk.String("nope")},
			want: Problem{Text: "GetCallerIdentity failed: UnauthorizedException: nope"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, throttled := classifyError(tt.op, tt.err, nil)

			if got != tt.want || throttled {
				t.Errorf("classifyError(%q, %v) = (%+v, %t), want (%+v, false)", tt.op, tt.err, got, throttled, tt.want)
			}
		})
	}
}
