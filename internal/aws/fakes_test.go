package aws

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Fixtures. The account ID and the keys are obviously fake; no real credential,
// token or account ever appears in a test.
const (
	testAccount     = "123456789012"
	testARN         = "arn:aws:sts::123456789012:assumed-role/ReadOnly/dev"
	ownerStartURL   = "https://example.awsapps.com/start"
	sessionStartURL = "https://corp.awsapps.com/start"

	// ownerConfig is a legacy SSO profile with only its region set in the
	// profile, the layout of the machine this integration was built for.
	ownerConfig = "[default]\nsso_start_url = " + ownerStartURL + "\nsso_region = us-east-1\nsso_account_id = 123456789012\nsso_role_name = ReadOnly\nsso_registration_scopes = sso:account:access\nregion = sa-east-1\n"

	// staticCredentials are temporary keys exported to the credentials file. The
	// SDK resolves them before the SSO settings of the same profile.
	staticCredentials = "[default]\naws_access_key_id = AKIDFAKEEXAMPLE\naws_secret_access_key = fake-secret-for-tests\naws_session_token = fake-session-for-tests\n"

	// sessionConfig is an sso_session profile: the start URL and the SSO region
	// live in the [sso-session] section, not in the profile.
	sessionConfig = "[profile dev]\nsso_session = corp\nsso_account_id = 123456789012\nsso_role_name = ReadOnly\nregion = eu-west-1\n\n[sso-session corp]\nsso_start_url = " + sessionStartURL + "\nsso_region = eu-central-1\nsso_registration_scopes = sso:account:access\n"

	// sessionCredentials are static keys for the dev profile, so the SDK never
	// builds its own SSO provider for it.
	sessionCredentials = "[dev]\naws_access_key_id = AKIDFAKEEXAMPLE\naws_secret_access_key = fake-secret-for-tests\n"
)

// fakeServices stands in for the SDK service clients: no test builds a real
// one, so no test can reach the network.
type fakeServices struct {
	gotMax       map[string][]int32       // MaxResults of every call, by operation
	gotRegions   map[string]string        // region of the config the last client was built from, by "SSO" or "OIDC"
	ssoRoles     map[string][]string      // account ID -> role names
	ssoErrs      map[string]error         // by operation: "ListAccounts", "ListAccountRoles"
	block        chan struct{}            // not nil: GetCallerIdentity counts the call, then waits for close (or ctx.Done unless ignoreCtx)
	ssoBlock     map[string]chan struct{} // by operation: that call counts, then waits for close or ctx.Done
	roleCredsErr map[string]error         // by "<account>/<role>"
	calls        map[string]int           // by operation name: "GetCallerIdentity", "ListAccounts", "CreateToken", ...
	identity     sts.GetCallerIdentityOutput
	oidcToken    string   // access token CreateToken returns (ExpiresIn 3600, RefreshToken "fake-refresh-2")
	gotTokens    []string // AccessToken of every SSO call
	identityErrs []error  // one per call, consumed in order; when empty the call succeeds
	ssoAccounts  []ssotypes.AccountInfo
	gotRoleCreds []string // "<account>/<role>" of every GetRoleCredentials call
	oidcCalls    int
	mu           sync.Mutex
	ssoPageSize  int32 // ListAccounts page size; 0 means 1000
	ignoreCtx    bool
}

// newFakeServices answers GetCallerIdentity with testAccount and testARN and
// knows no SSO account.
func newFakeServices() *fakeServices {
	return &fakeServices{
		calls:        make(map[string]int),
		ssoRoles:     make(map[string][]string),
		ssoErrs:      make(map[string]error),
		roleCredsErr: make(map[string]error),
		ssoBlock:     make(map[string]chan struct{}),
		gotMax:       make(map[string][]int32),
		gotRegions:   make(map[string]string),
		oidcToken:    "fake-oidc-access-token",
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

// SSO implements services. It remembers the region of cfg: the SSO portal lives
// in the sso_region of the profile, not in the region of its other services.
func (f *fakeServices) SSO(cfg awssdk.Config) ssoAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotRegions["SSO"] = cfg.Region
	return f
}

// OIDC implements services. It remembers the region of cfg like SSO does.
func (f *fakeServices) OIDC(cfg awssdk.Config) ssocreds.CreateTokenAPIClient {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotRegions["OIDC"] = cfg.Region
	return f
}

// waitBlocked is how a fake call behaves when the test told it to hang: it waits
// for close(block) or for the end of ctx. A nil block does not wait. After two
// seconds it gives up with another error, so a call that nothing bounded fails
// its test instead of hanging it.
func waitBlocked(ctx context.Context, block chan struct{}) error {
	if block == nil {
		return nil
	}
	select {
	case <-block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("the call was not bounded by a timeout")
	}
}

// record counts one call of op under the lock. A non-nil maxResults is the
// MaxResults of the request.
func (f *fakeServices) record(op, accessToken string, maxResults *int32) {
	f.calls[op]++
	f.gotTokens = append(f.gotTokens, accessToken)
	if maxResults != nil {
		f.gotMax[op] = append(f.gotMax[op], *maxResults)
	}
}

// ListAccounts implements ssoAPI. It serves ssoAccounts in pages of ssoPageSize
// (1000 when unset); the next token is the offset of the next page.
func (f *fakeServices) ListAccounts(ctx context.Context, in *sso.ListAccountsInput, _ ...func(*sso.Options)) (*sso.ListAccountsOutput, error) {
	f.mu.Lock()
	f.record("ListAccounts", awssdk.ToString(in.AccessToken), in.MaxResults)
	failure, block := f.ssoErrs["ListAccounts"], f.ssoBlock["ListAccounts"]
	accounts, pageSize := slices.Clone(f.ssoAccounts), int(f.ssoPageSize)
	f.mu.Unlock()

	if err := waitBlocked(ctx, block); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	if pageSize <= 0 {
		pageSize = 1000
	}
	start := 0
	if token := awssdk.ToString(in.NextToken); token != "" {
		offset, err := strconv.Atoi(token)
		if err != nil || offset < 0 || offset > len(accounts) {
			return nil, errors.New("fake ListAccounts: bad next token " + token)
		}
		start = offset
	}
	end := min(start+pageSize, len(accounts))
	out := &sso.ListAccountsOutput{AccountList: accounts[start:end]}
	if end < len(accounts) {
		out.NextToken = awssdk.String(strconv.Itoa(end))
	}
	return out, nil
}

// ListAccountRoles implements ssoAPI. It answers with ssoRoles[account] in one
// page.
func (f *fakeServices) ListAccountRoles(ctx context.Context, in *sso.ListAccountRolesInput, _ ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error) {
	account := awssdk.ToString(in.AccountId)
	f.mu.Lock()
	f.record("ListAccountRoles", awssdk.ToString(in.AccessToken), in.MaxResults)
	failure, block := f.ssoErrs["ListAccountRoles"], f.ssoBlock["ListAccountRoles"]
	roles := slices.Clone(f.ssoRoles[account])
	f.mu.Unlock()

	if err := waitBlocked(ctx, block); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	out := &sso.ListAccountRolesOutput{}
	for _, role := range roles {
		out.RoleList = append(out.RoleList, ssotypes.RoleInfo{AccountId: awssdk.String(account), RoleName: awssdk.String(role)})
	}
	return out, nil
}

// GetRoleCredentials implements ssoAPI. It hands out the same fake role keys,
// valid for one hour, unless roleCredsErr names the account and role.
func (f *fakeServices) GetRoleCredentials(ctx context.Context, in *sso.GetRoleCredentialsInput, _ ...func(*sso.Options)) (*sso.GetRoleCredentialsOutput, error) {
	key := awssdk.ToString(in.AccountId) + "/" + awssdk.ToString(in.RoleName)
	f.mu.Lock()
	f.record("GetRoleCredentials", awssdk.ToString(in.AccessToken), nil)
	f.gotRoleCreds = append(f.gotRoleCreds, key)
	failure, block := f.roleCredsErr[key], f.ssoBlock["GetRoleCredentials"]
	f.mu.Unlock()

	if err := waitBlocked(ctx, block); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return &sso.GetRoleCredentialsOutput{RoleCredentials: &ssotypes.RoleCredentials{
		AccessKeyId:     awssdk.String("AKIDFAKEROLE"),
		SecretAccessKey: awssdk.String("fake-role-secret"),
		SessionToken:    awssdk.String("fake-role-session"),
		Expiration:      time.Now().Add(time.Hour).UnixMilli(),
	}}, nil
}

// CreateToken implements ssocreds.CreateTokenAPIClient: the refresh of an
// sso_session token.
func (f *fakeServices) CreateToken(ctx context.Context, _ *ssooidc.CreateTokenInput, _ ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	f.mu.Lock()
	f.calls["CreateToken"]++
	f.oidcCalls++
	token, block := f.oidcToken, f.ssoBlock["CreateToken"]
	f.mu.Unlock()

	if err := waitBlocked(ctx, block); err != nil {
		return nil, err
	}
	return &ssooidc.CreateTokenOutput{
		AccessToken:  awssdk.String(token),
		ExpiresIn:    3600,
		RefreshToken: awssdk.String("fake-refresh-2"),
	}, nil
}

// hang makes every call of op wait until the test ends or the context of the
// call does. The channel is closed when the test is over, so a call that outlives
// its context (the SDK's credentials cache runs its provider under a context
// without deadline) does not stay parked in waitBlocked.
func (f *fakeServices) hang(t *testing.T, op string) {
	t.Helper()
	block := make(chan struct{})
	f.mu.Lock()
	f.ssoBlock[op] = block
	f.mu.Unlock()
	t.Cleanup(func() { close(block) })
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

// writeSSOToken writes the SSO token cache file the SDK names for key (the
// sso_session name or the start URL of a legacy profile), under the isolated
// home, as JSON with the access token and its expiry. With refresh it also holds
// the fields the SDK needs to refresh the token. It refuses to run when HOME is
// not the home isolateAWSEnv made, so a test can never touch the real cache.
func writeSSOToken(t *testing.T, key, accessToken string, expiresAt time.Time, refresh bool) {
	t.Helper()
	if problem := isolationProblem(); problem != "" {
		t.Fatal("writeSSOToken: " + problem)
	}
	path, err := ssocreds.StandardCachedTokenFilepath(key)
	if err != nil {
		t.Fatalf("token cache path for %q: %v", key, err)
	}
	fields := map[string]string{
		"accessToken": accessToken,
		"expiresAt":   expiresAt.UTC().Format(time.RFC3339),
	}
	if refresh {
		fields["refreshToken"] = "fake-refresh"
		fields["clientId"] = "fake-client"
		fields["clientSecret"] = "fake-client-secret"
	}
	content, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode the token file: %v", err)
	}
	writeTestFile(t, path, string(content))
}
