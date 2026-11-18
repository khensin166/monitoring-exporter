package discovery

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"docker-exporter/internal/collector"
	"docker-exporter/internal/model"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// Reconciler manages discovery and stats collection from Docker Engine.
type Reconciler struct {
	cli             *client.Client
	store           *collector.InMemoryStore
	interval        time.Duration
	dockerConnected atomic.Bool
	containersMon   atomic.Int64
}

// NewReconciler creates a new Discovery Reconciler.
func NewReconciler(store *collector.InMemoryStore, interval time.Duration) (*Reconciler, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	r := &Reconciler{
		cli:      cli,
		store:    store,
		interval: interval,
	}
	return r, nil
}

// Client returns the underlying Docker client.
func (r *Reconciler) Client() *client.Client {
	return r.cli
}

// IsConnected returns whether Docker Engine is currently reachable.
func (r *Reconciler) IsConnected() bool {
	return r.dockerConnected.Load()
}

// Start runs the initial discovery and periodic reconciliation loop.
func (r *Reconciler) Start(ctx context.Context) {
	log.Printf("[Discovery] Starting periodic reconciler with interval: %v", r.interval)

	// Perform initial discovery
	r.Reconcile(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Discovery] Shutting down reconciler...")
			return
		case <-ticker.C:
			r.Reconcile(ctx)
		}
	}
}

// Reconcile performs a complete scan of all containers and syncs state.
func (r *Reconciler) Reconcile(ctx context.Context) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 1. Verify connection
	_, err := r.cli.Ping(reqCtx)
	if err != nil {
		r.dockerConnected.Store(false)
		log.Printf("[Discovery] Warning: Docker Engine unreachable: %v", err)
		return
	}
	r.dockerConnected.Store(true)

	// 2. List all containers (running and stopped)
	containers, err := r.cli.ContainerList(reqCtx, types.ContainerListOptions{All: true})
	if err != nil {
		log.Printf("[Discovery] Error listing containers: %v", err)
		return
	}

	activeIDs := make(map[string]bool, len(containers))

	for _, ctr := range containers {
		shortID := ShortID(ctr.ID)
		activeIDs[shortID] = true
		r.processContainer(ctx, ctr.ID, shortID, ctr.Names, ctr.Image, ctr.State, ctr.Status)
	}

	// 3. Purge deleted containers from memory to eliminate stale metrics
	r.store.CleanStaleContainers(activeIDs)

	total, running := r.store.Count()
	r.containersMon.Store(int64(total))
	log.Printf("[Discovery] Reconciled %d containers (%d running, %d stopped)", total, running, total-running)
}

// ProcessSingleContainer updates a single container immediately (used by events watcher).
func (r *Reconciler) ProcessSingleContainer(ctx context.Context, fullID string) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	inspect, err := r.cli.ContainerInspect(reqCtx, fullID)
	if err != nil {
		// If container was removed between event and inspect, delete from store
		if client.IsErrNotFound(err) {
			r.store.DeleteContainer(ShortID(fullID))
		}
		return
	}

	shortID := ShortID(inspect.ID)
	names := []string{inspect.Name}
	state := inspect.State.Status
	status := inspect.State.Status
	if inspect.State.Running {
		status = "Up"
		if inspect.State.Health != nil {
			status = fmt.Sprintf("Up (%s)", inspect.State.Health.Status)
		}
	} else if inspect.State.Dead {
		status = "Dead"
	} else if inspect.State.Restarting {
		status = "Restarting"
	} else {
		status = fmt.Sprintf("Exited (%d)", inspect.State.ExitCode)
	}

	r.processContainer(ctx, inspect.ID, shortID, names, inspect.Config.Image, state, status)
}

func (r *Reconciler) processContainer(ctx context.Context, fullID, shortID string, names []string, image, state, status string) {
	cleanName := SanitizeContainerName(names)

	// Determine UP status
	isUp := 0.0
	if state == "running" {
		isUp = 1.0
	}

	// Inspect container for detailed state, restart count, and healthcheck
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 4*time.Second)
	defer inspectCancel()

	inspect, err := r.cli.ContainerInspect(inspectCtx, fullID)
	if err != nil {
		log.Printf("[Discovery] Error inspecting container %s: %v", shortID, err)
		return
	}

	// Health status calculation
	healthVal := -1.0 // default: no healthcheck
	if inspect.State.Health != nil {
		healthVal = model.ParseHealthStatus(inspect.State.Health.Status)
	}

	restartCount := float64(inspect.RestartCount)

	// Prepare metrics model
	m := &model.ContainerMetrics{
		ID:           shortID,
		FullID:       fullID,
		Name:         cleanName,
		Image:        image,
		State:        state,
		Status:       status,
		Up:           isUp,
		HealthStatus: healthVal,
		RestartCount: restartCount,
		LastUpdated:  time.Now(),
	}

	// If running, collect resource stats
	if isUp == 1.0 {
		statsCtx, statsCancel := context.WithTimeout(ctx, 4*time.Second)
		defer statsCancel()

		statsResp, err := r.cli.ContainerStatsOneShot(statsCtx, fullID)
		if err == nil && statsResp.Body != nil {
			defer statsResp.Body.Close()
			statsBytes, readErr := io.ReadAll(statsResp.Body)
			if readErr == nil {
				prevCPU := r.store.GetCPUState(shortID)
				cpuPct, memUsage, memLimit, memPct, netRx, netTx, blkRead, blkWrite, newCPU :=
					collector.CalculateStats(statsBytes, prevCPU)

				m.CPUPercent = cpuPct
				m.MemoryUsageBytes = memUsage
				m.MemoryLimitBytes = memLimit
				m.MemoryUsagePercent = memPct
				m.NetworkRxBytes = netRx
				m.NetworkTxBytes = netTx
				m.BlockReadBytes = blkRead
				m.BlockWriteBytes = blkWrite

				if newCPU != nil {
					r.store.SetCPUState(shortID, newCPU)
				}
			}
		}
	}

	r.store.SetContainer(shortID, m)
}

// SanitizeContainerName removes leading slashes and handles empty name slices.
func SanitizeContainerName(names []string) string {
	if len(names) == 0 || names[0] == "" {
		return "unnamed"
	}
	return strings.TrimPrefix(names[0], "/")
}

// ShortID truncates a 64-character container ID to 12 characters.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
