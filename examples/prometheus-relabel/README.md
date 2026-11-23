# Prometheus relabeling benchmark Collector

This distribution contains an OTLP receiver, the `prometheus_relabel`
processor, and the Remote Write exporter with `write_relabel_configs`.

From the repository root, build an amd64 Linux image with Go and Docker.

```sh
examples/prometheus-relabel/build-image.sh relabel-collector:dev
docker run --rm --platform linux/amd64 \
  --tmpfs /tmp:mode=1777 --entrypoint /relabelbench relabel-collector:dev \
  -repeats=1 -batches=2 -points=16
```

Run `go test -run '^$' -bench BenchmarkPipeline -benchmem ./internal/workload`
from this directory to measure allocations and v1 series construction on the
host. The container harness sends OTLP HTTP requests to the real Collector.
It decodes the first batch of Remote Write requests for each configuration and
verifies samples, names, labels, and timestamps before timing the local sink.
The sink discards bodies during timing and returns the expected v2 sample count.
The harness exits with an error if it finds unexpected output or request counts.

Use [benchmark-job.yaml](benchmark-job.yaml) on an amd64 Kubernetes cluster.
Set the image to a pullable digest and configure an image pull secret if the
registry requires one. Capture Job logs as JSON Lines before the Job expires.
The Collector and sink use loopback within the Job. It needs no Service or
Kubernetes API permissions.

See [the performance report](PERFORMANCE.md) for the measured results.
