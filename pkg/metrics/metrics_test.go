package metrics

import (
	"testing"
	"time"
)

func TestGetStatsSummary(t *testing.T) {
	points := []MetricPoint{
		{Timestamp: time.Now(), Value: 10.0},
		{Timestamp: time.Now(), Value: 20.0},
		{Timestamp: time.Now(), Value: 30.0},
	}

	min, max, avg := GetStatsSummary(points)

	if min != 10.0 {
		t.Errorf("expected min 10.0, got %.1f", min)
	}
	if max != 30.0 {
		t.Errorf("expected max 30.0, got %.1f", max)
	}
	if avg != 20.0 {
		t.Errorf("expected avg 20.0, got %.1f", avg)
	}
}

func TestGetStatsSummaryEmpty(t *testing.T) {
	min, max, avg := GetStatsSummary(nil)
	if min != 0 || max != 0 || avg != 0 {
		t.Errorf("expected 0 for empty slice, got min:%.1f, max:%.1f, avg:%.1f", min, max, avg)
	}
}

func TestFormatStatsSummary(t *testing.T) {
	points := []MetricPoint{
		{Timestamp: time.Now(), Value: 10.0},
		{Timestamp: time.Now(), Value: 20.0},
	}

	res := FormatStatsSummary(points, "%")
	expected := "min:10.0% max:20.0% avg:15.0%"
	if res != expected {
		t.Errorf("expected %s, got %s", expected, res)
	}

	res = FormatStatsSummary(points, "MB")
	expected = "min:10.0MB max:20.0MB avg:15.0MB"
	if res != expected {
		t.Errorf("expected %s, got %s", expected, res)
	}
}

func TestFormatByteStatsSummaryMB(t *testing.T) {
	const mb = 1024.0 * 1024.0
	points := []MetricPoint{
		{Timestamp: time.Now(), Value: 10.0 * mb},
		{Timestamp: time.Now(), Value: 20.0 * mb},
	}

	got := FormatByteStatsSummaryMB(points)
	want := "min:10.0MB max:20.0MB avg:15.0MB"
	if got != want {
		t.Errorf("FormatByteStatsSummaryMB() = %q, want %q", got, want)
	}

	if gotEmpty := FormatByteStatsSummaryMB(nil); gotEmpty != "min:0.0MB max:0.0MB avg:0.0MB" {
		t.Errorf("FormatByteStatsSummaryMB(nil) = %q, want all zeros", gotEmpty)
	}
}
