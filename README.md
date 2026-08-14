# ani-tui

[![CI](https://github.com/Killersumit/ani-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/Killersumit/ani-tui/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/Killersumit/ani-tui)](https://github.com/Killersumit/ani-tui/releases)
[![npm](https://img.shields.io/npm/v/ani-tui)](https://www.npmjs.com/package/ani-tui)
[![License: MIT](https://img.shields.io/badge/license-MIT-limegreen.svg)](LICENSE)

A full-screen, keyboard-first terminal frontend for [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli).

`ani-tui` does **not** fork ani-cli's scraping or playback logic. It runs the installed `ani-cli` script unchanged and replaces its `fzf`/rofi/dmenu prompts with native in-app selectors. Search results, seasons, episodes, qualities, and post-play actions stay inside one cohesive TUI.

## Highlights
## !!THERE ARE SOME ERROR SO DONT INSTALL YET, BUT U CAN STILL DOWNLOAD THE THIS REPO TO TEST!!

- Full-screen alternate-buffer interface built with Bubble Tea and Lip Gloss
- Search, continue watching, watch history, and backend activity screens
- Filterable selectors for every menu request made by ani-cli
- Native post-play controls for Next episode, Replay, Previous episode, Change quality, and Quit—each sent back to ani-cli without replacing its playback loop
- Multi-select episode ranges: press `space` on the range endpoints, then `enter`
- Persistent quality, sub/dub, stream/download, player, player flags, history, and update settings
- Command palette for less frequent actions (`ctrl+p`)
- ani-cli update, log viewing, history deletion, next-episode countdown, exact episode/range, and indexed result selection
- mpv, VLC, Syncplay, `catt`, debug links, downloads, custom players, intro skipping, attached playback, and exit-after-play
- Cohesive workspace rather than successive pop-ups: ani-cli requests, editors, confirmations, help, and commands render inline
- Persistent split-pane layout at wide widths: navigation and a live session timeline on the left, active content on the right
- Compact layout that keeps the application header, route context, session status, and footer visible
- Subtle spinner, moving progress pulse, completed-step indicators, toasts, activity output, and small-terminal fallback
- Optional mouse support for wide-layout context-pane clicks and wheel navigation
- Authenticated loopback bridge; raw ani-cli menu rows are returned unchanged
- Inline dependency doctor with active-feature checks, verified package-manager plans, terminal handoff, and bounded transient retries
- Single executable that doubles as ani-cli's recursive menu helper

## Requirements

1. A current [`ani-cli`](https://github.com/pystardust/ani-cli) installation. The integration is tested against ani-cli 5.0.2; ani-tui keeps `ani-cli` as a separate upstream backend command.
2. ani-cli's runtime tools for the features you use:
   - core tools such as `curl`, `sed`, and `grep`;
   - a supported player such as `mpv` or VLC;
   - `yt-dlp` or `ffmpeg` for downloads;
   - `ani-skip` only when intro skipping is enabled;
   - Syncplay only when that player is selected.
3. **Go 1.25.10 or newer only when building from source.** Release and npm installs do not require Go.

`fzf`, rofi, and dmenu are not needed: ani-tui is the menu frontend. Missing active dependencies are handled inside the TUI by the consent-based [dependency doctor](#dependency-doctor).

## Installation

Every route installs a global **`ani-tui`** command. None of them replaces or renames the upstream `ani-cli` command.

### Interactive curl installer (Linux and macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh | sh
ani-tui
```

The installer detects the operating system and CPU, lets an interactive user choose a user-local or system destination, shows download progress, verifies the selected GitHub Release artifact against `checksums.txt`, installs atomically, and checks `ani-tui --version`. The recommended user destination is `~/.local/bin`; if it is not already on `PATH`, the installer offers to update the appropriate shell profile.

Useful noninteractive forms:

```sh
# Accept defaults, installing the latest release for the current user
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh | sh -s -- --yes

# Install a pinned release system-wide (sudo may prompt)
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh | sh -s -- --version v0.1.0 --system
```

### npm (Linux, macOS, and Windows)

```sh
npm install --global ani-tui
ani-tui
```

The npm package supports Node.js 22 or newer and has no runtime JavaScript dependencies. Its postinstall script downloads the same-version native binary, verifies its SHA-256 checksum, and exposes `ani-tui` through npm's normal global shim—including an `ani-tui.cmd` launcher on Windows.

### GitHub Releases

Download the artifact for your platform from [GitHub Releases](https://github.com/Killersumit/ani-tui/releases):

| Platform | Assets |
|---|---|
| Linux | `ani-tui-linux-x64`, `ani-tui-linux-arm64`, `ani-tui-linux-armv7` |
| macOS | `ani-tui-darwin-x64`, `ani-tui-darwin-arm64` |
| Windows | `ani-tui-windows-x64.exe`, `ani-tui-windows-arm64.exe` |

Verify it with the release's `checksums.txt`, mark it executable on Unix, and place it in a directory on `PATH` under the name `ani-tui` (or `ani-tui.exe` on Windows). GitHub also publishes build-provenance attestations. With the GitHub CLI, an asset can be checked using:

```sh
gh attestation verify ./ani-tui-linux-x64 --repo Killersumit/ani-tui
```

### Build from source

```sh
git clone https://github.com/Killersumit/ani-tui.git
cd ani-tui
make check
make install PREFIX="$HOME/.local"
ani-tui
```

The Makefile embeds the version, commit, and UTC build time. `ani-tui --version` prints this release metadata. If `ani-cli` is not on `PATH`, launch ani-tui and use **Settings → ani-cli path**, or follow the dependency doctor's verified guidance.

## Uninstall

An uninstall removes the frontend command but preserves ani-tui configuration, ani-cli history, and downloaded media by default:

```sh
# npm installation
npm uninstall --global ani-tui

# Release/curl installation (interactive)
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/uninstall.sh | sh
```

To explicitly remove ani-tui's own configuration/cache/state as well:

```sh
curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/uninstall.sh | sh -s -- --purge
```

Even `--purge` never removes the separate `ani-cli` command, ani-cli history, or downloaded anime.

## Dependency doctor

ani-tui checks dependencies asynchronously at startup and again before each ani-cli operation. A launch-time check is blocking: ani-cli does not start until all requirements for that operation are available. The doctor checks core ani-cli tools plus only the optional tools enabled by the current configuration—for example, the selected player, Syncplay, intro skipping, the download backend, or `patch` for self-update. It does not require `fzf`, rofi, or dmenu.

When a requirement is missing, the active pane stays inside the persistent workspace and shows:

- the missing executable and why this operation needs it;
- the exact command ani-tui proposes to execute;
- a curated official package or package-manager URL;
- **yes/no** controls before any installation begins.

On confirmation, Bubble Tea temporarily releases terminal control to the installer, so `sudo`, password, repository, and package-manager prompts remain interactive. ani-tui then restores the full-screen interface and verifies that the required executable is actually resolvable; a zero installer exit status alone is not considered success.

Supported automatic routes are deliberately conservative:

| Platform | Detected managers |
|---|---|
| Linux | APT, DNF, pacman, Zypper, apk (and Homebrew when it is the detected fallback) |
| macOS | Homebrew |
| Windows | Scoop, winget |

Not every dependency has a safe package mapping on every distribution. In that case, the doctor gives official manual guidance instead of guessing a package name or enabling a third-party repository. A custom configured ani-cli path is never “repaired” by installing an unrelated default package. Scoop Extras plans explicitly add the `extras` bucket before installing its package.

Only failures classified as transient network/download errors are retried, with backoff, up to **five total attempts**. Permission errors, unavailable packages, bad arguments, checksum/signature failures, and all unknown failures stop immediately. Declining or canceling a blocking check leaves the launch stopped without changing the system.

## Keyboard map

| Key | Action |
|---|---|
| `/` | Focus a normal anime search |
| `enter` | Submit or select |
| `c` | Continue watching |
| `1`–`4` / `tab` | Discover, History, Settings, Activity |
| `ctrl+p`, `ctrl+k`, `:` | Open command palette |
| `↑` / `↓`, `j` / `k` | Navigate lists |
| `/` in an ani-cli selector | Filter choices |
| `space` in an episode selector | Mark range endpoints |
| `n`, `r`, `p`, `c`, `q` in Playback controls | Next, Replay, Previous, Change quality, Quit |
| `esc` | Back, close, or cancel a selector |
| `ctrl+c` | Cancel the running ani-cli task; quit when idle |
| `?` | Help |
| `q` | Quit when a text input is not focused |
| Mouse wheel | Navigate selectors, palette items, history, settings, and activity |
| Left click (wide layout) | Switch routes from the context pane |

Mouse handling can be toggled in **Settings → Mouse events** and takes effect after restart.

## Feature mapping

| ani-cli capability | ani-tui surface |
|---|---|
| Search and play | Discover search |
| `-c`, continue | `c` or command palette |
| `-e` / `-r`, exact episode/range | Command palette → **Play an exact episode or range** |
| `-S`, indexed search selection | Command palette → **Select search result by index** |
| `-d`, download | Settings → Default action |
| `-D`, delete history | Command palette with confirmation |
| `-l`, log viewer | Command palette → **Open ani-cli logs** |
| `-q`, quality | Settings |
| `-s`, Syncplay | Settings → Player |
| `-U [branch]`, update | Command palette; branch is configurable |
| `-v`, VLC | Settings → Player |
| `-N`, next episode countdown | Command palette |
| `--dub` | Settings → Audio mode |
| `--skip` | Settings → Skip intro |
| `--no-detach` | Settings → Detach player |
| `--exit-after-play` | Settings → Exit after play |
| `ANI_CLI_PLAYER` | Player / Custom player |
| `ANI_CLI_PLAYER_FLAGS` | Player flags |
| `ANI_CLI_DOWNLOAD_DIR` | Download directory |
| `ANI_CLI_HIST_DIR` | History directory |
| `ANI_CLI_LOG` | Playback logging |
| `ANI_CLI_BRANCH` | Update branch |
| Search, season, episode, and quality menus | Filterable selectors in the persistent active pane |
| Post-play Next, Replay, Previous, Change quality, Quit | Dedicated Playback controls in the same active pane; exact actions are returned to ani-cli |
| rofi/dmenu/fzf menu switches | Superseded by the native TUI selector |

## Configuration

The JSON configuration is stored at the first applicable location:

- `$ANI_TUI_CONFIG`, when set;
- `$XDG_CONFIG_HOME/ani-tui/config.json`, when set;
- Linux: `~/.config/ani-tui/config.json`;
- macOS: `~/Library/Application Support/ani-tui/config.json`;
- Windows: `%AppData%\\ani-tui\\config.json`.

It is written atomically with user-only (`0600`) file permissions where the platform supports Unix modes. Most users should edit it through the Settings screen.

Show its path with:

```sh
ani-tui --print-config
```

## How the integration works

ani-cli intentionally supports a custom menu program through `ANI_CLI_MENU`. ani-tui launches ani-cli with that variable pointing back to the same executable in helper mode. Each helper invocation:

1. reads candidate rows from standard input;
2. takes the final argument as ani-cli's prompt;
3. sends a token-authenticated JSON request to the full-screen parent over a random loopback TCP port;
4. waits while the user selects or cancels;
5. prints the selected **raw rows unchanged** to standard output.

The parent keeps terminal stdin attached to ani-cli so the script does not mistake the launch for a non-interactive pipeline and replace the custom menu with rofi or dmenu. Scraping, stream resolution, history updates, player startup, downloads, and self-update remain ani-cli's responsibility.

See [`docs/research-and-architecture.md`](docs/research-and-architecture.md) for the primary-source research and design rationale.

## Testing

```sh
make check
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
./scripts/test-distribution.sh
python3 ./scripts/test-installer-pty.py
(cd npm && npm test && npm pack --dry-run --ignore-scripts)
```

CI also formats, runs pinned `staticcheck`, scans reachable Go dependencies with `govulncheck`, lints the POSIX installers, builds every published OS/architecture target, verifies embedded version metadata, and inspects the npm tarball. The suite includes a mock ani-cli subprocess that exercises search, multi-row episode selection, repeated helper invocations, post-play actions, authenticated IPC, raw-row preservation, environment translation, configuration, history parsing, settings persistence, command startup, and mouse navigation. UI regressions verify that selectors, the palette, editors, confirmations, help, and dependency-doctor states remain inside the persistent split-pane shell, that compact mode retains application/session context, and that loading feedback moves without changing width. Doctor tests cover active requirement selection, executable alternatives, manager detection, exact plans, conservative mappings, Scoop bucket setup, transient/permanent classification, retry limits, stale async messages, launch blocking, terminal handoff, post-install verification, and bounded output capture. Real playback still depends on the user's installed ani-cli and media player.

## Troubleshooting

- **“ani-cli missing”** — accept a verified automatic route when one is offered, set an executable path in Settings, or put `ani-cli` on `PATH`. Custom paths require manual repair.
- **“Automatic installation is unavailable”** — no conservative mapping exists for the detected manager; follow the official manual link shown in the active pane.
- **Installer stopped** — inspect Activity for the attempt, failure classification, and bounded output. Only clearly transient network failures retry.
- **Player not found** — accept a verified mpv/VLC plan when offered or select another configured player.
- **Download fails** — install `yt-dlp` or `ffmpeg`, then check the Activity screen.
- **Intro skip fails** — install `ani-skip`; the feature is mpv-only upstream.
- **No logs** — enable Playback logging; Linux log viewing also depends on `journalctl` access.
- **Terminal is too small** — resize to at least 54×16; 88 columns or wider enables the persistent split-pane session workspace.

## Scope, attribution, and license

ani-tui is an independent frontend and is not affiliated with anime providers. It delegates lookup and playback behavior to [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli); follow upstream notices and the laws and terms that apply in your location.

Released under the [MIT License](LICENSE). See [CONTRIBUTING.md](CONTRIBUTING.md), [SUPPORT.md](SUPPORT.md), and [SECURITY.md](SECURITY.md) for project policies.
