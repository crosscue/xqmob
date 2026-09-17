# xqmob 0.1.4-rc1 package manifest

This archive is a **source release candidate** prepared for public open-source review under Apache-2.0.

## Frozen contracts

- tool: `xqmob 0.1.4-rc1`
- eventizer: `mobility-reference-v1.2`
- analytical contract: `xqmob-analysis-v0.2`
- MCP contract: `xqmob-mcp-v0.2`
- Crosscue Event Model wire/profile: `0.1` / `xq.mob:profile-0.1`

Contract versions remain unchanged. This candidate includes output-replacement safeguards and ingestion/timestamp correctness fixes; see `RELEASE_NOTES.md` for compatibility implications.

## Package contents

- Go source under `cmd/` and `internal/`;
- synthetic test fixtures under `testdata/`;
- CI workflow and GitHub contribution templates under `.github/`;
- eventizer, analytical, MCP, storage, accuracy, diagnostic and architecture documentation under `docs/`;
- Apache-2.0 `LICENSE`, project `NOTICE`, third-party notices and public contribution/security/governance files;
- `go.mod` and `go.sum` pinning the Go 1.25 module graph;
- `SOURCE_MANIFEST.sha256` containing checksums for source-package files.

## Not included

- compiled binaries;
- generated xqmob analytical outputs;
- private/proprietary datasets;
- vendored Go dependencies.

The candidate should be built/tested in a connected Go 1.25+ environment before promotion to a stable public tag.
