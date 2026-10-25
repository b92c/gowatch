// Package aws gives GoWatch a read-only view of an AWS account.
//
// An AWSClientManager resolves its settings (profile, region, endpoint and
// whether AWS is configured at all) when it is built, from environment
// variables and file metadata only. Everything that talks to AWS runs later, on
// one poller goroutine that main starts: it loads the SDK configuration, asks
// who the credentials belong to, lists the accounts of the SSO login of the
// profile, gets credentials for the accounts that aws.accounts asks to monitor,
// and publishes a snapshot of what it found. The dashboard reads that snapshot
// through GetState, which never waits for AWS.
//
// The package does not import internal/docker, internal/ui or internal/filter.
package aws

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
)

// Statuses of an account row. The dashboard colors them by their text.
const (
	StatusMonitored        = "monitored"
	StatusAvailable        = "available"
	StatusRequiresSSO      = "requires SSO"
	StatusFollowing        = "following"
	StatusFollowingLagging = "following (lagging)"
	StatusNotFollowedCap   = "not followed (cap)"
	StatusNotFollowed      = "-"
)

// Conn is the state of the connection to AWS, as the title of the AWS view
// shows it.
type Conn string

// The connection states.
const (
	ConnNotConfigured Conn = "not configured"
	ConnDisabled      Conn = "disabled"
	ConnConnecting    Conn = "connecting"
	ConnConnected     Conn = "connected"
	ConnError         Conn = "error"
)

// Identity is who the active credentials belong to, as sts:GetCallerIdentity
// reports it.
type Identity struct{ AccountID, ARN, UserID string }

// Account is one row of the AWS view.
type Account struct {
	UpdatedAt time.Time // time of the discovery that produced the row
	ID        string
	Name      string // never empty: the SSO name when known, otherwise the ID
	Role      string // role used to monitor the account through SSO; empty for the identity account
	Status    string
	Roles     []string // roles the SSO login can use in the account
	Identity  bool     // the account of the active credentials
}

// AWSState is a snapshot of what the manager knows. GetState returns a copy
// that shares nothing with the manager.
type AWSState struct {
	UpdatedAt      time.Time
	Identity       Identity
	LastError      Problem // connection problem: config, credentials or identity
	DiscoveryError Problem // SSO discovery problem; the identity can still be connected
	Region         string  // provisional until the SDK resolves it
	Profile        string
	EndpointURL    string
	EnabledReason  string
	Conn           Conn
	Accounts       []Account
	Enabled        bool
	Connected      bool
}

// clone returns a copy of s that shares no slice with it, the roles of each
// account included.
func (s AWSState) clone() AWSState {
	s.Accounts = slices.Clone(s.Accounts)
	for i := range s.Accounts {
		s.Accounts[i].Roles = slices.Clone(s.Accounts[i].Roles)
	}
	return s
}

// Options carries the AWS settings that come from the config file. The zero
// value reproduces the environment-only behavior.
type Options struct {
	// Enabled forces the integration on or off. Nil means auto: it is enabled
	// when the environment or the other options configure AWS.
	Enabled     *bool
	Region      string
	EndpointURL string
	// Profile is the shared config profile; AWS_PROFILE takes priority.
	Profile string
	// Accounts lists the extra accounts to monitor through SSO, each as
	// "<account id>" or "<account id>/<role name>". The config validated the
	// shape of every entry.
	Accounts []string
	// RefreshInterval is how often the poller makes a pass. Zero or less means
	// DefaultRefreshInterval.
	RefreshInterval time.Duration
	// DiscoveryInterval is how often a pass reloads the shared files and asks
	// AWS who the credentials belong to. Zero or less means
	// DefaultDiscoveryInterval.
	DiscoveryInterval time.Duration
}

// The built-in intervals, used when Options leaves them at zero.
const (
	DefaultRefreshInterval   = 15 * time.Second
	DefaultDiscoveryInterval = 60 * time.Second
)

// IsZero reports whether o changes nothing: it forces no enablement and sets no
// value. docker.Configure keeps the current manager for a zero Options.
func (o Options) IsZero() bool {
	return o.Enabled == nil && o.Region == "" && o.EndpointURL == "" && o.Profile == "" &&
		len(o.Accounts) == 0 && o.RefreshInterval == 0 && o.DiscoveryInterval == 0
}

// AWSClientManager holds the resolved AWS settings and the latest snapshot of
// the AWS state. It is safe for concurrent use.
//
// The poller goroutine does every AWS call; the mutex guards the snapshot, the
// started flag and the poller's handles, and is never held during a call.
type AWSClientManager struct {
	svc         services
	loadConfig  configLoader
	now         func() time.Time
	onLine      LineHook
	cancel      context.CancelFunc
	done        chan struct{}
	work        work
	state       AWSState
	settings    settings
	callTimeout time.Duration
	mu          sync.RWMutex
	started     bool
}

// NewAWSClientManager builds a manager from the environment and the shared AWS
// files only.
func NewAWSClientManager() *AWSClientManager {
	return NewAWSClientManagerWithOptions(Options{})
}

// NewAWSClientManagerWithOptions builds a manager from the environment and o,
// with the precedence rules of resolveSettings. It makes no AWS call and reads
// nothing but environment variables and the metadata of the two shared files.
// The initial state says whether AWS is not configured, disabled or about to
// connect.
func NewAWSClientManagerWithOptions(o Options) *AWSClientManager {
	s := resolveSettings(o)

	conn := ConnConnecting
	switch {
	case s.mode == modeOff:
		conn = ConnDisabled
	case !s.enabled:
		conn = ConnNotConfigured
	}

	return &AWSClientManager{
		settings: s,
		state: AWSState{
			Region:        s.region,
			Profile:       s.profile,
			EndpointURL:   s.endpointURL,
			EnabledReason: s.enabledReason,
			Conn:          conn,
			Enabled:       s.enabled,
		},
		work:        work{conn: conn},
		svc:         sdkServices{},
		loadConfig:  config.LoadDefaultConfig,
		now:         time.Now,
		callTimeout: defaultCallTimeout,
	}
}

// GetState returns a copy of the current snapshot. It never waits for an AWS
// call.
func (m *AWSClientManager) GetState() AWSState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.clone()
}

// Region is the region of the current snapshot: the provisional one until the
// SDK has resolved the configuration.
func (m *AWSClientManager) Region() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Region
}

// EndpointURL is the endpoint override of the current snapshot, if any.
func (m *AWSClientManager) EndpointURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.EndpointURL
}

// publishState replaces the snapshot with a copy of st.
func (m *AWSClientManager) publishState(st AWSState) {
	st = st.clone()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = st
}
