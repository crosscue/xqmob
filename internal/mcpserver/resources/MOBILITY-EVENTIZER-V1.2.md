# Crosscue Mobility Eventizer Algorithm v1.2

Status: implementation contract for `xqmob` `mobility-reference-v1.2`.

This document defines the deterministic processing choices that are intentionally not normative in Crosscue Event Model Mobility Profile 0.1.

## 1. Purpose

Mobility Profile 0.1 defines the semantic meaning and required mappings of START, STAY, DWELL, LEAVE, ENTER, GAP, DISCONTINUITY and END. It intentionally does not fully define how observations become those events.

`mobility-reference-v1.2` fills that gap for `xqmob` without modifying Core 0.1 or Mobility Profile 0.1. It retains the conservative ENTER correction from v1.1 and adds an explicit, provenance-recorded policy for observations whose horizontal accuracy is missing.

## 2. Input contract

Input observations contain:

- `id`: source-scoped entity identifier;
- `ts`: observation time;
- `lat`, `lon`: WGS84 geographic coordinates;
- `ha`: horizontal accuracy radius in metres. The column is required; individual values may be blank when `ha_policy=preserve`.

Coordinates are longitude/latitude when materialized as WKB/GeoParquet geometry.

## 3. Ordering and timestamp collapse

Observations are sorted by:

1. entity `id`;
2. timestamp ascending;
3. known horizontal accuracy before missing horizontal accuracy;
4. known horizontal accuracy ascending;
5. latitude ascending;
6. longitude ascending;
7. source row ascending (provenance-only final tie-break).

For multiple observations of the same entity at the exact same timestamp, one normalized observation is retained:

1. prefer an observation with known `ha` over one whose `ha` is missing;
2. when multiple observations have known HA, choose the smallest valid `ha`;
3. break remaining ties by latitude, then longitude;
4. use source row only as a final tie-break when normalized content is otherwise identical.

Semantic IDs do not depend on source row numbers, so reordering an otherwise identical export does not alter event/track/segment/presence/transition IDs.

The normalized observation records `sample_count` and `same_ts_scatter_m`.

## 4. Distance and horizontal accuracy

Raw centre-to-centre distance uses the haversine formula with mean Earth radius 6,371,008.8 m.

Horizontal accuracy handling is controlled independently from the distance policy:

```text
ha_policy = preserve | require
```

`preserve` accepts a blank HA value but does not impute one. The observation carries `ha_state=missing`; analytical `ha_m` is NULL; and Core `location.accuracy_m` is omitted when the event location is that observation. The 0.1.0 reference baseline and subsequent releases use `preserve` as the CLI default.

`require` rejects a blank HA value and exists as an explicit analyst-selected filter. The legacy value `allow-missing` is a compatibility alias for `preserve`.

The default uncertainty-aware distance policy remains:

```text
accuracy_policy = subtract_radii
```

Only known radii are subtracted:

```text
both known:       effective = max(0, raw - ha_previous - ha_current)
previous missing: effective = max(0, raw - ha_current)
current missing:  effective = max(0, raw - ha_previous)
both missing:     effective = raw
```

This does **not** mean that missing HA represents zero positional uncertainty. It means the eventizer does not invent an uncertainty radius that was not present in the evidence. `step_accuracy_state` records `both_known`, `previous_missing`, `current_missing`, or `both_missing` for each adjacency.

`ha` is not mapped to Core `confidence`.

## 5. Continuity

For adjacent normalized observations:

```text
dt_s = current.ts - previous.ts
raw_distance_m = centre-to-centre distance
effective_distance_m = uncertainty-adjusted distance
implied_speed_mps = effective_distance_m / dt_s
```

### GAP

A GAP is emitted when:

```text
dt_s > gap_threshold_s
```

Its magnitude is `dt_s` in seconds.

A GAP does not itself start a new segment.

### DISCONTINUITY

A DISCONTINUITY is emitted when either:

```text
effective_distance_m > max_jump_m
```

or:

```text
implied_speed_mps > max_speed_mps
```

Its required Core magnitude is the **raw** jump distance in metres. Effective distance and implied speed remain analytical context.

The incoming observation starts a new segment.

GAP and DISCONTINUITY are orthogonal. Both may be emitted for one adjacency.

## 6. Stable presence

Presence is based on a fixed anchor observation and the effective distance to that anchor.

A candidate becomes asserted stable presence after at least two supporting observations are within `move_radius_m` of the anchor under the accuracy policy.

This is deliberately a reference eventizer rule, not a claim that every such interval represents a behavioural stop.

## 7. Departure confirmation

While presence is asserted, an observation outside the movement radius becomes a departure candidate.

If `confirm_moves=false`, departure is accepted immediately.

If `confirm_moves=true`, departure is accepted after a second consecutive outside observation occurs no more than `confirm_window_s` after the first departure candidate.

If an observation returns inside the presence radius before confirmation, the candidate departure is cancelled.

On confirmed departure:

1. close the presence interval at the last supporting in-presence observation;
2. emit STAY or DWELL for that closed interval;
3. emit LEAVE at the last supporting observation;
4. begin seeking destination presence.

The two fixes used to confirm departure are not automatically asserted as destination presence.

## 8. Destination presence and ENTER

After confirmed movement, destination presence is asserted when a candidate anchor obtains a second supporting observation within `move_radius_m`.

ENTER is assigned retrospectively to the candidate anchor time/location **only when there is a pending transition created by a previously confirmed departure in the same semantic segment**. That pending departure is the evidential basis for asserting an arrival.

If stable presence is discovered while seeking presence but there is no pending confirmed departure, the presence interval is still retained but ENTER is suppressed. This includes `candidate_presence` reached after unstable observations at the beginning of a dataset. Absence of a known origin/LEAVE is insufficient to assert ENTER.

When a pending transition and destination presence match, a transition analytical projection links the origin and destination presence intervals. A defensive segment mismatch cancels the pending transition and suppresses ENTER.

## 9. Dataset boundaries

START marks the first available observation for the entity.

END marks the final available observation for the entity.

The first stable presence in a dataset does not receive a synthetic ENTER. The final stable presence does not receive a synthetic LEAVE.

## 10. Gaps and discontinuities during presence

A GAP or DISCONTINUITY closes an asserted presence interval at its final supporting observation and emits its STAY/DWELL interval event.

It does **not** emit LEAVE or ENTER. Missing or physically discontinuous evidence is insufficient to assert a precise presence transition.

Any pending movement transition is cancelled across the continuity break.

## 11. STAY and DWELL

For a closed presence interval:

```text
duration_s = end_time - event_time
```

- STAY iff `duration_s < dwell_threshold_s`;
- DWELL iff `duration_s >= dwell_threshold_s`.

The Core event location is the spherical mean centroid of supporting observations. Horizontal accuracy statistics are calculated from supporting observations with known HA only; the projection records HA coverage and leaves median/min/max HA NULL when none are known. Observation scatter remains a separate context/projection field. None of these are represented as Core confidence.

## 12. Segments and path length

Segments are separated only by DISCONTINUITY, in accordance with Mobility Profile 0.1.

GAP-spanning edges are excluded from `observed_path_m`, because no route was observed over that interval.

Segment geometry is a LineString when fully observed as one path component, or a MultiLineString when GAPs split the visible path. The semantic segment ID does not change across a GAP; only the visual path geometry is broken so an unobserved connection is not drawn. Track geometry similarly breaks across GAPs and DISCONTINUITIES.

## 13. Walking compatibility

`walk_max_speed_mps` and `walk_max_jump_m` create a conservative `speed_class` analytical label:

- `walk-compatible`;
- `non-walk-compatible`;
- `unknown` across gaps or missing time intervals.

This is not a transport-mode assertion.

## 14. Deterministic identifiers

Track, segment, presence, transition and event identifiers are deterministic SHA-256-derived opaque identifiers. Consumers must not infer semantic meaning from the identifier string.

Identical normalized input, source identity and eventizer parameters produce the same semantic IDs.

## 15. Versioning

Any change that can alter eventization decisions or projection semantics must change `eventizer_algorithm` to a new algorithm identifier/version. `mobility-reference-v1.2` retains the conservative ENTER rule introduced by v1.1 and adds the horizontal-accuracy policy and null-preserving uncertainty semantics defined in sections 2–4 and 11. Such a change does not, by itself, require a Core Event Model wire-version change.
