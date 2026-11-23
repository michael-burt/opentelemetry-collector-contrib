# Prometheus relabeling implementation plan

Implement the `prometheus_relabel` processor described in [the proposal](./PROPOSAL.md).
The prototype will expose Prometheus series labels to Prometheus's rule evaluator,
while retaining the original OTLP data points for output. Metric values will not
pass through Prometheus samples or a generic Prometheus-to-OTLP converter.

Build the processor and a Remote Write exporter with `write_relabel_configs` on
the same fork branch. Package both in one Collector image and exercise them in
dev-monitoring using identical synthetic OTLP inputs and local Remote Write
sinks. Commit the fixtures, build inputs, benchmark results, and a report alongside
the implementations.

The [SIG discussion](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/52988#issuecomment-7044964295)
calls for prototypes and a performance comparison. This plan develops the processor
on the fork, measures it against an exporter implementation, and keeps upstream
component admission separate from the prototype.

## Initial scope

| Input | Initial behavior with a nonempty rule list |
| --- | --- |
| Integer and double gauges | Supported |
| Cumulative monotonic integer and double sums | Supported |
| Cumulative explicit-bucket histograms | Supported, with consistent family edits |
| Summaries | Supported, with consistent family edits |
| Delta or unspecified temporality on sums and histograms | Drop the affected points with an unsupported-temporality reason |
| Nonmonotonic sums, exponential histograms, and other unsupported metric types | Drop the affected points with an unsupported-type reason |

An absent or empty rule list forwards the original batch directly, including types
outside the prototype's supported set. Once rules are configured, unsupported
points do not silently bypass the policy. Keep this behavior visible in the README
and in drop accounting.

Rules see translated data point attributes, `__name__`, and the structural `le` or
`quantile` labels for classic families. They do not see synthesized resource,
scope, external, `job`, `instance`, or metadata labels. Ordinary input attributes
with those names remain ordinary attributes. Do not generate `target_info` or scope
metrics. Process a received `up` metric like any other supported input.

## Processing model

Use three operations inside each `ConsumeMetrics` call:

1. Project one source point into its Prometheus label sets and evaluate the rules.
2. Stage the resulting point mapping and validate batch-local output identities.
3. Copy accepted source points into the output, applying name and attribute edits.

Projection retains a source reference, the original metric descriptor, attribute
provenance, and the constituent's role. A role identifies a scalar, histogram
bucket, histogram count or sum, summary quantile, summary count, or summary sum.
Bucket and quantile roles also retain their original index and label value.

Evaluate each constituent as it is projected. Retain the common result and the
information needed to validate the family, rather than a batch of `prompb` samples.
Source references and mutable label builders are local to the call. Copy any label
result that must outlive the next builder reset. Do not retain input `pdata` after
the call returns.

Batch-local collision checks mean output cannot be sent downstream immediately
after evaluating each point: a later point can invalidate an earlier candidate.
Stage source references and attribute edits first, then materialize accepted
output. Memory includes this candidate/index state and the output batch; streaming
projection does not make the whole processor constant-memory.

## Implementation sequence

### 1. Add configuration and the component skeleton

- Add a standalone component Go module, a metrics-only factory, and a
  `processorhelper` implementation. Start at development stability and use a custom
  Collector Builder configuration for the prototype.
- Align Prometheus, `prometheus/otlptranslator`, Collector, and `pdata` dependency
  versions with the branch. Keep the initial translation strategy fixed at
  `UnderscoreEscapingWithSuffixes`.
- Decode `metric_relabel_configs` into Prometheus `relabel.Config` values. Use a
  shared configuration adapter that invokes the pinned Prometheus version's YAML
  decoding, defaulting, and validation. The receiver's
  [configuration adapter](../../receiver/prometheusreceiver/config.go) is a reference
  for bridging Collector configuration and Prometheus YAML tags.
- Reject unknown fields, null rules, invalid regexes, invalid action settings, and
  invalid target labels at startup. Report the offending rule index. Preserve rule
  order and explicit zero/empty values according to Prometheus's own decoder.
- Keep parsed rules immutable. Allocate rule-evaluation builders and batch state
  per call so concurrent `ConsumeMetrics` calls do not share mutable state.
- Declare `MutatesData: false`: the prototype reads the input and owns its output.
  The empty-rule path forwards input without mutation. Any later in-place variant
  must change this capability and include the cost of fanout cloning in benchmarks.

Completion criteria: Collector config loading agrees with native Prometheus
decoding for defaults and validation; empty configuration forwards the batch;
traces and logs pipelines reject the component as an unsupported signal.

### 2. Implement scalar label projection and return mapping

- Use `prometheus/otlptranslator` for metric names, label names, and units. Reuse
  existing translation helpers where their behavior matches the proposal. Avoid
  importing the Remote Write exporter's grouping and wire-format construction.
- Build one canonical label set per gauge or sum point. Preserve the original
  attribute key, typed value, and projected label name/value alongside it.
- Match the translator's attribute-to-string and empty-value behavior. Preserve
  original attributes omitted from the projection unless a defined mapping makes
  them addressable by a rule; absence from both initial and final labels is not
  evidence of a deletion.
- Reject ambiguous attribute-key normalization within a point, including collisions
  with structural labels. Never overwrite an attribute to insert `__name__`, `le`,
  or `quantile`. Structural-label collisions apply only to roles that use the label.
- Call `relabel.ProcessBuilder` with the full rule list. Remove `__tmp*` labels only
  after all rules have run, then validate the resulting metric name and label names
  under the selected translation strategy.
- Preserve the original OTLP metric name if the final `__name__` equals the initial
  translated name. For a scalar rename, store the resulting `__name__` literally as
  the OTLP metric name. Keep unit and type metadata unchanged.
- Compare final labels with the initial projection. Keep unchanged attributes with
  their original keys and types, remove attributes whose visible labels disappeared,
  and store added or changed labels as string attributes. Preserve the original
  typed attribute when a sequence of rules restores its original projected value.

Completion criteria: scalar keep/drop/rename and attribute edits work without
converting metric values; input remains unchanged; integers such as `2^53 + 1`,
minimum `int64`, and maximum `int64` survive exactly.

### 3. Add classic histogram and summary families

- Reuse the established name suffixes and label formatting for `_bucket`, `_count`,
  `_sum`, `le`, and `quantile`. Include the implicit `le="+Inf"` bucket and omit a
  histogram's sum series when the OTLP point has no sum.
- Treat the label sets as views of the original point. There is no need to compute
  floating-point bucket counts, scalar samples, or quantile values to run relabeling.
  Preserve distribution values by copying the original point after validation.
- Accept a whole-family rule drop, or a result in which every constituent survives
  with the same common labels and a consistent family name. A histogram with `B`
  finite bounds has `B + 2` constituents, plus one when its sum is present. A summary
  with `Q` quantiles has `Q + 2` constituents.
- Require final structural labels to match their original values and roles. Reject
  a changed or removed `le`/`quantile`, partial family drop, constituent name that
  loses its role suffix, or disagreement on common labels/family name. Rules may
  temporarily edit structural labels if the final result restores them.
- Extract a renamed histogram or summary family base using its known roles and
  suffixes. Store that base as the output OTLP name; preserve the original name when
  the projected family names are unchanged. Do not attempt to reverse arbitrary
  underscore escaping or infer a new OTLP unit from a renamed suffix.
- Validate malformed distributions and structural-label formatting before applying
  rules. Use the existing compatibility rules for bounds, buckets, and quantiles;
  do not silently repair input by sorting or rebuilding it.

Completion criteria: consistent edits preserve all distribution fields; dropping
only `_count` or one bucket/quantile rejects the source point once; absent sums,
empty quantile sets, and the implicit infinity bucket have explicit fixtures.

### 4. Validate identities and rebuild accepted output

- Give every candidate an original OTLP stream identity and a resulting OTLP stream
  identity. Use semantic resource/scope identity, schema URLs, metric identity, and
  typed attributes rather than container indexes or a Prometheus-label hash alone.
  Preserve resource/scope containers and their metadata in the returned output.
- Keep source provenance even when separate OTLP sources project to identical
  Prometheus labels. That projection is not sufficient to merge them or reconstruct
  their original attribute types, resources, or scopes.
- Detect distinct original streams collapsing into the same resulting OTLP stream,
  including after a label drop or metric rename. Reject all affected candidates
  deterministically, even when timestamps differ. Allow multiple timestamps from
  the same original stream; reject duplicate resulting points at one timestamp.
- Detect conflicting metric descriptors under the same resulting resource/scope
  and metric name, including incompatible type, unit, temporality, or monotonicity.
  Preserve description and other metric metadata without silently choosing one
  conflicting descriptor. Compare full keys on hash matches.
- Keep identity checks within the OTLP resource/scope model. Do not reject distinct
  resources or scopes merely because the processor's restricted label view is
  identical. Flattening performed later by an exporter is a separate boundary.
- Materialize only accepted points. Copy resource and scope metadata once per
  retained container, metric headers once per resulting descriptor, and each point
  once with its typed `CopyTo` operation. Avoid copying every point in a metric for
  each renamed result. Preserve timestamps, start timestamps, flags, exemplars,
  optional fields, and all distribution fields.
- Prune containers left empty by drops. If every point survives unchanged after
  validation, return the original batch without building a second batch.

Completion criteria: results are independent of input ordering; collisions never
silently aggregate or select the first stream; separate fanout branches observe
their original input; renames that split or combine compatible metric containers
preserve each accepted point exactly once.

### 5. Add drop accounting and failure behavior

Register an internal counter that counts each rejected input point once, including
points represented by multiple buckets or quantiles. Give each point one reason:
`rule_drop`, `unsupported_type`, `unsupported_temporality`, `invalid_input`,
`ambiguous_labels`, `invalid_output`, `incomplete_family`, or `identity_collision`.
Keep metric names, label values, and resource identities out of telemetry labels.

All constituents dropped by rules count as `rule_drop`; a partial family drop
counts as `incomplete_family`. Run collision validation on otherwise valid
candidates, and account for invalidated candidates only once. Document that these
counters count processing attempts if an upstream component retries a batch.

Handled rule drops and conversion failures forward the valid remainder without a
batch error. Returning a batch error for those cases could suppress valid output
or cause it to be retried. Propagate downstream errors normally. An unexpected
internal failure before forwarding returns an error without partially emitting
the staged output. Do not introduce per-point logging, retries, or background work.

Completion criteria: mixed valid/invalid batches export the valid points, telemetry
totals reconcile with rejected input points, and downstream failure tests show no
processor-owned retry or partial send.

### 6. Establish semantic equivalence and pipeline tests

| Test group | Required coverage |
| --- | --- |
| Prometheus rule compatibility | Native defaults and validation; all actions accepted by the pinned `relabel.Config`; ordering, capture replacement, missing labels, separators, hashmod, equal actions, and temporary labels |
| Scalar preservation | Integer/double representation, large and negative integers, NaN/infinity/stale markers where valid, typed attributes, bytes/maps/slices, empty values, timestamps, flags, and exemplars |
| Container preservation | Resource/scope attributes and schema URLs; dropped-attribute counts and metric metadata supported by the pinned `pdata`; identical projected labels under different source containers |
| Family consistency | Histogram bucket/count/optional-sum and summary quantile/count/sum edits; absent sum versus zero sum; partial drops; structural-label changes; valid and invalid family renames |
| Output identity | Normalized-key ambiguity; distinguishing-label removal; rename collisions; mixed types/units/temporalities; repeated original streams; duplicate timestamps; deterministic rejection in permuted input |
| Processor integration | OTLP input to processor to capturing consumer; empty/all-dropped batches; concurrent calls and cancellation; nonmutation; downstream errors; fanout with a second unchanged branch |
| Exported series | Processor to Remote Write v1/v2 and decoded output; suffixes, external labels, metadata association, exemplars, and family membership |

Use the native Prometheus decoder and `ProcessBuilder` as the rule oracle, with
the same initial labels and rule list. Add independent expected-label and OTLP
fixtures so the tests also catch projection and reconstruction errors. Fuzz rule
evaluation/projection boundaries and family reconstruction for panics, mutation,
invalid output, and nondeterministic identity handling.

A renamed OTLP metric retains its original unit and type. A downstream exporter
can therefore add suffixes to the new name. Record this explicitly in exported-name
fixtures and documentation; do not claim exact exporter-stage name equivalence
where the processor and exporter have different naming inputs or strategies.

Completion criteria: component tests, race tests, scoped lint, config tests, and
pipeline fixtures pass; a runnable custom Collector demonstrates destination-specific
relabeling while preserving the other destination's input.

### 7. Implement Remote Write relabeling

Add `write_relabel_configs` to the existing exporter so each destination can
choose its own rules. Compile them with the same adapter as the processor.
Apply rules to the final Prometheus labels, including exporter-added external
labels and generated series, before the v1 series hash and the v2 symbol table.
Keep the existing batching, encoding, queue, WAL, and transport. Empty rules
take the current export path.

Process classic histogram and summary constituents independently. If a bucket
is dropped, drop exemplars that belonged to that bucket. Retain the v2 metadata
on surviving series. For v1, associate metadata with surviving family names
and suppress metadata for a family member renamed outside its role suffix.
Verify both wire formats by decoding actual requests.

### 8. Build one image and test it in dev-monitoring

Build an image with both paths using the fork's pinned Collector and Prometheus
modules. Run the same image and workload for baseline, processor, and exporter.
Use an isolated dev-monitoring workload with synthetic OTLP input and a local
Remote Write sink. Verify matched rules, drops, renames, and unchanged input to
the baseline using decoded output. Use equal CPU/memory settings and report the
image digest, workload, and resource limits so the runs can be reproduced.

### 9. Measure and report performance

Compare the processor followed by Remote Write, exporter-only relabeling, and
unmodified export. Use identical input, translation settings, and rule effects
for the comparison. Include no-match, drop, and rename cases, scalar and
histogram/summary input, and multiple labels and points per batch. Evaluate
differences in supported label inputs and partial family drops separately.

Run Go benchmarks with allocation reporting against equivalent output work.
Measure Collector throughput, CPU time, and peak resident memory in the
container. Keep wire verification outside the timed interval or apply the same
work to every mode. Report repetitions and variation as well as typical values.
Use the [translator benchmarks](../../pkg/translator/prometheusremotewrite/metrics_to_prw_test.go)
and [scrape benchmarks](../../receiver/prometheusreceiver/internal/transaction_bench_test.go)
as fixture references. Commit the commands, data, and interpretation in
`PERFORMANCE.md` and state which results are local and which are from the cluster.

Start with per-call state and reuse a base label set across constituents. Use the
measurements to decide whether a bounded cache or a different reconstruction
strategy is worth its CPU and memory costs.

### 10. Prepare the prototypes for review

Document the separate configurations, supported types, failure behavior, and
the build and benchmark workflow. Keep the processor in development and use
the custom build for evaluation. Follow the [new component process](../../docs/new-components.md)
for any later donation or distribution changes.
