# Prometheus relabeling performance

The same Collector image ran the baseline, processor, and exporter pipelines in
a dev-monitoring Job. The exporter had higher median throughput in 10 of 12
cases with active rules, with the biggest gain on scalar label renames. The
processor led on mixed-metric workloads that dropped half the points.

Tested source revision `66922446b7432203f8473628746d0b6c6b127c4d`
and Linux amd64 image digest `sha256:c4307ca180e087aad2da46266f5bd0f17f7e94af77c5f1445528e43b5167aa4d`.

## Workload and verification

The harness runs one Collector process at a time on the same Kubernetes pod.
The Job ran on an AWS `m6i.8xlarge` node with Kubernetes v1.35.9.
The pod has two CPUs, 512 MiB of memory, and `GOMAXPROCS=2`.
An OTLP HTTP client sends one request at a time. The Collector sends Snappy
compressed Remote Write to an HTTP sink on loopback. Export queues and retries
are disabled and the test uses no WAL.

Scalar batches have 256 gauge points. Mixed batches have 768 points across
a gauge, an eight-bound classic histogram, and a three-quantile summary.
Each series has the same four point attributes, with distinct shard and
instance values for distinct input streams. A mixed batch translates to
4,352 samples before any drops.

Each run decodes and verifies the first batch, warms up with 12 batches, and
times 120 batches. Verification compares series labels, timestamps and values
between the processor and exporter. It also compares both to baseline for
empty and nonmatching rules. In the drop case both relabeling paths drop half
the input points. In the rename case they replace the same point label.
The measured interval uses a sink that drains the requests. V1 sends metadata
in a separate request. V2 carries metadata within its series request.

There are three runs of each configuration with rotating pipeline order. CPU
time and peak resident memory come from Linux process accounting for the entire
Collector process, including startup, verification, warmup, and shutdown.
Throughput uses only the timed interval. The local Go benchmark uses the same
fixtures and materializes v1 series. It excludes HTTP, metadata requests, and
compression.

## Dev-monitoring results

Each throughput cell gives the median and full observed range in thousands of
input data points per second. CPU is median Collector CPU microseconds per
timed input point. RSS is median peak Collector MiB. CPU and RSS columns are
ordered baseline / processor / exporter.

| RW | Input | Policy | Baseline kpts/s | Processor kpts/s | Exporter kpts/s | CPU µs/point | Peak RSS MiB |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | Scalar | empty | 171.7 (171.2–179.1) | 180.0 (177.3–180.5) | 177.9 (176.4–179.3) | 8.62 / 8.53 / 8.53 | 38.1 / 38.3 / 38.3 |
| 1 | Scalar | no-match | 174.3 (169.9–175.0) | 115.3 (113.9–117.1) | 155.5 (154.5–156.7) | 8.67 / 13.31 / 9.63 | 38.3 / 38.6 / 38.3 |
| 1 | Scalar | drop | 180.5 (175.7–183.2) | 145.3 (136.2–148.1) | 174.1 (171.4–175.8) | 8.37 / 10.51 / 8.65 | 38.1 / 38.2 / 37.8 |
| 1 | Scalar | rename | 179.9 (166.9–183.4) | 87.1 (87.1–88.0) | 130.7 (129.4–139.4) | 8.33 / 17.01 / 11.47 | 38.1 / 38.8 / 38.3 |
| 1 | Mixed | empty | 78.9 (77.9–80.9) | 79.1 (78.8–79.1) | 78.7 (76.4–82.4) | 17.85 / 17.77 / 17.89 | 50.7 / 50.1 / 50.3 |
| 1 | Mixed | no-match | 78.9 (76.6–80.0) | 51.5 (50.2–52.1) | 56.8 (56.2–57.4) | 17.88 / 26.74 / 23.53 | 50.3 / 50.9 / 50.3 |
| 1 | Mixed | drop | 78.2 (77.6–78.7) | 83.9 (83.3–84.9) | 76.6 (76.5–76.9) | 17.80 / 16.69 / 18.38 | 50.1 / 44.2 / 44.2 |
| 1 | Mixed | rename | 78.7 (78.3–80.4) | 39.6 (39.3–39.7) | 40.6 (40.5–41.2) | 17.63 / 34.46 / 33.83 | 50.2 / 52.1 / 52.4 |
| 2 | Scalar | empty | 154.8 (151.9–155.3) | 153.6 (147.4–164.7) | 156.9 (156.8–158.2) | 9.92 / 9.85 / 9.72 | 37.5 / 37.4 / 37.5 |
| 2 | Scalar | no-match | 155.0 (150.5–156.8) | 99.9 (97.2–104.0) | 135.3 (135.0–136.0) | 9.82 / 15.41 / 11.11 | 37.4 / 37.6 / 37.8 |
| 2 | Scalar | drop | 155.0 (146.4–156.9) | 134.9 (134.6–136.3) | 164.1 (159.7–166.6) | 9.78 / 11.25 / 8.93 | 37.6 / 37.5 / 37.2 |
| 2 | Scalar | rename | 155.4 (152.9–158.1) | 78.5 (78.2–79.2) | 120.8 (115.3–122.9) | 9.89 / 19.46 / 12.24 | 37.5 / 37.8 / 38.0 |
| 2 | Mixed | empty | 65.6 (64.5–66.0) | 65.8 (65.3–66.8) | 65.5 (65.2–66.4) | 23.01 / 22.79 / 23.00 | 44.3 / 43.4 / 43.7 |
| 2 | Mixed | no-match | 65.7 (64.0–66.1) | 45.4 (44.8–46.3) | 50.0 (49.6–50.8) | 22.92 / 32.31 / 28.91 | 43.4 / 44.3 / 44.0 |
| 2 | Mixed | drop | 64.7 (64.6–65.5) | 72.2 (71.6–72.6) | 68.5 (67.1–68.9) | 23.20 / 20.42 / 21.44 | 43.2 / 41.1 / 40.6 |
| 2 | Mixed | rename | 65.5 (64.5–66.0) | 34.7 (34.3–35.2) | 37.3 (37.3–37.8) | 22.99 / 41.17 / 38.81 | 43.4 / 44.9 / 44.2 |

## Local allocation results

Go 1.26.7 on Apple M1 Pro, darwin/arm64, two logical processors. Entries are
medians of five samples with a 200 ms target per sample. Each cell gives
milliseconds per batch / MiB allocated per batch / allocations per batch.

| Input | Policy | Baseline | Processor | Exporter |
| --- | --- | ---: | ---: | ---: |
| Scalar | empty | 0.29 / 0.22 / 2,641 | 0.29 / 0.22 / 2,646 | 0.34 / 0.22 / 2,641 |
| Scalar | no-match | 0.31 / 0.22 / 2,641 | 0.79 / 0.70 / 8,948 | 0.42 / 0.26 / 2,905 |
| Scalar | drop | 0.29 / 0.22 / 2,641 | 0.51 / 0.41 / 5,590 | 0.35 / 0.20 / 2,391 |
| Scalar | rename | 0.29 / 0.22 / 2,641 | 1.31 / 1.02 / 18,432 | 0.62 / 0.40 / 4,697 |
| Mixed | empty | 3.91 / 4.09 / 33,474 | 3.94 / 4.09 / 33,479 | 3.94 / 4.09 / 33,474 |
| Mixed | no-match | 3.97 / 4.09 / 33,474 | 8.09 / 6.15 / 61,499 | 6.86 / 4.67 / 37,834 |
| Mixed | drop | 4.10 / 4.09 / 33,474 | 9.20 / 3.34 / 37,445 | 5.56 / 3.74 / 29,109 |
| Mixed | rename | 3.94 / 4.09 / 33,474 | 11.52 / 7.51 / 108,884 | 10.08 / 7.43 / 68,303 |

## Interpretation

For scalar label renames the exporter reached 131k versus 87k points/s in v1
and 121k versus 79k points/s in v2. For mixed-metric drops the processor reached
84k versus 77k points/s in v1 and 72k versus 69k points/s in v2. The baseline
sends twice as many samples for these drop workloads. The processor removes
rejected points before downstream translation, while the exporter evaluates the
rules during translation.

The local v1 conversion benchmark favored the exporter for mixed-metric drops
on this host. The cluster comparison uses a different CPU architecture and
includes receiver processing, v1 metadata generation, serialization, compression,
and HTTP. Treat the cluster result as a measurement of that complete workload.

The no-match and rename comparisons isolate the cost of applying rules to the
same labels and retaining every point. The processor also checks OTLP identities
and may rebuild OTLP before the exporter translates it. The exporter handles a
different label surface and can drop individual buckets or quantiles.

The test repeats a fixed synthetic batch in one pod. Production scrape traffic,
concurrent input, queue pressure, backend latency, and series churn can shift
these results. RSS includes process startup and warmup. The measured path ends
at the local HTTP sink.

## Reproduce

Build and run the [benchmark Collector](README.md) with the
[Job manifest](benchmark-job.yaml). The committed evidence contains
[dev-monitoring results](results/dev-monitoring.jsonl) and
[local allocation samples](results/local-bench.txt).

```sh
go test -cpu 2 -run '^$' -bench '^BenchmarkPipeline$' -benchmem -benchtime=200ms -count=5 ./internal/workload
```
