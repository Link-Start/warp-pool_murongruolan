#!/usr/bin/env bash
set -Eeuo pipefail

MODE="direct"
TOKEN=""
SERVER=""
DRY_RUN="false"

log() {
  printf '[WarpPool][ubuntu] %s\n' "$*"
}

fail() {
  printf '[WarpPool][ubuntu][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][ubuntu][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: $*"
    return 0
  fi
  "$@"
}

parse_args() {
  for arg in "$@"; do
    case "$arg" in
      --dry-run) DRY_RUN="true" ;;
      mode=*) MODE="${arg#mode=}" ;;
      token=*) TOKEN="${arg#token=}" ;;
      server=*) SERVER="${arg#server=}" ;;
      *) fail "unknown argument: $arg" ;;
    esac
  done
}

install_packages() {
  log "installing WireGuard and base tools"
  run apt-get update
  run apt-get install -y wireguard wireguard-tools iproute2 iptables curl ca-certificates gnupg
}

configure_wireguard_placeholder() {
  log "WireGuard package installed; config generation will be handled by WarpPool deploy flow"
}

maybe_install_warp() {
  if [ "$MODE" = "direct" ]; then
    log "direct mode selected, skipping Cloudflare WARP installation"
    return 0
  fi

  if [ "$MODE" != "warp" ]; then
    fail "unsupported mode: $MODE"
  fi

  local dir
  dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
  if [ ! -r "$dir/warp_install.sh" ]; then
    fail "WARP installer not found: $dir/warp_install.sh"
  fi

  run bash "$dir/warp_install.sh"
}

register_node_placeholder() {
  if [ -z "$TOKEN" ] && [ -z "$SERVER" ]; then
    return 0
  fi

  if [ -z "$TOKEN" ] || [ -z "$SERVER" ]; then
    fail "token and server must be provided together"
  fi

  log "registering node with WarpPool server"
  run curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\"}" \
    "$SERVER/register"
}

main() {
  parse_args "$@"
  install_packages
  configure_wireguard_placeholder
  maybe_install_warp
  register_node_placeholder
  log "Ubuntu installation completed"
}

main "$@"
