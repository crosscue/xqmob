# Contributing to xqmob

Thanks for considering a contribution to xqmob.

xqmob is a reference mobility implementation for the Crosscue Event Model. The
project values reproducibility, evidence preservation, explicit uncertainty and
clear separation between observed evidence, eventizer semantics and analytical
projections.

## Before opening a pull request

For significant semantic changes, open an issue first. Changes to any of these
surfaces require explicit versioning and accompanying contract documentation:

- `mobility-reference-*` eventizer behavior;
- `xqmob-analysis-*` projection schemas/meaning;
- `xqmob-mcp-*` tool/resource contracts;
- canonical Crosscue Event Model output behavior.

Implementation-only changes should not silently alter deterministic IDs,
canonical events, analytical row semantics or provenance.

## Development requirements

- Go 1.25+
- `CGO_ENABLED=1` and a suitable C toolchain for the DuckDB-enabled binary

Run the normal checks before submitting:

```bash
go mod tidy
go test ./...
go vet ./...
go test -race ./...
```

For an end-to-end smoke test:

```bash
go run ./cmd/xqmob eventize testdata/sample.csv --source contributor-test --out /tmp/xqmob-test
go run ./cmd/xqmob validate /tmp/xqmob-test/canonical/events.jsonl
go run ./cmd/xqmob inspect /tmp/xqmob-test
```

## Coding and compatibility expectations

- Prefer straightforward Go and small domain-focused packages.
- Keep the eventizer independent from MCP and presentation concerns.
- Preserve the distinction between canonical events and analytical projections.
- Treat horizontal accuracy as evidence, not as an implicit quality gate.
- Geohash/H3 are analytical indexes and must not silently become movement rules.
- MCP stdout is protocol-only; application diagnostics belong on stderr.
- Collection-returning MCP tools must remain bounded and paginated.
- Add regression coverage for every bug fix and for semantic/versioned changes.

## Tests for semantic changes

A semantic change should normally include:

1. a minimal synthetic fixture;
2. expected canonical event behavior;
3. expected analytical projection behavior where relevant;
4. documentation of the change in the corresponding versioned contract;
5. an explicit version bump for the affected contract.

## Sensitive data

Contributions MUST use synthetic or intentionally public examples. Do not
include customer or partner data, live or historical operational intelligence,
personal mobility traces, authentication material, secrets, or data subject to
contractual, statutory or classification controls.

See `SECURITY.md`.

## Licensing contributions

xqmob is licensed under Apache-2.0. By submitting a contribution, you represent
that you have the right to submit it under the project's Apache-2.0 terms.
There is currently no separate contributor licence agreement (CLA).

Do not contribute code, data or documentation that you do not have the right to
redistribute.
