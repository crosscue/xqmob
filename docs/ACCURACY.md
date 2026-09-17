# Horizontal accuracy as analytical evidence

Status: xqmob 0.1.4-rc1 / `mobility-reference-v1.2` / `xqmob-analysis-v0.2`.

Horizontal accuracy (`ha`) is treated as positional uncertainty attached to an observation. It is not Crosscue Event Model `confidence`, and xqmob does not convert it into confidence.

## Default preservation policy

The default is:

```text
ha_policy = preserve
```

A blank HA value is retained as unknown. It is not replaced with zero, a vendor default, a median, or any other imputed radius.

Analytical representations use:

```text
ha_m = NULL
ha_state = missing
```

When a Core Event location is sourced directly from such an observation, `location.accuracy_m` is omitted.

`--ha-policy require` is an explicit analyst-selected filter for workflows that require every retained observation to carry HA. `allow-missing` is a compatibility alias for `preserve`.

`--max-ha-m` is also an optional analyst-selected filter. It is disabled by default.

## Distance semantics

For two adjacent fixes with centre-to-centre distance `d`, the reference eventizer subtracts only uncertainty radii that are actually known:

```text
effective_distance = max(0, d - known_ha_1 - known_ha_2)
```

Therefore:

| Previous HA | Current HA | Step state | Uncertainty correction |
|---|---|---|---|
| known | known | `both_known` | subtract both radii |
| missing | known | `previous_missing` | subtract current radius only |
| known | missing | `current_missing` | subtract previous radius only |
| missing | missing | `both_missing` | no radius can defensibly be subtracted |

The last case does **not** mean the observations have perfect accuracy. It means the eventizer cannot quantify an uncertainty radius from the supplied evidence. That distinction is preserved in analytical attributes and diagnostics.

## Analytical support classes

Aggregated objects use:

- `all_known`: every supporting observation has HA;
- `mixed`: supporting observations contain both known and missing HA;
- `all_missing`: none of the supporting observations has HA;
- `none`: no applicable observation-level support is asserted.

These classes are descriptive, not quality thresholds.

### Entities

`entities.parquet` includes:

```text
observations_with_ha
observations_without_ha
ha_coverage_pct
ha_class
```

### Segments and presence intervals

Both layers expose known/missing counts, coverage and `accuracy_support`. Presence intervals retain nullable median/min/max HA calculated only from observations where HA is known.

Segments additionally expose `start_ha_state`, `end_ha_state`, and counts of `both_known`, `previous_missing`, `current_missing`, and `both_missing` adjacencies. Presence intervals expose endpoint HA state as well. These fields locate uncertainty within an object rather than reducing it to a single aggregate percentage.

### Transitions

Transition support is intentionally decomposed into three scopes:

- `origin_*`: the closed origin presence interval;
- `movement_*`: observations from the confirmed departure point through the first destination-presence anchor;
- `destination_*`: the subsequently closed destination presence interval.

The existing unprefixed `observations_with_ha`, `observations_without_ha`, `ha_coverage_pct`, and `accuracy_support` fields remain compatibility aliases for the movement window. The three scopes overlap at their boundaries and are descriptive, not additive.

### Analytical events

`events.parquet` contains:

```text
accuracy_state
accuracy_support
accuracy_known_count
accuracy_missing_count
```

For point events this is derived from the source observation. For edge events it is derived from `edge_accuracy_state`. For STAY/DWELL it is derived from the interval's supporting observation counts.

Canonical events retain the underlying accuracy/context fields; the flattened attributes are analytical conveniences.

## Why xqmob does not filter by default

Mobility observations are inherently uncertain spatial measurements. Horizontal accuracy is a key part of that uncertainty model. A fixed quality threshold such as `ha <= 50 m`, removal of missing HA, or vendor-specific filtering may be valid for a particular analytical question, but those choices should remain explicit and reproducible analytical decisions.

The reference pipeline therefore preserves representable HA evidence and exposes its effect on derived semantics. Analysts can subsequently filter observations or derived layers by raw HA, coverage, support class, event type, entity, geography or time without losing the unfiltered enriched layer.


The normative projection semantics are collected in `ANALYTICAL-CONTRACT.md`.
