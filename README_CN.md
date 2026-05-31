# WarpPool

基于 WireGuard 的多出口代理管理工具，面向小型 VPS / NAT VPS 节点。

[English](README.md) | [简体中文](README_CN.md)

> WarpPool 当前处于 MVP 阶段。如果主服务器可以 SSH 到出口节点，推荐使用 Push 部署；如果希望出口节点主动拉取配置，推荐使用 Deploy Token。

## 目录

- [项目概述](#项目概述)
- [核心功能](#核心功能)
- [架构](#架构)
- [支持平台](#支持平台)
- [环境要求](#环境要求)
- [安装](#安装)
- [快速开始：Push 部署 direct 节点](#快速开始push-部署-direct-节点)
- [WARP 模式](#warp-模式)
- [Pull 部署](#pull-部署)
- [Deploy Token](#deploy-token)
- [常用命令](#常用命令)
- [配置](#配置)
- [发布流程](#发布流程)
- [安全说明](#安全说明)
- [当前限制](#当前限制)

---

## 项目概述

WarpPool 让一台主服务器统一管理多台出口节点。应用程序连接主服务器上的本地代理端口，WarpPool 通过 WireGuard 隧道把流量送到不同出口节点。

出口模式：

- `direct`：流量直接从出口节点本机网络出去。
- `warp`：流量通过出口节点上的 Cloudflare 官方 WARP 客户端出去。

典型流量路径：

```text
应用程序
  -> 127.0.0.1:<本地代理端口>
  -> 主服务器 sing-box mixed/http/socks 入站
  -> WireGuard 隧道
  -> 出口节点
  -> direct 本机网络 或 Cloudflare WARP
  -> 目标网站
```

---

## 核心功能

- 自动生成和部署 WireGuard 隧道配置
- 通过 SSH Push 部署出口节点
- 可选 Cloudflare WARP 出口
- 用户自定义本地代理端口
- 支持 `socks5`、`http`、`mixed` 本地代理模式
- 生成 sing-box 配置并管理进程
- 导出 Clash 兼容 YAML
- 本地诊断命令：version、doctor、ping、speedtest、show

---

## 架构

```text
主服务器
  - WarpPool CLI
  - JSON 配置
  - WireGuard 客户端
  - sing-box 本地代理
  - Clash 导出

出口节点
  - WireGuard 服务端
  - IPv4 转发 / NAT
  - 可选 Cloudflare 官方 WARP 客户端
  - 不运行 WarpPool Agent
```

---

## 支持平台

### 主服务器

| 平台 | 状态 |
| --- | --- |
| Linux amd64 | 支持 |
| Linux arm64 | 支持 |

### 出口节点

| 系统 | 状态 |
| --- | --- |
| Ubuntu 20.04+ | 支持 |
| Debian 12+ | 支持 |
| Alpine 3.20+ | 支持 |

WARP 模式依赖 Cloudflare 官方 Linux 客户端包。内置安装脚本不支持 Alpine 的 WARP 模式。

### CPU 架构

| 架构 | 状态 | WARP |
| --- | --- | --- |
| amd64 | 支持 | Cloudflare 有包时支持 |
| arm64 | 支持 | Cloudflare 有包时支持 |

---

## 环境要求

主服务器：

- 推荐 Linux
- 一键安装脚本需要 root 权限
- 执行 `warppool wg up` 需要 WireGuard tools
- sing-box 可由一键安装脚本安装，也可以用户手动提供

出口节点：

- root SSH 权限
- `/dev/net/tun`
- IPv4 网络
- WireGuard UDP 端口已放通
- 根据系统使用 `apt` 或 `apk` 安装依赖

---

## 安装
一行命令安装
```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash
```
或者使用 `curl`：
```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash
```
安装脚本会：

1. 检测系统和 CPU 架构。
2. 安装基础依赖。
3. 下载并安装匹配的 WarpPool Release 包。
4. 安装 sing-box。
5. 创建 systemd 服务。


非交互安装

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- port=8080 --yes
```

安装指定版本：

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- version=v0.1.0
```

---

## 快速开始：Push 部署 direct 节点

安装完成后，部署出口节点：

```bash
warppool deploy
```

命令会依次询问节点名称、出口模式、代理协议、本地代理端口、SSH 地址、SSH 端口、SSH 用户、节点本机 WireGuard 监听端口，以及主服务器实际连接的 WireGuard 公网端点。出口模式和代理协议这类选项可以直接输入数字选择。

本地代理端口必须由用户填写，WarpPool 会检查配置内是否重复，并检查当前主服务器端口是否已被占用。SSH 地址没有默认值，必须填写真实可连接的 IP 或域名。

WireGuard 端口分两类：

- `wg-listen-port`：出口节点本机 WireGuard 监听端口，默认 `51820`。
- `wg-endpoint-port`：主服务器连接出口节点时使用的公网端口。NAT VPS 做端口转发时，这个端口通常和节点本机监听端口不同。

默认开启 SSH HostKey 校验。临时测试时可以显式跳过：

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

启动主服务器本地 WireGuard 客户端：

```bash
warppool wg up nat01
```

启动本地代理：

```bash
warppool proxy start
```

测试代理：

```bash
curl -x socks5h://127.0.0.1:10133 https://api.ipify.org
```

---

## WARP 模式

部署 WARP 出口节点：

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

安装脚本会使用 Cloudflare 官方 WARP 客户端，并执行：

```bash
warp-cli --accept-tos registration new
warp-cli --accept-tos mode proxy
warp-cli --accept-tos connect
```

验证结果必须包含：

```text
warp=on
```

验证命令：

```bash
curl --socks5 127.0.0.1:40000 https://www.cloudflare.com/cdn-cgi/trace
```

MVP 限制：当前 WARP 转发以 TCP 为主，暂不承诺 UDP / IPv6 完整支持。

---

## Pull 部署

可以直接在节点上执行安装脚本：

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct
```

WARP 模式：

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=warp
```

纯 Pull 模式用于安装节点依赖。如果需要自动获取 WireGuard 配置并注册回主服务器，请使用 Deploy Token。

---

## Deploy Token

一键安装脚本会配置注册监听端口。仅在需要 Deploy Token 注册时启动监听：

```bash
warppool listen start
```

生成安装命令：

```bash
warppool deploy-token
```

命令会询问节点名称、出口模式、代理协议和本地代理端口，然后输出一行节点安装命令。出口节点执行该命令后，会向主服务器请求 WireGuard 配置、启动 WireGuard，并完成注册。

如果出口节点是 NAT VPS，并且公网 UDP 端口映射和节点本机 WireGuard 监听端口不同，生成命令时可以传：

```bash
warppool deploy-token --wg-listen-port 51820 --wg-endpoint-port 30021
```

节点执行的一行安装命令会携带 `wg_listen_port` 和 `wg_endpoint_port`。节点本机继续监听 `51820`，主服务器 WireGuard 客户端连接公网端点的 `30021`。

---

## 常用命令

### 节点

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

### 代理

```bash
warppool proxy config -o sing-box.json
warppool proxy start
warppool proxy status
warppool proxy stop
```

### Clash 导出

```bash
warppool export clash -o clash.yaml
```

### 诊断

```bash
warppool version
warppool doctor
warppool ping nat01
warppool speedtest --proxy http://127.0.0.1:10133
```

MVP 说明：`speedtest` 当前使用 HTTP proxy URL 最稳。完整 SOCKS 代理测速会在稳定版前补齐。

---

## 配置

默认配置路径：

| 系统 | 路径 |
| --- | --- |
| Linux | `/etc/warppool/config.json` |

`warppool config init` 会创建这个 JSON 配置文件，并写入默认配置。它保存监听设置、默认代理设置、节点信息、Deploy Token 和 WireGuard 客户端私钥。一键安装脚本会在配置文件不存在时自动执行它。

请妥善保护配置文件。

---

## 安全说明

- 妥善保护 `/etc/warppool/config.json`。
- 不要把 SSH 密码、WireGuard 私钥、本地运行状态提交到 Git。
- 优先使用 SSH key 或交互式密码输入。
- `--insecure-skip-host-key-check` 仅用于临时测试。
- 本地代理端口默认监听 `127.0.0.1`。

---

## 当前限制

- 没有 Web UI。
- 没有数据库。
- 没有多用户权限系统。
- 不在远端运行 Agent。
- `upgrade` 和 `uninstall` 在 MVP 阶段是安全占位命令。
- sing-box 服务守护尚未最终完成，重启后需要重新执行 `warppool proxy start`。
- `warppool show` 输出脱敏会在稳定版前补齐。
