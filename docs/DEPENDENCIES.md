# Dependencies and licensing

xqmob intentionally keeps its direct dependency set small.

| Dependency | Version | Role | Licence |
|---|---:|---|---|
| parquet-go | v0.32.0 | Parquet/GeoParquet | Apache-2.0 |
| dimchansky/h3-go | v0.4.0 | H3 indexing | Apache-2.0 |
| duckdb-go/v2 | v2.10505.0 | embedded DuckDB analyst API | MIT |
| modelcontextprotocol/go-sdk | v1.7.0 | MCP server/transport | upstream mixed MIT/Apache-2.0 |

See `THIRD_PARTY_NOTICES.md` for release-packaging notes.

The repository does not vendor dependency source. `go.sum` pins the resolved
module graph. Before publishing binaries, generate a fresh dependency/licence
inventory from the release commit and preserve any required notices.
