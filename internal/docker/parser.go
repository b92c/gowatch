package docker

import (
	"encoding/binary"
	"io"
	"strings"

	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/moby/moby/api/types/container"
)

func ParseStats(statsJSON container.StatsResponse) metrics.ContainerStats {
	parsed := metrics.ContainerStats{
		MemUsage:       statsJSON.MemoryStats.Usage,
		PIDsCurrent:    statsJSON.PidsStats.Current,
		OOMEvents:      statsJSON.MemoryStats.Failcnt,
		NetRxBytes:     parseNetworkRxBytes(statsJSON),
		NetTxBytes:     parseNetworkTxBytes(statsJSON),
		NetRxPackets:   parseNetworkRxPackets(statsJSON),
		NetTxPackets:   parseNetworkTxPackets(statsJSON),
		DiskReadBytes:  parseDiskReadBytes(statsJSON),
		DiskWriteBytes: parseDiskWriteBytes(statsJSON),
		DiskReadOps:    parseDiskReadOps(statsJSON),
		DiskWriteOps:   parseDiskWriteOps(statsJSON),
	}

	if statsJSON.PreCPUStats.CPUUsage.TotalUsage == 0 || statsJSON.PreCPUStats.SystemUsage == 0 {
		return parsed
	}

	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)

	var onlineCPUs float64
	if statsJSON.CPUStats.OnlineCPUs != 0 {
		onlineCPUs = float64(statsJSON.CPUStats.OnlineCPUs)
	} else {
		onlineCPUs = float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		parsed.CPUPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}

	return parsed
}

func parseNetworkRxBytes(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, stats := range statsJSON.Networks {
		total += stats.RxBytes
	}
	return total
}

func parseNetworkTxBytes(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, stats := range statsJSON.Networks {
		total += stats.TxBytes
	}
	return total
}

func parseNetworkRxPackets(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, stats := range statsJSON.Networks {
		total += stats.RxPackets
	}
	return total
}

func parseNetworkTxPackets(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, stats := range statsJSON.Networks {
		total += stats.TxPackets
	}
	return total
}

func parseDiskReadBytes(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, entry := range statsJSON.BlkioStats.IoServiceBytesRecursive {
		if strings.EqualFold(entry.Op, "read") {
			total += entry.Value
		}
	}
	return total
}

func parseDiskWriteBytes(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, entry := range statsJSON.BlkioStats.IoServiceBytesRecursive {
		if strings.EqualFold(entry.Op, "write") {
			total += entry.Value
		}
	}
	return total
}

func parseDiskReadOps(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, entry := range statsJSON.BlkioStats.IoServicedRecursive {
		if strings.EqualFold(entry.Op, "read") {
			total += entry.Value
		}
	}
	return total
}

func parseDiskWriteOps(statsJSON container.StatsResponse) uint64 {
	var total uint64
	for _, entry := range statsJSON.BlkioStats.IoServicedRecursive {
		if strings.EqualFold(entry.Op, "write") {
			total += entry.Value
		}
	}
	return total
}

const (
	logFrameHeaderSize    = 8
	logFramePayloadSizeAt = 4
)

func ParseLogs(rawLogs io.ReadCloser) []string {
	var logs []string
	frameHeader := make([]byte, logFrameHeaderSize)

	for {
		_, err := io.ReadFull(rawLogs, frameHeader)
		if err != nil {
			break
		}

		payloadSize := int(binary.BigEndian.Uint32(frameHeader[logFramePayloadSizeAt:]))
		if payloadSize <= 0 {
			continue
		}

		payload := make([]byte, payloadSize)
		_, err = io.ReadFull(rawLogs, payload)
		if err != nil {
			break
		}

		logLine := strings.TrimSpace(string(payload))
		if logLine != "" {
			logs = append(logs, logLine)
		}
	}

	if len(logs) == 0 {
		return []string{"No logs available"}
	}

	return logs
}

func ParseLogLevel(line string) metrics.LogLevel {
	upper := strings.ToUpper(line)
	if upper == "" {
		return metrics.LogLevelUnknown
	}

	if strings.Contains(upper, "FATAL") || strings.Contains(upper, "\"LEVEL\":\"FATAL\"") || strings.Contains(upper, "LEVEL=FATAL") || strings.Contains(upper, "[CRITICAL]") || strings.Contains(upper, "CRITICAL:") {
		return metrics.LogLevelFatal
	}
	if strings.Contains(upper, "ERROR") || strings.Contains(upper, "ERR ") || strings.Contains(upper, "\"LEVEL\":\"ERROR\"") || strings.Contains(upper, "\"SEVERITY\":\"ERROR\"") || strings.Contains(upper, "LEVEL=ERROR") || strings.Contains(upper, "[ERR]") {
		return metrics.LogLevelError
	}
	if strings.Contains(upper, "WARN") || strings.Contains(upper, "WARNING") || strings.Contains(upper, "WRN ") || strings.Contains(upper, "\"LEVEL\":\"WARN\"") || strings.Contains(upper, "LEVEL=WARN") || strings.Contains(upper, "[WRN]") {
		return metrics.LogLevelWarn
	}
	if strings.Contains(upper, "INFO") || strings.Contains(upper, "INF ") || strings.Contains(upper, "\"LEVEL\":\"INFO\"") || strings.Contains(upper, "LEVEL=INFO") || strings.Contains(upper, "[INF]") {
		return metrics.LogLevelInfo
	}
	if strings.Contains(upper, "DEBUG") || strings.Contains(upper, "DBG ") || strings.Contains(upper, "\"LEVEL\":\"DEBUG\"") || strings.Contains(upper, "LEVEL=DEBUG") || strings.Contains(upper, "[DBG]") {
		return metrics.LogLevelDebug
	}
	if strings.Contains(upper, "TRACE") || strings.Contains(upper, "TRC ") || strings.Contains(upper, "\"LEVEL\":\"TRACE\"") || strings.Contains(upper, "LEVEL=TRACE") {
		return metrics.LogLevelTrace
	}

	return metrics.LogLevelUnknown
}
