# Third-party notices

This file records the direct non-standard-library dependencies used by xqmob
and the modules `go list -deps ./...` reports as reachable from the xqmob
packages on linux/amd64. It is intended as release-packaging guidance, not as a
substitute for reviewing the complete dependency graph when redistributing
binaries. `go.sum` is the checksum pin for the resolved module graph.

## Direct dependencies

| Dependency | Pinned version | Purpose | Upstream license |
|---|---:|---|---|
| `github.com/parquet-go/parquet-go` | `v0.32.0` | Parquet / GeoParquet read-write support | Apache-2.0 |
| `github.com/dimchansky/h3-go` | `v0.4.0` | Pure-Go H3 analytical indexing | Apache-2.0 |
| `github.com/duckdb/duckdb-go/v2` | `v2.10505.0` | Embedded DuckDB analyst queries for MCP | MIT |
| `github.com/modelcontextprotocol/go-sdk` | `v1.7.0` | MCP server and stdio transport | mixed MIT / Apache-2.0 upstream licensing |

The MCP Go SDK is undergoing an upstream licensing transition: new
contributions are Apache-2.0 while existing code whose relicensing has not been
completed remains under MIT-style terms. Consult the exact `LICENSE` file in
the pinned module version when preparing redistributed binaries.

`dimchansky/h3-go` reimplements algorithms from Uber H3 and includes its own
NOTICE/attribution requirements. Preserve those notices when required by the
form of redistribution.

## Reachable transitive modules

Licences below are taken from the LICENSE/NOTICE files in the pinned module
versions. Additional platform-specific `duckdb-go-bindings/lib/*` modules are
selected by GOOS/GOARCH at build time.

| Module | Version | Upstream license |
|---|---:|---|
| `github.com/andybalholm/brotli` | v1.2.0 | MIT |
| `github.com/duckdb/duckdb-go-bindings` | v0.10505.0 | MIT |
| `github.com/duckdb/duckdb-go-bindings/lib/linux-amd64` | v0.10505.0 | MIT (prebuilt DuckDB) |
| `github.com/go-viper/mapstructure/v2` | v2.5.0 | MIT |
| `github.com/google/jsonschema-go` | v0.4.3 | MIT |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/klauspost/compress` | v1.18.7 | BSD-3-Clause (some files Apache-2.0) |
| `github.com/parquet-go/bitpack` | v1.0.0 | Apache-2.0 |
| `github.com/parquet-go/jsonlite` | v1.0.0 | MIT |
| `github.com/pierrec/lz4/v4` | v4.1.25 | BSD-3-Clause |
| `github.com/segmentio/asm` | v1.1.3 | MIT |
| `github.com/segmentio/encoding` | v0.5.4 | MIT |
| `github.com/twpayne/go-geom` | v1.6.1 | BSD-2-Clause |
| `github.com/yosida95/uritemplate/v3` | v3.0.2 | BSD-3-Clause |
| `golang.org/x/oauth2` | v0.35.0 | BSD-3-Clause |
| `golang.org/x/sync` | v0.20.0 | BSD-3-Clause |
| `golang.org/x/sys` | v0.44.0 | BSD-3-Clause |
| `golang.org/x/time` | v0.15.0 | BSD-3-Clause |
| `google.golang.org/protobuf` | v1.36.11 | BSD-3-Clause |

`go.mod` also records further indirect modules (including Apache Arrow, FlatBuffers
and additional `golang.org/x` packages) that may be pulled in by the Parquet/DuckDB
graphs depending on build tags. Consult `go.mod` / `go.sum` for the complete pin set.

## Binary redistribution

The source release does not vendor these dependencies. A compiled xqmob binary
statically incorporates Go dependency code and, through `duckdb-go`, supported
prebuilt DuckDB components (MIT, Copyright 2018-2026 Stichting DuckDB Foundation).
Before publishing binary artifacts:

1. resolve and review the complete transitive dependency graph for the exact
   release commit;
2. preserve all required copyright, license and NOTICE material;
3. generate a machine-readable software bill of materials if your release
   process requires one;
4. verify that the binary and source archive contain the applicable notices.

Useful starting points include `go list -m -json all`, `go list -deps ./...`, a
Go license scanner, and your normal software-composition-analysis tooling.
