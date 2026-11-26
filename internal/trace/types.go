package trace

import (
	"hash/fnv"
	"sync"
	"time"
)

type Span struct {
	StartTime     time.Time         `json:"start_time"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	ServiceName   string            `json:"service_name"`
	OperationName string            `json:"operation_name"`
	StatusCode    string            `json:"status_code"`
	Duration      time.Duration     `json:"duration"`
}

type Trace struct {
	StartTime time.Time     `json:"start_time"`
	RootSpan  *Span         `json:"root_span,omitempty"`
	TraceID   string        `json:"trace_id"`
	Spans     []*Span       `json:"spans"`
	Duration  time.Duration `json:"duration"`
	HasError  bool          `json:"has_error"`
}

const MaxTraceStoreSize = 500
const TraceTTL = 5 * time.Minute

type TraceStore struct {
	traces     map[string]*Trace
	exporter   *Exporter
	order      []string
	sampleRate float64
	mu         sync.RWMutex
}

func NewTraceStore() *TraceStore {
	return &TraceStore{
		traces:     make(map[string]*Trace),
		order:      make([]string, 0, MaxTraceStoreSize),
		sampleRate: 1.0,
	}
}

// SetSampleRate configures deterministic trace sampling in [0.0, 1.0].
func (ts *TraceStore) SetSampleRate(rate float64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if rate < 0 {
		rate = 0
	} else if rate > 1 {
		rate = 1
	}
	ts.sampleRate = rate
}

// SetExporter attaches an optional trace exporter invoked when spans arrive.
func (ts *TraceStore) SetExporter(exp *Exporter) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.exporter = exp
}

func shouldSampleTrace(traceID string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(traceID))
	ratio := float64(h.Sum32()) / float64(^uint32(0))
	return ratio <= rate
}

func (ts *TraceStore) AddSpan(span *Span) {
	if span == nil || span.TraceID == "" {
		return
	}

	ts.mu.Lock()
	if !shouldSampleTrace(span.TraceID, ts.sampleRate) {
		ts.mu.Unlock()
		return
	}

	now := time.Now()

	var freshOrder []string
	for _, id := range ts.order {
		t, ok := ts.traces[id]
		if ok && now.Sub(t.StartTime) <= TraceTTL {
			freshOrder = append(freshOrder, id)
		} else {
			delete(ts.traces, id)
		}
	}
	ts.order = freshOrder

	tr, exists := ts.traces[span.TraceID]
	if !exists {
		if len(ts.order) >= MaxTraceStoreSize {
			oldest := ts.order[0]
			ts.order = ts.order[1:]
			delete(ts.traces, oldest)
		}
		tr = &Trace{
			TraceID:   span.TraceID,
			Spans:     make([]*Span, 0, 4),
			StartTime: span.StartTime,
		}
		if tr.StartTime.IsZero() {
			tr.StartTime = now
		}
		ts.traces[span.TraceID] = tr
		ts.order = append(ts.order, span.TraceID)
	} else if rawLine := span.Attributes["raw_line"]; rawLine != "" {
		for _, existing := range tr.Spans {
			if existing.ServiceName == span.ServiceName &&
				existing.SpanID == span.SpanID &&
				existing.Attributes["raw_line"] == rawLine {
				ts.mu.Unlock()
				return
			}
		}
	}

	tr.Spans = append(tr.Spans, span)
	if span.ParentSpanID == "" || tr.RootSpan == nil {
		tr.RootSpan = span
	}
	if span.StatusCode == "ERROR" {
		tr.HasError = true
	}

	if !span.StartTime.IsZero() {
		if span.StartTime.Before(tr.StartTime) {
			tr.StartTime = span.StartTime
		}
		endTime := span.StartTime.Add(span.Duration)
		if endTime.After(tr.StartTime.Add(tr.Duration)) {
			tr.Duration = endTime.Sub(tr.StartTime)
		}
	}

	exp := ts.exporter
	var trCopy Trace
	if exp != nil {
		trCopy = *tr
		trCopy.Spans = append([]*Span(nil), tr.Spans...)
	}
	ts.mu.Unlock()

	if exp != nil {
		exp.EnqueueTrace(trCopy)
	}
}

func (ts *TraceStore) GetTraces() []Trace {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]Trace, 0, len(ts.order))
	for _, id := range ts.order {
		if tr, ok := ts.traces[id]; ok {
			tCopy := *tr
			result = append(result, tCopy)
		}
	}
	return result
}

func (ts *TraceStore) GetTraceCount() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.traces)
}
