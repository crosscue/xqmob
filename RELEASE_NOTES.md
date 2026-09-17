# xqmob 0.1.4-rc1 release notes

`0.1.4-rc1` is the first public open-source release candidate for xqmob. It includes packaging, documentation, repository hardening and ingestion/data-safety fixes for external review before a public stable tag.

## Versions

- xqmob tool: `0.1.4-rc1`
- eventizer: `mobility-reference-v1.2` (unchanged)
- analytical projections: `xqmob-analysis-v0.2` (unchanged)
- MCP analyst contract: `xqmob-mcp-v0.2` (unchanged)
- Event Model wire: `0.1` (unchanged)
- Mobility Profile: `xq.mob:profile-0.1` (unchanged)

## Open-source release packaging

This candidate adds the repository material expected for public collaboration and redistribution:

- Apache License 2.0 (`LICENSE`);
- project `NOTICE` and direct dependency licensing notes;
- `CONTRIBUTING.md`;
- `SECURITY.md`;
- `CODE_OF_CONDUCT.md`;
- `GOVERNANCE.md`;
- `SUPPORT.md`;
- trademark/project-name guidance;
- GitHub issue and pull-request templates;
- architecture and public-release documentation;
- a simple Makefile for build/test/vet/race workflows;
- an example LLM/agent starting prompt for the MCP interface.

The README now states explicitly that xqmob is a reference implementation and that the Crosscue Event Model specification/profile remains the normative interoperability source.

## Dependency / binary-release notes

The source release does not vendor dependencies. Direct dependency licensing is summarized in `THIRD_PARTY_NOTICES.md` and `docs/DEPENDENCIES.md`.

`go.mod` and `go.sum` pin the Go 1.25 module graph used to prepare this candidate. Direct and notable transitive licensing is summarized in `THIRD_PARTY_NOTICES.md`. Before publishing binary artifacts, re-run a complete transitive licence/SCA scan from the release commit.

## Semantics and compatibility

The eventizer rules, thresholds, ID algorithm, Core/analytical schemas, spatial-indexing rules, Parquet defaults, HA policy and MCP contracts remain unchanged from xqmob 0.1.3.

Input normalization now preserves exact numeric nanosecond epochs and decimal/scientific-notation timestamps. Non-finite and out-of-range numeric timestamps are rejected. Values finer than one nanosecond are rounded to the nearest nanosecond, with ties away from zero. Analytical timestamps use a direct millisecond conversion, correcting pre-epoch rounding and dates outside the Unix-nanosecond range. These fixes can change observations, events and derived IDs for affected inputs; regenerate affected datasets from source. Canonical timestamps retain nanosecond precision; analytical timestamps remain millisecond precision.

Non-empty output directories now require explicit `--replace`. Existing dataset artifacts remain intact until ingestion, including partition/rejection-log writes and final flushes/closes, has succeeded. Replacement removes known artifacts and the entire diagnostics directory. Later materialization failures can still leave partial replacement output; this is not a transactional dataset swap.

The tool/producer version remains `0.1.4-rc1`, and the manifest records the replacement option when enabled. See `CHANGELOG.md` for the reconciled release sequence. Benchmark descriptions use configuration names rather than conflicting development version numbers.

Entity details returned by `inspect --json` now use snake_case field names matching the analytical columns (for example, `entity_id` and `observation_count`). Scripts using the previous Go-style keys such as `EntityID` must update their field lookups. Parquet columns and MCP responses are unchanged.

## Candidate review focus

External reviewers are particularly encouraged to examine:

1. Event Model/Profile conformance boundaries;
2. the evidence-versus-derived-analysis distinction;
3. analytical projection schemas and GeoParquet interoperability;
4. deterministic eventization behavior and provenance;
5. MCP read-only/query boundaries and stdout purity;
6. licensing/notices for planned source and binary distribution;
7. cross-platform CGO/DuckDB build reproducibility.
