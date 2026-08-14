# Research and architecture

Research snapshot: **14 August 2026**.

This document records the research performed before implementing ani-tui, the design lessons taken from OpenCode and Kilo Code, and the exact integration contract with ani-cli. Repository internals were inspected from local shallow clones; linked primary sources are provided so the conclusions are auditable.

## 1. What a terminal user interface is

A terminal user interface (TUI) is a stateful, screen-oriented application rendered through a terminal rather than a stream of prompts and answers. Both TUIs and ordinary CLIs use a terminal, but their interaction contracts differ:

| Streaming CLI | Full-screen TUI |
|---|---|
| Writes a chronological transcript | Reconciles a current screen from application state |
| Usually one command → one result | Long-lived event loop |
| Primarily line input | Keystrokes, focus, selection, resizing, sometimes mouse |
| Shell composition is central | In-app routes, overlays, lists, forms, and status are central |
| Output remains in scrollback | Usually uses the terminal's alternate screen |

A modern TUI therefore needs more than ANSI-colored text. It needs:

1. **State** — current route, focus, fields, selections, running work, errors, dimensions.
2. **Events** — keys, resize events, timers, process/IPC completion, and optional mouse events.
3. **Deterministic rendering** — a view derived from the latest state, with width-aware layout.
4. **Effects separated from updates** — subprocess and filesystem work must not block the event loop.
5. **Focus and modal ownership** — the command palette or selector must receive input before the page below it.
6. **Terminal lifecycle discipline** — enter/leave alternate screen, restore modes after errors/signals, and keep subprocess I/O from corrupting the screen.
7. **Discoverability** — visible shortcuts, a help layer, and a searchable command palette.
8. **Accessibility under constraints** — keyboard-complete operation, no color-only meaning, narrow-terminal fallback, and restrained animation.

The implementation selected the Elm-style Bubble Tea loop because these requirements map cleanly to `Model`, `Update`, `View`, and `Cmd`, while Go is easy to ship as one executable. This is an architectural choice, not an assertion that OpenCode or Kilo use Bubble Tea; their current implementations do not.

## 2. OpenCode primary-source study

Sources:

- [OpenCode repository](https://github.com/anomalyco/opencode)
- [TUI package](https://github.com/anomalyco/opencode/tree/dev/packages/tui)
- [Contributing guide](https://github.com/anomalyco/opencode/blob/dev/CONTRIBUTING.md)
- [OpenTUI](https://github.com/anomalyco/opentui)
- [Command registration/discoverability discussion](https://github.com/anomalyco/opencode/issues/5305)

Inspected snapshot: `anomalyco/opencode@e23586af2623f1bc2e8e6965d2d7acf7bd03d5c3`.

### 2.1 Technology and runtime

Current OpenCode's TUI is TypeScript/SolidJS on `@opentui/core`, `@opentui/solid`, and `@opentui/keymap`; its package does not implement the current UI in Go/Bubble Tea. OpenTUI supplies the terminal renderer and flex-style components, while Solid's reactive primitives compose application state into the tree.

`packages/tui/src/app.tsx` creates a CLI renderer with a target frame rate, explicit Ctrl-C handling, optional mouse support, selection handling, and terminal cleanup. The application layers context providers around routes: startup/runtime, configuration, project, SDK, synchronization, permissions, themes, dialogs, and related services. This is significant: terminal rendering is only one layer. Data synchronization and application policy are not embedded directly into every visual component.

### 2.2 Route and overlay model

OpenCode models navigation explicitly. At the inspected snapshot, its route context distinguishes home, session, and plugin routes, with typed route data and a central `navigate` operation. Dialogs are a separate stack above the current route. The dialog provider:

- remembers and blurs the previously focused renderable;
- pushes a modal keymap mode;
- handles Escape/Ctrl-C at the overlay boundary;
- restores focus when the stack clears;
- constrains modal width based on semantic sizes and terminal width;
- separates backdrop dismissal from text selection.

That design prevents one of the most common TUI failures: page actions firing while the user thinks a modal owns input.

### 2.3 Discoverability and progressive disclosure

OpenCode's command palette associates actions with titles, categories/groups, keybindings, and callbacks. Frequent actions can have direct keys, but remain searchable. Less frequent provider, model, theme, session, status, and diagnostic operations do not need permanent screen space. Dialogs and a sidebar disclose more context without turning the main route into a dense control panel.

### 2.4 Data and lifecycle lessons

The inspected app has SDK and synchronization contexts, event handling, permissions, cleanup/finalizers, an error boundary, startup loading, and explicit renderer destruction. The important lesson is not to conflate an interactive shell with a frontend. The UI consumes state and events from a runtime boundary; it does not synchronously perform every operation inside a key handler.

### 2.5 Lessons adopted

- Explicit routes rather than one enormous unstructured screen.
- Strict interaction/focus precedence, adapted into a persistent active pane rather than a stack of centered dialogs.
- Searchable command palette plus visible high-frequency shortcuts.
- Persistent status, navigation, and session context when width permits.
- Central theme tokens rather than ad hoc ANSI sequences.
- Async operation messages fed back into one state loop.
- Small-terminal fallback and render width calculations.
- Cleanup as a first-class lifecycle requirement.

## 3. Kilo Code primary-source study

Sources:

- [Kilo Code repository](https://github.com/Kilo-Org/kilocode)
- [CLI user documentation](https://kilo.ai/docs/code-with-ai/platforms/cli)
- [CLI reference](https://kilo.ai/docs/code-with-ai/platforms/cli-reference)
- [Architecture overview](https://kilo.ai/docs/contributing/architecture)
- [CLI/runtime boundary](https://kilo.ai/docs/contributing/architecture/cli-runtime)

Inspected snapshot: `Kilo-Org/kilocode@67cda85c94937a7dfad68993bdddc76cb0353c36` (release `v7.4.22`).

### 3.1 Relationship to OpenCode

Kilo's current CLI/TUI is an OpenCode-derived implementation, not an unrelated UI architecture. Its `packages/tui` retains the Solid/OpenTUI provider, route, prompt, dialog, plugin, theme, and synchronization structure, while using Kilo SDK/plugin packages and adding Kilo-specific integration.

Examples visible in the inspected source include:

- Kilo branding and terminal title behavior;
- Kilo SDK client creation;
- provider priority and authentication customization;
- model ordering/disclosure behavior;
- tool approvals and protected configuration messaging;
- network, terminal, question, and suggestion interaction arbitration;
- Kilo-specific session hydration, mentions, cost alerts, errors, and routes;
- configuration hot reload and workspace-scoped synchronization.

This demonstrates a useful extension pattern: preserve stable UI primitives and interaction rules while adapting the service boundary and domain-specific routes/components.

### 3.2 Runtime boundary

Kilo's architecture documentation describes clients communicating with a runtime/daemon and an embedded worker/RPC fallback. The practical TUI lesson is that the visual application should not own all backend logic. A typed client/event boundary permits the runtime to evolve independently and keeps asynchronous work from blocking rendering.

### 3.3 Lessons adopted

- Treat ani-cli as the authoritative runtime instead of copying its resolver into the frontend.
- Add a narrow adapter at the boundary rather than editing the upstream script.
- Keep frontend concepts domain-specific: Discover, History, Settings, Activity.
- Preserve escape hatches: custom player, extra player flags, update branch, and raw backend output.
- Put advanced behavior in a palette/settings layer without hiding core flows.

## 4. ani-cli primary-source study

Sources:

- [`pystardust/ani-cli`](https://github.com/pystardust/ani-cli)
- [README](https://github.com/pystardust/ani-cli/blob/master/README.md)
- [manual page](https://github.com/pystardust/ani-cli/blob/master/ani-cli.1)
- [environment-variable customization discussion](https://github.com/pystardust/ani-cli/discussions/591)
- [custom menu pull request history](https://github.com/pystardust/ani-cli/pull/801)

Inspected snapshot: `pystardust/ani-cli@92e9d796d23aef3ae94b52852f9c992e2bce4fe3` (ani-cli 5.0.2).

### 4.1 Upstream architecture

ani-cli is a POSIX shell program. At a high level it:

1. parses options and environment configuration;
2. obtains anime/search/episode/quality choices through a menu program;
3. queries the current upstream anime service and extracts media links;
4. updates history/log state;
5. starts a player, downloader, Syncplay, `catt`, or debug output;
6. may loop through post-play choices.

The current script uses `anidb.app`; older secondary material referring to AllAnime is stale for the inspected master.

ani-cli intentionally centralizes interactive choices in `menu()`. It resolves the menu executable from `ANI_CLI_MENU`, verifies it with quoted `command -v "$menu_program"`, and invokes it as `"$menu_program"`. This is the clean integration seam. The frontend should not parse human output after `fzf`; it should become the menu program.

### 4.2 Relevant feature contract

The current manual/script expose:

- search, continue, explicit episode or episode range;
- stream or download;
- history deletion and playback logs;
- quality choice;
- mpv, VLC, Syncplay, `catt`, debug, and custom player modes;
- dub mode and intro skipping;
- attached/no-detach playback and exit after play;
- indexed search-result selection;
- next-episode countdown;
- self-update with optional branch;
- configurable player flags, download/history paths, logging, branch, and menu program.

One integration detail is easy to get wrong: `-N` is a flag and consumes **no seconds argument**. Another is that ani-cli may make several menu calls in one process—search result, season, episodes, quality, then post-play—so the bridge must remain alive for the entire operation.

### 4.3 Why not scrape or fork ani-cli?

Duplicating its provider parsing in the TUI would immediately create two update-sensitive resolvers and violate the user's requirement that the frontend work *with* ani-cli. Calling ani-cli unchanged preserves upstream fixes, supported platforms, player/download behavior, and its environment-variable contract.

## 5. Chosen architecture

```text
┌────────────────────────────────────────────────────────────┐
│ ani-tui parent                                             │
│                                                            │
│ Bubble Tea model ─ workspace / workflow / config / output  │
│       ▲                       │                            │
│       │ bridge events         │ backend commands           │
│       │                       ▼                            │
│ token-authenticated TCP bridge on random 127.0.0.1 port    │
└──────────────▲───────────────────────────────┬─────────────┘
               │ JSON request/response         │ starts
               │                               ▼
┌──────────────┴───────────┐       ┌─────────────────────────┐
│ same ani-tui executable │◀──────│ ani-cli                 │
│ helper mode             │ menu  │ unmodified shell script │
│ stdin rows → bridge     │ calls │ resolver/player/history │
│ selected raw rows → out │       └─────────────────────────┘
└──────────────────────────┘
```

### 5.1 Parent TUI

The parent owns one authoritative `model` containing:

- route: Discover, History, Settings, or Activity;
- terminal dimensions and focus;
- search/editor state;
- history entries;
- inline palette, help, confirmation, editor, and ani-cli menu interactions;
- a workflow timeline of completed choices and the currently active request;
- current ani-cli operation and cancellation handle;
- dependency-doctor scan, offer, installer, backoff, and verification state;
- accumulated backend output;
- normalized persisted configuration.

Bubble Tea `Update` gives the active interaction precedence, then global bindings, then route-local bindings. `View` never replaces the application with a centered dialog. At 88 columns and wider it renders a persistent split pane: navigation and session flow on the left, with the active route or interaction on the right. Compact mode retains the global header/footer plus a two-line route/session context strip while replacing only the central content region. Under 54×16 it renders a clear resize message instead of corrupting the layout.

### 5.2 Backend adapter

The backend translates typed settings into ani-cli arguments and documented environment variables. It preserves a display-safe shell rendering for the Activity view, but passes raw argument strings to `exec.Cmd`; display quoting is never confused with process arguments.

The process manager:

- prevents overlapping operations;
- creates a cancellable child process;
- keeps terminal stdin attached;
- captures stdout/stderr without letting them paint over the TUI;
- emits completion output back into the event loop;
- can terminate the current process on Ctrl-C.

### 5.3 Recursive menu helper

The built executable checks for `--ani-tui-menu-helper` before launching Bubble Tea. The backend sets:

- `ANI_CLI_MENU` to the **raw absolute executable path**, including any whitespace;
- a random loopback address;
- a cryptographically random bearer token;
- a deadline.

The helper reads candidate rows exactly, sends prompt and rows as JSON, and prints selected rows without adding a newline. Exact preservation matters because ani-cli parses its own result labels and episode rows. Cancellation exits nonzero.

TCP rather than a Unix socket keeps the bridge straightforward across supported Go platforms; binding to `127.0.0.1:0` avoids a stable externally reachable service or fixed-port collision. The random token prevents another local process that happens to discover the port from submitting requests. The bridge closes when the parent exits.

### 5.4 Why terminal stdin remains attached

A naive implementation pipes all child descriptors. ani-cli checks whether stdin is a terminal and, in some non-interactive cases, can replace its chosen menu path with rofi/dmenu. The backend therefore leaves stdin attached while separately capturing stdout/stderr. Interactive choices still go through `ANI_CLI_MENU` and the bridge.

### 5.5 Dependency-doctor boundary

Dependency handling is a preflight state machine, not a synchronous key-handler check. Startup schedules a non-blocking informational scan. Every requested ani-cli operation schedules a tokenized blocking scan; the process manager receives no start command until that scan has no unresolved requirements. Scan, install-completion, and retry messages carry tokens and queue indices so canceled or superseded asynchronous results cannot mutate current UI state.

Requirements are derived from both the operation and normalized configuration:

- the configured ani-cli executable and its core `curl`, `sed`, and `grep` tools for ordinary/update work;
- only the player path active for this operation;
- Syncplay, `ani-skip`, a downloader, the platform log viewer, or `patch` only when the selected feature needs it;
- executable alternatives as groups, such as `yt-dlp` **or** `ffmpeg`, rather than requiring both.

Linux's early auto-player probe and Flatpak edge case are modeled separately from the final player check. Download mode does not inherit irrelevant player requirements, and a custom ani-cli path is not treated as repairable by installing a default distro package.

Package-manager detection is platform/distro aware and limited to known managers: APT, DNF, pacman, Zypper, apk, Homebrew, Scoop, and winget. Package IDs and source links are an allowlist curated from official manager documentation, distro inventories, Homebrew formula/cask pages, Scoop manifests, winget manifests, and upstream ani-cli guidance. Unsupported manager/dependency combinations intentionally fall back to an official manual URL. The doctor never enables a third-party repository or guesses that identical package names exist everywhere.

Each offered plan stores raw executable and argument vectors; it is executed directly without a shell. The separately rendered command is therefore disclosure, not an eval string. The executable path selected during manager detection is retained for execution. Non-root Linux system-manager plans require a detected `sudo`; otherwise no automatic plan is offered. Scoop Extras plans are ordered multi-step plans—`scoop bucket add extras` followed by `scoop install <package>`—and safely tolerate an already-present bucket.

The offer remains inline in the persistent active pane and shows the exact command chain and verified official URL before accepting `y`/`n`. Confirmation uses Bubble Tea's external-process handoff, which leaves the alternate screen and gives the installer's stdin/stdout/stderr direct terminal ownership for password and package-manager prompts. On return, the full-screen renderer resumes and checks the executable again. Installer success without a resolvable executable is a permanent verification failure.

Installer output is mirrored to the real terminal during handoff while a bounded tail is retained for diagnostics. Failure classification gives permanent conditions—permissions, unavailable packages, invalid arguments, signature/checksum failures—precedence over transient wording. Only explicit DNS/connectivity/timeouts, rate limiting, or transient HTTP server failures retry. Backoff is bounded to five total attempts; unknown failures stop immediately. Every attempt and result is added to Activity. Declining, canceling a scan/backoff, or exhausting an unresolved blocking queue prevents the ani-cli launch.

Primary references for the allowlist include the official [APT guide](https://www.debian.org/doc/manuals/apt-guide/ch2.en.html), [DNF command reference](https://dnf.readthedocs.io/en/latest/command_ref.html), [Arch pacman documentation](https://wiki.archlinux.org/title/Pacman), [Zypper install reference](https://documentation.suse.com/smart/systems-management/html/reference-zypper-install/index.html), [Alpine Package Keeper guide](https://wiki.alpinelinux.org/wiki/Alpine_Package_Keeper), [Homebrew manpage](https://docs.brew.sh/Manpage), [Scoop documentation](https://github.com/ScoopInstaller/Scoop), and [winget install reference](https://learn.microsoft.com/windows/package-manager/winget/install). Package identities were checked against official distro inventories, [Homebrew formulae](https://formulae.brew.sh/), [Scoop Main](https://github.com/ScoopInstaller/Main/tree/master/bucket) and [Extras](https://github.com/ScoopInstaller/Extras/tree/master/bucket) manifests, [winget manifests](https://github.com/microsoft/winget-pkgs/tree/master/manifests), and ani-cli's own platform instructions.

### 5.6 Configuration and history

Configuration follows XDG conventions, is normalized on load, and is atomically saved with `0600` permissions. It maps to upstream flags/environment rather than patching ani-cli.

History is read from the configured history directory, `ANI_CLI_HIST_DIR`, or the upstream XDG/default state path. Parsing is deliberately defensive: malformed lines are skipped instead of making the entire History route unusable.

## 6. Interaction design

### 6.1 Information architecture

- **Discover** — search-first home with Continue and quick actions.
- **History** — upstream watch history, filterable and playable.
- **Settings** — persistent upstream behavior and paths.
- **Activity** — current command, status, output, and error diagnostics.

The main screen is not a mirror of flags. Common intent is primary; implementation-level controls remain available under Settings and the palette.

### 6.2 Command palette

The palette is the progressive-disclosure layer learned from OpenCode/Kilo. It includes normal search, continue, exact episode/range, indexed selection, countdown, logs, update, history deletion, navigation, help, cancellation, and quit. High-frequency actions also have direct keys.

### 6.3 Persistent workspace and ani-cli selector behavior

Every upstream menu prompt appears in the workspace's active pane with its actual prompt and candidates. It does not cover the navigation/session context with a new pop-up. Typing filters case-insensitively. Episode prompts allow multiple marks so ani-cli can receive range endpoints. Enter sends the raw candidate rows; Escape cancels. When ani-cli asks its next question, the completed choice remains in the left-hand workflow timeline while the same active pane advances to the new request.

At wide widths the left pane combines stable route navigation with session status, a restrained animated progress rail, and completed/active/error stages. The right pane owns Discover, History, Settings, Activity, selectors, the command palette, confirmations, editors, and help. Compact terminals preserve the same interaction model through a route/session strip above the active region instead of falling back to centered dialogs.

### 6.4 Error and cancellation policy

Expected failures do not crash the renderer. They become status/output and a notice. The Activity route remains available for diagnostics. Ctrl-C cancels a running operation; when no operation owns it, Ctrl-C quits. Destructive history deletion requires explicit confirmation.

## 7. Verification strategy

Automated tests cover:

- exact flag and environment translation;
- countdown and update-branch edge cases;
- shell display quoting without contaminating raw process values;
- executable paths containing whitespace;
- bridge authentication and helper request/response behavior;
- multi-invocation mock ani-cli integration;
- raw candidate preservation and episode multi-selection;
- configuration defaults, normalization, permissions, and round trips;
- history path resolution and malformed records;
- command-palette feature reachability;
- settings editing and keyboard/mouse navigation;
- persistent-shell rendering for selectors, palette, confirmations, editors, and help;
- wide split-pane and compact route/session-context behavior;
- fixed-width subtle loading animation and workflow-stage transitions;
- responsive and too-small terminal rendering;
- race detection for the Go suite.

The release workflow runs `gofmt`, `go test -race ./...`, `go vet ./...`, and a stripped build. Real playback is necessarily an end-user integration surface because it depends on network availability, ani-cli's upstream service, and installed external players/downloaders.

## 8. Trade-offs and boundaries

- **No copied resolver:** requires an installed ani-cli, but avoids resolver drift.
- **Prompt inference:** ani-cli's menu API supplies prompt text, not a typed schema. Episode multi-select is inferred from upstream prompt wording. Raw rows remain authoritative.
- **Captured process output:** preserves the full-screen UI but means child status appears in Activity rather than terminal scrollback.
- **Keyboard-first:** optional Bubble Tea mouse events add wheel navigation and wide-layout sidebar route clicks; all workflows remain fully keyboard accessible.
- **One running operation:** simplifies ownership of a sequential ani-cli/menu conversation and avoids interleaved player/search state.
- **No media embedding:** the TUI orchestrates ani-cli; actual video stays in the selected external player, consistent with upstream behavior.
