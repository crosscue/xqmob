# xqmob performance and scale diagnostics

Status: xqmob 0.1.4-rc1 diagnostics/performance contract. These measurements describe an execution; they do not alter `mobility-reference-v1.2` semantics or `xqmob-analysis-v0.2` projections.

## Purpose

Large mobility exports should be evaluated on three separate axes:

1. semantic scaling — eventizer accounting continues to close as row count grows;
2. materialization scaling — enriched analytical storage remains predictable;
3. operational scaling — elapsed time, throughput, memory and temporary storage remain practical on the target workstation.

The 0.1.0 reference baseline includes measurements for the third axis, temporal diagnostics, execution/filesystem context, joint speed-only time/displacement characterization, structured source characterization, phase-aware memory sampling and custom temporary-workspace placement.

## Manifest fields

Performance statistics are written under:

```text
manifest.json -> stats.performance
```

The top-level values are:

```text
elapsed_seconds
input_rows_per_second
normalized_rows_per_second
events_per_second
peak_rss_bytes
peak_rss_available
temporary_peak_bytes
```

### Wall time and throughput

`elapsed_seconds` measures from entry to the pipeline until immediately before `manifest.json` is written. Throughput values use that same elapsed duration.

This makes repeated runs comparable while avoiding circular measurement of the manifest write that contains the measurement itself.

### Peak RSS

On Linux, `peak_rss_bytes` is read from `/proc/self/status` `VmHWM`, the process high-water resident set size. If the operating system does not expose that value, `peak_rss_available` is false and no substitute is invented.

Peak RSS is a process-level measurement. It includes the Go runtime, current partition rows, eventizer state, writer buffers and other process allocations.

### Temporary storage

The current v0.1 ingest architecture streams the CSV into deterministic hash partitions, then processes those partition files. Partition files are retained until successful completion unless `--keep-temp` preserves them beyond the run.

`temporary_peak_bytes` is therefore measured after partitioning and represents the high-water partition workspace for the current architecture.

It excludes durable output artifacts such as canonical JSONL and analytical Parquet files.

## Phase timings

`stats.performance.phase` records seconds spent in:

```text
partition_seconds
partition_read_seconds
sort_seconds
eventize_seconds
validate_seconds
write_seconds
writer_close_seconds
storage_accounting_seconds
```

These measurements are intended to reveal the dominant scaling bottleneck. They do not include every orchestration operation, so their sum can be slightly below total elapsed wall time.

The `write_seconds` phase includes analytical/canonical materialization and opt-in trace writing. `writer_close_seconds` includes final buffered flushes and Parquet writer closure.

## Step distributions

Run-level descriptive distributions are written under:

```text
manifest.json -> stats.step_distributions
```

for:

```text
raw_distance_m
effective_distance_m
implied_speed_mps
```

Each metric is split by:

```text
both_known
previous_missing
current_missing
both_missing
```

and reports:

```text
count
p50
p90
p99
```

Quantiles use the Jain-Chlamtac P² streaming estimator after the first five samples. The estimator is deterministic for a deterministic eventization order and requires constant memory per metric/state.

These quantiles are approximate summaries. They are useful for comparing evidence regimes and identifying heavy tails; they are not a replacement for querying `observations.parquet` when exact distribution analysis is required.

## Discontinuity cause attribution

A discontinuity already exists when either of these conditions is true:

```text
effective_distance_m > max_jump_m
implied_speed_mps > max_speed_mps
```

Each discontinuity is attributed to exactly one diagnostic reason:

```text
jump_only
speed_only
jump_and_speed
```

The reason counts must close to `discontinuity_edges`.

Each reason is also cross-tabulated by HA edge state. This is particularly useful when missing horizontal accuracy is associated with a different source/collection regime: the eventizer preserves that evidence while the diagnostics show whether the observed discontinuity population is dominated by large jumps, high implied speeds, or both.

## Baseline materialization results

Before performance instrumentation was added, a pre-release build was exercised on three larger exports:

| Input rows | Normalized | Events | Analysis bytes | Analysis/input | Bytes/normalized observation |
|---:|---:|---:|---:|---:|---:|
| 500,000 | 490,575 | 175,864 | 47.0 MiB | 1.28x | 100.4 |
| 1,000,000 | 980,806 | 392,915 | 103.4 MiB | 1.41x | 110.5 |
| 5,000,000 | 4,899,413 | 1,979,276 | 509.0 MiB | 1.39x | 108.9 |

This indicates near-linear analytical materialization for those runs. The 0.1.0 reference baseline records elapsed time, RSS, temporary workspace and execution/filesystem context so operational results can be compared across WSL-mounted and native filesystems.

## Interpretation

Performance measurements are diagnostic evidence about one run, machine, filesystem, Go runtime and parameter set. They should not be treated as universal product benchmarks.

For meaningful comparisons, keep source data, xqmob version, parameters, partition count, output profile, compression and storage hardware constant.

## Temporal diagnostics

Run-level distributions include approximate `step_dt_s` p50/p90/p99 values by HA edge state. Diagnostics also group edges into fixed descriptive `dt` bands and cross-tabulate discontinuity reason × HA state × time band.

The time bands are intentionally independent of configurable eventizer thresholds. They exist to characterize source timing regimes, especially cases where small displacements over very short intervals generate large implied speeds.

For `speed_only` discontinuities, approximate effective-distance p50/p90/p99 values are also recorded by HA edge state. This distinguishes short-delta jitter from materially large movement below the configured jump threshold.

## Execution and filesystem context

`stats.performance.execution_context` records GOOS/GOARCH, WSL detection, and path/mount/filesystem context for the input, output and temporary workspace.

On Linux/WSL, `/mnt/<drive>/...` paths are labelled `windows_mounted_path`; other paths are labelled `native_or_other`. `/proc/self/mountinfo` is used when available to record the matching mount point and filesystem type.

This metadata is descriptive. xqmob does not normalize or compensate elapsed/phase timings for WSL, Windows-mounted paths, filesystem type, caching, antivirus, storage hardware or other environmental effects. For meaningful scale comparisons, keep these contexts stable or report the differences explicitly.


## Speed-only time/displacement matrix

`stats.temporal_diagnostics.speed_only_dt_distance` records counts in a fixed `dt x effective_distance` matrix for `speed_only` discontinuities. `speed_only_accuracy_dt_distance` records the same matrix separately for each HA edge state.

The effective-distance bands are `<5m`, `5-50m`, `50-500m`, `500m-5km`, and `>=5km`. Like the fixed time bands, these are diagnostic bins rather than eventizer thresholds.

The provisional `speed_only_candidate_classes.micro_displacement` counter identifies `speed_only` cases with `dt < 1s` and effective displacement `<5m`. It is intended only as a scale-comparison measure while real datasets are characterized. No timing, continuity, segment, event, or filtering behavior changes because of this label.

Together with `speed_only_effective_distance_m` and `step_dt_s`, the matrix lets benchmark runs separate:

- tiny displacement over tiny `dt`;
- moderate displacement over short `dt`;
- materially large displacement over short `dt`.

This is particularly useful when comparing different source populations and when determining whether a future eventizer contract should distinguish kinematic anomalies from material trajectory discontinuities.

## Memory and working-set diagnostics

Sampled memory diagnostics are recorded under `stats.performance.memory`. This is intended to locate memory growth before attempting optimization, especially after a pre-release 5-million-row WSL/9P run reached about 3.1 GiB process peak RSS while retaining near-linear throughput.

`memory.by_phase` reports sampled maxima for current RSS, Go `HeapAlloc`, Go `HeapInuse`, and Go runtime `Sys`. Processing-loop phases are sampled every 1024 entities and whenever a new working-set maximum is observed; partition/read/sort/close boundaries are sampled directly. These values are diagnostic and are not a substitute for a Go heap/CPU profile.

`memory.working_set` records the largest loaded partition and largest single entity result as row/object counts, plus shallow struct byte counts. Shallow byte counts exclude referenced string/map/slice backing allocations and parquet-go internal buffers. The application-level Parquet sink batch size is also recorded so it is not confused with parquet-go's internal buffering. xqmob also reports total partition-writer buffer bytes (`partitions × 1 MiB`) and the 1 MiB canonical JSONL buffer. These explain fixed application-level buffering separately from partition rows and parquet-go internals.

### Custom temporary workspace

`--temp-dir DIR` places the deterministic partition/sort workspace under a caller-selected parent directory. This is particularly useful under WSL when input/output are on `/mnt/<drive>` but temporary I/O should be tested on the native Linux filesystem. `execution_context.temporary` describes the actual generated workspace, independently of the input and output paths.

No performance adjustment is made for WSL, 9P, filesystem type, cache state, antivirus, or storage hardware. Compare environments explicitly.

## Native Go heap snapshots

`--profile-memory DIR` enables opt-in retained-heap profiling using Go's native heap profiler. xqmob captures snapshots at fixed milestones:

```text
01-startup
02-post-partition
03-writers-open
04-after-25pct-partitions
05-after-50pct-partitions
06-after-75pct-partitions
07-pre-close
08-post-close
```

Before each snapshot xqmob forces a Go garbage collection so the heap profile reflects live retained allocations at a well-defined point. This is intentionally intrusive: a profiling run is diagnostic and its elapsed/phase throughput must not be compared directly with a normal benchmark run. `phase.heap_profile_seconds` records direct snapshot/GC time, but GC can also influence subsequent allocation and timing behavior.

Each entry under `stats.performance.memory.heap_profiles` records the snapshot label, path, file size, post-GC Go heap figures, and current RSS when available. The profile files are written to the caller-selected directory and are not included in xqmob output-storage expansion ratios.

Typical inspection:

```bash
xqmob eventize export_5m.csv --source test --out output --temp-dir ~/xqmob-tmp --profile-memory ~/xqmob-heaps
go tool pprof ./xqmob ~/xqmob-heaps/05-after-50pct-partitions.heap
```

Within pprof, `top`, `top -cum`, `list <function>` and `web`/`svg` (where Graphviz is available) can identify the allocation paths retaining memory. Heap profiling exists to guide implementation refactoring; it does not modify Event Model or analytical semantics.


## Bounded Parquet writer memory

Heap profiling of a 5-million-row run showed retained memory dominated by `parquet-go` dictionary-backed column buffers rather than xqmob eventizer state. The default writer previously allowed an effectively unlimited row group and unlimited per-column dictionary growth.

The v0.1.0 reference materialization therefore applies two explicit physical writer limits:

```text
MaxRowsPerRowGroup = 131072
DictionaryMaxBytes = 8 MiB per column per row group
```

`parquet-go` resets row-group state when the configured row limit is reached; when a column dictionary reaches the byte limit it falls back to PLAIN encoding for the remainder of that row group. These are storage/runtime controls and never alter canonical events, IDs, projection values, or geometry.

Advanced overrides:

```bash
xqmob eventize input.csv --source test \
  --parquet-row-group-rows 131072 \
  --parquet-dictionary-max-mib 8
```

A dictionary limit of `0` means unlimited and is intended only for comparison/testing. Row-group rows must be greater than zero.

The manifest records `stats.parquet.row_group_max_rows`, `stats.parquet.dictionary_max_bytes`, and per-file row/row-group counts. The same limits are written to Parquet key/value metadata. Closed writer/file/batch references are released immediately after close so post-close heap profiles do not retain closed Parquet writer structures unnecessarily.
