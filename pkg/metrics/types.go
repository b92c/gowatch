package metrics

import (
	"fmt"
	"time"
)

// MaxHistoryPoints defines how many data points we keep in memory (~2 mins with 2s polling)
const MaxHistoryPoints = 60

type LogLevel int

const (
	LogLevelUnknown LogLevel = iota
	LogLevelTrace
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelTrace:
		return "TRACE"
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type MetricPoint struct {
	Timestamp time.Time
	Value     float64
}

type ContainerStats struct {
	CPUHistory     []MetricPoint
	MemHistory     []MetricPoint
	DiskReadBytes  uint64
	NetTxBytes     uint64
	NetRxPackets   uint64
	NetTxPackets   uint64
	CPUPercent     float64
	DiskWriteBytes uint64
	DiskReadOps    uint64
	DiskWriteOps   uint64
	PIDsCurrent    uint64
	OOMEvents      uint64
	NetRxBytes     uint64
	MemUsage       uint64
}

type HostInfo struct {
	CPUCount int
	MemTotal uint64
	MemFree  uint64
}

func GetStatsSummary(values []MetricPoint) (min, max, avg float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}

	min = values[0].Value
	max = values[0].Value
	sum := 0.0

	for _, v := range values {
		if v.Value < min {
			min = v.Value
		}
		if v.Value > max {
			max = v.Value
		}
		sum += v.Value
	}

	avg = sum / float64(len(values))
	return min, max, avg
}

func FormatStatsSummary(values []MetricPoint, unit string) string {
	min, max, avg := GetStatsSummary(values)
	if unit == "%" {
		return fmt.Sprintf("min:%.1f%% max:%.1f%% avg:%.1f%%", min, max, avg)
	}
	return fmt.Sprintf("min:%.1f%s max:%.1f%s avg:%.1f%s", min, unit, max, unit, avg, unit)
}

// FormatByteStatsSummaryMB formats byte-denominated MetricPoints as MB summaries.
func FormatByteStatsSummaryMB(values []MetricPoint) string {
	minVal, maxVal, avgVal := GetStatsSummary(values)
	const mb = 1024.0 * 1024.0
	return fmt.Sprintf("min:%.1fMB max:%.1fMB avg:%.1fMB", minVal/mb, maxVal/mb, avgVal/mb)
}
