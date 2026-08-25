# ADR-006　发布版管理员登录与自动化 API 凭据分离

- 状态　accepted
- 日期　2026-08-25
- 相关组件　发布版 Web UI、HTTP API、Docker Compose Secret、CLI/定时任务、D3 写入权限

## 背景

项目的发布对象是运行 Docker 镜像的服务器管理员，而不是 Emby 普通观众。当前 D1/D2 使用一个应用 Bearer Token 保护所有 `/v1/*` 路由，安全边界清楚，但让人手动记忆和输入一串随机 Token，分发后的使用体验较差。

同时，CLI、定时任务和其他自动化客户端确实需要一种机器凭据；这类凭据应类似 Emby API Key 的调用方式，但只能访问本项目服务，不能与管理员密码或 Emby API Key 复用。

## 问题

需要在不引入多租户、用户注册和复杂账户后台的前提下，同时满足：

1. 服务器管理员可以使用自己设置的用户名和密码登录 Web UI。
2. CLI/定时任务拥有独立、可轮换的 Bearer 凭据。
3. Emby API Key 继续只存在服务端，不能被 UI 或自动化客户端接触。
4. 发布版 Docker 配置足够简单，不能依赖仓库内默认密码或手工修改镜像。

## 可选方案

### 方案 A：继续让管理员直接输入共享 Bearer Token

实现和维护成本最低，适合当前内部 D1/D2，但不适合作为公开发布镜像的主要人机登录方式。

### 方案 B：反向代理 Basic Auth

可以由 OpenResty、Caddy 或其他入口承担用户名密码校验，应用继续只看 Bearer Token。部署者需要额外配置反代，跨环境一致性和公网 HTTPS 依赖较强，不能作为镜像的唯一默认登录体验。

### 方案 C：应用管理员会话 + 独立自动化 Token（推荐）

Web UI 使用用户名密码登录，服务端签发短期 HttpOnly 会话 Cookie；CLI/定时任务继续使用独立 Bearer Token。管理员账号和密码通过 Docker Secret 或等价受保护文件配置，不在面板中提供账号管理、改密码或注销入口；轮换密码通过替换 Secret 并重建容器完成。

## 初步选择

选择方案 C，作为 D2.5 排期项，目标是在 D3 写入能力和公开发布前完成。D2.5-A/B 已在本地实现并通过自动化验证；尚未部署 C92/SH，D2.5-C scope、CSRF 和 D3 写入仍未完成。当前 D2 的共享 Bearer Token 继续作为过渡契约，直到管理员登录完成目标环境迁移验收。

### 管理员登录

- 每个部署实例配置自己的管理员用户名和密码，不提供通用默认密码。
- 用户名和密码分别从 `security.admin_username_file`、`security.admin_password_file` 读取；用户名为 1–64 字节，密码为 12–256 字节，均拒绝控制字符。密码不写入镜像、Git、日志、URL 或普通环境导出；启动后只保留随机盐 PBKDF2-HMAC-SHA256 派生校验材料。
- 登录成功后使用短期会话 Cookie；Cookie 应为 `HttpOnly`，并按 HTTP/HTTPS 部署设置 `Secure` 和 `SameSite` 属性。
- 不在面板实现账号注册、改密码、注销或多角色管理。密码轮换、强制失效和忘记密码恢复由部署者替换 Secret 并重建容器完成。
- D3 写入接口启用前必须补齐 CSRF、会话过期、失败限速、来源校验和审计边界。

### 自动化 API

- 自动化 Token 与管理员密码、Emby API Key 完全分离，每个部署实例独立生成和轮换。
- 继续使用 `Authorization: Bearer`，不接受 query 参数，不写入日志或响应。
- D2.5 首先支持一个只读自动化 Token；后续按最小权限规划 `media:read`、`subtitle:search`、`subtitle:preview`，D3 写入权限另设独立 scope 和门禁。
- 当前实现尚未细分 scope；在 D2.5 完成前，不能宣称已有细粒度权限隔离。

### 发布和网络边界

- Compose 示例提供 Secret 文件模板、权限预检和失败时的可行动提示；不把真实密码提交到仓库。
- 内网或 VPN 可以使用 HTTP；公网管理员入口仍需要 HTTPS、可信隧道或明确的网络白名单。用户名密码登录不会消除明文 HTTP 的窃听风险。
- 不复用 Emby API Key 作为管理员密码或自动化 Token。

## 选择原因

- 对发布镜像的服务器管理员更符合常见登录习惯，避免记忆随机 Token。
- 保留 Bearer Token 可以兼容脚本和 CI，不迫使自动化客户端模拟浏览器登录。
- 不引入用户注册、多租户和复杂后台，控制实现范围。
- Secret 轮换和容器重建与现有 Docker 运维方式一致，恢复路径清晰。

## 已知代价

- D2.5-C 和 D3 仍需要 CSRF、scope、审计和写权限测试；D2.5-A/B 已覆盖会话存储、Cookie、密码校验和登录限速。
- 没有面板注销时，现有会话依赖 TTL、浏览器清理或容器重建失效；必须在文档中明确操作方式。
- Docker Compose file-source Secret 仍依赖宿主机文件权限，不能把它误解为自动加密的密码保险箱。
- 管理员会话和自动化 Token 并存后，API 认证测试矩阵和日志脱敏范围会扩大。

## 排期和门禁

1. D2.5-A：固定管理员凭据 Secret、初始化/轮换流程和配置错误码。（本地完成）
2. D2.5-B：实现登录会话、Cookie 属性、TTL、重启失效和失败限速；保留 Bearer 自动化兼容。（本地完成）
3. D2.5-C：为自动化 Token 增加最小权限模型，至少区分只读和未来写入范围。
4. D2.5-D：Fake Emby、浏览器、CLI、Compose、日志脱敏和轮换/回滚验收。
5. D3 之前：管理员会话、CSRF 和写权限 scope 必须通过；未通过时继续保持 `write_enabled=false`。

## 验证依据

- [D2 搜索预览契约](../d2-search-preview-contract.md)
- [Phase 2 只读 Canary](../phase2-readonly-canary.md)
- [总体规划](../../Emby_STRM_Subtitle_Manager_Master_Plan_Revised.md)
- [Docker Compose Secrets 官方文档](https://docs.docker.com/reference/compose-file/secrets/)
