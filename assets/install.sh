#!/usr/bin/env bash
set -Eeuo pipefail

MODE="direct"
MODE_SET="false"
DRY_RUN="false"
TOKEN=""
SERVER=""
SERVER_HOST=""
SERVER_PORT="8080"
SERVER_PORT_SET="false"
ENDPOINT=""
WG_LISTEN_PORT="51820"
WG_ENDPOINT_PORT=""
BASE_URL="${WARPOOL_INSTALL_BASE_URL:-https://raw.githubusercontent.com/murongruolan/warp-pool/developer/assets}"
DOWNLOAD_DIR=""

log() {
  printf '[WarpPool] %s\n' "$*"
}

fail() {
  printf '[WarpPool][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool node installer

Usage:
  bash install.sh [mode=direct|warp] [token=xxx] [server=http://host:port] [server_host=host] [server_port=8080] [endpoint=host] [wg_listen_port=51820] [wg_endpoint_port=51820] [base_url=https://...] [--dry-run]

Examples:
  bash install.sh
  bash install.sh mode=warp
  bash install.sh token=xxxxx server=http://1.2.3.4:18080
  bash install.sh token=xxxxx server=http://1.2.3.4:8080 endpoint=5.6.7.8 wg_listen_port=51820 wg_endpoint_port=30021
  bash install.sh base_url=https://example.com/assets
  bash install.sh --dry-run mode=direct
USAGE
}

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
      --help|-h)
        usage
        exit 0
        ;;
      --dry-run)
        DRY_RUN="true"
        ;;
      mode=*)
        MODE="${arg#mode=}"
        MODE_SET="true"
        ;;
      token=*)
        TOKEN="${arg#token=}"
        ;;
      server=*)
        SERVER="${arg#server=}"
        ;;
      server_host=*)
        SERVER_HOST="${arg#server_host=}"
        ;;
      server_port=*)
        SERVER_PORT="${arg#server_port=}"
        SERVER_PORT_SET="true"
        ;;
      endpoint=*)
        ENDPOINT="${arg#endpoint=}"
        ;;
      wg_listen_port=*)
        WG_LISTEN_PORT="${arg#wg_listen_port=}"
        ;;
      wg_endpoint_port=*)
        WG_ENDPOINT_PORT="${arg#wg_endpoint_port=}"
        ;;
      base_url=*)
        BASE_URL="${arg#base_url=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
    esac
  done
}

validate_port_value() {
  local port="$1"
  case "$port" in
    ""|*[!0-9]*)
      return 1
      ;;
  esac
  [ "$port" -ge 1 ] && [ "$port" -le 65535 ]
}

is_ipv4_literal() {
  local value="$1"
  case "$value" in
    *[!0-9.]*|"")
      return 1
      ;;
  esac
  return 0
}

default_registration_port_for_host() {
  if is_ipv4_literal "$1"; then
    printf '8080\n'
    return 0
  fi
  printf '80\n'
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail "installer must run as root"
  fi
}

validate_mode() {
  case "$MODE" in
    direct|warp)
      ;;
    *)
      fail "unsupported mode: $MODE, expected direct or warp"
      ;;
  esac
}

is_interactive() {
  [ -r /dev/tty ] && [ -w /dev/tty ] && ( : </dev/tty >/dev/tty ) 2>/dev/null
}

choose_mode() {
  if [ "$MODE_SET" = "true" ]; then
    return 0
  fi
  if ! is_interactive; then
    return 0
  fi

  local choice
  while true; do
    printf 'WarpPool node exit mode:\n' >/dev/tty
    printf '  1. direct\n' >/dev/tty
    printf '  2. warp\n' >/dev/tty
    printf 'Select [1]: ' >/dev/tty
    read -r choice </dev/tty
    case "$choice" in
      ""|1)
        MODE="direct"
        return 0
        ;;
      2)
        MODE="warp"
        return 0
        ;;
      *)
        printf '[WarpPool] invalid selection\n' >/dev/tty
        ;;
    esac
  done
}

choose_registration_server() {
  if [ -n "$SERVER" ] || [ -n "$SERVER_HOST" ]; then
    if [ -z "$SERVER" ]; then
      validate_port_value "$SERVER_PORT" || fail "invalid server port: $SERVER_PORT"
      SERVER="http://$SERVER_HOST:$SERVER_PORT"
    fi
    return 0
  fi
  if ! is_interactive; then
    return 0
  fi

  local input port
  printf 'Main server IP/domain for auto registration (Enter to skip): ' >/dev/tty
  read -r input </dev/tty
  if [ -z "$input" ]; then
    log "main server address skipped; this run will only install node dependencies"
    return 0
  fi
  SERVER_HOST="$input"
  if [ "$SERVER_PORT_SET" != "true" ]; then
    SERVER_PORT="$(default_registration_port_for_host "$SERVER_HOST")"
  fi

  while true; do
    printf 'Main server registration port [%s]: ' "$SERVER_PORT" >/dev/tty
    read -r port </dev/tty
    port="${port:-$SERVER_PORT}"
    if validate_port_value "$port"; then
      SERVER_PORT="$port"
      SERVER="http://$SERVER_HOST:$SERVER_PORT"
      break
    fi
    printf '[WarpPool] invalid port, enter a number between 1 and 65535\n' >/dev/tty
  done

  if [ -z "$TOKEN" ]; then
    printf 'Deploy Token for auto registration (Enter to skip auto registration): ' >/dev/tty
    read -r TOKEN </dev/tty
    if [ -z "$TOKEN" ]; then
      log "no Deploy Token provided; this run will only install node dependencies"
      SERVER=""
    fi
  fi
}

validate_registration_args() {
  if [ -n "$SERVER" ] && [ -z "$TOKEN" ]; then
    fail "server was provided but token is missing; run warppool deploy-token on the main server, or leave server IP empty for manual setup"
  fi
  if [ -n "$TOKEN" ] && [ -z "$SERVER" ]; then
    fail "server is required when token is provided"
  fi
}

load_os_release() {
  if [ ! -r /etc/os-release ]; then
    fail "/etc/os-release not found, unsupported Linux distribution"
  fi

  # shellcheck disable=SC1091
  . /etc/os-release

  OS_ID="${ID:-}"
  OS_VERSION="${VERSION_ID:-}"
  if [ -z "$OS_ID" ]; then
    fail "cannot detect OS ID from /etc/os-release"
  fi
}

version_major() {
  printf '%s' "$1" | cut -d. -f1
}

check_supported_os() {
  local major
  major="$(version_major "$OS_VERSION")"

  case "$OS_ID" in
    debian)
      if [ "$major" -lt 12 ]; then
        fail "unsupported Debian version: $OS_VERSION, expected Debian 12+"
      fi
      CHILD_SCRIPT="install_debian.sh"
      ;;
    ubuntu)
      if [ "$major" -lt 20 ]; then
        fail "unsupported Ubuntu version: $OS_VERSION, expected Ubuntu 20.04+"
      fi
      CHILD_SCRIPT="install_ubuntu.sh"
      ;;
    alpine)
      local minor
      minor="$(printf '%s' "$OS_VERSION" | cut -d. -f2)"
      minor="${minor:-0}"
      if [ "$major" -lt 3 ] || { [ "$major" -eq 3 ] && [ "$minor" -lt 20 ]; }; then
        fail "unsupported Alpine version: $OS_VERSION, expected Alpine 3.20+"
      fi
      CHILD_SCRIPT="install_alpine.sh"
      ;;
    *)
      fail "unsupported OS: $OS_ID $OS_VERSION"
      ;;
  esac
}

check_arch() {
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64|aarch64|arm64)
      ;;
    *)
      fail "unsupported CPU architecture: $ARCH"
      ;;
  esac
}

check_tun() {
  if [ ! -c /dev/net/tun ]; then
    fail "TUN device is unavailable: /dev/net/tun not found or not a character device"
  fi
}

check_ip_stack() {
  if command -v ip >/dev/null 2>&1; then
    if ! ip -4 addr show scope global | grep -q 'inet '; then
      log "warning: no global IPv4 address detected"
    fi
    if ! ip -6 addr show scope global | grep -q 'inet6 '; then
      log "warning: no global IPv6 address detected"
    fi
    return 0
  fi

  log "warning: command 'ip' not found, IPv4/IPv6 check skipped"
}

check_existing_wireguard_state() {
  if ! command -v wg >/dev/null 2>&1; then
    return 0
  fi

  local interfaces
  interfaces="$(wg show interfaces 2>/dev/null || true)"
  if [ -z "$interfaces" ]; then
    return 0
  fi

  log "warning: existing WireGuard interfaces detected: $interfaces"
  log "warning: WarpPool deploy will run a precise WireGuard preflight before writing its config"
}

script_dir() {
  cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd
}

cleanup_download_dir() {
  if [ -n "$DOWNLOAD_DIR" ] && [ -d "$DOWNLOAD_DIR" ]; then
    rm -rf -- "$DOWNLOAD_DIR"
  fi
}

trap cleanup_download_dir EXIT

download_script() {
  local name="$1"
  local target="$2"

  if ! command -v curl >/dev/null 2>&1; then
    fail "curl is required to download installer script: $name"
  fi

  log "downloading $name from $BASE_URL" >&2
  curl -fsSL "$BASE_URL/$name" -o "$target" || fail "failed to download $name from $BASE_URL"
  chmod 0755 "$target"
}

prepare_child_script() {
  local dir child local_child
  dir="$(script_dir)"
  local_child="$dir/$CHILD_SCRIPT"
  if [ -r "$local_child" ]; then
    printf '%s\n' "$local_child"
    return 0
  fi

  DOWNLOAD_DIR="$(mktemp -d)"
  child="$DOWNLOAD_DIR/$CHILD_SCRIPT"
  download_script "$CHILD_SCRIPT" "$child"
  download_script "warp_install.sh" "$DOWNLOAD_DIR/warp_install.sh"
  download_script "wg_preflight.sh" "$DOWNLOAD_DIR/wg_preflight.sh"
  download_script "warp_forward.sh" "$DOWNLOAD_DIR/warp_forward.sh"
  download_script "singbox_install.sh" "$DOWNLOAD_DIR/singbox_install.sh"
  download_script "node_uninstall.sh" "$DOWNLOAD_DIR/node_uninstall.sh"
  printf '%s\n' "$child"
}

dispatch_child_script() {
  local child
  child="$(prepare_child_script)"
  if [ ! -r "$child" ]; then
    fail "child installer not found: $child"
  fi

  log "dispatching to $CHILD_SCRIPT, mode=$MODE"
  if [ "$DRY_RUN" = "true" ]; then
    bash "$child" --dry-run "mode=$MODE" "token=$TOKEN" "server=$SERVER" "endpoint=$ENDPOINT" "wg_listen_port=$WG_LISTEN_PORT" "wg_endpoint_port=$WG_ENDPOINT_PORT"
    return 0
  fi

  run bash "$child" "mode=$MODE" "token=$TOKEN" "server=$SERVER" "endpoint=$ENDPOINT" "wg_listen_port=$WG_LISTEN_PORT" "wg_endpoint_port=$WG_ENDPOINT_PORT"
}

main() {
  parse_args "$@"
  choose_mode
  choose_registration_server
  validate_mode
  validate_registration_args
  require_root
  load_os_release
  check_supported_os
  check_arch
  check_tun
  check_ip_stack
  check_existing_wireguard_state

  log "detected OS: $OS_ID $OS_VERSION"
  log "detected arch: $ARCH"
  log "selected mode: $MODE"

  dispatch_child_script
  log "installer completed"
}

main "$@"
