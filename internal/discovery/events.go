package discovery

import (
	"context"
	"log"
	"time"

	"docker-exporter/internal/collector"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// EventsWatcher listens to Docker Events API in real-time.
type EventsWatcher struct {
	reconciler *Reconciler
	store      *collector.InMemoryStore
}

// NewEventsWatcher initializes an EventsWatcher.
func NewEventsWatcher(reconciler *Reconciler, store *collector.InMemoryStore) *EventsWatcher {
	return &EventsWatcher{
		reconciler: reconciler,
		store:      store,
	}
}

// Start begins the event stream listening loop with auto-reconnection.
func (w *EventsWatcher) Start(ctx context.Context) {
	log.Println("[Events] Starting Docker real-time event watcher...")

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("[Events] Shutting down event watcher...")
			return
		default:
		}

		err := w.streamEvents(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("[Events] Stream disconnected (%v). Reconnecting in %v...", err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

func (w *EventsWatcher) streamEvents(ctx context.Context) error {
	eventFilters := filters.NewArgs()
	eventFilters.Add("type", "container")

	msgChan, errChan := w.reconciler.Client().Events(ctx, types.EventsOptions{
		Filters: eventFilters,
	})

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errChan:
			return err
		case msg := <-msgChan:
			w.handleEvent(ctx, msg)
		}
	}
}

func (w *EventsWatcher) handleEvent(ctx context.Context, msg events.Message) {
	action := msg.Action
	containerID := msg.Actor.ID
	shortID := ShortID(containerID)

	containerName := msg.Actor.Attributes["name"]
	if containerName == "" {
		containerName = shortID
	}

	switch action {
	case "start", "restart", "unpause":
		log.Printf("[Events] Detected %s for container %s (%s) -> updating immediately", action, containerName, shortID)
		w.reconciler.ProcessSingleContainer(ctx, containerID)

	case "die", "stop", "pause":
		log.Printf("[Events] Detected %s for container %s (%s) -> marking Up=0", action, containerName, shortID)
		w.reconciler.ProcessSingleContainer(ctx, containerID)

	case "destroy":
		log.Printf("[Events] Detected destroy for container %s (%s) -> purging from metrics", containerName, shortID)
		w.store.DeleteContainer(shortID)

	default:
		// Health status change event e.g. "health_status: healthy"
		if len(action) > 14 && action[:14] == "health_status:" {
			log.Printf("[Events] Detected %s for container %s (%s)", action, containerName, shortID)
			w.reconciler.ProcessSingleContainer(ctx, containerID)
		}
	}
}
