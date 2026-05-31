# WarpPool

WarpPool is a CLI tool for managing WireGuard-based exit nodes. A main server keeps local proxy ports, WireGuard client configs, diagnostics, and Clash exports. Exit nodes run WireGuard server configs and optional Cloudflare WARP forwarding.

Cloudflare WARP is optional. The default and recommended MVP path is `direct` mode.

## Status

WarpPool is in MVP stage.

Recommended for current use:

- Push deployment over SSH
- `direct` mode
- Linux main server
- Local proxy via sing-box `mixed` inbound

Not yet equal to Push deployment:

- Pull deployment
- Deploy Token deployment

These flows exist as installation and registration foundations, but the full WireGuard registration chain is still planned.

## Supported release packages

The release workflow builds:

- `warppool-linux-amd64.tar.gz`
- `warppool-linux-arm64.tar.gz`
- `warppool-windows-amd64.zip`
- `warppool-windows-arm64.zip`

ARM64 is supported for WarpPool itself, WireGuard packages, and sing-box auto-download. ARM32 is not supported in V1.

## Requirements

Main server:

- Linux is recommended for real traffic forwarding.
- Windows can generate config and run some local commands, but `wg-quick` management is Linux-focused.
- WireGuard tools installed locally when running `warppool wg up`.
- sing-box installed or available in a bundled `bin/` directory.

Exit node:

- Debian 12+
- Ubuntu 20.04+
- Alpine 3.20+
- root SSH access for Push deployment
- `/dev/net/tun`
- IPv4 connectivity

WARP mode:

- Uses the official Cloudflare WARP Linux client.
- Supported only when Cloudflare packages are available for the target OS and CPU.
- Alpine WARP mode is not supported by the built-in installer.
- Current WARP forwarding is TCP-first; UDP and IPv6 are not promised as complete in MVP.

## Quick start: direct mode with Push deployment

Initialize config:

```bash
warppool config init
```

Deploy an exit node:

```bash
warppool deploy \
  --name nat01 \
  --exit-mode direct \
  --proxy mixed \
  --port 10133 \
  --ssh-host 203.0.113.10 \
  --ssh-user root \
  --wg-listen-port 51820
```

By default, SSH host key verification is enabled. For temporary tests only:

```bash
warppool deploy \
  --name nat01 \
  --exit-mode direct \
  --port 10133 \
  --ssh-host 203.0.113.10 \
  --ssh-user root \
  --insecure-skip-host-key-check
```

Start the local WireGuard client on the main server:

```bash
warppool wg up nat01
```

Install sing-box on the main server if needed:

```bash
sudo bash assets/singbox_install.sh --yes source=default
```

Start local proxy ports:

```bash
warppool proxy start
```

Test the local proxy:

```bash
curl -x socks5h://127.0.0.1:10133 https://api.ipify.org
```

## WARP mode

WARP mode installs and configures the official Cloudflare WARP client on the exit node:

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

The installer runs:

```bash
warp-cli --accept-tos registration new
warp-cli --accept-tos mode proxy
warp-cli --accept-tos connect
```

It verifies:

```bash
curl --socks5 127.0.0.1:40000 https://www.cloudflare.com/cdn-cgi/trace
```

The result must include:

```text
warp=on
```

If WARP fails, use `direct` mode or a Cloudflare-supported OS.

## Node management

List nodes:

```bash
warppool node list
```

Show a node:

```bash
warppool show nat01
```

Print WireGuard client config:

```bash
warppool wg config nat01
```

Stop local WireGuard:

```bash
warppool wg down nat01
```

Remove local node metadata:

```bash
warppool node remove nat01
```

## Proxy management

Generate sing-box config:

```bash
warppool proxy config -o sing-box.json
```

Start:

```bash
warppool proxy start
```

Status:

```bash
warppool proxy status
```

Stop:

```bash
warppool proxy stop
```

The default local proxy protocol is `mixed`, so one port supports HTTP proxy and SOCKS5.

## Clash export

```bash
warppool export clash -o clash.yaml
```

For mixed ports, Clash can use the port as either `socks5` or `http`. WarpPool exports `socks5` by default.

## Diagnostics

Show version:

```bash
warppool version
```

Check local dependencies and ports:

```bash
warppool doctor
```

Ping WireGuard peer addresses:

```bash
warppool ping nat01
```

Run a light download test:

```bash
warppool speedtest --proxy http://127.0.0.1:10133
```

MVP note: `speedtest` is safest with HTTP proxy URLs. Full SOCKS proxy handling is planned before stable release.

## Pull deployment

The installation scripts can be used directly on a node:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | bash -s -- mode=direct
```

For WARP mode:

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | bash -s -- mode=warp
```

MVP note: Pull deployment installs dependencies and can register basic token data, but Push deployment is the complete recommended flow for generating and applying WireGuard configs.

## Deploy Token

Configure and start the registration listener:

```bash
warppool listen config --host 0.0.0.0 --port 18080 --public-host <main-server-ip>
warppool listen start
```

Generate an install command:

```bash
warppool deploy-token --name nat02 --port 10135 --exit-mode direct
```

MVP note: Deploy Token is currently a registration foundation. Use Push deployment when you need the full WireGuard setup to be applied automatically.

## sing-box installation

Use the built-in GitHub release URL:

```bash
sudo bash assets/singbox_install.sh --yes source=default
```

Use a custom URL:

```bash
sudo bash assets/singbox_install.sh source=custom url=https://example.com/sing-box-linux-amd64.tar.gz
```

Use an existing binary:

```bash
bash assets/singbox_install.sh source=existing
```

The default install path is:

```text
/usr/local/lib/warppool/bin/sing-box
```

## Release process

Release builds only run when a tag matching `v*.*.*` is pushed.

The version file contains a single line:

```text
0.1.0
```

Use PowerShell from the `developer` branch:

```powershell
.\scripts\release.ps1 patch
```

The script:

1. Checks that the current branch is `developer`.
2. Checks that the working tree is clean.
3. Bumps `VERSION`.
4. Ensures the new tag does not exist locally or remotely.
5. Commits with a Chinese release message.
6. Creates an annotated tag like `v0.1.1`.
7. Pushes `developer` and the tag.

This repository intentionally does not create releases from normal branch pushes.

## Security notes

- Keep `/etc/warppool/config.json` private. It can contain WireGuard client private keys.
- Do not put SSH passwords in scripts or Git.
- Prefer SSH keys or interactive password input.
- `--insecure-skip-host-key-check` is for temporary testing only.
- Local proxy ports bind to `127.0.0.1` by default.

## Current limitations

- No Web UI.
- No database.
- No multi-user permission model.
- No remote Agent.
- `upgrade` and `uninstall` are safe placeholder commands in MVP.
- Automatic sing-box service persistence is not finalized; restart `warppool proxy start` after reboot if needed.
