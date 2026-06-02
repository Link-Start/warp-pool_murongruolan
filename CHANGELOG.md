# Changelog

## v0.1.9

- Added Alpine WARP support based on `wgcf` generated WireGuard profiles and sing-box embedded WireGuard endpoints.
- Alpine WARP endpoint probing now prefers IPv6, falls back to IPv4, and finally falls back to the original domain.
- Alpine sing-box installation now prefers the Alpine package repository with `apk update && apk add --no-cache sing-box`.
- Added automatic fallback to the GitHub musl sing-box build when the Alpine package is missing, cannot run, or cannot load WarpPool's generated WARP config.
- Fixed failed Alpine WARP deployment caused by downloading a non-musl sing-box binary.

## v0.1.8

- Officially optimized WARP mode for 1 GB-class small disk exit nodes. Debian/Ubuntu installers now avoid the `wireguard` meta package, install only required WireGuard tools, use `--no-install-recommends` where possible, and clean package caches after install steps.
- Fixed WARP installation after lightweight dependency changes by installing `gpg` only when WARP mode needs the Cloudflare apt repository.
- Relaxed WARP disk preflight for small NAT VPS nodes: low disk space now shows a warning when it is above the hard minimum instead of blocking too early.
- `warppool node mode --method ssh` now reuses saved non-sensitive SSH defaults from Push deployment. SSH passwords are never saved.
- `warppool ping` now reports node latency target RTT, main-server direct HTTP latency, proxy egress IP, and proxy HTTP latency, with multiple fallback check URLs.

## v0.1.5

- Fixed node mode switch language inheritance. Pull-installed nodes now save the selected language, and `node_mode.sh` reads it for later direct/WARP switches.
- Added clearer one-time token expiry warnings for Deploy Token and node mode switch commands.
- Added Cloudflare WARP disk and inode preflight checks before installing the official WARP client, with clearer recovery guidance when `apt` fails because of low disk quota.
- Fixed SSH Push deployment for non-root users by automatically using `sudo` for privileged remote operations when available.
