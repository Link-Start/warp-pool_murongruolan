#!/usr/bin/env bash
set -Eeuo pipefail

MODE="direct"
TOKEN=""
SERVER=""
DRY_RUN="false"

log() {
  printf '[WarpPool][alpine] %s\n' "$*"
}

fail() {
  printf '[WarpPool][alpine][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][alpine][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
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
  run apk update
  run apk add wireguard-tools iproute2 iptables curl ca-certificates
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

  fail "Cloudflare official WARP client does not provide first-class Alpine packages; use direct mode or a supported OS"
}

register_node_placeholder() {
  if [ -n "$TOKEN" ] && [ -n "$SERVER" ]; then
    log "deploy-token registration requested; registration implementation will be added in listen service stage"
  fi
}

main() {
  parse_args "$@"
  install_packages
  configure_wireguard_placeholder
  maybe_install_warp
  register_node_placeholder
  log "Alpine installation completed"
}

main "$@"
