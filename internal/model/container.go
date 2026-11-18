package model

import "time"

// ContainerMetrics holds the current status and resource utilization metrics for a container.
type ContainerMetrics struct {
	ID                 string    // 12-character truncated ID
	FullID             string    // Full 64-character container ID
	Name               string    // Sanitized container name (no leading slash)
	Image              string    // Container image name and tag
	State              string    // Container state (e.g., "running", "exited", "dead")
	Status             string    // Human-readable status (e.g., "Up 3 days (healthy)")
	Up                 float64   // 1 if running, 0 otherwise
	HealthStatus       float64   // 1=healthy, 0=unhealthy, 2=starting, -1=no healthcheck
	RestartCount       float64   // Number of restarts
	CPUPercent         float64   // CPU utilization percentage
	MemoryUsageBytes   float64   // Memory used in bytes (excluding inactive cache)
	MemoryLimitBytes   float64   // Configured memory limit in bytes
	MemoryUsagePercent float64   // Memory utilization percentage
	NetworkRxBytes     float64   // Total bytes received across all network interfaces
	NetworkTxBytes     float64   // Total bytes transmitted across all network interfaces
	BlockReadBytes     float64   // Total block I/O read bytes
	BlockWriteBytes    float64   // Total block I/O write bytes
	LastUpdated        time.Time // Timestamp of last metric update
}

// CPUState stores historical CPU stats for calculating deltas.
type CPUState struct {
	TotalUsage  uint64
	SystemUsage uint64
	OnlineCPUs  uint32
}

// HealthStatusMapping maps Docker health string to numeric Prometheus gauge value.
// 1 = healthy, 0 = unhealthy, 2 = starting, -1 = none / unknown
func ParseHealthStatus(status string) float64 {
	switch status {
	case "healthy":
		return 1.0
	case "unhealthy":
		return 0.0
	case "starting":
		return 2.0
	default:
		return -1.0
	}
}
