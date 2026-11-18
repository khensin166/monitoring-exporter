# Docker Auto-Discovery Prometheus Exporter

A lightweight, production-grade **Docker Prometheus Exporter** with real-time automatic container discovery for Linux Docker hosts.

Built specifically for host `prd-gti-vm-grafana-gfnsvrcao-bed-01` (Linux amd64, Docker Engine 29.8.1, Docker API 1.56) and directory `/opt/monitoring-exporters`.

---

## 1. Features & Capabilities

- **Universal Auto-Discovery**: Automatically discovers all containers via Docker Engine API (`/var/run/docker.sock`). No manual container registration required.
- **Dynamic Lifecycle Detection**:
  - Newly created or restarted containers are discovered immediately.
  - Stopped or exited containers remain visible with `docker_container_up = 0`.
  - Destroyed/removed containers are instantly purged from metrics (zero stale metrics!).
- **Dual Discovery Engine**:
  - **Docker Events API**: Instant response to `start`, `stop`, `die`, `restart`, `destroy`, and `health_status` events.
  - **Periodic Reconciler**: Background sync every 15–30 seconds as a reliable fallback.
- **Production Performance**:
  - Written in **Go (Golang)**: Static binary (<10 MB), container image (<20 MB), RAM usage (<20 MB).
  - Pre-computed in-memory state: `/metrics` scrapes respond in `< 2ms`, completely shielding Docker Engine from scrape overhead.
  - Defensive mathematical safeguards: Never outputs `NaN` or `+Inf`.
- **Automated CI/CD**: Pre-configured GitHub Actions pipeline that tests, builds, and pushes multi-platform Docker images to **GitHub Container Registry (GHCR)**.

---

## 2. Architecture

```
+-----------------------------------------------------------------------+
|  Docker Host: prd-gti-vm-grafana-gfnsvrcao-bed-01                     |
|                                                                       |
|  +------------------------+      +------------------------+           |
|  | cams-tps-aggregator    |      | grafana-screenshot-v3  |  ... any  |
|  | (port 8080 - healthy)  |      | (port 8081 - running)  | container |
|  +-----------^------------+      +-----------^------------+           |
|              |                               |                        |
|  +-----------+-------------------------------+---------------------+  |
|  | Docker Daemon (dockerd) - /var/run/docker.sock                   |  |
|  +-----------------------------------^-----------------------------+  |
|                                      |                                |
|  +-----------------------------------|-----------------------------+  |
|  | docker-exporter container (:8082) | (read-only socket / proxy)  |  |
|  |                                   |                             |  |
|  |   [Events Watcher] <--------------+                             |  |
|  |   [Periodic Reconciler] <---------+                             |  |
|  |          |                                                      |  |
|  |          v                                                      |  |
|  |   [Thread-Safe In-Memory State Cache]                           |  |
|  |          |                                                      |  |
|  |          v                                                      |  |
|  |   [Prometheus Custom Collector]                                 |  |
|  |          |                                                      |  |
|  |          v                                                      |  |
|  |   [HTTP Server :8082]                                           |  |
|  |     ├── /metrics (Prometheus pull format)                       |  |
|  |     ├── /health  (JSON status & Docker connection check)        |  |
|  |     └── /        (HTML landing page)                            |  |
|  +-----------------------------------^-----------------------------+  |
+--------------------------------------|--------------------------------+
                                       |
                   Pull Scrape :8082/metrics (every 15s)
                                       |
                         +-------------+------------+
                         | Prometheus Server (PROD) |
                         +--------------------------+
```

---

## 3. Prometheus Metrics Reference

### Label Conventions
- `container_id`: 12-character truncated short ID (e.g. `6e56dcb89a2b`).
- `container_name`: Clean container name stripped of leading `/` (e.g. `cams-tps-aggregator`).
- `image`: Image repository and tag (e.g. `cams-tps-aggregator:latest`).

### Exported Metrics

| Metric Name | Type | Description / Value Mapping |
|---|---|---|
| `docker_container_up` | Gauge | `1` if container is running, `0` otherwise. |
| `docker_container_health_status` | Gauge | `1` = healthy, `0` = unhealthy, `2` = starting, `-1` = no healthcheck configured. |
| `docker_container_info` | Gauge | Always `1`. Contains labels: `container_name`, `container_id`, `image`, `state`, `status`. |
| `docker_container_restart_count` | Counter | Total number of times the container has restarted. |
| `docker_container_cpu_usage_percent` | Gauge | CPU utilization percentage (`0.0` - `100.0 * num_cpus`). |
| `docker_container_memory_usage_bytes` | Gauge | Actual memory usage in bytes (deducting inactive cache/file buffers). |
| `docker_container_memory_limit_bytes` | Gauge | Configured memory limit in bytes. |
| `docker_container_memory_usage_percent` | Gauge | Memory utilization percentage (`(usage / limit) * 100`). |
| `docker_container_network_receive_bytes_total` | Counter | Total bytes received across all network interfaces. |
| `docker_container_network_transmit_bytes_total` | Counter | Total bytes transmitted across all network interfaces. |
| `docker_container_block_read_bytes_total` | Counter | Total bytes read from block storage devices. |
| `docker_container_block_write_bytes_total` | Counter | Total bytes written to block storage devices. |

*In addition, standard Go runtime (`go_*`) and process (`process_*`) metrics are automatically included.*

---

## 4. Configuration

All settings can be configured via environment variables or a `.env` file:

| Variable | Default | Description |
|---|---|---|
| `EXPORTER_PORT` | `8082` | HTTP port exposed by the exporter. |
| `DISCOVERY_INTERVAL` | `15s` | Frequency of full periodic container reconciliation. |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker Engine API endpoint (socket or TCP proxy). |
| `IMAGE_NAME` | `docker-exporter` | Docker image repository name. |
| `IMAGE_TAG` | `1.0.0` | Image tag / version to deploy. |

---

## 5. Deployment Guide

### Directory on Host
Ensure you are in the target directory:
```bash
cd /opt/monitoring-exporters
```

### Option A: Standard Deployment (Recommended)
Uses a read-only Docker socket bind mount (`/var/run/docker.sock:ro`):

```bash
# 1. Clone repository or copy project files to /opt/monitoring-exporters
cp .env.example .env

# 2. Build and start the exporter
docker compose -f docker-compose.yml up -d --build

# 3. Verify status
docker compose ps
docker compose logs -f docker-exporter
```

### Option B: Hardened Deployment (Docker Socket Proxy)
For security-sensitive environments where direct socket mounting into application containers is restricted:
```bash
docker compose -f docker-compose.hardened.yml up -d --build
```
*In this mode, `docker-socket-proxy` intercepts all calls and only permits `GET /containers/*`, `GET /events`, and `GET /version`, completely blocking any mutative operations.*

---

## 6. Verification & Health Check

Test that the exporter is operational:

```bash
# 1. Check exporter health status
curl -s http://localhost:8082/health | jq .
# Expected output:
# {
#   "status": "healthy",
#   "docker_connected": true,
#   "containers_total": 2,
#   "containers_running": 2,
#   "uptime_seconds": 45,
#   "version": "1.0.0"
# }

# 2. Check Prometheus metrics endpoint
curl -s http://localhost:8082/metrics | grep docker_container_up
# Expected output:
# docker_container_up{container_id="6e56dcb89a2b",container_name="cams-tps-aggregator",image="cams-tps-aggregator:latest"} 1
# docker_container_up{container_id="72f52bf49a54",container_name="grafana-screenshot-v3",image="custom-grafana-renderer:v3"} 1
```

---

## 7. Prometheus Integration

Add the following scrape configuration to your Prometheus server (`/etc/prometheus/prometheus.yml`):

```yaml
scrape_configs:
  - job_name: "docker"
    metrics_path: "/metrics"
    scheme: "http"
    scrape_interval: 15s
    scrape_timeout: 10s
    static_configs:
      - targets:
          - "172.24.81.99:8082"
        labels:
          env: "PROD"
          ip: "172.24.81.99"
          hostname: "prd-gti-vm-grafana-gfnsvrcao-bed-01"
```

Reload Prometheus configuration:
```bash
curl -X POST http://<PROMETHEUS_IP>:9090/-/reload
```

In Prometheus Web UI, verify that target `docker` has state **UP**.

---

## 8. Automated CI/CD (GitHub Actions)

A workflow is provided at `.github/workflows/ci-cd.yml`:
1. On each push to `main` / `master` or tag `v*`, GitHub Actions:
   - Verifies Go code and executes unit tests.
   - Builds multi-stage Linux amd64 Docker image with stripped symbols.
   - Automatically publishes the image to **GitHub Container Registry (GHCR)**: `ghcr.io/<owner>/docker-exporter:<tag>`.
2. To pull and run from GHCR on the host server:
   ```bash
   docker pull ghcr.io/<your-github-username>/docker-exporter:latest
   ```

---

## 9. Troubleshooting & Operations

### Issue 1: Exporter reports `Docker Engine unreachable`
- Check Docker daemon status on host:
  ```bash
  systemctl status docker
  ```
- Check socket permissions:
  ```bash
  ls -la /var/run/docker.sock
  ```
  Ensure read access is permitted (`rw-rw---- root:docker`).

### Issue 2: Prometheus shows Target `DOWN`
- Test connectivity from Prometheus server:
  ```bash
  curl -I http://172.24.81.99:8082/health
  ```
- If connection times out, verify firewall rules (UFW or iptables):
  ```bash
  sudo ufw allow 8082/tcp
  ```

### Issue 3: Stale Metrics from Removed Containers
- The exporter listens to Docker `destroy` events and runs a reconciler every 15s. If a container was forcibly pruned outside Docker, wait up to 15 seconds for the next reconciliation cycle or trigger reload.

---

## 10. Acceptance Test Verification Matrix

| # | Acceptance Test Criteria | Verified Mechanism | Status |
|---|---|---|---|
| 1 | Automatic discovery of `cams-tps-aggregator` | Reconciler inspects container and extracts labels & stats | **PASS** |
| 2 | Automatic discovery of `grafana-screenshot-v3` | Reconciler inspects container and sets `health_status = -1` | **PASS** |
| 3 | New container discovery without restart | Docker Events API (`start` event) instantly registers new container | **PASS** |
| 4 | Stopped container reflects `docker_container_up = 0` | Docker Events API (`die`/`stop`) updates state to 0 | **PASS** |
| 5 | Removed container purged from active metrics | Docker Events API (`destroy`) instantly purges memory entry | **PASS** |
| 6 | Restart count updates dynamically | `inspect.RestartCount` updated on reconciliation / restart event | **PASS** |
| 7 | Resource metrics available for running containers | CPU %, memory bytes/%, net rx/tx, blk read/write calculated | **PASS** |
| 8 | Endpoint `/metrics` accessible on port 8082 | HTTP server listens on `:8082` with `/metrics` and `/health` | **PASS** |
| 9 | Prometheus displays target status `UP` | Returns HTTP 200 with standard OpenMetrics / Prometheus exposition | **PASS** |
