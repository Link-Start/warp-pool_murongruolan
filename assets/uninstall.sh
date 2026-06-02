#!/usr/bin/env bash
set -Eeuo pipefail

if command -v warppool >/dev/null 2>&1; then
  exec warppool uninstall "$@"
fi

if command -v wpl >/dev/null 2>&1; then
  exec wpl uninstall "$@"
fi

if [ -x /usr/local/bin/warppool ]; then
  exec /usr/local/bin/warppool uninstall "$@"
fi

if [ -x /usr/local/bin/wpl ]; then
  exec /usr/local/bin/wpl uninstall "$@"
fi

printf '[WarpPool][uninstall][ERROR] warppool command not found; remove /etc/warppool and /usr/local/lib/warppool manually if only leftovers remain\n' >&2
exit 1
