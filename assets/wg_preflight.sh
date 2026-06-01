#!/usr/bin/env bash
set -Eeuo pipefail

DEVICE=""
SERVER_ADDR=""
CLIENT_ADDR=""
LISTEN_PORT=""
AUTO_FIX="true"
DRY_RUN="false"
KERNEL_ONLY="false"

log() {
  printf '[WarpPool][wg-preflight] %s\n' "$*"
}

fail() {
  printf '[WarpPool][wg-preflight][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][wg-preflight][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool WireGuard preflight

Usage:
  bash wg_preflight.sh device=<name> server_addr=<cidr> client_addr=<cidr> listen_port=<port> [auto_fix=true|false] [--dry-run]
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
      device=*)
        DEVICE="${arg#device=}"
        ;;
      server_addr=*)
        SERVER_ADDR="${arg#server_addr=}"
        ;;
      client_addr=*)
        CLIENT_ADDR="${arg#client_addr=}"
        ;;
      listen_port=*)
        LISTEN_PORT="${arg#listen_port=}"
        ;;
      auto_fix=*)
        AUTO_FIX="${arg#auto_fix=}"
        ;;
      kernel_only=*|kernel-only=*)
        KERNEL_ONLY="${arg#*=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
    esac
  done
}

require_args() {
  if [ "$KERNEL_ONLY" = "true" ]; then
    return 0
  fi
  [ -n "$DEVICE" ] || fail "device is required"
  [ -n "$SERVER_ADDR" ] || fail "server_addr is required"
  [ -n "$CLIENT_ADDR" ] || fail "client_addr is required"
  [ -n "$LISTEN_PORT" ] || fail "listen_port is required"
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: $*"
    return 0
  fi
  "$@"
}

server_ip() {
  printf '%s' "$SERVER_ADDR" | cut -d/ -f1
}

server_prefix() {
  printf '%s' "$SERVER_ADDR" | cut -d/ -f2
}

client_ip() {
  printf '%s' "$CLIENT_ADDR" | cut -d/ -f1
}

cidr_network() {
  local cidr ip prefix a b c d value mask network
  cidr="$1"
  ip="${cidr%/*}"
  prefix="${cidr#*/}"
  IFS=. read -r a b c d <<EOF_IP
$ip
EOF_IP
  value=$(( (a << 24) | (b << 16) | (c << 8) | d ))
  if [ "$prefix" -eq 0 ]; then
    mask=0
  else
    mask=$(( (0xffffffff << (32 - prefix)) & 0xffffffff ))
  fi
  network=$(( value & mask ))
  printf '%d.%d.%d.%d/%d' \
    $(( (network >> 24) & 255 )) \
    $(( (network >> 16) & 255 )) \
    $(( (network >> 8) & 255 )) \
    $(( network & 255 )) \
    "$prefix"
}

conflict_is_warppool_owned() {
  local iface="$1"
  case "$iface" in
    wp*|wpc*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

stop_owned_interface() {
  local iface="$1"
  if [ "$AUTO_FIX" != "true" ]; then
    fail "WireGuard interface conflict: $iface. Stop it manually: wg-quick down $iface"
  fi

  log "stopping old WarpPool WireGuard interface: $iface"
  run wg-quick down "$iface" >/dev/null 2>&1 || true
  run systemctl disable "wg-quick@$iface" >/dev/null 2>&1 || true
}

check_same_device() {
  if ! ip link show "$DEVICE" >/dev/null 2>&1; then
    return 0
  fi

  if wg show "$DEVICE" >/dev/null 2>&1; then
    stop_owned_interface "$DEVICE"
    return 0
  fi

  if conflict_is_warppool_owned "$DEVICE"; then
    if [ "$AUTO_FIX" = "true" ]; then
      log "deleting stale WarpPool link: $DEVICE"
      run ip link delete "$DEVICE" || true
      return 0
    fi
    fail "stale WarpPool link exists: $DEVICE. Remove it manually: ip link delete $DEVICE"
  fi

  fail "network interface already exists and is not managed by WireGuard: $DEVICE"
}

check_address_conflicts() {
  local target_ip target_client_ip line iface addr
  target_ip="$(server_ip)"
  target_client_ip="$(client_ip)"

  while read -r line; do
    [ -n "$line" ] || continue
    iface="$(printf '%s' "$line" | awk '{print $2}')"
    addr="$(printf '%s' "$line" | awk '{print $4}')"
    iface="${iface%%@*}"
    [ -n "$iface" ] || continue
    [ -n "$addr" ] || continue

    if [ "$iface" = "$DEVICE" ]; then
      continue
    fi

    case "$addr" in
      "$target_ip/"*|"$target_client_ip/"*)
        if conflict_is_warppool_owned "$iface"; then
          stop_owned_interface "$iface"
          continue
        fi
        fail "IP address conflict: interface $iface already uses $addr. Stop or reconfigure it before installing WarpPool."
        ;;
    esac
  done <<EOF_ADDR
$(ip -o -4 addr show 2>/dev/null || true)
EOF_ADDR
}

check_route_conflicts() {
  local target_network line route iface
  target_network="$(cidr_network "$SERVER_ADDR")"

  while read -r line; do
    [ -n "$line" ] || continue
    route="$(printf '%s' "$line" | awk '{print $1}')"
    iface="$(printf '%s' "$line" | awk '{for (i=1;i<=NF;i++) if ($i=="dev") {print $(i+1); exit}}')"
    [ "$route" = "$target_network" ] || continue
    [ -n "$iface" ] || continue
    if [ "$iface" = "$DEVICE" ]; then
      continue
    fi
    if conflict_is_warppool_owned "$iface"; then
      stop_owned_interface "$iface"
      continue
    fi
    fail "route conflict for $target_network: $line. Stop or reconfigure the conflicting interface before installing WarpPool."
  done <<EOF_ROUTE
$(ip route show "$target_network" 2>/dev/null || true)
EOF_ROUTE
}

find_wg_interface_by_port() {
  local iface port
  for iface in $(wg show interfaces 2>/dev/null || true); do
    port="$(wg show "$iface" listen-port 2>/dev/null || true)"
    if [ "$port" = "$LISTEN_PORT" ]; then
      printf '%s\n' "$iface"
      return 0
    fi
  done
  return 1
}

check_port_conflicts() {
  local line owner
  owner="$(find_wg_interface_by_port || true)"
  if [ -n "$owner" ]; then
    if [ "$owner" = "$DEVICE" ] || conflict_is_warppool_owned "$owner"; then
      stop_owned_interface "$owner"
      return 0
    fi
    fail "UDP port conflict: WireGuard interface $owner already listens on $LISTEN_PORT. Stop or reconfigure it before installing WarpPool."
  fi

  if ! command -v ss >/dev/null 2>&1; then
    log "warning: ss command not found, UDP listen port check skipped"
    return 0
  fi

  line="$(ss -H -lunp 2>/dev/null | awk -v port=":$LISTEN_PORT" '$5 ~ port"$" {print; exit}' || true)"
  if [ -z "$line" ]; then
    return 0
  fi
  if wg show "$DEVICE" >/dev/null 2>&1; then
    stop_owned_interface "$DEVICE"
    return 0
  fi
  fail "UDP port conflict: $LISTEN_PORT is already in use: $line"
}

try_wireguard_kernel_probe() {
  local probe="$1"
  local output status
  set +e
  output="$(ip link add "$probe" type wireguard 2>&1)"
  status=$?
  set -e
  if [ "$status" -eq 0 ]; then
    ip link delete "$probe" >/dev/null 2>&1 || true
    return 0
  fi
  printf '%s\n' "$output"
  return "$status"
}

kernel_reboot_hint() {
  local running latest
  running="$(uname -r 2>/dev/null || true)"
  latest="$(ls /boot/vmlinuz-* 2>/dev/null | sed 's#.*/vmlinuz-##' | sort -V | tail -n 1 || true)"
  if [ -n "$running" ] && [ -n "$latest" ] && [ "$running" != "$latest" ]; then
    printf ' A newer kernel appears to be installed (%s), but the current running kernel is %s. Reboot the node and retry deployment.' "$latest" "$running"
  else
    printf ' Install or enable the WireGuard kernel module, or use a kernel/OS image with WireGuard support, then retry deployment.'
  fi
}

check_wireguard_kernel_support() {
  local probe output hint
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: check kernel WireGuard support"
    return 0
  fi

  probe="wpcheck$$"
  if output="$(try_wireguard_kernel_probe "$probe")"; then
    return 0
  fi

  if command -v modprobe >/dev/null 2>&1; then
    modprobe wireguard >/dev/null 2>&1 || true
    if output="$(try_wireguard_kernel_probe "$probe")"; then
      return 0
    fi
  fi

  hint="$(kernel_reboot_hint)"
  fail "current kernel does not support WireGuard interfaces: ip link add type wireguard failed: ${output:-unknown error}.${hint}"
}

fix_firewall_when_safe() {
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
    if ! ufw status 2>/dev/null | grep -Eq "(^| )$LISTEN_PORT/udp( |$)"; then
      if [ "$AUTO_FIX" = "true" ]; then
        log "allowing WireGuard UDP port in ufw: $LISTEN_PORT/udp"
        run ufw allow "$LISTEN_PORT/udp" >/dev/null
      else
        fail "ufw is active and UDP $LISTEN_PORT is not allowed. Run: ufw allow $LISTEN_PORT/udp"
      fi
    fi
  fi

  if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active firewalld >/dev/null 2>&1; then
    if ! firewall-cmd --query-port="$LISTEN_PORT/udp" >/dev/null 2>&1; then
      if [ "$AUTO_FIX" = "true" ]; then
        log "allowing WireGuard UDP port in firewalld: $LISTEN_PORT/udp"
        run firewall-cmd --add-port="$LISTEN_PORT/udp" >/dev/null
        run firewall-cmd --runtime-to-permanent >/dev/null || true
      else
        fail "firewalld is active and UDP $LISTEN_PORT is not allowed. Run: firewall-cmd --add-port=$LISTEN_PORT/udp --permanent && firewall-cmd --reload"
      fi
    fi
  fi

  if command -v iptables >/dev/null 2>&1 && iptables -S INPUT 2>/dev/null | grep -q '^-P INPUT DROP'; then
    if ! iptables -C INPUT -p udp --dport "$LISTEN_PORT" -j ACCEPT >/dev/null 2>&1; then
      if [ "$AUTO_FIX" = "true" ]; then
        log "allowing WireGuard UDP port in iptables INPUT: $LISTEN_PORT/udp"
        run iptables -I INPUT -p udp --dport "$LISTEN_PORT" -j ACCEPT
        log "warning: iptables runtime rule may not persist after reboot unless your system persists iptables rules"
      else
        fail "iptables INPUT policy is DROP and UDP $LISTEN_PORT is not allowed. Run: iptables -I INPUT -p udp --dport $LISTEN_PORT -j ACCEPT"
      fi
    fi
  fi
}

main() {
  parse_args "$@"
  require_args

  command -v ip >/dev/null 2>&1 || fail "ip command not found"
  command -v wg >/dev/null 2>&1 || fail "wg command not found"
  command -v wg-quick >/dev/null 2>&1 || fail "wg-quick command not found"

  if [ "$KERNEL_ONLY" = "true" ]; then
    log "checking kernel WireGuard support"
    check_wireguard_kernel_support
    log "kernel WireGuard support passed"
    return 0
  fi

  log "checking device=$DEVICE server_addr=$SERVER_ADDR listen_port=$LISTEN_PORT"
  check_wireguard_kernel_support
  check_same_device
  check_address_conflicts
  check_route_conflicts
  check_port_conflicts
  fix_firewall_when_safe
  log "preflight passed"
}

main "$@"
