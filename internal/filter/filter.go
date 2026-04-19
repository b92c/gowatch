package filter

import (
	"strings"

	"github.com/b92c/gowatch/internal/docker"
)

type FilterState struct {
	LabelFilters map[string]string
	SearchText   string
	StatusFilter []string
	Active       bool
}

func NewFilterState() FilterState {
	return FilterState{
		LabelFilters: make(map[string]string),
		Active:       false,
	}
}

func (f *FilterState) SetSearch(text string) {
	f.SearchText = strings.TrimSpace(text)
	f.Active = f.SearchText != "" || len(f.StatusFilter) > 0 || len(f.LabelFilters) > 0
}

func (f *FilterState) SetStatusFilter(status []string) {
	f.StatusFilter = status
	f.Active = f.SearchText != "" || len(f.StatusFilter) > 0 || len(f.LabelFilters) > 0
}

func (f *FilterState) SetLabelFilter(key, value string) {
	if value == "" {
		delete(f.LabelFilters, key)
	} else {
		f.LabelFilters[key] = value
	}
	f.Active = f.SearchText != "" || len(f.StatusFilter) > 0 || len(f.LabelFilters) > 0
}

func (f *FilterState) Clear() {
	f.SearchText = ""
	f.StatusFilter = nil
	f.LabelFilters = make(map[string]string)
	f.Active = false
}

func FilterContainers(containers docker.Containers, filter FilterState) docker.Containers {
	if !filter.Active {
		return containers
	}

	var filtered docker.Containers
	filtered.Host = containers.Host

	for _, c := range containers.C {
		if !matchesFilter(c, filter) {
			continue
		}

		filtered.C = append(filtered.C, c)

		serviceName := c.Service
		if serviceName == "" {
			if len(c.ID) >= 12 {
				serviceName = c.ID[:12]
			} else {
				serviceName = c.ID
			}
		}

		for _, line := range c.Log {
			filtered.FlatLogs = append(filtered.FlatLogs, docker.FormattedLog{
				Service: serviceName,
				Line:    line,
			})
		}
	}

	return filtered
}

func matchesFilter(c docker.Container, filter FilterState) bool {
	if filter.SearchText != "" {
		searchLower := strings.ToLower(filter.SearchText)
		match := false
		if strings.Contains(strings.ToLower(c.Service), searchLower) {
			match = true
		} else if strings.Contains(strings.ToLower(c.ID), searchLower) {
			match = true
		} else if strings.Contains(strings.ToLower(c.Image), searchLower) {
			match = true
		}
		if !match {
			return false
		}
	}

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
