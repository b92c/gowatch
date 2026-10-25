package aws

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/smithy-go"
)

// ssoInfo is what the SSO portal answers for one account. Every account carries
// an e-mail address on purpose: nothing in the state may ever show it.
func ssoInfo(id, name string) ssotypes.AccountInfo {
	return ssotypes.AccountInfo{
		AccountId:    awssdk.String(id),
		AccountName:  awssdk.String(name),
		EmailAddress: awssdk.String("root@example.com"),
	}
}

// ownerManager is the layout of the owner's machine (a legacy SSO profile, keys
// in the credentials file) with a fresh SSO login that reaches three accounts:
// testAccount "dev" with two roles, "222222222222" "beta" and "111111111111"
// "Alpha".
func ownerManager(t *testing.T, o Options) (*AWSClientManager, *fakeServices) {
	t.Helper()
	m, f := newTestManager(t, o, ownerConfig, staticCredentials)
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(time.Hour), false)
	f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo(testAccount, "dev"), ssoInfo("222222222222", "beta"), ssoInfo("111111111111", "Alpha")}
	f.ssoRoles[testAccount] = []string{"ReadOnly", "Admin"}
	f.ssoRoles["222222222222"] = []string{"ReadOnly"}
	f.ssoRoles["111111111111"] = []string{"ReadOnly"}
	return m, f
}

// rowView is what the tests compare of an account row.
type rowView struct {
	Name     string
	Status   string
	Roles    []string
	Identity bool
}

// statusRow is the part of an account row that the tests of statuses compare.
type statusRow struct{ Name, Status, Role string }

// srow builds one, so that a table of many reads as a table.
func srow(name, status, role string) statusRow {
	return statusRow{Name: name, Status: status, Role: role}
}

// statusRowsOf lists the name, status and role of each row.
func statusRowsOf(rows []Account) []statusRow {
	got := make([]statusRow, 0, len(rows))
	for _, a := range rows {
		got = append(got, srow(a.Name, a.Status, a.Role))
	}
	return got
}

func viewOf(rows []Account) []rowView {
	views := make([]rowView, 0, len(rows))
	for _, a := range rows {
		views = append(views, rowView{Name: a.Name, Status: a.Status, Roles: a.Roles, Identity: a.Identity})
	}
	return views
}

func TestListSSOAccountsPaginatesAndCaps(t *testing.T) {
	f := newFakeServices()
	f.ssoPageSize = 40
	for i := range 120 {
		id := fmt.Sprintf("%d", 100000000000+int64(i))
		f.ssoAccounts = append(f.ssoAccounts, ssoInfo(id, fmt.Sprintf("Acct %03d", i)))
		f.ssoRoles[id] = []string{"ReadOnly"}
	}
	var manyRoles []string
	for i := range 25 {
		manyRoles = append(manyRoles, fmt.Sprintf("Role%02d", i))
	}
	f.ssoRoles["100000000003"] = manyRoles

	accounts, failedOp, err := listSSOAccounts(context.Background(), f, "fake-access-token", time.Second)

	if err != nil || failedOp != "" {
		t.Fatalf("listSSOAccounts() error = %v (failed call %q), want none", err, failedOp)
	}
	if len(accounts) != 100 {
		t.Fatalf("listed %d accounts, want the cap of 100", len(accounts))
	}
	for i, a := range accounts {
		if want := fmt.Sprintf("%d", 100000000000+int64(i)); a.id != want || a.name != fmt.Sprintf("Acct %03d", i) {
			t.Fatalf("account %d = %+v, want %s %q: the API order must be kept", i, a, want, fmt.Sprintf("Acct %03d", i))
		}
	}
	if got := f.count("ListAccounts"); got != 3 {
		t.Errorf("ListAccounts was called %d times, want 3 (40, 40 and the 20 that fit under the cap)", got)
	}
	if got := f.count("ListAccountRoles"); got != 100 {
		t.Errorf("ListAccountRoles was called %d times, want once per kept account (100)", got)
	}
	if want := manyRoles[:20]; !reflect.DeepEqual(accounts[3].roles, want) {
		t.Errorf("roles of the account with 25 = %v, want the first 20, %v", accounts[3].roles, want)
	}
	if want := []string{"ReadOnly"}; !reflect.DeepEqual(accounts[4].roles, want) {
		t.Errorf("roles of another account = %v, want %v", accounts[4].roles, want)
	}
	for op, want := range map[string]int32{"ListAccounts": 100, "ListAccountRoles": 20} {
		for _, got := range f.gotMax[op] {
			if got != want {
				t.Fatalf("a %s call asked for MaxResults %d, want %d", op, got, want)
			}
		}
	}
	if len(f.gotTokens) != 103 {
		t.Errorf("%d calls carried a token, want 103", len(f.gotTokens))
	}
	for _, got := range f.gotTokens {
		if got != "fake-access-token" {
			t.Fatalf("a call carried the token %q, want the one that was given", got)
		}
	}
}

// With more accounts than the cap, listing stops at the page that reaches it.
func TestListSSOAccountsDoesNotAskForPagesBeyondTheCap(t *testing.T) {
	f := newFakeServices()
	f.ssoPageSize = 50
	for i := range 230 {
		id := fmt.Sprintf("%d", 100000000000+int64(i))
		f.ssoAccounts = append(f.ssoAccounts, ssoInfo(id, "Acct"))
		f.ssoRoles[id] = []string{"ReadOnly"}
	}

	accounts, _, err := listSSOAccounts(context.Background(), f, "fake-access-token", time.Second)

	if err != nil || len(accounts) != 100 {
		t.Fatalf("listSSOAccounts() = %d accounts, error %v, want 100 and none", len(accounts), err)
	}
	if got := f.count("ListAccounts"); got != 2 {
		t.Errorf("ListAccounts was called %d times, want 2: the second page already reaches the cap", got)
	}
}

// endlessPages answers every ListAccounts with an empty page and a next token.
type endlessPages struct {
	*fakeServices
	pages int
}

func (e *endlessPages) ListAccounts(context.Context, *sso.ListAccountsInput, ...func(*sso.Options)) (*sso.ListAccountsOutput, error) {
	e.pages++
	return &sso.ListAccountsOutput{NextToken: awssdk.String("again")}, nil
}

// A service that never stops paging cannot make the poller call it forever.
func TestListSSOAccountsStopsAnEndlessListing(t *testing.T) {
	e := &endlessPages{fakeServices: newFakeServices()}

	accounts, _, err := listSSOAccounts(context.Background(), e, "fake-access-token", time.Second)

	if err != nil || len(accounts) != 0 {
		t.Errorf("listSSOAccounts() = (%v, %v), want no account and no error", accounts, err)
	}
	if e.pages > maxSSOAccounts {
		t.Errorf("ListAccounts was called %d times, want at most %d", e.pages, maxSSOAccounts)
	}
}

func TestListSSOAccountsNames(t *testing.T) {
	f := newFakeServices()
	f.ssoAccounts = []ssotypes.AccountInfo{
		ssoInfo("111111111111", "Team [prod]"),
		{AccountId: awssdk.String("222222222222")},
		{AccountId: awssdk.String("333333333333"), AccountName: awssdk.String("")},
		{AccountName: awssdk.String("no id")},
	}

	accounts, _, err := listSSOAccounts(context.Background(), f, "fake-access-token", time.Second)

	if err != nil {
		t.Fatalf("listSSOAccounts() error = %v", err)
	}
	var names []string
	for _, a := range accounts {
		names = append(names, a.id+"="+a.name)
	}
	want := []string{"111111111111=Team [prod]", "222222222222=222222222222", "333333333333=333333333333"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("accounts = %v, want %v: a missing name is the ID, an entry without an ID is dropped", names, want)
	}
}

func TestListSSOAccountsNamesTheFailedCall(t *testing.T) {
	tests := []struct {
		name       string
		op         string
		wantFailed string
	}{
		{name: "the list fails", op: "ListAccounts", wantFailed: opListAccounts},
		{name: "the roles fail", op: "ListAccountRoles", wantFailed: opListAccountRoles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServices()
			f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo("111111111111", "a")}
			boom := errors.New("boom")
			f.ssoErrs[tt.op] = boom

			accounts, failedOp, err := listSSOAccounts(context.Background(), f, "fake-access-token", time.Second)

			if !errors.Is(err, boom) || failedOp != tt.wantFailed || accounts != nil {
				t.Errorf("listSSOAccounts() = (%v, %q, %v), want (nil, %q, boom)", accounts, failedOp, err, tt.wantFailed)
			}
		})
	}
}

// Every call of the listing has its own timeout.
func TestListSSOAccountsCallsAreBounded(t *testing.T) {
	tests := []struct {
		name       string
		blocked    string
		wantFailed string
	}{
		{name: "ListAccounts", blocked: "ListAccounts", wantFailed: opListAccounts},
		{name: "ListAccountRoles", blocked: "ListAccountRoles", wantFailed: opListAccountRoles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServices()
			f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo("111111111111", "a")}
			f.hang(t, tt.blocked)

			_, failedOp, err := listSSOAccounts(context.Background(), f, "fake-access-token", 50*time.Millisecond)

			if !errors.Is(err, context.DeadlineExceeded) || failedOp != tt.wantFailed {
				t.Errorf("listSSOAccounts() = (%q, %v), want the call %q to hit its deadline", failedOp, err, tt.wantFailed)
			}
		})
	}
}

// Review Focus 1: the owner's layout. Exported keys give the identity; the
// accounts come from the token the AWS CLI cached for the start URL.
func TestOwnerLayoutListsSSOAccountsFromTheCachedToken(t *testing.T) {
	m, f := ownerManager(t, Options{})

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	want := []rowView{
		{Name: "dev", Status: StatusMonitored, Identity: true, Roles: []string{"ReadOnly", "Admin"}},
		{Name: "Alpha", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
		{Name: "beta", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
	}
	if got := viewOf(st.Accounts); !reflect.DeepEqual(got, want) {
		t.Errorf("accounts = %+v, want %+v", got, want)
	}
	if st.DiscoveryError != (Problem{}) {
		t.Errorf("DiscoveryError = %+v, want none", st.DiscoveryError)
	}
	if st.Conn != ConnConnected || st.LastError != (Problem{}) {
		t.Errorf("Conn = %q, LastError = %+v, want connected without an error", st.Conn, st.LastError)
	}
	for _, a := range st.Accounts {
		if a.Role != "" {
			t.Errorf("account %q uses the role %q, want none: no entry of aws.accounts asks for it", a.Name, a.Role)
		}
	}
	if got := f.count("GetRoleCredentials"); got != 0 {
		t.Errorf("GetRoleCredentials was called %d times, want none without aws.accounts", got)
	}
	if got := f.gotRegions["SSO"]; got != "us-east-1" {
		t.Errorf("the SSO client was built for the region %q, want the sso_region of the profile, us-east-1", got)
	}
}

// The identity account is not part of the SSO list (exported keys of another
// account, an assumed role): it stays first under its ID, and every listed
// account is available.
func TestIdentityAccountOutsideTheSSOList(t *testing.T) {
	m, f := ownerManager(t, Options{})
	f.identity.Account = awssdk.String("999999999999")
	f.identity.Arn = awssdk.String("arn:aws:sts::999999999999:assumed-role/Other/dev")

	m.pollOnce(context.Background(), true)

	want := []rowView{
		{Name: "999999999999", Status: StatusMonitored, Identity: true},
		{Name: "Alpha", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
		{Name: "beta", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
		{Name: "dev", Status: StatusAvailable, Roles: []string{"ReadOnly", "Admin"}},
	}
	if got := viewOf(m.GetState().Accounts); !reflect.DeepEqual(got, want) {
		t.Errorf("accounts = %+v, want %+v", got, want)
	}
}

func TestDiscoveryExpiredTokenShowsHintAndRecovers(t *testing.T) {
	m, f := ownerManager(t, Options{})
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(-time.Hour), false)
	ctx := context.Background()

	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if got := f.count("ListAccounts"); got != 0 {
		t.Errorf("ListAccounts was called %d times with an expired token, want none", got)
	}
	if want := []rowView{{Name: testAccount, Status: StatusMonitored, Identity: true}}; !reflect.DeepEqual(viewOf(st.Accounts), want) {
		t.Errorf("accounts = %+v, want only the identity row %+v", viewOf(st.Accounts), want)
	}
	if st.Conn != ConnConnected || st.LastError != (Problem{}) {
		t.Errorf("Conn = %q, LastError = %+v: an expired SSO login must not hide the connected identity", st.Conn, st.LastError)
	}

	// The user logs in again; the next discovery finds the new token.
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(time.Hour), false)
	m.pollOnce(ctx, true)
	st = m.GetState()

	if st.DiscoveryError != (Problem{}) {
		t.Errorf("DiscoveryError after the login = %+v, want none", st.DiscoveryError)
	}
	if len(st.Accounts) != 3 {
		t.Errorf("accounts after the login = %+v, want the 3 listed accounts", viewOf(st.Accounts))
	}
}

// A login that never happened has no cache file at all.
func TestDiscoveryWithoutATokenFileShowsTheHint(t *testing.T) {
	m, f := newTestManager(t, Options{}, ownerConfig, staticCredentials)

	m.pollOnce(context.Background(), true)

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); m.GetState().DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", m.GetState().DiscoveryError, want)
	}
	if got := f.count("ListAccounts"); got != 0 {
		t.Errorf("ListAccounts was called %d times without a token, want none", got)
	}
}

// What was listed stays on screen while the login is expired: the title says
// why it is stale.
func TestDiscoveryKeepsTheLastListWhenTheLoginExpires(t *testing.T) {
	m, _ := ownerManager(t, Options{})
	ctx := context.Background()
	m.pollOnce(ctx, true)

	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(-time.Hour), false)
	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if len(st.Accounts) != 3 || st.Accounts[0].Name != "dev" || len(st.Accounts[0].Roles) != 2 {
		t.Errorf("accounts = %+v, want the list of the first pass", viewOf(st.Accounts))
	}
}

func TestDiscoveryFailureOfTheListIsClassifiedAndKeepsTheLastList(t *testing.T) {
	m, f := ownerManager(t, Options{})
	ctx := context.Background()
	m.pollOnce(ctx, true)

	f.ssoErrs["ListAccounts"] = &smithy.GenericAPIError{Code: "AccessDeniedException"}
	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := (Problem{Text: "access denied: ListAccounts"}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if len(st.Accounts) != 3 {
		t.Errorf("accounts = %+v, want the list of the first pass", viewOf(st.Accounts))
	}

	delete(f.ssoErrs, "ListAccounts")
	f.ssoErrs["ListAccountRoles"] = &smithy.GenericAPIError{Code: "UnauthorizedException"}
	m.pollOnce(ctx, true)

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); m.GetState().DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want the portal rejection of the roles call to read as an expired login, %+v", m.GetState().DiscoveryError, want)
	}
}

func TestDiscoveryThrottledSkipsTheRest(t *testing.T) {
	m, f := ownerManager(t, Options{})
	f.ssoErrs["ListAccounts"] = &smithy.GenericAPIError{Code: "TooManyRequestsException"}

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if want := (Problem{Text: textThrottled}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if got := f.count("ListAccountRoles"); got != 0 {
		t.Errorf("ListAccountRoles was called %d times after the list was throttled, want none", got)
	}
	if want := []rowView{{Name: testAccount, Status: StatusMonitored, Identity: true}}; !reflect.DeepEqual(viewOf(st.Accounts), want) {
		t.Errorf("accounts = %+v, want only the identity row %+v", viewOf(st.Accounts), want)
	}
	if m.discoverAccounts(context.Background()) {
		t.Error("discoverAccounts() = true after a throttled call, want false so the rest of the pass is skipped")
	}
}

// A call that never answers is cut by the call timeout and reported as such,
// not as an expired login.
func TestDiscoveryCallsAreBoundedByTheCallTimeout(t *testing.T) {
	m, f := ownerManager(t, Options{})
	f.hang(t, "ListAccounts")
	m.callTimeout = 100 * time.Millisecond

	m.pollOnce(context.Background(), true)

	if want := (Problem{Text: prefixUnreachable + "timeout"}); m.GetState().DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", m.GetState().DiscoveryError, want)
	}
	if got := f.count("ListAccountRoles"); got != 0 {
		t.Errorf("ListAccountRoles was called %d times after the list timed out, want none", got)
	}
}

// The portal answers with an e-mail address for every account, and the token is
// the key to the portal: neither may be anywhere in the state.
func TestDiscoveryNeverKeepsEmailsOrTokens(t *testing.T) {
	m, f := ownerManager(t, Options{})
	writeSSOToken(t, ownerStartURL, "fake-access-token-value", time.Now().Add(time.Hour), false)

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if len(st.Accounts) != 3 || f.count("ListAccounts") != 1 {
		t.Fatalf("the pass listed %d accounts in %d calls, want 3 in 1: the test would prove nothing", len(st.Accounts), f.count("ListAccounts"))
	}
	for _, got := range f.gotTokens {
		if got != "fake-access-token-value" {
			t.Fatalf("a call carried the token %q, want the one of the cache file", got)
		}
	}
	text := fmt.Sprintf("%+v", st)
	for _, secret := range []string{"root@example.com", "example.com", "fake-access-token-value"} {
		if strings.Contains(text, secret) {
			t.Errorf("the state shows %q: %s", secret, text)
		}
	}
}

// GoWatch's own code reads the token file of a legacy profile and nothing else:
// it never writes under ~/.aws, neither the token nor anything next to it. That
// holds for the credentials of a configured account too, which the SDK's
// provider mints from the same file.
func TestLegacyDiscoveryWritesNothingUnderAWSDir(t *testing.T) {
	m, f := ownerManager(t, Options{Accounts: []string{"222222222222"}})
	dir := filepath.Join(os.Getenv("HOME"), ".aws")
	longAgo := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, longAgo, longAgo)
	}); err != nil {
		t.Fatalf("age the files of %s: %v", dir, err)
	}
	before := snapshotDir(t, dir)
	if len(before) != 3 {
		t.Fatalf("files under %s = %v, want the config, the credentials and the token cache", dir, before)
	}

	m.pollOnce(context.Background(), true)
	m.pollOnce(context.Background(), true)

	if got := f.count("ListAccounts"); got != 2 {
		t.Fatalf("ListAccounts was called %d times, want 2: the test would prove nothing", got)
	}
	if got := f.count("GetRoleCredentials"); got != 1 {
		t.Fatalf("GetRoleCredentials was called %d times, want 1: the test would prove nothing", got)
	}
	if after := snapshotDir(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("the files under %s changed:\nbefore %v\nafter  %v", dir, before, after)
	}
}

// snapshotDir lists every file under dir with its size and modification time.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[path] = fmt.Sprintf("%d bytes, %s", info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func TestNonSSOProfileHasOnlyTheIdentityRow(t *testing.T) {
	m, f := newTestManager(t, Options{}, "[default]\nregion = eu-west-1\n", staticCredentials)
	now := time.Date(2026, 10, 25, 9, 8, 7, 0, time.UTC)
	m.now = func() time.Time { return now }

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	wantRows := []Account{{UpdatedAt: now, ID: testAccount, Name: testAccount, Status: StatusMonitored, Identity: true}}
	if !reflect.DeepEqual(st.Accounts, wantRows) {
		t.Errorf("Accounts = %+v, want %+v", st.Accounts, wantRows)
	}
	for _, op := range []string{"ListAccounts", "ListAccountRoles", "GetRoleCredentials", "CreateToken"} {
		if got := f.count(op); got != 0 {
			t.Errorf("%s was called %d times for a profile without SSO, want none", op, got)
		}
	}
	if st.DiscoveryError != (Problem{}) {
		t.Errorf("DiscoveryError = %+v, want none", st.DiscoveryError)
	}
}

// The next discovery reads the shared files again: a profile that stops being
// SSO-based drops the list and its problem.
func TestProfileThatStopsBeingSSOClearsTheList(t *testing.T) {
	m, _ := ownerManager(t, Options{})
	ctx := context.Background()
	m.pollOnce(ctx, true)
	if len(m.GetState().Accounts) != 3 {
		t.Fatalf("accounts = %+v, want the 3 listed accounts", viewOf(m.GetState().Accounts))
	}

	writeTestFile(t, os.Getenv("AWS_CONFIG_FILE"), "[default]\nregion = eu-west-1\n")
	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := []rowView{{Name: testAccount, Status: StatusMonitored, Identity: true}}; !reflect.DeepEqual(viewOf(st.Accounts), want) {
		t.Errorf("accounts = %+v, want only the identity row %+v", viewOf(st.Accounts), want)
	}
	if st.DiscoveryError != (Problem{}) {
		t.Errorf("DiscoveryError = %+v, want none", st.DiscoveryError)
	}
}

// The identity failing stops the pass there: nothing SSO is asked, and the rows
// of the last good pass stay, with the identity row carrying the problem.
func TestIdentityFailureStopsTheDiscoveryAndKeepsTheRows(t *testing.T) {
	m, f := ownerManager(t, Options{})
	ctx := context.Background()
	m.pollOnce(ctx, true)
	listed := f.count("ListAccounts")
	f.identityErrs = []error{&smithy.GenericAPIError{Code: "ExpiredToken"}}

	m.pollOnce(ctx, true)
	st := m.GetState()

	if got := f.count("ListAccounts"); got != listed {
		t.Errorf("ListAccounts was called %d times after the identity failed, want %d", got, listed)
	}
	if len(st.Accounts) != 3 {
		t.Fatalf("accounts = %+v, want the 3 rows of the last good pass", viewOf(st.Accounts))
	}
	if want := (Problem{Text: textBadKeys, Hint: hintBadKeys}).StatusText(); st.Accounts[0].Status != want {
		t.Errorf("identity row status = %q, want %q", st.Accounts[0].Status, want)
	}
	if st.Accounts[1].Status != StatusAvailable {
		t.Errorf("another row's status = %q, want it unchanged", st.Accounts[1].Status)
	}
}

func TestSSOClientsAreBuiltForTheSSORegion(t *testing.T) {
	m, f := newTestManager(t, Options{Profile: "dev"}, sessionConfig, sessionCredentials)
	writeSSOToken(t, "corp", "fake-access-token", time.Now().Add(time.Hour), true)
	f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo(testAccount, "dev")}
	f.ssoRoles[testAccount] = []string{"ReadOnly"}

	m.pollOnce(context.Background(), true)

	if got := m.GetState().Region; got != "eu-west-1" {
		t.Errorf("Region = %q, want the region of the profile, eu-west-1", got)
	}
	for _, client := range []string{"SSO", "OIDC"} {
		if got := f.gotRegions[client]; got != "eu-central-1" {
			t.Errorf("the %s client was built for the region %q, want the sso_region of the session, eu-central-1", client, got)
		}
	}
	if st := m.GetState(); len(st.Accounts) != 1 || st.Accounts[0].Name != "dev" || st.DiscoveryError != (Problem{}) {
		t.Errorf("accounts = %+v, DiscoveryError = %+v, want the one listed account", viewOf(st.Accounts), st.DiscoveryError)
	}
}

func TestAccountRows(t *testing.T) {
	now := time.Date(2026, 10, 25, 9, 8, 7, 0, time.UTC)
	list := []ssoAccount{
		{id: "333333333333", name: "beta", roles: []string{"ReadOnly"}},
		{id: "222222222222", name: "Alpha", roles: []string{"Admin", "ReadOnly"}},
		{id: testAccount, name: "dev", roles: []string{"ReadOnly"}},
		{id: "111111111111", name: "alpha", roles: nil},
	}
	tests := []struct {
		name string
		want []rowView
		in   accountsInput
	}{
		{
			name: "no SSO", in: accountsInput{now: now, identityID: testAccount},
			want: []rowView{{Name: testAccount, Status: StatusMonitored, Identity: true}},
		},
		{
			name: "SSO that was never listed", in: accountsInput{now: now, identityID: testAccount, isSSO: true},
			want: []rowView{{Name: testAccount, Status: StatusMonitored, Identity: true}},
		},
		{
			name: "the identity account is in the list: the SSO name and the roles",
			in:   accountsInput{now: now, identityID: testAccount, isSSO: true, sso: list},
			want: []rowView{
				{Name: "dev", Status: StatusMonitored, Identity: true, Roles: []string{"ReadOnly"}},
				{Name: "alpha", Status: StatusAvailable},
				{Name: "Alpha", Status: StatusAvailable, Roles: []string{"Admin", "ReadOnly"}},
				{Name: "beta", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
			},
		},
		{
			name: "the identity account is not in the list",
			in:   accountsInput{now: now, identityID: "999999999999", isSSO: true, sso: list[:2]},
			want: []rowView{
				{Name: "999999999999", Status: StatusMonitored, Identity: true},
				{Name: "Alpha", Status: StatusAvailable, Roles: []string{"Admin", "ReadOnly"}},
				{Name: "beta", Status: StatusAvailable, Roles: []string{"ReadOnly"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := accountRows(tt.in)

			if got := viewOf(rows); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("accountRows() = %+v, want %+v", got, tt.want)
			}
			for _, row := range rows {
				if !row.UpdatedAt.Equal(now) {
					t.Errorf("row %q UpdatedAt = %s, want %s", row.Name, row.UpdatedAt, now)
				}
			}
		})
	}

	// Two accounts with the same name come out by ID.
	rows := accountRows(accountsInput{now: now, identityID: testAccount, isSSO: true, sso: []ssoAccount{
		{id: "222222222222", name: "twin"}, {id: "111111111111", name: "twin"},
	}})
	if len(rows) != 3 || rows[1].ID != "111111111111" || rows[2].ID != "222222222222" {
		t.Errorf("rows of two accounts with the same name = %+v, want them ordered by ID", rows)
	}

	// The rows own their roles: changing them never changes the list they came from.
	rows = accountRows(accountsInput{now: now, identityID: testAccount, isSSO: true, sso: list})
	rows[0].Roles[0] = "changed"
	if list[2].roles[0] != "ReadOnly" {
		t.Error("a row shares its roles with the SSO list it was built from")
	}
}

func TestParseAccountEntry(t *testing.T) {
	tests := []struct {
		in   string
		want accountEntry
	}{
		{in: "123456789012", want: accountEntry{id: "123456789012"}},
		{in: "123456789012/ReadOnly", want: accountEntry{id: "123456789012", role: "ReadOnly"}},
		{in: "210987654321/Role+=,.@-_x", want: accountEntry{id: "210987654321", role: "Role+=,.@-_x"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseAccountEntry(tt.in); got != tt.want {
				t.Errorf("parseAccountEntry(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestConfiguredEntries(t *testing.T) {
	got := configuredEntries([]string{
		"210987654321/Admin", testAccount, "111111111111", "210987654321/ReadOnly", "111111111111", testAccount + "/Admin", "222222222222",
	}, testAccount)

	want := []accountEntry{{id: "210987654321", role: "Admin"}, {id: "111111111111"}, {id: "222222222222"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("configuredEntries() = %+v, want %+v: the identity account and the repeats are dropped, the first entry of an account wins", got, want)
	}
	if got := configuredEntries(nil, testAccount); len(got) != 0 {
		t.Errorf("configuredEntries(nil) = %+v, want none", got)
	}
}

func TestChooseRole(t *testing.T) {
	tests := []struct {
		entry       accountEntry
		wantProblem Problem
		name        string
		profileRole string
		wantRole    string
		roles       []string
		wantFirst   bool
	}{
		{name: "the role of the entry, when listed", entry: accountEntry{role: "Admin"}, roles: []string{"ReadOnly", "Admin"}, profileRole: "ReadOnly", wantRole: "Admin"},
		{
			name: "the role of the entry, when not listed", entry: accountEntry{role: "Admin"}, roles: []string{"ReadOnly"}, profileRole: "ReadOnly",
			wantProblem: Problem{Text: "role Admin not listed for this account"},
		},
		{
			name: "the role of the entry, with no role listed", entry: accountEntry{role: "Admin"}, profileRole: "ReadOnly",
			wantProblem: Problem{Text: "role Admin not listed for this account"},
		},
		{name: "the role of the profile, when listed", roles: []string{"Admin", "ReadOnly"}, profileRole: "ReadOnly", wantRole: "ReadOnly"},
		{name: "the first role, when the profile role is not listed", roles: []string{"Billing"}, profileRole: "ReadOnly", wantRole: "Billing", wantFirst: true},
		{name: "the first role, when the profile has none", roles: []string{"Billing", "Audit"}, wantRole: "Billing", wantFirst: true},
		{name: "no role at all", profileRole: "ReadOnly", wantProblem: Problem{Text: "no roles available"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, first, problem := chooseRole(tt.entry, tt.roles, tt.profileRole)

			if role != tt.wantRole || first != tt.wantFirst || problem != tt.wantProblem {
				t.Errorf("chooseRole(%+v, %v, %q) = (%q, %t, %+v), want (%q, %t, %+v)",
					tt.entry, tt.roles, tt.profileRole, role, first, problem, tt.wantRole, tt.wantFirst, tt.wantProblem)
			}
		})
	}
}

// accountsManager is the owner's layout with a fresh SSO login that reaches four
// accounts, and the given aws.accounts: testAccount "dev" (ReadOnly),
// "210987654321" "prod" (ReadOnly, Admin), "111111111111" "audit" (Billing) and
// "222222222222" "beta" (ReadOnly).
func accountsManager(t *testing.T, accounts ...string) (*AWSClientManager, *fakeServices) {
	t.Helper()
	m, f := newTestManager(t, Options{Accounts: accounts}, ownerConfig, staticCredentials)
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(time.Hour), false)
	f.ssoAccounts = []ssotypes.AccountInfo{
		ssoInfo(testAccount, "dev"), ssoInfo("210987654321", "prod"), ssoInfo("111111111111", "audit"), ssoInfo("222222222222", "beta"),
	}
	f.ssoRoles[testAccount] = []string{"ReadOnly"}
	f.ssoRoles["210987654321"] = []string{"ReadOnly", "Admin"}
	f.ssoRoles["111111111111"] = []string{"Billing"}
	f.ssoRoles["222222222222"] = []string{"ReadOnly"}
	return m, f
}

// statusOf finds the row of an account by name.
func statusOf(t *testing.T, st AWSState, name string) Account {
	t.Helper()
	for _, a := range st.Accounts {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("no row named %q in %+v", name, viewOf(st.Accounts))
	return Account{}
}

func TestConfiguredAccountsResolution(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin", "111111111111", "222222222222/Admin", "333333333333", testAccount)

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	want := []statusRow{
		srow("dev", "monitored", ""),
		srow("audit", "monitored (first role Billing)", "Billing"),
		srow("beta", "error: role Admin not listed for this account", ""),
		srow("prod", "monitored", "Admin"),
		srow("333333333333", "error: not accessible with this login", ""),
	}
	if got := statusRowsOf(st.Accounts); !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %+v, want %+v", got, want)
	}
	slices.Sort(f.gotRoleCreds)
	if wantCreds := []string{"111111111111/Billing", "210987654321/Admin"}; !reflect.DeepEqual(f.gotRoleCreds, wantCreds) {
		t.Errorf("GetRoleCredentials was called for %v, want exactly %v", f.gotRoleCreds, wantCreds)
	}
	if prod := statusOf(t, st, "prod"); !reflect.DeepEqual(prod.Roles, []string{"ReadOnly", "Admin"}) || prod.ID != "210987654321" {
		t.Errorf("the row of prod = %+v, want its ID and the roles of the list", prod)
	}
	if missing := statusOf(t, st, "333333333333"); missing.ID != "333333333333" || len(missing.Roles) != 0 {
		t.Errorf("the row of the missing account = %+v, want its ID as the name and no roles", missing)
	}
	for _, token := range f.gotTokens {
		if token != "fake-access-token" {
			t.Fatalf("a call carried the token %q, want the one of the cache file", token)
		}
	}
	if st.DiscoveryError != (Problem{}) {
		t.Errorf("DiscoveryError = %+v, want none: a problem on one account is not a discovery problem", st.DiscoveryError)
	}
	if len(m.work.minted) != 2 || m.work.minted["210987654321"].role != "Admin" || m.work.minted["111111111111"].role != "Billing" {
		t.Errorf("minted = %+v, want the credentials of prod as Admin and audit as Billing", m.work.minted)
	}
	if got := f.gotRegions["SSO"]; got != "us-east-1" {
		t.Errorf("the SSO client was built for the region %q, want us-east-1", got)
	}
}

// The minted accounts carry the configuration of the identity (region, retries)
// and their own credentials, so a CloudWatch Logs client can be built from them.
func TestMintedAccountsCarryTheirOwnCredentials(t *testing.T) {
	m, _ := accountsManager(t, "210987654321/Admin")

	m.pollOnce(context.Background(), true)
	conn, ok := m.work.minted["210987654321"]

	if !ok {
		t.Fatalf("minted = %+v, want the credentials of prod", m.work.minted)
	}
	if conn.cfg.Region != "sa-east-1" {
		t.Errorf("Region of the minted config = %q, want the resolved region, sa-east-1", conn.cfg.Region)
	}
	creds, err := conn.cfg.Credentials.Retrieve(context.Background())
	if err != nil || creds.AccessKeyID != "AKIDFAKEROLE" {
		t.Errorf("Credentials.Retrieve() = (%q, %v), want the keys of the role", creds.AccessKeyID, err)
	}
	if m.work.cfg.Credentials == conn.cfg.Credentials {
		t.Error("the minted config shares the credentials of the identity")
	}
}

func TestProfileRoleIsPreferredWhenListed(t *testing.T) {
	m, f := accountsManager(t, "210987654321")
	f.ssoRoles["210987654321"] = []string{"Admin", "ReadOnly"}

	m.pollOnce(context.Background(), true)

	prod := statusOf(t, m.GetState(), "prod")
	if prod.Status != StatusMonitored || prod.Role != "ReadOnly" {
		t.Errorf("row of prod = status %q, role %q, want monitored with the role of the profile, ReadOnly", prod.Status, prod.Role)
	}
	if want := []string{"210987654321/ReadOnly"}; !reflect.DeepEqual(f.gotRoleCreds, want) {
		t.Errorf("GetRoleCredentials was called for %v, want %v", f.gotRoleCreds, want)
	}
}

func TestMintFailureShowsOnTheAccountRow(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
	}{
		{name: "the portal rejects the token", err: &ssotypes.UnauthorizedException{}, wantStatus: "error: SSO token expired - run aws sso login"},
		{name: "the role may not be used", err: &smithy.GenericAPIError{Code: "ForbiddenException"}, wantStatus: "error: access denied: GetRoleCredentials"},
		{name: "an unknown failure", err: errors.New("boom\nsecond line"), wantStatus: "error: GetRoleCredentials failed: failed to refresh cached credentials, boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := accountsManager(t, "210987654321/Admin", "111111111111")
			f.roleCredsErr["210987654321/Admin"] = tt.err

			m.pollOnce(context.Background(), true)
			st := m.GetState()

			prod := statusOf(t, st, "prod")
			if prod.Status != tt.wantStatus || prod.Role != "" {
				t.Errorf("row of prod = status %q, role %q, want %q and no role", prod.Status, prod.Role, tt.wantStatus)
			}
			if audit := statusOf(t, st, "audit"); audit.Status != "monitored (first role Billing)" {
				t.Errorf("row of audit = %q, want the other entry to be monitored all the same", audit.Status)
			}
			if _, kept := m.work.minted["210987654321"]; kept {
				t.Error("the account whose credentials failed must not stay minted")
			}
			if st.DiscoveryError != (Problem{}) {
				t.Errorf("DiscoveryError = %+v, want none", st.DiscoveryError)
			}
		})
	}
}

// A failed mint is tried again on the next discovery, so the row recovers once
// the cause is gone.
func TestMintFailureIsRetriedOnTheNextDiscovery(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin")
	f.roleCredsErr["210987654321/Admin"] = &smithy.GenericAPIError{Code: "ForbiddenException"}
	ctx := context.Background()

	m.pollOnce(ctx, true)
	if got := statusOf(t, m.GetState(), "prod").Status; got != "error: access denied: GetRoleCredentials" {
		t.Fatalf("first pass: status = %q, want the access error", got)
	}

	delete(f.roleCredsErr, "210987654321/Admin")
	m.pollOnce(ctx, true)

	if got := statusOf(t, m.GetState(), "prod"); got.Status != StatusMonitored || got.Role != "Admin" {
		t.Errorf("second pass: row of prod = status %q, role %q, want monitored as Admin", got.Status, got.Role)
	}
	if got := f.count("GetRoleCredentials"); got != 2 {
		t.Errorf("GetRoleCredentials was called %d times, want 2 (the failure, then the retry)", got)
	}
}

func TestEntriesWithoutSSORequireSSO(t *testing.T) {
	m, f := newTestManager(t, Options{Accounts: []string{"210987654321", "210987654321/Admin", testAccount}}, "[default]\nregion = eu-west-1\n", staticCredentials)

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	want := []Account{
		{UpdatedAt: st.Accounts[0].UpdatedAt, ID: testAccount, Name: testAccount, Status: StatusMonitored, Identity: true},
		{UpdatedAt: st.Accounts[0].UpdatedAt, ID: "210987654321", Name: "210987654321", Status: StatusRequiresSSO},
	}
	if !reflect.DeepEqual(st.Accounts, want) {
		t.Errorf("Accounts = %+v, want %+v", st.Accounts, want)
	}
	for _, op := range []string{"ListAccounts", "ListAccountRoles", "GetRoleCredentials", "CreateToken"} {
		if got := f.count(op); got != 0 {
			t.Errorf("%s was called %d times for a profile without SSO, want none", op, got)
		}
	}
}

func TestEntriesWhenTheTokenExpiredBeforeAnyList(t *testing.T) {
	m, f := accountsManager(t, "210987654321")
	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(-time.Hour), false)

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if len(st.Accounts) != 2 {
		t.Fatalf("Accounts = %+v, want the identity row and the entry", viewOf(st.Accounts))
	}
	if got, want := st.Accounts[1], "error: SSO token expired - run aws sso login"; got.Status != want || got.Name != "210987654321" || got.ID != "210987654321" {
		t.Errorf("row of the entry = %+v, want its ID as the name and the status %q", got, want)
	}
	if got := f.count("GetRoleCredentials"); got != 0 {
		t.Errorf("GetRoleCredentials was called %d times without a token, want none", got)
	}
}

// What was monitored keeps being monitored while the login is expired: the
// credentials of the accounts are not asked for again, and the title says why
// the list is old.
func TestEntriesStayMonitoredWhenTheLoginExpires(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin", "222222222222/Admin")
	ctx := context.Background()
	m.pollOnce(ctx, true)
	before := f.count("GetRoleCredentials")

	writeSSOToken(t, ownerStartURL, "fake-access-token", time.Now().Add(-time.Hour), false)
	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if got := statusOf(t, st, "prod"); got.Status != StatusMonitored || got.Role != "Admin" {
		t.Errorf("row of prod = status %q, role %q, want it to stay monitored as Admin", got.Status, got.Role)
	}
	if got := statusOf(t, st, "beta").Status; got != "error: role Admin not listed for this account" {
		t.Errorf("row of beta = %q, want the role problem of the last list", got)
	}
	if got := f.count("GetRoleCredentials"); got != before {
		t.Errorf("GetRoleCredentials was called %d more times with an expired token, want none", got-before)
	}
}

func TestMintedAccountsAreReusedAcrossDiscoveries(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin", "111111111111")
	ctx := context.Background()

	m.pollOnce(ctx, true)
	m.pollOnce(ctx, true)

	if got := f.count("GetRoleCredentials"); got != 2 {
		t.Errorf("GetRoleCredentials was called %d times in two passes, want 2: once per monitored entry", got)
	}
	slices.Sort(f.gotRoleCreds)
	if want := []string{"111111111111/Billing", "210987654321/Admin"}; !reflect.DeepEqual(f.gotRoleCreds, want) {
		t.Errorf("GetRoleCredentials was called for %v, want %v", f.gotRoleCreds, want)
	}
	if got := statusOf(t, m.GetState(), "prod").Status; got != StatusMonitored {
		t.Errorf("status of prod after the second pass = %q, want monitored", got)
	}
}

// Credentials minted for a role stop being reused when the list says another
// role is the one to use, and when the account leaves the list.
func TestMintedAccountsFollowTheList(t *testing.T) {
	m, f := accountsManager(t, "111111111111")
	ctx := context.Background()
	m.pollOnce(ctx, true)

	f.ssoRoles["111111111111"] = []string{"Audit", "Billing"}
	m.pollOnce(ctx, true)

	if got := statusOf(t, m.GetState(), "audit"); got.Status != "monitored (first role Audit)" || got.Role != "Audit" {
		t.Errorf("row of audit = status %q, role %q, want the new first role, Audit", got.Status, got.Role)
	}
	if want := []string{"111111111111/Billing", "111111111111/Audit"}; !reflect.DeepEqual(f.gotRoleCreds, want) {
		t.Errorf("GetRoleCredentials was called for %v, want %v", f.gotRoleCreds, want)
	}

	f.ssoAccounts = f.ssoAccounts[:2]
	m.pollOnce(ctx, true)

	if got := statusOf(t, m.GetState(), "111111111111").Status; got != "error: not accessible with this login" {
		t.Errorf("status of an account that left the list = %q, want it to be not accessible", got)
	}
	if len(m.work.minted) != 0 || len(m.work.outcomes) != 0 {
		t.Errorf("minted = %v, outcomes = %v, want the credentials and the outcome of the account dropped", m.work.minted, m.work.outcomes)
	}
}

// An account whose role changed and whose new credentials cannot be had is not
// monitored with the old ones: the row says so, and so does the manager.
func TestFailedMintAfterARoleChangeDropsTheOldCredentials(t *testing.T) {
	m, f := accountsManager(t, "111111111111")
	ctx := context.Background()
	m.pollOnce(ctx, true)
	if got := m.work.minted["111111111111"].role; got != "Billing" {
		t.Fatalf("first pass: minted role = %q, want Billing", got)
	}

	f.ssoRoles["111111111111"] = []string{"Audit", "Billing"}
	f.roleCredsErr["111111111111/Audit"] = &smithy.GenericAPIError{Code: "ForbiddenException"}
	m.pollOnce(ctx, true)

	if got := statusOf(t, m.GetState(), "audit").Status; got != "error: access denied: GetRoleCredentials" {
		t.Errorf("row of audit = %q, want the access error", got)
	}
	if _, kept := m.work.minted["111111111111"]; kept {
		t.Error("the credentials of the old role stay minted after the new ones failed")
	}
}

// A throttled mint ends the pass: the entries after it make no call, and the
// rows come from what is known.
func TestThrottledMintSkipsTheRestOfTheEntries(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin", "111111111111")
	f.roleCredsErr["210987654321/Admin"] = &smithy.GenericAPIError{Code: "ThrottlingException"}

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if got := f.count("GetRoleCredentials"); got != 1 {
		t.Errorf("GetRoleCredentials was called %d times, want 1: the throttled call ends the pass", got)
	}
	if got, want := statusOf(t, st, "prod").Status, "error: throttled by AWS, slowing down"; got != want {
		t.Errorf("row of prod = %q, want %q", got, want)
	}
	if got := statusOf(t, st, "audit").Status; got != StatusAvailable {
		t.Errorf("row of audit = %q, want %q: it was not looked at in this pass", got, StatusAvailable)
	}
	if len(st.Accounts) != 4 {
		t.Errorf("Accounts = %+v, want the 4 rows of the list", viewOf(st.Accounts))
	}

	delete(f.roleCredsErr, "210987654321/Admin")
	m.pollOnce(context.Background(), true)

	if got := statusOf(t, m.GetState(), "audit").Status; got != "monitored (first role Billing)" {
		t.Errorf("row of audit on the next pass = %q, want it to be monitored", got)
	}
}

// A mint whose request never answers does not hold the poller: the pass goes on
// when the call timeout passes, and the row says the portal was unreachable. The
// request itself is not cut by the timeout, because the SDK's credentials cache
// runs it under a context without deadline.
func TestHangingMintDoesNotBlockThePoller(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin")
	f.hang(t, "GetRoleCredentials")
	m.callTimeout = 100 * time.Millisecond

	m.pollOnce(context.Background(), true)

	if got, want := statusOf(t, m.GetState(), "prod").Status, "error: AWS unreachable: timeout"; got != want {
		t.Errorf("row of prod = %q, want %q", got, want)
	}
}

// The keys of the monitored accounts live in the manager's own work and nowhere
// else: nothing the dashboard can read shows them.
func TestMintedCredentialsNeverReachTheState(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin")

	m.pollOnce(context.Background(), true)

	if got := f.count("GetRoleCredentials"); got != 1 {
		t.Fatalf("GetRoleCredentials was called %d times, want 1: the test would prove nothing", got)
	}
	text := fmt.Sprintf("%+v", m.GetState())
	for _, secret := range []string{"AKIDFAKEROLE", "fake-role-secret", "fake-role-session", "fake-access-token", "root@example.com"} {
		if strings.Contains(text, secret) {
			t.Errorf("the state shows %q: %s", secret, text)
		}
	}
}

// An sso_session profile mints through the SDK's token provider too: the token
// it refreshed is the one that goes to GetRoleCredentials, and the refresh
// happens once.
func TestSSOSessionEntriesUseTheSDKTokenProvider(t *testing.T) {
	m, f := newTestManager(t, Options{Profile: "dev", Accounts: []string{"210987654321/Admin"}}, sessionConfig, sessionCredentials)
	writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), true)
	f.oidcToken = "fake-new-access-token"
	f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo(testAccount, "dev"), ssoInfo("210987654321", "prod")}
	f.ssoRoles[testAccount] = []string{"ReadOnly"}
	f.ssoRoles["210987654321"] = []string{"Admin"}

	m.pollOnce(context.Background(), true)
	st := m.GetState()

	if got := statusOf(t, st, "prod"); got.Status != StatusMonitored || got.Role != "Admin" {
		t.Errorf("row of prod = status %q, role %q, want monitored as Admin", got.Status, got.Role)
	}
	if f.oidcCalls != 1 {
		t.Errorf("CreateToken was called %d times, want 1: the refreshed token is cached", f.oidcCalls)
	}
	for _, token := range f.gotTokens {
		if token != "fake-new-access-token" {
			t.Fatalf("a call carried the token %q, want the refreshed one", token)
		}
	}
	if want := []string{"210987654321/Admin"}; !reflect.DeepEqual(f.gotRoleCreds, want) {
		t.Errorf("GetRoleCredentials was called for %v, want %v", f.gotRoleCreds, want)
	}
}

// sessionAccountsManager is an sso_session profile with a fresh login that
// reaches testAccount "dev" and "210987654321" "prod" (Admin), and the entry
// "210987654321/Admin".
func sessionAccountsManager(t *testing.T) (*AWSClientManager, *fakeServices) {
	t.Helper()
	m, f := newTestManager(t, Options{Profile: "dev", Accounts: []string{"210987654321/Admin"}}, sessionConfig, sessionCredentials)
	writeSSOToken(t, "corp", "fake-access-token", time.Now().Add(time.Hour), true)
	f.ssoAccounts = []ssotypes.AccountInfo{ssoInfo(testAccount, "dev"), ssoInfo("210987654321", "prod")}
	f.ssoRoles[testAccount] = []string{"ReadOnly"}
	f.ssoRoles["210987654321"] = []string{"Admin"}
	return m, f
}

// The credentials of a monitored account outlive the pass that minted them: when
// they expire the SDK asks for them again. For an sso_session profile that goes
// through the token provider, which refreshes an expired token on the way.
func TestMintedSSOSessionCredentialsRefreshTheTokenLater(t *testing.T) {
	m, f := sessionAccountsManager(t)
	ctx := context.Background()
	m.pollOnce(ctx, true)
	conn, ok := m.work.minted["210987654321"]
	if !ok || f.oidcCalls != 0 {
		t.Fatalf("minted = %+v, CreateToken calls = %d, want the account minted without a refresh", m.work.minted, f.oidcCalls)
	}

	writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), true)
	f.oidcToken = "fake-new-access-token"
	cache, ok := conn.cfg.Credentials.(*awssdk.CredentialsCache)
	if !ok {
		t.Fatalf("minted credentials are a %T, want the SDK's credentials cache", conn.cfg.Credentials)
	}
	cache.Invalidate()

	creds, err := cache.Retrieve(ctx)

	if err != nil || creds.AccessKeyID != "AKIDFAKEROLE" {
		t.Fatalf("Retrieve() = (%q, %v), want the keys of the role after a token refresh", creds.AccessKeyID, err)
	}
	if f.oidcCalls != 1 {
		t.Errorf("CreateToken was called %d times, want 1", f.oidcCalls)
	}
	if got := f.gotTokens[len(f.gotTokens)-1]; got != "fake-new-access-token" {
		t.Errorf("the last call carried the token %q, want the refreshed one", got)
	}
}

// A legacy profile never refreshes anything: an expired token is an expired
// login, and the cache file is left as it is.
func TestMintedLegacyCredentialsNeverRefreshTheToken(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin")
	ctx := context.Background()
	m.pollOnce(ctx, true)
	conn, ok := m.work.minted["210987654321"]
	if !ok {
		t.Fatalf("minted = %+v, want the credentials of prod", m.work.minted)
	}

	writeSSOToken(t, ownerStartURL, "fake-old-access-token", time.Now().Add(-time.Hour), true)
	file, err := os.ReadFile(m.work.ssoTokenPath)
	if err != nil {
		t.Fatalf("read the token file: %v", err)
	}
	cache, ok := conn.cfg.Credentials.(*awssdk.CredentialsCache)
	if !ok {
		t.Fatalf("minted credentials are a %T, want the SDK's credentials cache", conn.cfg.Credentials)
	}
	cache.Invalidate()

	_, err = cache.Retrieve(ctx)

	var tokenErr *ssocreds.InvalidTokenError
	if !errors.As(err, &tokenErr) {
		t.Errorf("Retrieve() error = %v, want the SDK's invalid token error", err)
	}
	if got, _ := classifyError(opRoleAccess, err, nil); got != (Problem{Text: textSSOExpired, Hint: hintSSOExpired}) {
		t.Errorf("classifyError() = %+v, want the expired login", got)
	}
	if f.oidcCalls != 0 {
		t.Errorf("CreateToken was called %d times, want none for a legacy profile", f.oidcCalls)
	}
	if after, _ := os.ReadFile(m.work.ssoTokenPath); string(after) != string(file) {
		t.Error("the token file changed")
	}
}

// Accounts that aws.accounts stops naming lose their credentials.
func TestMintedAccountsNoLongerWantedAreDropped(t *testing.T) {
	m, _ := accountsManager(t, "210987654321/Admin", "111111111111")
	ctx := context.Background()
	m.pollOnce(ctx, true)
	if len(m.work.minted) != 2 {
		t.Fatalf("minted = %v, want the two accounts", m.work.minted)
	}

	m.settings.accounts = []string{"111111111111"}
	m.pollOnce(ctx, true)
	st := m.GetState()

	if _, kept := m.work.minted["210987654321"]; kept || len(m.work.minted) != 1 || len(m.work.outcomes) != 1 {
		t.Errorf("minted = %v, outcomes = %v, want only the account that is still named", m.work.minted, m.work.outcomes)
	}
	if got := statusOf(t, st, "prod").Status; got != StatusAvailable {
		t.Errorf("row of prod = %q, want it available again", got)
	}
}

// When the token cannot be read, an entry that still needs its credentials shows
// the problem of the token, not a message of the SDK's token provider.
func TestEntriesNeedingCredentialsShowTheTokenProblem(t *testing.T) {
	m, f := sessionAccountsManager(t)
	f.roleCredsErr["210987654321/Admin"] = &smithy.GenericAPIError{Code: "ForbiddenException"}
	ctx := context.Background()
	m.pollOnce(ctx, true)
	if got := statusOf(t, m.GetState(), "prod").Status; got != "error: access denied: GetRoleCredentials" {
		t.Fatalf("first pass: status = %q, want the access error", got)
	}
	calls := f.count("GetRoleCredentials")

	delete(f.roleCredsErr, "210987654321/Admin")
	writeSSOToken(t, "corp", "fake-old-access-token", time.Now().Add(-time.Hour), false)
	m.pollOnce(ctx, true)
	st := m.GetState()

	if want := "error: SSO token expired - run aws sso login"; statusOf(t, st, "prod").Status != want {
		t.Errorf("row of prod = %q, want %q", statusOf(t, st, "prod").Status, want)
	}
	if want := (Problem{Text: textSSOExpired, Hint: hintSSOExpired}); st.DiscoveryError != want {
		t.Errorf("DiscoveryError = %+v, want %+v", st.DiscoveryError, want)
	}
	if got := f.count("GetRoleCredentials"); got != calls {
		t.Errorf("GetRoleCredentials was called %d more times without a token, want none", got-calls)
	}
}

// When the listing fails, the portal cannot be relied on in that pass: an entry
// that still needs its credentials makes no call and shows the problem of the
// listing, an account that is monitored already stays so, and the next pass that
// can list gets the pending entry its credentials.
func TestListingFailureMakesNoMintCallAndShowsTheProblem(t *testing.T) {
	m, f := accountsManager(t, "210987654321/Admin", "111111111111")
	f.roleCredsErr["111111111111/Billing"] = &smithy.GenericAPIError{Code: "ForbiddenException"}
	ctx := context.Background()

	// First pass: prod is minted, audit is refused.
	m.pollOnce(ctx, true)
	if got := statusOf(t, m.GetState(), "audit").Status; got != "error: access denied: GetRoleCredentials" {
		t.Fatalf("first pass: status of audit = %q, want the access error", got)
	}
	calls := f.count("GetRoleCredentials")

	// Second pass: the listing fails, and audit would be allowed now.
	delete(f.roleCredsErr, "111111111111/Billing")
	f.ssoErrs["ListAccounts"] = &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	m.pollOnce(ctx, true)
	st := m.GetState()

	problem := Problem{Text: prefixUnreachable + "network error"}
	if st.DiscoveryError != problem {
		t.Errorf("second pass: DiscoveryError = %+v, want %+v", st.DiscoveryError, problem)
	}
	if got := f.count("GetRoleCredentials"); got != calls {
		t.Errorf("second pass: GetRoleCredentials was called %d more times after the listing failed, want none", got-calls)
	}
	if got := statusOf(t, st, "audit"); got.Status != problem.StatusText() || got.Role != "" {
		t.Errorf("second pass: row of audit = status %q, role %q, want %q and no role", got.Status, got.Role, problem.StatusText())
	}
	if got := statusOf(t, st, "prod"); got.Status != StatusMonitored || got.Role != "Admin" {
		t.Errorf("second pass: row of prod = status %q, role %q, want it to stay monitored as Admin", got.Status, got.Role)
	}
	if len(st.Accounts) != 4 {
		t.Errorf("second pass: Accounts = %+v, want the 4 rows of the last list", viewOf(st.Accounts))
	}

	// Third pass: the listing works again.
	delete(f.ssoErrs, "ListAccounts")
	m.pollOnce(ctx, true)
	st = m.GetState()

	if got := statusOf(t, st, "audit"); got.Status != "monitored (first role Billing)" || got.Role != "Billing" {
		t.Errorf("third pass: row of audit = status %q, role %q, want it monitored with its first role, Billing", got.Status, got.Role)
	}
	if got := f.count("GetRoleCredentials"); got != calls+1 {
		t.Errorf("third pass: GetRoleCredentials was called %d times in all, want %d", got, calls+1)
	}
	if st.DiscoveryError != (Problem{}) {
		t.Errorf("third pass: DiscoveryError = %+v, want none", st.DiscoveryError)
	}
}

// The account rows of a configured entry, as the pure function builds them.
func TestAccountRowsWithEntries(t *testing.T) {
	now := time.Date(2026, 10, 25, 9, 8, 7, 0, time.UTC)
	list := []ssoAccount{
		{id: testAccount, name: "dev", roles: []string{"ReadOnly"}},
		{id: "210987654321", name: "prod", roles: []string{"ReadOnly", "Admin"}},
		{id: "111111111111", name: "audit", roles: []string{"Billing"}},
	}
	entries := []accountEntry{{id: "999999999999"}, {id: "210987654321", role: "Admin"}, {id: "111111111111"}}
	outcomes := map[string]accountOutcome{
		"210987654321": {role: "Admin"},
		"111111111111": {role: "Billing", firstRole: true},
	}
	tests := []struct {
		name string
		want []statusRow
		in   accountsInput
	}{
		{
			name: "not SSO: every entry requires SSO, in config order",
			in:   accountsInput{now: now, identityID: testAccount, entries: entries},
			want: []statusRow{srow(testAccount, "monitored", ""), srow("999999999999", "requires SSO", ""), srow("210987654321", "requires SSO", ""), srow("111111111111", "requires SSO", "")},
		},
		{
			name: "SSO that was never listed: the problem of the discovery",
			in: accountsInput{
				now: now, identityID: testAccount, entries: entries, isSSO: true,
				listProblem: Problem{Text: textSSOExpired, Hint: hintSSOExpired},
			},
			want: []statusRow{
				srow(testAccount, "monitored", ""),
				srow("999999999999", "error: SSO token expired - run aws sso login", ""),
				srow("210987654321", "error: SSO token expired - run aws sso login", ""),
				srow("111111111111", "error: SSO token expired - run aws sso login", ""),
			},
		},
		{
			name: "SSO that was listed: outcomes, then the entries the list lacks",
			in:   accountsInput{now: now, identityID: testAccount, entries: entries, isSSO: true, sso: list, outcomes: outcomes},
			want: []statusRow{
				srow("dev", "monitored", ""),
				srow("audit", "monitored (first role Billing)", "Billing"),
				srow("prod", "monitored", "Admin"),
				srow("999999999999", "error: not accessible with this login", ""),
			},
		},
		{
			name: "SSO that was listed, with a problem on an entry and an entry not looked at yet",
			in: accountsInput{
				now: now, identityID: testAccount, entries: entries, isSSO: true, sso: list,
				outcomes: map[string]accountOutcome{"210987654321": {problem: Problem{Text: "role Admin not listed for this account"}}},
			},
			want: []statusRow{
				srow("dev", "monitored", ""),
				srow("audit", "available", ""),
				srow("prod", "error: role Admin not listed for this account", ""),
				srow("999999999999", "error: not accessible with this login", ""),
			},
		},
		{
			name: "SSO that was listed and is empty: every entry is out of reach",
			in:   accountsInput{now: now, identityID: testAccount, entries: entries[:1], isSSO: true, sso: []ssoAccount{}},
			want: []statusRow{srow(testAccount, "monitored", ""), srow("999999999999", "error: not accessible with this login", "")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusRowsOf(accountRows(tt.in)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rows = %+v, want %+v", got, tt.want)
			}
		})
	}
}
