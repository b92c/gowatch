package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go"
)

// stopWithin stops the manager and fails the test when that takes longer than d.
func stopWithin(t *testing.T, m *AWSClientManager, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
}

func TestStartDoesNothingWhenNotEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		creds    string
		wantConn Conn
		opts     Options
	}{
		{name: "not configured", wantConn: ConnNotConfigured},
		{name: "disabled", opts: Options{Enabled: boolPtr(false)}, config: ownerConfig, creds: staticCredentials, wantConn: ConnDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := newTestManager(t, tt.opts, tt.config, tt.creds)
			ctx := context.Background()

			m.Start(ctx, nil)
			time.Sleep(50 * time.Millisecond)

			if got := f.count("GetCallerIdentity"); got != 0 {
				t.Errorf("GetCallerIdentity was called %d times, want none", got)
			}
			if err := m.Stop(ctx); err != nil {
				t.Errorf("Stop() = %v, want nil", err)
			}
			if got := m.GetState().Conn; got != tt.wantConn {
				t.Errorf("Conn = %q, want %q", got, tt.wantConn)
			}
		})
	}
}

func TestStartReturnsAtOnceAndShowsConnecting(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.block = make(chan struct{})

	began := time.Now()
	m.Start(context.Background(), nil)
	if elapsed := time.Since(began); elapsed >= 100*time.Millisecond {
		t.Errorf("Start took %s, want it to return at once", elapsed)
	}
	if got := m.GetState().Conn; got != ConnConnecting {
		t.Errorf("Conn = %q, want %q while the first call is pending", got, ConnConnecting)
	}

	close(f.block)
	stopWithin(t, m, time.Second)
}

// The layout of the owner's machine (Review Focus 1): a legacy SSO profile with
// only the region in it, temporary keys in the credentials file and no AWS_*
// variable.
func TestOwnerLayoutConnectsWithStaticKeysAndProfileRegion(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	now := time.Date(2026, 10, 25, 9, 8, 7, 0, time.UTC)
	m.now = func() time.Time { return now }

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if st.Conn != ConnConnected || !st.Connected {
		t.Errorf("Conn = %q, Connected = %t, want connected", st.Conn, st.Connected)
	}
	if st.Region != "sa-east-1" {
		t.Errorf("Region = %q, want the region of the profile, sa-east-1", st.Region)
	}
	if st.Profile != "default" || st.EnabledReason != "shared config file found" {
		t.Errorf("Profile = %q, EnabledReason = %q, want default and %q", st.Profile, st.EnabledReason, "shared config file found")
	}
	if want := (Identity{AccountID: testAccount, ARN: testARN, UserID: "AROAFAKE:dev"}); st.Identity != want {
		t.Errorf("Identity = %+v, want %+v", st.Identity, want)
	}
	wantAccounts := []Account{{UpdatedAt: now, ID: testAccount, Name: testAccount, Status: StatusMonitored, Identity: true}}
	if !reflect.DeepEqual(st.Accounts, wantAccounts) {
		t.Errorf("Accounts = %+v, want %+v", st.Accounts, wantAccounts)
	}
	if st.LastError != (Problem{}) {
		t.Errorf("LastError = %+v, want none", st.LastError)
	}
	if !st.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %s, want %s", st.UpdatedAt, now)
	}
	if got := f.count("GetCallerIdentity"); got != 1 {
		t.Errorf("GetCallerIdentity was called %d times, want 1", got)
	}
}

func TestIdentityErrorsAreClassifiedAndRetried(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.identityErrs = []error{&smithy.GenericAPIError{Code: "ExpiredToken", Message: "expired"}}
	ctx := context.Background()

	m.pollOnce(ctx, true)
	st := m.GetState()
	if st.Conn != ConnError || st.Connected {
		t.Errorf("first pass: Conn = %q, Connected = %t, want an error", st.Conn, st.Connected)
	}
	if want := (Problem{Text: textBadKeys, Hint: hintBadKeys}); st.LastError != want {
		t.Errorf("first pass: LastError = %+v, want %+v", st.LastError, want)
	}
	if len(st.Accounts) != 0 {
		t.Errorf("first pass: Accounts = %+v, want none before the first success", st.Accounts)
	}

	m.pollOnce(ctx, true)
	if got := m.GetState().Conn; got != ConnConnected {
		t.Errorf("second pass: Conn = %q, want it to recover", got)
	}
}

func TestIdentityFailureAfterSuccessMarksTheIdentityRow(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	ctx := context.Background()
	m.pollOnce(ctx, true)
	f.identityErrs = []error{&smithy.GenericAPIError{Code: "AccessDenied"}}

	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := "access denied: GetCallerIdentity"; st.LastError.Text != want {
		t.Errorf("LastError.Text = %q, want %q", st.LastError.Text, want)
	}
	if len(st.Accounts) != 1 {
		t.Fatalf("Accounts = %+v, want the identity row to stay", st.Accounts)
	}
	if want := "error: access denied: GetCallerIdentity"; st.Accounts[0].Status != want {
		t.Errorf("identity row status = %q, want %q", st.Accounts[0].Status, want)
	}
}

func TestNoCredentialsInAutoMode(t *testing.T) {
	m, f := newTestManager(t, Options{}, "[default]\nregion = eu-west-1\n", "")

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if want := (Problem{Text: textNoProvider, Hint: hintNoProvider}); st.LastError != want {
		t.Errorf("LastError = %+v, want %+v", st.LastError, want)
	}
	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none without credentials", got)
	}
}

// A legacy SSO profile with no static keys and no cached token: the SDK reports
// an invalid token while resolving the credentials.
func TestExpiredLegacySSOTokenInIdentity(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, "")

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); st.LastError != want {
		t.Errorf("LastError = %+v, want %+v", st.LastError, want)
	}
	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none without a token", got)
	}
}

func TestThrottledIdentity(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.identityErrs = []error{&smithy.GenericAPIError{Code: "Throttling"}}

	m.pollOnce(context.Background(), true)

	if want := (Problem{Text: textThrottled}); m.GetState().LastError != want {
		t.Errorf("LastError = %+v, want %+v", m.GetState().LastError, want)
	}
}

func TestCallTimeout(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.block = make(chan struct{}) // the fake gives up when its context ends
	m.callTimeout = 100 * time.Millisecond

	m.pollOnce(context.Background(), true)

	if want := "AWS unreachable: timeout"; m.GetState().LastError.Text != want {
		t.Errorf("LastError.Text = %q, want %q", m.GetState().LastError.Text, want)
	}
}

// Loading the SDK configuration has the same timeout as every other call.
func TestLoadConfigIsBoundedByTheCallTimeout(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	m.callTimeout = 100 * time.Millisecond
	m.loadConfig = func(ctx context.Context, _ ...func(*config.LoadOptions) error) (awssdk.Config, error) {
		return awssdk.Config{}, waitForContext(ctx)
	}

	m.pollOnce(context.Background(), true)

	if want := "AWS unreachable: timeout"; m.GetState().LastError.Text != want {
		t.Errorf("LastError.Text = %q, want %q", m.GetState().LastError.Text, want)
	}
	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none after the load failed", got)
	}
}

// Resolving the credentials has the same timeout too, and a timeout is reported
// as such even when the provider comes from an SSO login.
func TestCredentialRetrievalIsBoundedByTheCallTimeout(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	m.callTimeout = 100 * time.Millisecond
	m.loadConfig = func(context.Context, ...func(*config.LoadOptions) error) (awssdk.Config, error) {
		provider := blockingProvider{sources: []awssdk.CredentialSource{awssdk.CredentialSourceProfileSSOLegacy, awssdk.CredentialSourceSSOLegacy}}
		return awssdk.Config{Region: "sa-east-1", Credentials: provider}, nil
	}

	m.pollOnce(context.Background(), true)

	if want := "AWS unreachable: timeout"; m.GetState().LastError.Text != want {
		t.Errorf("LastError.Text = %q, want %q", m.GetState().LastError.Text, want)
	}
	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none after the credentials timed out", got)
	}
}

// A pass that starts after the context ended makes no call and publishes
// nothing.
func TestPollOnceDoesNothingWhenItsContextIsCancelled(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := m.GetState()

	m.pollOnce(ctx, true)

	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none", got)
	}
	if after := m.GetState(); !after.UpdatedAt.Equal(before.UpdatedAt) || after.Conn != before.Conn {
		t.Errorf("state after the pass = %+v, want the state before it, %+v", after, before)
	}
}

// The SDK quotes the output of a credential_process that is not JSON in its
// error. The state must not carry any of it, whatever the loader and the
// credentials cache wrap around the error.
func TestCredentialProcessOutputNeverReachesTheState(t *testing.T) {
	const output = "fake-secret-0123456789abcdefghijklmnopqrstuvwxyz"
	m, f := newTestManager(t, Options{}, "[default]\nregion = eu-west-1\ncredential_process = echo "+output+"\n", "")

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if want := (Problem{Text: textProcessFailed, Hint: hintProcessFailed}); st.LastError != want {
		t.Errorf("LastError = %+v, want %+v", st.LastError, want)
	}
	if strings.Contains(st.LastError.Message(), "fake-secret") {
		t.Errorf("the state text %q carries the output of the process", st.LastError.Message())
	}
	if got := f.count("GetCallerIdentity"); got != 0 {
		t.Errorf("GetCallerIdentity was called %d times, want none without credentials", got)
	}
}

// Review Focus 2: the exported keys expire, the user exports new ones while
// GoWatch runs, and the next discovery picks them up without a restart.
func TestReexportedCredentialsRecoverWithoutRestart(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.identityErrs = []error{&smithy.GenericAPIError{Code: "ExpiredToken"}}
	var keys []string
	m.loadConfig = func(ctx context.Context, optFns ...func(*config.LoadOptions) error) (awssdk.Config, error) {
		cfg, err := config.LoadDefaultConfig(ctx, optFns...)
		if err != nil {
			return cfg, err
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return cfg, err
		}
		keys = append(keys, creds.AccessKeyID)
		return cfg, nil
	}
	ctx := context.Background()

	m.pollOnce(ctx, true)
	st := m.GetState()
	if st.Conn != ConnError || st.LastError != (Problem{Text: textBadKeys, Hint: hintBadKeys}) {
		t.Fatalf("first pass: Conn = %q, LastError = %+v, want the expired keys error", st.Conn, st.LastError)
	}

	credentialsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	writeTestFile(t, credentialsFile, "[default]\naws_access_key_id = AKIDFAKEREEXPORTED\naws_secret_access_key = fake-secret-for-tests\naws_session_token = fake-session-for-tests\n")

	m.pollOnce(ctx, true)
	if got := m.GetState().Conn; got != ConnConnected {
		t.Errorf("second pass: Conn = %q, want it to recover with the new keys", got)
	}
	if want := []string{"AKIDFAKEEXAMPLE", "AKIDFAKEREEXPORTED"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys seen by the loader = %v, want %v", keys, want)
	}
}

func TestRefreshPassWithoutDiscoveryMakesNoCalls(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	ctx := context.Background()

	m.pollOnce(ctx, true)
	m.pollOnce(ctx, false)

	if got := f.count("GetCallerIdentity"); got != 1 {
		t.Errorf("GetCallerIdentity was called %d times, want 1: a refresh pass makes no call", got)
	}
}

func TestStartRunsPassesUntilStopped(t *testing.T) {
	m, f := newTestManager(t, Options{RefreshInterval: 10 * time.Millisecond, DiscoveryInterval: 10 * time.Millisecond}, ownerConfig, staticCredentials)

	m.Start(context.Background(), nil)
	waitFor(t, time.Second, func() bool { return f.count("GetCallerIdentity") >= 3 })
	stopWithin(t, m, time.Second)

	n := f.count("GetCallerIdentity")
	time.Sleep(50 * time.Millisecond)
	if got := f.count("GetCallerIdentity"); got != n {
		t.Errorf("GetCallerIdentity calls went from %d to %d after Stop", n, got)
	}
}

func TestCancelledContextStopsThePoller(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx, nil)
	waitFor(t, time.Second, func() bool { return f.count("GetCallerIdentity") >= 1 })
	cancel()

	// The goroutine has to end because its parent context did, before anything
	// calls Stop: Stop cancels the poller's own context, which would hide a
	// poller that ignored the parent.
	select {
	case <-m.done:
	case <-time.After(time.Second):
		t.Fatal("the poller goroutine did not exit after its context was cancelled")
	}
	stopWithin(t, m, time.Second)

	waitFor(t, time.Second, func() bool { return runtime.NumGoroutine() <= baseline })
}

func TestStopIsBoundedByItsContext(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.block = make(chan struct{})
	f.ignoreCtx = true

	m.Start(context.Background(), nil)
	waitFor(t, time.Second, func() bool { return f.count("GetCallerIdentity") >= 1 })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := m.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop() = %v, want %v while a call ignores its context", err, context.DeadlineExceeded)
	}

	close(f.block)
	stopWithin(t, m, time.Second)
}

// Start is a no-op once the manager runs, and Stop can be called again.
func TestStartAndStopAreIdempotent(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	ctx := context.Background()

	m.Start(ctx, nil)
	m.Start(ctx, nil)
	waitFor(t, time.Second, func() bool { return f.count("GetCallerIdentity") >= 1 })
	stopWithin(t, m, time.Second)
	stopWithin(t, m, time.Second)

	if got := f.count("GetCallerIdentity"); got != 1 {
		t.Errorf("GetCallerIdentity was called %d times, want 1: a second Start must not start a second poller", got)
	}
}

func TestGetStateDoesNotWaitForAWSCalls(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)
	f.block = make(chan struct{})

	m.Start(context.Background(), nil)
	waitFor(t, time.Second, func() bool { return f.count("GetCallerIdentity") >= 1 })

	began := time.Now()
	m.GetState()
	if elapsed := time.Since(began); elapsed >= 50*time.Millisecond {
		t.Errorf("GetState took %s while a call was pending, want it not to wait", elapsed)
	}

	close(f.block)
	stopWithin(t, m, time.Second)
}

// Passes publish while the dashboard reads; the race detector checks it.
func TestPublishRacesWithGetState(t *testing.T) {
	m, _ := newTestManager(t, Options{RefreshInterval: time.Millisecond, DiscoveryInterval: time.Millisecond}, ownerConfig, staticCredentials)
	m.Start(context.Background(), nil)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				st := m.GetState()
				if len(st.Accounts) > 0 {
					st.Accounts[0].Name = "reader"
				}
			}
		}()
	}
	wg.Wait()

	stopWithin(t, m, time.Second)
}

// credentialSources reads the sources of a provider that reports them, and
// knows nothing about one that does not.
func TestCredentialSources(t *testing.T) {
	if got := credentialSources(nil); got != nil {
		t.Errorf("credentialSources(nil) = %v, want nil", got)
	}
	plain := awssdk.CredentialsProviderFunc(func(context.Context) (awssdk.Credentials, error) { return awssdk.Credentials{}, nil })
	if got := credentialSources(plain); got != nil {
		t.Errorf("credentialSources(plain provider) = %v, want nil", got)
	}

	home := isolateAWSEnv(t)
	writeTestFile(t, filepath.Join(home, ".aws", "config"), ownerConfig)
	cfg, err := config.LoadDefaultConfig(context.Background(), resolveSettings(Options{}).loadOptions()...)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() error = %v", err)
	}
	got := credentialSources(cfg.Credentials)
	want := []awssdk.CredentialSource{awssdk.CredentialSourceProfileSSOLegacy, awssdk.CredentialSourceSSOLegacy}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("credentialSources(legacy SSO profile) = %v, want %v", got, want)
	}
}
