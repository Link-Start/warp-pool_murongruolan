#!/usr/bin/env bash
set -Eeuo pipefail

MODE="direct"
TOKEN=""
SERVER=""
ENDPOINT=""
WG_LISTEN_PORT="51820"
WG_ENDPOINT_PORT=""
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
      endpoint=*) ENDPOINT="${arg#endpoint=}" ;;
      wg_listen_port=*) WG_LISTEN_PORT="${arg#wg_listen_port=}" ;;
      wg_endpoint_port=*) WG_ENDPOINT_PORT="${arg#wg_endpoint_port=}" ;;
      *) fail "unknown argument: $arg" ;;
    esac
  done
}

install_packages() {
  log "installing WireGuard and base tools"
  run apk update
  run apk add wireguard-tools iproute2 iptables curl ca-certificates python3
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

json_get_string() {
  local key="$1"
  python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1],""))' "$key"
}

json_get_node_string() {
  local key="$1"
  python3 -c 'import json,sys; print(json.load(sys.stdin).get("node",{}).get(sys.argv[1],""))' "$key"
}

write_remote_config() {
  local response="$1"
  SERVER_CONFIG="$(printf '%s' "$response" | json_get_string server_config)"
  WG_DEVICE="$(printf '%s' "$response" | json_get_node_string wg_device)"
  WG_SERVER_ADDR="$(printf '%s' "$response" | json_get_node_string wg_server_address)"
  WG_CLIENT_ADDR="$(printf '%s' "$response" | json_get_node_string wg_client_address)"
  [ -n "$SERVER_CONFIG" ] || fail "register prepare response missing server_config"
  [ -n "$WG_DEVICE" ] || fail "register prepare response missing node.wg_device"

  run mkdir -p /etc/wireguard
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: write /etc/wireguard/$WG_DEVICE.conf"
  else
    printf '%s\n' "$SERVER_CONFIG" >/etc/wireguard/"$WG_DEVICE".conf
    chmod 0600 /etc/wireguard/"$WG_DEVICE".conf
  fi
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
  egress="$(ip route show default 0.0.0.0/0 | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i=="dev") {print $(i+1); exit}}')"
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

register_node_placeholder() {
  if [ -z "$TOKEN" ] && [ -z "$SERVER" ]; then
    return 0
  fi

  if [ -z "$TOKEN" ] || [ -z "$SERVER" ]; then
    fail "token and server must be provided together"
  fi

  command -v python3 >/dev/null 2>&1 || fail "python3 is required for deploy-token registration"
  detect_endpoint
  prepare_endpoint_port
  generate_wg_keypair
  log "preparing node with WarpPool server"
  local response
  response="$(curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\",\"endpoint\":\"$ENDPOINT\",\"endpoint_port\":$WG_ENDPOINT_PORT,\"server_private_key\":\"$SERVER_PRIVATE_KEY\",\"server_public_key\":\"$SERVER_PUBLIC_KEY\",\"listen_port\":$WG_LISTEN_PORT}" \
    "$SERVER/register/prepare")" || fail "register prepare failed"
  write_remote_config "$response"
  start_remote_wireguard
  enable_direct_forwarding
  log "completing node registration"
  run curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\"}" \
    "$SERVER/register/complete" >/dev/null
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
