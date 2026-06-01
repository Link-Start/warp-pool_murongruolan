#!/usr/bin/env bash
set -Eeuo pipefail

POLICY="auto"
DRY_RUN="false"
MIN_WARP_FREE_MB="${WARPPOOL_WARP_MIN_FREE_MB:-${WARPOOL_WARP_MIN_FREE_MB:-1200}}"
MIN_APT_CACHE_FREE_MB="${WARPPOOL_WARP_MIN_APT_CACHE_MB:-${WARPOOL_WARP_MIN_APT_CACHE_MB:-300}}"
MIN_TMP_FREE_MB="${WARPPOOL_WARP_MIN_TMP_MB:-${WARPOOL_WARP_MIN_TMP_MB:-100}}"
MIN_FREE_INODES="${WARPPOOL_WARP_MIN_FREE_INODES:-${WARPOOL_WARP_MIN_FREE_INODES:-5000}}"
APT_DOWNLOAD_MB=0
APT_INSTALL_MB=0

log() {
  printf '[WarpPool][warp] %s\n' "$*"
}

fail() {
  printf '[WarpPool][warp][ERROR] %s\n' "$*" >&2
  exit 1
}

on_error() {
  local status=$?
  local line="$1"
  cleanup_package_cache >/dev/null 2>&1 || true
  printf '[WarpPool][warp][ERROR] command failed with exit %s at line %s: %s\n' "$status" "$line" "$BASH_COMMAND" >&2
  exit "$status"
}

trap 'on_error $LINENO' ERR

parse_args() {
  for arg in "$@"; do
    case "$arg" in
      --dry-run)
        DRY_RUN="true"
        ;;
      policy=*|warp_install=*)
        POLICY="${arg#*=}"
        ;;
      *)
        fail "unknown argument: $arg"
        ;;
    esac
  done
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

run() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: $*"
    return 0
  fi
  "$@"
}

cleanup_package_cache() {
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: clean apt cache"
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    apt-get clean >/dev/null 2>&1 || true
    rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*.deb /var/cache/apt/archives/partial/* 2>/dev/null || true
  fi
}

validate_non_negative_int() {
  case "$2" in
    ""|*[!0-9]*)
      fail "$1 must be a non-negative integer, got: $2"
      ;;
  esac
}

size_to_mb() {
  local value="$1"
  local unit="$2"
  value="$(printf '%s' "$value" | tr -d ',')"
  awk -v value="$value" -v unit="$unit" '
    BEGIN {
      mb = value + 0
      if (unit == "B") {
        mb = mb / 1024 / 1024
      } else if (unit == "kB" || unit == "KB" || unit == "KiB") {
        mb = mb / 1024
      } else if (unit == "GB" || unit == "GiB") {
        mb = mb * 1024
      } else if (unit == "TB" || unit == "TiB") {
        mb = mb * 1024 * 1024
      }
      printf "%d\n", int(mb + 0.999)
    }
  '
}

available_mb() {
  df -Pm "$1" 2>/dev/null | awk 'NR==2 {print $4}'
}

available_inodes() {
  df -Pi "$1" 2>/dev/null | awk 'NR==2 {print $4}'
}

log_disk_status() {
  log "disk status:"
  df -h / /usr /var /tmp 2>/dev/null || true
  log "inode status:"
  df -ih / /usr /var /tmp 2>/dev/null || true
}

check_free_mb() {
  local path="$1"
  local required="$2"
  local purpose="$3"
  local free
  free="$(available_mb "$path" || true)"
  if [ -z "$free" ]; then
    log "warning: cannot detect free space for $path"
    return 0
  fi
  if [ "$free" -lt "$required" ]; then
    log_disk_status
    fail "insufficient disk space for Cloudflare WARP install: $path has ${free}MB free, requires at least ${required}MB for $purpose. Free disk/quota or use direct mode."
  fi
}

check_free_inodes() {
  local path="$1"
  local required="$2"
  local free
  free="$(available_inodes "$path" || true)"
  if [ -z "$free" ]; then
    log "warning: cannot detect free inodes for $path"
    return 0
  fi
  if [ "$free" -lt "$required" ]; then
    log_disk_status
    fail "insufficient free inodes for Cloudflare WARP install: $path has ${free} free inodes, requires at least ${required}. Free disk/quota or use direct mode."
  fi
}

estimate_apt_space() {
  local output need_line add_line need_value need_unit add_value add_unit
  APT_DOWNLOAD_MB=0
  APT_INSTALL_MB=0
  output="$(LC_ALL=C apt-get -s install -y cloudflare-warp 2>/dev/null || true)"

  need_line="$(printf '%s\n' "$output" | sed -n 's/^Need to get \([^ ]*\) \([^ ]*\) of archives\..*/\1 \2/p' | tail -n 1)"
  add_line="$(printf '%s\n' "$output" | sed -n 's/^After this operation, \([^ ]*\) \([^ ]*\) of additional disk space will be used\..*/\1 \2/p' | tail -n 1)"

  if [ -n "$need_line" ]; then
    read -r need_value need_unit <<EOF
$need_line
EOF
    APT_DOWNLOAD_MB="$(size_to_mb "$need_value" "$need_unit")"
  fi
  if [ -n "$add_line" ]; then
    read -r add_value add_unit <<EOF
$add_line
EOF
    APT_INSTALL_MB="$(size_to_mb "$add_value" "$add_unit")"
  fi
}

preflight_basic_disk_space() {
  validate_non_negative_int WARPPOOL_WARP_MIN_FREE_MB "$MIN_WARP_FREE_MB"
  validate_non_negative_int WARPPOOL_WARP_MIN_APT_CACHE_MB "$MIN_APT_CACHE_FREE_MB"
  validate_non_negative_int WARPPOOL_WARP_MIN_TMP_MB "$MIN_TMP_FREE_MB"
  validate_non_negative_int WARPPOOL_WARP_MIN_FREE_INODES "$MIN_FREE_INODES"

  check_free_mb /var "$MIN_APT_CACHE_FREE_MB" "apt metadata/cache"
  check_free_mb /tmp "$MIN_TMP_FREE_MB" "temporary files"
  check_free_inodes /var "$MIN_FREE_INODES"
  check_free_inodes /tmp "$MIN_FREE_INODES"
}

preflight_warp_install_space() {
  local install_required var_required
  estimate_apt_space
  install_required=$((APT_INSTALL_MB + 200))
  var_required=$((APT_DOWNLOAD_MB + 200))
  if [ "$install_required" -lt "$MIN_WARP_FREE_MB" ]; then
    install_required="$MIN_WARP_FREE_MB"
  fi
  if [ "$var_required" -lt "$MIN_APT_CACHE_FREE_MB" ]; then
    var_required="$MIN_APT_CACHE_FREE_MB"
  fi

  log "disk preflight: apt download=${APT_DOWNLOAD_MB}MB, install=${APT_INSTALL_MB}MB, requiring /usr ${install_required}MB, /var ${var_required}MB, /tmp ${MIN_TMP_FREE_MB}MB"
  check_free_mb /usr "$install_required" "package unpack/install"
  check_free_mb /var "$var_required" "apt package cache"
  check_free_mb /tmp "$MIN_TMP_FREE_MB" "temporary files"
  check_free_inodes /usr "$MIN_FREE_INODES"
  check_free_inodes /var "$MIN_FREE_INODES"
  check_free_inodes /tmp "$MIN_FREE_INODES"
}

warp_installed() {
  command -v warp-cli >/dev/null 2>&1
}

repo_tools_ready() {
  command -v curl >/dev/null 2>&1 &&
    command -v gpg >/dev/null 2>&1 &&
    [ -r /etc/ssl/certs/ca-certificates.crt ]
}

ensure_repo_tools_debian_like() {
  if repo_tools_ready; then
    return 0
  fi

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install WARP repository tools: curl ca-certificates gpg"
    return 0
  fi

  preflight_basic_disk_space
  log "installing WARP repository tools"
  env DEBIAN_FRONTEND=noninteractive apt-get update
  if ! env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates gpg; then
    log "warning: failed to install package 'gpg', retrying with 'gnupg'"
    env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates gnupg
  fi
  cleanup_package_cache

  require_command curl
  require_command gpg
  if [ ! -r /etc/ssl/certs/ca-certificates.crt ]; then
    fail "ca-certificates bundle not found after installing repository tools"
  fi
}

install_cloudflare_repo_debian_like() {
  local keyring="/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg"
  local list="/etc/apt/sources.list.d/cloudflare-client.list"
  local codename

  codename="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME:-}")"
  if [ -z "$codename" ]; then
    fail "cannot detect Debian/Ubuntu codename for Cloudflare WARP repository"
  fi

  log "installing Cloudflare WARP apt repository"
  if [ "$POLICY" = "reinstall" ] && command -v apt-get >/dev/null 2>&1; then
    log "removing existing Cloudflare WARP package before reinstall"
    run env DEBIAN_FRONTEND=noninteractive apt-get remove -y cloudflare-warp || true
  fi

  if warp_installed && [ "$POLICY" != "reinstall" ]; then
    log "Cloudflare WARP is already installed; reusing existing installation"
    return 0
  fi

  if [ "$POLICY" = "reuse" ]; then
    fail "Cloudflare WARP is not installed and warp_install policy is reuse"
  fi

  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: install Cloudflare WARP apt repository and package"
    return 0
  fi

  ensure_repo_tools_debian_like
  curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor -o "$keyring"
  echo "deb [signed-by=$keyring] https://pkg.cloudflareclient.com/ $codename main" >"$list"

  env DEBIAN_FRONTEND=noninteractive apt-get update
  preflight_warp_install_space
  if ! env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends cloudflare-warp; then
    cleanup_package_cache
    log_disk_status
    fail "Cloudflare WARP package install failed. If the output says 'Disk quota exceeded', free disk space/quota and repair apt with: apt-get clean && dpkg --configure -a && apt-get -f install"
  fi
  cleanup_package_cache
}

register_and_connect() {
  require_command curl
  if [ "$DRY_RUN" = "true" ]; then
    log "dry-run: ensure warp-cli exists"
    log "dry-run: enable/restart warp-svc.service"
    log "dry-run: configure warp-cli registration, proxy mode, and connection"
    return 0
  fi
  require_command warp-cli

  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now warp-svc.service >/dev/null 2>&1 || systemctl restart warp-svc.service || true
  fi

  local registration=""
  registration="$(warp-cli --accept-tos registration show 2>&1 || true)"
  if printf '%s\n' "$registration" | grep -qi 'Missing registration'; then
    log "registering WARP client"
    warp-cli --accept-tos registration new || fail "warp-cli registration new failed"
  else
    log "WARP client already registered"
  fi

  registration="$(warp-cli --accept-tos registration show 2>&1 || true)"
  if printf '%s\n' "$registration" | grep -qi 'Missing registration'; then
    log "registering WARP client"
    warp-cli --accept-tos registration new || fail "warp-cli registration new failed"
  fi

  log "setting WARP proxy mode"
  warp-cli --accept-tos mode proxy || fail "warp-cli mode proxy failed"

  log "connecting WARP"
  warp-cli --accept-tos connect || fail "warp-cli connect failed"

  log "verifying WARP proxy"
  local trace=""
  local attempt
  for attempt in $(seq 1 30); do
    trace="$(curl --max-time 10 --socks5 127.0.0.1:40000 -fsSL https://www.cloudflare.com/cdn-cgi/trace || true)"
    if printf '%s\n' "$trace" | grep -q '^warp=on$'; then
      log "WARP proxy verified on attempt $attempt"
      return 0
    fi
    sleep 2
  done

  printf '%s\n' "$trace" >&2
  fail "WARP verification failed: expected warp=on via 127.0.0.1:40000"
}

main() {
  parse_args "$@"
  case "$POLICY" in
    auto|reuse|reinstall) ;;
    *) fail "unsupported warp install policy: $POLICY, expected auto, reuse, or reinstall" ;;
  esac

  if [ ! -r /etc/os-release ]; then
    fail "/etc/os-release not found"
  fi

  # shellcheck disable=SC1091
  . /etc/os-release

  case "${ID:-}" in
    debian|ubuntu)
      install_cloudflare_repo_debian_like
      ;;
    *)
      fail "unsupported OS for official Cloudflare WARP client: ${ID:-unknown}"
      ;;
  esac

  register_and_connect
  log "Cloudflare WARP installation completed"
}

main "$@"
