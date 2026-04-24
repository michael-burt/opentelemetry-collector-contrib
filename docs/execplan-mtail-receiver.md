# Add The `mtailreceiver`

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with the ExecPlan requirements and guidelines in the `execution-plan` skill.

## Purpose / Big Picture

After this change, a collector build can include a new `mtail` metrics receiver that reads log files, executes mtail programs against those log lines, and emits OpenTelemetry metrics into collector pipelines. A user will be able to point the receiver at one or more mtail program files and one or more log path patterns, start the collector, and observe metrics derived from those logs without running a separate mtail daemon.

The user-visible behavior to prove is:

1. The receiver loads `.mtail` programs from a configured file or directory.
2. The receiver tails configured logs using mtail-compatible file globbing behavior.
3. The receiver periodically snapshots the current mtail metric state and forwards it to the next `consumer.Metrics`.
4. A focused unit or integration-style test shows that a log line processed by an mtail program becomes an emitted OTLP metric.

## Progress

- [x] (2026-04-24 15:57Z) Read contrib component onboarding guidance in `CONTRIBUTING.md` and confirmed the required module layout, metadata generation, and repo wiring for a new receiver.
- [x] (2026-04-24 15:57Z) Read the mtail project documentation and source entrypoints relevant to embedding: `README.md`, `docs/Programming-Guide.md`, `docs/Language.md`, `internal/mtail/mtail.go`, `internal/runtime/runtime.go`, `internal/tailer/tail.go`, and `internal/exporter/otel.go`.
- [x] (2026-04-24 15:57Z) Chosen design: implement a collector-native metrics receiver that embeds a trimmed mtail core in-repo instead of importing `github.com/jaqx0r/mtail` as a dependency.
- [x] (2026-04-24 16:42Z) Scaffolded `receiver/mtailreceiver` with config, factory, metadata, README, doc.go, tests, lifecycle fixtures, and module metadata.
- [x] (2026-04-24 16:42Z) Vendored the mtail source set needed for compilation, runtime, tailing, and metric storage under `receiver/mtailreceiver/internal/mtail`, regenerated the parser, and rewrote imports to local receiver paths.
- [x] (2026-04-24 16:42Z) Implemented the receiver lifecycle and periodic snapshot export to `consumer.Metrics`.
- [x] (2026-04-24 16:42Z) Added metric conversion tests and an end-to-end receiver test using a sample mtail program and log file.
- [x] (2026-04-24 16:42Z) Ran metadata generation, module tidy, and focused `go test ./...` validation for `receiver/mtailreceiver`.
- [ ] Review final diff, commit the branch, and push to `origin`.

## Surprises & Discoveries

- Observation: mtail is not consumable as a normal public library for this use case because its core packages are beneath Go `internal/` paths.
  Evidence: core runtime and exporter packages live under `github.com/jaqx0r/mtail/internal/...`, which cannot be imported from `opentelemetry-collector-contrib`.

- Observation: contrib previously removed `promtailreceiver` due to maintenance and dependency cost.
  Evidence: `CHANGELOG.md` includes removal notes for `promtailreceiver` explaining that its Loki dependency footprint became too large.

- Observation: mtail already contains an OTel-oriented metric projection layer, but it produces SDK `metricdata` rather than collector `pmetric`.
  Evidence: `~/workspace/repos/github.com/mtail/internal/exporter/otel.go` builds `metricdata.ScopeMetrics` values from mtail’s store.

## Decision Log

- Decision: Keep the new receiver at `development` stability and do not assume inclusion in default contrib binaries.
  Rationale: contrib guidance expects new components to begin in development, and shipping this in default distributions would require more maturity, sponsor alignment, and broader testing than fits this implementation.
  Date/Author: 2026-04-24 / Codex

- Decision: Embed a focused mtail source subset under `receiver/mtailreceiver/internal/mtail` rather than import mtail directly or shell out to the mtail binary.
  Rationale: direct imports are blocked by Go `internal/` visibility, and subprocess orchestration would not satisfy the request to embed functionality and interfaces from the local mtail project.
  Date/Author: 2026-04-24 / Codex

- Decision: Exclude mtail’s HTTP server, legacy exporters, and daemon-specific surface from the receiver.
  Rationale: the collector receiver contract only needs lifecycle management, log tailing, program execution, in-memory metrics, and conversion into collector metrics. Carrying the HTTP server and extra exporters would increase maintenance cost without improving receiver behavior.
  Date/Author: 2026-04-24 / Codex

- Decision: Implement a receiver-owned periodic snapshot loop that converts mtail’s current metric store into `pmetric.Metrics`.
  Rationale: mtail’s runtime is event-driven while the collector receiver interface expects outbound metric batches delivered to a consumer. A periodic snapshot loop is the simplest stable bridge.
  Date/Author: 2026-04-24 / Codex

## Outcomes & Retrospective

The implementation now provides a development-stage `mtailreceiver` module that embeds the mtail compiler/runtime/tailer pipeline directly inside contrib and emits collector metrics snapshots on a configurable interval. The result preserves the mtail program model while staying inside the collector receiver contract.

The main residual gaps are repo-process rather than local functionality: the generated component test required a manual compatibility patch for the current local collector test helper signature, and the component is intentionally not yet wired into default distributions because it remains development stability only.

## Context and Orientation

This repository is the OpenTelemetry Collector Contrib monorepo. Each receiver lives in its own Go module beneath `receiver/<name>receiver`. A new receiver normally includes:

- `go.mod` and `go.sum` for the receiver module.
- `Makefile` that includes `../../Makefile.Common`.
- `metadata.yaml` for generated status and tests.
- `doc.go` with `//go:generate mdatagen metadata.yaml`.
- `factory.go`, `config.go`, receiver implementation files, tests, and `README.md`.

The main contrib guidance for new components is in `CONTRIBUTING.md`, especially the section beginning near “If you are writing a new component from scratch”.

The local mtail repository lives at `~/workspace/repos/github.com/mtail`. The parts relevant to this work are:

- `internal/runtime/...`: compiles mtail programs and runs their virtual machines.
- `internal/tailer/...`: discovers files and tails streams into log lines.
- `internal/metrics/...`: stores mtail metrics in memory.
- `internal/logline/logline.go`: log line representation passed from tailer to runtime.
- `internal/exporter/otel.go`: converts mtail store data into OTel SDK metric structures.

In this change, “embed mtail” means copying the necessary source into the new receiver module and adapting imports so the collector can build it as part of contrib.

## Plan of Work

First, create `receiver/mtailreceiver` as a normal contrib receiver module. The initial files should mirror a typical metrics receiver module: `Makefile`, `go.mod`, `metadata.yaml`, `doc.go`, `README.md`, `config.go`, `factory.go`, and one or more implementation files such as `receiver.go`, `adapter.go`, and test files. The metadata should declare `type: mtail`, class `receiver`, stability `development: [metrics]`, and no distributions.

Next, populate `receiver/mtailreceiver/internal/mtail` with the minimal source tree needed to run mtail programs against tailed log files. This should include the compiler, runtime, virtual machine, tailer, logstream support, metric store, datum types, log line types, and waker support. It should exclude mtail’s HTTP server package and exporter implementations that are unrelated to collector delivery. All copied imports that currently point at `github.com/jaqx0r/mtail/internal/...` must be rewritten to the new receiver-local package path.

Then implement the collector-facing receiver. The receiver should:

- validate config for `programs` and `logs`,
- create a mtail store,
- start the mtail runtime and tailer on `Start`,
- run a ticker using a configurable collection interval,
- on each tick, read the mtail metric snapshot and convert it into `pmetric.Metrics`,
- send non-empty batches to the next `consumer.Metrics`,
- stop the ticker and cancel mtail runtime/tailer work on `Shutdown`.

The metric conversion layer should preserve the key mtail semantics already expressed in mtail’s OTel exporter:

- counters become monotonic cumulative sums,
- integer and float gauges become gauges,
- timers should map to gauges as mtail’s existing OTel exporter does,
- histograms should become cumulative histograms with explicit bounds and bucket counts,
- text and hidden metrics should not be emitted.

Program names should become scope names in the emitted metrics, mirroring mtail’s `metricdata.ScopeMetrics` grouping.

Finally, add focused tests. One test should cover config validation. One test should cover mtail-to-`pmetric` conversion on counters, gauges, and histograms. One receiver test should create a temporary `.mtail` program and log file, start the receiver with a test consumer, append a matching log line, and assert that an emitted metric appears.

## Concrete Steps

Working directory for all commands in this plan:

    /Users/mburt/workspace/repos/github.com/opentelemetry-collector-contrib/.worktrees/opentelemetry-collector-contrib-0424-0953am

Create and iterate on the receiver:

    mkdir -p receiver/mtailreceiver
    # add module files and implementation

Generate module and metadata outputs after the receiver exists:

    cd receiver/mtailreceiver
    go generate ./...
    go mod tidy

Update repo wiring after the module is in place:

    cd /Users/mburt/workspace/repos/github.com/opentelemetry-collector-contrib/.worktrees/opentelemetry-collector-contrib-0424-0953am
    make crosslink
    make generate

Run focused verification while iterating:

    cd receiver/mtailreceiver
    go test ./...

If repo-level touched-file verification is needed, run only the minimal relevant targets and record failures precisely instead of guessing.

## Validation and Acceptance

Acceptance is met when all of the following are true:

1. `receiver/mtailreceiver` builds successfully with its own `go.mod`.
2. A test demonstrates that a simple mtail program such as:

       counter lines_total
       /$/ {
         lines_total++
       }

   when paired with a tailed log file, results in a collector metric named `lines_total`.
3. The emitted metric is a cumulative monotonic sum and is grouped under a scope named after the mtail program file.
4. `go test ./...` inside `receiver/mtailreceiver` passes.

If metadata generation adds lifecycle tests, they must pass as well unless the metadata explicitly and correctly opts out.

## Idempotence and Recovery

The implementation steps are additive. Re-running `go generate ./...`, `go mod tidy`, `make crosslink`, and `make generate` should be safe.

If import rewrites for the vendored mtail source go wrong, the safe recovery path is to re-copy the affected source files from `~/workspace/repos/github.com/mtail` and re-apply the path rewrite consistently. Avoid hand-editing large generated parser artifacts unless a build error proves they require it.

If repo-wide generation commands fail for unrelated pre-existing issues, capture the exact failure and continue validating the new receiver module locally rather than masking the unrelated breakage.

## Artifacts and Notes

Important design facts established during research:

    mtail core is behind Go internal package boundaries, so it cannot be imported directly from contrib.

    mtail already has a mapping from its metric store to OTel SDK metricdata, which can be used as the behavioral template for collector pdata conversion.

    contrib guidance expects new components to begin in development stability and only later advance into default distributions.

## Interfaces and Dependencies

At the end of this work, these interfaces and types should exist:

In `receiver/mtailreceiver/config.go`, define a `Config` type that at minimum exposes:

    Programs string `mapstructure:"programs"`
    Logs []string `mapstructure:"logs"`
    IgnoreRegex string `mapstructure:"ignore_regex"`
    CollectionInterval time.Duration `mapstructure:"collection_interval"`
    PollInterval time.Duration `mapstructure:"poll_interval"`
    EmitMetricTimestamp bool `mapstructure:"emit_metric_timestamp"`
    OmitMetricSource bool `mapstructure:"omit_metric_source"`
    SyslogUseCurrentYear bool `mapstructure:"syslog_use_current_year"`

In `receiver/mtailreceiver/receiver.go`, define a receiver type implementing:

    Start(context.Context, component.Host) error
    Shutdown(context.Context) error

The receiver must own:

    a mtail metric store
    a mtail runtime
    a mtail tailer
    a ticker-driven export loop
    the downstream `consumer.Metrics`

In `receiver/mtailreceiver/adapter.go`, define conversion helpers that transform the mtail metric store into `pmetric.Metrics`. The conversion must create:

    one `ResourceMetrics` container
    one `ScopeMetrics` per mtail program
    one `Metric` per mtail metric definition

Revision note: updated after implementation to reflect the actual delivered module, validation status, and remaining push/commit step.
