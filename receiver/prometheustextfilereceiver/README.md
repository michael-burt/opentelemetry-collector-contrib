# Prometheus Textfile Receiver

| Status                   |           |
| ------------------------ |-----------|
| Stability                | [development]  |
| Supported pipeline types | metrics   |
| Distributions            | [contrib] |

The Prometheus Textfile receiver reads Prometheus metric files from one or more directories, similar to the [Node Exporter Textfile Collector](https://github.com/prometheus/node_exporter#textfile-collector).

This receiver allows you to expose metrics via files in the Prometheus text format. This is particularly useful for generating metrics from shell scripts, cron jobs, or other processes that can write metrics to files but cannot expose HTTP endpoints.

## Configuration

The following settings are required:

- `directories`: A list of directories to search for `.prom` files. Supports glob patterns.

The following settings are optional:

- `collection_interval` (optional, default = 60s): This receiver collects metrics on an interval. Valid time units are ns, us (or µs), ms, s, m, h.
- `initial_delay` (optional, default = 1s): defines how long this receiver waits before starting.

### Example Configuration

```yaml
receivers:
  prometheustextfile:
    directories: 
      - /opt/metrics/textfiles/
    collection_interval: 30s
```

## Metrics

The Prometheus Textfile receiver converts all metrics from the text files using the same format as the Prometheus Node Exporter textfile collector. It preserves all metric types and labels.

In addition to the metrics from the files, the receiver generates:

- `prometheustextfile.file.mtime`: Unixtime mtime of textfiles successfully read (with a `file.path` label for the absolute file path)

## Usage

### Generating Compatible Metric Files

To generate metrics for the textfile receiver, create files with the `.prom` extension in the configured directories. Files must follow the [Prometheus text exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/#text-based-format).

Example of a valid `.prom` file:

```
# HELP node_role Node Role of the server
# TYPE node_role gauge
node_role{node_role="application-server"} 1
```

## Troubleshooting

Common issues with the Prometheus Textfile receiver:

- Files must have the `.prom` extension to be read
- Files with invalid Prometheus format are skipped and an error is logged
- Files containing client-side timestamps are rejected (timestamps are added by the receiver)
- Directory paths that don't exist will generate errors in the logs
- Ensure the OpenTelemetry Collector has read permissions for the directories and files 