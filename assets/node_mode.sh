#!/usr/bin/env bash
set -Eeuo pipefail

TOKEN=""
SERVER=""
MODE=""
NODE_NAME=""
WG_DEVICE=""
WG_SERVER_ADDR=""
WG_CLIENT_ADDR=""
WG_SERVER_IPV6_ADDR=""
WG_CLIENT_IPV6_ADDR=""
WARP_INSTALL="auto"
REMOVE_WARP="false"
TRANSPARENT_PORT="14000"
BASE_URL="${WARPPOOL_INSTALL_BASE_URL:-${WARPOOL_INSTALL_BASE_URL:-https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets}}"
DRY_RUN="false"
STATE_PATH="/etc/warppool-node/state.json"
DOWNLOAD_DIR=""
LANGUAGE="${WARPPOOL_LANG:-${WARPOOL_LANG:-}}"

log() {
  printf '[WarpPool][node-mode] %s\n' "$*"
}

fail() {
  printf '[WarpPool][node-mode][ERROR] %s\n' "$*" >&2
  exit 1
}

text() {
  if [ "${LANGUAGE:-en}" = "zh" ]; then
    printf '%s' "$2"
  else
    printf '%s' "$1"
  fi
}

log_i() {
  log "$(text "$1" "$2")"
}

fail_i() {
  fail "$(text "$1" "$2")"
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][node-mode][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool node mode switch helper

Usage:
  bash node_mode.sh token=<token> [server=http://host:port] [--dry-run]
  bash node_mode.sh mode=direct|warp node=<name> device=<wg-device> client_addr=<client-cidr> server_addr=<server-cidr>

When token is used, this script reads /etc/warppool-node/state.json for the
main server URL. If state is missing, pass server=http://host:port.
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
      token=*) TOKEN="${arg#token=}" ;;
      server=*) SERVER="${arg#server=}" ;;
      mode=*) MODE="${arg#mode=}" ;;
      node=*) NODE_NAME="${arg#node=}" ;;
      device=*) WG_DEVICE="${arg#device=}" ;;
      client_addr=*) WG_CLIENT_ADDR="${arg#client_addr=}" ;;
      server_addr=*) WG_SERVER_ADDR="${arg#server_addr=}" ;;
      client_ipv6_addr=*) WG_CLIENT_IPV6_ADDR="${arg#client_ipv6_addr=}" ;;
      server_ipv6_addr=*) WG_SERVER_IPV6_ADDR="${arg#server_ipv6_addr=}" ;;
      warp_install=*) WARP_INSTALL="${arg#warp_install=}" ;;
      remove_warp=*) REMOVE_WARP="${arg#remove_warp=}" ;;
      transparent_port=*) TRANSPARENT_PORT="${arg#transparent_port=}" ;;
      base_url=*) BASE_URL="${arg#base_url=}" ;;
      state_path=*) STATE_PATH="${arg#state_path=}" ;;
      lang=*|language=*) LANGUAGE="${arg#*=}" ;;
      *)
        fail "unknown argument: $arg"
        ;;
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
      return 1
      ;;
  esac
}

normalize_current_language() {
  local normalized
  normalized="$(normalize_language "$LANGUAGE" 2>/dev/null || true)"
  if [ -n "$normalized" ]; then
    LANGUAGE="$normalized"
  else
    LANGUAGE="en"
  fi
}

is_ipv6_literal() {
  local value="$1"
  value="${value#[}"
  value="${value%]}"
  case "$value" in
    *:*) return 0 ;;
  esac
  return 1
}

format_http_host() {
  local value="$1"
  value="${value#[}"
  value="${value%]}"
  if is_ipv6_literal "$value"; then
    printf '[%s]\n' "$value"
    return 0
  fi
  printf '%s\n' "$value"
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
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

validate_bool() {
  case "$2" in
    true|false) ;;
    *) fail "$1 must be true or false, got: $2" ;;
  esac
}

validate_args() {
  case "$MODE" in
    direct|warp) ;;
    "") ;;
    *) fail "unsupported mode: $MODE, expected direct or warp" ;;
  esac
  case "$WARP_INSTALL" in
    auto|reuse|reinstall) ;;
    *) fail "unsupported warp_install: $WARP_INSTALL, expected auto, reuse, or reinstall" ;;
  esac
  validate_bool remove_warp "$REMOVE_WARP"
  case "$TRANSPARENT_PORT" in
    ""|*[!0-9]*) fail "invalid transparent_port: $TRANSPARENT_PORT" ;;
  esac
}

decode_b64() {
  if base64 --help 2>&1 | grep -q -- '-d'; then
    base64 -d
    return $?
  fi
  base64 -D
}

json_get() {
  local key="$1"
  local path="$2"
  if [ ! -r "$path" ]; then
    return 1
  fi
  sed -n "s/^[[:space:]]*\"$key\"[[:space:]]*:[[:space:]]*\"\\(.*\\)\"[[:space:]]*,*[[:space:]]*$/\\1/p" "$path" | head -n 1
}

load_state() {
  if [ ! -r "$STATE_PATH" ]; then
    return 0
  fi
  [ -n "$SERVER" ] || SERVER="$(json_get server_url "$STATE_PATH" || true)"
  [ -n "$NODE_NAME" ] || NODE_NAME="$(json_get node_name "$STATE_PATH" || true)"
  [ -n "$WG_DEVICE" ] || WG_DEVICE="$(json_get wg_device "$STATE_PATH" || true)"
  [ -n "$WG_SERVER_ADDR" ] || WG_SERVER_ADDR="$(json_get wg_server_address "$STATE_PATH" || true)"
  [ -n "$WG_CLIENT_ADDR" ] || WG_CLIENT_ADDR="$(json_get wg_client_address "$STATE_PATH" || true)"
  [ -n "$WG_SERVER_IPV6_ADDR" ] || WG_SERVER_IPV6_ADDR="$(json_get wg_server_ipv6_address "$STATE_PATH" || true)"
  [ -n "$WG_CLIENT_IPV6_ADDR" ] || WG_CLIENT_IPV6_ADDR="$(json_get wg_client_ipv6_address "$STATE_PATH" || true)"
  [ -n "$LANGUAGE" ] || LANGUAGE="$(json_get language "$STATE_PATH" || true)"
  normalize_current_language
}

prompt_server_if_needed() {
  if [ -n "$SERVER" ] || [ "$SERVER" = "skip" ]; then
    return 0
  fi
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    fail_i "server URL is missing; pass server=http://<main-server>:<port>" "缺少主服务器地址；请传入 server=http://<主服务器>:<端口>"
  fi
  local host port
  printf '%s' "$(text "Main server IP/domain: " "主服务器 IP/域名: ")" >/dev/tty
  read -r host </dev/tty
  [ -n "$host" ] || fail_i "main server IP/domain is required" "主服务器 IP/域名不能为空"
  port="8080"
  if ! is_ipv6_literal "$host"; then
    case "$host" in
      *[!0-9.]*)
        port="80"
        ;;
    esac
  fi
  printf '%s' "$(text "Main server registration port [$port]: " "主服务器注册端口 [$port]: ")" >/dev/tty
  read -r input </dev/tty
  port="${input:-$port}"
  SERVER="http://$(format_http_host "$host"):$port"
}

load_mode_response() {
  local response="$1"
  OK=""
  MESSAGE_B64=""
  NODE_NAME_B64=""
  TARGET_MODE_B64=""
  WG_DEVICE_B64=""
  WG_SERVER_ADDR_B64=""
  WG_CLIENT_ADDR_B64=""
  WG_SERVER_IPV6_ADDR_B64=""
  WG_CLIENT_IPV6_ADDR_B64=""
  WARP_INSTALL_B64=""
  REMOVE_WARP="0"
  eval "$response"
  if [ "$OK" != "1" ]; then
    fail "mode switch prepare failed: $(printf '%s' "$MESSAGE_B64" | decode_b64)"
  fi
  NODE_NAME="$(printf '%s' "$NODE_NAME_B64" | decode_b64)"
  MODE="$(printf '%s' "$TARGET_MODE_B64" | decode_b64)"
  WG_DEVICE="$(printf '%s' "$WG_DEVICE_B64" | decode_b64)"
  WG_SERVER_ADDR="$(printf '%s' "$WG_SERVER_ADDR_B64" | decode_b64)"
  WG_CLIENT_ADDR="$(printf '%s' "$WG_CLIENT_ADDR_B64" | decode_b64)"
  WG_SERVER_IPV6_ADDR="$(printf '%s' "$WG_SERVER_IPV6_ADDR_B64" | decode_b64)"
  WG_CLIENT_IPV6_ADDR="$(printf '%s' "$WG_CLIENT_IPV6_ADDR_B64" | decode_b64)"
  WARP_INSTALL="$(printf '%s' "$WARP_INSTALL_B64" | decode_b64)"
  [ -n "$WARP_INSTALL" ] || WARP_INSTALL="auto"
  if [ "$REMOVE_WARP" = "1" ]; then
    REMOVE_WARP="true"
  else
    REMOVE_WARP="false"
  fi
}

fetch_mode_task() {
  if [ -z "$TOKEN" ]; then
    return 0
  fi
  [ "$SERVER" != "skip" ] || fail_i "server=skip cannot be used with token mode" "token 模式不能使用 server=skip"
  prompt_server_if_needed
  log_i "fetching mode switch task from WarpPool server" "正在从 WarpPool 主服务器获取切换任务"
  local response
  response="$(curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\"}" \
    "$SERVER/node-mode/prepare?format=sh")" || fail "node-mode prepare failed"
  load_mode_response "$response"
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
  require_command curl
  log_i "downloading $name from $BASE_URL" "正在从 $BASE_URL 下载 $name" >&2
  curl -fsSL "$BASE_URL/$name" -o "$target" || fail_i "failed to download $name from $BASE_URL" "从 $BASE_URL 下载 $name 失败"
  chmod 0755 "$target"
}

helper_path() {
  local name="$1"
  local dir local_path
  dir="$(script_dir)"
  local_path="$dir/$name"
  if [ -r "$local_path" ]; then
    printf '%s\n' "$local_path"
    return 0
  fi
  if [ -z "$DOWNLOAD_DIR" ]; then
    DOWNLOAD_DIR="$(mktemp -d)"
  fi
  download_script "$name" "$DOWNLOAD_DIR/$name"
  printf '%s\n' "$DOWNLOAD_DIR/$name"
}

detect_egress_interface() {
  (ip route show default 0.0.0.0/0 2>/dev/null; ip -6 route show default 2>/dev/null) | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i=="dev") {print $(i+1); exit}}'
}

client_ip() {
  printf '%s' "${WG_CLIENT_ADDR%%/*}"
}

client_ip6() {
  printf '%s' "${WG_CLIENT_IPV6_ADDR%%/*}"
}

has_ipv6_wireguard() {
  [ -n "$WG_CLIENT_IPV6_ADDR" ] && [ -n "$WG_SERVER_IPV6_ADDR" ]
}

remove_direct_forwarding() {
  local egress cip
  egress="$(detect_egress_interface || true)"
  cip="$(client_ip)"
  require_command iptables
  while iptables -C FORWARD -i "$WG_DEVICE" -j ACCEPT >/dev/null 2>&1; do
    run iptables -D FORWARD -i "$WG_DEVICE" -j ACCEPT
  done
  while iptables -C FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT >/dev/null 2>&1; do
    run iptables -D FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT
  done
  if [ -n "$egress" ]; then
    while iptables -t nat -C POSTROUTING -s "$cip/32" -o "$egress" -j MASQUERADE >/dev/null 2>&1; do
      run iptables -t nat -D POSTROUTING -s "$cip/32" -o "$egress" -j MASQUERADE
    done
  fi
  if has_ipv6_wireguard && command -v ip6tables >/dev/null 2>&1; then
    local cip6
    cip6="$(client_ip6)"
    while ip6tables -C FORWARD -i "$WG_DEVICE" -j ACCEPT >/dev/null 2>&1; do
      run ip6tables -D FORWARD -i "$WG_DEVICE" -j ACCEPT
    done
    while ip6tables -C FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT >/dev/null 2>&1; do
      run ip6tables -D FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT
    done
    if [ -n "$egress" ]; then
      while ip6tables -t nat -C POSTROUTING -s "$cip6/128" -o "$egress" -j MASQUERADE >/dev/null 2>&1; do
        run ip6tables -t nat -D POSTROUTING -s "$cip6/128" -o "$egress" -j MASQUERADE
      done
    fi
  fi
}

strip_warppool_post_hooks() {
  local conf="/etc/wireguard/$WG_DEVICE.conf"
  if [ ! -r "$conf" ]; then
    log "warning: WireGuard config not found: $conf"
    return 0
  fi
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: strip WarpPool PostUp/PostDown hooks from $conf"
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  awk '
    /^PostUp = .*WarpPool WARP forwarding enabled/ { next }
    /^PostDown = .*WarpPool WARP forwarding disabled/ { next }
    /^PostUp = .*iptables .*FORWARD .*%i/ { next }
    /^PostDown = .*iptables .*FORWARD .*%i/ { next }
    /^PostUp = .*ip6tables .*FORWARD .*%i/ { next }
    /^PostDown = .*ip6tables .*FORWARD .*%i/ { next }
    { print }
  ' "$conf" >"$tmp"
  cat "$tmp" >"$conf"
  rm -f "$tmp"
  chmod 0600 "$conf"
}

append_direct_hooks() {
  local conf="/etc/wireguard/$WG_DEVICE.conf"
  local egress cip cip6 post_up post_down
  if [ ! -r "$conf" ]; then
    log "warning: WireGuard config not found: $conf"
    return 0
  fi
  egress="$(detect_egress_interface)"
  [ -n "$egress" ] || fail "cannot detect default IPv4/IPv6 egress interface"
  cip="$(client_ip)"
  post_up="PostUp = sysctl -w net.ipv4.ip_forward=1; iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %i -j ACCEPT; iptables -C FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -C POSTROUTING -s $cip/32 -o $egress -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s $cip/32 -o $egress -j MASQUERADE"
  post_down="PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true; iptables -t nat -D POSTROUTING -s $cip/32 -o $egress -j MASQUERADE 2>/dev/null || true"
  if has_ipv6_wireguard; then
    cip6="$(client_ip6)"
    post_up="$post_up; sysctl -w net.ipv6.conf.all.forwarding=1; ip6tables -C FORWARD -i %i -j ACCEPT 2>/dev/null || ip6tables -A FORWARD -i %i -j ACCEPT; ip6tables -C FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || ip6tables -A FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT; ip6tables -t nat -C POSTROUTING -s $cip6/128 -o $egress -j MASQUERADE 2>/dev/null || ip6tables -t nat -A POSTROUTING -s $cip6/128 -o $egress -j MASQUERADE"
    post_down="$post_down; ip6tables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; ip6tables -D FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true; ip6tables -t nat -D POSTROUTING -s $cip6/128 -o $egress -j MASQUERADE 2>/dev/null || true"
  fi
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: append direct PostUp/PostDown hooks to $conf"
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  awk -v post_up="$post_up" -v post_down="$post_down" '
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
  ' "$conf" >"$tmp"
  cat "$tmp" >"$conf"
  rm -f "$tmp"
  chmod 0600 "$conf"
}

enable_direct_forwarding() {
  local egress cip cip6
  egress="$(detect_egress_interface)"
  [ -n "$egress" ] || fail "cannot detect default IPv4/IPv6 egress interface"
  cip="$(client_ip)"
  run mkdir -p /etc/sysctl.d
  if [ "$DRY_RUN" != "true" ]; then
    printf 'net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1\n' >/etc/sysctl.d/99-warppool.conf
  fi
  run sysctl -w net.ipv4.ip_forward=1
  run iptables -C FORWARD -i "$WG_DEVICE" -j ACCEPT 2>/dev/null || run iptables -A FORWARD -i "$WG_DEVICE" -j ACCEPT
  run iptables -C FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || run iptables -A FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT
  run iptables -t nat -C POSTROUTING -s "$cip/32" -o "$egress" -j MASQUERADE 2>/dev/null || run iptables -t nat -A POSTROUTING -s "$cip/32" -o "$egress" -j MASQUERADE
  if has_ipv6_wireguard; then
    cip6="$(client_ip6)"
    run sysctl -w net.ipv6.conf.all.forwarding=1
    run ip6tables -C FORWARD -i "$WG_DEVICE" -j ACCEPT 2>/dev/null || run ip6tables -A FORWARD -i "$WG_DEVICE" -j ACCEPT
    run ip6tables -C FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || run ip6tables -A FORWARD -o "$WG_DEVICE" -m state --state RELATED,ESTABLISHED -j ACCEPT
    run ip6tables -t nat -C POSTROUTING -s "$cip6/128" -o "$egress" -j MASQUERADE 2>/dev/null || run ip6tables -t nat -A POSTROUTING -s "$cip6/128" -o "$egress" -j MASQUERADE
  fi
}

ensure_warp_ready() {
  local installer
  if command -v apk >/dev/null 2>&1 && ! command -v apt-get >/dev/null 2>&1; then
    installer="$(helper_path warp_wgcf.sh)"
    run bash "$installer" "policy=$WARP_INSTALL"
    return 0
  fi
  installer="$(helper_path warp_install.sh)"
  run bash "$installer" "policy=$WARP_INSTALL"
}

enable_warp_forwarding() {
  local forwarder
  helper_path singbox_install.sh >/dev/null
  forwarder="$(helper_path warp_forward.sh)"
  run bash "$forwarder" action=up "device=$WG_DEVICE" "client_addr=$WG_CLIENT_ADDR" "server_addr=$WG_SERVER_ADDR" "transparent_port=$TRANSPARENT_PORT"
}

disable_warp_forwarding() {
  local forwarder
  forwarder="$(helper_path warp_forward.sh)"
  run bash "$forwarder" action=down "device=$WG_DEVICE" "client_addr=$WG_CLIENT_ADDR" "server_addr=$WG_SERVER_ADDR" "transparent_port=$TRANSPARENT_PORT"
}

remove_warp_package() {
  if [ "$REMOVE_WARP" != "true" ]; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    log_i "removing Cloudflare WARP package" "正在卸载 Cloudflare WARP 软件包"
    run env DEBIAN_FRONTEND=noninteractive apt-get remove -y cloudflare-warp
    return 0
  fi
  if command -v apk >/dev/null 2>&1; then
    log_i "removing wgcf WARP state" "正在删除 wgcf WARP 状态"
    run rm -rf /etc/warppool-node/warp
    run rm -f /usr/local/lib/warppool/bin/wgcf
    return 0
  fi
  log_i "warning: automatic WARP removal is not implemented for this system" "警告：当前系统暂不支持自动卸载 WARP"
}

write_state() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: mkdir -p $(dirname "$STATE_PATH")"
    log "dry-run: write $STATE_PATH"
    return 0
  fi
  mkdir -p "$(dirname "$STATE_PATH")"
  cat >"$STATE_PATH" <<EOF
{
  "server_url": "$SERVER",
  "node_name": "$NODE_NAME",
  "wg_device": "$WG_DEVICE",
  "wg_server_address": "$WG_SERVER_ADDR",
  "wg_client_address": "$WG_CLIENT_ADDR",
  "wg_server_ipv6_address": "$WG_SERVER_IPV6_ADDR",
  "wg_client_ipv6_address": "$WG_CLIENT_IPV6_ADDR",
  "last_mode": "$MODE",
  "language": "$LANGUAGE"
}
EOF
  chmod 0600 "$STATE_PATH"
}

complete_task() {
  if [ -z "$TOKEN" ] || [ -z "$SERVER" ] || [ "$SERVER" = "skip" ]; then
    return 0
  fi
  log_i "reporting mode switch completion" "正在上报模式切换完成状态"
  run curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\"}" \
    "$SERVER/node-mode/complete" >/dev/null
}

validate_metadata() {
  [ -n "$MODE" ] || fail "mode is required"
  [ -n "$WG_DEVICE" ] || fail "wg device is required"
  [ -n "$WG_CLIENT_ADDR" ] || fail "wg client address is required"
  [ -n "$WG_SERVER_ADDR" ] || fail "wg server address is required"
}

switch_to_warp() {
  log_i "switching node to warp mode" "正在将节点切换到 warp 模式"
  ensure_warp_ready
  strip_warppool_post_hooks
  remove_direct_forwarding
  enable_warp_forwarding
}

switch_to_direct() {
  log_i "switching node to direct mode" "正在将节点切换到 direct 模式"
  disable_warp_forwarding
  strip_warppool_post_hooks
  append_direct_hooks
  enable_direct_forwarding
  remove_warp_package
}

main() {
  parse_args "$@"
  normalize_current_language
  validate_args
  require_root
  load_state
  fetch_mode_task
  validate_metadata
  case "$MODE" in
    warp)
      switch_to_warp
      ;;
    direct)
      switch_to_direct
      ;;
  esac
  write_state
  complete_task
  log_i "node mode switch completed: $MODE" "节点模式切换完成：$MODE"
}

main "$@"
