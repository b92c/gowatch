package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func BenchmarkWatchContainers100(b *testing.B) {
	const count = 100
	summaries := make([]container.Summary, count)
	for i := range count {
		summaries[i] = container.Summary{
			ID:     fmt.Sprintf("bench-cnt-%03d", i),
			Image:  "alpine:latest",
			State:  "running",
			Status: "Up",
			Labels: map[string]string{
				"com.docker.compose.service": fmt.Sprintf("svc-%03d", i),
			},
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(summaries)
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"NCPU": 8, "MemTotal": 16 << 30})
		case strings.HasSuffix(r.URL.Path, "/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"memory_stats":{"usage":1048576}}`))
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	apiClient, err := client.New(
		client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")),
		client.WithScheme("http"),
		client.WithAPIVersion("1.52"),
		client.WithHTTPClient(NewPooledHTTPClient(16)),
	)
	if err != nil {
		b.Fatalf("client.New() error = %v", err)
	}
	defer func() { _ = apiClient.Close() }()

	Configure(Options{MaxConcurrency: 16})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := WatchContainers(ctx, apiClient); err != nil {
			b.Fatalf("WatchContainers() error = %v", err)
		}
	}
}
