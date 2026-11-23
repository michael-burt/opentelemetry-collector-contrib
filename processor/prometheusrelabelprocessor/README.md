# Prometheus metric relabeling processor

The development prototype applies Prometheus metric relabeling rules to OTLP
metrics. Use it on a metrics pipeline when the destination consumes OTLP or
when other processors need the relabeled data. For a Remote Write destination,
`write_relabel_configs` on the exporter can apply the rules directly to the
exported series.

```yaml
processors:
  prometheus_relabel:
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'agent_debug_.+'
        action: drop
      - source_labels: [cluster_name]
        regex: '(.+)'
        target_label: cluster
        replacement: '$$1'
```

Use `$$` for a literal dollar in the Collector configuration. The rule evaluator
receives `$1` in this example.

Rules see translated point attributes, the translated metric name, and classic
histogram or summary role labels. The processor keeps OTLP values, timestamps,
exemplars, and resource and scope metadata. A rename preserves the metric's unit
and type. An exporter may apply its naming strategy to the resulting OTLP name.

An empty rule list forwards the original batch. With rules configured, the
prototype supports gauges, cumulative monotonic sums, cumulative explicit
histograms, and summaries. Partial family drops and inconsistent family edits
reject the affected point. The processor counts rule drops and rejected inputs in
`otelcol_processor_prometheus_relabel_dropped_data_points` with a bounded
`reason` attribute. It detects stream and metric identity conflicts within a
batch.

See [the implementation plan](IMPLEMENTATION_PLAN.md) and the
[benchmark build](../../examples/prometheus-relabel/README.md).
