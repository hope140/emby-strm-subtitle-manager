# Phase 2-D1 只读 Canary 验收定义

状态：D1 代码切片和自动化门禁已完成；Docker 镜像、Compose、真实服务器部署和真实 Canary 尚未验收。

本文件定义 ADR-003 的 D1 范围和进入 D2 的门禁。它是实现和部署测试的契约，不把规划内容写成已完成事实。

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

必须覆盖电影、剧集和至少一个多媒体源样本。D1 不执行搜索、Fetch、Add、Replace、Delete、Upload、Refresh 或批量扫描。

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

`MediaContext` 至少保留 `ItemID`、`MediaSourceID`、媒体类型、标题、季集信息、Provider IDs、EmbyPath、映射后的目录和 `IsStrm`。多媒体源选择规则必须显式记录，不能依赖列表顺序的偶然行为。当前实现对单源自动选择，对多源要求显式 `media_source_id`，并校验空 ID、重复 ID 和多个 default。

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
- API Key 使用 Secret 或权限受控文件注入，不能写入镜像、前端资源、响应、普通日志或 Git。应用还需要独立的 `security.identity_key_file`，仅用于 Inventory 的稳定本地标识，不能复用 Emby API Key；`security.api_auth_token_file` 提供管理 API 的 Bearer Token，也必须与这两类 Secret 分离。Docker file source 的 uid/gid/mode 在不同实现中不作为可信授权依据，宿主机启动前应实际将该文件设为 `10001:10001`、`0400`。
- `write_enabled=false` 是默认且可验证的启动配置；D1 没有启用它的操作步骤。
- 日志默认脱敏，不记录 Token、候选原始 ID、认证参数 URL、字幕正文或本机绝对路径。
- Docker 默认只运行 D1 只读能力：`write_enabled=false`、`remote_search_enabled=false`，媒体挂载为只读，配置/Secret 与媒体目录分离，不默认公开管理端口。
- `/livez` 与只返回极小状态的 `/readyz` 是公开探针；所有 `/v1/*` 必须携带 `Authorization: Bearer <token>`。缺失、错误或通过 query 传入 Token 均返回统一 401，不回显凭据。

文件型 Secret 的 `uid`、`gid`、`mode` 选项在不同 Docker 实现中不能作为授权依据。宿主机应先实际执行 `chown 10001:10001` 和 `chmod 0400`，再用应用用户做容器内可读性预检，例如 `docker compose run --rm --no-deps --entrypoint sh app -c 'test -r /run/secrets/app_api_auth_token && test -r /run/secrets/emby_api_key && test -r /run/secrets/app_identity_key'`。预检只返回成功/失败，不输出 Secret 内容。

镜像构建使用 Compose 的 `IMAGE_TAG`、`BUILD_VERSION`、`BUILD_COMMIT`、`BUILD_TIME` 和 `BUILD_SOURCE` 参数。正式部署应将 `IMAGE_TAG` 固定为不可变发布标签或摘要，并在启动前用 `docker image inspect` 核对 `org.opencontainers.image.version`、`revision`、`created` 和 `source` 标签与构建记录一致；回滚时重新指定已验收的旧标签或摘要，不使用浮动标签覆盖当前版本。

## 4. 安全边界

- 应用不读取 STRM 文件内容，不解析 STRM 内部 URL，不访问媒体代理地址。
- PathMapper 使用规范化路径、最长前缀匹配和允许根目录校验，拒绝 `..`、错误前缀和链接逃逸。
- D1 路由不提供任何 POST、PUT、PATCH、DELETE 写操作；服务端也必须在业务层拒绝写配置。
- Emby URL 只允许配置值，不接受客户端传入的任意目标，避免形成 SSRF 代理。
- UI 只显示是否配置凭据和脱敏状态，不显示 API Key。
- 单实例是当前默认模型；多实例共享状态和分布式锁不属于 D1。

## 5. 自动化验收门禁

本地实现已执行并通过：

1. 格式化、静态检查、单元测试和构建均通过，且命令针对新后端实际包覆盖。
2. Fake Emby 覆盖 Movie、Episode、多 MediaSource、分页、缺少字段、非 2xx、超时和空字幕流。
3. MediaContext 保留 `MediaSourceID`，Movie/Episode 类型和 STRM 标记稳定。
4. PathMapper 覆盖 Windows/Linux 规则、最长前缀、越界路径、链接逃逸和映射失败。
5. Inventory 覆盖 Embedded/External 合并、重复 Sidecar、编码/扩展名边界和不可管理内嵌字幕。
6. 请求和响应测试证明 API Key、认证参数、STRM 内容和本机绝对路径不会泄露。
7. 使用受控测试夹具证明 D1 代码没有向 STRM 内部地址发起请求。

上述结果证明 Go 源码和自动化检查通过。Docker 镜像/Compose 构建、容器启动和真实服务器访问仍属于下一步部署验证，不能从本地结果推断通过。

这些自动化检查只能证明代码和受控环境行为，不能替代真实 Emby 验收。

## 6. 真实 Canary 验收门禁

在私网或 SSH 隧道环境部署单容器后，使用专用测试账号和已确认的只读配置完成：

- 一个真实 Movie、一个真实 Episode 和一个多媒体源 Item 均可浏览。
- `ItemID`、`MediaSourceID`、媒体类型、STRM 标记和字幕状态与 Emby 页面/API 一致。
- 映射成功的目录只读取当前 Item 所需范围；映射失败时安全拒绝并给出可行动错误。
- Embedded 字幕显示为不可管理，Emby 与文件系统同时发现的 Sidecar 不重复显示。
- 观察 Emby、应用和网络证据，确认 D1 没有写请求、Refresh、STRM 内部访问或凭据泄露。
- 浏览器开发者工具、应用日志和容器环境导出中均不存在 API Key 或认证参数 URL。
- 将 `write_enabled=false` 作为配置和运行时状态分别核对，不能只查看配置文件。

D1 只有在自动化门禁和真实 Canary 门禁都通过后，才允许进入 D2 搜索预览。任何一项失败都保留证据并回到只读边界修复。

## 7. 非目标

D1 明确不包含：

- 远程字幕搜索、Fetch、候选排序或 Provider 选择
- 预览 Artifact、字幕解析服务或候选 Token
- Add、Replace、Delete、Upload、Refresh 和归档旧文件
- 全库扫描、批量任务、定时任务和自动下载
- STRM 内容读取、115/CD2 访问、媒体代理或第二套媒体索引
- 公开互联网部署、账号系统、多实例锁和生产数据迁移

完成 Docker 产物验证以及真实 Canary 门禁前，不得以此文件宣称 D1 已部署或真实验收通过。
