# xqmob architecture

xqmob is a reference mobility implementation for the Crosscue Event Model.
Its architecture deliberately separates evidence ingestion, semantic
eventization, analytical materialization and LLM-facing analysis.

```text
normalized CSV evidence
        |
        v
validation / deterministic ordering
        |
        v
mobility-reference eventizer
        |
        +--> canonical/events.jsonl
        |
        +--> analytical projections (Parquet / GeoParquet)
                 |
                 +--> CLI inspect
                 +--> DuckDB analyst API
                          |
                          +--> read-only MCP over stdio
```

## Normative boundaries

The Crosscue Event Model specification/profile defines the interoperable event
contract. xqmob is a reference implementation of one reproducible mobility
eventizer and analytical projection model.

The following are independently versioned:

- `mobility-reference-v1.2`: eventizer behavior;
- `xqmob-analysis-v0.2`: analytical projection contract;
- `xqmob-mcp-v0.2`: MCP analyst contract;
- Event Model wire/profile versions.

## Evidence hierarchy

Normalized observations are the closest representation of supplied evidence.
Events, segments, presence intervals and transitions are derived analytical
interpretations. The analyst/MCP layer should preserve that distinction.

## Spatial indexes

Geohash and H3 are materialized for analytical convenience. Neither index drives
movement, presence or continuity decisions. Horizontal accuracy remains an
independent evidence field describing positional uncertainty.

## MCP boundary

MCP is read-only over an existing xqmob dataset. DuckDB creates in-memory views
over the analytical Parquet files; the source dataset is not imported or
modified. MCP stdout is reserved for protocol traffic and results are bounded.
