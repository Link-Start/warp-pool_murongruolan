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
BASE_URL="${WARPPOOL_INSTALL_BASE_URL:-${WARPOOL_INSTALL_BASE_URL:-https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets}}"
LANGUAGE="${WARPPOOL_LANG:-${WARPOOL_LANG:-}}"
DOWNLOAD_DIR=""

log() {
  printf '[WarpPool] %s\n' "$*"
}

fail() {
  printf '[WarpPool][ERROR] %s\n' "$*" >&2
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
  printf '[WarpPool][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

usage() {
  cat <<'USAGE'
WarpPool node installer

Usage:
  bash install.sh [mode=direct|warp|dual] [token=xxx] [server=http://host:port] [server_host=host] [server_port=8080] [endpoint=host] [wg_listen_port=51820] [wg_endpoint_port=51820] [base_url=https://...] [--dry-run]

Examples:
  bash install.sh
  bash install.sh mode=warp
  bash install.sh token=xxxxx server=http://1.2.3.4:18080
  bash install.sh token=xxxxx server=http://1.2.3.4:8080
  bash install.sh token=xxxxx server=http://1.2.3.4:8080 endpoint=5.6.7.8 wg_endpoint_port=30021
  bash install.sh base_url=https://example.com/assets
  bash install.sh lang=zh
  bash install.sh --dry-run mode=direct
USAGE
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log_i "dry-run: $*" "dry-run：$*"
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
      lang=*|language=*)
        LANGUAGE="${arg#*=}"
        ;;
      *)
        fail_i "unknown argument: $arg" "未知参数：$arg"
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

is_ipv6_literal() {
  local value="$1"
  value="${value#[}"
  value="${value%]}"
  case "$value" in
    *:*)
      return 0
      ;;
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

default_registration_port_for_host() {
  if is_ipv4_literal "$1" || is_ipv6_literal "$1"; then
    printf '8080\n'
    return 0
  fi
  printf '80\n'
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail_i "installer must run as root" "安装脚本必须以 root 权限运行"
  fi
}

validate_mode() {
  case "$MODE" in
    direct|warp|dual)
      ;;
    *)
      fail_i "unsupported mode: $MODE, expected direct, warp, or dual" "不支持的模式：$MODE，应为 direct、warp 或 dual"
      ;;
  esac
}

is_interactive() {
  [ -r /dev/tty ] && [ -w /dev/tty ] && ( : </dev/tty >/dev/tty ) 2>/dev/null
}

normalize_language() {
  case "$1" in
    zh|zh_CN|zh-CN|cn|CN|1)
      printf 'zh\n'
      ;;
    en|en_US|en-US|english|English|2)
      printf 'en\n'
      ;;
    *)
      return 1
      ;;
  esac
}

select_language() {
  local choice normalized
  if [ -n "$LANGUAGE" ]; then
    normalized="$(normalize_language "$LANGUAGE")" || fail "invalid language: $LANGUAGE, expected zh or en / 语言无效：$LANGUAGE，应为 zh 或 en"
    LANGUAGE="$normalized"
    return 0
  fi
  if ! is_interactive; then
    LANGUAGE="en"
    return 0
  fi

  while true; do
    printf '请选择语言 / Please select language:\n' >/dev/tty
    printf '  1. 简体中文\n' >/dev/tty
    printf '  2. English\n' >/dev/tty
    printf '选择 / Select [1]: ' >/dev/tty
    read -r choice </dev/tty
    case "$choice" in
      ""|1)
        LANGUAGE="zh"
        return 0
        ;;
      2)
        LANGUAGE="en"
        return 0
        ;;
      *)
        printf '无效选择 / Invalid selection\n' >/dev/tty
        ;;
    esac
  done
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
    if [ "$LANGUAGE" = "zh" ]; then
      printf 'WarpPool 节点出口模式：\n' >/dev/tty
    else
      printf 'WarpPool node exit mode:\n' >/dev/tty
    fi
    printf '  1. direct\n' >/dev/tty
    printf '  2. warp\n' >/dev/tty
    printf '  3. dual/direct+warp\n' >/dev/tty
    printf '%s' "$(text "Select [1]: " "选择 [1]: ")" >/dev/tty
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
      3)
        MODE="dual"
        return 0
        ;;
      *)
        printf '[WarpPool] %s\n' "$(text "invalid selection" "无效选择")" >/dev/tty
        ;;
    esac
  done
}

choose_registration_server() {
  if [ -n "$SERVER" ] || [ -n "$SERVER_HOST" ]; then
    if [ -z "$SERVER" ]; then
      validate_port_value "$SERVER_PORT" || fail "invalid server port: $SERVER_PORT"
      SERVER="http://$(format_http_host "$SERVER_HOST"):$SERVER_PORT"
    fi
    return 0
  fi
  if ! is_interactive; then
    return 0
  fi

  local input port
  log_i "recommended Pull flow: run 'warppool deploy-token' on the main server first, then execute the generated command on this node" "推荐的 Pull 流程：先在主服务器执行 'warppool deploy-token'，再把生成的一行命令复制到本节点执行"
  log_i "the generated command already contains server address and token; this interactive path is only for manual or dependency-only setup" "生成的命令已经包含主服务器地址和 token；当前交互流程只用于手动注册或仅安装节点依赖"
  printf '%s' "$(text "Main server IP/domain for auto registration (Enter to skip): " "主服务器 IP/域名，用于自动注册（回车跳过）: ")" >/dev/tty
  read -r input </dev/tty
  if [ -z "$input" ]; then
    log_i "main server address skipped; this run will only install node dependencies" "已跳过主服务器地址；本次只安装节点依赖"
    return 0
  fi
  SERVER_HOST="$input"
  if [ "$SERVER_PORT_SET" != "true" ]; then
    SERVER_PORT="$(default_registration_port_for_host "$SERVER_HOST")"
  fi

  while true; do
    if [ "$LANGUAGE" = "zh" ]; then
      printf '主服务器注册端口 [%s]: ' "$SERVER_PORT" >/dev/tty
    else
      printf 'Main server registration port [%s]: ' "$SERVER_PORT" >/dev/tty
    fi
    read -r port </dev/tty
    port="${port:-$SERVER_PORT}"
    if validate_port_value "$port"; then
      SERVER_PORT="$port"
      SERVER="http://$(format_http_host "$SERVER_HOST"):$SERVER_PORT"
      break
    fi
    printf '[WarpPool] %s\n' "$(text "invalid port, enter a number between 1 and 65535" "端口无效，请输入 1 到 65535 之间的数字")" >/dev/tty
  done

  if [ -z "$TOKEN" ]; then
    printf '%s' "$(text "Deploy Token for auto registration (Enter to skip auto registration): " "Deploy Token，用于自动注册（回车跳过自动注册）: ")" >/dev/tty
    read -r TOKEN </dev/tty
    if [ -z "$TOKEN" ]; then
      log_i "no Deploy Token provided; this run will only install node dependencies" "未填写 Deploy Token；本次只安装节点依赖"
      SERVER=""
    fi
  fi
}

choose_wireguard_ports() {
  if [ -z "$SERVER" ] || [ -z "$TOKEN" ]; then
    return 0
  fi
  if ! is_interactive; then
    return 0
  fi

  local input
  while true; do
    printf '%s' "$(text "WireGuard listen port on this node [$WG_LISTEN_PORT]: " "本节点 WireGuard 监听端口 [$WG_LISTEN_PORT]: ")" >/dev/tty
    read -r input </dev/tty
    input="${input:-$WG_LISTEN_PORT}"
    if validate_port_value "$input"; then
      WG_LISTEN_PORT="$input"
      break
    fi
    printf '[WarpPool] %s\n' "$(text "invalid port, enter a number between 1 and 65535" "端口无效，请输入 1 到 65535 之间的数字")" >/dev/tty
  done

  if [ -z "$ENDPOINT" ]; then
    printf '%s' "$(text "Public WireGuard endpoint host/IP for the main server (Enter to auto-detect): " "主服务器连接本节点的 WireGuard 公网端点 host/IP（回车自动检测）: ")" >/dev/tty
    read -r input </dev/tty
    ENDPOINT="$input"
    if [ -z "$ENDPOINT" ]; then
      log_i "public endpoint host/IP will be auto-detected on this node" "将自动检测本节点公网端点 host/IP"
    fi
  fi

  if [ -z "$WG_ENDPOINT_PORT" ]; then
    WG_ENDPOINT_PORT="$WG_LISTEN_PORT"
  fi
  while true; do
    printf '%s' "$(text "Public WireGuard endpoint port for the main server [$WG_ENDPOINT_PORT]: " "主服务器连接本节点的 WireGuard 公网端口 [$WG_ENDPOINT_PORT]: ")" >/dev/tty
    read -r input </dev/tty
    input="${input:-$WG_ENDPOINT_PORT}"
    if validate_port_value "$input"; then
      WG_ENDPOINT_PORT="$input"
      break
    fi
    printf '[WarpPool] %s\n' "$(text "invalid port, enter a number between 1 and 65535" "端口无效，请输入 1 到 65535 之间的数字")" >/dev/tty
  done
}

validate_registration_args() {
  if [ -n "$SERVER" ] && [ -z "$TOKEN" ]; then
    fail_i "server was provided but token is missing; run warppool deploy-token on the main server, or leave server IP empty for manual setup" "已填写主服务器但缺少 token；请在主服务器执行 warppool deploy-token，或留空主服务器地址改为手动配置"
  fi
  if [ -n "$TOKEN" ] && [ -z "$SERVER" ]; then
    fail_i "server is required when token is provided" "填写 token 时必须同时填写主服务器地址"
  fi
}

load_os_release() {
  if [ ! -r /etc/os-release ]; then
    fail_i "/etc/os-release not found, unsupported Linux distribution" "未找到 /etc/os-release，不支持当前 Linux 发行版"
  fi

  # shellcheck disable=SC1091
  . /etc/os-release

  OS_ID="${ID:-}"
  OS_VERSION="${VERSION_ID:-}"
  if [ -z "$OS_ID" ]; then
    fail_i "cannot detect OS ID from /etc/os-release" "无法从 /etc/os-release 识别系统 ID"
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
      if [ "$major" -lt 11 ]; then
        fail_i "unsupported Debian version: $OS_VERSION, expected Debian 11+" "不支持当前 Debian 版本：$OS_VERSION，需要 Debian 11+"
      fi
      CHILD_SCRIPT="install_debian.sh"
      ;;
    ubuntu)
      if [ "$major" -lt 20 ]; then
        fail_i "unsupported Ubuntu version: $OS_VERSION, expected Ubuntu 20.04+" "不支持当前 Ubuntu 版本：$OS_VERSION，需要 Ubuntu 20.04+"
      fi
      CHILD_SCRIPT="install_ubuntu.sh"
      ;;
    alpine)
      local minor
      minor="$(printf '%s' "$OS_VERSION" | cut -d. -f2)"
      minor="${minor:-0}"
      if [ "$major" -lt 3 ] || { [ "$major" -eq 3 ] && [ "$minor" -lt 20 ]; }; then
        fail_i "unsupported Alpine version: $OS_VERSION, expected Alpine 3.20+" "不支持当前 Alpine 版本：$OS_VERSION，需要 Alpine 3.20+"
      fi
      CHILD_SCRIPT="install_alpine.sh"
      ;;
    *)
      fail_i "unsupported OS: $OS_ID $OS_VERSION" "不支持当前系统：$OS_ID $OS_VERSION"
      ;;
  esac
}

check_arch() {
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64|aarch64|arm64)
      ;;
    *)
      fail_i "unsupported CPU architecture: $ARCH" "不支持当前 CPU 架构：$ARCH"
      ;;
  esac
}

check_tun() {
  if [ ! -c /dev/net/tun ]; then
    fail_i "TUN device is unavailable: /dev/net/tun not found or not a character device" "TUN 设备不可用：未找到 /dev/net/tun 或它不是字符设备"
  fi
}

check_ip_stack() {
  if command -v ip >/dev/null 2>&1; then
    if ! ip -4 addr show scope global | grep -q 'inet '; then
      log_i "warning: no global IPv4 address detected" "警告：未检测到全局 IPv4 地址"
    fi
    if ! ip -6 addr show scope global | grep -q 'inet6 '; then
      log_i "warning: no global IPv6 address detected" "警告：未检测到全局 IPv6 地址"
    fi
    return 0
  fi

  log_i "warning: command 'ip' not found, IPv4/IPv6 check skipped" "警告：未找到 ip 命令，已跳过 IPv4/IPv6 检测"
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

  log_i "warning: existing WireGuard interfaces detected: $interfaces" "警告：检测到已有 WireGuard 接口：$interfaces"
  log_i "warning: WarpPool deploy will run a precise WireGuard preflight before writing its config" "警告：WarpPool 部署时会在写入配置前执行精确的 WireGuard 预检"
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
    fail_i "curl is required to download installer script: $name" "需要 curl 来下载安装脚本：$name"
  fi

  log_i "downloading $name from $BASE_URL" "正在从 $BASE_URL 下载 $name" >&2
  curl -fsSL "$BASE_URL/$name" -o "$target" || fail_i "failed to download $name from $BASE_URL" "从 $BASE_URL 下载 $name 失败"
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
  download_script "warp_wgcf.sh" "$DOWNLOAD_DIR/warp_wgcf.sh"
  download_script "wg_preflight.sh" "$DOWNLOAD_DIR/wg_preflight.sh"
  download_script "warp_forward.sh" "$DOWNLOAD_DIR/warp_forward.sh"
  download_script "singbox_install.sh" "$DOWNLOAD_DIR/singbox_install.sh"
  download_script "node_uninstall.sh" "$DOWNLOAD_DIR/node_uninstall.sh"
  download_script "node_mode.sh" "$DOWNLOAD_DIR/node_mode.sh"
  printf '%s\n' "$child"
}

dispatch_child_script() {
  local child
  child="$(prepare_child_script)"
  if [ ! -r "$child" ]; then
    fail_i "child installer not found: $child" "未找到子安装脚本：$child"
  fi

  if [ -n "$SERVER" ] && [ -n "$TOKEN" ] && [ "$MODE_SET" != "true" ]; then
    log_i "dispatching to $CHILD_SCRIPT, exit mode will be read from the main server Deploy Token" "切换到 $CHILD_SCRIPT，出口模式将从主服务器 Deploy Token 读取"
  else
    log_i "dispatching to $CHILD_SCRIPT, mode=$MODE" "切换到 $CHILD_SCRIPT，模式=$MODE"
  fi
  if [ "$DRY_RUN" = "true" ]; then
    bash "$child" --dry-run "mode=$MODE" "token=$TOKEN" "server=$SERVER" "endpoint=$ENDPOINT" "wg_listen_port=$WG_LISTEN_PORT" "wg_endpoint_port=$WG_ENDPOINT_PORT" "lang=$LANGUAGE"
    return 0
  fi

  run bash "$child" "mode=$MODE" "token=$TOKEN" "server=$SERVER" "endpoint=$ENDPOINT" "wg_listen_port=$WG_LISTEN_PORT" "wg_endpoint_port=$WG_ENDPOINT_PORT" "lang=$LANGUAGE"
}

main() {
  parse_args "$@"
  select_language
  choose_registration_server
  if [ -z "$SERVER" ] || [ -z "$TOKEN" ]; then
    choose_mode
  fi
  choose_wireguard_ports
  validate_mode
  validate_registration_args
  require_root
  load_os_release
  check_supported_os
  check_arch
  check_tun
  check_ip_stack
  check_existing_wireguard_state

  log_i "detected OS: $OS_ID $OS_VERSION" "检测到系统：$OS_ID $OS_VERSION"
  log_i "detected arch: $ARCH" "检测到架构：$ARCH"
  if [ -n "$SERVER" ] && [ -n "$TOKEN" ] && [ "$MODE_SET" != "true" ]; then
    log_i "selected mode: will be fetched from the main server Deploy Token" "已选择模式：将从主服务器 Deploy Token 获取"
  else
    log_i "selected mode: $MODE" "已选择模式：$MODE"
  fi

  dispatch_child_script
  log_i "installer completed" "安装脚本执行完成"
}

main "$@"
