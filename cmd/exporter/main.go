package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docker-exporter/internal/collector"
	"docker-exporter/internal/discovery"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version   = "1.0.0"
	startTime = time.Now()
)

type Config struct {
	Port              string
	DiscoveryInterval time.Duration
}

func loadConfig() Config {
	port := os.Getenv("EXPORTER_PORT")
	if port == "" {
		port = "8082"
	}

	intervalStr := os.Getenv("DISCOVERY_INTERVAL")
	if intervalStr == "" {
		intervalStr = "15s"
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Printf("[Config] Invalid DISCOVERY_INTERVAL '%s', defaulting to 15s", intervalStr)
		interval = 15 * time.Second
	}

	return Config{
		Port:              port,
		DiscoveryInterval: interval,
	}
}

func main() {
	cfg := loadConfig()
	log.Printf("==================================================")
	log.Printf("Starting Docker Auto-Discovery Prometheus Exporter")
	log.Printf("Version:            %s", version)
	log.Printf("Port:               %s", cfg.Port)
	log.Printf("Discovery Interval: %v", cfg.DiscoveryInterval)
	log.Printf("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize thread-safe metric store
	store := collector.NewInMemoryStore()

	// Initialize Docker Discovery Reconciler
	reconciler, err := discovery.NewReconciler(store, cfg.DiscoveryInterval)
	if err != nil {
		log.Fatalf("[Init] Failed to initialize Docker client: %v", err)
	}

	// Initialize Docker Events Watcher
	eventsWatcher := discovery.NewEventsWatcher(reconciler, store)

	// Start background processes
	go reconciler.Start(ctx)
	go eventsWatcher.Start(ctx)

	// Setup Prometheus Registry
	reg := prometheus.NewRegistry()

	// Include standard Go runtime and process metrics (matching existing environment)
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collector.NewDockerCollector(store),
	)

	// HTTP Multiplexer
	mux := http.NewServeMux()

	// /metrics endpoint
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// /health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		total, running := store.Count()
		isConnected := reconciler.IsConnected()

		healthStatus := "healthy"
		statusCode := http.StatusOK
		if !isConnected {
			healthStatus = "degraded"
		}

		resp := map[string]interface{}{
			"status":             healthStatus,
			"docker_connected":   isConnected,
			"containers_total":   total,
			"containers_running": running,
			"uptime_seconds":     int(time.Since(startTime).Seconds()),
			"version":            version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// / (root) landing page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		total, running := store.Count()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Docker Prometheus Exporter</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 40px; background: #f8fafc; color: #1e293b; }
        .card { background: white; padding: 24px; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); max-width: 600px; }
        h1 { margin-top: 0; color: #0284c7; }
        a { color: #0284c7; text-decoration: none; font-weight: bold; }
        a:hover { text-decoration: underline; }
        .badge { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; background: #e0f2fe; color: #0369a1; }
        ul { line-height: 1.8; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Docker Prometheus Exporter <span class="badge">v%s</span></h1>
        <p>Auto-discovery Prometheus exporter for Docker containers.</p>
        <p><strong>Status:</strong> Docker Connected: <code>%t</code> | Containers: <code>%d total (%d running)</code></p>
        <hr/>
        <ul>
            <li><a href="/metrics">/metrics</a> - Prometheus exposition format</li>
            <li><a href="/health">/health</a> - Healthcheck JSON status</li>
        </ul>
    </div>
</body>
</html>`, version, reconciler.IsConnected(), total, running)
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server runner
	go func() {
		log.Printf("[HTTP] Exporter listening on http://0.0.0.0:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[HTTP] Server fatal error: %v", err)
		}
	}()

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("[Shutdown] Received signal %v, terminating gracefully...", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Shutdown] HTTP server shutdown error: %v", err)
	}

	cancel() // Stop discovery reconciler and events watcher
	log.Println("[Shutdown] Exporter stopped cleanly.")
}
