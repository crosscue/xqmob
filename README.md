# xqmob

`xqmob` is a Go CLI that eventizes normalized AdTech / mobility observations into the Crosscue Event Model Mobility Profile 0.1 and materializes analysis-ready spatial projections.

Input is a CSV containing:

```text
id,ts,lat,lon,ha
```

where `ha` is horizontal accuracy in metres.

The implementation targets:

- Crosscue Event Model Core wire version `0.1`
- Mobility Profile `xq.mob:profile-0.1`
- reference algorithm `mobility-reference-v1.2`
- analytical projection contract `xqmob-analysis-v0.2`
- local MCP analyst contract `xqmob-mcp-v0.2`
- GeoParquet `1.1.0`, WKB geometry, default OGC:CRS84 longitude/latitude coordinates
- dual analytical spatial indexing: geohash plus H3 (default resolution 11)

The Event Model repository is: <https://github.com/crosscue/event-model>

## Reference implementation and status

`0.1.4-rc1` is the first public open-source release candidate. It packages the reference implementation for external review under Apache-2.0, with safer output replacement, checked ingestion writes and exact numeric timestamp parsing. Mobility thresholds and versioned eventizer rules are unchanged; corrected timestamp parsing can change results for inputs previously rounded or incorrectly accepted.

xqmob is a **reference implementation**, not the normative Crosscue Event Model specification. The specification/profile defines interoperability requirements; xqmob demonstrates one reproducible producer, analytical projection model and domain-specific MCP analyst interface. Alternative conformant implementations are expected and welcome.

The independently versioned contracts remain frozen for this release candidate:

- eventizer: `mobility-reference-v1.2`;
- analytical projections: `xqmob-analysis-v0.2`;
- MCP analyst interface: `xqmob-mcp-v0.2`;
- Event Model wire/profile: `0.1` / `xq.mob:profile-0.1`.

The MCP analyst interface remains backward-compatible with both `xqmob-analysis-v0.1` and `xqmob-analysis-v0.2` datasets. H3 tools are available only when H3 was materialized by analytical contract v0.2.

See `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, `CONTRIBUTING.md`, `SECURITY.md` and `docs/PUBLICATION_CHECKLIST.md` before publishing or redistributing release artifacts.

## Safety, privacy and dual use

xqmob eventizes location-time observations and materializes spatial analytical projections. That is analytically useful, but it can also increase the sensitivity of data and derived products. Combining otherwise ordinary traces can materially increase identifiability through correlation.

Implementers are responsible for appropriate legal authority, data minimisation, access controls, retention, audit, redaction, and handling of sensitive or personally identifying information. Pseudonymisation does not necessarily prevent re-identification when location and time are sufficiently distinctive. The MCP analyst interface queries a local dataset; it does not enforce access control, classification, or retention policy.

Examples and test fixtures in this repository are synthetic. Contributors MUST NOT submit customer data, operational intelligence, personal mobility traces, or other sensitive datasets as public fixtures.

See [`SECURITY.md`](SECURITY.md), [`CONTRIBUTING.md`](CONTRIBUTING.md) and the Event Model [`WHAT-THIS-IS-NOT.md`](https://github.com/crosscue/event-model/blob/main/WHAT-THIS-IS-NOT.md).

## Build

Go 1.25+ is required. xqmob embeds DuckDB for the MCP analyst interface, so the single MCP-capable binary also requires `CGO_ENABLED=1` and a suitable C toolchain at build time. The DuckDB Go client statically links its supported prebuilt DuckDB library into the executable.

```bash
go mod tidy
CGO_ENABLED=1 go build -o xqmob ./cmd/xqmob
```

The pinned non-standard-library dependencies are:

- `github.com/parquet-go/parquet-go` for Parquet/GeoParquet;
- `github.com/dimchansky/h3-go` for pure-Go H3 indexing;
- `github.com/duckdb/duckdb-go/v2` for read-only analytical queries in MCP mode;
- `github.com/modelcontextprotocol/go-sdk v1.7.0` for the MCP server/stdio transport and MCP 2026-07-28 compatibility.

The H3 implementation targets H3 Core 4.5.0. MCP uses DuckDB only at query time; ordinary eventization still writes the same canonical and analytical formats as v0.1.1.

GitHub Actions runs unit tests, `go vet`, nullable-HA Parquet round-trip integration, H3 reference compatibility, both output profiles, both HA policies, canonical validation, diagnostic/performance assertions, and MCP/DuckDB integration tests against the real pinned dependencies.

## Run

Either argument order is accepted:

```bash
./xqmob eventize input.csv --source vendor-a --out output
```

or:

```bash
./xqmob eventize --source vendor-a --out output input.csv
```

Then validate the canonical event stream:

```bash
./xqmob validate output/canonical/events.jsonl
```

The output directory must be new or empty. To explicitly replace a previous dataset:

```bash
./xqmob eventize input.csv --source vendor-a --out output --replace
```

`--replace` removes known dataset artifacts and the entire `diagnostics/` directory after input ingestion succeeds. A missing input, invalid header, strict-mode rejection or ingestion write failure leaves the previous dataset artifacts intact. Replacement is not transactional after ingestion: a later failure can leave partial new output. Keep a backup if the previous dataset must survive every failure. Unrelated files outside the known artifact paths are retained. With `--replace`, a custom `--temp-dir` must be outside the output diagnostics directory.

### Terminal output

Normal `eventize` output is intentionally concise and reports the operational result: normalized rows, analytical object counts, HA coverage, elapsed time/throughput, peak RSS when available, and canonical/analysis storage. Complete diagnostics are always retained in `manifest.json`.

Use:

```bash
./xqmob eventize input.csv --source vendor-a --out output --verbose
```

to print the full diagnostic/source-characterization report during eventization. After a run, use:

```bash
./xqmob inspect --diagnostics output
```

to print the same stored diagnostic detail without rerunning the eventizer. `xqmob inspect output` remains a compact dataset summary.

Version information is available with:

```bash
./xqmob --version
```

## Standard output

The default `standard` profile avoids materializing whole-track geometry because it substantially duplicates the more analytically useful segment paths:

```text
output/
├── manifest.json
├── rejects.csv
├── canonical/
│   └── events.jsonl
└── analysis/
    ├── entities.parquet
    ├── observations.parquet
    ├── events.parquet
    ├── segments.parquet
    ├── presence_intervals.parquet
    └── transitions.parquet
```

Use `--full` (equivalent to `--output-profile full`) to additionally create:

```text
analysis/tracks.parquet
```

`canonical/events.jsonl` is the semantic source of truth. The Parquet files are analytical projections designed for visualisation, filtering, SQL, GIS and downstream enrichment.

All spatial Parquet tables contain GeoParquet metadata and a `geometry` WKB column. Spatial analytical rows also carry H3 cell indexes alongside the existing geohash-oriented fields. `entities.parquet` is a non-spatial entity index but includes H3 start/end cells and a unique-H3-cell count. `track_id` remains stable and is retained throughout the projections even when `tracks.parquet` is not materialized.

## Why both canonical and analytical outputs?

The input is evidentially simple: time-stamped location observations. Eventization adds several linked analytical levels without discarding the underlying evidence:

```text
entities
  └── tracks
       └── segments
            ├── presence_intervals
            ├── transitions
            ├── events
            └── observations
```

This lets an analyst move from a high-level entity summary down to the exact observations that support a presence or transition.


## Inspecting an output

`xqmob inspect` provides a lightweight analytical summary without requiring separate Parquet tooling:

```bash
./xqmob inspect output
```

For one raw entity ID, xqmob reads the compact entity index only:

```bash
./xqmob inspect --entity entity-123 output
```

Use `--json` for a machine-readable report. Dataset-only inspection reads `manifest.json`; entity inspection reads `analysis/entities.parquet` and does not scan observations, events or geometry layers.

## MCP / LLM analyst interface

`xqmob mcp` exposes one existing xqmob output directory as a read-only, domain-specific MCP server over stdio:

```bash
./xqmob mcp output
```

DuckDB opens in memory and creates views directly over the analytical Parquet/GeoParquet files. The dataset is not imported, copied or modified. stdout is reserved exclusively for MCP protocol messages; server diagnostics go to stderr. The default DuckDB worker count is four and can be changed with `--duckdb-threads`.

The MCP contract deliberately exposes mobility concepts rather than arbitrary SQL:

```text
dataset_summary
search_entities
get_entity
get_entity_timeline
get_presence
get_transitions
get_segments
get_events
get_observations
describe_h3_cell
explain_event
explain_discontinuity
```

Collection tools default to 100 rows and hard-cap each response at 1000 rows using opaque cursors. `get_observations` requires an entity ID, or an H3 cell when H3 is materialized, so an LLM cannot accidentally request the complete observation table in one call. MCP can open both `xqmob-analysis-v0.1` and v0.2 datasets; `describe_h3_cell` is registered only for v0.2/H3-capable datasets.

Static MCP resources provide the manifest, dataset capabilities, analyst guidance, the analytical contract matching the opened dataset, spatial-indexing contract and eventizer contract. The analyst guidance preserves the evidence hierarchy: normalized observations are evidence; events, segments, presence intervals and transitions are derived interpretations.

A generic MCP-client configuration is conceptually:

```json
{
  "mcpServers": {
    "xqmob": {
      "command": "/path/to/xqmob",
      "args": ["mcp", "/path/to/output"]
    }
  }
}
```

Client configuration syntax varies by host. See [`docs/MCP.md`](docs/MCP.md) for the tool/resource contract and the intended xq-family pattern.

## Reference defaults

The defaults are those published by Mobility Profile 0.1:

| Parameter | Default |
|---|---:|
| `geohash_precision` | 8 |
| `move_radius_m` | 100 |
| `dwell_threshold_s` | 900 |
| `gap_threshold_s` | 7200 |
| `max_speed_mps` | 50 |
| `max_jump_m` | 50000 |
| `confirm_moves` | true |
| `confirm_window_s` | 900 |
| `walk_max_speed_mps` | 6 |
| `walk_max_jump_m` | 8000 |

Every emitted Core Event records the effective eventizer values in `provenance.parameters`. H3 is deliberately excluded because it is an analytical materialization choice, not an eventizer parameter.

Analytical spatial indexing defaults to H3 resolution **11** and can be changed with `--h3-resolution 0..15`. The chosen resolution is recorded in `manifest.json` and Parquet key/value metadata. Coarser H3 parents are intentionally not stored because they can be derived downstream.

## Spatial indexing: geohash + H3

xqmob deliberately carries both spatial index forms. Geohash remains useful for human inspection and as existing eventizer context; H3 is materialized for machine aggregation, neighbourhood analysis and origin/destination flows. Neither grid defines movement, presence or continuity: eventization remains based on metric distance, HA, time and kinematic thresholds.

Default analytical H3 resolution is 11:

```bash
./xqmob eventize input.csv --source vendor-a --out output --h3-resolution 11
```

Key projection fields include `h3_cell` on observations, located events and presence intervals; `origin_h3_cell` / `destination_h3_cell` / `same_h3_cell` on transitions; and start/end H3 cells plus unique-cell counts on entity/track summaries. See [`docs/SPATIAL-INDEXING.md`](docs/SPATIAL-INDEXING.md).

## Timestamp input

`ts` accepts RFC3339 timestamps or decimal Unix epoch values, including scientific notation. Numeric scale is inferred from absolute magnitude: below `1e11` means seconds, `1e11` to below `1e14` milliseconds, `1e14` to below `1e17` microseconds, and `1e17` or above nanoseconds. Numeric values must fit the signed 64-bit range before scaling and produce an RFC3339 year from 0000 to 9999. Non-finite and out-of-range values are rejected.

Integer epochs retain their exact precision, including nanoseconds. Fractions are parsed exactly and only precision finer than one nanosecond is rounded, with ties away from zero. Decimal inputs are limited to 128 characters and exponents from -400 to 400. Analytical Parquet timestamps have millisecond precision; canonical event timestamps retain nanosecond precision where supplied.

All emitted times are UTC RFC3339.

## Input quality

Invalid rows are quarantined to `rejects.csv`. Use `--strict` to stop at the first invalid row. Rejects carry a stable `reason_code` plus a human-readable `reason`, and counts by reason are written to `manifest.json` and printed at the end of the run. Malformed horizontal accuracy remains invalid (`non_numeric_ha`, `non_finite_ha`, `negative_ha`), while zero HA remains valid.

Horizontal accuracy is treated as evidence, not as an ingestion gate. The default `--ha-policy preserve` retains blank HA as unknown: `ha_m` is NULL in Parquet and Core `location.accuracy_m` is omitted when the event location is that source observation. `--ha-policy require` is retained as an explicit analyst-selected filter; `allow-missing` remains accepted as a compatibility alias for `preserve`. `--max-ha-m` is likewise an optional analyst-selected filter and is disabled by default. Missing HA is never imputed. Distance calculations subtract only accuracy radii that are actually known.

The analytical layers expose HA coverage and support classes so filtering can occur downstream without reinterpreting missing values. See [`docs/ACCURACY.md`](docs/ACCURACY.md) and [`docs/ANALYTICAL-CONTRACT.md`](docs/ANALYTICAL-CONTRACT.md).

Identical timestamps for the same entity are collapsed deterministically by preferring a fix with known HA, then selecting the lowest known horizontal accuracy value, then latitude and longitude. Source row is only a final provenance tie-break when normalized content is otherwise identical. Semantic IDs do not depend on CSV row position, so reordering an otherwise identical export preserves event/track/segment/presence/transition IDs. The analytical observation records the number of collapsed samples and their spatial scatter.

## Large inputs

The first pass streams the CSV into deterministic hash partitions by `id`. Each partition is then sorted by entity and timestamp before eventization. This avoids holding the entire source file in memory and guarantees that all observations for an entity are owned by one partition.

The current v0.1 partition sorter loads one partition at a time in memory. It is therefore bounded by the largest partition, not by the whole source file. A later implementation can replace the per-partition sort with sorted runs + merge without changing event semantics.

By default the temporary partition workspace is created under the output directory. Use `--temp-dir DIR` to place it on another filesystem, for example `~/xqmob-tmp` under WSL while durable output remains under `/mnt/d`. The actual input/output/temp filesystem context is recorded in `manifest.json`. Use `--keep-temp` only when the generated partition files are needed for debugging.

For retained-heap diagnosis, `--profile-memory DIR` writes native Go heap profiles at fixed milestones (startup, post-partition, writers-open, 25/50/75% of partitions, pre-close and post-close). Profiling forces a GC before each snapshot so these runs are diagnostic; do not compare their elapsed timings directly with ordinary benchmark runs. Inspect snapshots with `go tool pprof ./xqmob DIR/<snapshot>.heap`.


### Source and memory characterization

The 0.1.0 reference baseline includes diagnostic characterization of the structured `both_missing + speed_only + dt<5s + 50-500m` population observed in large exports, coordinate-precision transitions, and immediate A→B→A reversal diagnostics. It also uses bounded Parquet row groups/dictionaries to address writer-memory growth observed at 5 million rows. These diagnostic and physical storage features do not change eventizer rules.

Performance output also includes sampled RSS/Go-heap maxima by phase plus maximum partition/entity working-set counts. These are observability aids, not benchmark normalization or quality labels. See [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md) and [`docs/DIAGNOSTICS.md`](docs/DIAGNOSTICS.md).

### Output size and compression

All Parquet/GeoParquet projections now use explicit **ZSTD page compression by default**. `parquet-go` otherwise defaults to uncompressed pages, which made early output-size baselines unnecessarily large.

Alternative codecs are available for benchmarking or interoperability:

```bash
./xqmob eventize input.csv --source vendor-a --parquet-compression zstd
./xqmob eventize input.csv --source vendor-a --parquet-compression snappy
./xqmob eventize input.csv --source vendor-a --parquet-compression none
```

The v0.1.0 reference release bounds Parquet physical buffering explicitly:

```text
parquet_row_group_rows       = 131072
parquet_dictionary_max_bytes = 8388608   # 8 MiB per column per row group
```

These settings are physical materialization controls only. They do not alter Event Model semantics or analytical values. Advanced benchmarking overrides are available with `--parquet-row-group-rows` and `--parquet-dictionary-max-mib`; a dictionary limit of `0` means unlimited. The selected limits and actual row-group counts per analytical file are recorded in `manifest.json` and Parquet key/value metadata.

Every run records per-file byte counts and the following aggregate metrics in `manifest.json` and prints them at completion:

- input bytes;
- canonical JSONL bytes and expansion ratio;
- analytical projection bytes and expansion ratio;
- total measured artifact bytes and expansion ratio;
- analytical bytes per normalized observation.

The measurement intentionally excludes `manifest.json` itself and temporary partition files. This makes output growth a regression metric rather than an anecdotal observation. See [`docs/STORAGE.md`](docs/STORAGE.md).

### Eventizer observability

`xqmob` also reports why the state machine produced its results. The manifest and CLI include counters for same-timestamp collapse, GAP/DISCONTINUITY edges, presence start/end reasons, departure candidates, confirmed departures, transition cancellations and unresolved transitions. Selected counters are also present per entity in `entities.parquet`.

Accuracy attribution covers key decisions: continuous/GAP/DISCONTINUITY edges, departure candidates, confirmed departures and confirmed transitions are counted by the HA support available to the relevant edge or transition. This makes the effect of unknown HA observable without forcing an HA filter.

This is particularly useful for sparse AdTech data: a low transition count can be distinguished between a genuine lack of confirmed movement, presence intervals terminated by gaps/discontinuities, departure candidates failing confirmation, and pending transitions that never reach a stable destination. Departure-candidate outcome counters are designed to close exactly to the number of candidate sequences. See [`docs/DIAGNOSTICS.md`](docs/DIAGNOSTICS.md).

For a specific entity, repeatable `--trace-id` flags write opt-in per-observation decision traces under `diagnostics/traces/` without creating a global decision-log layer:

```bash
./xqmob eventize input.csv --source vendor-a --out output --trace-id entity-123
```

Trace storage is separately accounted as diagnostic bytes and is excluded from the analytical expansion ratio.

### Performance and scale observability

The 0.1.0 reference baseline includes a scale envelope in `manifest.json`: elapsed wall time, input/normalized/event throughput, Linux peak RSS when available, deterministic partition-workspace high-water bytes, and phase timings for partition/read/sort/eventize/validate/write/close/storage accounting. It also includes step-time quantiles, fixed time-delta buckets, discontinuity reason × HA × time attribution, speed-only displacement distributions, explicit WSL/filesystem path context, speed-only `dt × effective-distance` matrices, HA-stratified matrices, and a provisional diagnostic `micro_displacement` class. `xqmob inspect` surfaces the same summary.

It also records approximate p50/p90/p99 step distributions by HA edge state for raw distance, uncertainty-adjusted effective distance and implied speed. These use constant-memory streaming estimators rather than retaining a diagnostic copy of every step.

DISCONTINUITY edges are now diagnostically attributed as `jump_only`, `speed_only`, or `jump_and_speed`, with each reason cross-tabulated by HA edge state. This is intended to explain source/evidence regimes without changing eventization or imposing an HA filter. See [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md).

Diagnostics attribute discontinuities by fixed time-delta bands and record `step_dt_s` quantiles. Performance manifests also identify WSL and input/output/temp mount/filesystem context; this is particularly useful when benchmarking outputs under `/mnt/c` or `/mnt/d`, where Windows-mounted filesystem overhead may differ from the WSL/Linux filesystem. xqmob records this context but applies no timing correction.

The joint diagnostic matrix for `speed_only` discontinuities uses fixed time and effective-distance bands. The provisional `micro_displacement` label (`dt < 1s`, effective displacement `<5m`) is diagnostic only: such observations still produce the same DISCONTINUITY and segment boundary under `mobility-reference-v1.2`.

Precision-transition cross-tabs describe the structured source population alongside consecutive-edge reversal diagnostics (`exact A→B→A`, return-within-5m, near-opposite bearing, near-symmetric distance, and precision A→B→A). These are source-characterization labels only. Optional `--profile-memory` snapshots expose retained Go heap allocation paths for implementation optimization without changing output semantics.

## Important semantic choices

- horizontal accuracy is positional uncertainty, not Event Model `confidence`, and is preserved as first-class analytical evidence rather than filtered by default;
- movement and continuity tests use an uncertainty-adjusted distance that subtracts only known HA radii; with both HA values known this is `max(0, centre_distance - ha1 - ha2)`;
- DISCONTINUITY magnitude remains raw centre-to-centre jump distance, as required by the profile;
- GAP and DISCONTINUITY are independent and may both occur on the same adjacency;
- DISCONTINUITY starts a new segment; GAP alone does not;
- gaps and discontinuities terminate an asserted presence interval but do **not** manufacture LEAVE or ENTER events;
- START/END are dataset bookkeeping boundaries and do not imply physical appearance/disappearance;
- presence at the start/end of a dataset is not given synthetic ENTER/LEAVE events;
- a confirmed movement is required before LEAVE; ENTER is emitted only when stable destination presence completes the corresponding pending transition;
- stable presence discovered without a confirmed prior departure is retained but does not receive a synthetic ENTER.

See [`docs/MOBILITY-EVENTIZER-V1.2.md`](docs/MOBILITY-EVENTIZER-V1.2.md) for the current algorithm contract. Earlier v1/v1.1 documents are retained for comparison.

## Conformance

`eventize` runs an internal Mobility Profile 0.1 semantic check before writing generated Core Events, and `xqmob validate` can re-check a JSONL stream. This is intentionally a focused producer-side guard, not a replacement for the Event Model repository's authoritative layered schema/vocabulary/profile conformance tooling.

## Visualisation

The GeoParquet projections are intended to load directly into tools that understand GeoParquet/WKB. Typical styling:

- `observations`: small points, filter by entity/time/HA/`ha_state`/`step_accuracy_state`;
- `events`: colour by `event_type`, optionally filter by `accuracy_support`;
- `tracks` (full profile only): entity-level trajectory lines;
- `segments`: continuous trajectory portions, with HA coverage, endpoint state and edge-support attributes;
- `presence_intervals`: points sized by `duration_s`, colour by `classification`, optionally filter by HA support/coverage;
- `transitions`: origin/destination lines between confirmed presence intervals, with separate origin/movement/destination HA-support attributes.

The flattened `events.parquet` exists specifically so visualisation tools do not need to understand nested Core Event JSON.

## CLI flags

Run:

```bash
./xqmob eventize -h
```

Key materialization/storage flags are:

```text
--output-profile standard|full
--replace                      # explicitly replace an existing dataset
--full                         # shorthand for full
--parquet-compression zstd|snappy|none
--parquet-row-group-rows 131072
--parquet-dictionary-max-mib 8
--ha-policy preserve|require     # allow-missing remains an alias
--trace-id ID                    # repeatable targeted decision trace
--temp-dir DIR                   # optional spill/sort workspace parent
--profile-memory DIR             # opt-in Go heap snapshots; perturbs timings
--verbose                        # print full diagnostics during eventization
```

MCP mode has a deliberately small operational surface:

```text
xqmob mcp [--duckdb-threads N] OUTPUT
```

`xqmob inspect --diagnostics OUTPUT` prints the full stored diagnostic report, while `xqmob inspect OUTPUT` remains concise. The manifest also records `analysis_contract=xqmob-analysis-v0.2`. All output/storage choices are recorded in `manifest.json`; tracing and materialization choices do not alter event semantics. Reusing a non-empty output directory requires `--replace`. After successful ingestion, replacement removes known prior artifacts and stale diagnostics so a full-profile `tracks.parquet` or trace cannot leak into a later run.


## Open-source project files

- [`CONTRIBUTING.md`](CONTRIBUTING.md) — contribution and compatibility expectations;
- [`SECURITY.md`](SECURITY.md) — private vulnerability reporting guidance;
- [`GOVERNANCE.md`](GOVERNANCE.md) — current maintainer-led governance model;
- [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) — participation expectations;
- [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) — direct dependency licensing notes;
- [`TRADEMARKS.md`](TRADEMARKS.md) — project-name and branding clarification;
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — high-level system boundaries;
- [`docs/PUBLICATION_CHECKLIST.md`](docs/PUBLICATION_CHECKLIST.md) — release-candidate promotion checklist.

## License

xqmob is licensed under the Apache License, Version 2.0. See [`LICENSE`](LICENSE). Third-party components remain under their respective licences; see [`NOTICE`](NOTICE) and [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
