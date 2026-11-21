package filter

import (
	"maps"
	"strings"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
)

type FilterState struct {
	LabelFilters map[string]string
	SearchText   string
	StatusFilter []string
	MinLogLevel  metrics.LogLevel
	Active       bool
}

func NewFilterState() FilterState {
	return FilterState{
		LabelFilters: make(map[string]string),
		MinLogLevel:  metrics.LogLevelUnknown,
		Active:       false,
	}
}

func (f *FilterState) updateActive() {
	f.Active = f.SearchText != "" || len(f.StatusFilter) > 0 || len(f.LabelFilters) > 0 || f.MinLogLevel > metrics.LogLevelUnknown
}

func (f *FilterState) SetSearch(text string) {
	f.SearchText = strings.TrimSpace(text)
	f.updateActive()
}

func (f *FilterState) SetStatusFilter(status []string) {
	f.StatusFilter = status
	f.updateActive()
}

func (f *FilterState) SetLabelFilter(key, value string) {
	if f.LabelFilters == nil {
		f.LabelFilters = make(map[string]string)
	}
	if value == "" {
		delete(f.LabelFilters, key)
	} else {
		f.LabelFilters[key] = value
	}
	f.updateActive()
}

// SetLabelFilters replaces all active label filters.
func (f *FilterState) SetLabelFilters(labels map[string]string) {
	if len(labels) == 0 {
		f.LabelFilters = make(map[string]string)
	} else {
		f.LabelFilters = maps.Clone(labels)
	}
	f.updateActive()
}

// CycleStatusFilter cycles through common container states and clears back to all.
func (f *FilterState) CycleStatusFilter() {
	if len(f.StatusFilter) == 0 {
		f.StatusFilter = []string{"running"}
	} else if len(f.StatusFilter) == 1 {
		switch strings.ToLower(f.StatusFilter[0]) {
		case "running":
			f.StatusFilter = []string{"restarting"}
		case "restarting":
			f.StatusFilter = []string{"exited"}
		case "exited":
			f.StatusFilter = []string{"paused"}
		default:
			f.StatusFilter = nil
		}
	} else {
		f.StatusFilter = nil
	}
	f.updateActive()
}

func (f *FilterState) SetMinLogLevel(level metrics.LogLevel) {
	f.MinLogLevel = level
	f.updateActive()
}

func (f *FilterState) CycleMinLogLevel() {
	switch f.MinLogLevel {
	case metrics.LogLevelUnknown:
		f.MinLogLevel = metrics.LogLevelInfo
	case metrics.LogLevelInfo:
		f.MinLogLevel = metrics.LogLevelWarn
	case metrics.LogLevelWarn:
		f.MinLogLevel = metrics.LogLevelError
	case metrics.LogLevelError:
		f.MinLogLevel = metrics.LogLevelUnknown
	default:
		f.MinLogLevel = metrics.LogLevelUnknown
	}
	f.updateActive()
}

func (f *FilterState) Clear() {
	f.SearchText = ""
	f.StatusFilter = nil
	f.LabelFilters = make(map[string]string)
	f.MinLogLevel = metrics.LogLevelUnknown
	f.Active = false
}

// FilterContainers applies the filters to the containers and their logs.
//
// The search text works on two levels. A container whose service name, ID or
// image contains it (ignoring case) is listed, and all of its log lines are
// shown. A container that does not match is not listed, but the lines of its
// logs that contain the text are still shown, as long as it passes the status
// and label filters. The minimum log level applies to every line.
func FilterContainers(containers docker.Containers, filter FilterState) docker.Containers {
	if !filter.Active {
		return containers
	}

	var filtered docker.Containers
	filtered.Host = containers.Host
	filtered.Traces = containers.Traces
	filtered.AWS = containers.AWS

	effFilter, searchLower, traceFilter := parseSearchQualifiers(filter)

	for _, c := range containers.C {
		if !matchesStatusAndLabels(c, effFilter) {
			continue
		}

		listed := matchesSearch(c, searchLower)
		if listed {
			filtered.C = append(filtered.C, c)
		}

		serviceName := c.Service
		if serviceName == "" {
			if len(c.ID) >= 12 {
				serviceName = c.ID[:12]
			} else {
				serviceName = c.ID
			}
		}

		for _, line := range c.Log {
			lowerLine := strings.ToLower(line)
			if !listed && !strings.Contains(lowerLine, searchLower) {
				continue
			}
			if traceFilter != "" && !strings.Contains(lowerLine, traceFilter) {
				continue
			}
			level := docker.ParseLogLevel(line)
			if effFilter.MinLogLevel > metrics.LogLevelUnknown && level < effFilter.MinLogLevel {
				continue
			}
			filtered.FlatLogs = append(filtered.FlatLogs, docker.FormattedLog{
				Service: serviceName,
				Line:    line,
				Level:   level,
			})
		}
	}

	return filtered
}

// parseSearchQualifiers extracts status:<state>, label:<key>[=<val>], and
// trace:<trace_id> tokens from filter.SearchText and merges them into a copy
// of filter, returning the remaining free-text search and trace filter in lower case.
func parseSearchQualifiers(filter FilterState) (FilterState, string, string) {
	eff := filter
	if len(filter.StatusFilter) > 0 {
		eff.StatusFilter = append([]string(nil), filter.StatusFilter...)
	}
	eff.LabelFilters = maps.Clone(filter.LabelFilters)
	if eff.LabelFilters == nil {
		eff.LabelFilters = make(map[string]string)
	}

	if filter.SearchText == "" {
		return eff, "", ""
	}

	var freeTokens []string
	var inlineStatuses []string
	var traceFilter string
	for _, tok := range strings.Fields(filter.SearchText) {
		lowerTok := strings.ToLower(tok)
		if val, ok := strings.CutPrefix(lowerTok, "status:"); ok && val != "" {
			inlineStatuses = append(inlineStatuses, val)
			continue
		}
		if val, ok := strings.CutPrefix(lowerTok, "trace:"); ok && val != "" {
			traceFilter = val
			continue
		}
		if rawLabel, ok := strings.CutPrefix(tok, "label:"); ok && rawLabel != "" {
			if k, v, hasEq := strings.Cut(rawLabel, "="); hasEq && k != "" {
				eff.LabelFilters[k] = v
				continue
			}
			eff.LabelFilters[rawLabel] = "*"
			continue
		}
		freeTokens = append(freeTokens, lowerTok)
	}

	if len(inlineStatuses) > 0 {
		eff.StatusFilter = append(eff.StatusFilter, inlineStatuses...)
	}

	return eff, strings.Join(freeTokens, " "), traceFilter
}

// matchesSearch reports whether the container's service name, ID or image
// contains the search text. searchLower is already lower case; an empty search
// matches every container.
func matchesSearch(c docker.Container, searchLower string) bool {
	if searchLower == "" {
		return true
	}
	return strings.Contains(strings.ToLower(c.Service), searchLower) ||
		strings.Contains(strings.ToLower(c.ID), searchLower) ||
		strings.Contains(strings.ToLower(c.Image), searchLower)
}

// matchesStatusAndLabels applies the status and label filters.
func matchesStatusAndLabels(c docker.Container, filter FilterState) bool {
	if len(filter.StatusFilter) > 0 {
		found := false
		for _, status := range filter.StatusFilter {
			if strings.ToLower(c.State) == strings.ToLower(status) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for key, value := range filter.LabelFilters {
		actual, ok := c.Labels[key]
		if !ok && key == "com.docker.compose.service" && c.Service != "" {
			actual = c.Service
			ok = true
		}
		if !ok {
			return false
		}
		if value != "*" && actual != value {
			return false
		}
	}

	return true
}
