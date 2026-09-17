# xqmob MCP analyst interface v0.2

Status: xqmob `0.1.4-rc1` / `xqmob-mcp-v0.2`.

`xqmob mcp <output-dir>` starts a read-only Model Context Protocol server over stdio for one existing xqmob dataset.

## Purpose

The MCP interface lets an LLM interrogate millions of mobility observations through domain concepts rather than loading raw files into a context window. DuckDB queries the Parquet/GeoParquet analytical layer directly; xqmob does not import the dataset into another database file.

MCP does not run or modify the eventizer. It does not alter canonical JSONL, analytical projections, IDs, or the manifest. The opened output directory is treated as read-only evidence plus derived analytical projections.

The MCP analyst interface accepts both `xqmob-analysis-v0.1` and `xqmob-analysis-v0.2` datasets. v0.1 datasets predate materialized H3 columns; xqmob creates DuckDB compatibility views with nullable H3 fields so all non-H3 tools remain available without re-eventization. H3 filtering and `describe_h3_cell` are exposed only when the opened dataset materializes H3 (v0.2).

## Transport

The MCP interface currently supports stdio only:

```bash
xqmob mcp /path/to/xqmob-output
```

Optional DuckDB worker count:

```bash
xqmob mcp --duckdb-threads 8 /path/to/xqmob-output
```

stdin and stdout are reserved for newline-delimited MCP JSON-RPC. All xqmob/server diagnostics go to stderr. MCP mode must never print application logging to stdout.

A generic MCP-host configuration is conceptually:

```json
{
  "mcpServers": {
    "xqmob": {
      "command": "/path/to/xqmob",
      "args": ["mcp", "/path/to/xqmob-output"]
    }
  }
}
```

Exact host configuration syntax varies by client.

## Build implication

The MCP implementation embeds DuckDB through `duckdb-go`. The single xqmob binary therefore becomes a CGO/native-linked build. `duckdb-go` ships prebuilt DuckDB static libraries for its supported Linux/macOS/Windows architectures, but `CGO_ENABLED=1` and a suitable C toolchain are required at build time.

Go 1.25+ is required by the pinned MCP Go SDK. xqmob uses the current `github.com/modelcontextprotocol/go-sdk v1.7.0`, which supports MCP `2026-07-28` and retains compatibility with earlier supported MCP protocol versions.

## Architecture

```text
MCP client / future UI / future API
                |
          xqmob analyst API
                |
             DuckDB
                |
      analysis/*.parquet
```

The MCP handlers do not embed ad-hoc file logic. They call the mobility-domain analyst API. DuckDB opens in memory and creates views over the existing files, with filesystem access restricted to the opened dataset and the configuration then locked. A single database connection is used so the in-memory view catalog is stable and query execution remains serialized conservatively in v0.2.

This layering is intentional: a future CLI query command, UI, or service can reuse the analyst API without copying MCP handlers.

## Tools

### Dataset and entity discovery

- `dataset_summary` — compact manifest/statistics summary; recommended first call.
- `search_entities` — filtered/sorted entity discovery using observation, presence, transition, discontinuity, dwell, HA-support and HA-coverage criteria.
- `get_entity` — one entity summary.
- `get_entity_timeline` — chronological union of semantic events, presence intervals and confirmed transitions.

### Derived mobility objects

- `get_presence`
- `get_transitions`
- `get_segments`
- `get_events`

Each supports domain-appropriate filters and optional RFC3339 time bounds.

### Evidence and explanation

- `get_observations` — normalized observations; requires `entity_id` or `h3_cell`.
- `explain_event` — event plus nearby supporting observations and linked presence/segment context.
- `explain_discontinuity` — adjacent evidence, configured thresholds, HA state and derived jump/speed reason. A discontinuity remains a continuity-model violation, not proof of physical travel.

### H3 spatial analysis

- `describe_h3_cell` — observations/entities, presence/dwell, incoming/outgoing transitions, top origin/destination cells and top entities for one materialized H3 cell.

`describe_h3_cell` is registered only for datasets with materialized H3 (`xqmob-analysis-v0.2`). H3 filters on other tools return an explicit capability error for v0.1 datasets. The tool uses the H3 resolution already materialized in the dataset manifest. H3 remains an analytical index and does not become a movement/accuracy rule.

## Bounded results and pagination

Collection tools default to 100 rows and hard-cap at 1000 rows per call. Pagination uses opaque cursors. The server never returns the whole observation table merely because a model omitted filters.

`get_observations` is intentionally stricter and requires an entity ID, or an H3 cell when H3 is materialized.

The v0.2 cursor remains an opaque offset cursor. It is sufficient for bounded local analysis; a later contract can introduce keyset cursors if very deep pagination becomes a real workflow.

## Resources

- `xqmob://dataset/manifest`
- `xqmob://dataset/capabilities`
- `xqmob://guidance/analyst`
- `xqmob://contract/analytical`
- `xqmob://contract/spatial-indexing`
- `xqmob://contract/eventizer`

The analyst guidance makes the evidence hierarchy explicit: observations are normalized source evidence; events, segments, presence intervals and transitions are algorithmically derived interpretations. GAP and DISCONTINUITY must not be over-interpreted as observed physical activity.

## Query safety

MCP contract v0.2 exposes no arbitrary SQL. Every query is a fixed SELECT/aggregation shape; values are passed as SQL parameters and selectable sort fields are allow-listed. DuckDB is an in-memory catalog over the opened dataset's Parquet files: filesystem access is restricted to that dataset directory, `enable_external_access` is disabled after the allowlist is set, and the DuckDB configuration is then locked. The in-memory catalog itself cannot use `access_mode=READ_ONLY` because views still have to be created.

A constrained read-only SQL tool can be considered later if real analyst workflows justify it, but it is deliberately not part of the reference MCP contract.

## xq-family pattern

`xqmob` is the reference domain implementation. Other Crosscue tools should reuse only transport-level conventions while exposing their own analytical vocabulary:

```text
xqmob mcp  -> presence, transitions, segments, H3 cells
xqnet mcp  -> flows, sessions, endpoints, protocol activity
xqrf  mcp  -> detections, spectral activity, bursts, emitters
```

A small `internal/mcpbase` package owns stdio/logging conventions. Domain tools and schemas stay inside each xq tool rather than being forced through a generic lowest-common-denominator MCP server.
