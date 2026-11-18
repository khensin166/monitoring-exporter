package collector

import (
	"encoding/json"
	"math"
	"testing"

	"docker-exporter/internal/model"

	"github.com/docker/docker/api/types"
)

func TestCalculateCPUPercent(t *testing.T) {
	stats := &types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage:  200000000,
					PercpuUsage: []uint64{100000000, 100000000},
				},
				SystemUsage: 2000000000,
				OnlineCPUs:  2,
			},
		},
	}

	prev := &model.CPUState{
		TotalUsage:  100000000,
		SystemUsage: 1000000000,
		OnlineCPUs:  2,
	}

	// cpuDelta = 100,000,000
	// sysDelta = 1,000,000,000
	// cpuPercent = (100,000,000 / 1,000,000,000) * 2 * 100 = 20.0%
	pct, next := calculateCPUPercent(stats, prev)
	if math.Abs(pct-20.0) > 0.001 {
		t.Fatalf("expected 20.0%% CPU, got %f", pct)
	}
	if next == nil || next.TotalUsage != 200000000 {
		t.Fatalf("expected next CPU state to have total usage 200000000")
	}
}

func TestCalculateCPUPercent_ZeroDelta(t *testing.T) {
	stats := &types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage: 100000,
				},
				SystemUsage: 100000,
				OnlineCPUs:  1,
			},
		},
	}

	// Identical previous state -> systemDelta = 0
	prev := &model.CPUState{
		TotalUsage:  100000,
		SystemUsage: 100000,
		OnlineCPUs:  1,
	}

	pct, _ := calculateCPUPercent(stats, prev)
	if pct != 0.0 || math.IsNaN(pct) || math.IsInf(pct, 0) {
		t.Fatalf("expected 0.0, got %f (NaN=%v, Inf=%v)", pct, math.IsNaN(pct), math.IsInf(pct, 0))
	}
}

func TestCalculateMemoryStats(t *testing.T) {
	stats := &types.StatsJSON{
		Stats: types.Stats{
			MemoryStats: types.MemoryStats{
				Usage: 500 * 1024 * 1024,  // 500 MB
				Limit: 1000 * 1024 * 1024, // 1000 MB
				Stats: map[string]uint64{
					"inactive_file": 100 * 1024 * 1024, // 100 MB cache
				},
			},
		},
	}

	usage, limit, pct := calculateMemoryStats(stats)

	expectedUsage := float64(400 * 1024 * 1024)
	if usage != expectedUsage {
		t.Fatalf("expected usage %f, got %f", expectedUsage, usage)
	}

	expectedLimit := float64(1000 * 1024 * 1024)
	if limit != expectedLimit {
		t.Fatalf("expected limit %f, got %f", expectedLimit, limit)
	}

	if math.Abs(pct-40.0) > 0.001 {
		t.Fatalf("expected 40%%, got %f", pct)
	}
}

func TestCalculateNetworkAndBlockStats(t *testing.T) {
	statsJSON := types.StatsJSON{
		Networks: map[string]types.NetworkStats{
			"eth0": {RxBytes: 1500, TxBytes: 3000},
			"eth1": {RxBytes: 500, TxBytes: 1000},
		},
		Stats: types.Stats{
			BlkioStats: types.BlkioStats{
				IoServiceBytesRecursive: []types.BlkioStatEntry{
					{Op: "Read", Value: 8192},
					{Op: "Write", Value: 16384},
					{Op: "read", Value: 2048},
				},
			},
		},
	}

	data, err := json.Marshal(statsJSON)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	_, _, _, _, netRx, netTx, blkRead, blkWrite, _ := CalculateStats(data, nil)

	if netRx != 2000 {
		t.Fatalf("expected netRx 2000, got %f", netRx)
	}
	if netTx != 4000 {
		t.Fatalf("expected netTx 4000, got %f", netTx)
	}
	if blkRead != 10240 {
		t.Fatalf("expected blkRead 10240, got %f", blkRead)
	}
	if blkWrite != 16384 {
		t.Fatalf("expected blkWrite 16384, got %f", blkWrite)
	}
}
