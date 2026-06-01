#!/usr/bin/env bash
set -Eeuo pipefail

POLICY="auto"
DRY_RUN="false"

log() {
  printf '[WarpPool][warp] %s\n' "$*"
}

fail() {
  printf '[WarpPool][warp][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][warp][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

parse_args() {
  for arg in "$@"; do
    case "$arg" in
      --dry-run)
        DRY_RUN="true"
        ;;
      policy=*|warp_install=*)
        POLICY="${arg#*=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
    esac
  done
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: $*"
    return 0
  fi
  "$@"
}

warp_installed() {
  command -v warp-cli >/dev/null 2>&1
}

install_cloudflare_repo_debian_like() {
  require_command curl
  require_command gpg

  local keyring="/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg"
  local list="/etc/apt/sources.list.d/cloudflare-client.list"
  local codename

  codename="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME:-}")"
  if [ -z "$codename" ]; then
    fail "cannot detect Debian/Ubuntu codename for Cloudflare WARP repository"
  fi

  log "installing Cloudflare WARP apt repository"
  if [ "$POLICY" = "reinstall" ] && command -v apt-get >/dev/null 2>&1; then
    log "removing existing Cloudflare WARP package before reinstall"
    run env DEBIAN_FRONTEND=noninteractive apt-get remove -y cloudflare-warp || true
  fi

  if warp_installed && [ "$POLICY" != "reinstall" ]; then
    log "Cloudflare WARP is already installed; reusing existing installation"
    return 0
  fi

  if [ "$POLICY" = "reuse" ]; then
    fail "Cloudflare WARP is not installed and warp_install policy is reuse"
  fi

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install Cloudflare WARP apt repository and package"
    return 0
  fi

  curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor -o "$keyring"
  echo "deb [signed-by=$keyring] https://pkg.cloudflareclient.com/ $codename main" >"$list"

  env DEBIAN_FRONTEND=noninteractive apt-get update
  env DEBIAN_FRONTEND=noninteractive apt-get install -y cloudflare-warp
}

register_and_connect() {
  require_command curl
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: ensure warp-cli exists"
    log "dry-run: enable/restart warp-svc.service"
    log "dry-run: configure warp-cli registration, proxy mode, and connection"
    return 0
  fi
  require_command warp-cli

  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now warp-svc.service >/dev/null 2>&1 || systemctl restart warp-svc.service || true
  fi

  local registration=""
  registration="$(warp-cli --accept-tos registration show 2>&1 || true)"
  if printf '%s\n' "$registration" | grep -qi 'Missing registration'; then
    log "registering WARP client"
    warp-cli --accept-tos registration new || fail "warp-cli registration new failed"
  else
    log "WARP client already registered"
  fi

  registration="$(warp-cli --accept-tos registration show 2>&1 || true)"
  if printf '%s\n' "$registration" | grep -qi 'Missing registration'; then
    log "registering WARP client"
    warp-cli --accept-tos registration new || fail "warp-cli registration new failed"
  fi

  log "setting WARP proxy mode"
  warp-cli --accept-tos mode proxy || fail "warp-cli mode proxy failed"

  log "connecting WARP"
  warp-cli --accept-tos connect || fail "warp-cli connect failed"

  log "verifying WARP proxy"
  local trace=""
  local attempt
  for attempt in $(seq 1 30); do
    trace="$(curl --max-time 10 --socks5 127.0.0.1:40000 -fsSL https://www.cloudflare.com/cdn-cgi/trace || true)"
    if printf '%s\n' "$trace" | grep -q '^warp=on$'; then
      log "WARP proxy verified on attempt $attempt"
      return 0
    fi
    sleep 2
  done

  printf '%s\n' "$trace" >&2
  fail "WARP verification failed: expected warp=on via 127.0.0.1:40000"
}

main() {
  parse_args "$@"
  case "$POLICY" in
    auto|reuse|reinstall) ;;
    *) fail "unsupported warp install policy: $POLICY, expected auto, reuse, or reinstall" ;;
  esac

  if [ ! -r /etc/os-release ]; then
    fail "/etc/os-release not found"
  fi

  # shellcheck disable=SC1091
  . /etc/os-release

  case "${ID:-}" in
    debian|ubuntu)
      install_cloudflare_repo_debian_like
      ;;
    *)
      fail "unsupported OS for official Cloudflare WARP client: ${ID:-unknown}"
      ;;
  esac

  register_and_connect
  log "Cloudflare WARP installation completed"
}

main "$@"
