# ani-tui on npm

This package installs the native **ani-tui** executable and exposes it as a global `ani-tui` command. Node.js 22 or newer is supported:

```sh
npm install --global ani-tui
ani-tui
```

The dependency-free postinstall script detects Linux, macOS, or Windows and downloads the matching binary from the same-version [GitHub Release](https://github.com/Killersumit/ani-tui/releases). It refuses to install an artifact that does not match the release's SHA-256 checksum.

ani-tui is a frontend for the separate [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli) backend. It never replaces the `ani-cli` command. When a required or currently selected optional dependency is missing, the in-app dependency doctor explains the official source and exact installation command, then asks for consent.

Supported native targets:

- Linux: x64, ARM64, ARMv7
- macOS: x64, Apple Silicon
- Windows: x64, ARM64

Uninstalling the npm package removes the command and downloaded native binary while preserving ani-tui configuration and all ani-cli data:

```sh
npm uninstall --global ani-tui
```

Project documentation and source: <https://github.com/Killersumit/ani-tui>
