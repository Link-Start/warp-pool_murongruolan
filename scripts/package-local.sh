#!/usr/bin/env bash
set -Eeuo pipefail

ARCH="${ARCH:-amd64}"
OUTPUT_DIR="${OUTPUT_DIR:-dist}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/package-local.sh [amd64|arm64]

Environment:
  ARCH=amd64|arm64
  OUTPUT_DIR=dist
USAGE
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  usage
  exit 0
fi

if [ "${1:-}" != "" ]; then
  ARCH="$1"
fi

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "unsupported arch: $ARCH, expected amd64 or arm64" >&2; exit 1 ;;
esac

[ -f VERSION ] || { echo "VERSION not found" >&2; exit 1; }
[ -d assets ] || { echo "assets directory not found" >&2; exit 1; }

VERSION="$(tr -d '[:space:]' < VERSION)"
case "$VERSION" in
  *[!0-9.]*|"") echo "invalid VERSION: $VERSION" >&2; exit 1 ;;
esac

COMMIT="$(git rev-parse HEAD)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PACKAGE_NAME="warppool-linux-$ARCH"
WORK_DIR="$OUTPUT_DIR/$PACKAGE_NAME"
PACKAGE_PATH="$OUTPUT_DIR/$PACKAGE_NAME.tar.gz"

case "$WORK_DIR" in
  "$OUTPUT_DIR"/warppool-linux-*) ;;
  *) echo "refusing to remove unexpected work dir: $WORK_DIR" >&2; exit 1 ;;
esac
rm -rf -- "$WORK_DIR"
mkdir -p "$WORK_DIR/assets"

GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build \
  -ldflags "-s -w -X github.com/murongruolan/warp-pool/internal/cli.version=v$VERSION -X github.com/murongruolan/warp-pool/internal/cli.commit=$COMMIT -X github.com/murongruolan/warp-pool/internal/cli.date=$DATE" \
  -o "$WORK_DIR/warppool" ./cmd/warppool

cp -R assets/. "$WORK_DIR/assets/"
printf '%s' "$VERSION" >"$WORK_DIR/VERSION"
tar -C "$OUTPUT_DIR" -czf "$PACKAGE_PATH" "$PACKAGE_NAME"

printf 'local package created: %s\n' "$PACKAGE_PATH"
