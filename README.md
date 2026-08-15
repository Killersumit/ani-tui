# ani-tui

[![CI](https://github.com/Killersumit/ani-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/Killersumit/ani-tui/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/Killersumit/ani-tui)](https://github.com/Killersumit/ani-tui/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-limegreen.svg)](LICENSE)

A full-screen, keyboard-first interface for [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli).

`ani-tui` keeps search, episode selection, settings, activity, and playback controls inside one persistent terminal interface. It uses the installed `ani-cli` command for lookup and playback, so it does not replace or modify ani-cli.

## Screenshots

Screenshot space is ready in [`docs/images`](docs/images/README.md).

Suggested files:

- `docs/images/overview.png`
- `docs/images/playback-controls.png`
- `docs/images/dependency-doctor.png`

Upload the images there and they can be displayed in this section.

## Highlights

- Persistent full-screen split-pane layout
- Search, history, continue watching, settings, and activity views
- Native selectors for anime, seasons, episodes, and quality
- Playback controls for **Next**, **Replay**, **Previous**, **Change quality**, and **Quit**
- Multi-select episode ranges
- Support for mpv, VLC, Syncplay, downloads, custom players, and intro skipping
- Inline dependency checks with confirmation before installing anything
- Linux, macOS, and Windows release binaries
- A separate global `ani-tui` command that leaves `ani-cli` unchanged

## Requirements

You need:

1. A current [`ani-cli`](https://github.com/pystardust/ani-cli) installation.
2. A supported media player such as mpv or VLC.
3. The tools required by the ani-cli features you use, such as `curl`, `sed`, `grep`, `yt-dlp`, or `ffmpeg`.

Go is needed only when building from source.

If something is missing, ani-tui shows the dependency and proposed installation command inside the TUI. It always asks for confirmation before installing anything.

## Install

### Linux and macOS

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh | sh
ani-tui
```

Install a specific version noninteractively:

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh \
  | sh -s -- --version v0.1.0 --yes
```

The installer downloads the correct GitHub Release binary, verifies its SHA-256 checksum, and installs it as `ani-tui`.

### Windows and manual installation

Download the correct file from [GitHub Releases](https://github.com/Killersumit/ani-tui/releases):

| Platform | Release file |
|---|---|
| Linux x64 | `ani-tui-linux-x64` |
| Linux ARM64 | `ani-tui-linux-arm64` |
| Linux ARMv7 | `ani-tui-linux-armv7` |
| macOS Intel | `ani-tui-darwin-x64` |
| macOS Apple Silicon | `ani-tui-darwin-arm64` |
| Windows x64 | `ani-tui-windows-x64.exe` |
| Windows ARM64 | `ani-tui-windows-arm64.exe` |

On Linux or macOS, make the downloaded file executable and place it somewhere on `PATH`:

```sh
chmod +x ani-tui-*
mv ani-tui-* "$HOME/.local/bin/ani-tui"
```

Each release includes `checksums.txt` and GitHub build-provenance attestations.

## Build from source

Go 1.25.10 or newer is required.

```sh
git clone https://github.com/Killersumit/ani-tui.git
cd ani-tui
make check
make install PREFIX="$HOME/.local"
ani-tui
```

## Basic controls

| Key | Action |
|---|---|
| `/` | Search |
| `enter` | Submit or select |
| `space` | Mark episode-range endpoints |
| `↑` / `↓` or `j` / `k` | Navigate |
| `1`–`4` or `tab` | Change section |
| `ctrl+p` | Open the command palette |
| `n` | Next episode during playback |
| `r` | Replay during playback |
| `p` | Previous episode during playback |
| `c` | Change quality during playback |
| `q` | Quit playback or quit while idle |
| `esc` | Go back or cancel |
| `?` | Open help |

## Configuration

Most options can be changed from the **Settings** screen.

To print the configuration-file location:

```sh
ani-tui --print-config
```

If `ani-cli` is not on `PATH`, open **Settings → ani-cli path** and select its executable.

## Uninstall

Remove the ani-tui command while keeping configuration and user data:

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/uninstall.sh | sh
```

Explicitly remove ani-tui configuration, cache, and state too:

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/uninstall.sh \
  | sh -s -- --purge
```

Even with `--purge`, the separate `ani-cli` command, ani-cli history, and downloaded media are not removed.

## Documentation

- [Architecture and research](docs/research-and-architecture.md)
- [Release guide](docs/RELEASING.md)
- [Contributing](CONTRIBUTING.md)
- [Support](SUPPORT.md)
- [Security policy](SECURITY.md)

## License and attribution

ani-tui is an independent frontend and is not affiliated with anime providers. It delegates lookup and playback behavior to [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli). Follow upstream notices and the laws and terms that apply in your location.

Released under the [MIT License](LICENSE).
