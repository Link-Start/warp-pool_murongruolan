#!/usr/bin/env bash
set -Eeuo pipefail

REPO="${WARPPOOL_REPO:-${WARPOOL_REPO:-murongruolan/warp-pool}}"
RELEASE_VERSION="${WARPPOOL_VERSION:-${WARPOOL_VERSION:-latest}}"
INSTALL_DIR="${WARPPOOL_INSTALL_DIR:-${WARPOOL_INSTALL_DIR:-/usr/local/bin}}"
LIB_DIR="${WARPPOOL_LIB_DIR:-${WARPOOL_LIB_DIR:-/usr/local/lib/warppool}}"
CONFIG_PATH="${WARPPOOL_CONFIG_PATH:-${WARPOOL_CONFIG_PATH:-/etc/warppool/config.json}}"
LISTEN_HOST="${WARPPOOL_LISTEN_HOST:-${WARPOOL_LISTEN_HOST:-0.0.0.0}}"
DEFAULT_LISTEN_PORT="${WARPPOOL_LISTEN_PORT:-${WARPOOL_LISTEN_PORT:-8080}}"
LISTEN_PORT="${WARPPOOL_LISTEN_PORT_VALUE:-${WARPOOL_LISTEN_PORT_VALUE:-}}"
PUBLIC_HOST="${WARPPOOL_PUBLIC_HOST:-${WARPOOL_PUBLIC_HOST:-}}"
LANGUAGE="${WARPPOOL_LANG:-${WARPOOL_LANG:-}}"
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
  lang=zh|en
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
        RELEASE_VERSION="${arg#version=}"
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
      lang=*|language=*)
        LANGUAGE="${arg#*=}"
        ;;
      *)
        fail_i "unknown argument: $arg" "未知参数：$arg"
        ;;
    esac
  done
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log_i "dry-run: $*" "dry-run：$*"
    return 0
  fi
  "$@"
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail_i "installer must run as root; use: wget -qO- <url> | sudo bash" "安装脚本必须以 root 权限运行；用法：wget -qO- <url> | sudo bash"
  fi
}

require_linux() {
  if [ "$(uname -s | tr '[:upper:]' '[:lower:]')" != "linux" ]; then
    fail_i "main server installer only supports Linux" "主服务器安装脚本仅支持 Linux"
  fi
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
  if [ "$YES" = "true" ]; then
    LANGUAGE="en"
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

check_existing_installation() {
  local existing=0
  local command_path=""
  local uninstall_bin="warppool"

  if command -v warppool >/dev/null 2>&1; then
    command_path="$(command -v warppool)"
    uninstall_bin="$command_path"
    existing=1
    log_i "existing warppool command found: $command_path" "检测到已存在 warppool 命令：$command_path"
  fi
  if [ -e "$INSTALL_DIR/warppool" ]; then
    if [ -z "$command_path" ]; then
      uninstall_bin="$INSTALL_DIR/warppool"
    fi
    existing=1
    log_i "existing WarpPool binary found: $INSTALL_DIR/warppool" "检测到已存在 WarpPool 二进制：$INSTALL_DIR/warppool"
  fi
  if [ -e "$CONFIG_PATH" ]; then
    existing=1
    log_i "existing WarpPool config found: $CONFIG_PATH" "检测到已存在 WarpPool 配置：$CONFIG_PATH"
  fi
  if [ -d "$LIB_DIR" ]; then
    existing=1
    log_i "existing WarpPool installation directory found: $LIB_DIR" "检测到已存在 WarpPool 安装目录：$LIB_DIR"
  fi

  if [ "$existing" -eq 0 ]; then
    return 0
  fi

  if [ "$LANGUAGE" = "zh" ]; then
    cat >&2 <<EOF
[WarpPool][server][ERROR] 检测到本机已经安装过 WarpPool。

如需重新安装，请先执行卸载命令：
  sudo $uninstall_bin uninstall --force

如需同时清理本地 WireGuard 配置、代理/监听服务和运行状态：
  sudo $uninstall_bin uninstall --force --clean-wg --clean-proxy

如果卸载命令不存在，说明可能只剩配置或安装目录残留，请手动检查并清理：
  $CONFIG_PATH
  $LIB_DIR

卸载完成后，再重新执行安装脚本。
EOF
  else
    cat >&2 <<EOF
[WarpPool][server][ERROR] WarpPool already appears to be installed on this machine.

To reinstall, uninstall it first:
  sudo $uninstall_bin uninstall --force

To also remove local WireGuard configs, proxy/listener services, and runtime state:
  sudo $uninstall_bin uninstall --force --clean-wg --clean-proxy

If the uninstall command does not exist, only config or install directory remnants may remain. Check and remove manually:
  $CONFIG_PATH
  $LIB_DIR

After uninstall completes, rerun this installer.
EOF
  fi
  exit 1
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
  local major minor
  major="$(version_major "$OS_VERSION")"
  case "$OS_ID" in
    debian)
      if [ "$major" -lt 12 ]; then
        fail_i "unsupported Debian version: $OS_VERSION, expected Debian 12+" "不支持当前 Debian 版本：$OS_VERSION，需要 Debian 12+"
      fi
      ;;
    ubuntu)
      if [ "$major" -lt 20 ]; then
        fail_i "unsupported Ubuntu version: $OS_VERSION, expected Ubuntu 20.04+" "不支持当前 Ubuntu 版本：$OS_VERSION，需要 Ubuntu 20.04+"
      fi
      ;;
    alpine)
      minor="$(printf '%s' "$OS_VERSION" | cut -d. -f2)"
      minor="${minor:-0}"
      if [ "$major" -lt 3 ] || { [ "$major" -eq 3 ] && [ "$minor" -lt 20 ]; }; then
        fail_i "unsupported Alpine version: $OS_VERSION, expected Alpine 3.20+" "不支持当前 Alpine 版本：$OS_VERSION，需要 Alpine 3.20+"
      fi
      ;;
    *)
      fail_i "unsupported OS: $OS_ID $OS_VERSION" "不支持当前系统：$OS_ID $OS_VERSION"
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
      fail_i "unsupported CPU architecture: $raw, expected amd64 or arm64" "不支持当前 CPU 架构：$raw，需要 amd64 或 arm64"
      ;;
  esac
}

install_base_packages() {
  log_i "installing base packages" "正在安装基础软件包"
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

  log_i "warning: cannot verify port availability because python, ss, and netstat are unavailable" "警告：无法检测端口占用，因为 python、ss、netstat 都不可用"
  return 0
}

is_interactive() {
  [ "$YES" != "true" ] && [ -r /dev/tty ] && [ -w /dev/tty ]
}

choose_listen_port() {
  local input candidate

  if [ -n "$LISTEN_PORT" ]; then
    if ! validate_port_value "$LISTEN_PORT"; then
      fail_i "invalid listener port: $LISTEN_PORT" "监听端口无效：$LISTEN_PORT"
    fi
    if ! port_available "$LISTEN_HOST" "$LISTEN_PORT"; then
      fail_i "listener port is already in use: $LISTEN_HOST:$LISTEN_PORT" "监听端口已被占用：$LISTEN_HOST:$LISTEN_PORT"
    fi
    return 0
  fi

  if ! is_interactive; then
    LISTEN_PORT="$DEFAULT_LISTEN_PORT"
    if ! port_available "$LISTEN_HOST" "$LISTEN_PORT"; then
      fail_i "default listener port is already in use: $LISTEN_HOST:$LISTEN_PORT; rerun with port=<port>" "默认监听端口已被占用：$LISTEN_HOST:$LISTEN_PORT；请用 port=<端口> 重新执行"
    fi
    return 0
  fi

  while true; do
    if [ "$LANGUAGE" = "zh" ]; then
      printf 'WarpPool 注册监听端口 [%s]: ' "$DEFAULT_LISTEN_PORT" >/dev/tty
    else
      printf 'WarpPool registration listener port [%s]: ' "$DEFAULT_LISTEN_PORT" >/dev/tty
    fi
    read -r input </dev/tty
    candidate="${input:-$DEFAULT_LISTEN_PORT}"

    if ! validate_port_value "$candidate"; then
      printf '[WarpPool][server] %s\n' "$(text "invalid port, enter a number between 1 and 65535" "端口无效，请输入 1 到 65535 之间的数字")" >/dev/tty
      continue
    fi
    if ! port_available "$LISTEN_HOST" "$candidate"; then
      printf '[WarpPool][server] %s\n' "$(text "port $candidate is already in use, choose another one" "端口 $candidate 已被占用，请换一个端口")" >/dev/tty
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
  fail_i "curl or wget is required to download $url" "需要 curl 或 wget 来下载 $url"
}

release_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  if [ "$RELEASE_VERSION" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$REPO" "$asset"
    return 0
  fi

  local tag="$RELEASE_VERSION"
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
    log_i "dry-run: download WarpPool release from $url" "dry-run：从 $url 下载 WarpPool 发布包"
    log_i "dry-run: install binary to $INSTALL_DIR/warppool" "dry-run：安装二进制到 $INSTALL_DIR/warppool"
    log_i "dry-run: install assets to $LIB_DIR/assets" "dry-run：安装资源文件到 $LIB_DIR/assets"
    return 0
  fi

  WORK_DIR="$(mktemp -d)"
  archive="$WORK_DIR/warppool.tar.gz"
  log_i "downloading WarpPool release: $url" "正在下载 WarpPool 发布包：$url"
  fetch "$url" "$archive" || fail_i "failed to download WarpPool release package; publish a release first or pass version=vX.Y.Z" "下载 WarpPool 发布包失败；请先发布 Release，或传入 version=vX.Y.Z"

  tar -xzf "$archive" -C "$WORK_DIR" || fail_i "failed to extract WarpPool release package" "解压 WarpPool 发布包失败"
  binary="$(find "$WORK_DIR" -type f -name warppool | head -n 1 || true)"
  if [ -z "$binary" ]; then
    fail_i "warppool binary not found in release package" "发布包中未找到 warppool 二进制文件"
  fi

  mkdir -p "$INSTALL_DIR" "$LIB_DIR/assets"
  cp "$binary" "$INSTALL_DIR/warppool" || fail_i "failed to install warppool binary" "安装 warppool 二进制失败"
  chmod 0755 "$INSTALL_DIR/warppool"

  assets_dir="$(find "$WORK_DIR" -type d -name assets | head -n 1 || true)"
  if [ -n "$assets_dir" ]; then
    cp -R "$assets_dir/." "$LIB_DIR/assets/"
  else
    log_i "warning: assets directory not found in release package" "警告：发布包中未找到 assets 目录"
  fi
}

install_singbox() {
  local script="$LIB_DIR/assets/singbox_install.sh"
  if [ "$DRY_RUN" = "true" ]; then
    log_i "dry-run: install sing-box through $script" "dry-run：通过 $script 安装 sing-box"
    return 0
  fi
  if [ ! -r "$script" ]; then
    log_i "warning: sing-box installer not found: $script" "警告：未找到 sing-box 安装脚本：$script"
    log_i "warning: install sing-box manually before running warppool proxy start" "警告：请在执行 warppool proxy start 前手动安装 sing-box"
    return 0
  fi

  bash "$script" --yes source=default
}

initialize_config() {
  local bin="$INSTALL_DIR/warppool"
  local args
  if [ "$DRY_RUN" = "true" ]; then
    log_i "dry-run: initialize config at $CONFIG_PATH if missing" "dry-run：如果配置不存在，则初始化 $CONFIG_PATH"
    log_i "dry-run: configure listener $LISTEN_HOST:$LISTEN_PORT" "dry-run：配置监听地址 $LISTEN_HOST:$LISTEN_PORT"
    return 0
  fi

  if [ ! -x "$bin" ]; then
    fail_i "warppool binary is not executable: $bin" "warppool 二进制不可执行：$bin"
  fi

  if [ ! -f "$CONFIG_PATH" ]; then
    "$bin" --config "$CONFIG_PATH" config init --language "$LANGUAGE"
  else
    log_i "config already exists, keeping: $CONFIG_PATH" "配置已存在，保留原配置：$CONFIG_PATH"
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
    log_i "dry-run: create systemd service for Deploy Token listener" "dry-run：创建 Deploy Token 监听 systemd 服务"
    log_i "dry-run: create systemd service for local proxy" "dry-run：创建本地代理 systemd 服务"
    return 0
  fi
  if [ "$OS_ID" = "alpine" ]; then
    log_i "warning: systemd service creation skipped on Alpine" "警告：Alpine 上跳过 systemd 服务创建"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    log_i "warning: systemctl not found, Deploy Token listener service not created" "警告：未找到 systemctl，未创建 Deploy Token 监听服务"
    return 0
  fi

  "$bin" --config "$CONFIG_PATH" listen service install --warppool-bin "$bin"
  "$bin" --config "$CONFIG_PATH" listen service enable
  "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" --singbox-bin "$LIB_DIR/bin/sing-box"
}

print_next_steps() {
  if [ "$LANGUAGE" = "zh" ]; then
    cat <<EOF

WarpPool 主服务器安装完成。

监听配置：
  主机：$LISTEN_HOST
  端口：$LISTEN_PORT
  配置：$CONFIG_PATH

后续常用命令：
  warppool deploy --name nat01 --exit-mode direct --proxy mixed --port 10133 --ssh-host <出口节点IP> --ssh-user root
  warppool wg up nat01
  warppool proxy service enable

Deploy Token 监听已完成配置，但不会自动启动。
只在需要时启动：
  warppool listen start

EOF
    return 0
  fi

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
  select_language
  require_root
  require_linux
  check_existing_installation
  load_os_release
  check_supported_os
  detect_arch
  install_base_packages
  choose_listen_port

  log_i "detected OS: $OS_ID $OS_VERSION" "检测到系统：$OS_ID $OS_VERSION"
  log_i "detected arch: $ARCH" "检测到架构：$ARCH"
  log_i "selected listener: $LISTEN_HOST:$LISTEN_PORT" "已选择监听地址：$LISTEN_HOST:$LISTEN_PORT"

  download_and_install_warppool
  install_singbox
  initialize_config
  create_systemd_services
  print_next_steps
}

main "$@"
