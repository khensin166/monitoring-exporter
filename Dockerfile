# ==============================================================================
# Build Stage
# ==============================================================================
FROM golang:1.21-alpine3.19 AS builder

WORKDIR /src

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build static binary for linux/amd64 with symbols stripped for minimal size
ARG VERSION=1.0.0
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION} -extldflags '-static'" \
    -o /bin/docker-exporter \
    ./cmd/exporter

# ==============================================================================
# Production Stage
# ==============================================================================
FROM alpine:3.19

# Install security updates and certificates
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user (UID 10001)
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

# Copy compiled binary from builder
COPY --from=builder /bin/docker-exporter /usr/local/bin/docker-exporter

# Expose default exporter port
EXPOSE 9209

# Default environment variables
ENV EXPORTER_PORT=9209 \
    DISCOVERY_INTERVAL=15s \
    DOCKER_HOST=unix:///var/run/docker.sock

# Healthcheck
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9209/health || exit 1

# Entrypoint
ENTRYPOINT ["/usr/local/bin/docker-exporter"]
