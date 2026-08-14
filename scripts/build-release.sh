#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VERSION=${VERSION:-}
GO=${GO:-go}
OUTPUT=${OUTPUT:-release}
[ -n "$VERSION" ] || { printf '%s\n' 'build-release: VERSION is required (for example v0.1.0)' >&2; exit 2; }
printf '%s' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$' || {
    printf 'build-release: invalid release version: %s\n' "$VERSION" >&2
    exit 2
}

COMMIT=${COMMIT:-$(git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)}
BUILD_DATE=${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildDate=$BUILD_DATE"

rm -rf "$OUTPUT"
mkdir -p "$OUTPUT"

build_target() {
    os=$1 arch=$2 release_arch=$3 suffix=$4 goarm=$5
    asset="ani-tui-${os}-${release_arch}${suffix}"
    printf 'Building %-29s (%s/%s)\n' "$asset" "$os" "$arch"
    if [ -n "$goarm" ]; then
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=$goarm "$GO" build -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT/$asset" .
    else
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch "$GO" build -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT/$asset" .
    fi
}

build_target linux amd64 x64 '' ''
build_target linux arm64 arm64 '' ''
build_target linux arm armv7 '' 7
build_target darwin amd64 x64 '' ''
build_target darwin arm64 arm64 '' ''
build_target windows amd64 x64 .exe ''
build_target windows arm64 arm64 .exe ''

(
    cd "$OUTPUT"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum ani-tui-* > checksums.txt
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 ani-tui-* > checksums.txt
    else
        printf '%s\n' 'build-release: sha256sum or shasum is required' >&2
        exit 1
    fi
)
printf '\nRelease artifacts are ready in %s/\n' "$OUTPUT"
