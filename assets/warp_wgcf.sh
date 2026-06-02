#!/usr/bin/env bash
set -Eeuo pipefail

POLICY="auto"
WGCF_VERSION="${WARPPOOL_WGCF_VERSION:-v2.2.31}"
INSTALL_DIR="${WARPPOOL_WGCF_INSTALL_DIR:-/usr/local/lib/warppool/bin}"
STATE_DIR="${WARPPOOL_WGCF_STATE_DIR:-/etc/warppool-node/warp}"
DRY_RUN="false"

log() {
  printf '[WarpPool][wgcf] %s\n' "$*"
}

fail() {
  printf '[WarpPool][wgcf][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][wgcf][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool wgcf helper

Usage:
  bash warp_wgcf.sh [policy=auto|reuse|reinstall] [version=v2.2.31] [install_dir=/path] [state_dir=/path] [--dry-run]

This helper downloads wgcf and generates a Cloudflare WARP WireGuard profile.
It is primarily used on Alpine, where the official cloudflare-warp package is
not available through apk.
USAGE
}

parse_args() {
  for arg in "$@"; do
    case "$arg" in
      --help|-h)
        usage
        exit 0
        ;;
      --dry-run)
        DRY_RUN="true"
        ;;
      policy=*) POLICY="${arg#policy=}" ;;
      version=*) WGCF_VERSION="${arg#version=}" ;;
      install_dir=*) INSTALL_DIR="${arg#install_dir=}" ;;
      state_dir=*) STATE_DIR="${arg#state_dir=}" ;;
      *) fail "unknown argument: $arg" ;;
    esac
  done
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: $*"
    return 0
  fi
  "$@"
}

require_root() {
  if [ "$DRY_RUN" = "true" ]; then
    return 0
  fi
  [ "$(id -u)" -eq 0 ] || fail "must run as root"
}

require_command() {
  local name="$1"
  command -v "$name" >/dev/null 2>&1 || fail "required command not found: $name"
}

normalize_args() {
  case "$POLICY" in
    auto|reuse|reinstall) ;;
    *) fail "unsupported policy: $POLICY, expected auto, reuse, or reinstall" ;;
  esac

  WGCF_TAG="$WGCF_VERSION"
  case "$WGCF_TAG" in
    v*) ;;
    *) WGCF_TAG="v$WGCF_TAG" ;;
  esac
  WGCF_NUMBER="${WGCF_TAG#v}"

  PROFILE_PATH="$STATE_DIR/wgcf-profile.conf"
  ACCOUNT_PATH="$STATE_DIR/wgcf-account.toml"
  WGCF_BIN="$INSTALL_DIR/wgcf"
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)
      WGCF_ARCH="amd64"
      ;;
    aarch64|arm64)
      WGCF_ARCH="arm64"
      ;;
    *)
      fail "unsupported CPU architecture for wgcf: $arch"
      ;;
  esac
}

install_base_packages() {
  if command -v apk >/dev/null 2>&1; then
    run apk add --no-cache curl ca-certificates
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    run env DEBIAN_FRONTEND=noninteractive apt-get update
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates
    return 0
  fi
  require_command curl
}

download_wgcf() {
  if [ -x "$WGCF_BIN" ] && [ "$POLICY" != "reinstall" ]; then
    log "reusing wgcf: $WGCF_BIN"
    return 0
  fi

  detect_arch
  local url
  url="https://github.com/ViRb3/wgcf/releases/download/$WGCF_TAG/wgcf_${WGCF_NUMBER}_linux_$WGCF_ARCH"

  run mkdir -p "$INSTALL_DIR"
  log "downloading wgcf from $url"
  run curl -fL -o "$WGCF_BIN" "$url"
  run chmod 0755 "$WGCF_BIN"

  if [ "$DRY_RUN" != "true" ] && ! "$WGCF_BIN" --help >/dev/null 2>&1; then
    fail "downloaded wgcf cannot run: $WGCF_BIN"
  fi
}

profile_exists() {
  [ -s "$PROFILE_PATH" ]
}

generate_profile() {
  if profile_exists && [ "$POLICY" != "reinstall" ]; then
    log "reusing WARP WireGuard profile: $PROFILE_PATH"
    return 0
  fi

  if [ "$POLICY" = "reuse" ]; then
    fail "WARP WireGuard profile not found: $PROFILE_PATH"
  fi

  run mkdir -p "$STATE_DIR"
  if [ "$POLICY" = "reinstall" ]; then
    run rm -f "$PROFILE_PATH" "$ACCOUNT_PATH"
  fi

  log "generating Cloudflare WARP WireGuard profile through wgcf"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: cd $STATE_DIR && $WGCF_BIN register --accept-tos && $WGCF_BIN generate"
    return 0
  fi

  (
    cd "$STATE_DIR"
    if [ ! -s "$ACCOUNT_PATH" ]; then
      "$WGCF_BIN" register --accept-tos
    else
      log "reusing wgcf account: $ACCOUNT_PATH"
    fi
    "$WGCF_BIN" generate
  )
  [ -s "$PROFILE_PATH" ] || fail "wgcf did not generate profile: $PROFILE_PATH"
  chmod 0600 "$PROFILE_PATH" "$ACCOUNT_PATH" 2>/dev/null || true
}

main() {
  parse_args "$@"
  normalize_args
  require_root
  install_base_packages
  download_wgcf
  generate_profile
  log "WARP WireGuard profile ready: $PROFILE_PATH"
}

main "$@"
