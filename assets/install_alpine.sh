#!/usr/bin/env bash
set -Eeuo pipefail

MODE="direct"
TOKEN=""
SERVER=""
ENDPOINT=""
WG_LISTEN_PORT="51820"
WG_ENDPOINT_PORT=""
DRY_RUN="false"
NODE_EXIT_MODE=""
SERVER_URL_FOR_STATE=""
NODE_NAME=""
LANGUAGE="${WARPPOOL_LANG:-${WARPOOL_LANG:-en}}"

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
      endpoint=*) ENDPOINT="${arg#endpoint=}" ;;
      wg_listen_port=*) WG_LISTEN_PORT="${arg#wg_listen_port=}" ;;
      wg_endpoint_port=*) WG_ENDPOINT_PORT="${arg#wg_endpoint_port=}" ;;
      lang=*|language=*) LANGUAGE="${arg#*=}" ;;
      *) fail "unknown argument: $arg" ;;
    esac
  done
}

normalize_language() {
  case "$1" in
    zh|zh_CN|zh-CN|cn|CN|1)
      printf 'zh\n'
      ;;
    en|en_US|en-US|english|English|2|"")
      printf 'en\n'
      ;;
    *)
      printf 'en\n'
      ;;
  esac
}

install_packages() {
  log "installing WireGuard and base tools"
  run apk update
  run apk add wireguard-tools iproute2 iptables curl ca-certificates coreutils
}

validate_wireguard_ports() {
  case "$WG_LISTEN_PORT" in
    ""|*[!0-9]*) fail "invalid wg_listen_port: $WG_LISTEN_PORT" ;;
  esac
  if [ -n "$WG_ENDPOINT_PORT" ]; then
    case "$WG_ENDPOINT_PORT" in
      *[!0-9]*) fail "invalid wg_endpoint_port: $WG_ENDPOINT_PORT" ;;
    esac
  fi
}

log_wireguard_ready() {
  log "WireGuard package installed; config generation will be handled by WarpPool deploy flow"
}

install_node_uninstaller() {
  local dir script
  dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
  script="$dir/node_uninstall.sh"
  if [ ! -r "$script" ]; then
    log "warning: node uninstaller not found: $script"
    return 0
  fi
  run cp "$script" /usr/local/bin/warppool-node-uninstall
  run chmod 0755 /usr/local/bin/warppool-node-uninstall
  log "installed node uninstaller: /usr/local/bin/warppool-node-uninstall"
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

detect_endpoint() {
  if [ -n "$ENDPOINT" ]; then
    return 0
  fi
  ENDPOINT="$(curl --max-time 10 -fsSL https://api.ipify.org || true)"
  if [ -z "$ENDPOINT" ]; then
    ENDPOINT="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  fi
  [ -n "$ENDPOINT" ] || fail "cannot detect public endpoint; rerun with endpoint=<ip>"
}

generate_wg_keypair() {
  SERVER_PRIVATE_KEY="$(wg genkey)"
  SERVER_PUBLIC_KEY="$(printf '%s' "$SERVER_PRIVATE_KEY" | wg pubkey)"
}

prepare_endpoint_port() {
  if [ -z "$WG_ENDPOINT_PORT" ]; then
    WG_ENDPOINT_PORT="$WG_LISTEN_PORT"
  fi
}

decode_b64() {
  if base64 --help 2>&1 | grep -q -- '-d'; then
    base64 -d
    return $?
  fi
  base64 -D
}

load_prepare_response() {
  local response="$1"
  OK=""
  MESSAGE_B64=""
  WG_DEVICE_B64=""
  WG_SERVER_ADDR_B64=""
  WG_CLIENT_ADDR_B64=""
  NODE_EXIT_MODE_B64=""
  SERVER_CONFIG_B64=""
  eval "$response"
  if [ "$OK" != "1" ]; then
    fail "register prepare failed: $(printf '%s' "$MESSAGE_B64" | decode_b64)"
  fi
  SERVER_CONFIG="$(printf '%s' "$SERVER_CONFIG_B64" | decode_b64)"
  WG_DEVICE="$(printf '%s' "$WG_DEVICE_B64" | decode_b64)"
  WG_SERVER_ADDR="$(printf '%s' "$WG_SERVER_ADDR_B64" | decode_b64)"
  WG_CLIENT_ADDR="$(printf '%s' "$WG_CLIENT_ADDR_B64" | decode_b64)"
  NODE_EXIT_MODE="$(printf '%s' "$NODE_EXIT_MODE_B64" | decode_b64)"
  [ -z "$NODE_EXIT_MODE" ] || MODE="$NODE_EXIT_MODE"
}

fetch_registration_info() {
  if [ -z "$TOKEN" ] && [ -z "$SERVER" ]; then
    return 0
  fi

  if [ -z "$TOKEN" ] || [ -z "$SERVER" ]; then
    fail "token and server must be provided together"
  fi

  log "fetching node settings from WarpPool server"
  local response
  response="$(curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\"}" \
    "$SERVER/register/info?format=sh")" || fail "register info failed"
  load_prepare_response "$response"
  [ -n "$NODE_EXIT_MODE" ] || fail "register info response missing node exit mode"
  log "main server selected mode: $MODE"
}

write_remote_config() {
  local response="$1"
  load_prepare_response "$response"
  [ -n "$SERVER_CONFIG" ] || fail "register prepare response missing server_config"
  [ -n "$WG_DEVICE" ] || fail "register prepare response missing node.wg_device"

  if [ "$MODE" = "direct" ]; then
    SERVER_CONFIG="$(append_direct_forwarding_hooks "$SERVER_CONFIG")"
  fi

  run mkdir -p /etc/wireguard
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: write /etc/wireguard/$WG_DEVICE.conf"
  else
    printf '%s\n' "$SERVER_CONFIG" >/etc/wireguard/"$WG_DEVICE".conf
    chmod 0600 /etc/wireguard/"$WG_DEVICE".conf
  fi
}

extract_node_name() {
  local response="$1"
  NODE_NAME_B64=""
  eval "$response"
  NODE_NAME="$(printf '%s' "$NODE_NAME_B64" | decode_b64)"
}

write_node_state() {
  if [ -z "$SERVER" ] || [ -z "$WG_DEVICE" ]; then
    return 0
  fi
  SERVER_URL_FOR_STATE="$SERVER"
  LANGUAGE="$(normalize_language "$LANGUAGE")"
  run mkdir -p /etc/warppool-node
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: write /etc/warppool-node/state.json"
    return 0
  fi
  cat >/etc/warppool-node/state.json <<EOF
{
  "server_url": "$SERVER_URL_FOR_STATE",
  "node_name": "$NODE_NAME",
  "wg_device": "$WG_DEVICE",
  "wg_server_address": "$WG_SERVER_ADDR",
  "wg_client_address": "$WG_CLIENT_ADDR",
  "last_mode": "$MODE",
  "language": "$LANGUAGE"
}
EOF
  chmod 0600 /etc/warppool-node/state.json
  log "saved node state: /etc/warppool-node/state.json"
}

detect_egress_interface() {
  ip route show default 0.0.0.0/0 | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i=="dev") {print $(i+1); exit}}'
}

append_direct_forwarding_hooks() {
  local config_text="$1"
  local egress client_ip post_up post_down
  egress="$(detect_egress_interface)"
  [ -n "$egress" ] || fail "cannot detect default egress interface"
  client_ip="${WG_CLIENT_ADDR%%/*}"
  post_up="PostUp = sysctl -w net.ipv4.ip_forward=1; iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %i -j ACCEPT; iptables -C FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -C POSTROUTING -s $client_ip/32 -o $egress -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s $client_ip/32 -o $egress -j MASQUERADE"
  post_down="PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true; iptables -t nat -D POSTROUTING -s $client_ip/32 -o $egress -j MASQUERADE 2>/dev/null || true"
  printf '%s\n' "$config_text" | awk -v post_up="$post_up" -v post_down="$post_down" '
    /^\[Peer\][[:space:]]*$/ && !inserted {
      print post_up
      print post_down
      inserted = 1
    }
    { print }
    END {
      if (!inserted) {
        print post_up
        print post_down
      }
    }
  '
}

start_remote_wireguard() {
  local dir
  dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
  if [ -r "$dir/wg_preflight.sh" ]; then
    run bash "$dir/wg_preflight.sh" "device=$WG_DEVICE" "server_addr=$WG_SERVER_ADDR" "client_addr=$WG_CLIENT_ADDR" "listen_port=$WG_LISTEN_PORT" auto_fix=true
  fi
  run wg-quick down "$WG_DEVICE" >/dev/null 2>&1 || true
  run wg-quick up "$WG_DEVICE"
}

enable_direct_forwarding() {
  if [ "$MODE" != "direct" ]; then
    return 0
  fi
  local egress client_ip
  egress="$(detect_egress_interface)"
  [ -n "$egress" ] || fail "cannot detect default egress interface"
  client_ip="${WG_CLIENT_ADDR%%/*}"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: enable IPv4 forwarding and MASQUERADE via $egress"
    return 0
  fi
  sysctl -w net.ipv4.ip_forward=1
  iptables -C FORWARD -i "$WG_DEVICE" -j ACCEPT 2>/dev/null || iptables -A FORWARD -i "$WG_DEVICE" -j ACCEPT
  iptables -C FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT
  iptables -t nat -C POSTROUTING -s "$client_ip/32" -o "$egress" -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s "$client_ip/32" -o "$egress" -j MASQUERADE
}

register_node() {
  if [ -z "$TOKEN" ] && [ -z "$SERVER" ]; then
    return 0
  fi

  if [ -z "$TOKEN" ] || [ -z "$SERVER" ]; then
    fail "token and server must be provided together"
  fi

  detect_endpoint
  prepare_endpoint_port
  generate_wg_keypair
  log "preparing node with WarpPool server"
  local response
  response="$(curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\",\"endpoint\":\"$ENDPOINT\",\"endpoint_port\":$WG_ENDPOINT_PORT,\"server_private_key\":\"$SERVER_PRIVATE_KEY\",\"server_public_key\":\"$SERVER_PUBLIC_KEY\",\"listen_port\":$WG_LISTEN_PORT}" \
    "$SERVER/register/prepare?format=sh")" || fail "register prepare failed"
  extract_node_name "$response"
  write_remote_config "$response"
}

main() {
  parse_args "$@"
  validate_wireguard_ports
  install_packages
  log_wireguard_ready
  install_node_uninstaller
  fetch_registration_info
  maybe_install_warp
  register_node
  if [ -n "$TOKEN" ] && [ -n "$SERVER" ]; then
    start_remote_wireguard
    enable_direct_forwarding
    write_node_state
    log "completing node registration"
    run curl -fsS \
      -X POST \
      -H 'Content-Type: application/json' \
      -d "{\"token\":\"$TOKEN\"}" \
      "$SERVER/register/complete" >/dev/null
    log "node auto registration completed; local proxy service should start automatically on the main server"
  fi
  if [ -z "$TOKEN" ] && [ -z "$SERVER" ]; then
    log "node dependencies installed only; no main server registration was performed"
    log "to auto-register later, run 'warppool deploy-token' on the main server and execute the generated command on this node"
  fi
  log "Alpine installation completed"
}

main "$@"
