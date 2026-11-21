package filter

import (
	"path/filepath"
	"testing"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
)

func TestFilterMinLogLevel(t *testing.T) {
	fs := NewFilterState()
	if fs.Active {
		t.Errorf("expected initial FilterState to be inactive")
	}

	fs.SetMinLogLevel(metrics.LogLevelWarn)
	if !fs.Active {
		t.Errorf("expected FilterState to be active after SetMinLogLevel")
	}
	if fs.MinLogLevel != metrics.LogLevelWarn {
		t.Errorf("expected MinLogLevel = LogLevelWarn, got %v", fs.MinLogLevel)
	}

	fs.CycleMinLogLevel()
	if fs.MinLogLevel != metrics.LogLevelError {
		t.Errorf("expected MinLogLevel = LogLevelError after cycle, got %v", fs.MinLogLevel)
	}

	fs.CycleMinLogLevel()
	if fs.MinLogLevel != metrics.LogLevelUnknown {
		t.Errorf("expected MinLogLevel = LogLevelUnknown after cycle, got %v", fs.MinLogLevel)
	}

	fs.CycleMinLogLevel()
	if fs.MinLogLevel != metrics.LogLevelInfo {
		t.Errorf("expected MinLogLevel = LogLevelInfo after cycle, got %v", fs.MinLogLevel)
	}
}

func TestFilterContainersLogLevel(t *testing.T) {
	cnts := docker.Containers{
		C: []docker.Container{
			{
				ID:      "123456789012",
				Service: "web",
				Log: []string{
					"[INFO] App started",
					"[WARN] High latency",
					"[ERROR] Database error",
					"[DEBUG] Processing request",
				},
			},
		},
	}

	fs := NewFilterState()
	fs.SetMinLogLevel(metrics.LogLevelWarn)

	filtered := FilterContainers(cnts, fs)

	if len(filtered.C) != 1 {
		t.Fatalf("expected 1 container, got %d", len(filtered.C))
	}

	if len(filtered.FlatLogs) != 2 {
		t.Fatalf("expected 2 flat logs (WARN and ERROR), got %d", len(filtered.FlatLogs))
	}

	if filtered.FlatLogs[0].Level != metrics.LogLevelWarn {
		t.Errorf("expected first log to be WARN, got %v", filtered.FlatLogs[0].Level)
	}
	if filtered.FlatLogs[1].Level != metrics.LogLevelError {
		t.Errorf("expected second log to be ERROR, got %v", filtered.FlatLogs[1].Level)
	}
}

func searchFixture() docker.Containers {
	return docker.Containers{
		C: []docker.Container{
			{
				ID:      "aaaaaaaaaaaa1111",
				Service: "api",
				Image:   "acme/api:1",
				State:   "running",
				Log:     []string{"[INFO] api started", "[ERROR] api crashed"},
			},
			{
				ID:      "bbbbbbbbbbbb2222",
				Service: "worker",
				Image:   "acme/worker:1",
				State:   "running",
				Log:     []string{"[INFO] job done", "[WARN] Slow call to API gateway", "[ERROR] queue broken"},
			},
			{
				ID:      "cccccccccccc3333",
				Service: "db",
				Image:   "postgres:16",
				State:   "exited",
				Log:     []string{"[INFO] ready", "[ERROR] Failed to reach api"},
			},
		},
	}
}

func lineTexts(logs []docker.FormattedLog) []string {
	out := make([]string, len(logs))
	for i, l := range logs {
		out[i] = l.Service + ": " + l.Line
	}
	return out
}

func serviceNames(cs []docker.Container) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Service
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilterContainersSearchInLogText(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*FilterState)
		wantList  []string
		wantLines []string
	}{
		{
			name:      "name match keeps every line and a text match adds lines of the others",
			setup:     func(f *FilterState) { f.SetSearch("api") },
			wantList:  []string{"api"},
			wantLines: []string{"api: [INFO] api started", "api: [ERROR] api crashed", "worker: [WARN] Slow call to API gateway", "db: [ERROR] Failed to reach api"},
		},
		{
			name:      "text-only match brings just the matching lines and no table row",
			setup:     func(f *FilterState) { f.SetSearch("broken") },
			wantList:  nil,
			wantLines: []string{"worker: [ERROR] queue broken"},
		},
		{
			name:      "match is case-insensitive and trimmed",
			setup:     func(f *FilterState) { f.SetSearch("  SLOW CALL ") },
			wantList:  nil,
			wantLines: []string{"worker: [WARN] Slow call to API gateway"},
		},
		{
			name:      "image match lists the container with all its lines",
			setup:     func(f *FilterState) { f.SetSearch("POSTGRES") },
			wantList:  []string{"db"},
			wantLines: []string{"db: [INFO] ready", "db: [ERROR] Failed to reach api"},
		},
		{
			name: "min log level applies to name matches and text matches",
			setup: func(f *FilterState) {
				f.SetSearch("api")
				f.SetMinLogLevel(metrics.LogLevelError)
			},
			wantList:  []string{"api"},
			wantLines: []string{"api: [ERROR] api crashed", "db: [ERROR] Failed to reach api"},
		},
		{
			name: "min log level can drop a text match",
			setup: func(f *FilterState) {
				f.SetSearch("slow call")
				f.SetMinLogLevel(metrics.LogLevelError)
			},
			wantList:  nil,
			wantLines: nil,
		},
		{
			name: "status filter excludes the text matches of other containers",
			setup: func(f *FilterState) {
				f.SetSearch("api")
				f.SetStatusFilter([]string{"running"})
			},
			wantList:  []string{"api"},
			wantLines: []string{"api: [INFO] api started", "api: [ERROR] api crashed", "worker: [WARN] Slow call to API gateway"},
		},
		{
			name: "status filter alone still lists by status",
			setup: func(f *FilterState) {
				f.SetStatusFilter([]string{"exited"})
			},
			wantList:  []string{"db"},
			wantLines: []string{"db: [INFO] ready", "db: [ERROR] Failed to reach api"},
		},
		{
			name: "label filter applies to the text matches",
			setup: func(f *FilterState) {
				f.SetSearch("api")
				f.SetLabelFilter("com.docker.compose.service", "worker")
			},
			wantList:  nil,
			wantLines: []string{"worker: [WARN] Slow call to API gateway"},
		},
		{
			name:      "no match at all",
			setup:     func(f *FilterState) { f.SetSearch("nothing-like-this") },
			wantList:  nil,
			wantLines: nil,
		},
		{
			name:      "empty search lists everything",
			setup:     func(f *FilterState) { f.SetMinLogLevel(metrics.LogLevelWarn) },
			wantList:  []string{"api", "worker", "db"},
			wantLines: []string{"api: [ERROR] api crashed", "worker: [WARN] Slow call to API gateway", "worker: [ERROR] queue broken", "db: [ERROR] Failed to reach api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewFilterState()
			tt.setup(&fs)
			got := FilterContainers(searchFixture(), fs)
			if list := serviceNames(got.C); !equalStrings(list, tt.wantList) {
				t.Errorf("listed containers = %v, want %v", list, tt.wantList)
			}
			if lines := lineTexts(got.FlatLogs); !equalStrings(lines, tt.wantLines) {
				t.Errorf("log lines = %v, want %v", lines, tt.wantLines)
			}
		})
	}
}

func TestFilterContainersEmptySearchIsUnchanged(t *testing.T) {
	fs := NewFilterState()
	in := searchFixture()
	if got := FilterContainers(in, fs); len(got.C) != len(in.C) {
		t.Errorf("inactive filter changed the containers: %d, want %d", len(got.C), len(in.C))
	}

	fs.SetSearch("   ")
	if fs.Active {
		t.Error("a blank search must not activate the filter")
	}
}

func TestFilterContainersLabelsAndQualifiers(t *testing.T) {
	cnts := docker.Containers{
		C: []docker.Container{
			{
				ID:      "111111111111",
				Service: "api",
				State:   "running",
				Labels:  map[string]string{"env": "prod", "team": "core"},
				Log:     []string{"[INFO] ready"},
			},
			{
				ID:      "222222222222",
				Service: "worker",
				State:   "exited",
				Labels:  map[string]string{"env": "staging"},
				Log:     []string{"[ERROR] failed"},
			},
		},
	}

	fs := NewFilterState()
	fs.SetLabelFilters(map[string]string{"env": "prod"})
	got := FilterContainers(cnts, fs)
	if list := serviceNames(got.C); !equalStrings(list, []string{"api"}) {
		t.Errorf("SetLabelFilters(env=prod) = %v, want [api]", list)
	}

	fs = NewFilterState()
	fs.SetSearch("status:exited label:env=staging")
	got = FilterContainers(cnts, fs)
	if list := serviceNames(got.C); !equalStrings(list, []string{"worker"}) {
		t.Errorf("inline qualifiers = %v, want [worker]", list)
	}

	fs = NewFilterState()
	fs.SetSearch("label:team")
	got = FilterContainers(cnts, fs)
	if list := serviceNames(got.C); !equalStrings(list, []string{"api"}) {
		t.Errorf("label presence qualifier = %v, want [api]", list)
	}
}

func TestCycleStatusFilter(t *testing.T) {
	fs := NewFilterState()
	want := [][]string{
		{"running"},
		{"restarting"},
		{"exited"},
		{"paused"},
		nil,
	}
	for i, w := range want {
		fs.CycleStatusFilter()
		if !equalStrings(fs.StatusFilter, w) {
			t.Errorf("step %d: StatusFilter = %v, want %v", i, fs.StatusFilter, w)
		}
	}
}

func TestSaveAndLoadFilterState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "filter_state.json")

	loadedMissing, err := LoadFilterState(path)
	if err != nil {
		t.Fatalf("LoadFilterState missing: %v", err)
	}
	if loadedMissing.Active {
		t.Errorf("missing file should return inactive FilterState")
	}

	want := NewFilterState()
	want.SetSearch("api")
	want.SetStatusFilter([]string{"running"})
	want.SetLabelFilters(map[string]string{"env": "prod"})
	want.SetMinLogLevel(metrics.LogLevelWarn)

	if err := SaveFilterState(path, want); err != nil {
		t.Fatalf("SaveFilterState: %v", err)
	}

	got, err := LoadFilterState(path)
	if err != nil {
		t.Fatalf("LoadFilterState: %v", err)
	}
	if got.SearchText != want.SearchText || !equalStrings(got.StatusFilter, want.StatusFilter) ||
		got.LabelFilters["env"] != "prod" || got.MinLogLevel != want.MinLogLevel || !got.Active {
		t.Errorf("loaded state = %+v, want %+v", got, want)
	}
}
