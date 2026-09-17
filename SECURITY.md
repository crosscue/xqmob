# Security policy

xqmob processes location-time observations and derived mobility structure.
Combining otherwise ordinary traces can materially increase sensitivity through
correlation. Pseudonymised identifiers are not necessarily anonymous when
location and time are sufficiently distinctive.

## Supported versions

Security fixes are normally applied to the latest published xqmob release and,
where practical, the immediately preceding compatible release line.

## Reporting a vulnerability

Please do not open a public GitHub issue for a suspected security vulnerability.
Use GitHub's private vulnerability reporting / Security Advisory mechanism for
the repository. If that mechanism is unavailable, use the current private
contact route published by Crosscue at <https://crosscue.uk/>.

Include enough information to reproduce and assess the issue, such as affected
version, platform, invocation, relevant configuration and a minimal proof of
concept where safe to provide one.

Please avoid accessing data you do not own or have permission to test.

## Public issue hygiene

Do not include the following in public issues, pull requests or fixtures:

- customer or partner data;
- live or historical operational intelligence;
- personal mobility traces;
- authentication material or secrets;
- protected source identifiers;
- sensitive collection parameters;
- data subject to contractual, statutory or classification controls.

Use synthetic or intentionally public data when reproducing issues.

## Scope notes

xqmob processes potentially sensitive location evidence. Security reports may
therefore include issues involving:

- unexpected data disclosure through CLI/MCP output;
- path traversal or unsafe file handling;
- malformed input causing denial of service;
- MCP protocol boundary violations (especially stdout contamination);
- DuckDB query-layer escape from the intended read-only analytical interface;
- dependency or native-library vulnerabilities affecting the shipped binary.
