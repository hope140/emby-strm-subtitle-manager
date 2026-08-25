# ADR-006　发布版管理员登录与自动化 API 凭据分离

- 状态　accepted
- 日期　2026-08-25
- 相关组件　发布版 Web UI、HTTP API、Docker Compose environment、CLI/定时任务、D3 写入权限

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

Web UI 使用用户名密码登录，服务端签发短期 HttpOnly 会话 Cookie；CLI/定时任务继续使用独立 Bearer Token。管理员账号和密码由私有 Docker Compose 文件的 `environment` 直接提供，固定变量名为 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD`，不使用 `.env`、管理员 Docker Secret 或初始化脚本，不在面板中提供账号管理、改密码或注销入口；轮换密码通过修改私有 Compose 并重建容器完成。

## 最终选择

选择方案 C，作为 D2.5 排期项，目标是在 D3 写入能力和公开发布前完成。D2.5-A/B/C 已在本地实现并通过自动化验证；A/B/D 已基于公开 `b9916d1` 完成 C92 app-only 部署与认证验收，随后 a70bf89 完整 MediaSources 修正完成了 app-only 重建，784ad32 scope 版本也完成了 app-only 发布与本机探针验收。SH/FRP/OpenResty 未在本任务处理。D3 Add 已补齐 CSRF、写 scope 和专用写入边界，并在 C92 独立 Canary 窗口完成真实闭环；当前 D2 的 Bearer 继续作为只读自动化兼容契约，D3 写 scope 只在独立 Canary 配置中临时开放。

### 管理员登录

- 每个部署实例配置自己的管理员用户名和密码，不提供通用默认密码。
- 用户名和密码分别从 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD` 读取；缺失、空值或非法值直接阻止启动。用户名为 1–64 字节，密码为 6–256 字节，均拒绝控制字符。密码不写入镜像、Git、日志、URL 或响应；启动后只保留随机盐 PBKDF2-HMAC-SHA256 派生校验材料。
- 登录成功后使用短期会话 Cookie；Cookie 应为 `HttpOnly`，并按 HTTP/HTTPS 部署设置 `Secure` 和 `SameSite` 属性。
- 不在面板实现账号注册、改密码、注销或多角色管理。密码轮换、强制失效和忘记密码恢复由部署者修改私有 Compose 的 environment 并重建容器完成。
- D3 写入接口启用前必须通过 CSRF、会话过期、失败限速、来源校验、allowlist 和审计/history 边界；当前 Add 契约已补齐这些门禁，真实 C92 结果记录在 [D3 C92 Canary 验收](../d3-c92-canary-acceptance-20260825.md)。

### 自动化 API

- 自动化 Token 与管理员密码、Emby API Key 完全分离，每个部署实例独立生成和轮换。
- 继续使用 `Authorization: Bearer`，不接受 query 参数，不写入日志或响应。
- D2.5 支持一个只读自动化 Token，并按 `media:read`、`subtitle:search`、`subtitle:preview` 做路由检查；部署者可以在 `security.api_auth_scopes` 中删除只读 scope。
- `subtitle:write` 仅在 D3 Canary 与 `write_enabled=true` 同时配置时允许；基础配置仍拒绝写能力，D3 还必须另设 scope、CSRF、allowlist、审计/history 和真实门禁。

### 发布和网络边界

- Compose 示例直接声明两个空的 environment 占位符；实际部署文件必须由部署者填入值，不把真实密码提交到仓库。
- 内网或 VPN 可以使用 HTTP；公网管理员入口仍需要 HTTPS、可信隧道或明确的网络白名单。用户名密码登录不会消除明文 HTTP 的窃听风险。
- 不复用 Emby API Key 作为管理员密码或自动化 Token。

## 选择原因

- 对发布镜像的服务器管理员更符合常见登录习惯，避免记忆随机 Token。
- 保留 Bearer Token 可以兼容脚本和 CI，不迫使自动化客户端模拟浏览器登录。
- 不引入用户注册、多租户和复杂后台，控制实现范围。
- environment 轮换和容器重建与现有 Docker 运维方式一致，恢复路径清晰。

## 已知代价

- D3 本地已覆盖 CSRF、写权限 scope、allowlist、原子写入、Refresh/轮询、history/quarantine 和 Fake Emby 测试；D2.5-A/B/C 已覆盖会话、Cookie、密码校验、登录限速和只读 scope，D2.5-D 已覆盖本地及 C92 app-only 发布证据。真实 C92 Add、字幕流和客户端读取已由 [D3 C92 Canary 验收](../d3-c92-canary-acceptance-20260825.md)确认。
- 没有面板注销时，现有会话依赖 TTL、浏览器清理或容器重建失效；必须在文档中明确操作方式。
- Compose environment 会出现在容器配置元数据中，不能把它当作加密密码保险箱；Docker 主机访问权限必须受控。
- 管理员会话和自动化 Token 并存后，API 认证测试矩阵和日志脱敏范围会扩大。

## 排期和门禁

1. D2.5-A：固定管理员凭据 environment、轮换流程和配置错误码。（本地完成）
2. D2.5-B：实现登录会话、Cookie 属性、TTL、重启失效和失败限速；保留 Bearer 自动化兼容。（本地完成）
3. D2.5-C：为自动化 Token 增加最小权限模型，至少区分只读和未来写入范围。（已完成并随 784ad32 发布到 C92）
4. D2.5-D：Fake Emby、浏览器、CLI、Compose、日志脱敏和轮换/回滚验收。（已完成）
5. D3 之前：管理员会话、CSRF 和写权限 scope 必须通过；真实验收窗口之外继续保持 `write_enabled=false`。

## D3 状态更新

D3 Add 已补齐管理员会话 CSRF、同源来源检查、`subtitle:write` scope、D3 Item allowlist、Artifact 绑定、原子非覆盖版本写入、Emby Refresh/轮询、history/quarantine 和幂等操作 ID。Bearer 写请求不使用浏览器 CSRF，但仍必须具备独立写 scope；基础配置和基础 Compose 仍拒绝或不提供写入。真实 C92 Add、字幕流和实际客户端读取已完成，Replace、Delete、Upload 与批量能力继续关闭。详见 [D3 专用样本 Add 契约](../d3-dedicated-add-contract.md) 和 [D3 C92 Canary 验收](../d3-c92-canary-acceptance-20260825.md)。

## 验证依据

- [D2 搜索预览契约](../d2-search-preview-contract.md)
- [Phase 2 只读 Canary](../phase2-readonly-canary.md)
- [总体规划](../../SubBridge_Master_Plan_Revised.md)
- [Compose 示例](../../deploy/compose.example.yaml)
