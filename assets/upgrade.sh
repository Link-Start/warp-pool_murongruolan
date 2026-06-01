#!/usr/bin/env bash
set -Eeuo pipefail

REPO="${WARPPOOL_REPO:-${WARPOOL_REPO:-murongruolan/warp-pool}}"
VERSION_ARG="${WARPPOOL_VERSION:-${WARPOOL_VERSION:-latest}}"
INSTALL_DIR="${WARPPOOL_INSTALL_DIR:-${WARPOOL_INSTALL_DIR:-/usr/local/bin}}"
LIB_DIR="${WARPPOOL_LIB_DIR:-${WARPOOL_LIB_DIR:-/usr/local/lib/warppool}}"
CONFIG_PATH="${WARPPOOL_CONFIG_PATH:-${WARPOOL_CONFIG_PATH:-/etc/warppool/config.json}}"
DRY_RUN="false"
YES="false"
WORK_DIR=""
expected=""
actual=""

log() {
  printf '[WarpPool][upgrade] %s\n' "$*"
}

fail() {
  printf '[WarpPool][upgrade][ERROR] %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf -- "$WORK_DIR"
  fi
}

trap cleanup EXIT

usage() {
  cat <<'USAGE'
WarpPool upgrade helper

Usage:
  warppool upgrade [--version latest|v0.1.1] [--yes] [--dry-run]

Environment:
  WARPPOOL_REPO=owner/name
  WARPPOOL_INSTALL_DIR=/usr/local/bin
  WARPPOOL_LIB_DIR=/usr/local/lib/warppool
  WARPPOOL_CONFIG_PATH=/etc/warppool/config.json
USAGE
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --help|-h)
        usage
        exit 0
        ;;
      --version)
        shift
        [ "$#" -gt 0 ] || fail "--version requires a value"
        VERSION_ARG="$1"
        ;;
      --version=*)
        VERSION_ARG="${1#--version=}"
        ;;
      --repo)
        shift
        [ "$#" -gt 0 ] || fail "--repo requires a value"
        REPO="$1"
        ;;
      --repo=*)
        REPO="${1#--repo=}"
        ;;
      --yes|-y)
        YES="true"
        ;;
      --dry-run)
        DRY_RUN="true"
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
    shift
  done
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail "upgrade must run as root"
  fi
}

detect_arch() {
  local raw
  raw="$(uname -m)"
  case "$raw" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported CPU architecture: $raw" ;;
  esac
}

fetch() {
  local url="$1"
  local target="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL "$url" -o "$target"
    return $?
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -O "$target" "$url"
    return $?
  fi
  fail "curl or wget is required"
}

release_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  if [ "$VERSION_ARG" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$REPO" "$asset"
    return 0
  fi
  local tag="$VERSION_ARG"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$tag" "$asset"
}

checksum_url() {
  if [ "$VERSION_ARG" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/checksums.txt\n' "$REPO"
    return 0
  fi
  local tag="$VERSION_ARG"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/checksums.txt\n' "$REPO" "$tag"
}

fallback_download_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  local tag
  tag="$(tr -d '[:space:]' </usr/local/lib/warppool/VERSION 2>/dev/null || true)"
  [ -n "$tag" ] || tag="$(tr -d '[:space:]' </etc/warppool/VERSION 2>/dev/null || true)"
  [ -n "$tag" ] || tag="0.1.1"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$tag" "$asset"
}

confirm() {
  if [ "$YES" = "true" ]; then
    return 0
  fi
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    fail "non-interactive upgrade requires --yes"
  fi
  printf 'Upgrade WarpPool from %s? [y/N]: ' "$(release_url)" >/dev/tty
  read -r answer </dev/tty
  case "$answer" in
    y|Y|yes|YES) ;;
    *) fail "upgrade cancelled" ;;
  esac
}

backup_existing() {
  BACKUP_DIR="/var/backups/warppool/$(date -u +%Y%m%dT%H%M%SZ)"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: backup existing installation to $BACKUP_DIR"
    return 0
  fi
  mkdir -p "$BACKUP_DIR"
  if [ -x "$INSTALL_DIR/warppool" ]; then
    cp "$INSTALL_DIR/warppool" "$BACKUP_DIR/warppool"
  fi
  if [ -d "$LIB_DIR/assets" ]; then
    mkdir -p "$BACKUP_DIR/assets"
    cp -R "$LIB_DIR/assets/." "$BACKUP_DIR/assets/"
  fi
  if [ -f "$CONFIG_PATH" ]; then
    cp "$CONFIG_PATH" "$BACKUP_DIR/config.json"
  fi
  log "backup created: $BACKUP_DIR"
}

download_and_verify() {
  WORK_DIR="$(mktemp -d)"
  ARCHIVE="$WORK_DIR/warppool.tar.gz"
  CHECKSUMS="$WORK_DIR/checksums.txt"
  ASSET_NAME="warppool-linux-${ARCH}.tar.gz"

  log "downloading $(release_url)"
  if ! fetch "$(release_url)" "$ARCHIVE"; then
    log "latest download failed; trying fallback version URL"
    fetch "$(fallback_download_url)" "$ARCHIVE"
  fi
  if fetch "$(checksum_url)" "$CHECKSUMS"; then
    if command -v sha256sum >/dev/null 2>&1; then
      expected=""
      actual=""
      expected="$(awk -v asset="$ASSET_NAME" '$2 ~ asset "$" {print $1; exit}' "$CHECKSUMS")"
      [ -n "$expected" ] || fail "checksum for $ASSET_NAME not found"
      actual="$(sha256sum "$ARCHIVE" | awk '{print $1}')"
      [ "$actual" = "$expected" ] || fail "checksum mismatch for $ASSET_NAME"
    else
      log "warning: sha256sum not found, checksum verification skipped"
    fi
  else
    log "warning: checksums.txt not available, checksum verification skipped"
  fi

  tar -xzf "$ARCHIVE" -C "$WORK_DIR"
  NEW_BINARY="$(find "$WORK_DIR" -type f -name warppool | head -n 1 || true)"
  NEW_ASSETS="$(find "$WORK_DIR" -type d -name assets | head -n 1 || true)"
  [ -n "$NEW_BINARY" ] || fail "warppool binary not found in release package"
  [ -n "$NEW_ASSETS" ] || fail "assets directory not found in release package"
}

install_new_version() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install binary to $INSTALL_DIR/warppool"
    log "dry-run: install assets to $LIB_DIR/assets"
    return 0
  fi
  mkdir -p "$INSTALL_DIR" "$LIB_DIR/assets"

  local tmp_binary tmp_assets old_assets
  tmp_binary="$(mktemp "$INSTALL_DIR/.warppool.new.XXXXXX")"
  cp "$NEW_BINARY" "$tmp_binary"
  chmod 0755 "$tmp_binary"
  mv -f "$tmp_binary" "$INSTALL_DIR/warppool"

  tmp_assets="$(mktemp -d "$LIB_DIR/assets.new.XXXXXX")"
  cp -R "$NEW_ASSETS/." "$tmp_assets/"
  old_assets="$LIB_DIR/assets.old.$$"
  if [ -d "$LIB_DIR/assets" ]; then
    mv "$LIB_DIR/assets" "$old_assets"
  fi
  mv "$tmp_assets" "$LIB_DIR/assets"
  rm -rf -- "$old_assets"

  log "installed binary: $INSTALL_DIR/warppool"
  log "installed assets: $LIB_DIR/assets"
}

write_version_marker() {
  if [ "$DRY_RUN" = "true" ]; then
    return 0
  fi
  local value="$VERSION_ARG"
  if [ "$value" = "latest" ]; then
    value="$("$INSTALL_DIR/warppool" version 2>/dev/null | awk '/^version:/ {print $2; exit}' || true)"
    [ -n "$value" ] || value="latest"
  fi
  mkdir -p "$LIB_DIR"
  printf '%s\n' "$value" >"$LIB_DIR/VERSION" || true
}

restart_services() {
  if ! command -v systemctl >/dev/null 2>&1; then
    log "systemctl not found, skipping service restart"
    return 0
  fi
  for service in warppool-listen.service warppool-proxy.service; do
    if systemctl list-unit-files "$service" >/dev/null 2>&1; then
      if systemctl is-enabled "$service" >/dev/null 2>&1 || systemctl is-active "$service" >/dev/null 2>&1; then
        log "restarting $service"
        if [ "$DRY_RUN" = "true" ]; then
          log "dry-run: systemctl restart $service"
        else
          systemctl restart "$service" || log "warning: failed to restart $service"
        fi
      fi
    fi
  done
}

refresh_systemd_units() {
  local bin="$INSTALL_DIR/warppool"
  local SINGBOX_BIN=""
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: refresh systemd unit files"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    return 0
  fi
  if [ -x "$bin" ]; then
    "$bin" --config "$CONFIG_PATH" listen service install --warppool-bin "$bin" || log "warning: failed to refresh listen service"
    if [ -x "$LIB_DIR/bin/sing-box" ]; then
      SINGBOX_BIN="$LIB_DIR/bin/sing-box"
    else
      SINGBOX_BIN="$(command -v sing-box || true)"
    fi
    if [ -n "$SINGBOX_BIN" ]; then
      "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" --singbox-bin "$SINGBOX_BIN" || log "warning: failed to refresh proxy service"
    else
      "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" || log "warning: failed to refresh proxy service"
    fi
  fi
}

refresh_singbox() {
  local script="$LIB_DIR/assets/singbox_install.sh"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: refresh sing-box through $script"
    return 0
  fi
  if [ -r "$script" ]; then
    bash "$script" --yes source=default || log "warning: failed to refresh sing-box"
  else
    log "warning: sing-box installer not found after upgrade: $script"
  fi
}

main() {
  parse_args "$@"
  require_root
  command -v tar >/dev/null 2>&1 || fail "tar is required"
  detect_arch
  confirm
  backup_existing
  download_and_verify
  install_new_version
  write_version_marker
  refresh_singbox
  refresh_systemd_units
  restart_services
  log "upgrade completed"
}

main "$@"
