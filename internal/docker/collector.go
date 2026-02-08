// Package docker comment :)
package docker

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type HostInfo struct {
	CPUCount int
	MemTotal uint64
	MemFree  uint64
}

type ContainerStats struct {
	CPUPercent float64
	MemUsage   uint64
}

type ContainerLog struct {
	ID   string
	Logs []string
}

type FormattedLog struct {
	Service string
	Line    string
}

var (
	previousStats = make(map[string]container.StatsResponse)
	statsMutex    sync.RWMutex
)

func getContainerStats(ctx context.Context, apiClient *client.Client, containerID string) ContainerStats {
	stats, err := apiClient.ContainerStats(ctx, containerID, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		return ContainerStats{CPUPercent: 0, MemUsage: 0}
	}
	defer stats.Body.Close()

	var statsJSON container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return ContainerStats{CPUPercent: 0, MemUsage: 0}
	}

	statsMutex.Lock()
	prevStats, exists := previousStats[containerID]
	if exists {
		statsJSON.PreCPUStats = prevStats.CPUStats
	}
	previousStats[containerID] = statsJSON
	statsMutex.Unlock()

	cpuPercent, memUsage := ParseStats(statsJSON)
	return ContainerStats{CPUPercent: cpuPercent, MemUsage: memUsage}
}

func getContainerLogs(ctx context.Context, apiClient *client.Client, containerID string) []string {
	logs, err := apiClient.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "50",
	})
	if err != nil {
		return []string{"Error fetching logs"}
	}
	defer logs.Close()

	return ParseLogs(logs)
}

func getHostInfo() HostInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return HostInfo{
		CPUCount: runtime.NumCPU(),
		MemTotal: memStats.Sys,
		MemFree:  memStats.Frees,
	}
}

type Containers struct {
	C        []Container
	Host     HostInfo
	Logs     []ContainerLog
	FlatLogs []FormattedLog
}

type Container struct {
	ID         string
	Image      string
	Status     string
	State      string
	Command    string
	DependsOn  string
	Service    string
	SOVersion  string
	WorkingDir string
	ConfigFile string
	CreatedAt  int64
	CPUPercent float64
	MemUsage   uint64
	Log        []string
}

func WatchContainers(ctx context.Context, apiClient *client.Client) (Containers, error) {
	cntList, err := apiClient.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return Containers{}, err
	}

	var containers Containers
	for _, c := range cntList.Items {
		stat := getContainerStats(ctx, apiClient, c.ID)
		logs := getContainerLogs(ctx, apiClient, c.ID)
		containers.C = append(containers.C, Container{
			ID: c.ID, Image: c.Image, Status: c.Status, State: string(c.State),
			Command: c.Command, DependsOn: c.Labels["com.docker.compose.depends_on"],
			Service:    c.Labels["com.docker.compose.service"],
			SOVersion:  c.Labels["org.opencontainers.image.ref.name"] + " " + c.Labels["org.opencontainers.image.version"],
			WorkingDir: c.Labels["com.docker.compose.project.working_dir"], ConfigFile: c.Labels["com.docker.compose.project.config_files"],
			CreatedAt:  c.Created,
			CPUPercent: stat.CPUPercent,
			MemUsage:   stat.MemUsage,
			Log:        logs,
		})
	}
	containers.Host = getHostInfo()

	for _, c := range containers.C {
		serviceName := c.Service
		if serviceName == "" {
			if len(c.ID) >= 12 {
				serviceName = c.ID[:12]
			} else {
				serviceName = c.ID
			}
		}
		for _, line := range c.Log {
			containers.FlatLogs = append(containers.FlatLogs, FormattedLog{
				Service: serviceName,
				Line:    line,
			})
		}
	}

	return containers, nil
}
