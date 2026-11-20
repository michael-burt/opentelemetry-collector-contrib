# Prometheus relabel processor proposal

Add a metrics processor with the component type `prometheus_relabel`. It takes OpenTelemetry metrics, converts them to Prometheus series in memory, runs Prometheus relabel rules, and converts the surviving series back to OpenTelemetry metrics.

The use case is running an existing Prometheus metric relabel policy in a Collector metrics pipeline. That policy might apply to metrics received over OTLP or to one branch of a pipeline that also serves other destinations. A processor can apply the rules after data enters the Collector and before the selected exporters.

## Configuration

The proposed `metric_relabel_configs` field takes the same ordered list of rules as Prometheus. Reuse Prometheus's `model/relabel` package for rule validation, defaults, and execution. Compile rules at startup and apply them to each converted series. Start with the `UnderscoreEscapingWithSuffixes` translation strategy so rules see Prometheus-normalized names, including type and unit suffixes, in `__name__`. An empty rule list passes the OTLP batch through directly. [Prometheus relabel configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config)

```yaml
processors:
  prometheus_relabel/metrics:
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'agent_debug_.+'
        action: drop
      - source_labels: [cluster_name]
        regex: '(.+)'
        target_label: cluster
        replacement: '$1'
        action: replace
```

The initial rules see the metric name, projected datapoint attributes, and series labels such as `le` and `quantile`. Resource and scope attributes stay in the OTLP envelope. Rules that use those fields, including policies that expect `job` and `instance` from resource attributes, need them copied to datapoint attributes upstream. The rules run at the processor's position in the pipeline, so their input excludes scrape discovery metadata and exporter-added labels. A received `up` series is eligible for relabeling like any other input series.

## Conversion and reconstruction

1. Convert the OTLP batch to in-memory Prometheus series with labels and samples. Reuse the existing Prometheus naming and sample conversion logic where it fits. Carry batch-local provenance alongside each series, including its original resource, scope, metric, point, and role in a split family. Keep distinct OTLP origins separate even when their projected labels match, and retain original values for fields that the Prometheus representation cannot preserve. [Outbound translator](../../pkg/translator/prometheusremotewrite/metrics_to_prw.go)
2. Run the rule list over each series label set. Mark dropped series and keep the result for each survivor. Compare the input and output labels so unchanged names and attributes can retain their original OTLP spelling and types. Write new or changed labels as string datapoint attributes. Consume `__name__` as the metric name and strip temporary `__tmp*` labels from the output.
3. Rebuild OTLP metrics under their original resource and scope. Retain the original metric name unless a rule changes it. Regroup points by the emitted metric name and compatible descriptor. Where a family uses split series, use their roles to reconstruct the distribution and require surviving members to agree on the output family name and non-structural labels.

The transport translators are useful reference points. The outbound Remote Write translator groups samples by Prometheus series identity. The inbound receiver relies on wire metadata and drops typed summaries and classic histograms. This processor needs its own in-memory reconstruction contract with access to the original batch. That also keeps serialization, compression, and network I/O out of the data path. [Remote Write receiver limitations](../../receiver/prometheusremotewritereceiver/README.md#known-limitations) · [OpenTelemetry and Prometheus compatibility](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/)

## Round-trip limits

- **Names and labels.** Prometheus translation can add suffixes, normalize keys, and stringify non-string attributes. Rules see those projected values. Distinct OTLP keys can map to the same Prometheus label. The first version should detect ambiguous projections before relabeling and report affected points. A downstream Prometheus exporter may apply its own name and label translation again.
- **Types and metadata.** Start with gauges, cumulative monotonic sums, cumulative explicit-bucket histograms, and summaries. Defer delta metrics and exponential histograms until their conversion behavior has coverage. Preserve descriptions, units, schema URLs, integer values, nanosecond timestamps, start times, flags, and exemplars from the original points when rebuilding a supported survivor. Prometheus sample values and timestamps alone can lose integer precision or sub-millisecond precision.
- **Histograms and summaries.** A classic histogram expands into `_bucket`, `_sum`, and `_count` series. A summary expands into quantile series plus `_sum` and `_count`. The first version should accept whole-family drops and consistent family-wide edits. A partially dropped family, a rule that changes structural `le` or `quantile` labels, or an inconsistent family rename makes the original point unreconstructable under this contract. For example, dropping a summary's `_count` while keeping its quantiles drops the entire point on conversion back to OTLP.
- **Series identity.** A `labeldrop` or rename can make distinct points share the same final OTLP stream identity and timestamp. Detect within-batch conflicts and drop the conflicting points with a diagnostic. Report an empty or invalid output metric name as a conversion failure. Identity conflicts across batches remain possible. Aggregation belongs in a separate step.
- **Dropped data.** Count rule drops separately from unsupported points, projection collisions, and incomplete or conflicting output. Drop an affected point or family while forwarding valid output from the same batch. Use bounded reason labels for diagnostics and avoid attaching user label values to telemetry.

## Performance

The processor adds a conversion pass in each direction and runs the rule list between them. For a batch with `S` projected series and `R` rules, expect up to `S × R` rule evaluations. Label count, string length, and regex complexity also affect cost. A histogram with `B` explicit bounds can expand to `B + 3` classic series per point. A summary with `Q` quantiles can expand to `Q + 2` series. A rule that drops most of the output still pays the up-front projection cost.

Peak memory can include the input batch, projected labels and samples, provenance, and rebuilt output at the same time. Keep intermediate state batch-local, reuse compiled rules, and skip rebuilding dropped points. Benchmark a bounded chunking strategy if large batches drive peak memory. Chunking must keep related series together and preserve the batch's collision checks.

## Validation before implementation ships

- Compare rule results with Prometheus on the same projected labels, including defaults, ordering, `__name__` rewrites, and label deletion.
- Use OTLP round-trip fixtures for no-op rules, typed attributes, distinct resources and scopes, whole-family drops, incomplete distributions, output identity collisions, and unsupported input.
- Benchmark pass-through against the round trip with no-op, drop-heavy, and rename-heavy rule sets. Vary batch size, labels, and histogram or summary width. Report input points per second, projected series per second, bytes and allocations per input point, and peak heap under concurrent batches. Include receiver-side metric relabeling as a baseline on equivalent Prometheus input.
