package trace

import (
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
	traces map[string]*Trace
	order  []string
	mu     sync.RWMutex
}

func NewTraceStore() *TraceStore {
	return &TraceStore{
		traces: make(map[string]*Trace),
		order:  make([]string, 0, MaxTraceStoreSize),
	}
}

func (ts *TraceStore) AddSpan(span *Span) {
	if span == nil || span.TraceID == "" {
		return
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

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
