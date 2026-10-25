package docker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/aws"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/moby/moby/client"
)

var idCounter atomic.Int64

// uniqueID returns a container ID that no other test (or repeated run, with
// -count) has used: the collector keeps per-container state in package globals.
func uniqueID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(idCounter.Add(1), 10)
}

// saveSettings snapshots everything Configure can change and restores it when
// the test ends, so tests do not leak configuration into each other.
func saveSettings(t *testing.T) {
	t.Helper()
	settingsMutex.RLock()
	tail, history, manager := logTailLines, historyMaxPoints, globalAWSManager
	settingsMutex.RUnlock()

	t.Cleanup(func() {
		settingsMutex.Lock()
		logTailLines, historyMaxPoints, globalAWSManager = tail, history, manager
		settingsMutex.Unlock()
		globalAlertEngine.SetRules(alert.DefaultRules())
		// Alerts raised through the global engine would otherwise outlive the
		// test: with no containers left, Evaluate resolves them all.
		globalAlertEngine.Evaluate(nil, metrics.HostInfo{})
		globalAlertEngine.ClearResolvedAlerts()
	})
}

// isolateAWSEnv keeps the host's AWS setup out of a test: every AWS_* variable
// is blanked, HOME points to a new temporary directory and the two shared file
// variables name files in it that do not exist. It is the helper of the same
// name in internal/aws, repeated here because test helpers do not cross
// packages. Every test that builds an AWS manager, directly or through
// Configure, calls it first.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "AWS_") {
			t.Setenv(name, "")
		}
	}
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"AWS_ENDPOINT_URL", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_EC2_METADATA_DISABLED",
	} {
		t.Setenv(name, "")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))
}

// forgetContainer drops the per-container state the collector keeps in
// package globals, once the test is done with it.
func forgetContainer(t *testing.T, id string) {
	t.Helper()
	t.Cleanup(func() {
		statsMutex.Lock()
		defer statsMutex.Unlock()
		delete(historyStore, id)
		delete(previousStats, id)
	})
}

// fakeDaemon serves just enough of the Docker API for the collector helpers.
// logTails receives the "tail" query parameter of every logs request.
func fakeDaemon(t *testing.T, logTails chan<- string) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/logs"):
			if logTails != nil {
				logTails <- r.URL.Query().Get("tail")
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	apiClient, err := client.New(
		client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
		client.WithAPIVersion("1.52"),
	)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	t.Cleanup(func() { _ = apiClient.Close() })
	return apiClient
}

func TestBuiltInSettingsMatchLegacyValues(t *testing.T) {
	if defaultLogTailLines != 50 {
		t.Errorf("defaultLogTailLines = %d, want 50 (the former hard-coded tail)", defaultLogTailLines)
	}
	if metrics.MaxHistoryPoints != 60 {
		t.Errorf("metrics.MaxHistoryPoints = %d, want 60", metrics.MaxHistoryPoints)
	}
}

func TestConfigureSettings(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		wantTail    int
		wantHistory int
	}{
		{"zero options keep the defaults", Options{}, defaultLogTailLines, metrics.MaxHistoryPoints},
		{"both overridden", Options{LogTailLines: 10, HistoryMaxPoints: 20}, 10, 20},
		{"only the tail", Options{LogTailLines: 5}, 5, metrics.MaxHistoryPoints},
		{"only the history", Options{HistoryMaxPoints: 300}, defaultLogTailLines, 300},
		{"negative values are ignored", Options{LogTailLines: -1, HistoryMaxPoints: -1}, defaultLogTailLines, metrics.MaxHistoryPoints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveSettings(t)
			Configure(Options{LogTailLines: defaultLogTailLines, HistoryMaxPoints: metrics.MaxHistoryPoints})

			Configure(tt.opts)

			if got := currentLogTailLines(); got != tt.wantTail {
				t.Errorf("log tail = %d, want %d", got, tt.wantTail)
			}
			if got := currentHistoryMaxPoints(); got != tt.wantHistory {
				t.Errorf("history limit = %d, want %d", got, tt.wantHistory)
			}
		})
	}
}

func TestGetContainerStatsHonorsHistoryLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int // 0 keeps the built-in default
		calls int
		want  int
	}{
		{"default limit is 60", 0, 65, 60},
		{"below the default limit nothing is trimmed", 0, 12, 12},
		{"configured limit trims history", 10, 15, 10},
		{"configured limit above the default", 80, 85, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveSettings(t)
			Configure(Options{HistoryMaxPoints: metrics.MaxHistoryPoints})
			if tt.limit > 0 {
				Configure(Options{HistoryMaxPoints: tt.limit})
			}
			apiClient := fakeDaemon(t, nil)
			id := uniqueID("history-limit")
			forgetContainer(t, id)

			var got metrics.ContainerStats
			for range tt.calls {
				got = getContainerStats(context.Background(), apiClient, id)
			}

			if len(got.CPUHistory) != tt.want || len(got.MemHistory) != tt.want {
				t.Errorf("history lengths = cpu %d / mem %d, want %d", len(got.CPUHistory), len(got.MemHistory), tt.want)
			}
		})
	}
}

func TestGetContainerLogsTail(t *testing.T) {
	// A tail of 0 means "keep the built-in default".
	tests := []struct {
		name string
		want string
		tail int
	}{
		{name: "default tail is 50", want: "50"},
		{name: "configured tail", tail: 7, want: "7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveSettings(t)
			Configure(Options{LogTailLines: defaultLogTailLines})
			if tt.tail > 0 {
				Configure(Options{LogTailLines: tt.tail})
			}
			tails := make(chan string, 1)
			apiClient := fakeDaemon(t, tails)

			getContainerLogs(context.Background(), apiClient, uniqueID("tail-test"))

			select {
			case got := <-tails:
				if got != tt.want {
					t.Errorf("tail query = %q, want %q", got, tt.want)
				}
			default:
				t.Fatal("the daemon never received a logs request")
			}
		})
	}
}

func TestConfigureAlertRules(t *testing.T) {
	host := metrics.HostInfo{CPUCount: 1, MemTotal: 1 << 30}
	dead := func(id string) []alert.ContainerEvaluatorData {
		return []alert.ContainerEvaluatorData{{ID: id, Service: id, State: "dead"}}
	}
	firedFor := func(alerts []alert.Alert, id string) bool {
		for _, a := range alerts {
			if a.ContainerID == id {
				return true
			}
		}
		return false
	}
	engine := GetGlobalAlertEngine()

	t.Run("rules", func(t *testing.T) {
		saveSettings(t)
		Configure(Options{AlertRules: alert.DefaultRules()}) // do not depend on what ran before

		id := uniqueID("rules-default")
		if !firedFor(engine.Evaluate(dead(id), host), id) {
			t.Fatal("the built-in CONTAINER_DOWN rule should fire with the default rules")
		}

		Configure(Options{AlertRules: []alert.AlertRule{{
			ID: "ONLY_OOM", Metric: "oom_events", Severity: alert.SeverityCritical, ConsecutiveHits: 1,
		}}})
		id = uniqueID("rules-custom")
		if firedFor(engine.Evaluate(dead(id), host), id) {
			t.Error("after Configure with custom rules the built-in CONTAINER_DOWN rule must not fire")
		}

		Configure(Options{AlertRules: []alert.AlertRule{}})
		id = uniqueID("rules-empty")
		if firedFor(engine.Evaluate(dead(id), host), id) {
			t.Error("an empty, non-nil rule list must switch alerting off, not fall back to the defaults")
		}

		Configure(Options{})
		id = uniqueID("rules-kept")
		if firedFor(engine.Evaluate(dead(id), host), id) {
			t.Error("nil AlertRules must keep the current rules (still none here)")
		}

		Configure(Options{AlertRules: alert.DefaultRules()})
		id = uniqueID("rules-restored")
		if !firedFor(engine.Evaluate(dead(id), host), id) {
			t.Error("restoring the built-in rules should make CONTAINER_DOWN fire again")
		}
	})

	// The subtest's cleanup has run: the alerts it raised must be gone.
	if left := engine.GetActiveAlertsSnapshot(); len(left) != 0 {
		t.Errorf("alerts leaked into the global engine: %+v", left)
	}
}

func TestConfigureAWS(t *testing.T) {
	isolateAWSEnv(t)
	saveSettings(t)

	before := GetGlobalAWSManager()
	Configure(Options{})
	if GetGlobalAWSManager() != before {
		t.Error("zero aws.Options must keep the existing AWS manager")
	}
	Configure(Options{AWS: aws.Options{Accounts: []string{}}})
	if GetGlobalAWSManager() != before {
		t.Error("aws.Options with an empty account list must keep the existing AWS manager")
	}

	Configure(Options{AWS: aws.Options{Region: "ap-south-1", EndpointURL: "http://localhost:4566"}})
	mgr := GetGlobalAWSManager()
	if mgr == before {
		t.Fatal("non-zero aws.Options must replace the AWS manager")
	}
	if mgr.Region() != "ap-south-1" || mgr.EndpointURL() != "http://localhost:4566" || !mgr.GetState().Enabled {
		t.Errorf("manager = region %q endpoint %q state %+v", mgr.Region(), mgr.EndpointURL(), mgr.GetState())
	}

	// A profile alone is a non-zero option too: it replaces the manager.
	Configure(Options{AWS: aws.Options{Profile: "dev"}})
	profiled := GetGlobalAWSManager()
	if profiled == mgr {
		t.Fatal("aws.Options with only a profile must replace the AWS manager")
	}
	if got := profiled.GetState().Profile; got != "dev" {
		t.Errorf("manager profile = %q, want dev", got)
	}

	// So are the accounts alone.
	Configure(Options{AWS: aws.Options{Accounts: []string{"123456789012"}}})
	if GetGlobalAWSManager() == profiled {
		t.Fatal("aws.Options with only accounts must replace the AWS manager")
	}
}

// Configure is meant to run once before collection starts, but the settings it
// touches are read from the collector goroutine; run with -race to catch any
// unsynchronized access.
func TestConfigureIsSafeAlongsideCollection(t *testing.T) {
	isolateAWSEnv(t)
	saveSettings(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			Configure(Options{LogTailLines: 10 + i%5, HistoryMaxPoints: 20 + i%5, AWS: aws.Options{Region: "eu-west-1"}})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = currentLogTailLines()
			_ = currentHistoryMaxPoints()
			_ = GetGlobalAWSManager().GetState()
		}
	}()
	wg.Wait()
}

func TestTrimHistory(t *testing.T) {
	points := func(n int) []metrics.MetricPoint {
		out := make([]metrics.MetricPoint, n)
		for i := range out {
			out[i].Value = float64(i)
		}
		return out
	}
	tests := []struct {
		name      string
		in        int
		limit     int
		wantLen   int
		wantFirst float64
	}{
		{"under the limit", 5, 10, 5, 0},
		{"at the limit", 10, 10, 10, 0},
		{"one over drops the oldest", 11, 10, 10, 1},
		{"far over keeps the newest", 25, 10, 10, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimHistory(points(tt.in), tt.limit)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if got[0].Value != tt.wantFirst {
				t.Errorf("first value = %v, want %v", got[0].Value, tt.wantFirst)
			}
		})
	}
}

// main feeds the OTLP receiver with the global trace store, which is also the
// store WatchContainers reads the dashboard traces from.
func TestOTLPReceiverFeedsTheGlobalTraceStore(t *testing.T) {
	// The receiver does not report the port it bound, so pick a free one and
	// retry if someone else grabs it before Start does.
	var (
		receiver *trace.OTLPReceiver
		addr     string
	)
	for attempt := 0; attempt < 5; attempt++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr = l.Addr().String()
		_ = l.Close() // free the port for the receiver

		receiver = trace.NewOTLPReceiver(addr, GetGlobalTraceStore())
		if err = receiver.Start(); err == nil {
			break
		}
		receiver = nil
	}
	if receiver == nil {
		t.Fatal("could not start the OTLP receiver on a free port")
	}
	t.Cleanup(func() { _ = receiver.Stop(context.Background()) })

	// The trace store has no delete, so use an ID no other run can have used.
	traceID := uniqueID("otlpwiringtrace")
	payload := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"otlp-wiring"}}]},` +
		`"scopeSpans":[{"spans":[{"traceId":"` + traceID + `","spanId":"span1","name":"GET /ping",` +
		`"startTimeUnixNano":"1000000","endTimeUnixNano":"3000000","status":{"code":"OK"}}]}]}]}`
	resp, err := http.Post("http://"+addr+"/v1/traces", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /v1/traces: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	for _, tr := range GetGlobalTraceStore().GetTraces() {
		if tr.TraceID == traceID {
			return
		}
	}
	t.Error("the posted trace never reached the global trace store")
}
