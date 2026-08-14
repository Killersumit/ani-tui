# Changelog

All notable changes to ani-tui are documented here. The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Dedicated in-workspace playback controls for ani-cli's Next episode, Replay, Previous episode, Change quality, and Quit actions, with one-key shortcuts and exact upstream action forwarding.
- Regression coverage for all five playback actions and repeated post-play menu requests.

### Security

- Raised the source-build minimum to Go 1.25.10 so release builds include the standard-library fix for GO-2026-4971.

## [0.1.0] - 2026-08-14

### Added

- Persistent full-screen Bubble Tea workspace with wide split-pane and compact `54x16` layouts.
- Native inline selectors for ani-cli search, season, episode, quality, and post-play prompts.
- Continue watching, history, activity, settings, exact/range playback, indexed selection, update, log, and countdown workflows.
- Authenticated loopback menu bridge that preserves raw ani-cli rows and terminal stdin behavior.
- Active-configuration dependency doctor with verified package-manager plans, explicit consent, terminal handoff, post-install verification, and bounded transient retries.
- Linux, macOS, and Windows native release targets with SHA-256 checksums and build-provenance attestations.
- Interactive curl installer, data-preserving uninstaller with explicit `--purge`, and dependency-free npm global package.
- CI, CodeQL, release automation, community templates, and distribution regression tests.

### Fixed

- Reliable process output draining, option precedence, executable validation, compact rendering, and Discover search-focus restoration after route navigation.

[Unreleased]: https://github.com/Killersumit/ani-tui/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Killersumit/ani-tui/releases/tag/v0.1.0
