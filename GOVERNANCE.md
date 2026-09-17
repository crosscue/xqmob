# Governance

xqmob is currently maintained as a Crosscue-led reference implementation.

## Decision model

The project is maintainer-led. Maintainers are responsible for release scope,
compatibility decisions, security response and interpretation of the reference
implementation contracts.

Substantial changes should be discussed publicly before merge when practical,
especially changes to:

- event semantics;
- analytical contracts;
- deterministic identity/provenance;
- MCP contracts;
- dependency or toolchain requirements.

The Crosscue Event Model specification is a separate project and remains the
normative source for Event Model conformance. xqmob is a reference
implementation, not the specification itself.

## Versioned contracts

xqmob deliberately versions independent surfaces separately:

- tool version: `xqmob` release version;
- eventizer: `mobility-reference-*`;
- analytical projections: `xqmob-analysis-*`;
- MCP analyst interface: `xqmob-mcp-*`;
- Crosscue Event Model wire/profile versions.

A change to one surface does not imply that the others must change.
