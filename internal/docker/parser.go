package docker

import (
	"bufio"
	"io"

	"github.com/moby/moby/api/types/container"
)

func ParseStats(statsJSON container.StatsResponse) (cpu float64, mem uint64) {
	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
	cpuPercent := 0.0
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}

	memUsage := statsJSON.MemoryStats.Usage

	return cpuPercent, memUsage
}

func ParseLogs(rawLogs io.ReadCloser) []string {
	var logs []string
	scanner := bufio.NewScanner(rawLogs)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 8 {
			logs = append(logs, line[8:])
		}
	}

	if len(logs) == 0 {
		return []string{"No logs available"}
	}

	return logs
}
