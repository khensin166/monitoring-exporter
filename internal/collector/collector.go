package collector

import (
	"sync"

	"docker-exporter/internal/model"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricStore defines the interface for accessing current container state.
type MetricStore interface {
	GetAllContainers() map[string]*model.ContainerMetrics
}

// DockerCollector implements the prometheus.Collector interface.
type DockerCollector struct {
	store MetricStore

	upDesc            *prometheus.Desc
	healthDesc        *prometheus.Desc
	infoDesc          *prometheus.Desc
	restartDesc       *prometheus.Desc
	cpuPercentDesc    *prometheus.Desc
	memUsageDesc      *prometheus.Desc
	memLimitDesc      *prometheus.Desc
	memPercentDesc    *prometheus.Desc
	netRxDesc         *prometheus.Desc
	netTxDesc         *prometheus.Desc
	blockReadDesc     *prometheus.Desc
	blockWriteDesc    *prometheus.Desc
}

// NewDockerCollector initializes a new DockerCollector with pre-allocated metric descriptors.
func NewDockerCollector(store MetricStore) *DockerCollector {
	labels := []string{"container_name", "container_id", "image"}
	infoLabels := []string{"container_name", "container_id", "image", "state", "status"}

	return &DockerCollector{
		store: store,

		upDesc: prometheus.NewDesc(
			"docker_container_up",
			"1 if container is currently running, 0 otherwise.",
			labels, nil,
		),
		healthDesc: prometheus.NewDesc(
			"docker_container_health_status",
			"Healthcheck status of the container (1=healthy, 0=unhealthy, 2=starting, -1=no healthcheck).",
			labels, nil,
		),
		infoDesc: prometheus.NewDesc(
			"docker_container_info",
			"Informational metric about the container with status and state labels.",
			infoLabels, nil,
		),
		restartDesc: prometheus.NewDesc(
			"docker_container_restart_count",
			"Total number of times the container has restarted.",
			labels, nil,
		),
		cpuPercentDesc: prometheus.NewDesc(
			"docker_container_cpu_usage_percent",
			"Current CPU usage percentage of the container.",
			labels, nil,
		),
		memUsageDesc: prometheus.NewDesc(
			"docker_container_memory_usage_bytes",
			"Current memory usage of the container in bytes (excluding inactive cache).",
			labels, nil,
		),
		memLimitDesc: prometheus.NewDesc(
			"docker_container_memory_limit_bytes",
			"Configured memory limit of the container in bytes.",
			labels, nil,
		),
		memPercentDesc: prometheus.NewDesc(
			"docker_container_memory_usage_percent",
			"Current memory usage percentage of the container.",
			labels, nil,
		),
		netRxDesc: prometheus.NewDesc(
			"docker_container_network_receive_bytes_total",
			"Total received bytes across all network interfaces for this container.",
			labels, nil,
		),
		netTxDesc: prometheus.NewDesc(
			"docker_container_network_transmit_bytes_total",
			"Total transmitted bytes across all network interfaces for this container.",
			labels, nil,
		),
		blockReadDesc: prometheus.NewDesc(
			"docker_container_block_read_bytes_total",
			"Total bytes read from block devices for this container.",
			labels, nil,
		),
		blockWriteDesc: prometheus.NewDesc(
			"docker_container_block_write_bytes_total",
			"Total bytes written to block devices for this container.",
			labels, nil,
		),
	}
}

// Describe sends the super-set of all descriptors of metrics the Collector can possibly export.
func (c *DockerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.upDesc
	ch <- c.healthDesc
	ch <- c.infoDesc
	ch <- c.restartDesc
	ch <- c.cpuPercentDesc
	ch <- c.memUsageDesc
	ch <- c.memLimitDesc
	ch <- c.memPercentDesc
	ch <- c.netRxDesc
	ch <- c.netTxDesc
	ch <- c.blockReadDesc
	ch <- c.blockWriteDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *DockerCollector) Collect(ch chan<- prometheus.Metric) {
	containers := c.store.GetAllContainers()

	for _, ctr := range containers {
		commonVals := []string{ctr.Name, ctr.ID, ctr.Image}

		// Status Metrics (Available for all containers, running or stopped)
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, ctr.Up, commonVals...)
		ch <- prometheus.MustNewConstMetric(c.healthDesc, prometheus.GaugeValue, ctr.HealthStatus, commonVals...)
		ch <- prometheus.MustNewConstMetric(c.restartDesc, prometheus.CounterValue, ctr.RestartCount, commonVals...)

		// Info Metric
		infoVals := []string{ctr.Name, ctr.ID, ctr.Image, ctr.State, ctr.Status}
		ch <- prometheus.MustNewConstMetric(c.infoDesc, prometheus.GaugeValue, 1.0, infoVals...)

		// Resource Utilization Metrics (Only emitted for running containers)
		if ctr.Up == 1.0 {
			ch <- prometheus.MustNewConstMetric(c.cpuPercentDesc, prometheus.GaugeValue, ctr.CPUPercent, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.memUsageDesc, prometheus.GaugeValue, ctr.MemoryUsageBytes, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.memLimitDesc, prometheus.GaugeValue, ctr.MemoryLimitBytes, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.memPercentDesc, prometheus.GaugeValue, ctr.MemoryUsagePercent, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.netRxDesc, prometheus.CounterValue, ctr.NetworkRxBytes, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.netTxDesc, prometheus.CounterValue, ctr.NetworkTxBytes, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.blockReadDesc, prometheus.CounterValue, ctr.BlockReadBytes, commonVals...)
			ch <- prometheus.MustNewConstMetric(c.blockWriteDesc, prometheus.CounterValue, ctr.BlockWriteBytes, commonVals...)
		}
	}
}

// InMemoryStore holds the thread-safe state of all discovered containers.
type InMemoryStore struct {
	mu         sync.RWMutex
	containers map[string]*model.ContainerMetrics
	cpuStates  map[string]*model.CPUState
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		containers: make(map[string]*model.ContainerMetrics),
		cpuStates:  make(map[string]*model.CPUState),
	}
}

// GetAllContainers returns a copy of the active containers map.
func (s *InMemoryStore) GetAllContainers() map[string]*model.ContainerMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*model.ContainerMetrics, len(s.containers))
	for k, v := range s.containers {
		copyMetric := *v
		result[k] = &copyMetric
	}
	return result
}

// SetContainer adds or updates a container's metrics.
func (s *InMemoryStore) SetContainer(id string, metrics *model.ContainerMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers[id] = metrics
}

// DeleteContainer removes a container from the store and its CPU state history.
func (s *InMemoryStore) DeleteContainer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.containers, id)
	delete(s.cpuStates, id)
}

// GetCPUState gets the previous CPU state for a container.
func (s *InMemoryStore) GetCPUState(id string) *model.CPUState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cpuStates[id]
}

// SetCPUState updates the CPU state history for a container.
func (s *InMemoryStore) SetCPUState(id string, cpu *model.CPUState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cpuStates[id] = cpu
}

// CleanStaleContainers keeps only containers present in activeIDs, removing destroyed ones.
func (s *InMemoryStore) CleanStaleContainers(activeIDs map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.containers {
		if !activeIDs[id] {
			delete(s.containers, id)
			delete(s.cpuStates, id)
		}
	}
}

// Count returns total and running container counts.
func (s *InMemoryStore) Count() (total int, running int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total = len(s.containers)
	for _, c := range s.containers {
		if c.Up == 1.0 {
			running++
		}
	}
	return total, running
}
