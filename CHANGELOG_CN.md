# 更新日志

## v0.1.5

- 修复节点模式切换的语言继承问题。Pull 安装的节点会保存用户选择的语言，后续执行 `node_mode.sh` 切换 direct/WARP 时会继续使用该语言。
- 为 Deploy Token 和节点模式切换命令增加更醒目的“一次性 token / 有效期”提示。
- 为 Cloudflare WARP 官方客户端安装增加磁盘空间和 inode 预检；当 `apt` 因磁盘配额不足失败时，给出更明确的恢复提示。
- 修复 SSH Push 部署非 root 用户失败的问题；远端用户不是 root 时，会在可用的情况下自动使用 `sudo` 执行需要权限的操作。
