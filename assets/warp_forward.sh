#!/usr/bin/env bash
set -Eeuo pipefail

ACTION="up"
DEVICE=""
CLIENT_ADDR=""
TRANSPARENT_PORT="14000"
WARP_PROXY_HOST="127.0.0.1"
WARP_PROXY_PORT="40000"
SINGBOX_BIN=""
STATE_DIR="/var/lib/warppool/warp-forward"
AUTO_INSTALL_SINGBOX="true"
VERIFY_WARP="true"
DRY_RUN="false"

log() {
  printf '[WarpPool][warp-forward] %s\n' "$*"
}

fail() {
  printf '[WarpPool][warp-forward][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][warp-forward][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool WARP forwarding helper

Usage:
  bash warp_forward.sh action=up|down|status device=<wg-device> client_addr=<client-cidr> [transparent_port=14000] [--dry-run]

This script redirects TCP traffic entering the WireGuard device to a local
sing-box redirect inbound, then sends it to Cloudflare WARP local proxy
127.0.0.1:40000.
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
      action=*) ACTION="${arg#action=}" ;;
      device=*) DEVICE="${arg#device=}" ;;
      client_addr=*) CLIENT_ADDR="${arg#client_addr=}" ;;
      transparent_port=*) TRANSPARENT_PORT="${arg#transparent_port=}" ;;
      warp_proxy_host=*) WARP_PROXY_HOST="${arg#warp_proxy_host=}" ;;
      warp_proxy_port=*) WARP_PROXY_PORT="${arg#warp_proxy_port=}" ;;
      singbox_bin=*) SINGBOX_BIN="${arg#singbox_bin=}" ;;
      state_dir=*) STATE_DIR="${arg#state_dir=}" ;;
      auto_install_singbox=*) AUTO_INSTALL_SINGBOX="${arg#auto_install_singbox=}" ;;
      verify_warp=*) VERIFY_WARP="${arg#verify_warp=}" ;;
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
  if [ "$(id -u)" -ne 0 ]; then
    fail "must run as root"
  fi
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    fail "required command not found: $name"
  fi
}

validate_bool() {
  local name="$1"
  local value="$2"
  case "$value" in
    true|false) ;;
    *) fail "$name must be true or false, got: $value" ;;
  esac
}

validate_args() {
  case "$ACTION" in
    up|down|status) ;;
    *) fail "unsupported action: $ACTION, expected up, down, or status" ;;
  esac

  case "$DEVICE" in
    ""|*[!a-zA-Z0-9_.-]*)
      fail "invalid WireGuard device name: $DEVICE"
      ;;
  esac

  if [ -z "$CLIENT_ADDR" ]; then
    fail "client_addr is required"
  fi
  CLIENT_IP="${CLIENT_ADDR%%/*}"
  case "$CLIENT_IP" in
    *[!0-9.]*|"")
      fail "invalid IPv4 client address: $CLIENT_ADDR"
      ;;
  esac

  case "$TRANSPARENT_PORT" in
    *[!0-9]*|"")
      fail "invalid transparent_port: $TRANSPARENT_PORT"
      ;;
  esac
  if [ "$TRANSPARENT_PORT" -lt 1 ] || [ "$TRANSPARENT_PORT" -gt 65535 ]; then
    fail "transparent_port must be between 1 and 65535: $TRANSPARENT_PORT"
  fi

  case "$WARP_PROXY_PORT" in
    *[!0-9]*|"")
      fail "invalid warp_proxy_port: $WARP_PROXY_PORT"
      ;;
  esac

  validate_bool auto_install_singbox "$AUTO_INSTALL_SINGBOX"
  validate_bool verify_warp "$VERIFY_WARP"

  SAFE_DEVICE="$DEVICE"
  CONFIG_PATH="$STATE_DIR/$SAFE_DEVICE.json"
  PID_PATH="$STATE_DIR/$SAFE_DEVICE.pid"
  LOG_PATH="$STATE_DIR/$SAFE_DEVICE.log"
  UNIT_NAME="warppool-warp-forward-$SAFE_DEVICE.service"
  UNIT_PATH="/etc/systemd/system/$UNIT_NAME"
}

script_dir() {
  cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd
}

resolve_singbox() {
  if [ -n "$SINGBOX_BIN" ]; then
    if [ ! -x "$SINGBOX_BIN" ]; then
      fail "sing-box binary is not executable: $SINGBOX_BIN"
    fi
    return 0
  fi

  if [ -x "/usr/local/lib/warppool/bin/sing-box" ]; then
    SINGBOX_BIN="/usr/local/lib/warppool/bin/sing-box"
    return 0
  fi

  if command -v sing-box >/dev/null 2>&1; then
    SINGBOX_BIN="$(command -v sing-box)"
    return 0
  fi

  if [ "$AUTO_INSTALL_SINGBOX" != "true" ]; then
    if [ "$DRY_RUN" = "true" ]; then
      SINGBOX_BIN="/usr/local/lib/warppool/bin/sing-box"
      log "dry-run: assume sing-box binary: $SINGBOX_BIN"
      return 0
    fi
    fail "sing-box not found; install it or set auto_install_singbox=true"
  fi

  local installer
  installer="$(script_dir)/singbox_install.sh"
  if [ ! -r "$installer" ]; then
    fail "sing-box not found and installer missing: $installer"
  fi

  log "sing-box not found, installing to /usr/local/lib/warppool/bin"
  run bash "$installer" --yes source=default install_dir=/usr/local/lib/warppool/bin
  SINGBOX_BIN="/usr/local/lib/warppool/bin/sing-box"
  if [ "$DRY_RUN" != "true" ] && [ ! -x "$SINGBOX_BIN" ]; then
    fail "sing-box installation did not produce executable: $SINGBOX_BIN"
  fi
}

verify_warp_proxy() {
  if [ "$VERIFY_WARP" != "true" ]; then
    log "skip WARP proxy verification"
    return 0
  fi

  require_command curl
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: verify WARP proxy via socks5h://$WARP_PROXY_HOST:$WARP_PROXY_PORT"
    return 0
  fi

  local trace
  trace="$(curl --max-time 20 --socks5-hostname "$WARP_PROXY_HOST:$WARP_PROXY_PORT" -fsSL https://www.cloudflare.com/cdn-cgi/trace || true)"
  if ! printf '%s\n' "$trace" | grep -q '^warp=on$'; then
    printf '%s\n' "$trace" >&2
    fail "WARP proxy verification failed: expected warp=on from $WARP_PROXY_HOST:$WARP_PROXY_PORT"
  fi
  log "WARP proxy verified: warp=on"
}

write_singbox_config() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: write sing-box config: $CONFIG_PATH"
    return 0
  fi

  mkdir -p "$STATE_DIR"
  cat >"$CONFIG_PATH" <<EOF
{
  "log": {
    "level": "info"
  },
  "inbounds": [
    {
      "type": "redirect",
      "tag": "wg-warp-redirect",
      "listen": "127.0.0.1",
      "listen_port": $TRANSPARENT_PORT
    }
  ],
  "outbounds": [
    {
      "type": "socks",
      "tag": "warp-proxy",
      "server": "$WARP_PROXY_HOST",
      "server_port": $WARP_PROXY_PORT,
      "version": "5"
    }
  ],
  "route": {
    "rules": [
      {
        "inbound": [
          "wg-warp-redirect"
        ],
        "outbound": "warp-proxy"
      }
    ],
    "auto_detect_interface": true
  }
}
EOF
  chmod 0600 "$CONFIG_PATH"
}

systemd_available() {
  command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]
}

start_singbox() {
  if systemd_available; then
    if [ "$DRY_RUN" = "true" ]; then
      log "dry-run: write systemd unit: $UNIT_PATH"
      log "dry-run: systemctl enable --now $UNIT_NAME"
      return 0
    fi

    cat >"$UNIT_PATH" <<EOF
[Unit]
Description=WarpPool WARP forward for $DEVICE
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$SINGBOX_BIN run -c $CONFIG_PATH
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable --now "$UNIT_NAME"
    return 0
  fi

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: start sing-box in background"
    return 0
  fi

  if [ -r "$PID_PATH" ]; then
    local pid
    pid="$(cat "$PID_PATH" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1; then
      log "sing-box already running with pid $pid"
      return 0
    fi
  fi

  nohup "$SINGBOX_BIN" run -c "$CONFIG_PATH" >"$LOG_PATH" 2>&1 &
  echo "$!" >"$PID_PATH"
}

stop_singbox() {
  if systemd_available; then
    run systemctl disable --now "$UNIT_NAME"
    if [ "$DRY_RUN" != "true" ]; then
      rm -f "$UNIT_PATH"
      systemctl daemon-reload
    fi
    return 0
  fi

  if [ -r "$PID_PATH" ]; then
    local pid
    pid="$(cat "$PID_PATH" 2>/dev/null || true)"
    if [ -n "$pid" ]; then
      run kill "$pid"
    fi
  fi
  run rm -f "$PID_PATH"
}

add_iptables_rules() {
  require_command iptables
  run iptables -C INPUT -i "$DEVICE" -p tcp -j ACCEPT 2>/dev/null || run iptables -A INPUT -i "$DEVICE" -p tcp -j ACCEPT
  run iptables -t nat -C PREROUTING -i "$DEVICE" -s "$CLIENT_IP/32" -p tcp -j REDIRECT --to-ports "$TRANSPARENT_PORT" 2>/dev/null || run iptables -t nat -A PREROUTING -i "$DEVICE" -s "$CLIENT_IP/32" -p tcp -j REDIRECT --to-ports "$TRANSPARENT_PORT"
}

delete_iptables_rules() {
  require_command iptables
  while iptables -t nat -C PREROUTING -i "$DEVICE" -s "$CLIENT_IP/32" -p tcp -j REDIRECT --to-ports "$TRANSPARENT_PORT" >/dev/null 2>&1; do
    run iptables -t nat -D PREROUTING -i "$DEVICE" -s "$CLIENT_IP/32" -p tcp -j REDIRECT --to-ports "$TRANSPARENT_PORT"
  done
  while iptables -C INPUT -i "$DEVICE" -p tcp -j ACCEPT >/dev/null 2>&1; do
    run iptables -D INPUT -i "$DEVICE" -p tcp -j ACCEPT
  done
}

status() {
  log "device=$DEVICE client=$CLIENT_IP transparent_port=$TRANSPARENT_PORT warp_proxy=$WARP_PROXY_HOST:$WARP_PROXY_PORT"
  if systemd_available; then
    systemctl is-active "$UNIT_NAME" 2>/dev/null || true
  elif [ -r "$PID_PATH" ]; then
    cat "$PID_PATH"
  fi
  iptables -t nat -S PREROUTING 2>/dev/null | grep -- "$DEVICE" || true
}

main() {
  parse_args "$@"
  validate_args
  require_root

  case "$ACTION" in
    up)
      resolve_singbox
      verify_warp_proxy
      write_singbox_config
      start_singbox
      add_iptables_rules
      status
      log "WARP forwarding enabled for $DEVICE"
      ;;
    down)
      delete_iptables_rules
      stop_singbox
      log "WARP forwarding disabled for $DEVICE"
      ;;
    status)
      status
      ;;
  esac
}

main "$@"
