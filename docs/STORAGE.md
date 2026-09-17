# xqmob storage and materialization policy

Storage efficiency is a first-class property of the analytical layer, but it must not be achieved by removing semantic enrichment or source traceability.

## Baseline

An early pre-release run without explicit Parquet compression over approximately 100,000 normalized AdTech rows produced roughly:

- source CSV: ~7 MB;
- complete output directory: ~80 MB;
- canonical `events.jsonl`: ~33 MB;
- analytical/materialized remainder: ~47 MB, or roughly 6–7× the source CSV.

These values are an empirical baseline rather than a normative size target. CSV size also depends heavily on ID length, timestamp representation and numeric precision.

That experimental Parquet writer did not explicitly select a compression codec. `parquet-go` defaults unspecified columns to uncompressed pages. The 0.1.0 reference baseline explicitly uses ZSTD by default.

## Compressed standard-profile baseline

A subsequent standard-profile run over a 99,726-row AdTech export produced:

- input CSV: 7.5 MiB;
- valid input rows: 84,251;
- rejected rows: 15,475;
- normalized observations after same-timestamp collapse: 82,932;
- canonical `events.jsonl`: 32.4 MiB (4.34× input);
- complete analytical layer: 7.6 MiB (1.02× input);
- measured total output artifacts: 41.4 MiB (5.55× input);
- analytical storage: 96.3 bytes per normalized observation.

This strongly suggests that ZSTD plus omission of redundant whole-track geometry solved the principal analytical-materialization inflation seen in the uncompressed experiment. In this run, the analytical layer was approximately the size of the source CSV while containing observations, flattened events, segments, presence intervals, transitions and entity summaries. Canonical JSONL remains the dominant storage component and is intentionally left uncompressed in the 0.1.0 reference baseline for observability.

## Principles

1. Preserve semantic enrichment and evidence traceability before optimizing representation.
2. Remove redundant physical materializations before removing useful analytical fields.
3. Keep eventization semantics independent of output/storage choices.
4. Measure every run so regressions can be compared across data sets and versions.
5. Prefer segments over complete track geometry as the default movement visualization layer.

## Output profiles

### standard

Default. Produces:

- canonical events JSONL;
- entities;
- observations;
- flattened events;
- segments;
- presence intervals;
- transitions.

It does not materialize `tracks.parquet`. Stable `track_id` values remain available in the other projections.

### full

Adds `tracks.parquet`, including complete dataset-bounded track geometry. Use this when whole-track rendering or a dedicated track relation is worth the additional duplication.

Invoke with either:

```bash
xqmob eventize input.csv --source example --output-profile full
```

or:

```bash
xqmob eventize input.csv --source example --full
```

## Compression

Default:

```text
parquet_compression = zstd
```

Supported values are `zstd`, `snappy`, and `none`. The selected codec is recorded in `manifest.json` and in Parquet key/value metadata as `xq:compression`.

Compression is a materialization choice only. It must not alter canonical events, IDs, projection values or geometry.

## Manifest accounting

`manifest.json` includes a `storage` object containing:

- `input_bytes`;
- `canonical_bytes`;
- `analysis_bytes`;
- `rejects_bytes`;
- `output_artifact_bytes_excluding_manifest`;
- canonical, analytical and total expansion ratios;
- analytical bytes per normalized observation;
- per-artifact byte counts sorted largest-first.

`manifest.json` itself and temporary partition files are deliberately excluded from the measured artifact total.

## Future optimization candidates

The following should be benchmarked against real exports before implementation:

- optional compression of canonical JSONL may be reconsidered later; v0.1.0 deliberately retains plain JSONL for direct observability;
- narrower analytical event representation for repeated provenance/context values;
- dictionary/delta/byte-stream encodings on high-volume columns;
- larger and explicitly tuned Parquet row groups/pages;
- partitioned analytical output for very large data sets;
- visualization-on-demand outputs such as PMTiles rather than permanently materializing every geometry layer;
- external merge sorting to bound memory independently of partition skew.

Any optimization should be evaluated using both bytes per normalized observation and analyst usability, not size alone.

## Pre-release HA-policy comparison baseline

The same 99,726-row export was run under both HA policies before the reference baseline was finalized.

`require`:

- normalized observations: 82,932;
- analytical layer: 7.7 MiB (1.04× input);
- analytical bytes per normalized observation: 97.6;
- canonical JSONL: 34.7 MiB.

`allow-missing` (now the compatibility alias for the default `preserve` policy):

- normalized observations: 98,048;
- analytical layer: 9.4 MiB (1.26× input);
- analytical bytes per normalized observation: 100.4;
- canonical JSONL: 42.8 MiB.

The analytical layer remains compact even when the missing-HA observations are retained. The much larger analytical effect of those observations (for example on discontinuities and transitions) is therefore treated as an evidence-quality/interpretation issue rather than a storage problem. The 0.1.0 reference baseline includes HA-support attribution so those effects can be examined without making HA filtering the reference ingestion policy.


## Parquet physical limits

A pre-release 5-million-row heap profile showed that Parquet dictionary buffers, not eventizer structures, dominated retained Go heap. The 0.1.0 reference baseline bounds row groups at 131,072 rows and per-column dictionaries at 8 MiB by default while retaining ZSTD compression. These controls are expected to trade a small amount of footer/row-group overhead for substantially lower writer memory. Per-file actual row-group counts are recorded in the manifest so file-size, queryability, and memory can be benchmarked together.
