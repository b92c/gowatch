package docker

import (
	"context"
	"encoding/json"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/b92c/gowatch/internal/alert"
	"github.com/b92c/gowatch/internal/aws"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/pkg/metrics"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type ContainerLog struct {
	ID   string
	Logs []string
}

type FormattedLog struct {
	Service string
	Line    string
	Level   metrics.LogLevel
}

// defaultLogTailLines is the number of log lines fetched per container when
// Configure does not override it.
const defaultLogTailLines = 50

var (
	previousStats     = make(map[string]container.StatsResponse)
	historyStore      = make(map[string]metrics.ContainerStats)
	globalTraceStore  = trace.NewTraceStore()
	globalCorrelator  = trace.NewCorrelator(globalTraceStore)
	globalAWSManager  = aws.NewAWSClientManager()
	globalAlertEngine = alert.NewAlertEngine(nil)
	statsMutex        sync.RWMutex

	// settingsMutex guards globalAWSManager and the two values below, which
	// Configure may replace.
	settingsMutex    sync.RWMutex
	logTailLines     = defaultLogTailLines
	historyMaxPoints = metrics.MaxHistoryPoints
)

// Options tunes the collector. A zero value keeps the built-in default, so
// the zero Options changes nothing. AlertRules is the exception: nil keeps the
// default rules, while an empty non-nil slice installs no rules at all.
type Options struct {
	AlertRules       []alert.AlertRule
	AWS              aws.Options
	LogTailLines     int
	HistoryMaxPoints int
}

// Configure applies o to the collector. Call it once at startup, before the
// collection loop begins; it is safe to call concurrently with collection.
func Configure(o Options) {
	settingsMutex.Lock()
	if o.LogTailLines > 0 {
		logTailLines = o.LogTailLines
	}
	if o.HistoryMaxPoints > 0 {
		historyMaxPoints = o.HistoryMaxPoints
	}
	if !o.AWS.IsZero() {
		globalAWSManager = aws.NewAWSClientManagerWithOptions(o.AWS)
	}
	settingsMutex.Unlock()

	if o.AlertRules != nil {
		globalAlertEngine.SetRules(o.AlertRules)
	}
}

func currentLogTailLines() int {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return logTailLines
}

func currentHistoryMaxPoints() int {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return historyMaxPoints
}

// trimHistory keeps only the newest limit points.
func trimHistory(points []metrics.MetricPoint, limit int) []metrics.MetricPoint {
	if len(points) > limit {
		return points[len(points)-limit:]
	}
	return points
}

func GetGlobalTraceStore() *trace.TraceStore {
	return globalTraceStore
}

func GetGlobalAWSManager() *aws.AWSClientManager {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return globalAWSManager
}

func GetGlobalAlertEngine() *alert.AlertEngine {
	return globalAlertEngine
}

func getContainerStats(ctx context.Context, apiClient *client.Client, containerID string) metrics.ContainerStats {
	stats, err := apiClient.ContainerStats(ctx, containerID, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		return metrics.ContainerStats{}
	}
	defer stats.Body.Close()

	var statsJSON container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return metrics.ContainerStats{}
	}

	statsMutex.Lock()
	defer statsMutex.Unlock()

	prevStats, exists := previousStats[containerID]
	if exists {
		statsJSON.PreCPUStats = prevStats.CPUStats
	}
	previousStats[containerID] = statsJSON

	stat := ParseStats(statsJSON)

	h, exists := historyStore[containerID]
	if !exists {
		h = metrics.ContainerStats{}
	}

	now := time.Now()
	h.CPUPercent = stat.CPUPercent
	h.MemUsage = stat.MemUsage
	h.NetRxBytes = stat.NetRxBytes
	h.NetTxBytes = stat.NetTxBytes
	h.NetRxPackets = stat.NetRxPackets
	h.NetTxPackets = stat.NetTxPackets
	h.DiskReadBytes = stat.DiskReadBytes
	h.DiskWriteBytes = stat.DiskWriteBytes
	h.DiskReadOps = stat.DiskReadOps
	h.DiskWriteOps = stat.DiskWriteOps
	h.PIDsCurrent = stat.PIDsCurrent
	h.OOMEvents = stat.OOMEvents

	h.CPUHistory = append(h.CPUHistory, metrics.MetricPoint{Timestamp: now, Value: stat.CPUPercent})
	h.MemHistory = append(h.MemHistory, metrics.MetricPoint{Timestamp: now, Value: float64(stat.MemUsage)})

	maxPoints := currentHistoryMaxPoints()
	h.CPUHistory = trimHistory(h.CPUHistory, maxPoints)
	h.MemHistory = trimHistory(h.MemHistory, maxPoints)

	historyStore[containerID] = h
	return h
}

func getContainerLogs(ctx context.Context, apiClient *client.Client, containerID string) []string {
	logs, err := apiClient.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(currentLogTailLines()),
	})
	if err != nil {
		return []string{"Error fetching logs"}
	}
	defer logs.Close()

	return ParseLogs(logs)
}

func getHostInfo(ctx context.Context, apiClient *client.Client, totalMemUsed uint64) metrics.HostInfo {
	cpuCount := runtime.NumCPU()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memTotal := memStats.Sys
	memFree := memStats.Sys - memStats.Alloc

	if apiClient != nil {
		if info, err := apiClient.Info(ctx, client.InfoOptions{}); err == nil {
			cpuCount = info.Info.NCPU
			if info.Info.MemTotal >= 0 {
				memTotal = uint64(info.Info.MemTotal) // #nosec G115 -- MemTotal is checked to be non-negative above
			} else {
				memTotal = 0
			}
			if memTotal > totalMemUsed {
				memFree = memTotal - totalMemUsed
			} else {
				memFree = 0
			}
		}
	}

	return metrics.HostInfo{
		CPUCount: cpuCount,
		MemTotal: memTotal,
		MemFree:  memFree,
	}
}

type Containers struct {
	C        []Container
	Logs     []ContainerLog
	FlatLogs []FormattedLog
	Traces   []trace.Trace
	Alerts   []alert.Alert
	AWS      aws.AWSState
	Host     metrics.HostInfo
}

type Container struct {
	WorkingDir     string
	SOVersion      string
	Status         string
	State          string
	Command        string
	DependsOn      string
	ID             string
	Image          string
	ConfigFile     string
	Service        string
	CPUHistory     []metrics.MetricPoint
	Log            []string
	MemHistory     []metrics.MetricPoint
	CPUPercent     float64
	NetTxBytes     uint64
	NetRxPackets   uint64
	NetTxPackets   uint64
	DiskReadBytes  uint64
	DiskWriteBytes uint64
	DiskReadOps    uint64
	DiskWriteOps   uint64
	PIDsCurrent    uint64
	OOMEvents      uint64
	CreatedAt      int64
	NetRxBytes     uint64
	MemUsage       uint64
}

func WatchContainers(ctx context.Context, apiClient *client.Client) (Containers, error) {
	cntList, err := apiClient.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return Containers{}, err
	}

	var containers Containers
	var totalMemUsed uint64
	var evalData []alert.ContainerEvaluatorData

	for _, c := range cntList.Items {
		stat := getContainerStats(ctx, apiClient, c.ID)
		totalMemUsed += stat.MemUsage
		logs := getContainerLogs(ctx, apiClient, c.ID)
		containers.C = append(containers.C, Container{
			ID: c.ID, Image: c.Image, Status: c.Status, State: string(c.State),
			Command: c.Command, DependsOn: c.Labels["com.docker.compose.depends_on"],
			Service:    c.Labels["com.docker.compose.service"],
			SOVersion:  c.Labels["org.opencontainers.image.ref.name"] + " " + c.Labels["org.opencontainers.image.version"],
			WorkingDir: c.Labels["com.docker.compose.project.working_dir"], ConfigFile: c.Labels["com.docker.compose.project.config_files"],
			CreatedAt:      c.Created,
			CPUPercent:     stat.CPUPercent,
			MemUsage:       stat.MemUsage,
			NetRxBytes:     stat.NetRxBytes,
			NetTxBytes:     stat.NetTxBytes,
			NetRxPackets:   stat.NetRxPackets,
			NetTxPackets:   stat.NetTxPackets,
			DiskReadBytes:  stat.DiskReadBytes,
			DiskWriteBytes: stat.DiskWriteBytes,
			DiskReadOps:    stat.DiskReadOps,
			DiskWriteOps:   stat.DiskWriteOps,
			PIDsCurrent:    stat.PIDsCurrent,
			OOMEvents:      stat.OOMEvents,
			CPUHistory:     stat.CPUHistory,
			MemHistory:     stat.MemHistory,
			Log:            logs,
		})

		evalData = append(evalData, alert.ContainerEvaluatorData{
			ID:         c.ID,
			Service:    c.Labels["com.docker.compose.service"],
			State:      string(c.State),
			CPUPercent: stat.CPUPercent,
			MemUsage:   stat.MemUsage,
			OOMEvents:  stat.OOMEvents,
		})
	}
	containers.Host = getHostInfo(ctx, apiClient, totalMemUsed)

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
			level := ParseLogLevel(line)
			globalCorrelator.ProcessLogLine(serviceName, line)
			containers.FlatLogs = append(containers.FlatLogs, FormattedLog{
				Service: serviceName,
				Line:    line,
				Level:   level,
			})
		}
	}
	containers.Traces = globalTraceStore.GetTraces()
	containers.AWS = GetGlobalAWSManager().GetState()
	containers.Alerts = globalAlertEngine.Evaluate(evalData, containers.Host)

	return containers, nil
}
