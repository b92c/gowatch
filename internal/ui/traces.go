package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/rivo/tview"
)

// renderTracesText builds the ASCII flamegraph/waterfall and correlated log view
// for active traces.
func renderTracesText(t Theme, traces []trace.Trace, logs []docker.FormattedLog) string {
	accent, muted, text := t.Tag(t.Accent), t.Tag(t.Muted), t.Tag(t.Text)
	okTag, errTag := t.Tag(t.StatusOK), t.Tag(t.StatusError)

	if len(traces) == 0 {
		return "\n  " + muted + "No active traces recorded yet. Enable tracing.otlp_receiver or emit traceparent/trace_id in logs.[-]\n"
	}

	var b strings.Builder
	for i := len(traces) - 1; i >= 0; i-- {
		tr := traces[i]
		statusTag := okTag + "OK[-]"
		if tr.HasError {
			statusTag = errTag + "ERR[-]"
		}
		dur := tr.Duration.Truncate(time.Millisecond)
		fmt.Fprintf(&b, "\n %sTrace %s[-] (%d spans, %v) %s\n",
			accent, tview.Escape(tr.TraceID), len(tr.Spans), dur, statusTag)

		renderSpanTree(&b, t, tr)

		var correlated []docker.FormattedLog
		for _, fl := range logs {
			if fl.TraceID == tr.TraceID || strings.Contains(fl.Line, tr.TraceID) {
				correlated = append(correlated, fl)
			}
		}
		if len(correlated) > 0 {
			fmt.Fprintf(&b, "   %sCorrelated Logs:%s\n", muted, text)
			limit := min(len(correlated), 5)
			for _, fl := range correlated[:limit] {
				fmt.Fprintf(&b, "     %s[%s]%s %s\n",
					accent, tview.Escape(fl.Service), text, tview.Escape(fl.Line))
			}
		}
	}
	return b.String()
}

func renderSpanTree(b *strings.Builder, t Theme, tr trace.Trace) {
	if len(tr.Spans) == 0 {
		return
	}
	accent, muted, text := t.Tag(t.Accent), t.Tag(t.Muted), t.Tag(t.Text)
	errTag, okTag := t.Tag(t.StatusError), t.Tag(t.StatusOK)

	children := make(map[string][]*trace.Span)
	byID := make(map[string]*trace.Span, len(tr.Spans))
	for _, s := range tr.Spans {
		if s.SpanID != "" {
			byID[s.SpanID] = s
		}
	}
	var roots []*trace.Span
	for _, s := range tr.Spans {
		if s.ParentSpanID == "" || byID[s.ParentSpanID] == nil {
			roots = append(roots, s)
		} else {
			children[s.ParentSpanID] = append(children[s.ParentSpanID], s)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].StartTime.Before(roots[j].StartTime) })

	totalDur := tr.Duration
	if totalDur <= 0 {
		totalDur = time.Millisecond
	}

	const barWidth = 20
	var walk func(s *trace.Span, depth int)
	walk = func(s *trace.Span, depth int) {
		indent := strings.Repeat("  ", depth)
		ratio := float64(s.Duration) / float64(totalDur)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		filled := max(int(ratio*barWidth), 1)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		barTag := okTag
		if s.StatusCode == "ERROR" {
			barTag = errTag
		}
		fmt.Fprintf(b, "   %s%s|%s%s|[-] %s%s[-] %s%s[-] %s(%v)[-]\n",
			indent, barTag, bar, text,
			accent, tview.Escape(s.ServiceName),
			text, tview.Escape(s.OperationName),
			muted, s.Duration.Truncate(time.Millisecond))

		kids := children[s.SpanID]
		sort.Slice(kids, func(i, j int) bool { return kids[i].StartTime.Before(kids[j].StartTime) })
		for _, k := range kids {
			walk(k, depth+1)
		}
	}

	for _, r := range roots {
		walk(r, 0)
	}
}
