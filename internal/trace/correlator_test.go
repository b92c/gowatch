package trace

import (
	"testing"
	"time"
)

func TestTraceStoreAddSpanAndEviction(t *testing.T) {
	ts := NewTraceStore()
	span1 := &Span{
		TraceID:       "trace-1",
		SpanID:        "span-1",
		ServiceName:   "api",
		OperationName: "GET /users",
		StartTime:     time.Now(),
		Duration:      100 * time.Millisecond,
		StatusCode:    "OK",
	}

	ts.AddSpan(span1)

	if ts.GetTraceCount() != 1 {
		t.Fatalf("expected trace count 1, got %d", ts.GetTraceCount())
	}

	traces := ts.GetTraces()
	if len(traces) != 1 || traces[0].TraceID != "trace-1" {
		t.Fatalf("expected trace-1 in traces")
	}

	if traces[0].RootSpan == nil || traces[0].RootSpan.OperationName != "GET /users" {
		t.Fatalf("expected root span GET /users")
	}
}

func TestCorrelatorProcessLogLine(t *testing.T) {
	ts := NewTraceStore()
	c := NewCorrelator(ts)

	line1 := "2026-04-21 12:00:00 [INFO] 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01 GET /api/v1/orders"
	span1 := c.ProcessLogLine("orders-service", line1)

	if span1 == nil {
		t.Fatalf("expected span extracted from W3C traceparent")
	}
	if span1.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || span1.SpanID != "00f067aa0ba902b7" {
		t.Fatalf("unexpected trace/span ID: %s / %s", span1.TraceID, span1.SpanID)
	}
	if span1.OperationName != "GET /api/v1/orders" {
		t.Fatalf("expected operation 'GET /api/v1/orders', got %q", span1.OperationName)
	}

	line2 := `{"level":"error","trace_id":"1234567890abcdef1234567890abcdef","span_id":"abcdef1234567890","msg":"POST /api/v1/checkout failed 500"}`
	span2 := c.ProcessLogLine("checkout-service", line2)

	if span2 == nil {
		t.Fatalf("expected span extracted from JSON trace_id")
	}
	if span2.StatusCode != "ERROR" {
		t.Fatalf("expected span status ERROR, got %s", span2.StatusCode)
	}

	if ts.GetTraceCount() != 2 {
		t.Fatalf("expected 2 active traces, got %d", ts.GetTraceCount())
	}
}

func TestCorrelatorDeduplicatesRepeatedLogLine(t *testing.T) {
	ts := NewTraceStore()
	c := NewCorrelator(ts)

	line := "2026-04-21 12:00:00 [INFO] 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01 GET /api/v1/orders"
	if span := c.ProcessLogLine("orders-service", line); span == nil {
		t.Fatalf("expected initial span to be extracted")
	}
	if span := c.ProcessLogLine("orders-service", line); span == nil {
		t.Fatalf("expected repeated call to still return parsed span")
	}

	traces := ts.GetTraces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if len(traces[0].Spans) != 1 {
		t.Fatalf("expected 1 span after deduplication, got %d", len(traces[0].Spans))
	}
}

func BenchmarkProcessLogLine(b *testing.B) {
	b.Run("Miss", func(b *testing.B) {
		c := NewCorrelator(NewTraceStore())
		line := "2026-04-21 12:00:00 [INFO] health check ok status=200 latency=2ms"
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = c.ProcessLogLine("api", line)
		}
	})

	b.Run("Hit", func(b *testing.B) {
		c := NewCorrelator(NewTraceStore())
		line := "2026-04-21 12:00:00 [INFO] 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01 GET /api/v1/orders"
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = c.ProcessLogLine("api", line)
		}
	})
}
