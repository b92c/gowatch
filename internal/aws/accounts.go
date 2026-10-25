package aws

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// The most SSO accounts, and the most roles of each, that discovery keeps. They
// are also the page sizes it asks for.
const (
	maxSSOAccounts = 100
	maxSSORoles    = 20
)

// The texts of the problems of an account row.
const (
	textNoRoles       = "no roles available"
	textNotAccessible = "not accessible with this login"
)

// discoverIdentity loads the SDK configuration, resolves the credentials and
// asks STS who they belong to, each step under its own timeout. It records the
// outcome in m.work and reports whether the pass may go on: any failure,
// throttling included, ends it, and the next discovery tries again from the
// start, so keys exported again in the meantime are picked up. It does not build
// the account rows: on success discoverAccounts runs next, then accountRows.
func (m *AWSClientManager) discoverIdentity(ctx context.Context) bool {
	w := &m.work

	cctx, cancel := m.callContext(ctx)
	cfg, err := m.loadConfig(cctx, m.settings.loadOptions()...)
	cancel()
	if err != nil {
		p, _ := classifyError(opLoadConfig, err, nil)
		return m.failIdentity(p)
	}
	w.cfg, w.region, w.loaded = cfg, cfg.Region, true

	if w.cfg.Credentials == nil {
		return m.failIdentity(Problem{Text: textNoProvider, Hint: hintNoProvider})
	}
	cctx, cancel = m.callContext(ctx)
	_, err = w.cfg.Credentials.Retrieve(cctx)
	cancel()
	if err != nil {
		p, _ := classifyError(opAuth, err, credentialSources(w.cfg.Credentials))
		return m.failIdentity(p)
	}

	cctx, cancel = m.callContext(ctx)
	out, err := m.svc.STS(w.cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	cancel()
	if err != nil {
		p, _ := classifyError(opGetCallerIdentity, err, nil)
		return m.failIdentity(p)
	}

	w.identity = Identity{AccountID: awssdk.ToString(out.Account), ARN: awssdk.ToString(out.Arn), UserID: awssdk.ToString(out.UserId)}
	w.conn, w.connProblem = ConnConnected, Problem{}
	return true
}

// failIdentity records a connection problem. The row of the identity, if the
// manager has shown one, carries the problem in its status; the other rows stay
// as they were. It always reports false: the pass stops.
func (m *AWSClientManager) failIdentity(p Problem) bool {
	w := &m.work
	w.conn, w.connProblem = ConnError, p

	now := m.now()
	for i := range w.accounts {
		if w.accounts[i].Identity {
			w.accounts[i].Status = p.StatusText()
			w.accounts[i].UpdatedAt = now
		}
	}
	return false
}

// credentialSources lists the credential sources the SDK resolved p from, or
// nil when p does not say.
func credentialSources(p awssdk.CredentialsProvider) []awssdk.CredentialSource {
	if source, ok := p.(awssdk.CredentialProviderSource); ok {
		return source.ProviderSources()
	}
	return nil
}

// ssoAccount is one account of the SSO login and the roles the login can use in
// it. The e-mail address the portal sends with each account is never read.
type ssoAccount struct {
	id    string
	name  string // never empty: the SSO name, otherwise the ID
	roles []string
}

// listSSOAccounts lists the accounts of an SSO login and the roles in each, with
// the access token of the login. It asks ListAccounts for pages of 100 until the
// service has no more or 100 accounts are kept, in the order the service
// answers, then ListAccountRoles once per account for at most 20 roles. Every
// call has its own timeout. When a call fails the list is nil and failedOp names
// the call, for classifyError. On success the list is never nil, even when the
// login reaches no account.
func listSSOAccounts(ctx context.Context, api ssoAPI, accessToken string, timeout time.Duration) (accounts []ssoAccount, failedOp string, err error) {
	accounts = []ssoAccount{}
	var next *string
	// A page holds at least one account, so a listing is never cut short by
	// counting pages; a service that keeps answering empty pages is.
	for range maxSSOAccounts {
		page, err := listAccountsPage(ctx, api, accessToken, next, timeout)
		if err != nil {
			return nil, opListAccounts, err
		}
		for _, info := range page.AccountList {
			id := awssdk.ToString(info.AccountId)
			if id == "" || len(accounts) == maxSSOAccounts {
				continue
			}
			name := awssdk.ToString(info.AccountName)
			if name == "" {
				name = id
			}
			accounts = append(accounts, ssoAccount{id: id, name: name})
		}
		if len(accounts) == maxSSOAccounts || awssdk.ToString(page.NextToken) == "" {
			break
		}
		next = page.NextToken
	}

	for i := range accounts {
		roles, err := listAccountRoles(ctx, api, accessToken, accounts[i].id, timeout)
		if err != nil {
			return nil, opListAccountRoles, err
		}
		accounts[i].roles = roles
	}
	return accounts, "", nil
}

// listAccountsPage asks for one page of accounts under its own timeout.
func listAccountsPage(ctx context.Context, api ssoAPI, accessToken string, next *string, timeout time.Duration) (*sso.ListAccountsOutput, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return api.ListAccounts(cctx, &sso.ListAccountsInput{
		AccessToken: awssdk.String(accessToken),
		MaxResults:  awssdk.Int32(maxSSOAccounts),
		NextToken:   next,
	})
}

// listAccountRoles asks for the first page of roles of one account under its own
// timeout and keeps at most maxSSORoles of them.
func listAccountRoles(ctx context.Context, api ssoAPI, accessToken, accountID string, timeout time.Duration) ([]string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := api.ListAccountRoles(cctx, &sso.ListAccountRolesInput{
		AccessToken: awssdk.String(accessToken),
		AccountId:   awssdk.String(accountID),
		MaxResults:  awssdk.Int32(maxSSORoles),
	})
	if err != nil {
		return nil, err
	}
	var roles []string
	for _, role := range out.RoleList {
		if name := awssdk.ToString(role.RoleName); name != "" && len(roles) < maxSSORoles {
			roles = append(roles, name)
		}
	}
	return roles, nil
}

// discoverAccounts lists the accounts of the SSO login of the profile and
// resolves the entries of aws.accounts against the list. It runs after a
// successful identity pass and records its outcome in m.work: the list, or the
// problem that kept it from being read. A profile without SSO has neither. A
// problem leaves the last list as it was, so the dashboard keeps what it knew and
// says why it is old; the entries are resolved against that list all the same,
// but without a call to the portal: an entry that still needs its credentials
// shows the problem, and the accounts that are monitored already stay so. It
// reports false when AWS throttled a call, and then the rest of the pass makes no
// call.
func (m *AWSClientManager) discoverAccounts(ctx context.Context) bool {
	w := &m.work

	profile, isSSO := detectSSO(ctx, m.settings)
	w.sso, w.isSSO = profile, isSSO
	if !isSSO {
		w.discoveryProblem, w.ssoAccounts = Problem{}, nil
		w.minted, w.outcomes = nil, nil
		return true
	}

	accessToken, err := m.bearerToken(ctx, profile)
	if err != nil {
		p, throttled := classifyError(opSSOAccess, err, nil)
		w.discoveryProblem = p
		if throttled {
			return false
		}
		// Without a token no credentials can be minted: an entry that needs
		// them shows the same problem.
		return m.resolveEntries(ctx, p)
	}

	ssoCfg := w.cfg.Copy()
	ssoCfg.Region = profile.region
	w.ssoAPI = m.svc.SSO(ssoCfg)

	accounts, failedOp, err := listSSOAccounts(ctx, w.ssoAPI, accessToken, m.callTimeout)
	if err != nil {
		p, throttled := classifyError(failedOp, err, nil)
		w.discoveryProblem = p
		if throttled {
			return false
		}
		// The portal failed in this pass, so no credentials are asked of it: an
		// entry that needs them shows the problem of the listing.
		return m.resolveEntries(ctx, p)
	}
	w.ssoAccounts, w.discoveryProblem = accounts, Problem{}
	return m.resolveEntries(ctx, Problem{})
}

// accountEntry is one entry of aws.accounts: an account and, optionally, the role
// to monitor it with.
type accountEntry struct {
	id   string
	role string // empty: pick one (chooseRole)
}

// parseAccountEntry reads "<account id>" or "<account id>/<role>". The config
// already checked the shape.
func parseAccountEntry(s string) accountEntry {
	id, role, _ := strings.Cut(s, "/")
	return accountEntry{id: id, role: role}
}

// configuredEntries parses aws.accounts. The result keeps the config order and
// leaves out the account of the active credentials, which is always monitored,
// and every repeat: the first entry of an account wins.
func configuredEntries(accounts []string, identityID string) []accountEntry {
	var entries []accountEntry
	seen := map[string]struct{}{identityID: {}}
	for _, s := range accounts {
		e := parseAccountEntry(s)
		if _, repeated := seen[e.id]; repeated {
			continue
		}
		seen[e.id] = struct{}{}
		entries = append(entries, e)
	}
	return entries
}

// chooseRole picks the role to monitor an account with, from the roles the login
// can use in it: the role of the entry when listed; for an entry without one, the
// sso_role_name of the profile when listed, otherwise the first role listed
// (firstRole is then true, and the status says so). A role that cannot be used is
// a Problem with its Text set.
func chooseRole(e accountEntry, roles []string, profileRole string) (role string, firstRole bool, problem Problem) {
	if e.role != "" {
		if slices.Contains(roles, e.role) {
			return e.role, false, Problem{}
		}
		return "", false, Problem{Text: "role " + e.role + " not listed for this account"}
	}
	if profileRole != "" && slices.Contains(roles, profileRole) {
		return profileRole, false, Problem{}
	}
	if len(roles) == 0 {
		return "", false, Problem{Text: textNoRoles}
	}
	return roles[0], true, Problem{}
}

// accountOutcome is what resolving an entry of aws.accounts came to.
type accountOutcome struct {
	role      string // the role the account is monitored with
	problem   Problem
	firstRole bool // the role is the first one listed, not one that was asked for
}

// accountConn is the CloudWatch Logs access of a monitored SSO account: the SDK
// config of the identity with the credentials of the account and role.
type accountConn struct {
	cfg  awssdk.Config
	role string
}

// mintAccount builds the credentials of an SSO account and role with the SDK's
// provider and checks them once: Retrieve makes the one GetRoleCredentials call.
// The provider reads the token from the cache file of the login, or from the SDK's
// token provider for an sso_session profile, so it never writes a legacy cache
// file. The config it returns is the identity's, with those credentials.
//
// The credentials cache runs the provider under a context without deadline, so the
// GetRoleCredentials request can outlive callContext; the poller itself returns
// when the deadline passes.
func (m *AWSClientManager) mintAccount(ctx context.Context, id, role string) (awssdk.Config, error) {
	w := &m.work
	provider := ssocreds.New(w.ssoAPI, id, role, w.sso.startURL, func(o *ssocreds.Options) {
		o.CachedTokenFilepath = w.ssoTokenPath
		if w.ssoTokenProvider != nil {
			o.SSOTokenProvider = w.ssoTokenProvider
		}
	})
	cache := awssdk.NewCredentialsCache(provider)

	cctx, cancel := m.callContext(ctx)
	defer cancel()
	if _, err := cache.Retrieve(cctx); err != nil {
		return awssdk.Config{}, err
	}

	cfg := w.cfg.Copy()
	cfg.Credentials = cache
	return cfg, nil
}

// resolveEntries goes through the entries of aws.accounts that the SSO list
// contains. For each it chooses the role and mints the credentials of the account;
// credentials minted earlier for the same role are kept, with no call. A role that
// cannot be used, or a failed mint, is the problem of that account; blocked, when
// set (the token could not be read, or the listing failed), is the problem of
// every account that needs a mint but cannot have one, so no call is made for it.
// Accounts that are no longer wanted or listed lose their credentials. It reports
// false when a call was throttled: the entries after it are left for the next
// pass.
func (m *AWSClientManager) resolveEntries(ctx context.Context, blocked Problem) bool {
	w := &m.work
	entries := configuredEntries(m.settings.accounts, w.identity.AccountID)
	if len(entries) == 0 {
		w.minted, w.outcomes = nil, nil
		return true
	}
	if w.minted == nil {
		w.minted = make(map[string]accountConn)
	}
	if w.outcomes == nil {
		w.outcomes = make(map[string]accountOutcome)
	}

	wanted := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		wanted[e.id] = struct{}{}
	}
	for id := range w.minted {
		if _, ok := wanted[id]; !ok {
			delete(w.minted, id)
		}
	}
	for id := range w.outcomes {
		if _, ok := wanted[id]; !ok {
			delete(w.outcomes, id)
		}
	}

	listed := make(map[string]ssoAccount, len(w.ssoAccounts))
	for _, a := range w.ssoAccounts {
		listed[a.id] = a
	}

	for _, e := range entries {
		account, ok := listed[e.id]
		if !ok {
			delete(w.minted, e.id)
			delete(w.outcomes, e.id)
			continue
		}

		role, firstRole, problem := chooseRole(e, account.roles, w.sso.roleName)
		if problem.Text != "" {
			delete(w.minted, e.id)
			w.outcomes[e.id] = accountOutcome{problem: problem}
			continue
		}
		if conn, ok := w.minted[e.id]; ok && conn.role == role {
			w.outcomes[e.id] = accountOutcome{role: role, firstRole: firstRole}
			continue
		}

		delete(w.minted, e.id)
		if blocked.Text != "" {
			w.outcomes[e.id] = accountOutcome{problem: blocked}
			continue
		}
		cfg, err := m.mintAccount(ctx, e.id, role)
		if err != nil {
			p, throttled := classifyError(opRoleAccess, err, nil)
			w.outcomes[e.id] = accountOutcome{problem: p}
			if throttled {
				return false
			}
			continue
		}
		w.minted[e.id] = accountConn{cfg: cfg, role: role}
		w.outcomes[e.id] = accountOutcome{role: role, firstRole: firstRole}
	}
	return true
}

// accountsInput is what accountRows builds the rows from.
type accountsInput struct {
	now         time.Time
	outcomes    map[string]accountOutcome // by account ID, for entries found in the SSO list
	listProblem Problem                   // work.discoveryProblem, used when no list is known
	identityID  string
	sso         []ssoAccount   // the last SSO list, nil when none was ever listed
	entries     []accountEntry // aws.accounts in config order, identity account and duplicates removed
	isSSO       bool
}

// buildAccountRows builds the rows from what the poller knows.
func (m *AWSClientManager) buildAccountRows() []Account {
	w := &m.work
	return accountRows(accountsInput{
		now:         m.now(),
		identityID:  w.identity.AccountID,
		sso:         w.ssoAccounts,
		entries:     configuredEntries(m.settings.accounts, w.identity.AccountID),
		outcomes:    w.outcomes,
		listProblem: w.discoveryProblem,
		isSSO:       w.isSSO,
	})
}

// accountRows builds the rows of the AWS view. The account of the active
// credentials comes first, always monitored, under its SSO name when the login
// lists it. Then come the listed accounts, ordered by name (ignoring case) and
// then by ID: available, or, for an entry of aws.accounts, what resolving it came
// to. Last come the entries the list lacks, in config order: "requires SSO"
// without an SSO login, the problem of the discovery when no list was ever read,
// and "not accessible with this login" when the list does not contain them. Each
// row carries the roles the login can use in it.
func accountRows(in accountsInput) []Account {
	identity := Account{UpdatedAt: in.now, ID: in.identityID, Name: in.identityID, Status: StatusMonitored, Identity: true}
	listed := make(map[string]struct{}, len(in.sso))
	var others []ssoAccount
	for _, a := range in.sso {
		listed[a.id] = struct{}{}
		if a.id == in.identityID {
			identity.Name, identity.Roles = a.name, slices.Clone(a.roles)
			continue
		}
		others = append(others, a)
	}
	slices.SortFunc(others, func(a, b ssoAccount) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.id, b.id))
	})

	entryIDs := make(map[string]struct{}, len(in.entries))
	for _, e := range in.entries {
		entryIDs[e.id] = struct{}{}
	}

	rows := make([]Account, 0, 1+len(others)+len(in.entries))
	rows = append(rows, identity)
	for _, a := range others {
		row := Account{UpdatedAt: in.now, ID: a.id, Name: a.name, Status: StatusAvailable, Roles: slices.Clone(a.roles)}
		if _, isEntry := entryIDs[a.id]; isEntry {
			if outcome, ok := in.outcomes[a.id]; ok {
				row.Status, row.Role = outcome.status(), outcome.role
			}
		}
		rows = append(rows, row)
	}
	for _, e := range in.entries {
		if _, ok := listed[e.id]; !ok {
			rows = append(rows, Account{UpdatedAt: in.now, ID: e.id, Name: e.id, Status: in.missingEntryStatus()})
		}
	}
	return rows
}

// status is the status of an account whose entry of aws.accounts came to o.
func (o accountOutcome) status() string {
	switch {
	case o.problem.Text != "":
		return o.problem.StatusText()
	case o.firstRole:
		return StatusMonitored + " (first role " + o.role + ")"
	}
	return StatusMonitored
}

// missingEntryStatus is the status of an entry of aws.accounts that the SSO list
// does not contain.
func (in accountsInput) missingEntryStatus() string {
	switch {
	case !in.isSSO:
		return StatusRequiresSSO
	case in.sso == nil && in.listProblem.Text != "":
		return in.listProblem.StatusText()
	}
	return Problem{Text: textNotAccessible}.StatusText()
}
