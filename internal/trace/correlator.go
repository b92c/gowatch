package trace

import (
	"regexp"
	"strings"
	"time"
)

var (
	traceparentRegex = regexp.MustCompile(`00-([a-fA-F0-9]{32})-([a-fA-F0-9]{16})-[a-fA-F0-9]{2}`)
	jsonTraceRegex   = regexp.MustCompile(`"(?:trace_id|traceId)"\s*:\s*"([a-fA-F0-9]{16,32})"`)
	jsonSpanRegex    = regexp.MustCompile(`"(?:span_id|spanId)"\s*:\s*"([a-fA-F0-9]{16})"`)
	logfmtTraceRegex = regexp.MustCompile(`(?:trace_id|traceId)=([a-fA-F0-9]{16,32})`)
	logfmtSpanRegex  = regexp.MustCompile(`(?:span_id|spanId)=([a-fA-F0-9]{16})`)
	reqIDRegex       = regexp.MustCompile(`(?:request_id|requestId|x-request-id)=([a-zA-Z0-9_-]+)`)
)

type Correlator struct {
	store *TraceStore
}

func NewCorrelator(store *TraceStore) *Correlator {
	if store == nil {
		store = NewTraceStore()
	}
	return &Correlator{store: store}
}

func (c *Correlator) Store() *TraceStore {
	return c.store
}

func (c *Correlator) ProcessLogLine(service string, line string) *Span {
	if line == "" || !hasTraceHint(line) {
		return nil
	}

	var traceID, spanID string

	if matches := traceparentRegex.FindStringSubmatch(line); len(matches) > 2 {
		traceID = matches[1]
		spanID = matches[2]
	} else if matches := jsonTraceRegex.FindStringSubmatch(line); len(matches) > 1 {
		traceID = matches[1]
		if spanMatches := jsonSpanRegex.FindStringSubmatch(line); len(spanMatches) > 1 {
			spanID = spanMatches[1]
		}
	} else if matches := logfmtTraceRegex.FindStringSubmatch(line); len(matches) > 1 {
		traceID = matches[1]
		if spanMatches := logfmtSpanRegex.FindStringSubmatch(line); len(spanMatches) > 1 {
			spanID = spanMatches[1]
		}
	} else if matches := reqIDRegex.FindStringSubmatch(line); len(matches) > 1 {
		traceID = matches[1]
	}

	if traceID == "" {
		return nil
	}

	statusCode := "OK"
	upper := strings.ToUpper(line)
	if strings.Contains(upper, "ERROR") || strings.Contains(upper, "FATAL") || strings.Contains(upper, "500") || strings.Contains(upper, "502") || strings.Contains(upper, "503") {
		statusCode = "ERROR"
	}

	span := &Span{
		TraceID:       traceID,
		SpanID:        spanID,
		ServiceName:   service,
		OperationName: extractOperationName(line),
		StartTime:     time.Now(),
		StatusCode:    statusCode,
		Attributes:    map[string]string{"raw_line": line},
	}

	c.store.AddSpan(span)
	return span
}

func extractOperationName(line string) string {
	upper := strings.ToUpper(line)
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	for _, m := range methods {
		if idx := strings.Index(upper, m+" "); idx != -1 {
			parts := strings.Fields(line[idx:])
			if len(parts) >= 2 {
				return parts[0] + " " + parts[1]
			}
		}
	}
	return "log_event"
}

func hasTraceHint(line string) bool {
	return strings.Contains(line, "00-") ||
		containsASCIIFold(line, "trace") ||
		containsASCIIFold(line, "span") ||
		containsASCIIFold(line, "request")
}

func containsASCIIFold(s, lowerSubstr string) bool {
	n := len(lowerSubstr)
	if len(s) < n {
		return false
	}
	for i := 0; i <= len(s)-n; i++ {
		match := true
		for j := 0; j < n; j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lowerSubstr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
