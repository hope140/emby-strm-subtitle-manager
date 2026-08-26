# Phase 2-D1 只读 Canary 验收定义

状态：D1 代码切片、Linux 自动化门禁、C92 Docker Compose 部署、公网 HTTPS 及 Movie/Episode STRM Canary 已验收；真实版本组样本已找到，a70bf89 的完整 MediaSources 读取、两个真实 Item 的应用/API source 对应核对以及真实多源 API 对应和 D2 409 安全拒绝 Canary 已通过，证据见 [D2 多源真实 API Canary](../records/acceptance/d2-multisource-c92-canary-acceptance-20260825.md)。真实浏览器 UI 显式 source 点击和多源正向 Search/Fetch/Preview 仍未验收。按 ADR-005，D1 已满足有条件进入 D2 的门禁，但真实多源正向支持仍未收口。

本文件定义 ADR-003 的 D1 范围，并按 [ADR-005](../decisions/adr/005-conditional-d2-entry-without-live-multisource.md) 区分进入单源 D2 的门禁和真实多源搜索支持门禁。C92 已找到真实版本组；详情读取必须在 `Fields` 中包含 `AlternateMediaSources`，相关证据见 [D2 多版本 MediaSources 实测记录](../records/acceptance/d2-multisource-c92-sample.md)。D2 的接口、安全预算和测试矩阵，以及 D2-B1 当前后端状态见 [D2 搜索预览契约](d2-search-preview-contract.md)。它不授权真实 Canary、部署或重启。

## 1. D1 目标与范围

D1 只建立一个可部署的只读纵向切片：

```text
浏览 Emby 媒体
  → 选择 Movie 或 Episode
  → 生成 MediaContext
  → 解析 MediaSource 和 STRM 标记
  → 通过 PathMapper 得到受控本地目录
  → 合并 Emby MediaStreams 与 Sidecar 字幕
  → 展示字幕状态
```

自动化必须覆盖电影、剧集和多 MediaSource；真实 Canary 必须覆盖 Movie 与 Episode，真实多源 Item 在可获得时补验。缺少真实多源样本不再阻断单源 D2，但在补验前不得宣称或启用真实多源搜索。D1 不执行搜索、Fetch、Add、Replace、Delete、Upload、Refresh 或批量扫描。

## 2. D1 只读 API

当前 D1 实现提供 7 个 GET 路由，其中 3 个运维路由和 4 个业务路由：

```text
GET /livez
GET /readyz
GET /v1/health
GET /v1/emby/libraries
GET /v1/emby/items
GET /v1/media/{itemId}
GET /v1/media/{itemId}/subtitles
```

`/livez` 只检查进程响应；`/readyz` 会实际调用 Emby 的只读库列表接口，并对结果做短时成功/失败缓存；`/v1/health` 返回版本、功能开关和 Emby readiness 状态。除上述 GET 外，D1 不提供业务路由。

API 必须由服务端使用 `ItemID` 重新向 Emby 查询。客户端不能提交可直接访问的物理路径、STRM 内部地址或 Emby 原始候选 ID 作为可信写入依据。D1 响应只返回展示所需的脱敏领域字段。

`MediaContext` 至少保留 `ItemID`、`MediaSourceID`、媒体类型、标题、季集信息、Provider IDs、Inventory 使用的 EmbyPath、映射后的目录和 `IsStrm`。多媒体源选择规则必须显式记录，不能依赖列表顺序的偶然行为。当前实现对单源自动选择，对多源要求显式 `media_source_id`，并校验空 ID、重复 ID 和多个 default。STRM 的 PathMapper、PathGuard、LocalDirectory 和 STRM 判断基于 Item.Path；非 STRM 仅在 selected MediaSource.Path 是本地路径时使用它，远程 source path 不得进入本地映射。STRM 多源只共享 Item.Path 的 sidecar 目录，MediaStreams 仍保持 source-specific。

字幕清单需要区分 Embedded、External 和 Sidecar，记录发现来源、格式、语言、Forced/Default 状态和 `Manageable`。内嵌字幕永远不可管理；同一 Sidecar 同时被 Emby 和文件系统发现时只能展示一条。Inventory 只枚举受控目录和读取文件元数据，不读取 STRM、媒体或字幕正文；目录、映射或 MediaStreams 不完整且没有已知字幕时报告 `unknown`，已有字幕时报告 `present` 并保留不完整 warning，不能伪装成“没有字幕”。

## 3. 配置与默认部署

默认运行方式是 Linux Docker Compose 单应用容器。以下是语义配置示例，实际端点和路径由部署环境注入：

```yaml
emby:
  url: <EMBY_PRIVATE_URL>
  api_key_file: /run/secrets/emby_api_key

security:
  identity_key_file: /run/secrets/app_identity_key
  api_auth_token_file: /run/secrets/app_api_auth_token
  session_cookie_secure: false

server:
  listen_address: <APP_BIND_ADDRESS>

features:
  write_enabled: false

path_mappings:
  - emby: <EMBY_MEDIA_ROOT>
    local: /media
```

Compose 部署必须满足：

- 应用容器加入 Emby 所在私网，或通过已存在的 SSH 隧道访问，不新增公网暴露。
- 媒体目录挂载为只读；应用数据目录与媒体目录分离。
- API Key 使用 Secret 或权限受控文件注入，不能写入镜像、前端资源、响应、普通日志或 Git。应用还需要独立的 `security.identity_key_file`，仅用于 Inventory 的稳定本地标识，不能复用 Emby API Key；`security.api_auth_token_file` 提供 CLI/自动化使用的 Bearer Token；`APP_ADMIN_USERNAME` 与 `APP_ADMIN_PASSWORD` 由私有 Compose environment 提供。四类凭据必须分离，管理员 environment 不进入公开示例、Git、日志或响应。
- `write_enabled=false` 是默认且可验证的启动配置；D1 没有启用它的操作步骤。
- 日志默认脱敏，不记录 Token、候选原始 ID、认证参数 URL、字幕正文或本机绝对路径。
- 公网反代使用仓库提供的安全日志基线，只记录粗粒度路由和状态，不记录完整请求行、query、Authorization、Referer、原始 Item ID 或候选 ID；模板见 [OpenResty 公网入口基线](../guides/openresty-public-entry.md)。
- Docker 默认只运行 D1 只读能力：`write_enabled=false`、`remote_search_enabled=false`，媒体挂载为只读，配置/Secret 与媒体目录分离，不默认公开管理端口。
- 若后续获得 D2 单独授权，Compose 必须把宿主机专用的 `/replace/with/dedicated/d2-preview-cache` 仅绑定到容器 `/var/lib/subbridge/d2-preview-cache`，该宿主目录实际 owner 为 `10001:10001`、mode 为 `0700`，且位于媒体映射之外；rootfs 仍保持只读、`/media` 仍保持只读。Canary allowlist 使用新增 `d2_canary_items` file-source Secret，只读注入到 `/run/secrets/d2_canary_items`，宿主文件实际按 `10001:10001`、`0400` 准备，不能只依赖 Compose uid/gid/mode 字段。
- `/livez` 与只返回极小状态的 `/readyz` 是公开探针；`POST /v1/auth/login` 接受 Compose environment 中的管理员凭据，其余 `/v1/*` 必须携带有效管理员会话或 `Authorization: Bearer <token>`。管理员 environment 缺失或非法时服务启动失败，不回退 Bearer-only UI；缺失、错误或通过 query 传入 Token 均返回统一 401，不回显凭据。Bearer 只读 scope 由 `security.api_auth_scopes` 控制，缺少所需 scope 返回 403；写 scope 在 `write_enabled=false` 时拒绝。

文件型服务端凭据的 `uid`、`gid`、`mode` 选项在不同 Docker 实现中不能作为授权依据。宿主机应先实际核对 `emby_api_key`、`app_identity_key`、`app_api_auth_token` 的权限，再用应用用户做容器内可读性预检，例如 `docker compose run --rm --no-deps --entrypoint sh app -c 'test -r /run/secrets/app_api_auth_token && test -r /run/secrets/emby_api_key && test -r /run/secrets/app_identity_key && test -r /etc/subbridge/config.yaml'`。管理员 environment 由启动校验覆盖，预检只返回成功/失败，不输出凭据内容。

镜像构建使用 Compose 的 `IMAGE_TAG`、`BUILD_VERSION`、`BUILD_COMMIT`、`BUILD_TIME` 和 `BUILD_SOURCE` 参数。正式部署应将 `IMAGE_TAG` 固定为不可变发布标签或摘要，并在启动前用 `docker image inspect` 核对 `org.opencontainers.image.version`、`revision`、`created` 和 `source` 标签与构建记录一致；回滚时重新指定已验收的旧标签或摘要，不使用浮动标签覆盖当前版本。

## 4. 安全边界

- 应用不读取 STRM 文件内容，不解析 STRM 内部 URL，不访问媒体代理地址。
- PathMapper 使用规范化路径、最长前缀匹配和允许根目录校验，拒绝 `..`、错误前缀和链接逃逸。
- D1 路由不提供任何 POST、PUT、PATCH、DELETE 写操作；服务端也必须在业务层拒绝写配置。
- Emby URL 只允许配置值，不接受客户端传入的任意目标，避免形成 SSRF 代理。
- UI 只显示是否配置凭据和脱敏状态，不显示 API Key。
- 单实例是当前默认模型；多实例共享状态和分布式锁不属于 D1。

## 5. 自动化验收门禁

本地与 Linux 验证均已执行并通过：

1. 格式化、静态检查、单元测试和构建均通过，且命令针对新后端实际包覆盖。
2. Fake Emby 覆盖 Movie、Episode、多 MediaSource、分页、缺少字段、非 2xx、超时和空字幕流。
3. MediaContext 保留 `MediaSourceID`，Movie/Episode 类型和 STRM 标记稳定。
4. PathMapper 覆盖 Windows/Linux 规则、最长前缀、越界路径、链接逃逸和映射失败。
5. Inventory 覆盖 Embedded/External 合并、重复 Sidecar、编码/扩展名边界和不可管理内嵌字幕。
6. 请求和响应测试证明 API Key、认证参数、STRM 内容和本机绝对路径不会泄露。
7. 使用受控测试夹具证明 D1 代码没有向 STRM 内部地址发起请求。

上述结果证明 Go 源码和自动化检查通过。历史 C92 D1 版本的 Docker Compose schema/build、host-network、UID 10001、只读 root、只读媒体、三份 D1 文件凭据权限、`/readyz`、Bearer 401 和版本溯源标签已通过部署验收；D2.5 env-only 版本及 a70bf89 MediaSources 修正已在 C92 app-only 验收，过程见 [D2.5 目标环境迁移预检](../records/reviews/d2.5-target-migration-preflight-20260825.md)。

这些自动化检查只能证明代码和受控环境行为，不能替代真实 Emby 验收。

## 6. 真实 Canary 验收门禁

在私网或 SSH 隧道环境部署单容器后，使用专用测试账号和已确认的只读配置完成以下门禁。Movie 与 Episode STRM 已完成；2026-08-24 两轮有界真实 Emby 扫描覆盖 11 个媒体库、1,026 个最新样本和 938 个分层样本未命中多媒体源；2026-08-25 环境负责人提供的已知 Movie 版本组通过 `AlternateMediaSources` 详情读取补齐了真实样本，a70bf89 部署后两个真实 Item 的 source 集合与应用响应也已完成只读对应核对，随后真实多源 API/source 对应和 D2 409 安全拒绝 Canary 已完成。真实浏览器 UI 显式 source 点击和多源正向 Search/Fetch/Preview 保留为独立待验门禁：

- 一个真实 Movie 和一个真实 Episode 均可浏览；找到多媒体源 Item 后补验同一流程。
- `ItemID`、`MediaSourceID`、媒体类型、STRM 标记和字幕状态与 Emby 页面/API 一致。
- 映射成功的目录只读取当前 Item 所需范围；映射失败时安全拒绝并给出可行动错误。
- Embedded 字幕显示为不可管理，Emby 与文件系统同时发现的 Sidecar 不重复显示。
- 观察 Emby、应用和网络证据，确认 D1 没有写请求、Refresh、STRM 内部访问或凭据泄露。
- 浏览器开发者工具、应用日志和容器环境导出中均不存在 API Key 或认证参数 URL。
- 将 `write_enabled=false` 作为配置和运行时状态分别核对，不能只查看配置文件。

按 ADR-005，自动化门禁、Movie/Episode STRM Canary、部署和安全边界通过后，可以有条件进入 D2 的契约、实现和单源 Canary。当前多源自动化的 409、真实 API/source 对应和显式 source API 选择已通过，真实多源结果见 [D2 多源真实 API Canary](../records/acceptance/d2-multisource-c92-canary-acceptance-20260825.md)；真实浏览器 UI 显式 source 点击及多源正向 Search/Fetch/Preview 仍待完成。因此 D2 首轮仍只支持单源 Movie/Episode，多源请求必须安全拒绝，稳定响应契约由 [D2 搜索预览契约](d2-search-preview-contract.md) 和测试固定。`remote_search_enabled=false` 继续作为默认值，实际搜索仍需 D2 专项授权和验收。D3 和所有写入门禁不变。任何已要求门禁失败都保留证据并回到对应边界修复。

## 7. 非目标

D1 明确不包含：

- 远程字幕搜索、Fetch、候选排序或 Provider 选择
- 预览 Artifact、字幕解析服务或候选 Token
- Add、Replace、Delete、Upload、Refresh 和归档旧文件
- 全库扫描、批量任务、定时任务和自动下载
- STRM 内容读取、115/CD2 访问、媒体代理或第二套媒体索引
- 公开互联网部署、账号系统、多实例锁和生产数据迁移

本文件只确认已经完成的部署、公网 HTTPS、STRM Canary 以及真实多源 API/source 对应和安全拒绝证据。当前可以宣称 D1 已满足 ADR-005 的单源 D2 条件入口；浏览器 UI source 点击和多源正向能力仍未通过，不得宣称真实多源搜索支持或把安全拒绝写成正向支持。
