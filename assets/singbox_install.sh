#!/usr/bin/env bash
set -Eeuo pipefail

SOURCE="${WARPOOL_SINGBOX_SOURCE:-}"
VERSION="${WARPOOL_SINGBOX_VERSION:-latest}"
VARIANT="${WARPOOL_SINGBOX_VARIANT:-default}"
CUSTOM_URL="${WARPOOL_SINGBOX_URL:-}"
INSTALL_DIR="${WARPOOL_SINGBOX_INSTALL_DIR:-/usr/local/lib/warppool/bin}"
DRY_RUN="false"
YES="false"
WORK_DIR=""
tmp_target=""

GITHUB_API_LATEST="https://api.github.com/repos/SagerNet/sing-box/releases/latest"
GITHUB_RELEASE_BASE="https://github.com/SagerNet/sing-box/releases/download"

log() {
  printf '[WarpPool][sing-box] %s\n' "$*"
}

fail() {
  printf '[WarpPool][sing-box][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  printf '[WarpPool][sing-box][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

cleanup() {
  if [ -n "${tmp_target:-}" ] && [ -e "$tmp_target" ]; then
    rm -f -- "$tmp_target"
  fi
  if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
    rm -rf -- "$WORK_DIR"
  fi
}

trap 'on_error $LINENO' ERR
trap cleanup EXIT

usage() {
  cat <<'USAGE'
WarpPool sing-box installer

Usage:
  bash singbox_install.sh [source=default|custom|existing] [url=https://...] [version=latest|v1.13.12] [variant=default|glibc|musl] [install_dir=/path] [--dry-run] [--yes]

Sources:
  default   Use WarpPool built-in GitHub release URL.
  custom    Download from a user-provided URL.
  existing  Do not download; only verify an existing sing-box binary.

Variants:
  default   sing-box-<version>-linux-<arch>.tar.gz
  glibc     sing-box-<version>-linux-<arch>-glibc.tar.gz
  musl      sing-box-<version>-linux-<arch>-musl.tar.gz

Examples:
  bash singbox_install.sh
  bash singbox_install.sh --yes source=default
  bash singbox_install.sh source=default variant=glibc
  bash singbox_install.sh source=custom url=https://example.com/sing-box-linux-amd64.tar.gz
  bash singbox_install.sh source=existing
  bash singbox_install.sh --dry-run source=default install_dir=/tmp/warppool-bin
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
      --yes|-y)
        YES="true"
        ;;
      source=*)
        SOURCE="${arg#source=}"
        ;;
      url=*)
        CUSTOM_URL="${arg#url=}"
        ;;
      version=*)
        VERSION="${arg#version=}"
        ;;
      variant=*)
        VARIANT="${arg#variant=}"
        ;;
      install_dir=*)
        INSTALL_DIR="${arg#install_dir=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
    esac
  done
}

is_interactive() {
  [ -t 0 ] && [ "$YES" != "true" ]
}

choose_source() {
  if [ -n "$SOURCE" ]; then
    return 0
  fi

  if is_interactive; then
    log "Choose sing-box source:"
    log "  1) built-in GitHub release URL (default)"
    log "  2) manually enter download URL"
    log "  3) skip download and use existing sing-box"
    printf 'Select [1/2/3]: '
    local choice
    read -r choice
    case "$choice" in
      ""|1)
        SOURCE="default"
        ;;
      2)
        SOURCE="custom"
        ;;
      3)
        SOURCE="existing"
        ;;
      *)
        fail "invalid source selection: $choice"
        ;;
    esac
    return 0
  fi

  SOURCE="default"
}

normalize_source() {
  case "$SOURCE" in
    default|github|builtin|system)
      SOURCE="default"
      ;;
    custom|manual)
      SOURCE="custom"
      ;;
    existing|skip|none)
      SOURCE="existing"
      ;;
    *)
      fail "unsupported source: $SOURCE, expected default, custom, or existing"
      ;;
  esac
}

normalize_variant() {
  case "$VARIANT" in
    default|"")
      SINGBOX_VARIANT_SUFFIX=""
      ;;
    glibc|musl)
      SINGBOX_VARIANT_SUFFIX="-$VARIANT"
      ;;
    *)
      fail "unsupported variant: $VARIANT, expected default, glibc, or musl"
      ;;
  esac
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    fail "required command not found: $name"
  fi
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  if [ "$os" != "linux" ]; then
    fail "unsupported OS for this installer: $os, expected linux"
  fi

  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)
      SINGBOX_ARCH="amd64"
      ;;
    aarch64|arm64)
      SINGBOX_ARCH="arm64"
      ;;
    *)
      fail "unsupported CPU architecture for sing-box auto download: $arch"
      ;;
  esac
}

resolve_latest_version() {
  local json tag
  require_command curl
  log "resolving latest sing-box release from GitHub"
  json="$(curl -fsSL "$GITHUB_API_LATEST")" || fail "failed to query GitHub latest release API: $GITHUB_API_LATEST"
  tag="$(printf '%s\n' "$json" | sed -n 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$tag" ]; then
    fail "cannot parse latest sing-box release tag from GitHub API"
  fi

  SINGBOX_TAG="$tag"
  SINGBOX_VERSION="${tag#v}"
}

resolve_fixed_version() {
  local input="$1"
  if [ "${input#v}" != "$input" ]; then
    SINGBOX_TAG="$input"
    SINGBOX_VERSION="${input#v}"
    return 0
  fi

  SINGBOX_TAG="v$input"
  SINGBOX_VERSION="$input"
}

build_default_url() {
  if [ "$VERSION" = "latest" ]; then
    resolve_latest_version
  else
    resolve_fixed_version "$VERSION"
  fi

  DOWNLOAD_URL="$GITHUB_RELEASE_BASE/$SINGBOX_TAG/sing-box-$SINGBOX_VERSION-linux-$SINGBOX_ARCH$SINGBOX_VARIANT_SUFFIX.tar.gz"
}

ask_custom_url() {
  if [ -n "$CUSTOM_URL" ]; then
    return 0
  fi
  if is_interactive; then
    printf 'Enter sing-box download URL: '
    read -r CUSTOM_URL
  fi
  if [ -z "$CUSTOM_URL" ]; then
    fail "url is required when source=custom"
  fi
}

existing_binary() {
  if [ -x "/usr/local/lib/warppool/bin/sing-box" ]; then
    printf '%s\n' "/usr/local/lib/warppool/bin/sing-box"
    return 0
  fi

  if command -v sing-box >/dev/null 2>&1; then
    command -v sing-box
    return 0
  fi

  return 1
}

verify_existing() {
  local binary
  binary="$(existing_binary || true)"
  if [ -z "$binary" ]; then
    fail "sing-box binary not found; install it first or run with source=default"
  fi

  log "found existing sing-box: $binary"
  "$binary" version >/dev/null 2>&1 || fail "existing sing-box cannot run: $binary"
  "$binary" version | head -n 1
}

prepare_download_url() {
  case "$SOURCE" in
    default)
      build_default_url
      ;;
    custom)
      ask_custom_url
      DOWNLOAD_URL="$CUSTOM_URL"
      ;;
    existing)
      return 0
      ;;
  esac
}

ensure_install_dir() {
  INSTALL_DIR="${INSTALL_DIR%/}"
  if [ -z "$INSTALL_DIR" ]; then
    fail "install_dir cannot be empty"
  fi

  if [ "$DRY_RUN" = "true" ]; then
    return 0
  fi

  if [ "$INSTALL_DIR" = "/usr/local/lib/warppool/bin" ] && [ "$(id -u)" -ne 0 ]; then
    fail "installer must run as root to write $INSTALL_DIR; set install_dir=... for a user-writable path"
  fi

  mkdir -p "$INSTALL_DIR" || fail "failed to create install directory: $INSTALL_DIR"
}

extract_archive_binary() {
  local archive="$1"
  local extract_dir="$2"
  local binary

  mkdir -p "$extract_dir"
  tar -xzf "$archive" -C "$extract_dir" || fail "failed to extract archive: $archive"
  binary="$(find "$extract_dir" -type f -name sing-box | head -n 1 || true)"
  if [ -z "$binary" ]; then
    fail "sing-box binary not found in archive: $archive"
  fi
  printf '%s\n' "$binary"
}

download_and_install() {
  local target source_file archive_name binary version_line
  target="$INSTALL_DIR/sing-box"

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: source=$SOURCE"
    log "dry-run: variant=$VARIANT"
    log "dry-run: download URL: $DOWNLOAD_URL"
    log "dry-run: install target: $target"
    return 0
  fi

  require_command curl
  WORK_DIR="$(mktemp -d)"
  archive_name="$WORK_DIR/sing-box-download"
  log "downloading sing-box from $DOWNLOAD_URL"
  curl -fL "$DOWNLOAD_URL" -o "$archive_name" || fail "failed to download sing-box from $DOWNLOAD_URL; retry with source=custom url=..."

  case "$DOWNLOAD_URL" in
    *.tar.gz|*.tgz)
      require_command tar
      binary="$(extract_archive_binary "$archive_name" "$WORK_DIR/extract")"
      ;;
    *)
      source_file="$archive_name"
      binary="$source_file"
      ;;
  esac

  tmp_target="$(mktemp "$INSTALL_DIR/.sing-box.new.XXXXXX")" || fail "failed to create temporary sing-box target"
  cp "$binary" "$tmp_target" || fail "failed to copy sing-box to $tmp_target"
  chmod 0755 "$tmp_target" || fail "failed to chmod sing-box: $tmp_target"
  mv -f "$tmp_target" "$target" || fail "failed to replace sing-box at $target"
  tmp_target=""
  "$target" version >/dev/null 2>&1 || fail "installed sing-box cannot run: $target; try a glibc/musl-specific custom URL"
  version_line="$("$target" version | head -n 1)"
  log "installed $version_line to $target"
}

main() {
  parse_args "$@"
  choose_source
  normalize_source
  normalize_variant

  if [ "$SOURCE" = "existing" ]; then
    verify_existing
    return 0
  fi

  require_command uname
  require_command sed
  detect_platform
  prepare_download_url
  ensure_install_dir
  download_and_install
}

main "$@"
