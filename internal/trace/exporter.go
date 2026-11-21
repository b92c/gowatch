package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Exporter struct {
	client    *http.Client
	targetURL string
	format    string
}

func NewExporter(targetURL string) *Exporter {
	return NewExporterWithFormat(targetURL, "gowatch")
}

// NewExporterWithFormat creates an Exporter for "otlp", "jaeger", or "gowatch".
func NewExporterWithFormat(targetURL, format string) *Exporter {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" {
		f = "gowatch"
	}
	return &Exporter{
		targetURL: targetURL,
		format:    f,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// EnqueueTrace exports tr asynchronously in the background.
func (e *Exporter) EnqueueTrace(tr Trace) {
	if e == nil || e.targetURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.ExportTrace(ctx, tr)
	}()
}

func (e *Exporter) ExportTrace(ctx context.Context, tr Trace) error {
	if e == nil || e.targetURL == "" {
		return nil
	}

	payload, err := e.encodeTrace(tr)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.targetURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req) // #nosec G107 -- targetURL is configured in trusted config
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("exporter HTTP error: %d", resp.StatusCode)
	}

	return nil
}

func (e *Exporter) encodeTrace(tr Trace) ([]byte, error) {
	switch e.format {
	case "otlp":
		svc := "gowatch"
		if tr.RootSpan != nil && tr.RootSpan.ServiceName != "" {
			svc = tr.RootSpan.ServiceName
		}
		spans := make([]map[string]any, 0, len(tr.Spans))
		for _, s := range tr.Spans {
			startNs := s.StartTime.UnixNano()
			endNs := s.StartTime.Add(s.Duration).UnixNano()
			spans = append(spans, map[string]any{
				"traceId":           s.TraceID,
				"spanId":            s.SpanID,
				"parentSpanId":      s.ParentSpanID,
				"name":              s.OperationName,
				"startTimeUnixNano": strconv.FormatInt(startNs, 10),
				"endTimeUnixNano":   strconv.FormatInt(endNs, 10),
				"status":            map[string]string{"code": s.StatusCode},
			})
		}
		return json.Marshal(map[string]any{
			"resourceSpans": []map[string]any{{
				"resource": map[string]any{
					"attributes": []map[string]any{{
						"key":   "service.name",
						"value": map[string]string{"stringValue": svc},
					}},
				},
				"scopeSpans": []map[string]any{{
					"spans": spans,
				}},
			}},
		})
	case "jaeger":
		svc := "gowatch"
		if tr.RootSpan != nil && tr.RootSpan.ServiceName != "" {
			svc = tr.RootSpan.ServiceName
		}
		spans := make([]map[string]any, 0, len(tr.Spans))
		for _, s := range tr.Spans {
			spans = append(spans, map[string]any{
				"traceID":       s.TraceID,
				"spanID":        s.SpanID,
				"parentSpanID":  s.ParentSpanID,
				"operationName": s.OperationName,
				"startTime":     s.StartTime.UnixMicro(),
				"duration":      s.Duration.Microseconds(),
			})
		}
		return json.Marshal(map[string]any{
			"process": map[string]any{"serviceName": svc},
			"spans":   spans,
		})
	default:
		return json.Marshal(tr)
	}
}

type OTLPReceiver struct {
	server   *http.Server
	store    *TraceStore
	listener net.Listener
	addr     string
	mu       sync.Mutex
	running  bool
}

func NewOTLPReceiver(addr string, store *TraceStore) *OTLPReceiver {
	if addr == "" {
		addr = "127.0.0.1:4318"
	}
	if store == nil {
		store = NewTraceStore()
	}
	return &OTLPReceiver{
		addr:  addr,
		store: store,
	}
}

func (r *OTLPReceiver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return nil
	}

	l, err := net.Listen("tcp", r.addr)
	if err != nil {
		return fmt.Errorf("failed to bind OTLP receiver to %s: %w", r.addr, err)
	}
	r.listener = l

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)

	r.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	r.running = true
	go func() {
		_ = r.server.Serve(l)
	}()

	return nil
}

func (r *OTLPReceiver) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running || r.server == nil {
		return nil
	}

	r.running = false
	return r.server.Shutdown(ctx)
}

func (r *OTLPReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []struct {
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
					Key string `json:"key"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []struct {
					Status struct {
						Code string `json:"code"`
					} `json:"status"`
					TraceID           string `json:"traceId"`
					SpanID            string `json:"spanId"`
					ParentSpanID      string `json:"parentSpanId"`
					Name              string `json:"name"`
					StartTimeUnixNano int64  `json:"startTimeUnixNano,string"`
					EndTimeUnixNano   int64  `json:"endTimeUnixNano,string"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	for _, rs := range payload.ResourceSpans {
		serviceName := "unknown"
		for _, attr := range rs.Resource.Attributes {
			if attr.Key == "service.name" {
				serviceName = attr.Value.StringValue
				break
			}
		}

		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				startTime := time.Unix(0, s.StartTimeUnixNano)
				endTime := time.Unix(0, s.EndTimeUnixNano)
				duration := endTime.Sub(startTime)
				if duration < 0 {
					duration = 0
				}

				span := &Span{
					TraceID:       s.TraceID,
					SpanID:        s.SpanID,
					ParentSpanID:  s.ParentSpanID,
					ServiceName:   serviceName,
					OperationName: s.Name,
					StartTime:     startTime,
					Duration:      duration,
					StatusCode:    s.Status.Code,
				}
				r.store.AddSpan(span)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}
