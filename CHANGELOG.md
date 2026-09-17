# Changelog

This changelog defines the tool's release sequence: 0.1.0 reference baseline, 0.1.1 H3 projections, 0.1.2 MCP, 0.1.3 MCP compatibility, and 0.1.4-rc1 public release candidate. The eventizer, analytical and MCP contracts are versioned independently; see the release notes and contract documents for semantic detail. Earlier experimental benchmark configurations are described by their features, not separate release numbers.

## 0.1.4-rc1

First public open-source release candidate. Adds Apache-2.0 licensing, contribution/security/governance material, dependency notices and public-release hardening.

- Non-empty output directories require `--replace`; existing dataset artifacts survive failed ingestion.
- Partition and rejection-log write, flush and close failures are propagated.
- Numeric epochs are parsed exactly, preserving nanosecond distinctions and rejecting non-finite/out-of-range timestamps. Analytical millisecond conversion handles pre-epoch and distant dates correctly.
- Documentation uses one release chronology and describes earlier benchmark configurations without conflicting version numbers.

Contract versions and mobility thresholds are unchanged. Timestamp corrections can change normalized observations, derived events and IDs for affected inputs; affected datasets should be regenerated from source.

## 0.1.3

MCP compatibility fix: the analyst server can open both `xqmob-analysis-v0.1` and `xqmob-analysis-v0.2` datasets; H3-dependent functionality is capability-gated.

## 0.1.2

Introduced the domain-specific read-only MCP analyst interface over DuckDB (`xqmob-mcp-v0.1`, subsequently revised to v0.2 compatibility behavior).

## 0.1.1

Added H3 alongside geohash in analytical projections and introduced `xqmob-analysis-v0.2`. H3 remained analytical-only and did not affect eventization.

## 0.1.0

Reference baseline of the mobility eventizer and analysis-ready projections, with `mobility-reference-v1.2`, `xqmob-analysis-v0.1`, canonical Event Model JSONL and Parquet/GeoParquet outputs. Includes preservation-first HA handling, accuracy attribution, temporal/source/reversal diagnostics, execution and memory instrumentation, heap profiling, and bounded ZSTD Parquet materialization.
