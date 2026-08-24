# D1 部署验收报告

状态：D1 部署、公网 HTTPS 与 STRM Canary 部分通过，真实多媒体源门禁尚未完成。

本报告记录 D1 只读切片在已授权 C92 环境的部署和真实 STRM Canary 结果。报告只保留可复核的摘要，不记录私有路径、Item 标识、媒体标题、Secret、认证参数或私有 URL。

## 版本与归档

- 验收代码提交：`a38b1f7ee39101e328813c0284976dbcc033866c`
- 归档 SHA256：`E93CD043FC6BF4AD1626993B3870B2BF52E7410CAFC551B974FFE9E87F1DB342`

## 已通过

- Docker Compose schema、镜像构建和 host-network 变体通过。
- 容器 UID 10001、只读 root、媒体只读挂载和三份 Secret 权限检查通过。
- `/readyz` readiness 检查、Bearer 认证错误时的 401 行为和版本溯源标签检查通过。
- FRP 新代理启用单代理 payload 加密，原有 9 条代理与新增代理均保持 running；共享 FRPC 的全局 TLS 配置未变更。
- SH loopback remote port 的 `/readyz` 返回 200；主机防火墙对公网应用 remote port 有显式 IPv4/IPv6 DENY，独立外部探针失败且 DROP 计数增加。
- 公网 HTTPS 证书严格校验通过；`/readyz` 返回 200，HTTP 跳转 HTTPS，无、错误或 query Bearer 返回 401，正确 Bearer 返回 200。
- Linux 全包测试通过且无 skip。
- 真实库浏览中的 Movie 与 Episode STRM 均为 mapped、inventory complete、present，且无 warning。
- `write_enabled=false`、`remote_search_enabled=false` 保持关闭；本次没有搜索或写入行为。

## 尚未完成

- 对最近 1000 个真实 Movie/Episode 做了有上限的只读检查，尚未找到多媒体源 Item。自动化的 409 和显式 `media_source_id` 选择测试已通过，但不能替代真实多源 Item 验收。
- 当前个人部署保留面板默认 access log 配置；验收期间从未通过 query 发送真实 Token。面向发布的统一安全日志与反代模板见 [OpenResty 公网入口基线](../deploy/openresty/README.md)，它不会自动修改现有服务器配置。

## 结论与下一步

D1 的代码、自动化、C92 部署、公网 HTTPS 和 Movie/Episode STRM 只读 Canary 已具备可复核证据；在补齐真实多媒体源样本并完成相应验收前，继续保持只读和搜索关闭，不进入 D2。
