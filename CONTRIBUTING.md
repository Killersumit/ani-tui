# Contributing to ani-tui

Thank you for helping improve ani-tui.

## Scope

ani-tui is a cohesive full-screen frontend for [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli). Scraping, source selection, stream resolution, playback, downloads, and upstream command flags should normally be fixed in ani-cli. This repository owns the persistent TUI workspace, bridge, configuration, process integration, dependency doctor, and distribution tooling.

Please search existing issues first. Use a feature issue before a large UI, dependency-policy, or packaging change.

## Development setup

Requirements: Go 1.25.10 or newer, Node 22 or newer for npm-package tests, and POSIX shell tools for distribution tests.

```sh
git clone https://github.com/Killersumit/ani-tui.git
cd ani-tui
make check
sh ./scripts/test-distribution.sh
(cd npm && npm test)
```

Run a development build with `make build`, then execute `./ani-tui`. A real end-to-end session also requires ani-cli and the active playback dependencies.

## Design and safety rules

- Keep interactions in the persistent full-screen workspace; do not reintroduce chains of modal screens.
- Preserve the split-pane context/active-content model and the `54x16` compact fallback.
- Keep ani-cli as the separate backend command. The frontend command is always `ani-tui`.
- Preserve raw ani-cli menu rows across the bridge.
- Never install a package without an explicit yes/no decision.
- Add automatic dependency mappings only when the manager/package and official documentation URL are verified.
- Do not retry ambiguous, permission, package-resolution, checksum, or other permanent failures.
- Uninstall must preserve all user data by default and must never remove ani-cli data.

The research and architecture rationale is in [`docs/research-and-architecture.md`](docs/research-and-architecture.md).

## Tests and pull requests

Run and report the relevant checks:

```sh
gofmt -w *.go
go test -race ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
shellcheck install.sh uninstall.sh scripts/*.sh
sh ./scripts/test-distribution.sh
python3 ./scripts/test-installer-pty.py
(cd npm && npm test && npm pack --dry-run --ignore-scripts)
```

Add a regression test for every behavior fix. UI changes should be checked at both a normal split-pane size and `54x16`. Keep pull requests focused, explain user-visible behavior, and update documentation and packaging metadata when necessary.

By contributing, you agree that your contribution is licensed under this repository's MIT License.
