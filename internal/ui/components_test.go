package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/b92c/gowatch/pkg/metrics"
)

func TestRenderSparkline(t *testing.T) {
	tests := []struct {
		name      string
		values    []metrics.MetricPoint
		maxPoints int
		expected  int
	}{
		{
			name:      "empty values",
			values:    []metrics.MetricPoint{},
			maxPoints: 10,
			expected:  10,
		},
		{
			name: "fewer than maxPoints",
			values: []metrics.MetricPoint{
				{Value: 1.0},
				{Value: 2.0},
			},
			maxPoints: 10,
			expected:  10,
		},
		{
			name: "more than maxPoints",
			values: []metrics.MetricPoint{
				{Value: 1.0}, {Value: 2.0}, {Value: 3.0}, {Value: 4.0},
				{Value: 5.0}, {Value: 6.0}, {Value: 7.0}, {Value: 8.0},
				{Value: 9.0}, {Value: 10.0}, {Value: 11.0},
			},
			maxPoints: 10,
			expected:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sparkline := RenderSparkline(tt.values, tt.maxPoints)
			runeCount := utf8.RuneCountInString(sparkline)
			if runeCount != tt.expected {
				t.Errorf("expected length of %d runes, got %d (string: %q)", tt.expected, runeCount, sparkline)
			}
		})
	}
}
