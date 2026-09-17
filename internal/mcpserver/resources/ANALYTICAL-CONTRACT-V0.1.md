# xqmob analytical contract v0.1

Contract identifier: `xqmob-analysis-v0.1`

Status: reference analytical projection contract for xqmob 0.1.0.

This contract defines how xqmob materializes an enriched mobility analysis layer around the canonical Crosscue Event Model stream. It is deliberately versioned separately from both the Core Event wire format and the mobility eventizer algorithm.

## 1. Separation of concerns

Three versioned surfaces are independent:

```text
Crosscue Event Model wire     0.1
Mobility eventizer algorithm  mobility-reference-v1.2
Analytical projection         xqmob-analysis-v0.1
```

The eventizer determines semantic events. The analytical contract determines how evidence and derived structures are exposed for analysis and visualisation. Changing analytical columns MUST NOT silently change eventizer semantics.

`canonical/events.jsonl` remains the canonical semantic stream. Parquet/GeoParquet layers are linked analytical projections and are not additional Core Event types.

## 2. Evidence-preserving principle

xqmob SHOULD preserve representable source evidence before imposing analyst-specific quality choices.

Horizontal accuracy (`ha`) is positional uncertainty attached to an observation. It is analytically important because mobility observations are probabilistic spatial measurements. It is not Event Model `confidence`.

The reference analytical behaviour is therefore:

- known HA is retained as supplied;
- missing HA is retained as unknown/NULL;
- missing HA is never imputed as zero or as a default radius;
- malformed, non-finite or negative HA is invalid because it is not representable as horizontal-accuracy evidence;
- `--ha-policy require` and `--max-ha-m` are explicit analyst-selected filters, not reference quality rules;
- derived objects expose the HA evidence that supported them so analysts can stratify or filter later.

The contract does not declare `all_known`, `mixed`, or `all_missing` objects to be good or bad. Those are descriptive evidence-support classes only.

## 3. Linked analytical hierarchy

The standard output exposes:

```text
entities
  └── tracks (logical; materialized only in full profile)
       └── segments
            ├── observations
            ├── semantic events
            ├── presence intervals
            └── transitions
```

Stable IDs link the layers. A user SHOULD be able to begin with the compact entity index and move toward the underlying observations without reconstructing identity or continuity relationships.

## 4. Accuracy-support vocabulary

Object-level support uses:

- `all_known`: every supporting observation has HA;
- `mixed`: both known and missing HA occur in the support set;
- `all_missing`: no supporting observation has HA;
- `none`: no applicable support set is asserted.

Observation adjacencies use:

- `both_known`;
- `previous_missing`;
- `current_missing`;
- `both_missing`.

Coverage percentages are descriptive ratios of known HA observations to all observations in the stated support set.

## 5. Layer responsibilities

### observations.parquet

The evidence-facing normalized observation layer. It preserves nullable `ha_m`, observation HA state, edge HA state, continuity calculations, trajectory membership and point geometry.

### events.parquet

A flattened analytical representation of canonical Core Events. It exists so SQL/GIS/visualisation tools do not need to parse nested JSON. Canonical JSONL remains authoritative.

### segments.parquet

The primary trajectory line layer in the standard profile. Segments are physically continuous portions separated by DISCONTINUITY. GAP-only edges do not create semantic segment breaks, but path geometry is split across GAPs so visualisation does not imply an observed route.

The layer exposes object-level HA coverage, endpoint HA state and counts of adjacency-support states. This allows uncertainty to be located within a segment rather than represented only by one aggregate percentage.

### presence_intervals.parquet

Stable-presence intervals with centroid geometry, duration, STAY/DWELL classification, observation scatter, nullable HA statistics, support coverage, endpoint HA state and explicit start/end reasons.

`candidate_presence` and `movement_arrival` MUST remain distinct because only the latter completes a confirmed movement transition.

### transitions.parquet

Confirmed movement between a closed origin presence interval and a subsequently confirmed destination presence interval.

Transition accuracy evidence is exposed in three non-additive support scopes:

1. **origin presence** — all observations in the closed origin presence interval;
2. **movement window** — observations from the confirmed departure point through the first destination-presence anchor;
3. **destination presence** — all observations in the subsequently closed destination presence interval.

The scopes intentionally overlap at their boundaries and MUST NOT be summed as if they were disjoint populations.

For backward compatibility, the existing transition fields:

```text
observations_with_ha
observations_without_ha
ha_coverage_pct
accuracy_support
```

continue to describe the **movement window**. New `origin_*`, `movement_*`, and `destination_*` fields make that meaning explicit.

### entities.parquet

A compact entity index intended for triage, discovery and `xqmob inspect`. It summarizes structure, temporal coverage, movement/presence counts, HA coverage/class and selected eventizer diagnostics.

### tracks.parquet

Optional full-profile materialization. Whole-track geometry is omitted from the standard profile because it largely duplicates segment paths.

## 6. Visualisation contract

Spatial projections use GeoParquet 1.1 metadata, WKB geometry and OGC:CRS84 longitude/latitude coordinates.

The standard profile SHOULD be directly useful for map visualisation without reconstructing geometry:

- observations: Point;
- events: Point where an event has a location;
- segments: LineString/MultiLineString-like WKB path representation as supported by the writer;
- presence intervals: Point;
- transitions: origin-to-destination LineString.

Geometry is a visualization/analysis materialization of linked evidence and MUST NOT be interpreted as adding certainty beyond the source observations.

## 7. Traceability and provenance

Every run records:

- effective eventizer configuration;
- eventizer version;
- analytical contract version;
- output profile and compression;
- storage accounting;
- eventizer diagnostics;
- accuracy-support summaries.

Parquet files carry `xq:eventizer` and `xq:analysis_contract` metadata.

Semantic IDs remain independent of CSV row ordering when normalized content is unchanged. Source row numbers are provenance, not identity.

## 8. Analytical freedom

Downstream analysts are free to filter or stratify using any combination of:

- raw HA values;
- missing/present HA;
- HA coverage;
- support class;
- time/geography;
- event type;
- entity/segment/presence/transition attributes;
- vendor/source-specific rules.

xqmob does not designate one such workflow as universally correct. The purpose of the enriched layer is to preserve enough evidence and attribution for those choices to be explicit and reproducible.

## 9. Versioning

Changes that add compatible projection columns may remain within `xqmob-analysis-v0.1` while xqmob is pre-1.0. A future change that redefines the meaning of an existing analytical field, changes support-set semantics, or materially alters layer relationships SHOULD increment the analytical contract identifier independently of the eventizer algorithm.
