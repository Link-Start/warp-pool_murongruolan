#!/usr/bin/env bash
set -Eeuo pipefail

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

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
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
  curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor -o "$keyring"
  echo "deb [signed-by=$keyring] https://pkg.cloudflareclient.com/ $codename main" >"$list"

  apt-get update
  apt-get install -y cloudflare-warp
}

register_and_connect() {
  require_command warp-cli
  require_command curl

  log "registering WARP client"
  warp-cli --accept-tos registration new || fail "warp-cli registration new failed"

  log "setting WARP proxy mode"
  warp-cli --accept-tos mode proxy || fail "warp-cli mode proxy failed"

  log "connecting WARP"
  warp-cli --accept-tos connect || fail "warp-cli connect failed"

  log "verifying WARP proxy"
  local trace
  trace="$(curl --max-time 20 --socks5 127.0.0.1:40000 -fsSL https://www.cloudflare.com/cdn-cgi/trace || true)"
  if ! printf '%s\n' "$trace" | grep -q '^warp=on$'; then
    printf '%s\n' "$trace" >&2
    fail "WARP verification failed: expected warp=on"
  fi
}

main() {
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
