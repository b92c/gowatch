package aws

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Fixtures. The account ID and the keys are obviously fake; no real credential,
// token or account ever appears in a test.
const (
	testAccount = "123456789012"
	testARN     = "arn:aws:sts::123456789012:assumed-role/ReadOnly/dev"

	// ownerConfig is a legacy SSO profile with only its region set in the
	// profile, the layout of the machine this integration was built for.
	ownerConfig = "[default]\nsso_start_url = https://example.awsapps.com/start\nsso_region = us-east-1\nsso_account_id = 123456789012\nsso_role_name = ReadOnly\nsso_registration_scopes = sso:account:access\nregion = sa-east-1\n"

	// staticCredentials are temporary keys exported to the credentials file. The
	// SDK resolves them before the SSO settings of the same profile.
	staticCredentials = "[default]\naws_access_key_id = AKIDFAKEEXAMPLE\naws_secret_access_key = fake-secret-for-tests\naws_session_token = fake-session-for-tests\n"
)

// fakeServices stands in for the SDK service clients: no test builds a real
// one, so no test can reach the network.
type fakeServices struct {
	calls        map[string]int // by operation name: "GetCallerIdentity", later "ListAccounts", ...
	block        chan struct{}  // not nil: GetCallerIdentity counts the call, then waits for close (or ctx.Done unless ignoreCtx)
	identity     sts.GetCallerIdentityOutput
	identityErrs []error // one per call, consumed in order; when empty the call succeeds
	mu           sync.Mutex
	ignoreCtx    bool
}

// newFakeServices answers GetCallerIdentity with testAccount and testARN.
func newFakeServices() *fakeServices {
	return &fakeServices{
		calls: make(map[string]int),
		identity: sts.GetCallerIdentityOutput{
			Account: awssdk.String(testAccount),
			Arn:     awssdk.String(testARN),
			UserId:  awssdk.String("AROAFAKE:dev"),
		},
	}
}

// STS implements services.
func (f *fakeServices) STS(awssdk.Config) stsAPI { return f }

// GetCallerIdentity implements stsAPI.
func (f *fakeServices) GetCallerIdentity(ctx context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.mu.Lock()
	f.calls["GetCallerIdentity"]++
	var failure error
	if len(f.identityErrs) > 0 {
		failure, f.identityErrs = f.identityErrs[0], f.identityErrs[1:]
	}
	block, ignoreCtx, out := f.block, f.ignoreCtx, f.identity
	f.mu.Unlock()

	if block != nil {
		if ignoreCtx {
			<-block
		} else {
			select {
			case <-block:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	if failure != nil {
		return nil, failure
	}
	return &out, nil
}

// count is how many times the operation was called.
func (f *fakeServices) count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[op]
}

// waitForContext blocks until ctx ends and returns its error, as a call that
// never answers does. After two seconds it gives up with another error, so a
// call that nothing bounded fails its test instead of hanging it.
func waitForContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("the call was not bounded by a timeout")
	}
}

// blockingProvider is a credentials provider that never answers by itself and
// reports the credential sources it was given.
type blockingProvider struct{ sources []awssdk.CredentialSource }

// Retrieve implements awssdk.CredentialsProvider.
func (blockingProvider) Retrieve(ctx context.Context) (awssdk.Credentials, error) {
	return awssdk.Credentials{}, waitForContext(ctx)
}

// ProviderSources implements awssdk.CredentialProviderSource.
func (p blockingProvider) ProviderSources() []awssdk.CredentialSource { return p.sources }

// newTestManager isolates the environment, writes <home>/.aws/config and
// <home>/.aws/credentials (an empty content skips the file) and returns a
// manager whose services are a new fakeServices and whose call timeout is
// 500ms. The manager is stopped when the test ends.
func newTestManager(t *testing.T, o Options, configContent, credentialsContent string) (*AWSClientManager, *fakeServices) {
	t.Helper()
	home := isolateAWSEnv(t)
	if configContent != "" {
		writeTestFile(t, filepath.Join(home, ".aws", "config"), configContent)
	}
	if credentialsContent != "" {
		writeTestFile(t, filepath.Join(home, ".aws", "credentials"), credentialsContent)
	}

	m := NewAWSClientManagerWithOptions(o)
	f := newFakeServices()
	m.svc = f
	m.callTimeout = 500 * time.Millisecond
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = m.Stop(ctx)
	})
	return m, f
}

// waitFor polls cond every 2ms and fails the test when it is still false after
// the given time.
func waitFor(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", within)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
