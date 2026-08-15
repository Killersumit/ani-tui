#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
TMP_ROOT=${TMPDIR:-/tmp}
FIXTURE=$(mktemp -d "$TMP_ROOT/ani-tui-distribution.XXXXXX")
cleanup() { rm -rf "$FIXTURE"; }
trap cleanup 0 HUP INT TERM

case "$(uname -s)" in
    Linux) os=linux; suffix= ;;
    Darwin) os=darwin; suffix= ;;
    *) printf '%s\n' 'distribution fixture supports Linux and macOS test hosts' >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch=x64 ;;
    arm64|aarch64) arch=arm64 ;;
    armv7l) arch=armv7 ;;
    *) printf '%s\n' 'unsupported fixture architecture' >&2; exit 1 ;;
esac

asset="ani-tui-${os}-${arch}${suffix}"
BIN="$FIXTURE/home/.local/bin"
mkdir -p "$FIXTURE/release" "$BIN" "$FIXTURE/config/ani-tui" \
    "$FIXTURE/cache/ani-tui" "$FIXTURE/state/ani-tui"
cat > "$FIXTURE/release/$asset" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
    printf '%s\n' 'ani-tui v9.9.9-fixture (commit test)'
    exit 0
fi
printf '%s\n' 'fixture'
EOF
chmod 0755 "$FIXTURE/release/$asset"
if command -v sha256sum >/dev/null 2>&1; then
    hash=$(sha256sum "$FIXTURE/release/$asset" | awk '{print $1}')
else
    hash=$(shasum -a 256 "$FIXTURE/release/$asset" | awk '{print $1}')
fi
printf '%s  %s\n' "$hash" "$asset" > "$FIXTURE/release/checksums.txt"
printf '%s\n' '{"mouse":true}' > "$FIXTURE/config/ani-tui/config.json"
printf '%s\n' 'cache fixture' > "$FIXTURE/cache/ani-tui/cache.db"
printf '%s\n' 'state fixture' > "$FIXTURE/state/ani-tui/history.json"

ANI_TUI_DOWNLOAD_BASE="file://$FIXTURE/release" \
ANI_TUI_INSTALL_DIR="$BIN" \
    sh "$ROOT/install.sh" --yes --no-modify-path >/dev/null
"$BIN/ani-tui" --version | grep -q 'v9.9.9-fixture'
[ "$(find "$BIN/ani-tui" -perm 0755 -print)" = "$BIN/ani-tui" ]

# The uninstaller can find the standard user-local command even before a new
# shell has added that directory to PATH.
HOME="$FIXTURE/home" XDG_CONFIG_HOME="$FIXTURE/config" \
XDG_CACHE_HOME="$FIXTURE/cache" XDG_STATE_HOME="$FIXTURE/state" \
    sh "$ROOT/uninstall.sh" --yes >/dev/null
[ ! -e "$BIN/ani-tui" ]
[ -e "$FIXTURE/config/ani-tui/config.json" ]
[ -e "$FIXTURE/cache/ani-tui/cache.db" ]
[ -e "$FIXTURE/state/ani-tui/history.json" ]

ANI_TUI_DOWNLOAD_BASE="file://$FIXTURE/release" \
ANI_TUI_INSTALL_DIR="$BIN" \
    sh "$ROOT/install.sh" --yes --no-modify-path >/dev/null
HOME="$FIXTURE/home" XDG_CONFIG_HOME="$FIXTURE/config" XDG_CACHE_HOME="$FIXTURE/cache" XDG_STATE_HOME="$FIXTURE/state" \
ANI_TUI_INSTALL_DIR="$BIN" \
    sh "$ROOT/uninstall.sh" --yes --purge >/dev/null
[ ! -e "$BIN/ani-tui" ]
[ ! -e "$FIXTURE/config/ani-tui" ]
[ ! -e "$FIXTURE/cache/ani-tui" ]
[ ! -e "$FIXTURE/state/ani-tui" ]

mkdir -p "$FIXTURE/bad-release"
cp "$FIXTURE/release/$asset" "$FIXTURE/bad-release/$asset"
printf '%064d  %s\n' 0 "$asset" > "$FIXTURE/bad-release/checksums.txt"
if ANI_TUI_DOWNLOAD_BASE="file://$FIXTURE/bad-release" ANI_TUI_INSTALL_DIR="$FIXTURE/bad-bin" \
    sh "$ROOT/install.sh" --yes --no-modify-path >/dev/null 2>&1; then
    printf '%s\n' 'installer accepted an invalid checksum' >&2
    exit 1
fi
[ ! -e "$FIXTURE/bad-bin/ani-tui" ]

printf '%s\n' 'distribution installer/uninstaller fixture passed'
