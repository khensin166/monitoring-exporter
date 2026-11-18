package collector

import (
	"encoding/json"
	"math"
	"strings"

	"docker-exporter/internal/model"

	"github.com/docker/docker/api/types"
)

// CalculateStats parses Docker ContainerStatsOneShot and calculates CPU, Memory, Network, and Block I/O metrics.
func CalculateStats(statsJSON []byte, prevCPU *model.CPUState) (cpuPct, memUsage, memLimit, memPct, netRx, netTx, blkRead, blkWrite float64, newCPU *model.CPUState) {
	var stats types.StatsJSON
	if err := json.Unmarshal(statsJSON, &stats); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, prevCPU
	}

	// 1. CPU Calculation
	cpuPct, newCPU = calculateCPUPercent(&stats, prevCPU)

	// 2. Memory Calculation
	memUsage, memLimit, memPct = calculateMemoryStats(&stats)

	// 3. Network I/O Calculation
	netRx, netTx = calculateNetworkStats(&stats)

	// 4. Block I/O Calculation
	blkRead, blkWrite = calculateBlockIOStats(&stats)

	return cpuPct, memUsage, memLimit, memPct, netRx, netTx, blkRead, blkWrite, newCPU
}

func calculateCPUPercent(stats *types.StatsJSON, prevCPU *model.CPUState) (float64, *model.CPUState) {
	currentTotal := stats.CPUStats.CPUUsage.TotalUsage
	currentSystem := stats.CPUStats.SystemUsage

	onlineCPUs := stats.CPUStats.OnlineCPUs
	if onlineCPUs == 0 {
		onlineCPUs = uint32(len(stats.CPUStats.CPUUsage.PercpuUsage))
		if onlineCPUs == 0 {
			onlineCPUs = 1
		}
	}

	newCPU := &model.CPUState{
		TotalUsage:  currentTotal,
		SystemUsage: currentSystem,
		OnlineCPUs:  onlineCPUs,
	}

	if prevCPU == nil {
		// Use PreCPUStats from Docker if available
		if stats.PreCPUStats.CPUUsage.TotalUsage > 0 && stats.PreCPUStats.SystemUsage > 0 {
			prevCPU = &model.CPUState{
				TotalUsage:  stats.PreCPUStats.CPUUsage.TotalUsage,
				SystemUsage: stats.PreCPUStats.SystemUsage,
				OnlineCPUs:  onlineCPUs,
			}
		} else {
			return 0.0, newCPU
		}
	}

	cpuDelta := float64(currentTotal) - float64(prevCPU.TotalUsage)
	systemDelta := float64(currentSystem) - float64(prevCPU.SystemUsage)

	if systemDelta <= 0.0 || cpuDelta <= 0.0 {
		return 0.0, newCPU
	}

	cpuPercent := (cpuDelta / systemDelta) * float64(onlineCPUs) * 100.0

	// Defensive check
	if math.IsNaN(cpuPercent) || math.IsInf(cpuPercent, 0) || cpuPercent < 0.0 {
		cpuPercent = 0.0
	}

	return cpuPercent, newCPU
}

func calculateMemoryStats(stats *types.StatsJSON) (usage, limit, percent float64) {
	rawUsage := float64(stats.MemoryStats.Usage)
	limit = float64(stats.MemoryStats.Limit)

	// Deduct cache/inactive_file to match 'docker stats' CLI behavior:
	// In cgroup v1: stats.MemoryStats.Stats["total_inactive_file"] or stats["cache"]
	// In cgroup v2: stats.MemoryStats.Stats["inactive_file"]
	var cache float64
	if val, ok := stats.MemoryStats.Stats["inactive_file"]; ok {
		cache = float64(val)
	} else if val, ok := stats.MemoryStats.Stats["total_inactive_file"]; ok {
		cache = float64(val)
	} else if val, ok := stats.MemoryStats.Stats["cache"]; ok {
		cache = float64(val)
	}

	if rawUsage > cache {
		usage = rawUsage - cache
	} else {
		usage = rawUsage
	}

	if limit > 0 {
		percent = (usage / limit) * 100.0
	} else {
		percent = 0.0
	}

	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0.0 {
		percent = 0.0
	}
	if percent > 100.0 {
		// In case limit is not constrained (e.g., host RAM limit)
		percent = math.Min(percent, 100.0)
	}

	return usage, limit, percent
}

func calculateNetworkStats(stats *types.StatsJSON) (rx, tx float64) {
	for _, net := range stats.Networks {
		rx += float64(net.RxBytes)
		tx += float64(net.TxBytes)
	}
	return rx, tx
}

func calculateBlockIOStats(stats *types.StatsJSON) (readBytes, writeBytes float64) {
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		op := strings.ToLower(entry.Op)
		switch op {
		case "read":
			readBytes += float64(entry.Value)
		case "write":
			writeBytes += float64(entry.Value)
		}
	}
	return readBytes, writeBytes
}
