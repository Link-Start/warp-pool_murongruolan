#!/usr/bin/env bash
set -Eeuo pipefail

REPO="${WARPOOL_REPO:-murongruolan/warp-pool}"
VERSION="${WARPOOL_VERSION:-latest}"
INSTALL_DIR="${WARPOOL_INSTALL_DIR:-/usr/local/bin}"
LIB_DIR="${WARPOOL_LIB_DIR:-/usr/local/lib/warppool}"
CONFIG_PATH="${WARPOOL_CONFIG_PATH:-/etc/warppool/config.json}"
LISTEN_HOST="${WARPOOL_LISTEN_HOST:-0.0.0.0}"
DEFAULT_LISTEN_PORT="${WARPOOL_LISTEN_PORT:-8080}"
LISTEN_PORT="${WARPOOL_LISTEN_PORT_VALUE:-}"
PUBLIC_HOST="${WARPOOL_PUBLIC_HOST:-}"
YES="false"
DRY_RUN="false"
WORK_DIR=""

log() {
  printf '[WarpPool][server] %s\n' "$*"
}

fail() {
  printf '[WarpPool][server][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][server][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

cleanup() {
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf -- "$WORK_DIR"
  fi
}

trap 'on_error $LINENO' ERR
trap cleanup EXIT

usage() {
  cat <<'USAGE'
WarpPool main server installer

Usage:
  wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash
  curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash

Options:
  version=latest|v0.1.0
  repo=owner/name
  port=8080
  public_host=1.2.3.4
  install_dir=/usr/local/bin
  config=/etc/warppool/config.json
  --yes
  --dry-run
USAGE
}

parse_args() {
  for arg in "$@"; do
    case "$arg" in
      --help|-h)
        usage
        exit 0
        ;;
      --yes|-y)
        YES="true"
        ;;
      --dry-run)
        DRY_RUN="true"
        ;;
      version=*)
        VERSION="${arg#version=}"
        ;;
      repo=*)
        REPO="${arg#repo=}"
        ;;
      port=*)
        LISTEN_PORT="${arg#port=}"
        ;;
      public_host=*)
        PUBLIC_HOST="${arg#public_host=}"
        ;;
      install_dir=*)
        INSTALL_DIR="${arg#install_dir=}"
        ;;
      config=*)
        CONFIG_PATH="${arg#config=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
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
  if [ "$(id -u)" -ne 0 ]; then
    fail "installer must run as root; use: wget -qO- <url> | sudo bash"
  fi
}

require_linux() {
  if [ "$(uname -s | tr '[:upper:]' '[:lower:]')" != "linux" ]; then
    fail "main server installer only supports Linux"
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
  local major minor
  major="$(version_major "$OS_VERSION")"
  case "$OS_ID" in
    debian)
      if [ "$major" -lt 12 ]; then
        fail "unsupported Debian version: $OS_VERSION, expected Debian 12+"
      fi
      ;;
    ubuntu)
      if [ "$major" -lt 20 ]; then
        fail "unsupported Ubuntu version: $OS_VERSION, expected Ubuntu 20.04+"
      fi
      ;;
    alpine)
      minor="$(printf '%s' "$OS_VERSION" | cut -d. -f2)"
      minor="${minor:-0}"
      if [ "$major" -lt 3 ] || { [ "$major" -eq 3 ] && [ "$minor" -lt 20 ]; }; then
        fail "unsupported Alpine version: $OS_VERSION, expected Alpine 3.20+"
      fi
      ;;
    *)
      fail "unsupported OS: $OS_ID $OS_VERSION"
      ;;
  esac
}

detect_arch() {
  local raw
  raw="$(uname -m)"
  case "$raw" in
    x86_64|amd64)
      ARCH="amd64"
      ;;
    aarch64|arm64)
      ARCH="arm64"
      ;;
    *)
      fail "unsupported CPU architecture: $raw, expected amd64 or arm64"
      ;;
  esac
}

install_base_packages() {
  log "installing base packages"
  case "$OS_ID" in
    debian|ubuntu)
      run env DEBIAN_FRONTEND=noninteractive apt-get update
      run env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl wget tar wireguard wireguard-tools iproute2 iptables systemd
      ;;
    alpine)
      run apk update
      run apk add ca-certificates curl wget tar wireguard-tools iproute2 iptables
      ;;
  esac
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

port_available_with_python() {
  local python_bin="$1"
  local host="$2"
  local port="$3"
  "$python_bin" - "$host" "$port" <<'PY'
import socket
import sys

host = sys.argv[1]
port = int(sys.argv[2])
family = socket.AF_INET6 if ":" in host else socket.AF_INET
sock = socket.socket(family, socket.SOCK_STREAM)
try:
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind((host, port))
except OSError:
    sys.exit(1)
finally:
    sock.close()
PY
}

port_available() {
  local host="$1"
  local port="$2"

  if command -v python3 >/dev/null 2>&1; then
    port_available_with_python python3 "$host" "$port"
    return $?
  fi
  if command -v python >/dev/null 2>&1; then
    port_available_with_python python "$host" "$port"
    return $?
  fi
  if command -v ss >/dev/null 2>&1; then
    ! ss -H -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(:|\\])${port}$"
    return $?
  fi
  if command -v netstat >/dev/null 2>&1; then
    ! netstat -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(:|\\])${port}$"
    return $?
  fi

  log "warning: cannot verify port availability because python, ss, and netstat are unavailable"
  return 0
}

is_interactive() {
  [ "$YES" != "true" ] && [ -r /dev/tty ] && [ -w /dev/tty ]
}

choose_listen_port() {
  local input candidate

  if [ -n "$LISTEN_PORT" ]; then
    if ! validate_port_value "$LISTEN_PORT"; then
      fail "invalid listener port: $LISTEN_PORT"
    fi
    if ! port_available "$LISTEN_HOST" "$LISTEN_PORT"; then
      fail "listener port is already in use: $LISTEN_HOST:$LISTEN_PORT"
    fi
    return 0
  fi

  if ! is_interactive; then
    LISTEN_PORT="$DEFAULT_LISTEN_PORT"
    if ! port_available "$LISTEN_HOST" "$LISTEN_PORT"; then
      fail "default listener port is already in use: $LISTEN_HOST:$LISTEN_PORT; rerun with port=<port>"
    fi
    return 0
  fi

  while true; do
    printf 'WarpPool registration listener port [%s]: ' "$DEFAULT_LISTEN_PORT" >/dev/tty
    read -r input </dev/tty
    candidate="${input:-$DEFAULT_LISTEN_PORT}"

    if ! validate_port_value "$candidate"; then
      printf '[WarpPool][server] invalid port, enter a number between 1 and 65535\n' >/dev/tty
      continue
    fi
    if ! port_available "$LISTEN_HOST" "$candidate"; then
      printf '[WarpPool][server] port %s is already in use, choose another one\n' "$candidate" >/dev/tty
      continue
    fi

    LISTEN_PORT="$candidate"
    return 0
  done
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
  fail "curl or wget is required to download $url"
}

release_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  if [ "$VERSION" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$REPO" "$asset"
    return 0
  fi

  local tag="$VERSION"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$tag" "$asset"
}

download_and_install_warppool() {
  local url archive binary assets_dir
  url="$(release_url)"

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: download WarpPool release from $url"
    log "dry-run: install binary to $INSTALL_DIR/warppool"
    log "dry-run: install assets to $LIB_DIR/assets"
    return 0
  fi

  WORK_DIR="$(mktemp -d)"
  archive="$WORK_DIR/warppool.tar.gz"
  log "downloading WarpPool release: $url"
  fetch "$url" "$archive" || fail "failed to download WarpPool release package; publish a release first or pass version=vX.Y.Z"

  tar -xzf "$archive" -C "$WORK_DIR" || fail "failed to extract WarpPool release package"
  binary="$(find "$WORK_DIR" -type f -name warppool | head -n 1 || true)"
  if [ -z "$binary" ]; then
    fail "warppool binary not found in release package"
  fi

  mkdir -p "$INSTALL_DIR" "$LIB_DIR/assets"
  cp "$binary" "$INSTALL_DIR/warppool" || fail "failed to install warppool binary"
  chmod 0755 "$INSTALL_DIR/warppool"

  assets_dir="$(find "$WORK_DIR" -type d -name assets | head -n 1 || true)"
  if [ -n "$assets_dir" ]; then
    cp -R "$assets_dir/." "$LIB_DIR/assets/"
  else
    log "warning: assets directory not found in release package"
  fi
}

install_singbox() {
  local script="$LIB_DIR/assets/singbox_install.sh"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install sing-box through $script"
    return 0
  fi
  if [ ! -r "$script" ]; then
    log "warning: sing-box installer not found: $script"
    log "warning: install sing-box manually before running warppool proxy start"
    return 0
  fi

  bash "$script" --yes source=default
}

initialize_config() {
  local bin="$INSTALL_DIR/warppool"
  local args
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: initialize config at $CONFIG_PATH if missing"
    log "dry-run: configure listener $LISTEN_HOST:$LISTEN_PORT"
    return 0
  fi

  if [ ! -x "$bin" ]; then
    fail "warppool binary is not executable: $bin"
  fi

  if [ ! -f "$CONFIG_PATH" ]; then
    "$bin" --config "$CONFIG_PATH" config init
  else
    log "config already exists, keeping: $CONFIG_PATH"
  fi

  args=(--config "$CONFIG_PATH" listen config --host "$LISTEN_HOST" --port "$LISTEN_PORT")
  if [ -n "$PUBLIC_HOST" ]; then
    args+=(--public-host "$PUBLIC_HOST")
  fi
  "$bin" "${args[@]}"
}

create_systemd_services() {
  local bin="$INSTALL_DIR/warppool"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: create systemd service for Deploy Token listener"
    log "dry-run: create systemd service for local proxy"
    return 0
  fi
  if [ "$OS_ID" = "alpine" ]; then
    log "warning: systemd service creation skipped on Alpine"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    log "warning: systemctl not found, Deploy Token listener service not created"
    return 0
  fi

  "$bin" --config "$CONFIG_PATH" listen service install --warppool-bin "$bin"
  "$bin" --config "$CONFIG_PATH" listen service enable
  "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" --singbox-bin "$LIB_DIR/bin/sing-box"
}

print_next_steps() {
  cat <<EOF

WarpPool main server installation completed.

Listener config:
  host: $LISTEN_HOST
  port: $LISTEN_PORT
  config: $CONFIG_PATH

Next commands:
  warppool deploy --name nat01 --exit-mode direct --proxy mixed --port 10133 --ssh-host <exit-node-ip> --ssh-user root
  warppool wg up nat01
  warppool proxy service enable

Deploy Token listener is configured but not started automatically.
Start it only when needed:
  warppool listen start

EOF
}

main() {
  parse_args "$@"
  require_root
  require_linux
  load_os_release
  check_supported_os
  detect_arch
  install_base_packages
  choose_listen_port

  log "detected OS: $OS_ID $OS_VERSION"
  log "detected arch: $ARCH"
  log "selected listener: $LISTEN_HOST:$LISTEN_PORT"

  download_and_install_warppool
  install_singbox
  initialize_config
  create_systemd_services
  print_next_steps
}

main "$@"
