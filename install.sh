#!/bin/sh
# Interactive, checksum-verifying installer for ani-tui.
# Usage: curl -fsSL https://raw.githubusercontent.com/Killersumit/ani-tui/main/install.sh | sh

set -eu

REPOSITORY="Killersumit/ani-tui"
VERSION=${ANI_TUI_VERSION:-latest}
INSTALL_DIR=${ANI_TUI_INSTALL_DIR:-}
ASSUME_YES=${ANI_TUI_YES:-0}
SYSTEM_INSTALL=0
MODIFY_PATH=1

usage() {
    cat <<'EOF'
Install ani-tui from an official GitHub release.

Usage: install.sh [options]

Options:
  --version VERSION       Install a release such as v0.1.0 (default: latest)
  --install-dir DIRECTORY Install into a specific directory
  --user                  Install into ~/.local/bin (the default)
  --system                Install into /usr/local/bin (may require sudo)
  -y, --yes               Accept defaults and replace an existing binary
  --no-modify-path        Do not offer to update a shell profile
  -h, --help              Show this help

Environment equivalents: ANI_TUI_VERSION, ANI_TUI_INSTALL_DIR, ANI_TUI_YES.
For test mirrors, ANI_TUI_DOWNLOAD_BASE may override the release download URL.
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version)
            [ "$#" -ge 2 ] || { printf '%s\n' 'ani-tui installer: --version needs a value' >&2; exit 2; }
            VERSION=$2
            shift 2
            ;;
        --version=*) VERSION=${1#*=}; shift ;;
        --install-dir)
            [ "$#" -ge 2 ] || { printf '%s\n' 'ani-tui installer: --install-dir needs a value' >&2; exit 2; }
            INSTALL_DIR=$2
            shift 2
            ;;
        --install-dir=*) INSTALL_DIR=${1#*=}; shift ;;
        --user)
            [ -n "${HOME:-}" ] || { printf '%s\n' 'ani-tui installer: HOME is not set; use --install-dir' >&2; exit 2; }
            INSTALL_DIR="$HOME/.local/bin"
            SYSTEM_INSTALL=0
            shift
            ;;
        --system)
            INSTALL_DIR=/usr/local/bin
            SYSTEM_INSTALL=1
            shift
            ;;
        -y|--yes) ASSUME_YES=1; shift ;;
        --no-modify-path) MODIFY_PATH=0; shift ;;
        -h|--help) usage; exit 0 ;;
        --) shift; break ;;
        *) printf 'ani-tui installer: unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
    esac
done

if [ "$#" -ne 0 ]; then
    printf 'ani-tui installer: unexpected argument: %s\n' "$1" >&2
    exit 2
fi

if [ -t 2 ] && [ "${NO_COLOR:-}" = "" ]; then
    BOLD=$(printf '\033[1m')
    GREEN=$(printf '\033[32m')
    LIME=$(printf '\033[38;5;154m')
    DIM=$(printf '\033[2m')
    RESET=$(printf '\033[0m')
else
    BOLD='' GREEN='' LIME='' DIM='' RESET=''
fi

INTERACTIVE=0
if [ "$ASSUME_YES" != 1 ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
    INTERACTIVE=1
fi

say() { printf '%s\n' "$*"; }
step() { printf '\n%s[%s/5]%s %s%s%s\n' "$LIME" "$1" "$RESET" "$BOLD" "$2" "$RESET"; }
fail() { printf '\n%sani-tui installer: %s%s\n' "$BOLD" "$*" "$RESET" >&2; exit 1; }

prompt_yes_no() {
    prompt=$1
    default=${2:-yes}
    [ "$ASSUME_YES" = 1 ] && return 0
    [ "$INTERACTIVE" = 1 ] || return 1
    if [ "$default" = yes ]; then suffix='[Y/n]'; else suffix='[y/N]'; fi
    while :; do
        printf '%s %s ' "$prompt" "$suffix" > /dev/tty
        IFS= read -r answer < /dev/tty || answer=
        case "$answer" in
            y|Y|yes|YES|Yes) return 0 ;;
            n|N|no|NO|No) return 1 ;;
            '') if [ "$default" = yes ]; then return 0; else return 1; fi ;;
            *) printf '%s\n' '  Please answer yes or no.' > /dev/tty ;;
        esac
    done
}

say "${BOLD}ani-tui installer${RESET}"
say "${DIM}A full-screen interface for pystardust/ani-cli${RESET}"

step 1 "Detecting your system"
OS_RAW=$(uname -s 2>/dev/null || true)
ARCH_RAW=$(uname -m 2>/dev/null || true)
case "$OS_RAW" in
    Linux) OS=linux; EXE_SUFFIX= ;;
    Darwin) OS=darwin; EXE_SUFFIX= ;;
    MINGW*|MSYS*|CYGWIN*) OS=windows; EXE_SUFFIX=.exe ;;
    *) fail "unsupported operating system: ${OS_RAW:-unknown}. Use a GitHub release asset instead." ;;
esac
case "$ARCH_RAW" in
    x86_64|amd64) ARCH=x64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    armv7l) ARCH=armv7 ;;
    *) fail "unsupported architecture: ${ARCH_RAW:-unknown}. Use a supported GitHub release asset." ;;
esac
if [ "$OS" != linux ] && [ "$ARCH" = armv7 ]; then
    fail "$OS/$ARCH is not a supported release target"
fi
say "  Platform  ${OS}/${ARCH}"

if [ -z "$INSTALL_DIR" ]; then
    [ -n "${HOME:-}" ] || fail 'HOME is not set; use --install-dir'
    if [ "$INTERACTIVE" = 1 ]; then
        say ""
        say "  Installation scope:"
        say "    ${LIME}1${RESET}) User   ${DIM}$HOME/.local/bin (recommended, no sudo)${RESET}"
        say "    ${LIME}2${RESET}) System ${DIM}/usr/local/bin (may ask for sudo)${RESET}"
        printf '  Choose [1]: ' > /dev/tty
        IFS= read -r scope < /dev/tty || scope=
        case "$scope" in
            2) INSTALL_DIR=/usr/local/bin; SYSTEM_INSTALL=1 ;;
            ''|1) INSTALL_DIR="$HOME/.local/bin" ;;
            *) fail "invalid installation scope: $scope" ;;
        esac
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi
fi
say "  Destination $INSTALL_DIR/ani-tui${EXE_SUFFIX}"

case "$VERSION" in
    latest) RELEASE_PATH=latest/download ;;
    v*)
        printf '%s' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$' || fail "invalid release version: $VERSION"
        RELEASE_PATH="download/$VERSION"
        ;;
    *)
        VERSION="v$VERSION"
        printf '%s' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$' || fail "invalid release version: $VERSION"
        RELEASE_PATH="download/$VERSION"
        ;;
esac
DEFAULT_BASE="https://github.com/$REPOSITORY/releases/$RELEASE_PATH"
DOWNLOAD_BASE=${ANI_TUI_DOWNLOAD_BASE:-$DEFAULT_BASE}
ASSET="ani-tui-${OS}-${ARCH}${EXE_SUFFIX}"
CHECKSUMS=checksums.txt

TMP_ROOT=${TMPDIR:-/tmp}
TMP_DIR=$(mktemp -d "$TMP_ROOT/ani-tui.XXXXXX" 2>/dev/null || mktemp -d 2>/dev/null) || fail 'could not create a temporary directory'
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup 0 HUP INT TERM

if command -v curl >/dev/null 2>&1; then
    download() {
        url=$1 destination=$2
        if [ -t 2 ]; then
            curl -fL --retry 3 --retry-delay 1 --progress-bar "$url" -o "$destination"
        else
            curl -fsSL --retry 3 --retry-delay 1 "$url" -o "$destination"
        fi
    }
elif command -v wget >/dev/null 2>&1; then
    download() {
        url=$1 destination=$2
        if [ -t 2 ]; then
            wget --show-progress -O "$destination" "$url"
        else
            wget -q -O "$destination" "$url"
        fi
    }
else
    fail 'curl or wget is required to download ani-tui'
fi

step 2 "Downloading the ${VERSION} release"
say "  Asset     $ASSET"
if ! download "$DOWNLOAD_BASE/$ASSET" "$TMP_DIR/$ASSET"; then
    if [ "$DOWNLOAD_BASE" = "$DEFAULT_BASE" ]; then
        fail "release asset unavailable: $ASSET. No binary was installed. The maintainer must publish the requested GitHub Release first."
    fi
    fail "download failed: $DOWNLOAD_BASE/$ASSET"
fi
if ! download "$DOWNLOAD_BASE/$CHECKSUMS" "$TMP_DIR/$CHECKSUMS"; then
    if [ "$DOWNLOAD_BASE" = "$DEFAULT_BASE" ]; then
        fail "checksums.txt is missing from the requested GitHub Release; no binary was installed"
    fi
    fail "could not download release checksums"
fi

step 3 "Verifying SHA-256 checksum"
EXPECTED=$(awk -v file="$ASSET" '$2 == file || $2 == ("*" file) { print $1; exit }' "$TMP_DIR/$CHECKSUMS")
[ -n "$EXPECTED" ] || fail "$ASSET is missing from checksums.txt"
case "$EXPECTED" in
    *[!0-9A-Fa-f]*|'') fail "checksums.txt contains an invalid SHA-256 value for $ASSET" ;;
esac
[ "${#EXPECTED}" -eq 64 ] || fail "checksums.txt contains an invalid SHA-256 value for $ASSET"
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "$TMP_DIR/$ASSET" | awk '{print $1}')
elif command -v openssl >/dev/null 2>&1; then
    ACTUAL=$(openssl dgst -sha256 "$TMP_DIR/$ASSET" | awk '{print $NF}')
else
    fail 'sha256sum, shasum, or openssl is required to verify the release'
fi
EXPECTED=$(printf '%s' "$EXPECTED" | tr 'A-F' 'a-f')
ACTUAL=$(printf '%s' "$ACTUAL" | tr 'A-F' 'a-f')
[ "$EXPECTED" = "$ACTUAL" ] || fail "checksum mismatch for $ASSET; the file was not installed"
say "  ${GREEN}Verified${RESET}  $ACTUAL"

TARGET="$INSTALL_DIR/ani-tui${EXE_SUFFIX}"
if [ -e "$TARGET" ] && ! prompt_yes_no "  Replace the existing $TARGET?" yes; then
    fail 'installation cancelled; the existing binary was not changed'
fi

step 4 "Installing the global ani-tui command"
install_unprivileged() {
    mkdir -p "$INSTALL_DIR" || return 1
    test_file="$INSTALL_DIR/.ani-tui-write-test.$$"
    if (umask 077 && : > "$test_file") 2>/dev/null; then
        rm -f "$test_file"
    else
        rm -f "$test_file" 2>/dev/null || true
        return 1
    fi
    temp_target="$TARGET.tmp.$$"
    cp "$TMP_DIR/$ASSET" "$temp_target" || return 1
    chmod 0755 "$temp_target" || return 1
    mv -f "$temp_target" "$TARGET" || return 1
}

if ! install_unprivileged; then
    [ "$OS" != windows ] || fail "cannot write to $INSTALL_DIR; choose a user-writable --install-dir"
    command -v sudo >/dev/null 2>&1 || fail "cannot write to $INSTALL_DIR and sudo is unavailable"
    say "  Administrative permission is needed for $INSTALL_DIR."
    sudo mkdir -p "$INSTALL_DIR"
    privileged_temp=$(sudo mktemp "$INSTALL_DIR/.ani-tui-install.XXXXXX") || fail "could not create a temporary file in $INSTALL_DIR"
    if ! sudo install -m 0755 "$TMP_DIR/$ASSET" "$privileged_temp" || ! sudo mv -f "$privileged_temp" "$TARGET"; then
        sudo rm -f "$privileged_temp" 2>/dev/null || true
        fail "could not install ani-tui into $INSTALL_DIR"
    fi
fi

[ -x "$TARGET" ] || fail "installation did not create an executable at $TARGET"
INSTALLED_VERSION=$("$TARGET" --version 2>/dev/null || true)
[ -n "$INSTALLED_VERSION" ] || fail 'the installed binary did not pass its version check'
say "  ${GREEN}Installed${RESET} $INSTALLED_VERSION"

path_has_dir() {
    case ":${PATH:-}:" in *":$1:"*) return 0 ;; *) return 1 ;; esac
}

quote_for_shell() {
    escaped=$(printf '%s' "$1" | sed "s/'/'\\\\''/g")
    printf "'%s'" "$escaped"
}

PATH_NOTE=
if ! path_has_dir "$INSTALL_DIR"; then
    shell_name=$(basename "${SHELL:-sh}")
    QUOTED_INSTALL_DIR=$(quote_for_shell "$INSTALL_DIR")
    case "$shell_name" in
        fish)
            PATH_LINE="fish_add_path --global --prepend $QUOTED_INSTALL_DIR"
            PROFILE="$HOME/.config/fish/config.fish"
            ;;
        zsh)
            PATH_LINE="export PATH=$QUOTED_INSTALL_DIR:\"\$PATH\""
            PROFILE="$HOME/.zshrc"
            ;;
        bash)
            PATH_LINE="export PATH=$QUOTED_INSTALL_DIR:\"\$PATH\""
            PROFILE="$HOME/.bashrc"
            ;;
        *)
            PATH_LINE="export PATH=$QUOTED_INSTALL_DIR:\"\$PATH\""
            PROFILE="$HOME/.profile"
            ;;
    esac
    PATH_NOTE="Run this now, or open a new terminal: $PATH_LINE"
    if [ "$INTERACTIVE" = 1 ] && [ "$MODIFY_PATH" = 1 ] && [ "$SYSTEM_INSTALL" = 0 ]; then
        if { [ ! -f "$PROFILE" ] || ! grep -F "$PATH_LINE" "$PROFILE" >/dev/null 2>&1; } &&
            prompt_yes_no "  Add $INSTALL_DIR to your shell PATH?" yes; then
            mkdir -p "$(dirname "$PROFILE")"
            {
                printf '\n# Added by the ani-tui installer\n'
                printf '%s\n' "$PATH_LINE"
            } >> "$PROFILE"
            say "  Updated   $PROFILE"
        fi
    fi
fi

step 5 "Ready"
say "  Launch with:  ${BOLD}ani-tui${RESET}"
say "  Backend:      ani-cli remains a separate upstream command"
say "  If ani-cli or an active optional dependency is missing, ani-tui's"
say "  dependency doctor will explain it and ask before installing anything."
if [ -n "$PATH_NOTE" ]; then
    say ""
    say "  ${LIME}$PATH_NOTE${RESET}"
fi
say ""
say "  Uninstall without deleting user data:"
say "  ${DIM}curl -fsSL https://raw.githubusercontent.com/$REPOSITORY/main/uninstall.sh | sh${RESET}"
