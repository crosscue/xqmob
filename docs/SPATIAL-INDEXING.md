# xqmob spatial indexing

Status: xqmob 0.1.4-rc1 / `xqmob-analysis-v0.2`; H3 projections introduced in xqmob 0.1.1.

## Purpose

xqmob carries two discrete spatial indexes because they serve different analytical roles:

- **geohash** is compact, hierarchical by string prefix, and useful for human inspection/debugging;
- **H3** is a hierarchical hexagonal discrete global grid suited to aggregation, neighbourhood operations and origin/destination flow analysis.

Neither grid is the movement model. The reference eventizer uses metric/geodesic distance, horizontal accuracy, elapsed time and configured kinematic thresholds. Crossing a geohash or H3 boundary alone never creates an event.

## H3 materialization

The reference default is H3 resolution 11. The CLI accepts `--h3-resolution 0..15`. The selected resolution is recorded in `manifest.json` as `config.h3_resolution` and in each Parquet file as `xq:h3_resolution`.

H3 indexes are written as the standard hexadecimal string form. Column names remain stable (`h3_cell`) when resolution changes. Coarser parents are not stored because they are directly derivable from the materialized cell; this avoids redundant columns and lets analysts choose aggregation resolution downstream.

The implementation uses pure-Go `github.com/dimchansky/h3-go v0.4.0`, behaviorally compatible with H3 Core 4.5.0. This preserves xqmob's no-cgo build model.

## Projection fields

- `observations.parquet`: `geohash`, `h3_cell`
- `events.parquet`: `h3_cell` when the canonical event has a location
- `presence_intervals.parquet`: `h3_cell` for the representative centroid
- `transitions.parquet`: `origin_h3_cell`, `destination_h3_cell`, `same_h3_cell`
- `segments.parquet`: `start_h3_cell`, `end_h3_cell`
- `entities.parquet`: `start_h3_cell`, `end_h3_cell`, `unique_h3_cells`
- `tracks.parquet` (full profile): `start_h3_cell`, `end_h3_cell`, `unique_h3_cells`

## Accuracy

H3 resolution is an analytical discretization and MUST NOT be interpreted as positional uncertainty. `ha` remains the source field describing horizontal accuracy. xqmob does not select H3 resolution from HA and does not replace missing HA with a cell-size assumption.

## Flow analysis

The transition projection makes H3 useful without requiring analysts to reconstruct origin/destination cells from geometry. Typical grouping is simply by `origin_h3_cell` and `destination_h3_cell`. `same_h3_cell=true` identifies transitions whose confirmed origin and destination fall in the same selected-resolution cell; this does not invalidate the transition because eventization operates continuously rather than on cell boundaries.
