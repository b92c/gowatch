package trace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOTLPReceiverAndExporter(t *testing.T) {
	ts := NewTraceStore()
	receiver := NewOTLPReceiver("127.0.0.1:43189", ts)

	if err := receiver.Start(); err != nil {
		t.Fatalf("failed to start OTLP receiver: %v", err)
	}
	defer func() {
		_ = receiver.Stop(context.Background())
	}()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	exporter := NewExporter(testServer.URL)
	tr := Trace{
		TraceID: "test-trace-id",
		Spans: []*Span{
			{TraceID: "test-trace-id", SpanID: "span-1", ServiceName: "test-service"},
		},
	}

	err := exporter.ExportTrace(context.Background(), tr)
	if err != nil {
		t.Fatalf("expected export to succeed, got %v", err)
	}
}

func TestExporterFormatsAndSampling(t *testing.T) {
	var payloads []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		payloads = append(payloads, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	root := &Span{
		TraceID:       "0123456789abcdef0123456789abcdef",
		SpanID:        "0123456789abcdef",
		ServiceName:   "api",
		OperationName: "GET /health",
		StartTime:     time.Now(),
		Duration:      25 * time.Millisecond,
		StatusCode:    "OK",
	}
	tr := Trace{TraceID: root.TraceID, RootSpan: root, Spans: []*Span{root}}

	if err := NewExporterWithFormat(srv.URL, "otlp").ExportTrace(context.Background(), tr); err != nil {
		t.Fatalf("otlp export: %v", err)
	}
	if err := NewExporterWithFormat(srv.URL, "jaeger").ExportTrace(context.Background(), tr); err != nil {
		t.Fatalf("jaeger export: %v", err)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if _, ok := payloads[0]["resourceSpans"]; !ok {
		t.Errorf("otlp payload missing resourceSpans: %+v", payloads[0])
	}
	if _, ok := payloads[1]["spans"]; !ok {
		t.Errorf("jaeger payload missing spans: %+v", payloads[1])
	}

	ts := NewTraceStore()
	ts.SetSampleRate(0)
	ts.AddSpan(root)
	if got := ts.GetTraceCount(); got != 0 {
		t.Errorf("SampleRate=0 should drop spans, got %d traces", got)
	}

	ts.SetSampleRate(1.0)
	ts.AddSpan(root)
	if got := ts.GetTraceCount(); got != 1 {
		t.Errorf("SampleRate=1 should keep spans, got %d traces", got)
	}
}
