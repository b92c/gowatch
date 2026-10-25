package aws

import (
	"context"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
)

// LineHook receives each new CloudWatch log line, with the name of the service
// it came from.
type LineHook func(service, line string)

// defaultCallTimeout bounds every call the poller makes to AWS, the loading of
// the SDK configuration included.
const defaultCallTimeout = 10 * time.Second

// work is what the poller goroutine keeps between passes; only that goroutine
// touches it.
type work struct {
	lastDiscovery time.Time
	cfg           awssdk.Config
	identity      Identity
	connProblem   Problem
	conn          Conn   // starts as the initial state's Conn
	region        string // cfg.Region once a config was loaded
	accounts      []Account
	loaded        bool
}

// Start begins polling AWS in a new goroutine and returns at once. It does
// nothing when the integration is not enabled or already started. The poller
// runs until ctx is cancelled or Stop is called.
func (m *AWSClientManager) Start(ctx context.Context, onLine LineHook) {
	m.mu.Lock()
	if !m.settings.enabled || m.started {
		m.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.started = true
	m.cancel = cancel
	m.done = make(chan struct{})
	m.onLine = onLine
	m.mu.Unlock()

	go m.run(runCtx)
}

// Stop cancels the poller and waits for its goroutine to finish, or for ctx to
// end, in which case it returns the error of ctx. It returns nil when the
// poller never started, and can be called again.
func (m *AWSClientManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel == nil {
		return nil
	}

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the poller goroutine: a discovery pass at once, then a pass every
// refresh interval, which is also a discovery pass whenever the discovery
// interval has gone by since the last one.
func (m *AWSClientManager) run(ctx context.Context) {
	defer close(m.done)

	m.pollOnce(ctx, true)

	ticker := time.NewTicker(m.settings.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollOnce(ctx, m.now().Sub(m.work.lastDiscovery) >= m.settings.discoveryInterval)
		}
	}
}

// pollOnce makes one pass and publishes what it found. A discovery pass asks AWS
// who the credentials belong to; the other passes make no call.
func (m *AWSClientManager) pollOnce(ctx context.Context, discovery bool) {
	if ctx.Err() != nil {
		return
	}
	if discovery {
		m.work.lastDiscovery = m.now()
		m.discoverIdentity(ctx)
	}
	m.publish()
}

// callContext bounds one call to AWS.
func (m *AWSClientManager) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, m.callTimeout)
}

// publish builds a snapshot from the settings and the poller's work and makes it
// the state the dashboard reads. The region is the provisional one until the SDK
// has loaded a configuration.
func (m *AWSClientManager) publish() {
	w := &m.work
	region := m.settings.region
	if w.loaded {
		region = w.region
	}
	m.publishState(AWSState{
		UpdatedAt:     m.now(),
		Identity:      w.identity,
		LastError:     w.connProblem,
		Region:        region,
		Profile:       m.settings.profile,
		EndpointURL:   m.settings.endpointURL,
		EnabledReason: m.settings.enabledReason,
		Conn:          w.conn,
		Accounts:      w.accounts,
		Enabled:       m.settings.enabled,
		Connected:     w.conn == ConnConnected,
	})
}
