package docker

import (
	"math"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestParseStatsExtractsExtendedMetrics(t *testing.T) {
	stats := container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 200},
			SystemUsage: 300,
			OnlineCPUs:  2,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100},
			SystemUsage: 200,
		},
		MemoryStats: container.MemoryStats{
			Usage:   1024,
			Failcnt: 2,
		},
		PidsStats: container.PidsStats{
			Current: 8,
		},
		Networks: map[string]container.NetworkStats{
			"eth0": {RxBytes: 100, TxBytes: 200, RxPackets: 10, TxPackets: 20},
			"eth1": {RxBytes: 300, TxBytes: 400, RxPackets: 30, TxPackets: 40},
		},
		BlkioStats: container.BlkioStats{
			IoServiceBytesRecursive: []container.BlkioStatEntry{
				{Op: "READ", Value: 500},
				{Op: "Write", Value: 700},
			},
			IoServicedRecursive: []container.BlkioStatEntry{
				{Op: "read", Value: 5},
				{Op: "WRITE", Value: 7},
			},
		},
	}

	parsed := ParseStats(stats)

	if math.Abs(parsed.CPUPercent-200.0) > 0.001 {
		t.Fatalf("expected CPUPercent=200.0, got %.3f", parsed.CPUPercent)
	}
	if parsed.MemUsage != 1024 {
		t.Fatalf("expected MemUsage=1024, got %d", parsed.MemUsage)
	}
	if parsed.NetRxBytes != 400 || parsed.NetTxBytes != 600 {
		t.Fatalf("expected Net bytes 400/600, got %d/%d", parsed.NetRxBytes, parsed.NetTxBytes)
	}
	if parsed.NetRxPackets != 40 || parsed.NetTxPackets != 60 {
		t.Fatalf("expected Net packets 40/60, got %d/%d", parsed.NetRxPackets, parsed.NetTxPackets)
	}
	if parsed.DiskReadBytes != 500 || parsed.DiskWriteBytes != 700 {
		t.Fatalf("expected Disk bytes 500/700, got %d/%d", parsed.DiskReadBytes, parsed.DiskWriteBytes)
	}
	if parsed.DiskReadOps != 5 || parsed.DiskWriteOps != 7 {
		t.Fatalf("expected Disk ops 5/7, got %d/%d", parsed.DiskReadOps, parsed.DiskWriteOps)
	}
	if parsed.PIDsCurrent != 8 {
		t.Fatalf("expected PIDsCurrent=8, got %d", parsed.PIDsCurrent)
	}
	if parsed.OOMEvents != 2 {
		t.Fatalf("expected OOMEvents=2, got %d", parsed.OOMEvents)
	}
}

func TestParseStatsWithoutPreviousSampleKeepsCPUZero(t *testing.T) {
	stats := container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 500},
			SystemUsage: 1000,
			OnlineCPUs:  4,
		},
		MemoryStats: container.MemoryStats{
			Usage:   2048,
			Failcnt: 1,
		},
		PidsStats: container.PidsStats{
			Current: 3,
		},
		Networks: map[string]container.NetworkStats{
			"eth0": {RxBytes: 111, TxBytes: 222, RxPackets: 11, TxPackets: 22},
		},
		BlkioStats: container.BlkioStats{
			IoServiceBytesRecursive: []container.BlkioStatEntry{
				{Op: "Read", Value: 333},
				{Op: "Write", Value: 444},
			},
			IoServicedRecursive: []container.BlkioStatEntry{
				{Op: "Read", Value: 3},
				{Op: "Write", Value: 4},
			},
		},
	}

	parsed := ParseStats(stats)

	if parsed.CPUPercent != 0 {
		t.Fatalf("expected CPUPercent=0 without previous sample, got %.3f", parsed.CPUPercent)
	}
	if parsed.MemUsage != 2048 {
		t.Fatalf("expected MemUsage=2048, got %d", parsed.MemUsage)
	}
	if parsed.NetRxBytes != 111 || parsed.NetTxBytes != 222 {
		t.Fatalf("expected Net bytes 111/222, got %d/%d", parsed.NetRxBytes, parsed.NetTxBytes)
	}
	if parsed.NetRxPackets != 11 || parsed.NetTxPackets != 22 {
		t.Fatalf("expected Net packets 11/22, got %d/%d", parsed.NetRxPackets, parsed.NetTxPackets)
	}
	if parsed.DiskReadBytes != 333 || parsed.DiskWriteBytes != 444 {
		t.Fatalf("expected Disk bytes 333/444, got %d/%d", parsed.DiskReadBytes, parsed.DiskWriteBytes)
	}
	if parsed.DiskReadOps != 3 || parsed.DiskWriteOps != 4 {
		t.Fatalf("expected Disk ops 3/4, got %d/%d", parsed.DiskReadOps, parsed.DiskWriteOps)
	}
	if parsed.PIDsCurrent != 3 {
		t.Fatalf("expected PIDsCurrent=3, got %d", parsed.PIDsCurrent)
	}
	if parsed.OOMEvents != 1 {
		t.Fatalf("expected OOMEvents=1, got %d", parsed.OOMEvents)
	}
}
