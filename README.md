# WarpPool

WireGuard-based multi-exit proxy management for small VPS and NAT VPS nodes.

[English](README.md) | [简体中文](README_CN.md)

> WarpPool is an MVP-stage CLI project. Push deployment is recommended when the main server can SSH into the exit node. Deploy Token is recommended when the exit node should pull its configuration from the main server.

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

- Linux recommended
- Root permission for the one-line installer
- WireGuard tools for `warppool wg up`
- sing-box installed by the one-line installer or provided manually

Exit node:

- Root SSH access
- `/dev/net/tun`
- IPv4 connectivity
- UDP port allowed for WireGuard
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

1. Detect OS and CPU architecture.
2. Install base dependencies.
3. Download and install the matching WarpPool release package.
4. Install sing-box.
5. Create systemd services.

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

By default, SSH host key verification is enabled. For temporary tests only:

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
warppool proxy start
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

MVP limitation: WARP forwarding is TCP-first. UDP and IPv6 are not promised as complete yet.

---

## Pull Deployment

Pull installation scripts are available:

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct
```

WARP mode:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=warp
```

Pure Pull mode installs node dependencies. To automatically receive WireGuard config and register back to the main server, use Deploy Token.

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
warppool node list
warppool show nat01
warppool node remove nat01
```

### WireGuard

```bash
warppool wg config nat01
warppool wg up nat01
warppool wg status nat01
warppool wg down nat01
```

### Proxy

```bash
warppool proxy config -o sing-box.json
warppool proxy start
warppool proxy status
warppool proxy stop
```

### Clash Export

```bash
warppool export clash -o clash.yaml
```

### Diagnostics

```bash
warppool version
warppool doctor
warppool ping nat01
warppool speedtest --proxy http://127.0.0.1:10133
```

MVP note: `speedtest` is safest with HTTP proxy URLs. Full SOCKS proxy handling is planned before stable release.

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

Release builds only run on tags matching:

```text
v*.*.*
```

`VERSION` contains one line:

```text
0.1.0
```

From the `developer` branch:

```powershell
.\scripts\release.ps1 patch
```

The script:

1. Checks the current branch is `developer`.
2. Checks the working tree is clean.
3. Bumps `VERSION`.
4. Ensures the new tag does not exist locally or remotely.
5. Commits with a Chinese release message.
6. Creates an annotated tag like `v0.1.1`.
7. Pushes `developer` and the tag.

Normal branch pushes do not create GitHub Releases.

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
- `upgrade` and `uninstall` are safe placeholder commands in MVP.
- sing-box service persistence is not finalized; restart `warppool proxy start` after reboot if needed.
- `warppool show` output hardening is planned before stable release.
