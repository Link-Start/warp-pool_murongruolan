# WarpPool

WireGuard-based multi-exit proxy management for small VPS and NAT VPS nodes.

[English](README.md) | [简体中文](README_CN.md)

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Supported Platforms](#supported-platforms)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start: Push Deployment with Direct Mode](#quick-start-push-deployment-with-direct-mode)
- [WARP Mode](#warp-mode)
- [Pull Deployment](#pull-deployment)
- [Deploy Token](#deploy-token)
- [Commands](#commands)
- [Configuration](#configuration)
- [Changelog](#changelog)
- [Release Process](#release-process)
- [Security Notes](#security-notes)
- [Current Limitations](#current-limitations)

---

## Overview

WarpPool lets one main server manage multiple exit nodes. Applications connect to local proxy ports on the main server, and WarpPool routes traffic through WireGuard tunnels to different exit nodes.

Exit mode:

- `direct`: traffic exits through the node's own network.
- `warp`: traffic exits through Cloudflare WARP on the node.

Typical flow:

```text
Application
  -> 127.0.0.1:<local proxy port>
  -> sing-box mixed/http/socks inbound on the main server
  -> WireGuard tunnel
  -> exit node
  -> direct network or Cloudflare WARP
  -> target website
```

---

## Features

- WireGuard tunnel generation and deployment
- SSH Push deployment for exit nodes
- Optional Cloudflare WARP egress
- User-defined local proxy ports
- `socks5`, `http`, and `mixed` local proxy modes
- sing-box config generation and process management
- Clash-compatible YAML export
- Local diagnostics: version, doctor, ping, speedtest, show

---

## Architecture

```text
Main server
  - WarpPool CLI
  - JSON config
  - WireGuard client
  - sing-box local proxy
  - Clash export

Exit node
  - WireGuard server
  - IPv4 forwarding / NAT
  - optional Cloudflare WARP egress runtime
  - no WarpPool Agent
```

---

## Supported Platforms

### Main server

| Platform | Status |
| --- | --- |
| Linux amd64 | Supported |
| Linux arm64 | Supported |

### Exit node

| OS | Status |
| --- | --- |
| Ubuntu 20.04+ | Supported |
| Debian 11+ | Supported |
| Alpine 3.20+ | Supported |

WARP mode is supported on Ubuntu/Debian and Alpine. Ubuntu/Debian use the official Cloudflare WARP client when available. Alpine uses `wgcf` to generate a WARP WireGuard profile and sing-box's embedded WireGuard endpoint, because Cloudflare does not provide first-class Alpine `apk` packages.

Recommended disk size: around 1 GB. The installer is optimized for small disks by installing only required WireGuard tools, avoiding the WireGuard meta package on Debian/Ubuntu, and cleaning package caches after installation steps.

### CPU Architecture

| Architecture | Status | WARP |
| --- | --- | --- |
| amd64 | Supported | Supported |
| arm64 | Supported | Supported |

---

## Requirements

Main server:

- Root permission for the one-line installer
- WireGuard tools for `warppool wg up` (installed automatically by the official installer)
- sing-box installed by the one-line installer or provided manually

Exit node:

- Root SSH access
- `/dev/net/tun`
- IPv4 connectivity
- `apt` or `apk` package manager depending on OS

---

## Installation

One-line installation:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash
```

Or use `curl`:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash
```

The installer will:

1. Ask for the interactive language: Simplified Chinese or English.
2. Detect OS and CPU architecture.
3. Install base dependencies.
4. Download and install the matching WarpPool release package.
5. Install sing-box.
6. Create systemd services.

The selected language is saved to the WarpPool config. Later interactive commands such as `warppool deploy`, `warppool deploy-token`, and `warppool uninstall` use the same language.

Non-interactive installation:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- port=8080 --yes
```

Install a specific version:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- version=v0.1.1
```

---

## Quick Start: Push Deployment with Direct Mode

After installation, deploy an exit node:

```bash
warppool deploy
```

The command asks for node name, exit mode, proxy protocol, local proxy port, SSH host, SSH port, SSH user, the node-side WireGuard listen port, and the public WireGuard endpoint used by the main server. Menu-style fields can be selected with numbers.

The local proxy port must be entered by the user. WarpPool checks for duplicate ports in the config and also checks whether the port is currently occupied on the main server. SSH host has no default value; enter the real reachable IP or domain.

WireGuard ports are split into two values:

- `wg-listen-port`: the WireGuard listen port on the exit node. Default: `51820`.
- `wg-endpoint-port`: the public port used by the main server to connect to the exit node. On NAT VPS nodes, this is often different from the node-side listen port.

Push mode asks for the SSH port, the node-side WireGuard listen port, and the public WireGuard mapped port. NAT nodes commonly use non-standard SSH and UDP mapped ports, so enter the real forwarded ports from your provider.

By default, SSH host key verification is enabled. If the default `known_hosts` file does not exist during interactive deployment, WarpPool asks whether to skip SSH HostKey verification for this deployment. For non-interactive deployment or temporary tests, pass:

```bash
warppool deploy \
  --name nat01 \
  --exit-mode direct \
  --port 10133 \
  --ssh-host 203.0.113.10 \
  --ssh-user root \
  --wg-listen-port 51820 \
  --wg-endpoint-port 30021 \
  --insecure-skip-host-key-check
```

Deployment starts the local proxy service automatically. To start it manually:

```bash
warppool node start nat01
```

Test the proxy:

```bash
curl -x socks5h://127.0.0.1:10133 https://api.ipify.org
```

---

## WARP Mode

Deploy a WARP exit node:

```bash
warppool deploy \
  --name warp01 \
  --exit-mode warp \
  --proxy mixed \
  --port 10134 \
  --ssh-host 203.0.113.11 \
  --ssh-user root \
  --wg-listen-port 51821
```

On Ubuntu/Debian, the installer uses the official Cloudflare WARP client and runs:

```bash
warp-cli --accept-tos registration new
warp-cli --accept-tos mode proxy
warp-cli --accept-tos connect
```

Verification requires:

```text
warp=on
```

from:

```bash
curl --socks5 127.0.0.1:40000 https://www.cloudflare.com/cdn-cgi/trace
```

Current limitation: WARP forwarding is TCP-first. UDP and IPv6 are not promised as complete yet.

On Alpine, WARP mode uses:

```text
wgcf generated WARP WireGuard profile
  -> sing-box embedded WireGuard endpoint
  -> endpoint probing: IPv6 first, IPv4 fallback, domain fallback
```

The installer verifies WARP by checking `warp=on`. If all endpoint candidates fail, it reports the tried path and asks you to check IPv6 connectivity, UDP 2408 outbound access, DNS, or provider WARP restrictions.

WARP mode is optimized for 1 GB-class small disk nodes. WarpPool installs WARP-specific tools only when WARP mode is selected, uses `--no-install-recommends` where possible, cleans package caches after each step, and warns instead of blocking when disk space is below the recommended threshold but above the hard minimum.

---

## Pull Deployment

Recommended flow: run `warppool deploy-token` on the main server first, then copy the generated one-line install command to the exit node. This makes the main server the source of truth for node name, exit mode, local proxy protocol, and local proxy port. The exit node only provides node-side WireGuard/NAT endpoint information.

If you run the node installer directly on the exit node:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash
```

The script enters a manual setup menu:

1. Enter the main server IP/domain. Press Enter to install node dependencies only.
2. If a main server address is entered, enter the registration port. IPv4 defaults to `8080`; domains default to `80`.
3. Enter a Deploy Token if auto registration is needed.
4. For auto registration, enter the node-side WireGuard listen port and the public UDP port used by the main server.

If the main server IP or Deploy Token is left empty, the script only installs node dependencies. It will not write WireGuard config and will not create a node record on the main server. You can later run `warppool deploy-token` on the main server and execute the generated one-line command on the node.

For dependency-only installation, pass the exit mode to decide whether WARP should be installed:

```bash
# direct mode installs WireGuard and base dependencies only
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct

# WARP mode installs the WARP runtime for the node OS
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=warp
```

For auto registration, normally use the command printed by `warppool deploy-token`:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- token=<token> server=http://<main-server-ip>:8080
```

The node first reads the exit mode stored in the Deploy Token from the main server, then decides whether WARP should be installed.

---

## Deploy Token

The one-line installer configures the registration listener port. Start the listener only when you need Deploy Token registration:

```bash
warppool listen start
```

Generate token command:

```bash
warppool deploy-token
```

The command asks for node name, exit mode, proxy protocol, and local proxy port. It then prints the Deploy Token plus a one-line node installation command. The exit node uses that command to request WireGuard config, start WireGuard, and complete registration. After registration, the main server starts the local proxy service automatically.

To avoid duplicate configuration, Deploy Token uses these sources of truth:

- Main server: node name, exit mode, proxy protocol, local proxy port.
- Exit node: node-side WireGuard listen port, auto-detected or manually entered public endpoint, and NAT-mapped public UDP port.

For NAT VPS nodes where the public UDP mapping differs from the node-side WireGuard listen port, run the generated install command on the exit node and enter the provider-mapped public UDP port when prompted.

---

## Commands

### Node

```bash
warppool node list # List nodes
warppool node show nat01 # Show node nat01 details and runtime status
warppool node start nat01 # Start local proxy service for nat01 and enable autostart
warppool node stop nat01 # Stop local proxy service
warppool node status nat01 # Show node nat01 runtime status
warppool node mode nat01 warp # Switch nat01 to WARP egress; auto-detect and install/reuse WARP
warppool node mode nat01 direct # Switch nat01 back to direct egress
warppool remove nat01 # Remove node nat01 record only
warppool node remove nat01 --clean-wg # Remove node nat01 and delete local WG client config
```

`remove` only removes the node record. Add `--clean-wg` when you also want to stop and delete the local WireGuard client config on the main server.

`warppool node mode` defaults to Pull mode and prints a command to run on the exit node. The exit node detects WARP automatically: reuse it when already installed, otherwise install it. Advanced options:

```bash
warppool node mode nat01 warp --warp-install reuse # Reuse existing WARP only; fail if missing
warppool node mode nat01 warp --warp-install reinstall # Force reinstall WARP
warppool node mode nat01 direct --remove-warp # Remove WARP after switching back to direct
warppool node mode nat01 warp --method ssh # Switch automatically over SSH
```

Pull mode first reads `/etc/warppool-node/state.json` on the exit node, so normally you do not need to enter the main server address again. For old nodes without this state file, the script prompts for the server address, or you can use the fallback command printed by the main server with `server=http://<main-server-ip>:<port>`.

SSH mode reuses the non-sensitive SSH connection information saved during Push deployment, including SSH host, port, user, SSH key path, known_hosts path, and host-key-check preference. SSH passwords are never saved. During interactive mode switching, saved SSH host, port, and user are shown as defaults; press Enter to reuse them or type new values to override.

### WireGuard

```bash
warppool wg config nat01 # Print local WireGuard client config for nat01
warppool wg up nat01 # Start system WireGuard client for diagnostics
warppool wg status nat01 # Show local WireGuard status for nat01
warppool wg down nat01 # Stop local WireGuard client for nat01
```

### Proxy

```bash
warppool proxy config -o sing-box.json # Generate sing-box config
warppool proxy start # Start local proxy as a temporary process
warppool proxy service install # Create local proxy systemd service
warppool proxy service enable # Start local proxy for all nodes and enable autostart
warppool proxy status # Show local proxy status
warppool proxy stop # Stop the temporary local proxy process
```

### Clash Export

```bash
warppool export clash -o clash.yaml # Export Clash-compatible config
```

### Diagnostics

```bash
warppool version # Show version information
warppool doctor # Check local runtime and port status
warppool ping nat01 # Test node public endpoint latency, direct HTTP latency, and proxy egress IP/latency
warppool upgrade --yes # Upgrade binary and bundled installer assets
warppool speedtest --proxy http://127.0.0.1:10133 # Run a simple speed test through a proxy
```

On an exit node, WARP backend probing can be run manually for diagnostics:

```bash
bash /path/to/warp_forward.sh action=probe device=wpnat01 client_addr=10.200.0.2/32 server_addr=10.200.0.1/32 backend=wireguard
```

`warppool ping` uses multiple fallback HTTP check URLs by default:

```text
https://api.ipify.org
https://icanhazip.com
https://ifconfig.me/ip
```

Custom URLs can be supplied as a comma-separated list:

```bash
warppool ping nat01 --url https://api.ipify.org,https://icanhazip.com
```

Note: `speedtest` is safest with HTTP proxy URLs. Full SOCKS proxy handling is planned.

### Uninstall

```bash
warppool uninstall --force # Uninstall WarpPool program and runtime state from the main server
```

`uninstall` is only for uninstalling the main server program. It asks whether to remove local WireGuard client configs and whether to remove local proxy/listener services plus runtime state. To remove a node, use `warppool remove <name>`.

### Remote Node Uninstall

Push deployment installs a helper command on the exit node:

```bash
warppool-node-uninstall
```

Common usage on the exit node:

```bash
warppool-node-uninstall device=wpnat01 # Remove one WarpPool WireGuard device
warppool-node-uninstall all=true # Remove all WarpPool WireGuard devices on this node
warppool-node-uninstall all=true remove_warp=true # Also remove/clean WARP runtime
warppool-node-uninstall all=true remove_wireguard=true # Also remove WireGuard packages
```

If exactly one `/etc/wireguard/wp*.conf` file exists, `warppool-node-uninstall` can run without arguments. If multiple WarpPool devices exist, pass `device=<wg-device>` or `all=true`.

For Pull-only nodes where the helper was not installed, run:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/node_uninstall.sh | sudo bash -s -- all=true
```

---

## Configuration

Default config path:

| OS | Path |
| --- | --- |
| Linux | `/etc/warppool/config.json` |

`warppool config init` creates this JSON config file with default values. It stores listener settings, default proxy settings, node metadata, deploy tokens, and WireGuard client private keys. The one-line installer runs it automatically when the config file does not exist.

Keep the config file private.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

---

## Release Process

Release builds only run when tags are pushed. Normal branch pushes do not create GitHub Releases.

Published packages:

- `warppool-linux-amd64.tar.gz`
- `warppool-linux-arm64.tar.gz`

`VERSION` contains one line:

```text
0.1.1
```

From the `developer` branch:

```powershell
.\scripts\release.ps1 patch
git push origin developer v0.1.1
```

The script checks the working tree, updates `VERSION`, creates a Chinese commit, and creates an annotated tag. GitHub Actions verifies that the tag equals `v<VERSION>`.

---

## Security Notes

- Keep `/etc/warppool/config.json` private.
- Do not commit SSH passwords, WireGuard private keys, or local runtime files.
- Prefer SSH keys or interactive password input.
- `--insecure-skip-host-key-check` is for temporary testing only.
- Local proxy ports bind to `127.0.0.1` by default.

---

## Current Limitations

- No Web UI.
- No database.
- No multi-user permission model.
- No remote Agent.
- `upgrade` updates the main binary and bundled installer assets. Existing config is preserved.
