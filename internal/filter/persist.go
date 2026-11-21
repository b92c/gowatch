package filter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/b92c/gowatch/pkg/metrics"
)

type persistedFilterState struct {
	LabelFilters map[string]string `json:"labels,omitempty"`
	SearchText   string            `json:"search,omitempty"`
	StatusFilter []string          `json:"status,omitempty"`
	MinLogLevel  metrics.LogLevel  `json:"min_log_level,omitempty"`
}

// SaveFilterState writes the active filter state to path with mode 0600.
func SaveFilterState(path string, state FilterState) error {
	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o700); err != nil {
		return err
	}
	payload := persistedFilterState{
		SearchText:   state.SearchText,
		StatusFilter: state.StatusFilter,
		LabelFilters: state.LabelFilters,
		MinLogLevel:  state.MinLogLevel,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(cleanPath, data, 0o600)
}

// LoadFilterState reads a previously saved filter state from path.
// If the file does not exist, it returns an empty FilterState and nil error.
func LoadFilterState(path string) (FilterState, error) {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath) // #nosec G304 -- path is the user's own config directory
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewFilterState(), nil
		}
		return FilterState{}, err
	}
	var payload persistedFilterState
	if err := json.Unmarshal(data, &payload); err != nil {
		return FilterState{}, err
	}
	fs := NewFilterState()
	fs.SetSearch(payload.SearchText)
	fs.SetStatusFilter(payload.StatusFilter)
	fs.SetLabelFilters(payload.LabelFilters)
	fs.SetMinLogLevel(payload.MinLogLevel)
	return fs, nil
}
