# WarpPool

基于 WireGuard 的多出口代理管理工具，面向小型 VPS / NAT VPS 节点。

[English](README.md) | [简体中文](README_CN.md)

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
- [更新日志](#更新日志)
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
| Debian 11+ | 支持 |
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

- 一键安装脚本需要 root 权限
- 执行 `warppool wg up` 需要 WireGuard tools(使用脚本会默认安装)
- sing-box 可由一键安装脚本安装，也可以用户手动提供

出口节点：

- root SSH 权限
- `/dev/net/tun`
- IPv4 网络
- 根据系统使用 `apt` 或 `apk` 安装依赖

---

## 安装
一行命令安装
```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash

# 或使用wget
wget -qO- ···
```
安装脚本会：

1. 询问交互语言：简体中文或 English。
2. 检测系统和 CPU 架构。
3. 安装基础依赖。
4. 下载并安装匹配的 WarpPool Release 包。
5. 安装 sing-box。
6. 创建 systemd 服务。

安装时选择的语言会写入 WarpPool 配置。后续 `warppool deploy`、`warppool deploy-token`、`warppool uninstall` 等交互命令会使用同一种语言。


非交互安装

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- port=8080 --yes

# 安装指定版本：
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install_server.sh | sudo bash -s -- version=v0.1.1
```
---

## 快速开始：Push 部署 direct 节点

安装完成后，部署出口节点：

```bash
warppool deploy
```

WireGuard 端口分两类：

- `wg-listen-port`：出口节点本机 WireGuard 监听端口，默认 `51820`。
- `wg-endpoint-port`：主服务器连接出口节点时使用的公网端口。NAT VPS 做端口转发时，这个端口通常和节点本机监听端口不同。

默认开启 SSH HostKey 校验。如果交互部署时默认 `known_hosts` 文件不存在，WarpPool 会询问本次部署是否跳过 SSH HostKey 校验。非交互部署或临时测试时可以显式传入：

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

部署成功后会自动启动本地代理服务。也可以手动启动：

```bash
warppool node start nat01
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

验证命令：

```bash
curl --socks5 127.0.0.1:40000 https://www.cloudflare.com/cdn-cgi/trace
```

当前限制：WARP 转发以 TCP 为主，暂不承诺 UDP / IPv6 完整支持。

---

## Pull 部署

推荐先在主服务器执行 `warppool deploy-token`，再把输出的一行安装命令复制到出口节点执行。这样节点名称、出口模式、本地代理协议、本地代理端口都由主服务器确定，出口节点只需要填写本机 WireGuard/NAT 端点信息。

如果直接在出口节点执行安装脚本：

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash
```

脚本会进入手动交互菜单：

1. 询问主服务器 IP/域名，不填将只安装节点依赖。
2. 如果填写了主服务器地址，将询问注册端口。
3. 如需自动注册，需要填写 Deploy Token。
4. 自动注册时会询问本节点 WireGuard 监听端口和主服务器连接本节点的公网 UDP 端口。

如果不填写主服务器 IP/域名 或不填写 Deploy Token，脚本只安装节点依赖，不会写入 WireGuard 配置，也不会在主服务器生成节点记录。后续可以在主服务器执行 `warppool deploy-token`，再把生成的一行命令复制到节点执行。

仅安装节点依赖时可以指定出口模式，用于决定是否安装 WARP：

```bash
# direct 模式，只安装 WireGuard 等基础依赖
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=direct

# WARP 模式，会额外安装 Cloudflare WARP
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- mode=warp
```

携带 Deploy Token 自动注册时，通常直接使用 `warppool deploy-token` 输出的命令：

```bash
wget -qO- https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/install.sh | sudo bash -s -- token=<token> server=http://<主服务器IP>:8080
```

这时节点会先从主服务器读取 Deploy Token 中保存的出口模式，再决定是否安装 WARP。

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

命令会询问节点名称、出口模式、代理协议、本地代理端口，然后输出 Deploy Token 和一行节点安装命令。出口节点执行该命令后，会向主服务器请求 WireGuard 配置、启动 WireGuard，并完成注册。注册完成后主服务器会自动启动本地代理服务。

为避免重复配置，Deploy Token 流程按下面规则决定配置来源：

- 主服务器决定：节点名称、出口模式、代理协议、本地代理端口。
- 出口节点决定：本机 WireGuard 监听端口、自动检测或手动填写的公网端点、NAT 映射后的公网 UDP 端口。

如果出口节点是 NAT VPS，并且公网 UDP 端口映射和节点本机 WireGuard 监听端口不同，在出口节点执行安装命令后，根据提示填写映射出来的公网 UDP 端口即可。

---

## 常用命令

### 节点相关

```bash
warppool node list # 查看节点列表
warppool node show nat01 # 查看节点nat01信息和运行状态
warppool node start nat01 # 启动节点nat01对应的本地代理服务并设置开机自启
warppool node stop nat01 # 停止本地代理服务
warppool node status nat01 # 查看节点nat01运行状态
warppool node mode nat01 warp # 将节点nat01切换为WARP出口，默认自动检测并安装/复用WARP
warppool node mode nat01 direct # 将节点nat01切换回直连出口
warppool remove nat01 # 移除节点nat01 仅移除记录
warppool node remove nat01 --clean-wg # 移除节点nat01 并WG删除客户端配置
```

`warppool node mode` 默认使用 Pull 方式生成一条需要在出口节点执行的命令。出口节点会自动检测 WARP：已安装则复用，未安装则自动安装。也可以指定：

```bash
warppool node mode nat01 warp --warp-install reuse # 只复用已安装的WARP，未安装则报错
warppool node mode nat01 warp --warp-install reinstall # 强制重装WARP
warppool node mode nat01 direct --remove-warp # 切回直连后同时卸载WARP
warppool node mode nat01 warp --method ssh # 通过SSH自动切换，不需要手动复制命令
```

Pull 切换命令会优先读取出口节点上的 `/etc/warppool-node/state.json`，正常情况下不需要再次填写主服务器地址；旧节点没有该状态文件时，脚本会提示手动填写，或按主服务器输出的备用命令携带 `server=http://<主服务器IP>:<端口>` 执行。

### WireGuard相关

```bash
warppool wg config nat01 # 输出节点nat01的本地WireGuard客户端配置
warppool wg up nat01 # 启动节点nat01的系统WireGuard客户端，主要用于诊断
warppool wg status nat01 # 查看节点nat01的本地WireGuard状态
warppool wg down nat01 # 停止节点nat01的本地WireGuard客户端
```

### 代理

```bash
warppool proxy config -o sing-box.json # 生成sing-box配置文件
warppool proxy start # 临时启动本地代理进程
warppool proxy service install # 创建本地代理systemd服务
warppool proxy service enable # 启动全部节点的本地代理并设置开机自启
warppool proxy status # 查看本地代理状态
warppool proxy stop # 停止临时启动的本地代理进程
```

### Clash 导出

```bash
warppool export clash -o clash.yaml # 导出Clash兼容配置
```

### 诊断

```bash
warppool version # 查看版本信息
warppool doctor # 检查本机运行环境和端口状态
warppool ping nat01 # 测试到节点nat01的WireGuard连通性
warppool speedtest --proxy http://127.0.0.1:10133 # 通过指定代理进行简易测速
warppool upgrade --yes # 更新主程序二进制和安装脚本资源
```

说明：`speedtest` 当前使用 HTTP proxy URL。完整 SOCKS 代理测速将在后续支持。

### 卸载

```bash
warppool uninstall --force # 卸载主服务器上的WarpPool程序和运行状态
```

`uninstall` 专用于卸载主服务器程序。
移除节点请使用 `warppool remove <name>`。

### 远端出口节点卸载

Push 部署会在出口节点安装一个辅助命令：

```bash
warppool-node-uninstall
```

在出口节点上常用：

```bash
warppool-node-uninstall device=wpnat01 # 卸载指定WarpPool WireGuard设备
warppool-node-uninstall all=true # 卸载本节点所有WarpPool WireGuard设备
warppool-node-uninstall all=true remove_warp=true # 同时卸载Cloudflare WARP
warppool-node-uninstall all=true remove_wireguard=true # 同时卸载WireGuard软件包
```

如果 `/etc/wireguard/wp*.conf` 只有一个配置文件，可以直接执行 `warppool-node-uninstall`。如果存在多个 WarpPool 设备，需要传 `device=<wg-device>` 或 `all=true`。

如果是 Pull 安装且节点上没有该辅助命令，可以直接执行：

```bash
curl -fsSL https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets/node_uninstall.sh | sudo bash -s -- all=true
```

---

## 配置

默认配置路径：

| 系统 | 路径 |
| --- | --- |
| Linux | `/etc/warppool/config.json` |

`warppool config init` 会创建这个 JSON 配置文件，并写入默认配置。它保存监听设置、默认代理设置、节点信息、Deploy Token 和 WireGuard 客户端私钥。一键安装脚本会在配置文件不存在时自动执行它。

请妥善保护配置文件。

---

## 更新日志

见 [CHANGELOG_CN.md](CHANGELOG_CN.md)。

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
- `upgrade` 会更新主程序二进制和内置安装脚本资源，但不会修改现有配置文件。
