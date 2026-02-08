package docker

import (
	"io"
	"strings"

	"github.com/moby/moby/api/types/container"
)

func ParseStats(statsJSON container.StatsResponse) (cpu float64, mem uint64) {
	if statsJSON.PreCPUStats.CPUUsage.TotalUsage == 0 || statsJSON.PreCPUStats.SystemUsage == 0 {
		return 0.0, statsJSON.MemoryStats.Usage
	}

	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
	cpuPercent := 0.0

	var onlineCPUs float64
	if statsJSON.CPUStats.OnlineCPUs != 0 {
		onlineCPUs = float64(statsJSON.CPUStats.OnlineCPUs)
	} else {
		onlineCPUs = float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}

	memUsage := statsJSON.MemoryStats.Usage

	return cpuPercent, memUsage
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
