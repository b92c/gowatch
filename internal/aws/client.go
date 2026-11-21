package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
	creds       Credentials
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
		creds:       LoadCredentials(),
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

// HasLiveConfig reports whether an explicit endpoint or AWS credentials are present.
func (m *AWSClientManager) HasLiveConfig() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.endpointURL != "" || m.creds.AccessKeyID != ""
}

func (m *AWSClientManager) serviceEndpoint(service string) string {
	m.mu.RLock()
	ep := m.endpointURL
	region := m.region
	m.mu.RUnlock()
	if ep != "" {
		return strings.TrimRight(ep, "/")
	}
	return fmt.Sprintf("https://%s.%s.amazonaws.com", service, region)
}

// DoSignedRequest executes an AWS HTTP request signed with SigV4 when credentials are available.
func (m *AWSClientManager) DoSignedRequest(ctx context.Context, service, method, path string, headers map[string]string, body []byte) ([]byte, int, error) {
	url := m.serviceEndpoint(service) + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	m.mu.RLock()
	creds := m.creds
	region := m.region
	client := m.httpClient
	m.mu.RUnlock()

	if creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		SignRequest(req, body, creds, region, service, time.Now())
	}

	resp, err := client.Do(req) // #nosec G107 -- AWS SDK-style endpoint configured by user/environment
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return respBody, resp.StatusCode, fmt.Errorf("aws %s HTTP %d", service, resp.StatusCode)
	}
	return respBody, resp.StatusCode, nil
}

// CollectAll refreshes CloudWatch, Lambda, X-Ray, and CloudFormation resources
// when the AWS integration is enabled.
func (m *AWSClientManager) CollectAll(ctx context.Context) error {
	if m == nil || !m.GetState().Enabled {
		return nil
	}

	var (
		all  []AWSResource
		errs []error
	)

	if cwRes, err := NewCloudWatchCollector(m).ListLogGroups(ctx); err != nil {
		errs = append(errs, err)
	} else {
		all = append(all, cwRes...)
	}
	if lambdaRes, err := NewLambdaCollector(m).ListFunctions(ctx); err != nil {
		errs = append(errs, err)
	} else {
		all = append(all, lambdaRes...)
	}
	if xrayRes, err := NewXRayCollector(m).FetchServiceGraph(ctx); err != nil {
		errs = append(errs, err)
	} else {
		all = append(all, xrayRes...)
	}
	if cfRes, err := NewCloudFormationCollector(m).ListStacks(ctx); err != nil {
		errs = append(errs, err)
	} else {
		all = append(all, cfRes...)
	}

	if len(all) == 0 && len(errs) > 0 {
		joined := errors.Join(errs...)
		m.SetConnected(false, joined.Error())
		return joined
	}

	m.SetResources(all)
	return nil
}
