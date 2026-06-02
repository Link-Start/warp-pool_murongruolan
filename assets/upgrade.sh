#!/usr/bin/env bash
set -Eeuo pipefail

REPO="${WARPPOOL_REPO:-${WARPOOL_REPO:-murongruolan/warp-pool}}"
VERSION_ARG="${WARPPOOL_VERSION:-${WARPOOL_VERSION:-latest}}"
INSTALL_DIR="${WARPPOOL_INSTALL_DIR:-${WARPOOL_INSTALL_DIR:-/usr/local/bin}}"
LIB_DIR="${WARPPOOL_LIB_DIR:-${WARPOOL_LIB_DIR:-/usr/local/lib/warppool}}"
CONFIG_PATH="${WARPPOOL_CONFIG_PATH:-${WARPOOL_CONFIG_PATH:-/etc/warppool/config.json}}"
LANGUAGE="${WARPPOOL_LANGUAGE:-${WARPOOL_LANGUAGE:-en}}"
LOCAL_FILE=""
DRY_RUN="false"
YES="false"
WORK_DIR=""
expected=""
actual=""
ARCH=""
NEW_BINARY=""
NEW_ASSETS=""
ARCHIVE=""
CHECKSUMS=""
ASSET_NAME=""
CURRENT_STEP=""
CURRENT_ARG=""
PROGRESS_RUNNING="false"
PROGRESS_STATE=""
PROGRESS_PID=""
PROGRESS_STARTED_AT=""
PROGRESS_LINE_COUNT=0

log() {
  if [ "${PROGRESS_RUNNING:-false}" = "true" ] && [ -t 1 ]; then
    printf '[WarpPool][upgrade] %s\n' "$*"
    render_progress
    return 0
  fi
  printf '[WarpPool][upgrade] %s\n' "$*"
}

fail() {
  stop_progress
  printf '[WarpPool][upgrade][ERROR] %s\n' "$*" >&2
  exit 1
}

cleanup() {
  stop_progress
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf -- "$WORK_DIR"
  fi
}

trap cleanup EXIT

usage() {
  cat <<'USAGE'
WarpPool upgrade helper

Usage:
  warppool upgrade [--version latest|v0.1.1] [--yes] [--dry-run]
  warppool upgrade --file /path/to/warppool-linux-amd64.tar.gz [--yes] [--dry-run]

Environment:
  WARPPOOL_REPO=owner/name
  WARPPOOL_INSTALL_DIR=/usr/local/bin
  WARPPOOL_LIB_DIR=/usr/local/lib/warppool
  WARPPOOL_CONFIG_PATH=/etc/warppool/config.json
  WARPPOOL_LANGUAGE=en|zh
USAGE
}

normalize_language() {
  case "$LANGUAGE" in
    zh|zh_CN|zh-CN|cn|CN) LANGUAGE="zh" ;;
    *) LANGUAGE="en" ;;
  esac
}

t() {
  local en="$1"
  local zh="$2"
  if [ "$LANGUAGE" = "zh" ]; then
    printf '%s\n' "$zh"
  else
    printf '%s\n' "$en"
  fi
}

progress_text() {
  local key="$1"
  local arg="${2:-}"
  case "$key" in
    confirm)
      t "Waiting for confirmation..." "等待用户确认..."
      ;;
    backup)
      t "Backing up current installation..." "正在备份当前安装..."
      ;;
    package)
      if [ -n "$arg" ]; then
        t "Using local package: $arg" "使用本地安装包：$arg"
      else
        t "Downloading release package..." "正在下载发布包..."
      fi
      ;;
    verify)
      t "Verifying package architecture and version..." "正在校验安装包架构和版本..."
      ;;
    install)
      t "Installing binary and bundled assets..." "正在安装二进制和资源文件..."
      ;;
    singbox)
      t "Refreshing sing-box..." "正在刷新 sing-box..."
      ;;
    systemd)
      t "Refreshing systemd service files..." "正在刷新 systemd 服务文件..."
      ;;
    restart)
      t "Restarting WarpPool services..." "正在重启 WarpPool 服务..."
      ;;
    done)
      t "Upgrade completed." "升级完成。"
      ;;
    *)
      printf '%s\n' "$key"
      ;;
  esac
}

progress_stage() {
  case "$1" in
    confirm) printf '1' ;;
    backup) printf '2' ;;
    package) printf '3' ;;
    verify) printf '4' ;;
    install) printf '5' ;;
    singbox) printf '6' ;;
    systemd) printf '7' ;;
    restart) printf '8' ;;
    done) printf '8' ;;
    *) printf '1' ;;
  esac
}

progress_bar() {
  local stage="$1"
  local total=8
  local width=12
  local filled=$((stage * width / total))
  [ "$filled" -lt 1 ] && filled=1
  [ "$filled" -gt "$width" ] && filled="$width"
  local empty=$((width - filled))
  printf '['
  printf '%*s' "$filled" '' | tr ' ' '='
  printf '%*s' "$empty" '' | tr ' ' '-'
  printf ']'
}

render_progress() {
  [ "$PROGRESS_RUNNING" = "true" ] || return 0
  [ -t 1 ] || return 0
  local key="$CURRENT_STEP"
  [ -n "$key" ] || return 0
  local stage total elapsed msg bar
  stage="$(progress_stage "$key")"
  total=8
  elapsed="$(($(date +%s) - PROGRESS_STARTED_AT))"
  msg="$(progress_text "$key" "$CURRENT_ARG")"
  bar="$(progress_bar "$stage")"
  clear_progress_output
  local label
  if [ "$LANGUAGE" = "zh" ]; then
    if [ -n "$LOCAL_FILE" ]; then
      label="本地包升级"
    else
      label="在线升级"
    fi
    draw_progress_block "$(printf '[WarpPool] %s %s/%s %s | 已用 %ss\n当前步骤：%s' "$bar" "$stage" "$total" "$label" "$elapsed" "$msg")"
  else
    if [ -n "$LOCAL_FILE" ]; then
      label="local package upgrade"
    else
      label="release upgrade"
    fi
    draw_progress_block "$(printf '[WarpPool] %s %s/%s %s | %ss elapsed\nCurrent step: %s' "$bar" "$stage" "$total" "$label" "$elapsed" "$msg")"
  fi
}

progress_loop() {
  while true; do
    if [ -n "${PROGRESS_STATE:-}" ] && [ -r "$PROGRESS_STATE" ]; then
      {
        IFS= read -r CURRENT_STEP || CURRENT_STEP=""
        IFS= read -r CURRENT_ARG || CURRENT_ARG=""
      } <"$PROGRESS_STATE"
    fi
    render_progress
    sleep 1
  done
}

start_progress() {
  [ -t 1 ] || return 0
  PROGRESS_RUNNING="true"
  PROGRESS_STATE="$(mktemp)"
  PROGRESS_STARTED_AT="$(date +%s)"
  PROGRESS_LINE_COUNT=0
  progress_loop &
  PROGRESS_PID="$!"
}

stop_progress() {
  if [ "${PROGRESS_RUNNING:-false}" = "true" ]; then
    PROGRESS_RUNNING="false"
    if [ -n "${PROGRESS_PID:-}" ]; then
      kill "$PROGRESS_PID" >/dev/null 2>&1 || true
      wait "$PROGRESS_PID" 2>/dev/null || true
    fi
    if [ -t 1 ]; then
      clear_progress_output
      printf '\n'
    fi
    if [ -n "${PROGRESS_STATE:-}" ]; then
      rm -f -- "$PROGRESS_STATE"
      PROGRESS_STATE=""
    fi
  fi
}

set_step() {
  CURRENT_STEP="$1"
  CURRENT_ARG="${2:-}"
  if [ -t 1 ]; then
    if [ -n "${PROGRESS_STATE:-}" ]; then
      {
        printf '%s\n' "$CURRENT_STEP"
        printf '%s\n' "$CURRENT_ARG"
      } >"$PROGRESS_STATE"
    fi
    render_progress
  else
    log "$(progress_text "$CURRENT_STEP" "$CURRENT_ARG")"
  fi
}

clear_progress_output() {
  [ -t 1 ] || return 0
  while [ "${PROGRESS_LINE_COUNT:-0}" -gt 0 ]; do
    printf '\r\033[2K'
    PROGRESS_LINE_COUNT=$((PROGRESS_LINE_COUNT - 1))
    if [ "$PROGRESS_LINE_COUNT" -gt 0 ]; then
      printf '\033[1A'
    fi
  done
}

draw_progress_block() {
  local text="$1"
  printf '%s' "$text"
  PROGRESS_LINE_COUNT=1
  case "$text" in
    *$'\n'*) PROGRESS_LINE_COUNT=2 ;;
  esac
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --help|-h)
        usage
        exit 0
        ;;
      --version)
        shift
        [ "$#" -gt 0 ] || fail "--version requires a value"
        VERSION_ARG="$1"
        ;;
      --version=*)
        VERSION_ARG="${1#--version=}"
        ;;
      --repo)
        shift
        [ "$#" -gt 0 ] || fail "--repo requires a value"
        REPO="$1"
        ;;
      --repo=*)
        REPO="${1#--repo=}"
        ;;
      --file|--local)
        shift
        [ "$#" -gt 0 ] || fail "--file requires a value"
        LOCAL_FILE="$1"
        ;;
      --file=*|--local=*)
        LOCAL_FILE="${1#*=}"
        ;;
      --lang|--language)
        shift
        [ "$#" -gt 0 ] || fail "--language requires a value"
        LANGUAGE="$1"
        ;;
      --lang=*|--language=*)
        LANGUAGE="${1#*=}"
        ;;
      --yes|-y)
        YES="true"
        ;;
      --dry-run)
        DRY_RUN="true"
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
    shift
  done
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail "upgrade must run as root"
  fi
}

detect_arch() {
  local raw
  raw="$(uname -m)"
  case "$raw" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported CPU architecture: $raw" ;;
  esac
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
  fail "curl or wget is required"
}

release_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  if [ "$VERSION_ARG" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/%s\n' "$REPO" "$asset"
    return 0
  fi
  local tag="$VERSION_ARG"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$tag" "$asset"
}

checksum_url() {
  if [ "$VERSION_ARG" = "latest" ]; then
    printf 'https://github.com/%s/releases/latest/download/checksums.txt\n' "$REPO"
    return 0
  fi
  local tag="$VERSION_ARG"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/checksums.txt\n' "$REPO" "$tag"
}

fallback_download_url() {
  local asset="warppool-linux-${ARCH}.tar.gz"
  local tag
  tag="$(tr -d '[:space:]' </usr/local/lib/warppool/VERSION 2>/dev/null || true)"
  [ -n "$tag" ] || tag="$(tr -d '[:space:]' </etc/warppool/VERSION 2>/dev/null || true)"
  [ -n "$tag" ] || tag="0.1.1"
  case "$tag" in
    v*) ;;
    *) tag="v$tag" ;;
  esac
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$tag" "$asset"
}

confirm() {
  set_step confirm
  if [ "$YES" = "true" ]; then
    return 0
  fi
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    fail "non-interactive upgrade requires --yes"
  fi
  if [ -n "$LOCAL_FILE" ]; then
    printf '%s\n' "$(t "WARNING: local packages are trusted as root-installed code. Only use packages you built or trust." "警告：本地安装包会以 root 权限安装，请只使用自己构建或信任的安装包。")" >/dev/tty
    printf '%s' "$(t "Upgrade WarpPool from local package $LOCAL_FILE? [y/N]: " "从本地安装包 $LOCAL_FILE 升级 WarpPool？[y/N]：")" >/dev/tty
  else
    printf 'Upgrade WarpPool from %s? [y/N]: ' "$(release_url)" >/dev/tty
  fi
  read -r answer </dev/tty
  case "$answer" in
    y|Y|yes|YES) ;;
    *) fail "upgrade cancelled" ;;
  esac
}

backup_existing() {
  set_step backup
  BACKUP_DIR="/var/backups/warppool/$(date -u +%Y%m%dT%H%M%SZ)"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: backup existing installation to $BACKUP_DIR"
    return 0
  fi
  mkdir -p "$BACKUP_DIR"
  if [ -x "$INSTALL_DIR/warppool" ]; then
    cp "$INSTALL_DIR/warppool" "$BACKUP_DIR/warppool"
  fi
  if [ -d "$LIB_DIR/assets" ]; then
    mkdir -p "$BACKUP_DIR/assets"
    cp -R "$LIB_DIR/assets/." "$BACKUP_DIR/assets/"
  fi
  if [ -f "$CONFIG_PATH" ]; then
    cp "$CONFIG_PATH" "$BACKUP_DIR/config.json"
  fi
  log "backup created: $BACKUP_DIR"
}

download_and_verify() {
  WORK_DIR="$(mktemp -d)"
  ARCHIVE="$WORK_DIR/warppool.tar.gz"
  CHECKSUMS="$WORK_DIR/checksums.txt"
  ASSET_NAME="warppool-linux-${ARCH}.tar.gz"

  set_step package "$LOCAL_FILE"
  if [ -n "$LOCAL_FILE" ]; then
    [ -f "$LOCAL_FILE" ] || fail "$(t "local package not found: $LOCAL_FILE" "本地安装包不存在：$LOCAL_FILE")"
    [ -s "$LOCAL_FILE" ] || fail "$(t "local package is empty: $LOCAL_FILE" "本地安装包为空：$LOCAL_FILE")"
    cp "$LOCAL_FILE" "$ARCHIVE"
    log "using local package: $LOCAL_FILE"
    log "$(t "WARNING: local packages are trusted as root-installed code. Only use packages you built or trust." "警告：本地安装包会以 root 权限安装，请只使用自己构建或信任的安装包。")"
  else
    log "downloading $(release_url)"
    if ! fetch "$(release_url)" "$ARCHIVE"; then
      log "latest download failed; trying fallback version URL"
      fetch "$(fallback_download_url)" "$ARCHIVE"
    fi
    if fetch "$(checksum_url)" "$CHECKSUMS"; then
      if command -v sha256sum >/dev/null 2>&1; then
        expected=""
        actual=""
        expected="$(awk -v asset="$ASSET_NAME" '$2 ~ asset "$" {print $1; exit}' "$CHECKSUMS")"
        [ -n "$expected" ] || fail "checksum for $ASSET_NAME not found"
        actual="$(sha256sum "$ARCHIVE" | awk '{print $1}')"
        [ "$actual" = "$expected" ] || fail "checksum mismatch for $ASSET_NAME"
      else
        log "warning: sha256sum not found, checksum verification skipped"
      fi
    else
      log "warning: checksums.txt not available, checksum verification skipped"
    fi
  fi

  tar -xzf "$ARCHIVE" -C "$WORK_DIR"
  NEW_BINARY="$(find "$WORK_DIR" -type f -name warppool | head -n 1 || true)"
  NEW_ASSETS="$(find "$WORK_DIR" -type d -name assets | head -n 1 || true)"
  [ -n "$NEW_BINARY" ] || fail "$(t "warppool binary not found in release package" "发布包中未找到 warppool 二进制文件")"
  [ -n "$NEW_ASSETS" ] || fail "$(t "assets directory not found in release package" "发布包中未找到 assets 目录")"
  verify_package
}

current_version() {
  if [ -x "$INSTALL_DIR/warppool" ]; then
    "$INSTALL_DIR/warppool" version 2>/dev/null | awk '/^version:/ {print $2; exit}' || true
  fi
}

binary_version() {
  "$1" version 2>/dev/null | awk '/^version:/ {print $2; exit}' || true
}

verify_package() {
  set_step verify
  [ -x "$NEW_BINARY" ] || chmod 0755 "$NEW_BINARY" || fail "$(t "cannot make package binary executable" "无法给安装包中的二进制文件添加可执行权限")"
  local new_version current
  if ! "$NEW_BINARY" version >/dev/null 2>&1; then
    fail "$(t "package binary cannot run on this system; it may be the wrong CPU architecture. Expected linux-$ARCH." "安装包中的二进制无法在当前系统运行，可能是 CPU 架构不匹配。当前系统需要 linux-$ARCH。")"
  fi
  new_version="$(binary_version "$NEW_BINARY")"
  [ -n "$new_version" ] || fail "$(t "cannot detect package version" "无法检测安装包版本")"
  current="$(current_version)"
  if [ -n "$current" ] && [ "$new_version" = "$current" ]; then
    log "warning: package version equals installed version: $new_version"
  fi
  log "package version: $new_version"
  [ -z "$current" ] || log "installed version: $current"
}

install_new_version() {
  set_step install
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install binary to $INSTALL_DIR/warppool"
    log "dry-run: install assets to $LIB_DIR/assets"
    return 0
  fi
  mkdir -p "$INSTALL_DIR" "$LIB_DIR/assets"

  local tmp_binary tmp_assets old_assets
  tmp_binary="$(mktemp "$INSTALL_DIR/.warppool.new.XXXXXX")"
  cp "$NEW_BINARY" "$tmp_binary"
  chmod 0755 "$tmp_binary"
  mv -f "$tmp_binary" "$INSTALL_DIR/warppool"

  tmp_assets="$(mktemp -d "$LIB_DIR/assets.new.XXXXXX")"
  cp -R "$NEW_ASSETS/." "$tmp_assets/"
  old_assets="$LIB_DIR/assets.old.$$"
  if [ -d "$LIB_DIR/assets" ]; then
    mv "$LIB_DIR/assets" "$old_assets"
  fi
  mv "$tmp_assets" "$LIB_DIR/assets"
  rm -rf -- "$old_assets"

  log "installed binary: $INSTALL_DIR/warppool"
  log "installed assets: $LIB_DIR/assets"
}

refresh_command_aliases() {
  local target alias_path
  target="$INSTALL_DIR/warppool"
  alias_path="$INSTALL_DIR/wpl"

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: refresh command alias $alias_path -> $target"
    return 0
  fi

  if [ -e "$alias_path" ] && [ ! -L "$alias_path" ]; then
    log "warning: command alias $alias_path already exists and is not a symlink; keeping existing file"
    return 0
  fi

  ln -sf "$target" "$alias_path"
  log "installed command alias: $alias_path -> $target"
}

service_exists() {
  command -v systemctl >/dev/null 2>&1 || return 1
  systemctl list-unit-files "$1" >/dev/null 2>&1 || systemctl status "$1" >/dev/null 2>&1
}

service_should_restart() {
  local service="$1"
  service_exists "$service" || return 1
  systemctl is-enabled "$service" >/dev/null 2>&1 || systemctl is-active "$service" >/dev/null 2>&1
}

stop_service_for_upgrade() {
  local service="$1"
  service_exists "$service" || return 0
  if systemctl is-active "$service" >/dev/null 2>&1; then
    systemctl stop "$service" >/dev/null 2>&1 || log "warning: failed to stop $service before upgrade"
  fi
}

quiet_run() {
  local output status
  set +e
  output="$("$@" 2>&1)"
  status=$?
  set -e
  if [ "$status" -ne 0 ] && [ -n "$output" ]; then
    printf '%s\n' "$output" >&2
  fi
  return "$status"
}

write_version_marker() {
  if [ "$DRY_RUN" = "true" ]; then
    return 0
  fi
  local value="$VERSION_ARG"
  if [ "$value" = "latest" ]; then
    value="$("$INSTALL_DIR/warppool" version 2>/dev/null | awk '/^version:/ {print $2; exit}' || true)"
    [ -n "$value" ] || value="latest"
  fi
  mkdir -p "$LIB_DIR"
  printf '%s\n' "$value" >"$LIB_DIR/VERSION" || true
}

restart_services() {
  set_step restart
  if ! command -v systemctl >/dev/null 2>&1; then
    log "systemctl not found, skipping service restart"
    return 0
  fi
  for service in warppool-listen.service warppool-proxy.service; do
    if service_should_restart "$service"; then
      log "restarting $service"
      if [ "$DRY_RUN" = "true" ]; then
        log "dry-run: systemctl restart $service"
      else
        systemctl restart "$service" || log "warning: failed to restart $service"
      fi
    fi
  done
}

refresh_systemd_units() {
  set_step systemd
  local bin="$INSTALL_DIR/warppool"
  local SINGBOX_BIN=""
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: refresh systemd unit files"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    return 0
  fi
  if [ -x "$bin" ]; then
    quiet_run "$bin" --config "$CONFIG_PATH" listen service install --warppool-bin "$bin" || log "warning: failed to refresh listen service"
    if [ -x "$LIB_DIR/bin/sing-box" ]; then
      SINGBOX_BIN="$LIB_DIR/bin/sing-box"
    else
      SINGBOX_BIN="$(command -v sing-box || true)"
    fi
    if [ -n "$SINGBOX_BIN" ]; then
      quiet_run "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" --singbox-bin "$SINGBOX_BIN" || log "warning: failed to refresh proxy service"
    else
      quiet_run "$bin" --config "$CONFIG_PATH" proxy service install --warppool-bin "$bin" || log "warning: failed to refresh proxy service"
    fi
  fi
}

refresh_singbox() {
  set_step singbox
  local script="$LIB_DIR/assets/singbox_install.sh"
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: refresh sing-box through $script"
    return 0
  fi
  if [ -r "$script" ]; then
    quiet_run bash "$script" --yes source=auto || log "warning: failed to refresh sing-box"
  else
    log "warning: sing-box installer not found after upgrade: $script"
  fi
}

main() {
  parse_args "$@"
  normalize_language
  require_root
  command -v tar >/dev/null 2>&1 || fail "tar is required"
  detect_arch
  confirm
  if [ "$DRY_RUN" != "true" ]; then
    stop_service_for_upgrade warppool-proxy.service
  fi
  start_progress
  backup_existing
  download_and_verify
  install_new_version
  refresh_command_aliases
  write_version_marker
  refresh_singbox
  refresh_systemd_units
  restart_services
  set_step done
  stop_progress
  log "upgrade completed"
}

main "$@"
