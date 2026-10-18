package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/pkg/metrics"
)

// exportFilePrefix and exportFileTimeLayout build the name of an export file:
// gowatch-logs-YYYYMMDD-HHMMSS.json, in local time.
const (
	exportFilePrefix     = "gowatch-logs-"
	exportFileTimeLayout = "20060102-150405"
	exportFileSuffix     = ".json"
	exportFileMode       = 0o600
	// maxExportNameTries bounds the -1, -2, ... suffixes tried when the name is taken.
	maxExportNameTries = 1000
)

// exportFilters is what was filtering the logs when they were exported.
type exportFilters struct {
	Search      string `json:"search"`
	MinLogLevel string `json:"min_log_level"`
}

// exportLine is one exported log line.
type exportLine struct {
	Service string `json:"service"`
	Level   string `json:"level,omitempty"`
	Line    string `json:"line"`
}

// logsExport is the document written by an export. Its fields are laid out for
// memory (see make field-check), not for the file: MarshalJSON writes the keys
// in the order exported_at, filters, count, logs.
type logsExport struct {
	Filters    exportFilters `json:"filters"`
	ExportedAt string        `json:"exported_at"`
	Logs       []exportLine  `json:"logs"`
	Count      int           `json:"count"`
}

// MarshalJSON writes the document with its keys in reading order.
func (e logsExport) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, field := range []struct {
		value any
		key   string
	}{
		{key: "exported_at", value: e.ExportedAt},
		{key: "filters", value: e.Filters},
		{key: "count", value: e.Count},
		{key: "logs", value: e.Logs},
	} {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeJSONValue(&buf, field.key); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		if err := writeJSONValue(&buf, field.value); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// writeJSONValue appends the compact JSON of v, with <, > and & left as they
// are (the lines are raw text, not HTML).
func writeJSONValue(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1) // Encode ends with a newline
	return nil
}

// levelName is the name of a log level for the export; empty when it is not known.
func levelName(l metrics.LogLevel) string {
	if l < metrics.LogLevelTrace || l > metrics.LogLevelFatal {
		return ""
	}
	return l.String()
}

// newLogsExport builds the document for the given filters and lines.
func newLogsExport(now time.Time, search string, minLevel metrics.LogLevel, logs []docker.FormattedLog) logsExport {
	lines := make([]exportLine, 0, len(logs)) // never nil: an empty export has "logs": []
	for _, l := range logs {
		lines = append(lines, exportLine{Service: l.Service, Level: levelName(l.Level), Line: l.Line})
	}
	return logsExport{
		ExportedAt: now.Format(time.RFC3339),
		Filters:    exportFilters{Search: search, MinLogLevel: levelName(minLevel)},
		Count:      len(lines),
		Logs:       lines,
	}
}

// writeLogsExport writes the logs to a new JSON file in dir and returns its
// path. The file is named gowatch-logs-YYYYMMDD-HHMMSS.json after now (local
// time); when that name exists, -1, -2, ... is added before the extension. An
// existing file is never overwritten. The file is readable by its owner only.
func writeLogsExport(dir string, now time.Time, search string, minLevel metrics.LogLevel, logs []docker.FormattedLog) (string, error) {
	// An encoder, to keep <, > and & as they are: the lines are raw text, not HTML.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(newLogsExport(now, search, minLevel, logs)); err != nil { // Encode ends the document with a newline
		return "", fmt.Errorf("encoding the logs: %w", err)
	}
	data := buf.Bytes()

	base := exportFilePrefix + now.Local().Format(exportFileTimeLayout)
	for attempt := 0; attempt <= maxExportNameTries; attempt++ {
		name := base + exportFileSuffix
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d%s", base, attempt, exportFileSuffix)
		}
		path := filepath.Join(dir, name)

		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, exportFileMode) // #nosec G304 -- the directory is the working directory and the name is generated here
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("no free file name %s%s after %d tries", base, exportFileSuffix, maxExportNameTries)
}
