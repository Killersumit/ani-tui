#!/bin/sh
# Safe uninstaller for ani-tui. User data is preserved unless --purge is used.

set -eu

PURGE=${ANI_TUI_PURGE:-0}
ASSUME_YES=${ANI_TUI_YES:-0}
INSTALL_DIR=${ANI_TUI_INSTALL_DIR:-}

usage() {
    cat <<'EOF'
Remove the ani-tui command while preserving user data by default.

Usage: uninstall.sh [options]

Options:
  --install-dir DIRECTORY Remove ani-tui from a specific directory
  --purge                 Also remove ani-tui configuration/cache/state
  -y, --yes               Do not ask for confirmation
  -h, --help              Show this help

The upstream ani-cli command, its history, and downloaded anime are never
removed. If npm installed ani-tui, this script uses npm uninstall --global.
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --install-dir)
            [ "$#" -ge 2 ] || { printf '%s\n' 'ani-tui uninstaller: --install-dir needs a value' >&2; exit 2; }
            INSTALL_DIR=$2
            shift 2
            ;;
        --install-dir=*) INSTALL_DIR=${1#*=}; shift ;;
        --purge) PURGE=1; shift ;;
        -y|--yes) ASSUME_YES=1; shift ;;
        -h|--help) usage; exit 0 ;;
        --) shift; break ;;
        *) printf 'ani-tui uninstaller: unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
    esac
done

[ "$#" -eq 0 ] || { printf 'ani-tui uninstaller: unexpected argument: %s\n' "$1" >&2; exit 2; }

if [ -t 2 ] && [ "${NO_COLOR:-}" = "" ]; then
    BOLD=$(printf '\033[1m')
    GREEN=$(printf '\033[32m')
    YELLOW=$(printf '\033[33m')
    DIM=$(printf '\033[2m')
    RESET=$(printf '\033[0m')
else
    BOLD='' GREEN='' YELLOW='' DIM='' RESET=''
fi

INTERACTIVE=0
if [ "$ASSUME_YES" != 1 ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then INTERACTIVE=1; fi

fail() { printf '%sani-tui uninstaller: %s%s\n' "$BOLD" "$*" "$RESET" >&2; exit 1; }
prompt_yes_no() {
    [ "$ASSUME_YES" = 1 ] && return 0
    [ "$INTERACTIVE" = 1 ] || return 1
    printf '%s [y/N] ' "$1" > /dev/tty
    IFS= read -r answer < /dev/tty || answer=
    case "$answer" in y|Y|yes|YES|Yes) return 0 ;; *) return 1 ;; esac
}

printf '%sani-tui uninstaller%s\n' "$BOLD" "$RESET"
printf '%sThe ani-cli backend, ani-cli history, and downloads will not be touched.%s\n\n' "$DIM" "$RESET"

NPM_MANAGED=0
NPM_PREFIX=
NPM_ROOT=
COMMAND_PATH=
if [ -n "$INSTALL_DIR" ]; then
    if [ -e "$INSTALL_DIR/ani-tui" ]; then
        COMMAND_PATH="$INSTALL_DIR/ani-tui"
    elif [ -e "$INSTALL_DIR/ani-tui.exe" ]; then
        COMMAND_PATH="$INSTALL_DIR/ani-tui.exe"
    fi
else
    COMMAND_PATH=$(command -v ani-tui 2>/dev/null || true)
    if [ -z "$COMMAND_PATH" ] && [ -n "${HOME:-}" ]; then
        for candidate in "$HOME/.local/bin/ani-tui" "$HOME/bin/ani-tui" /usr/local/bin/ani-tui \
            "$HOME/.local/bin/ani-tui.exe" "$HOME/bin/ani-tui.exe" /usr/local/bin/ani-tui.exe; do
            if [ -e "$candidate" ]; then
                COMMAND_PATH=$candidate
                break
            fi
        done
    fi
    if command -v npm >/dev/null 2>&1; then
        NPM_PREFIX=$(npm prefix --global 2>/dev/null || true)
        NPM_ROOT=$(npm root --global 2>/dev/null || true)
        if [ -n "$NPM_ROOT" ] && [ -f "$NPM_ROOT/ani-tui/package.json" ] && [ -n "$COMMAND_PATH" ]; then
            case "$COMMAND_PATH" in
                "$NPM_PREFIX"/*) NPM_MANAGED=1 ;;
            esac
        fi
    fi
fi

if [ "$NPM_MANAGED" = 1 ]; then
    printf 'Found npm installation under %s\n' "$NPM_PREFIX"
    if ! prompt_yes_no 'Remove the global npm package ani-tui?'; then
        fail 'uninstall cancelled'
    fi
    npm uninstall --global ani-tui
    printf '%sRemoved%s npm package ani-tui\n' "$GREEN" "$RESET"
elif [ -n "$COMMAND_PATH" ]; then
    case "$COMMAND_PATH" in
        */*) ;;
        *) fail "refusing to remove a command that is not an explicit path: $COMMAND_PATH" ;;
    esac
    case "$(basename "$COMMAND_PATH")" in
        ani-tui|ani-tui.exe) ;;
        *) fail "refusing to remove unexpected command path: $COMMAND_PATH" ;;
    esac
    printf 'Found %s\n' "$COMMAND_PATH"
    if ! prompt_yes_no "Remove $COMMAND_PATH?"; then
        fail 'uninstall cancelled'
    fi
    if rm -f "$COMMAND_PATH" 2>/dev/null; then
        :
    elif command -v sudo >/dev/null 2>&1; then
        sudo rm -f "$COMMAND_PATH"
    else
        fail "cannot remove $COMMAND_PATH (permission denied and sudo is unavailable)"
    fi
    [ ! -e "$COMMAND_PATH" ] || fail "could not remove $COMMAND_PATH"
    printf '%sRemoved%s %s\n' "$GREEN" "$RESET" "$COMMAND_PATH"
else
    printf '%sNo ani-tui command was found on PATH.%s\n' "$YELLOW" "$RESET"
fi

[ -n "${HOME:-}" ] || fail 'HOME is not set; cannot determine ani-tui data paths'
case "$(uname -s 2>/dev/null || true)" in
    Darwin)
        DEFAULT_CONFIG_BASE="$HOME/Library/Application Support"
        DEFAULT_CACHE_BASE="$HOME/Library/Caches"
        DEFAULT_STATE_BASE="$HOME/Library/Application Support"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        DEFAULT_CONFIG_BASE=${APPDATA:-$HOME/.config}
        DEFAULT_CACHE_BASE=${LOCALAPPDATA:-$HOME/.cache}
        DEFAULT_STATE_BASE=${LOCALAPPDATA:-$HOME/.local/state}
        ;;
    *)
        DEFAULT_CONFIG_BASE="$HOME/.config"
        DEFAULT_CACHE_BASE="$HOME/.cache"
        DEFAULT_STATE_BASE="$HOME/.local/state"
        ;;
esac
CONFIG_DIR="${XDG_CONFIG_HOME:-$DEFAULT_CONFIG_BASE}/ani-tui"
CACHE_DIR="${XDG_CACHE_HOME:-$DEFAULT_CACHE_BASE}/ani-tui"
STATE_DIR="${XDG_STATE_HOME:-$DEFAULT_STATE_BASE}/ani-tui"

if [ "$PURGE" = 1 ]; then
    if [ -n "${ANI_TUI_CONFIG:-}" ]; then
        CONFIG_PATH=$ANI_TUI_CONFIG
        rm -f "$CONFIG_PATH"
        printf '%sPurged%s %s\n' "$GREEN" "$RESET" "$CONFIG_PATH"
    else
        rm -rf "$CONFIG_DIR"
        printf '%sPurged%s %s\n' "$GREEN" "$RESET" "$CONFIG_DIR"
    fi
    [ "$CACHE_DIR" = "$CONFIG_DIR" ] || rm -rf "$CACHE_DIR"
    [ "$STATE_DIR" = "$CONFIG_DIR" ] || rm -rf "$STATE_DIR"
    printf '%sPurged%s %s\n' "$GREEN" "$RESET" "$CACHE_DIR"
    printf '%sPurged%s %s\n' "$GREEN" "$RESET" "$STATE_DIR"
else
    printf '\n%sPreserved user data%s (for example %s).\n' "$GREEN" "$RESET" "$CONFIG_DIR"
    printf 'Run again with %s--purge%s only if you want ani-tui data removed.\n' "$BOLD" "$RESET"
fi

printf '\n%sDone.%s ani-cli and its data remain installed.\n' "$BOLD" "$RESET"
