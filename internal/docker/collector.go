package docker

import (
	"context"
	"encoding/json"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
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
	TraceID string
	SpanID  string
	Level   metrics.LogLevel
}

// defaultLogTailLines is the number of log lines fetched per container when
// Configure does not override it.
const (
	defaultLogTailLines   = 50
	defaultMaxConcurrency = 16
	hostInfoCacheTTL      = 30 * time.Second
)

var (
	previousStats     = make(map[string]container.StatsResponse)
	historyStore      = make(map[string]metrics.ContainerStats)
	globalTraceStore  = trace.NewTraceStore()
	globalCorrelator  = trace.NewCorrelator(globalTraceStore)
	globalAWSManager  = aws.NewAWSClientManager()
	globalAlertEngine = alert.NewAlertEngine(nil)
	statsMutex        sync.RWMutex

	hostCacheMutex sync.Mutex
	cachedCPUCount int
	cachedMemTotal uint64
	cachedHostAt   time.Time

	// settingsMutex guards globalAWSManager and the values below, which
	// Configure may replace.
	settingsMutex        sync.RWMutex
	logTailLines         = defaultLogTailLines
	historyMaxPoints     = metrics.MaxHistoryPoints
	maxConcurrency       = defaultMaxConcurrency
	includeAllContainers bool
)

// Options tunes the collector. A zero value keeps the built-in default, so
// the zero Options changes nothing. AlertRules is the exception: nil keeps the
// default rules, while an empty non-nil slice installs no rules at all.
type Options struct {
	AWS                  aws.Options
	AlertRules           []alert.AlertRule
	LogTailLines         int
	HistoryMaxPoints     int
	MaxConcurrency       int
	IncludeAllContainers bool
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
	if o.MaxConcurrency > 0 {
		maxConcurrency = o.MaxConcurrency
	}
	if o.IncludeAllContainers {
		includeAllContainers = true
	}
	if o.AWS != (aws.Options{}) {
		globalAWSManager = aws.NewAWSClientManagerWithOptions(o.AWS)
	}
	settingsMutex.Unlock()

	if o.AlertRules != nil {
		globalAlertEngine.SetRules(o.AlertRules)
	}
}

func currentMaxConcurrency() int {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	if maxConcurrency <= 0 {
		return defaultMaxConcurrency
	}
	return maxConcurrency
}

func currentIncludeAllContainers() bool {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return includeAllContainers
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

// trimHistory keeps only the newest limit points by compacting in place.
func trimHistory(points []metrics.MetricPoint, limit int) []metrics.MetricPoint {
	if len(points) > limit {
		copy(points, points[len(points)-limit:])
		return points[:limit]
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

	maxPoints := currentHistoryMaxPoints()

	statsMutex.Lock()
	prevStats, exists := previousStats[containerID]
	if exists {
		statsJSON.PreCPUStats = prevStats.CPUStats
	}
	previousStats[containerID] = statsJSON
	statsMutex.Unlock()

	stat := ParseStats(statsJSON)

	statsMutex.Lock()
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

	h.CPUHistory = trimHistory(h.CPUHistory, maxPoints)
	h.MemHistory = trimHistory(h.MemHistory, maxPoints)

	historyStore[containerID] = h

	out := h
	out.CPUHistory = slices.Clone(h.CPUHistory)
	out.MemHistory = slices.Clone(h.MemHistory)
	statsMutex.Unlock()
	return out
}

func pruneStaleContainers(active []container.Summary) {
	activeIDs := make(map[string]struct{}, len(active))
	for _, c := range active {
		activeIDs[c.ID] = struct{}{}
	}
	statsMutex.Lock()
	for id := range previousStats {
		if _, ok := activeIDs[id]; !ok {
			delete(previousStats, id)
		}
	}
	for id := range historyStore {
		if _, ok := activeIDs[id]; !ok {
			delete(historyStore, id)
		}
	}
	statsMutex.Unlock()
}

func resetHostInfoCache() {
	hostCacheMutex.Lock()
	cachedCPUCount = 0
	cachedMemTotal = 0
	cachedHostAt = time.Time{}
	hostCacheMutex.Unlock()
}

func getContainerLogs(ctx context.Context, apiClient *client.Client, containerID string) []string {
	return getContainerLogsWithTail(ctx, apiClient, containerID, currentLogTailLines())
}

func getContainerLogsWithTail(ctx context.Context, apiClient *client.Client, containerID string, tailLines int) []string {
	logs, err := apiClient.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tailLines),
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
		hostCacheMutex.Lock()
		fresh := cachedCPUCount > 0 && time.Since(cachedHostAt) < hostInfoCacheTTL
		cCPU, cMem := cachedCPUCount, cachedMemTotal
		hostCacheMutex.Unlock()

		if fresh {
			cpuCount = cCPU
			memTotal = cMem
		} else if info, err := apiClient.Info(ctx, client.InfoOptions{}); err == nil {
			cpuCount = info.Info.NCPU
			if info.Info.MemTotal >= 0 {
				memTotal = uint64(info.Info.MemTotal) // #nosec G115 -- MemTotal is checked to be non-negative above
			} else {
				memTotal = 0
			}
			hostCacheMutex.Lock()
			cachedCPUCount = cpuCount
			cachedMemTotal = memTotal
			cachedHostAt = time.Now()
			hostCacheMutex.Unlock()
		}

		if memTotal > totalMemUsed {
			memFree = memTotal - totalMemUsed
		} else {
			memFree = 0
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
	Labels         map[string]string
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
	cntList, err := apiClient.ContainerList(ctx, client.ContainerListOptions{All: currentIncludeAllContainers()})
	if err != nil {
		return Containers{}, err
	}

	tailLines := currentLogTailLines()
	maxConc := currentMaxConcurrency()

	containers := Containers{
		C:        make([]Container, len(cntList.Items)),
		FlatLogs: make([]FormattedLog, 0, len(cntList.Items)*tailLines),
	}
	evalData := make([]alert.ContainerEvaluatorData, len(cntList.Items))

	var (
		totalMemAtomic atomic.Uint64
		wg             sync.WaitGroup
	)
	sem := make(chan struct{}, maxConc)

	for i, c := range cntList.Items {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, cnt container.Summary) {
			defer func() {
				<-sem
				wg.Done()
			}()

			var stat metrics.ContainerStats
			if cnt.State != "exited" && cnt.State != "dead" {
				stat = getContainerStats(ctx, apiClient, cnt.ID)
				totalMemAtomic.Add(stat.MemUsage)
			}
			logs := getContainerLogsWithTail(ctx, apiClient, cnt.ID, tailLines)

			var labels map[string]string
			if len(cnt.Labels) > 0 {
				labels = maps.Clone(cnt.Labels)
			}

			containers.C[idx] = Container{
				Labels: labels,
				ID:     cnt.ID, Image: cnt.Image, Status: cnt.Status, State: string(cnt.State),
				Command: cnt.Command, DependsOn: cnt.Labels["com.docker.compose.depends_on"],
				Service:    cnt.Labels["com.docker.compose.service"],
				SOVersion:  cnt.Labels["org.opencontainers.image.ref.name"] + " " + cnt.Labels["org.opencontainers.image.version"],
				WorkingDir: cnt.Labels["com.docker.compose.project.working_dir"], ConfigFile: cnt.Labels["com.docker.compose.project.config_files"],
				CreatedAt:      cnt.Created,
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
			}

			evalData[idx] = alert.ContainerEvaluatorData{
				ID:             cnt.ID,
				Service:        cnt.Labels["com.docker.compose.service"],
				State:          string(cnt.State),
				Logs:           logs,
				CPUPercent:     stat.CPUPercent,
				MemUsage:       stat.MemUsage,
				OOMEvents:      stat.OOMEvents,
				DiskReadBytes:  stat.DiskReadBytes,
				DiskWriteBytes: stat.DiskWriteBytes,
			}
		}(i, c)
	}
	wg.Wait()

	pruneStaleContainers(cntList.Items)
	containers.Host = getHostInfo(ctx, apiClient, totalMemAtomic.Load())

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
			var traceID, spanID string
			if sp := globalCorrelator.ProcessLogLine(serviceName, line); sp != nil {
				traceID = sp.TraceID
				spanID = sp.SpanID
			}
			containers.FlatLogs = append(containers.FlatLogs, FormattedLog{
				Service: serviceName,
				Line:    line,
				TraceID: traceID,
				SpanID:  spanID,
				Level:   level,
			})
		}
	}
	containers.Traces = globalTraceStore.GetTraces()
	awsMgr := GetGlobalAWSManager()
	if awsMgr.GetState().Enabled {
		_ = awsMgr.CollectAll(ctx)
	}
	containers.AWS = awsMgr.GetState()
	containers.Alerts = globalAlertEngine.Evaluate(evalData, containers.Host)

	return containers, nil
}
