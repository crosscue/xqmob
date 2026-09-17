# Public release checklist

Use this checklist when promoting a release candidate to a public GitHub tag.

## Source and licensing

- [ ] Confirm `LICENSE` is Apache-2.0.
- [ ] Review `NOTICE` and `THIRD_PARTY_NOTICES.md` against the exact dependency graph.
- [ ] Run `go mod tidy` with the supported Go toolchain and commit the resulting `go.sum`.
- [ ] Run a dependency licence/SCA scan for source and planned binary artifacts.
- [ ] Confirm sample/test data may be redistributed.
- [ ] Confirm no secrets, customer data, internal paths or proprietary fixtures are present.
- [ ] Confirm branding/trademark statements are appropriate.

## Engineering checks

```bash
go test ./...
go vet ./...
go test -race ./...
```

Also run the eventization/validation smoke test documented in `CONTRIBUTING.md`.

Recommended additional checks before a public binary release:

- `govulncheck ./...`;
- secret scanning (for example GitHub secret scanning or gitleaks);
- SBOM generation for release binaries;
- malware/signing checks required by your release process.

## Compatibility

- [ ] Verify `xqmob --version` reports the intended tool and contract versions.
- [ ] Confirm canonical fixture output changed only where explicitly intended.
- [ ] Confirm analytical-contract and MCP-contract version bumps match actual schema/API changes.
- [ ] Confirm MCP emits no application text on stdout.
- [ ] Confirm `xqmob validate` passes generated canonical events.

## Documentation

- [ ] README status/version text matches the release.
- [ ] Release notes describe user-visible changes and compatibility.
- [ ] Eventizer/analytical/MCP contracts referenced by the release exist in `docs/`.
- [ ] Security and contribution paths are enabled in repository settings.

## GitHub release

- [ ] Tag with the intended semantic version (for this candidate: `v0.1.4-rc1`).
- [ ] Attach source archive/checksum and any approved binaries.
- [ ] Include the checksum and dependency/SBOM artifacts where applicable.
- [ ] Mark pre-release/release-candidate appropriately.
