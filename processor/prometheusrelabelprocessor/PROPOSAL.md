# Prometheus metric relabeling processor

Add a metrics-only Collector processor named `prometheus_relabel`. It translates
OpenTelemetry metrics (`pmetric.Metrics`) to Prometheus time series in memory,
applies Prometheus relabeling rules to the series label sets, and converts the
result back to `pmetric.Metrics`.

## Motivation

A Collector can send the same input metrics to multiple destinations. An
operator may want to drop a high-cardinality label on one path, or reuse a
Prometheus metric relabeling policy on metrics received over OTLP. Putting the
processor on one metrics pipeline makes that policy specific to the pipeline
and keeps its output available to OTLP exporters.

The request to add `write_relabel_configs` to the Prometheus Remote Write exporter
raised this use case. The Prometheus Interoperability SIG discussed both a
processor limited to data point attributes and an exporter implementation that
could relabel the series it already translates. This proposal describes the
processor and defines how to compare it with the exporter approach.
[Collector Contrib #52988][write-request] · [SIG discussion summary][sig-thread]

## Where rules run

Prometheus uses the same [`relabel_config`][relabel-config] syntax at different
points in its pipeline. The stage determines which labels the rules can see.

| Configuration | Operates on | Stage |
| --- | --- | --- |
| `relabel_configs` | Target labels, including service discovery metadata | Before scraping |
| `metric_relabel_configs` in a scrape job | Labels on scraped samples | After applying target labels, before ingestion |
| `write_relabel_configs` | Series labels for a Remote Write destination | After adding external labels, before sending |
| `metric_relabel_configs` in this proposal | Labels on time series translated from Collector data points | Inside the configured Collector metrics pipeline |

The Collector's [Prometheus receiver][scrape-receiver] already runs scrape-time
metric relabeling through Prometheus's scrape manager. Prometheus also proposes
[`receive_relabel_configs` for OTLP and Remote Write ingestion][receive-relabel],
with the configuration placement still under review. That proposal applies rules
before Prometheus storage. This processor applies them before Collector export.

## Grafana Alloy precedent

Grafana Alloy is a Collector distribution with two pipeline runtimes. Its
embedded OTel Engine runs the upstream Collector with YAML configuration.
`prometheus.relabel` belongs to Alloy's Default Engine, where components can
exchange Prometheus samples through an in-process appender.
[Alloy engines][alloy-engines] · [Alloy relabel component][alloy-relabel-code]

Alloy's `otelcol.exporter.prometheus` accepts `pmetric.Metrics` and appends
translated samples. `prometheus.relabel` uses the same rule evaluator for
samples, native histograms, exemplars, metadata, and start-timestamp markers.
It passes a zero series reference downstream so the next appender resolves
series identity from the new labels.
Alloy's `otelcol.receiver.prometheus` builds OTLP metrics when the appender
commits. This provides a reference for the conversion boundary and for keeping
all data associated with a renamed or dropped series together.
[Alloy forward conversion][alloy-to-prom] · [Relabel hooks][alloy-relabel-hooks] · [Alloy return conversion][alloy-from-prom]

The return conversion reconstructs resources and scopes from Prometheus
labels and info metrics, and reads metric family metadata from a scrape
metadata store. On an OTLP input path, Alloy fanout supplies an empty store
when the context lacks one, and the reverse bridge's v1 `UpdateMetadata`
method is a no-op. A round trip through this path therefore cannot guarantee
the original metric types, resource and scope attributes, or numeric precision.
Keep the original OTLP data point and metric descriptor available during our
return conversion so relabeling only changes the intended fields.
[Alloy fanout context][alloy-fanout] · [Resource and scope mapping][alloy-context] · [Numeric conversion][alloy-numbers]

## Configuration and label mapping

Use `relabel.Config` and `relabel.ProcessBuilder` from Prometheus's
`model/relabel` package for action semantics, defaults, validation, and rule
order. Validate the rule list at startup. With an empty list, forward the batch
directly.
[Prometheus relabeling implementation][relabel-code]

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

Rules see `__name__`, labels translated from OTLP data point attributes, and
`le` or `quantile` on the corresponding classic histogram or summary series.
Use the existing `UnderscoreEscapingWithSuffixes` translation strategy for the
first prototype, including unit and type suffixes where applicable. Resource
and instrumentation scope stay on the OTLP metrics returned by the processor.
To match a resource attribute in a rule, copy it to a data point attribute
upstream. Exporter external labels, resource-derived `job` and `instance`,
`target_info`, and generated scope labels are supplied by other stages. A
received `up` metric is a normal input to the processor.
[Prometheus OTLP translation strategies][translation-strategy] · [OpenTelemetry compatibility specification][compat]

## Translation and output

1. **Translate OTLP data points to Prometheus series.** Use the shared
   `prometheus/otlptranslator` name and unit helpers and reuse time series
   conversion logic where the two paths agree. In memory, each translated
   sample keeps a reference to its source data point and the role of the
   sample in its metric family, such as a bucket, count, or quantile. Keep
   track of the original attribute keys and types for the return conversion.
   [Collector translator migration #39827][translator-migration] · [Classic histogram and summary conversion][to-prom]
2. **Apply rules to the label set.** A Prometheus time series is identified by
   its labels, including `__name__`. Relabeling rewrites those labels or drops
   the series. Preserve the mapping to the input when distinct OTLP sources
   produce the same Prometheus label set. Use `__name__` for metric renames and
   discard temporary `__tmp` labels after evaluating the full list.
   [Prometheus data model][prom-model] · [Relabeling configuration][relabel-config]
3. **Rebuild the OTLP data points.** Preserve the original name when the rule
   list leaves `__name__` unchanged. Retain unchanged attributes with their
   original keys and value types, remove attributes whose labels were
   deleted, and write new or modified labels as string data point attributes.
   Reuse the input metric type, unit, description, temporality, and
   monotonicity. Copy values, timestamps, start timestamps, flags, exemplars,
   and distribution fields from the source data point to avoid precision or
   metadata loss. Emit each result under its original resource and scope.
   [OpenTelemetry metrics data model][otel-model]

The existing conversion paths show where shared code would help. The Remote
Write exporter calls `FromMetrics` or `FromMetricsV2`, which return series
grouped for their wire formats and already incorporate exporter settings.
The Prometheus scrape receiver assembles OTLP histogram and summary data
points from samples grouped by metric family and associated metadata. The
Remote Write receiver drops typed summaries and classic histograms because
their constituent series may arrive in separate requests. A processor can
keep that association within the original OTLP batch and use the in-memory
series directly.
[Remote Write v1 translator][rw-v1] · [Remote Write v2 translator][rw-v2] ·
[Scrape receiver metric-family assembly][from-scrape] · [Remote Write receiver limitations][from-rw]

## Round-trip limits

- **Metric types.** Start with gauges, cumulative monotonic sums, cumulative
  explicit-bucket histograms, and summaries. Delta temporality and exponential
  histograms are outside the initial supported set. Native histograms and
  native histograms with custom buckets (NHCB) have their own mapping and
  relabeling behavior. For example, a native histogram is a single series,
  so rules that match a classic histogram's `le` labels have different input.
  [Prometheus native histogram specification][native-histograms]
- **Classic histograms and summaries.** A classic histogram has `_bucket`
  series, including `le="+Inf"`, a `_count` series, and a `_sum` series when
  the input has a sum. A summary has quantile series plus `_sum` and
  `_count`. Support dropping the whole point or making a consistent edit
  across its constituent series. Treat a partial drop, a change to `le` or
  `quantile`, or disagreement on the resulting family name or common labels
  as a conversion failure for that point. For example, dropping just a
  summary's `_count` also drops the quantiles on the way back to OTLP.
  [Compatibility specification][compat] · [Scrape receiver implementation][from-scrape]
- **Name and attribute collisions.** With underscore escaping, `cluster.name`
  and `cluster_name` can become the same label. Reject ambiguous mappings and
  count the affected data points. Validate the output Prometheus
  name and label set and check for duplicate OTLP data points or conflicting
  metric identities in the batch. Removing a distinguishing label can collapse
  independent streams, even when timestamps differ. The first version has
  batch-local checks. Deliberate aggregation is a separate operation.
  [Collector translator naming rules][collector-names] · [OpenTelemetry metrics data model][otel-model]
- **Data loss accounting.** Forward valid output and count drops by bounded
  reason, separating configured rule drops from unsupported types, invalid
  names, ambiguous labels, and incomplete or conflicting output. A configured
  policy should take effect on supported input with explicit diagnostics for
  points that cannot be processed.

## Interoperability work to track

- The [2026 interoperability survey][interop-survey] identified naming and
  resource metadata as areas for convergence. Prometheus supports configurable
  OTLP name translation today, and the [OpenMetrics 2.0 release candidate][openmetrics]
  specifies how to expose UTF-8 metric and label names. The [OpenTelemetry
  resource and entities proposal][entities] and the [Prometheus native
  metadata design][native-metadata] address how to represent source identity.
  Keep the processor's mapping explicit and reuse shared translators as those
  conventions evolve.
- [PROM-39][type-unit] defines reserved type and unit labels.
  [PROM-104][metadata-relabel] defines metadata relabeling using virtual
  `__type__`, `__unit__`, and `__help__` labels. The Prometheus scrape
  implementation is behind a feature flag. The first processor prototype
  preserves OTLP metric metadata and applies rules to metric names and series
  labels. Compatibility with metadata-aware rules needs its own design.
  [Prometheus #20101][metadata-relabel-impl]
- Prometheus's [native OTLP delta ingestion][native-delta] keeps delta samples
  as deltas. Extending the processor to delta points should preserve
  temporality and start timestamps. This is independent of whether a
  downstream backend can ingest those metrics.

## Performance and prototype comparison

The processor adds forward translation, relabeling, and return translation
before export. For `S` translated label sets and `R` rules, relabeling can
evaluate up to `S × R` rules. If the forward conversion expands each data
point independently, a classic histogram with `B` explicit bounds yields
`B + 2` series plus one if it has a sum. A summary with `Q` quantiles yields
`Q + 2` series. Drop-heavy policies still pay for the forward translation.
Peak heap can include the original batch, translated samples and label sets,
and the output batch. Alloy's appender boundary suggests a lower-allocation
prototype that evaluates each translated label set as it is emitted. It can
retain the relabeling results required for family reconstruction and collision
checks without buffering a full intermediate batch of Prometheus samples.
[Alloy forward conversion][alloy-to-prom] · [Relabel hooks][alloy-relabel-hooks]

Compare the processor with a baseline pipeline and with an exporter prototype
that relabels the translated series before constructing Remote Write requests.
The exporter prototype can see the final external labels and avoid the return
translation. On a pipeline that uses the processor followed by that exporter,
the exporter also translates OTLP to Prometheus again. Compare emitted series
where rules have the same label inputs, and record differences when the
exporter can act on individual classic histogram or summary series. Compare
scrape-time relabeling on equivalent scraped input and OTTL where the
`filter` or `transform` processor can express the same policy over OTLP data.
[Remote Write v1 export path][rw-export-v1] · [Remote Write v2 export path][rw-export-v2] ·
[Filter processor][filter] · [Transform processor][transform]

For the exporter prototype, a candidate hook is after constructing the final
label set and before grouping samples into series. That also puts the hook
before Remote Write v2 interns label names and values in its symbol table.
Check metadata association after a rename in both versions. The v1 exporter
builds metadata separately from the series, while v2 associates metadata with
each time series. [Remote Write v1 export path][rw-export-v1] · [Remote Write v2 translator][rw-v2]

Measure data points and translated samples per second, CPU, bytes and
allocations per input point, and peak heap at representative concurrency. Use
no rules, rules with no matches, drop-heavy and rename-heavy rules, and vary
batch size, label cardinality, and bucket or quantile count. Start with
batch-local state. Test caching rule results by input label set separately
under repeated labels and churn before deciding whether a bounded cache is
worth its memory. Alloy caches rewritten label sets and drop decisions, clears
the cache on configuration updates, and evicts entries on stale markers. Its
default LRU caps the entry count, while its optional TTL cache tracks active
series without a hard size cap. Benchmark cache hits and misses separately
from the two conversions. [Alloy relabel cache][alloy-cache] · [Alloy rule evaluation][alloy-evaluation] · [Prometheus #19675][receive-relabel]

Test the same label sets against Prometheus for rule defaults and ordering.
Assert OTLP round-trip behavior for typed attributes, resources and scopes,
timestamps, metadata, an absent histogram sum, partial family drops,
`__name__` rewrites, and identity collisions. Compare the final exported series
as well as the returned OTLP, since a Prometheus exporter can apply its own
translation strategy.

[write-request]: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/52988
[sig-thread]: https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/52988#issuecomment-7044964295
[relabel-config]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
[scrape-receiver]: ../../receiver/prometheusreceiver/README.md
[receive-relabel]: https://github.com/prometheus/prometheus/pull/19675
[alloy-engines]: https://grafana.com/docs/grafana-cloud/send-data/alloy/introduction/otel_alloy/#how-the-engines-fit-together
[alloy-relabel-code]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/prometheus/relabel/relabel.go#L29-L65
[alloy-to-prom]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/otelcol/exporter/prometheus/internal/convert/convert.go#L128-L153
[alloy-relabel-hooks]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/prometheus/relabel/relabel.go#L189-L272
[alloy-from-prom]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/otelcol/receiver/prometheus/internal/transaction.go#L566-L595
[alloy-fanout]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/prometheus/fanout.go#L186-L208
[alloy-context]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/otelcol/receiver/prometheus/internal/transaction.go#L428-L466
[alloy-numbers]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/otelcol/exporter/prometheus/internal/convert/convert.go#L480-L486
[alloy-cache]: https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.relabel/#arguments
[alloy-evaluation]: https://github.com/grafana/alloy/blob/64f2c2e324cc86b485f40136cac41d4670f55c46/internal/component/prometheus/relabel/relabel.go#L302-L356
[relabel-code]: https://github.com/prometheus/prometheus/blob/v0.316.0/model/relabel/relabel.go
[translation-strategy]: https://prometheus.io/docs/guides/opentelemetry/#utf-8
[compat]: https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/
[translator-migration]: https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/39827
[to-prom]: ../../pkg/translator/prometheusremotewrite/helper.go
[prom-model]: https://prometheus.io/docs/concepts/data_model/
[otel-model]: https://opentelemetry.io/docs/specs/otel/metrics/data-model/
[rw-v1]: ../../pkg/translator/prometheusremotewrite/metrics_to_prw.go
[rw-v2]: ../../pkg/translator/prometheusremotewrite/metrics_to_prw_v2.go
[from-scrape]: ../../receiver/prometheusreceiver/internal/metricfamily.go
[from-rw]: ../../receiver/prometheusremotewritereceiver/README.md#known-limitations
[native-histograms]: https://prometheus.io/docs/specs/native_histograms/#otlp
[collector-names]: ../../pkg/translator/prometheus/README.md#labels
[interop-survey]: https://opentelemetry.io/blog/2026/otel-prometheus-interoperability/
[openmetrics]: https://prometheus.io/docs/specs/om/open_metrics_spec_2_0/
[entities]: https://github.com/open-telemetry/oteps/blob/main/text/entities/0264-resource-and-entities.md
[native-metadata]: https://docs.google.com/document/d/1yYnyD7oJDvJhzFaigdniq6y302Mvp9gDcJUeAj3pJ0s/edit
[type-unit]: https://github.com/prometheus/proposals/pull/39
[metadata-relabel]: https://github.com/prometheus/proposals/pull/104
[metadata-relabel-impl]: https://github.com/prometheus/prometheus/pull/20101
[native-delta]: https://github.com/prometheus/prometheus/pull/16360
[rw-export-v1]: ../../exporter/prometheusremotewriteexporter/exporter.go
[rw-export-v2]: ../../exporter/prometheusremotewriteexporter/exporter_v2.go
[filter]: ../filterprocessor/README.md
[transform]: ../transformprocessor/README.md
