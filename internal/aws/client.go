package aws

import (
	"net/http"
	"os"
	"sync"
	"time"
)

type AWSResource struct {
	Metrics     map[string]float64 `json:"metrics,omitempty"`
	LastUpdated time.Time          `json:"last_updated"`
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	Name        string             `json:"name"`
	Region      string             `json:"region"`
	Status      string             `json:"status"`
}

type AWSState struct {
	Region      string        `json:"region"`
	EndpointURL string        `json:"endpoint_url,omitempty"`
	LastError   string        `json:"last_error,omitempty"`
	Resources   []AWSResource `json:"resources"`
	Enabled     bool          `json:"enabled"`
	Connected   bool          `json:"connected"`
}

type AWSClientManager struct {
	httpClient  *http.Client
	region      string
	endpointURL string
	state       AWSState
	mu          sync.RWMutex
}

// Options carries the AWS settings that come from the config file. The zero
// value reproduces the environment-only behavior.
type Options struct {
	// Enabled forces the integration on or off. Nil means auto: it is enabled
	// when the environment or the other options configure AWS.
	Enabled     *bool
	Region      string
	EndpointURL string
}

// NewAWSClientManager builds a manager from the AWS_* environment variables only.
func NewAWSClientManager() *AWSClientManager {
	return NewAWSClientManagerWithOptions(Options{})
}

// NewAWSClientManagerWithOptions builds a manager from the environment and o.
// AWS_* environment variables take priority over the corresponding options:
// region is AWS_REGION, then AWS_DEFAULT_REGION, then o.Region, then
// us-east-1; the endpoint is AWS_ENDPOINT_URL, then o.EndpointURL. An explicit
// o.Enabled always wins; otherwise the integration is enabled when
// credentials, a profile, a region or an endpoint are configured.
func NewAWSClientManagerWithOptions(o Options) *AWSClientManager {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = o.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	endpointURL := os.Getenv("AWS_ENDPOINT_URL")
	if endpointURL == "" {
		endpointURL = o.EndpointURL
	}

	enabled := os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" || endpointURL != "" || os.Getenv("AWS_REGION") != "" || o.Region != ""
	if o.Enabled != nil {
		enabled = *o.Enabled
	}

	return &AWSClientManager{
		region:      region,
		endpointURL: endpointURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		state: AWSState{
			Enabled:     enabled,
			Region:      region,
			EndpointURL: endpointURL,
			Resources:   make([]AWSResource, 0),
		},
	}
}

func (m *AWSClientManager) GetState() AWSState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resCopy := make([]AWSResource, len(m.state.Resources))
	copy(resCopy, m.state.Resources)
	st := m.state
	st.Resources = resCopy
	return st
}

func (m *AWSClientManager) SetConnected(connected bool, errStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Connected = connected
	m.state.LastError = errStr
}

func (m *AWSClientManager) SetResources(resources []AWSResource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Resources = resources
	m.state.Connected = true
	m.state.LastError = ""
}

func (m *AWSClientManager) Region() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.region
}

func (m *AWSClientManager) EndpointURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.endpointURL
}
