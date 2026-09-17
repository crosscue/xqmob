# xqmob analytical projections

The Parquet outputs are materialized analytical projections, not additional Crosscue Core Event types. Horizontal accuracy is retained as analytical evidence; the projections expose both raw/null HA and aggregate support attributes so downstream users can choose their own filtering strategy.

## observations.parquet

One row per normalized/collapsed observation. Includes source evidence location, nullable horizontal accuracy, `ha_state`, per-edge `step_accuracy_state`, geohash, `h3_cell`, continuity calculations, segment/presence assignment and WKB Point geometry.

This is the primary evidence-facing analytical layer. Missing HA is represented as NULL, not zero.

## events.parquet

Flattened representation of `canonical/events.jsonl`, including semantic feature/action/state/polarity, event type, magnitude, IDs, geometry, context JSON and effective eventizer parameters.

Accuracy convenience fields are:

```text
accuracy_state
accuracy_support
accuracy_known_count
accuracy_missing_count
```

For observation/edge/presence events these are derived from the canonical context rather than from a synthesized confidence score. Located event rows additionally materialize `h3_cell`; this is derived from the event location and is not added to canonical JSONL.

## tracks.parquet

**Full output profile only.** One dataset-bounded track per entity ID, including coverage, counts, path metrics, HA coverage, `ha_class`, geohash/H3 spatial diversity, start/end H3 cells and WKB trajectory geometry.

The standard profile deliberately omits this projection because complete track paths substantially duplicate segment geometry. Stable `track_id` values remain present throughout the other projections.

## segments.parquet

Continuous portions of tracks separated by DISCONTINUITY events. Includes bounds, path/displacement metrics, gap counts and WKB trajectory geometry, plus:

```text
observations_with_ha
observations_without_ha
ha_coverage_pct
accuracy_support
start_ha_state
end_ha_state
edge_both_known_count
edge_previous_missing_count
edge_current_missing_count
edge_both_missing_count
start_h3_cell
end_h3_cell
```

## presence_intervals.parquet

One row per closed stable-presence interval. Includes centroid Point geometry, duration, STAY/DWELL classification, supporting observation count, scatter, nullable horizontal-accuracy statistics, HA coverage, `accuracy_support`, `start_ha_state`, and `end_ha_state`.

`start_reason` distinguishes:

- `dataset_start`;
- `continuity_reset`;
- `candidate_presence`;
- `movement_arrival`.

This preserves the epistemic distinction between stable presence discovered without a known prior departure and presence that completes a confirmed movement.

## transitions.parquet

Confirmed movement from an origin presence interval to a subsequently confirmed destination presence interval. Includes origin/destination IDs, timing, straight-line distance, observed path distance and LineString geometry.

It retains the compatibility fields:

```text
observations_with_ha
observations_without_ha
ha_coverage_pct
accuracy_support
```

which cover the movement window from the confirmed departure point through the first destination-presence anchor. It additionally exposes:

```text
origin_observations_with_ha
origin_observations_without_ha
origin_ha_coverage_pct
origin_accuracy_support

movement_observations_with_ha
movement_observations_without_ha
movement_ha_coverage_pct
movement_accuracy_support

destination_observations_with_ha
destination_observations_without_ha
destination_ha_coverage_pct
destination_accuracy_support
```

Transition spatial indexing includes `origin_h3_cell`, `destination_h3_cell`, and `same_h3_cell`, enabling direct cell-to-cell flow aggregation at the configured resolution.

Origin and destination scopes use the full linked presence intervals. The scopes overlap and are not additive. They are intended for analytical filtering/stratification, not automatic exclusion.

## entities.parquet

One row per entity. Provides a compact index for triage and analytical discovery before loading detailed observation data.

Accuracy fields include:

```text
observations_with_ha
observations_without_ha
ha_coverage_pct
median_ha_m
ha_class
```

`ha_class` is `all_known`, `mixed`, or `all_missing`. The entity index also carries `start_h3_cell`, `end_h3_cell`, and `unique_h3_cells`.

The entity projection also carries selected eventizer diagnostic counters so sparse or discontinuous entities can be identified without loading detailed layers.

## Analytical contract

The projection semantics in this document are governed by `xqmob-analysis-v0.2`; see `ANALYTICAL-CONTRACT.md`. The contract is versioned independently from the eventizer algorithm.

## Traceability

All projection rows carry the eventizer version directly or are linked to rows that do. `manifest.json` records the complete effective configuration for the run. Targeted `--trace-id` CSVs under `diagnostics/traces/` are diagnostic artifacts, not analytical projections.

## Materialization metadata

The configured H3 resolution and pinned H3 implementation/Core versions are stored as Parquet key/value metadata. H3 is an analytical materialization and does not appear in Core provenance parameters.

All Parquet projections use the configured page compression codec (ZSTD by default). `manifest.json` records the output profile, compression codec and per-layer byte counts. These are storage/materialization properties and do not change the eventizer contract.

## Horizontal-accuracy semantics

The default `ha_policy=preserve` keeps missing HA as NULL. Aggregate HA statistics use only known values. Coverage/support fields describe the evidence available to each analytical object. `ha_policy=require` and `max_ha_m` are opt-in analyst-selected filters rather than reference defaults.

See `ACCURACY.md` for the full model.
