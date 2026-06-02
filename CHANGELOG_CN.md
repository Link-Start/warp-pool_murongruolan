# 更新日志

## v0.1.10

- 新增短命令 `wpl`，等效于 `warppool`，例如 `wpl node list`、`wpl ping nat01`。
- 新增子节点卸载短命令 `wpl-node-uninstall`，等效于 `warppool-node-uninstall`。
- 优化 `warppool ping` 中文输出，将“节点公网地址”调整为更准确的“节点延迟检测地址”。
- 修复中文模式下 `warppool ping` 仍混入英文 `mode`、`proxy check ok` 等提示的问题。
- 增强卸载安全性：主服务器卸载只会删除指向 WarpPool 的 `wpl` 软链接，不会误删其他程序占用的同名文件。

## v0.1.9

- 新增 Alpine WARP 支持，基于 `wgcf` 生成 WireGuard 配置，并通过 sing-box 内置 WireGuard endpoint 出口。
- Alpine WARP 端点探测改为优先 IPv6，失败后回退 IPv4，最后兜底原始域名。
- Alpine 上 sing-box 安装优先使用系统仓库：`apk update && apk add --no-cache sing-box`。
- 当 Alpine 仓库包不存在、无法运行，或无法加载 WarpPool 生成的 WARP 配置时，自动回退到 GitHub musl 版本。
- 修复 Alpine WARP 部署时误下载非 musl 版 sing-box 导致二进制无法运行的问题。

## v0.1.8

- 正式优化 1G 级别小硬盘出口节点的 WARP 模式安装。Debian/Ubuntu 安装脚本不再安装 `wireguard` 元包，只安装必要的 WireGuard tools，尽量使用 `--no-install-recommends`，并在安装步骤后清理软件包缓存。
- 修复轻量依赖调整后 WARP 安装缺少 `gpg` 的问题；仅在 WARP 模式需要 Cloudflare apt 仓库时安装 `gpg`。
- 放宽小硬盘 NAT VPS 的 WARP 磁盘预检：硬盘低于推荐空间但高于硬性最低空间时只提示风险，不再过早阻止安装。
- `warppool node mode --method ssh` 会复用 Push 部署时保存的非敏感 SSH 默认值。SSH 密码不会保存。
- `warppool ping` 新增节点延迟检测地址 RTT、主服务器直连 HTTP 延迟、代理出口 IP 和代理 HTTP 延迟检测，并支持多个 HTTP 检测地址兜底。

## v0.1.5

- 修复节点模式切换的语言继承问题。Pull 安装的节点会保存用户选择的语言，后续执行 `node_mode.sh` 切换 direct/WARP 时会继续使用该语言。
- 为 Deploy Token 和节点模式切换命令增加更醒目的“一次性 token / 有效期”提示。
- 为 Cloudflare WARP 官方客户端安装增加磁盘空间和 inode 预检；当 `apt` 因磁盘配额不足失败时，给出更明确的恢复提示。
- 修复 SSH Push 部署非 root 用户失败的问题；远端用户不是 root 时，会在可用的情况下自动使用 `sudo` 执行需要权限的操作。
