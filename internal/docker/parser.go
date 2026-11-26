package docker

import (
	"encoding/binary"
	"io"
	"strings"

	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/moby/moby/api/types/container"
)

func ParseStats(statsJSON container.StatsResponse) metrics.ContainerStats {
	var netRxBytes, netTxBytes, netRxPackets, netTxPackets uint64
	for _, stats := range statsJSON.Networks {
		netRxBytes += stats.RxBytes
		netTxBytes += stats.TxBytes
		netRxPackets += stats.RxPackets
		netTxPackets += stats.TxPackets
	}

	diskReadBytes, diskWriteBytes := parseBlkioReadWrite(statsJSON.BlkioStats.IoServiceBytesRecursive)
	diskReadOps, diskWriteOps := parseBlkioReadWrite(statsJSON.BlkioStats.IoServicedRecursive)

	parsed := metrics.ContainerStats{
		MemUsage:       statsJSON.MemoryStats.Usage,
		PIDsCurrent:    statsJSON.PidsStats.Current,
		OOMEvents:      statsJSON.MemoryStats.Failcnt,
		NetRxBytes:     netRxBytes,
		NetTxBytes:     netTxBytes,
		NetRxPackets:   netRxPackets,
		NetTxPackets:   netTxPackets,
		DiskReadBytes:  diskReadBytes,
		DiskWriteBytes: diskWriteBytes,
		DiskReadOps:    diskReadOps,
		DiskWriteOps:   diskWriteOps,
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

func parseBlkioReadWrite(entries []container.BlkioStatEntry) (readVal, writeVal uint64) {
	for _, entry := range entries {
		if strings.EqualFold(entry.Op, "read") {
			readVal += entry.Value
		} else if strings.EqualFold(entry.Op, "write") {
			writeVal += entry.Value
		}
	}
	return readVal, writeVal
}

const (
	logFrameHeaderSize    = 8
	logFramePayloadSizeAt = 4
	maxLogFrameSize       = 1 << 20
)

func ParseLogs(rawLogs io.ReadCloser) []string {
	var logs []string
	frameHeader := make([]byte, logFrameHeaderSize)
	scratch := make([]byte, 0, 4096)

	for {
		_, err := io.ReadFull(rawLogs, frameHeader)
		if err != nil {
			break
		}

		payloadSize := int(binary.BigEndian.Uint32(frameHeader[logFramePayloadSizeAt:]))
		if payloadSize <= 0 {
			continue
		}

		readSize := min(payloadSize, maxLogFrameSize)
		remaining := payloadSize - readSize

		if readSize > cap(scratch) {
			scratch = make([]byte, readSize)
		} else {
			scratch = scratch[:readSize]
		}

		_, err = io.ReadFull(rawLogs, scratch)
		if err != nil {
			break
		}

		if remaining > 0 {
			if _, err := io.CopyN(io.Discard, rawLogs, int64(remaining)); err != nil {
				break
			}
		}

		logLine := strings.TrimSpace(string(scratch))
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
