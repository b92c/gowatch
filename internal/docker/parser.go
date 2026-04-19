package docker

import (
	"io"
	"strings"

	"github.com/moby/moby/api/types/container"
)

func ParseStats(statsJSON container.StatsResponse) ContainerStats {
	parsed := ContainerStats{
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

func ParseLogs(rawLogs io.ReadCloser) []string {
	var logs []string
	header := make([]byte, 8)

	for {
		// Read the 8-byte Docker multiplexed stream header
		// Format: [stream_type (1 byte)][padding (3 bytes)][size (4 bytes big-endian)]
		_, err := io.ReadFull(rawLogs, header)
		if err != nil {
			break
		}

		// Parse payload size from bytes 4-7 (big-endian uint32)
		size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
		if size <= 0 {
			continue
		}

		// Read the exact payload bytes for this frame
		payload := make([]byte, size)
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
