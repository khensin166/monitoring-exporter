module docker-exporter

go 1.21

require (
	github.com/docker/docker v24.0.9+incompatible
	github.com/prometheus/client_golang v1.19.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

replace (
	github.com/distribution/reference => github.com/distribution/reference v0.5.0
	github.com/docker/go-connections => github.com/docker/go-connections v0.5.0
	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.9.3
	golang.org/x/time => golang.org/x/time v0.5.0
)
