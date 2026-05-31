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
- [Release Process](#release-process)
- [Security Notes](#security-notes)
- [Current Limitations](#current-limitations)

---

## Overview

WarpPool lets one main server manage multiple exit nodes. Applications connect to local proxy ports on the main server, and WarpPool routes traffic through WireGuard tunnels to different exit nodes.

Exit mode:

- `direct`: traffic exits through the node's own network.
- `warp`: traffic exits through the official Cloudflare WARP client running on the node.

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
  - optional official Cloudflare WARP client
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
| Debian 12+ | Supported |
| Alpine 3.20+ | Supported |

WARP mode depends on Cloudflare's official Linux client packages. Alpine WARP mode is not supported by the built-in installer.

### CPU Architecture

| Architecture | Status | WARP |
| --- | --- | --- |
| amd64 | Supported | Supported when Cloudflare package exists |
| arm64 | Supported | Supported when Cloudflare package exists |

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
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- version=v0.1.0
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

Start the local WireGuard client:

```bash
warppool wg up nat01
```

Start local proxy:

```bash
warppool proxy service enable
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

The installer uses the official Cloudflare WARP client and runs:

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

---

## Pull Deployment

Pull installation scripts are available:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash
```

The script enters an interactive menu:

1. Select exit mode. Default: `direct`.
2. Enter the main server IP/domain. Press Enter to skip auto registration.
3. If a main server address is entered, enter the registration port. IPv4 defaults to `8080`; domains default to `80`.
4. Enter a Deploy Token if auto registration is needed.

If the main server IP or Deploy Token is left empty, the script only installs node dependencies. It will not write WireGuard config and will not create a node record on the main server. You can later run `warppool deploy-token` on the main server and execute the generated one-line command on the node.

Non-interactive direct mode:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct
```

Non-interactive WARP mode:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=warp
```

Auto-register with Deploy Token:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct token=<token> server=http://<main-server-ip>:8080
```

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

The command will ask for node name, exit mode, proxy protocol, and local proxy port. It then prints a one-line node installation command. The exit node uses that command to request WireGuard config, start WireGuard, and complete registration.

For NAT VPS nodes where the public UDP mapping differs from the node-side WireGuard listen port, generate the command with:

```bash
warppool deploy-token --wg-listen-port 51820 --wg-endpoint-port 30021
```

The generated node installer command will include `wg_listen_port` and `wg_endpoint_port`. The node listens on `51820`, while the main server connects to the public endpoint port `30021`.

---

## Commands

### Node

```bash
warppool node list # List nodes
warppool show nat01 # Show node nat01 details
warppool remove nat01 # Remove node nat01 record only
warppool node remove nat01 --clean-wg # Remove node nat01 and delete local WG client config
```

`remove` only removes the node record. Add `--clean-wg` when you also want to stop and delete the local WireGuard client config on the main server.

### WireGuard

```bash
warppool wg config nat01 # Print local WireGuard client config for nat01
warppool wg up nat01 # Start local WireGuard client for nat01
warppool wg status nat01 # Show local WireGuard status for nat01
warppool wg down nat01 # Stop local WireGuard client for nat01
```

### Proxy

```bash
warppool proxy config -o sing-box.json # Generate sing-box config
warppool proxy start # Start local proxy as a temporary process
warppool proxy service install # Create local proxy systemd service
warppool proxy service enable # Start local proxy and enable autostart
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
warppool ping nat01 # Test WireGuard connectivity to nat01
warppool speedtest --proxy http://127.0.0.1:10133 # Run a simple speed test through a proxy
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
warppool-node-uninstall all=true remove_warp=true # Also remove Cloudflare WARP package
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

## Release Process

Release builds only run when tags are pushed. Normal branch pushes do not create GitHub Releases.

Published packages:

- `warppool-linux-amd64.tar.gz`
- `warppool-linux-arm64.tar.gz`

`VERSION` contains one line:

```text
0.1.0
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
- `upgrade` is currently a safe placeholder command.
