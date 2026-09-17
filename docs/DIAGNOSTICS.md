# xqmob eventizer diagnostics

Status: diagnostic contract for xqmob 0.1.4-rc1 / `mobility-reference-v1.2`.

Diagnostics explain eventizer decisions; they are not Crosscue Core Events. Aggregate counters are written under:

```text
manifest.json -> stats.diagnostics
```

Selected entity-level counters are also materialized in `analysis/entities.parquet`. Targeted decision traces are opt-in.

## Reject diagnostics

`rejects.csv` contains stable `reason_code` values and a human-readable `reason`. Counts are aggregated in the manifest and printed by the CLI.

HA handling in the 0.1.0 reference baseline and subsequent releases is preservation-first:

- blank/missing HA is retained under the default `ha_policy=preserve`;
- `missing_ha` occurs only when the analyst explicitly selects `ha_policy=require`;
- `non_numeric_ha`: HA cannot be parsed as a number;
- `non_finite_ha`: NaN or infinity;
- `negative_ha`: HA is below zero;
- `ha_exceeds_max`: known HA exceeds an explicit `--max-ha-m` filter.

Zero HA remains valid. Missing HA is never substituted.

Other reject codes include `empty_id`, `invalid_ts`, `invalid_lat`, `invalid_lon`, and `csv_parse_error`.

## Continuity diagnostics

Counters include:

- `same_timestamp_collapsed`;
- `continuous_edges`;
- `gap_edges`;
- `discontinuity_edges`;
- `gap_and_discontinuity_edges`.

For a complete run:

```text
continuous_edges + gap_edges + discontinuity_edges - gap_and_discontinuity_edges
= normalized_observations - entities
```

## Presence diagnostics

Presence confirmations are classified by how the presence search began:

- dataset start;
- continuity reset after GAP/DISCONTINUITY;
- candidate presence without a known departure;
- movement arrival following a confirmed departure.

Presence endings are classified as confirmed movement, GAP, DISCONTINUITY or dataset end.

`enter_suppressed_no_pending_transition` counts stable presence assertions for which ENTER was deliberately not emitted because no confirmed prior departure/pending transition supported an arrival assertion.

## Departure candidate diagnostics

When move confirmation is enabled, every departure-candidate sequence ends in exactly one outcome:

- `departures_confirmed`;
- `departure_candidates_returned_inside`;
- `departure_candidate_window_expired`;
- `departure_candidates_cancelled_gap`;
- `departure_candidates_cancelled_discontinuity`;
- `departure_candidates_cancelled_eof`.

The closure invariant is:

```text
departure_candidate_sequences
= departures_confirmed
+ departure_candidates_returned_inside
+ departure_candidate_window_expired
+ departure_candidates_cancelled_gap
+ departure_candidates_cancelled_discontinuity
+ departure_candidates_cancelled_eof
```

`departure_candidate_unaccounted` should therefore be zero.

## Transition diagnostics

A confirmed departure creates one pending transition. It ends as one of:

- confirmed destination/transition;
- cancelled by GAP;
- cancelled by DISCONTINUITY;
- cancelled by defensive segment mismatch;
- unresolved at EOF.

`pending_transition_unaccounted` should be zero after entity EOF. The CLI reports confirmed/pending completion percentage.

## Accuracy attribution

`stats.diagnostics.eventizer.accuracy_attribution` describes the HA evidence available to decisions; it is not a threshold or confidence score.

The following decision groups are attributed:

```text
continuous_edges
gap_edges
discontinuity_edges
departure_candidates
departures_confirmed
transitions_confirmed
```

Edge-based groups use:

- `both_known`;
- `previous_missing`;
- `current_missing`;
- `both_missing`.

Confirmed transitions use aggregate support across their supporting observations:

- `all_known`;
- `mixed`;
- `all_missing`.

The CLI prints these distributions after the run-level HA coverage summary. They are intended to answer questions such as whether discontinuities or movement assertions disproportionately involve unknown HA without requiring the source data to be filtered first.

## Accuracy population summaries

The run accuracy summary also counts entities, presence intervals and transitions by `all_known`, `mixed`, and `all_missing`. This lets an analyst stratify the enriched layer by evidence support.

## Targeted entity traces

Use repeatable `--trace-id` flags to write per-observation decision traces:

```bash
xqmob eventize input.csv --source vendor-a --out output \
  --trace-id entity-123 \
  --trace-id entity-456
```

Trace files are written under `diagnostics/traces/` and record source row/time, coordinates, nullable HA state, step distance/effective distance, continuity flags, eventizer state, candidate/pending-transition state, decision code and emitted semantic events.

Trace storage is accounted separately from the analytical layer.

## Interpretation discipline

Diagnostics describe how a parameterized run behaved. They should make data quality and uncertainty visible before an analyst changes thresholds or filters.

`mobility-reference-v1.2` does not synthesize Event Model confidence from horizontal accuracy. The reference CLI now preserves missing HA by default and exposes its relationship to eventizer decisions as analytical evidence.

## Discontinuity reason attribution

Each discontinuity is classified diagnostically as `jump_only`, `speed_only`, or `jump_and_speed` using the same effective-distance and implied-speed tests that already determine continuity. The reason counts must sum to `discontinuity_edges`.

`discontinuity_reason_accuracy` cross-tabulates each reason by `both_known`, `previous_missing`, `current_missing`, and `both_missing`. These fields explain the evidence regime associated with a continuity break; they do not change the break itself.

## Step distributions and performance

Diagnostics also record constant-memory approximate p50/p90/p99 distributions for raw distance, effective distance and implied speed by HA edge state, plus wall-time/throughput/phase, peak-RSS and temporary-workspace measurements. See [`PERFORMANCE.md`](PERFORMANCE.md).

## Temporal diagnostics

Descriptive time-delta diagnostics were motivated by a pre-release 100k-row run in which most discontinuities were `speed_only`, including many both-missing-HA edges with small effective displacement. The goal is to expose whether very short observation intervals are turning positional jitter into extreme implied speed.

`stats.step_distributions.step_dt_s` records approximate p50/p90/p99 `dt` by HA edge state. `stats.temporal_diagnostics` additionally records:

- `edge_dt_buckets`: all entity-adjacent edges by HA state and fixed `dt` band;
- `discontinuity_dt_buckets`: discontinuity edges by HA state and fixed `dt` band;
- `discontinuity_reason_accuracy_dt`: discontinuity reason × HA state × `dt` band;
- `speed_only_effective_distance_m`: approximate effective-distance p50/p90/p99 for speed-only discontinuities by HA state.

The fixed bands are `<1s`, `1-5s`, `5-30s`, `30-60s`, `1-5m`, `5-30m`, `30m-2h`, and `>=2h`; a defensive `non_positive` bucket should normally remain empty. These are diagnostic groupings, not eventization thresholds.

All temporal discontinuity counts are designed to close to the existing discontinuity counters. These diagnostics do not change event semantics, HA policy, speed thresholds, jump thresholds or gap thresholds.


## Speed-only time/displacement diagnostics

The joint diagnostic view of `speed_only` discontinuities distinguishes small spatial changes over very short intervals from materially large movement that happens to remain below `max_jump_m`.

`stats.temporal_diagnostics.speed_only_dt_distance` cross-tabulates `speed_only` edges by the existing time bands and these effective-distance bands:

- `lt_5m`
- `5_50m`
- `50_500m`
- `500m_5km`
- `ge_5km`

`stats.temporal_diagnostics.speed_only_accuracy_dt_distance` adds the existing HA edge state as a third dimension.

`stats.temporal_diagnostics.speed_only_candidate_classes` also exposes a deliberately provisional diagnostic label:

- `micro_displacement`: `speed_only`, `dt < 1s`, and effective displacement `< 5m`;
- `other_speed_only`: every other `speed_only` discontinuity.

These classes are descriptive only. A `micro_displacement` case is still a DISCONTINUITY and still starts a new segment exactly as in `mobility-reference-v1.2`. The label does not assert noise, jitter, bad data, or a preferred analyst filter.

The aggregate matrix, HA-stratified matrix, and candidate classes are each required to sum exactly to the existing `speed_only` discontinuity count.

## Structured source characterization

A pre-release 5-million-row run exposed a large, tightly structured both-missing-HA speed-only population. `stats.spatial_diagnostics` provides descriptive source characterization of that population.

The first target population is fixed as:

```text
step_accuracy_state = both_missing
reason = speed_only
0 < step_dt_s < 5
effective_distance_m >= 50
effective_distance_m < 500
```

For this population xqmob records distance/time quantiles, absolute coordinate deltas, 16-way bearing, rounded 1-m distance modes, 10-m east/north displacement vectors, and source lexical coordinate decimal precision. These diagnostics are designed to reveal regular spatial quantization, preferred directions, coordinate-resolution effects, or other structured source regimes without asserting a cause.

They do not change `DISCONTINUITY`, segmentation, event IDs, thresholds, HA policy, or any Core Event field.

The 1-m distance and 10-m vector histograms are retained in full in `manifest.json`; the CLI and `inspect` print only the ten most frequent bins for readability.

## Precision transitions and immediate reversals

Precision-transition and reversal diagnostics describe the structured source-characterization population without altering `mobility-reference-v1.2` decisions. For each target edge, the manifest records the lexical coordinate-precision transition (`previous_precision -> current_precision`) and cross-tabulates that transition against:

- 16-sector bearing;
- rounded 1-m effective-distance mode;
- rounded 10-m east/north displacement vector.

These fields are intended to test whether a repeated spatial mode coincides with changes between source coordinate representations. They describe the input representation; they do not assert quantization, privacy transformation, vendor switching, positioning technology, or any other cause.

For consecutive target-population edges, xqmob also reports immediate reversal diagnostics:

- `consecutive_target_edge_pairs`;
- `exact_a_b_a_returns`: the third coordinate exactly equals the first parsed coordinate;
- `return_within_5m`: first-to-third great-circle distance is less than 5 m;
- `near_opposite_bearing_22_5deg`: consecutive bearings differ from 180 degrees by no more than 22.5 degrees;
- `near_symmetric_distance_within_5m`: the two target-edge effective distances differ by no more than 5 m;
- `a_b_a_like_pairs`: return-within-5m, near-opposite bearing, and near-symmetric distance all hold;
- `precision_a_b_a_returns`: first and third lexical precision classes match and differ from the middle precision class;
- `precision_triplets`: full lexical precision A→B→C counts.

The 5 m and 22.5-degree values are fixed diagnostic bands. They are not movement, continuity, accuracy, or eventization thresholds. No reversal diagnostic changes `DISCONTINUITY`, segment boundaries, event IDs, or Core Event output.
