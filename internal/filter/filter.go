package filter

import (
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
	if value == "" {
		delete(f.LabelFilters, key)
	} else {
		f.LabelFilters[key] = value
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

	searchLower := strings.ToLower(filter.SearchText)

	for _, c := range containers.C {
		if !matchesStatusAndLabels(c, filter) {
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
			if !listed && !strings.Contains(strings.ToLower(line), searchLower) {
				continue
			}
			level := docker.ParseLogLevel(line)
			if filter.MinLogLevel > metrics.LogLevelUnknown && level < filter.MinLogLevel {
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
		switch key {
		case "com.docker.compose.service":
			if c.Service != value {
				return false
			}
		}
	}

	return true
}
