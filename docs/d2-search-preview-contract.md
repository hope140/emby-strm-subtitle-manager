# D2 搜索预览契约与安全设计

- 状态　D2 后端、内嵌只读 UI 与 D2.5 管理员认证已实现；C92 单源真实 Provider API Canary、完整 source 字段、应用/API 对应和多源 409 安全拒绝均已通过，真实管理 UI 点击验收和多源正向支持仍待独立任务
- 日期　2026-08-25
- 适用范围　ADR-003 的 D2、ADR-005 规定的单源条件入口
- 当前实现　后端、安全门禁、内嵌 UI、Compose environment 管理员登录、Fake Emby 测试和 C92 单源真实 API Canary 已完成；C92 管理 UI Search→Fetch→Preview 点击流程和多源正向 Search、Fetch、Preview 尚未完成

本文把 D2 的搜索、Fetch、预览和安全边界固定下来。它不授权真实环境启用远程搜索，不改变 `remote_search_enabled=false` 的默认值，也不授权部署、重启、媒体库写入或 Emby 配置修改。

## 1. 范围与门禁

D2 只做以下只读工作：

- 对一个 Movie 或 Episode 重新读取 Emby Item，并以包含 `AlternateMediaSources` 的详情响应确认完整 source 列表。
- 通过 Emby Remote Subtitle Bridge 搜索候选。
- 以服务端签发的短期候选 Token 触发单个候选 Fetch。
- 对 Fetch 到的字幕做大小、编码、格式和解析校验，生成短期 `PreviewArtifact`。
- 以受限的 cue 数据向 UI 提供预览。

D2 明确不做以下工作：

- 不调用 Emby 的下载保存接口，不调用 Refresh，不执行 Add、Replace、Delete、Upload 或批量任务。
- 不写媒体库目录。D2 只能写项目专用的短期预览缓存。
- 不读取 STRM 内容、不访问 STRM 内部地址、不把 `MediaSource.Path` 当作可请求 URL。
- 不进入 D3。即使 Artifact 已通过校验，也不能在本轮直接落盘或触发 Emby 重新识别。

默认和普通部署期间，`remote_search_enabled` 保持 `false`。只有取得独立 D2 授权后，Canary 窗口才临时将它设为 `true`，并同时启用服务端 Canary Item allowlist；窗口结束立即关闭。Canary 通过后是否长期开放另行决定，代码合并、UI 出现按钮或本文件落地都不构成授权。

## 2. 首轮支持范围与安全前置检查

首轮 Search、Fetch、Preview 只支持 `Type` 为 `Movie` 或 `Episode` 且刚刚从 Emby 详情接口读取到的完整 `MediaSources` 数量为 1 的 Item。真实版本组样本已经证明，列表响应或缺少 `AlternateMediaSources` 的请求不能作为完整 source 事实。

每次 Search、Fetch 和 Preview 都要在服务端重新取得 Item 并检查以下条件：

1. Item 存在，类型是 Movie 或 Episode。
2. 详情请求的 `Fields` 必须包含 `AlternateMediaSources`；客户端将响应中的完整 `MediaSources`（以及某些版本单独返回的备用 source 字段）合并，只抑制两个字段之间重复的非空 source ID；同一字段内部的重复必须保留给源校验拒绝，最终恰好有一项且 source ID 非空。
3. 请求中的 `media_source_id` 若提供，必须精确匹配这唯一 source；省略时只允许在数量为 1 时自动使用该 source。
4. Candidate Token 或 PreviewArtifact 绑定的 Item、source、语言和当前认证上下文仍然一致。
5. Item 或 source 在两次请求之间发生变化时，旧 Token/Artifact 不能继续驱动上游请求。

多 MediaSource 在真实样本验收前始终 fail closed。无论客户端是否提交了显式 source ID，只要当前 Item 的 source 数量大于 1，三个接口都返回 `409 d2_multisource_unsupported`，不得选择第一项、默认项或上一次使用的 source。零 source 或 source 结构无效分别返回 `media_source_unavailable` 或 `emby_invalid_response`，也不能降级为猜测。D1 的显式 source 浏览能力不自动扩大为 D2 的多源搜索、Fetch 或预览支持。

### 2.1 Emby 版本组读取规则

Emby 4.9.x 的版本组可能在列表查询中返回多个关联 Item，且默认响应只带当前默认 source。对选中的 Item 重新读取详情时，必须显式请求：

```text
Fields=Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources
```

加入 `AlternateMediaSources` 后，C92 的真实版本组详情响应为每个 Item 提供两个 `MediaSources`。D2 以该详情响应作为 source 绑定事实；不能根据列表中的 Item 数量、默认 source、同名标题或 `PresentationUniqueKey` 猜测 source。若备用 source 字段被 Emby 版本单独返回，客户端在 DTO 边界合并，只抑制两个字段之间的重复 source；同一字段内部的重复仍交给 source 校验拒绝，再进入本节的多源门禁。

## 3. 总体调用边界

```text
HTTP API
  │ 重新读取 Item，检查单源和功能开关
  ▼
D2 Orchestrator
  ├─ EmbyRemoteSubtitleProvider
  │    └─ Emby Remote Subtitle Search / Fetch
  ├─ CandidateStore
  │    └─ 短期 Token → 服务端候选映射
  ├─ SubtitleValidator / Parser
  │    └─ 大小、编码、格式、cue 和内容校验
  └─ PreviewArtifactStore
       └─ 项目私有短期缓存，不在媒体根目录
```

D2 只通过已配置的 Emby 基础地址和服务端 `X-Emby-Token` 访问 Emby。浏览器不能直接访问 Emby，不能提交 Provider 下载地址或候选原始 ID。

Provider 适配器只允许调用以下两个 Emby 只读接口：

```text
GET /Items/{ItemId}/RemoteSearch/Subtitles/{Language}
  ?MediaSourceId={server-selected-source-id}
  &IsForced={forced}
  &IsPerfectMatch=false
  &IsHearingImpaired=false
GET /Providers/Subtitles/Subtitles/{ServerOnlySubtitleId}
```

`MediaSourceId` 必须使用服务端重新读取到的唯一 source ID，即使客户端省略 `media_source_id` 也不能省略该上游 query 参数。请求体的 `forced` 一律映射为 `IsForced=true|false`；首轮固定显式传 `IsPerfectMatch=false` 和 `IsHearingImpaired=false`，客户端不能覆盖这两个值。参数名依据 [Emby Search API](https://dev.emby.media/reference/RestAPI/SubtitleService/getItemsByIdRemotesearchSubtitlesByLanguage.html)。`ServerOnlySubtitleId` 只能在服务端 Candidate 映射中使用；Fetch 依据 [Emby Fetch API](https://dev.emby.media/reference/RestAPI/SubtitleService/getProvidersSubtitlesSubtitlesById.html)，保存接口 [Emby Save API](https://dev.emby.media/reference/RestAPI/SubtitleService/postItemsByIdRemotesearchSubtitlesBySubtitleid.html) 明确禁止调用。D2 也禁止调用任何 Refresh 接口或其他写入接口。

## 4. HTTP API 契约

### 4.1 通用约定

- 路由继续位于 `/v1/*`。`POST /v1/auth/login` 使用部署者配置的管理员用户名和密码签发短期 HttpOnly 会话；其他 D2 路由接受有效管理员会话或现有独立 Bearer 自动化 Token。即使请求同时带有有效会话，`token` query 参数也会被拒绝；Bearer 使用 `security.api_auth_scopes` 的只读 scope 集合，Search 需要 `subtitle:search`，Fetch/Preview 需要 `subtitle:preview`，缺少 scope 返回 403；写 scope 当前被配置校验拒绝。
- 请求和响应使用 UTF-8 JSON。每个 D2 JSON 请求体上限为 8 KiB，未知字段应拒绝，避免客户端误以为 Provider 或搜索词已被上游接受。
- 失败响应沿用当前 envelope：

  ```json
  {
    "error": {
      "code": "stable_error_code",
      "message": "safe user-facing message"
    },
    "request_id": "request-id"
  }
  ```

- 错误消息只说明下一步，不包含 Emby 响应体、上游 URL、候选原始 ID、媒体路径、字幕正文或凭据。
- 所有 D2 成功响应都不返回 Emby 原始候选 ID、下载 URL、媒体路径、STRM 内容或原始 Provider JSON。
- `itemId` 和 `media_source_id` 只作为服务端重新读取和绑定的定位输入；日志不记录其原文。

### 4.2 Search

```text
POST /v1/media/{itemId}/subtitles/search
```

请求体：

```json
{
  "media_source_id": "optional-single-source-id",
  "language": "zh-CN",
  "forced": false
}
```

约束：

- `media_source_id` 可省略，但只有单 source 时才可省略；提供后必须匹配服务端刚读取的唯一 source。
- `language` 缺省时使用服务端配置的默认语言；首轮只接受项目已定义的中文语言别名并归一化为 canonical language。
- `forced` 缺省为 `false`。
- 不接受 `provider_name`、`search_term`、`download_url`、`media_path` 或 Emby 原始候选 ID。Provider 标签只能在已经返回的候选上做本地筛选和排序。

成功响应为 HTTP 200：

```json
{
  "language": "zh-CN",
  "candidates": [
    {
      "token": "opaque-candidate-token",
      "provider": "Thunder",
      "name": "display-only-name",
      "language": "zho",
      "format": "srt",
      "comment": "display-only-comment",
      "is_hash_match": false,
      "score": 0,
      "state": "ready",
      "expires_at": "2026-08-24T00:00:00Z"
    }
  ],
  "truncated": false,
  "capabilities": {
    "supports_provider_selection": false,
    "supports_custom_query": false,
    "supports_hash_match": true
  }
}
```

响应规则：

- 最多返回 20 个候选。超过上限时按服务端确定顺序裁剪，并将 `truncated` 设为 `true`；不能把裁剪伪装成完整结果。
- 候选名称、Comment、Provider 和 Reasons 必须长度受限并作为纯文本处理。UI 使用文本节点渲染，不把它们当 HTML。
- 无候选是 HTTP 200、空 `candidates`，不是服务端异常。
- Bridge 返回的 Provider 标签不表示本次请求可以精确调度 Provider；候选标签仍只用于展示、筛选和排序。

### 4.3 Fetch

```text
POST /v1/media/{itemId}/subtitles/fetch
```

请求体：

```json
{
  "candidate_token": "opaque-candidate-token"
}
```

服务端按以下顺序处理：

1. 验证认证、功能开关和单源 MediaContext。
2. 在当前认证上下文、Item、source、语言和实例范围内解析 Candidate Token。
3. 从服务端短期映射中取得 Emby 原始候选 ID；该 ID 不进入 HTTP 响应、日志或错误消息。
4. 通过 Emby Remote Subtitle Fetch 获取有界字节流。
5. 完成字幕验证、解析和 canonical UTF-8 归一化。
6. 成功后创建 PreviewArtifact，并只返回 Artifact 元数据。

成功响应为 HTTP 200：

```json
{
  "artifact_token": "opaque-preview-artifact-token",
  "provider": "Thunder",
  "language": "zh-CN",
  "format": "srt",
  "byte_length": 12345,
  "cue_count": 603,
  "content_sha256": "canonical-content-hash",
  "preview_ready": true,
  "expires_at": "2026-08-24T00:00:00Z"
}
```

Fetch 不返回原始字幕正文，也不返回可供浏览器再次请求的上游 URL。对同一个尚未过期且绑定一致的 Candidate Token，成功 Fetch 应保持幂等并复用已有 Artifact，不重复打击 Provider。一个候选失败只改变该候选状态，不使同次 Search 的其他候选失效。

### 4.4 Preview

```text
POST /v1/media/{itemId}/subtitles/preview
```

请求体：

```json
{
  "artifact_token": "opaque-preview-artifact-token",
  "offset": 0,
  "limit": 200
}
```

`offset` 缺省为 0，`limit` 缺省为 200，单次最多 500，整个 JSON 响应上限为 1 MiB。Preview 先调用一次受 3 秒预算约束的 `GetItem`，重新确认 Item、唯一 source、认证上下文和 Canary allowlist 绑定；随后只读取已经校验的 Artifact，不重新调用 Provider Fetch，不写媒体库。若校验失败，不读取 Artifact 内容并返回对应稳定错误码。

成功响应为 HTTP 200：

```json
{
  "format": "srt",
  "language": "zh-CN",
  "byte_length": 12345,
  "cue_count": 603,
  "content_sha256": "canonical-content-hash",
  "offset": 0,
  "limit": 200,
  "truncated": true,
  "cues": [
    {
      "index": 1,
      "start_ms": 1000,
      "end_ms": 2000,
      "text": "plain-text-cue"
    }
  ]
}
```

Preview 不提供返回整个 Artifact 的下载接口。`text` 是纯文本，客户端必须按文本显示；单条 cue、总 cue 数和响应字节数都受资源上限约束。

## 5. 稳定错误码

错误码是客户端可以依赖的分类，错误消息可以在不改变语义的情况下改写。上游 HTTP 状态和原始异常不透传。

| HTTP | `error.code` | 适用条件 |
|---:|---|---|
| 400 | `invalid_request` | JSON、字段、语言、分页或路径参数不合法 |
| 401 | `unauthorized` | 管理员会话/Bearer Token 缺失或错误 |
| 401 | `invalid_credentials` | 管理员登录用户名或密码错误 |
| 403 | `remote_search_disabled` | 功能开关关闭，或独立 D2 授权/Canary 条件未满足 |
| 403 | `canary_item_not_allowed` | Canary 窗口已开启，但 Item 不在服务端 allowlist |
| 404 | `media_not_found` | Item 不存在 |
| 404 | `candidate_invalid` | Candidate Token 不属于当前认证上下文、Item、source 或实例 |
| 404 | `artifact_invalid` | PreviewArtifact 不属于当前认证上下文、Item、source 或实例 |
| 409 | `d2_multisource_unsupported` | 当前 Item 的 MediaSource 数量大于 1 |
| 409 | `media_source_mismatch` | 请求 source 与重新读取到的唯一 source 不一致 |
| 410 | `candidate_expired` | Candidate Token 已超过 TTL |
| 410 | `artifact_expired` | PreviewArtifact 已超过 TTL |
| 413 | `subtitle_too_large` | Fetch 字节数超过上限 |
| 422 | `unsupported_media_type` | Item 不是 D2 首轮支持的 Movie/Episode |
| 422 | `subtitle_invalid` | 空内容、编码、结构、时间轴或内容安全校验失败 |
| 422 | `subtitle_format_unsupported` | 不是首轮允许的 SRT、ASS 或 SSA |
| 429 | `rate_limited` | 用户、Item 或全局并发/频率预算耗尽 |
| 429 | `login_rate_limited` | 管理员登录失败次数达到有界限速阈值 |
| 502 | `media_source_unavailable` | Emby 没有返回可用的唯一 source |
| 502 | `emby_unavailable` | Emby 网络不可用或返回不可分类的服务失败 |
| 502 | `emby_invalid_response` | Emby 响应形状、重定向或大小违反契约 |
| 502 | `provider_search_failed` | Search 无法得到可用的候选列表 |
| 502 | `candidate_fetch_failed` | 当前候选 Fetch 失败；其他候选仍保留 |
| 503 | `preview_store_unavailable` | 私有 Artifact 存储不可用 |
| 503 | `session_unavailable` | 管理员会话无法签发 |
| 504 | `emby_timeout` | Emby Search 请求超时 |
| 504 | `candidate_fetch_timeout` | 当前候选 Fetch 在预算内未完成 |
| 500 | `internal_error` | 未分类的服务端错误；不得带内部细节 |

`candidate_fetch_failed` 和 `candidate_fetch_timeout` 是候选局部结果，不得把整个 Search 结果集标记为失败。若未来增加“自动试下一个候选”的内部模式，内部状态仍必须使用同一分类，并遵守第 7 节的尝试预算。

## 6. Candidate Token 与 PreviewArtifact 生命周期

### 6.1 Candidate Token

- 由 `crypto/rand` 生成至少 32 字节随机值，使用无填充 base64url 作为不透明字符串。
- 只在服务进程内存中保存 Token 的摘要和服务端候选映射。映射包含 Emby 原始候选 ID、Item/source 绑定、语言、Provider 展示字段、签发时间、过期时间和状态。
- Token TTL 默认 10 分钟，从 Search 响应签发时开始计算；服务重启、认证上下文变化、Item/source 变化都会使其失效。
- 每次 Search 最多创建 20 个 Token；每个认证上下文最多保留 100 个活跃候选，全实例最多 1,000 个。超出时先拒绝新请求或回收已过期项，不能无限增长。
- 成功 Fetch 对同一 Token 幂等；失败候选只能按第 7 节的重试规则再次尝试，不能通过重复请求绕过尝试预算。
- Token 只出现在 JSON 请求/响应体，不放在 URL、query、Cookie、日志或异常文本中。

D2 不维护过期 tombstone。若映射仍在内存中但已超过 TTL，首次查找返回 `410 candidate_expired` 并立即删除该映射；清理任务已经删除、服务重启后没有映射、allowlist 窗口已关闭或绑定从未存在时，统一返回 `404 candidate_invalid`。因此客户端必须把两种结果都处理为“重新 Search”，不能依赖 410 永远可重复获得。Artifact 采用完全相同的规则：仍在存储索引中的已过期记录返回 `410 artifact_expired`，清理/重启后返回 `404 artifact_invalid`。

当前 D2 的 Bearer 与管理员会话都只代表同一单实例管理员边界，因此“认证上下文绑定”仍是实例范围，不等同于按人区分的账号绑定。未来若引入多用户身份，必须把服务端用户/会话身份加入绑定后才能宣称用户级隔离。

### 6.2 PreviewArtifact

- 只有通过 Validator 和 Parser 后才生成 Artifact。Artifact 保存 canonical UTF-8 字节、格式、语言、cue 数量、字节数、内容 SHA-256 和绑定信息。
- 原始 Provider URL、Emby 原始候选 ID、STRM 内容和认证信息不写入 Artifact。
- 文件存储在配置的专用、稳定 `d2.cache_dir` 下；该目录必须是显式绝对路径，位于媒体根目录、Web 静态目录和备份/回收目录之外。不能回退到临时目录，也不能使用文件系统根、媒体根、媒体祖先/子目录或符号链接路径。目录权限为 `0700`，文件权限为 `0600`。
- 单个 Artifact 的 canonical 内容上限为 4 MiB，单实例最多保留 256 个未过期 Artifact，每个认证上下文最多 64 个。
- Artifact TTL 默认 20 分钟，从生成时开始计算。清理任务至少每分钟运行一次；启动时新的实例范围必须使旧实例 Artifact 全部不可用，并回收旧缓存文件。
- Artifact Token 与当前认证上下文、Item、唯一 source 和语言绑定。Preview 前仍要重新确认 Item/source，不能只相信生成时的状态。
- D2 不把 Artifact 写入媒体库、SQLite 业务历史或备份目录。若未来记录审计，只保留结果码、耗时、字节数、cue 数和内容哈希等脱敏元数据。

## 7. Provider 失败隔离、超时、重试与尝试预算

### 7.1 Provider 边界

V1 使用 Emby Remote Subtitle Bridge。Emby Bridge 不提供请求级 Provider 选择或自定义搜索词，因此 Provider 标签仅用于结果展示、筛选和排序。D2 不为了实现标签而偷偷改用 Native Provider。

Search 是一次有界的 Emby 请求；Search 阶段不对所有候选自动 Fetch。Fetch 以 Candidate Token 为单位，候选之间相互隔离。

### 7.2 默认预算

| 资源 | D2 默认上限 |
|---|---:|
| 单次 `GetItem` 重读超时 | 3 秒 |
| Emby Search 子预算 | 15 秒 |
| 单候选 Provider Fetch 子预算（含至多一次重试） | 20 秒 |
| Search 端到端请求预算 | 20 秒，含 `GetItem`、Search、候选投影和缓存 |
| Fetch 端到端请求预算 | 25 秒，含 `GetItem`、Provider Fetch、Validator/Parser 和 Artifact 写入 |
| Preview Artifact 读取/投影子预算 | 2 秒 |
| Preview 端到端请求预算 | 5 秒，含一次 `GetItem` 和本地 Artifact 读取 |
| 单候选最大上游尝试 | 2 次，首次加至多 1 次重试 |
| 未来自动试候选的不同候选数 | 3 个，D2 公开接口不自动启用 |
| 全实例同时进行的 Search/Fetch | 4 个 |
| 同一 Item 同时进行的远程操作 | 1 个 |
| Search 每认证上下文频率 | 10 次/分钟 |
| Fetch 每认证上下文频率 | 20 次/分钟 |
| Preview 每认证上下文频率 | 60 次/分钟 |
| Provider Search 响应体 | 1 MiB |
| 客户端 `Retry-After` | 固定不超过 1 秒 |

当前服务的 `http.Server.WriteTimeout=30s` 可以容纳 D2 最大 25 秒 Fetch 预算，并保留至少 5 秒响应写出余量；实现不得把 Fetch 预算提高到 30 秒或以上。若后续修改 `WriteTimeout`，必须保持它严格大于 25 秒并重新验证。每个阶段使用独立 context deadline，不能用默认的 30 秒 Emby 客户端超时覆盖 D2 子预算。并发和频率超限统一返回 `429 rate_limited`，响应的 `Retry-After` 只允许整数秒 `0` 或 `1`，不得排队到无界内存。

### 7.3 重试分类

只对明确的临时失败重试一次：请求超时、连接重置和 HTTP 429。429 的等待时间最多 1 秒；不能无界遵循上游 `Retry-After`。以下情况不自动重试：

- 上游 4xx，包括候选地址失效导致的错误。
- Emby 对候选返回的未分类 5xx；不能因为一个候选的 500 就重复打击同一候选。
- 空内容、HTML/JSON、压缩或二进制响应、编码错误、格式错误和 Parser 校验失败。
- 超过字节、cue、行长度或并发资源上限。

一次候选失败只更新该候选的状态和脱敏失败分类。Search 缓存中的其他候选继续可见，不能因为第一个候选失败而清空列表、取消其他候选或把 Search 误判为全局失败。

## 8. 字幕内容校验与资源上限

Fetch 得到的内容必须先有界读取，再解码和解析。不能因为响应声明了较小的 `Content-Length` 就跳过实际读取上限。

- 原始和 canonical 内容均不得超过 4 MiB。
- Emby Search 响应体不得超过 1 MiB；投影后的 `provider` 最多 64 字节，`format`、语言和状态字段最多 32 字节，`name`、`comment` 和其他展示文本各最多 512 字节；Reasons 最多 8 项、每项最多 128 字节。
- 只接受首轮约定的 SRT、ASS、SSA；扩展名、Emby 格式字段和内容特征不一致时拒绝。
- 支持约定的 UTF-8/BOM 及 UTF-16 字幕输入，统一转换为 canonical UTF-8；非法编码拒绝。
- 拒绝空内容、HTML、JSON、压缩包、明显的二进制响应和 Provider 错误页。
- Parser 必须校验 cue 结构、时间戳顺序、结束时间不早于开始时间和总 cue 数。总 cue 数上限为 10,000，单行文本上限为 8 KiB。
- 预览单次最多返回 500 个 cue，JSON 响应最多 1 MiB；超过任一上限时只返回完整 cue，设置 `truncated=true` 和总 `cue_count`，不能静默丢失校验结果。
- Provider 的名称、标题、Comment 和 cue 文本都按纯文本返回并限制长度；前端不得 `innerHTML` 渲染。
- `content_sha256` 是 canonical UTF-8 字节的 SHA-256，用于同一 Artifact 的内容确认，不代表允许把内容写入媒体库。

## 9. SSRF、日志和候选 ID 防泄漏

### 9.1 SSRF 与网络边界

- Emby 基础地址只能来自受校验的服务端配置，不能由请求体、Candidate Token 或 Provider 响应改变。
- Emby URL 必须是无凭据、无 query、无 fragment 的 `http` 或 `https` 地址；禁止跟随重定向。D2 不直接请求候选的下载 URL。
- Fetch 只向固定 Emby API 路径发起请求，并在服务端设置 `X-Emby-Token`；凭据不进入 URL、浏览器或错误消息。
- 请求体不接受 `url`、`media_path`、`strm_url`、`proxy` 或任意 header。应用不读取 STRM 内容，不把 `MediaSource.Path` 交给 HTTP 客户端。
- 响应体、响应头、连接数和超时全部有界；Provider 返回的 URL 只作为被拒绝的非信任数据，不能再触发二次请求。

### 9.2 日志与响应脱敏

允许记录的字段限于固定 route label、request ID、HTTP 状态、稳定错误码、耗时、候选数量、尝试次数、Artifact 大小、cue 数和缓存命中/过期结果。Item ID、source ID、Candidate Token、Artifact Token 如需关联，只记录使用独立进程密钥计算的短 HMAC 引用，不能记录原文。

禁止记录或返回：

- Emby API Key、应用 Bearer Token、Cookie、认证头和 Secret 文件内容。
- Emby 原始候选 ID、Provider 原始 JSON、完整下载 URL、query、响应体和上游异常文本。
- Item/MediaSource 原始路径、STRM 内部地址、字幕正文、标题中未经限制的隐私数据。

应用日志、反向代理安全日志和测试失败输出都要遵守同一规则。请求日志使用固定路由模板，不记录完整请求行和 query。

## 10. `remote_search_enabled` 与 Canary

默认配置必须得到服务端和操作流程的双重约束：

1. 普通部署和 Canary 关闭状态下，`remote_search_enabled=false`、`d2.canary.enabled=false`。三个接口在访问 Emby 前统一返回 `403 remote_search_disabled`，上游请求数必须为零。
2. 本轮 D2-B 只实现“服务端 Canary Item allowlist”方案。allowlist 文件位于媒体根目录和仓库之外的受保护配置目录，一行一个精确 Item ID，权限按 Secret 文件处理；不进入 Git、响应、日志或前端。
3. 获得独立 D2 授权后，Canary 窗口临时设置 `remote_search_enabled=true`、`d2.canary.enabled=true`，并要求 allowlist 非空。服务启动或配置加载时如果 `remote_search_enabled=true` 但 Canary 未启用、allowlist 缺失或为空，必须 fail closed，拒绝启动或保持所有 D2 请求关闭；不能退化成全库开放。
4. Canary 请求在服务端重新读取 Item 后，按规范化后的真实 Item ID 做精确 allowlist 匹配。未命中返回 `403 canary_item_not_allowed`；管理员会话和 Bearer Token 都不能绕过该检查。Search、Fetch、Preview 都必须执行该检查，Token/Artifact 还要绑定 allowlist generation，allowlist 变化后旧状态失效。
5. Canary 窗口结束立即同时关闭 `d2.canary.enabled` 和 `remote_search_enabled`，之后所有 D2 请求恢复 `remote_search_disabled`。Canary 通过后是否长期开放另行授权，并需新的契约/配置审查；本轮不实现无 allowlist 的全库长期开放模式。

安全配置示例（默认关闭，只展示受保护路径形状）：

```yaml
features:
  remote_search_enabled: false

d2:
  # 专用、稳定、绝对路径；必须位于所有媒体映射之外。
  cache_dir: /var/lib/subbridge/d2-preview-cache
  canary:
    enabled: false
    # 一行一个精确 Item ID，通过只读 Secret 注入，文件源位于媒体映射之外。
    item_allowlist_file: /run/secrets/d2_canary_items

path_mappings:
  - emby: /media
    local: /media
```

启用 D2 时必须同时保留稳定的 `cache_dir` 和非空 Canary allowlist；不能使用 `/`、`/srv`、媒体映射的根/祖先/子目录、临时目录或通过 symlink/reparse point 到达其他位置的路径。示例路径不代表本机目录，也不应把真实 Item ID 写入仓库。

默认 D1 Compose 不依赖 D2 cache 或 allowlist 文件。获得独立授权并准备宿主专用目录、权限和 Secret 后，才显式合并 `deploy/compose.d2-canary.example.yaml`：

```sh
docker compose -f compose.example.yaml -f compose.d2-canary.example.yaml config --quiet
docker compose -f compose.host-network.example.yaml -f compose.d2-canary.example.yaml config --quiet
```

overlay 只增加精确的可写 cache bind 和 `d2_canary_items` file-source Secret，不改变 rootfs 的 `read_only: true` 或 `/media` 的只读挂载；overlay 本身也不替代配置中的 D2 开关和 Canary allowlist 门禁。

Canary 只允许 allowlist 中已批准的单源 Movie/Episode，且必须验证搜索、候选局部失败、Fetch、Validator、Preview、日志脱敏以及“无媒体写入、无 Refresh、无 D3 路由”。真实多源样本到位前，不得通过 Canary 结论启用多源搜索。allowlist 不是客户端提示，而是服务端每个接口的硬门禁。

## 11. D2 实现阶段的模块变更清单

以下是 D2 的模块边界和实现状态。本轮已完成后端、安全门禁、内嵌 UI 和 Fake Emby 测试；UI 不扩大 D2 的服务器端门禁或默认关闭状态。

| 模块 | D2 工作 |
|---|---|
| `internal/config` | 增加 D2 超时、专用稳定缓存目录、大小/并发/TTL、`d2.canary.enabled` 和受保护 `item_allowlist_file` 配置；默认关闭，拒绝“远程搜索开启但 cache_dir/Canary allowlist 缺失”以及私有路径与媒体映射 overlap 的配置 |
| `internal/domain` | 增加服务端 Candidate、Artifact、cue 和状态模型；原始候选 ID 不能带 JSON 序列化标签 |
| `internal/media` | 复用 Item/MediaSource 重新读取和绑定逻辑，增加 D2 的“恰好单 source”前置检查；不改变 D1 的多源浏览语义 |
| `internal/embyclient` | 详情读取固定请求 `AlternateMediaSources` 并保留完整 source 列表；增加 Remote Subtitle Search/Fetch 的只读调用、强制传入服务端 `MediaSourceId`、固定 `IsForced`/`IsPerfectMatch`/`IsHearingImpaired`、路径转义、响应大小上限、分阶段超时和错误分类；不增加 Save/Refresh/Write 方法 |
| 新增 `internal/subtitleprovider` | 实现 `EmbyRemoteSubtitleProvider`、ProviderCapabilities、候选投影和候选级错误/重试分类 |
| 新增 `internal/subtitle` 或等价 Parser 层 | 实现 Validator、格式/编码识别、canonical UTF-8、SRT/ASS/SSA Parser 和 cue 资源上限；只选择性复用已确认的 Parser 核心 |
| 新增 `internal/preview` | 实现 CandidateStore、PreviewArtifactStore、Token 绑定、TTL、无 tombstone 的过期/清理状态、allowlist generation、幂等 Fetch、清理、并发和缓存目录权限 |
| `internal/httpapi` | 增加 Search/Fetch/Preview 路由、8 KiB 请求体限制、稳定错误 envelope、限流、单源 fail-closed、Canary allowlist 检查和日志字段；不注册 D3 写路由 |
| `internal/httpui` | 增加候选展示、Fetch/Preview 状态、过期和局部失败提示；只用纯文本渲染，不保存 Token 到 localStorage/sessionStorage |
| `cmd/server` | 组装上述依赖、加载受保护 Canary allowlist、创建实例级缓存作用域、启动清理任务并在关闭时停止；不改变部署或远端服务 |
| `scripts/verify.ps1` 与测试夹具 | 加入 D2 静态、单元、Fake Emby 集成和安全边界检查；真实 Canary 仍单独执行 |

`inventory`、`pathmap`、Installer、Reconciler 和 D3 写入模块在 D2 不应承担新写入职责。D2 的缓存写入不能复用媒体目录或 Installer 的备份目录。

## 12. 实现阶段测试矩阵

### 单元测试

- Candidate Token 和 Artifact 的随机性、摘要存储、TTL、绑定、重启失效、过期清理和成功 Fetch 幂等。
- Candidate 数量、活跃项、Artifact 字节数、cue 数、行长度、并发数和频率上限。
- 单源 Movie/Episode 通过；零 source、重复 source、多个 source、错误 source、非 Movie/Episode 和 Item/source 变化均安全拒绝。
- Provider 错误分类：timeout、connection reset、429 只重试一次；4xx、未分类 5xx、格式错误和校验错误不重试；未来 fallback 最多三个不同候选。
- UTF-8/BOM、UTF-16、SRT、ASS、SSA、空内容、HTML/JSON、压缩/二进制、非法时间轴和超限内容。
- canonical hash、cue 分页、纯文本投影和过长 Provider 字段截断。
- URL/redirect/`MediaPath`/query 凭据拒绝，以及错误 envelope 不含敏感数据。

### Fake Emby 集成测试

- `remote_search_enabled=false` 时三个 D2 接口均在访问上游前返回 `remote_search_disabled`，上游请求数为零；`remote_search_enabled=true` 但 Canary 未启用或 allowlist 缺失时配置 fail closed。
- Canary allowlist 命中时有效管理员会话或 Bearer 可以执行 D2；未命中 Item 返回 `403 canary_item_not_allowed`，不能通过改写客户端请求绕过。
- 单源 Movie/Episode Search 请求始终带服务端唯一 `MediaSourceId`，并明确传 `IsForced`、`IsPerfectMatch=false`、`IsHearingImpaired=false`；响应不含候选原始 ID/URL，结果上限和 `truncated` 正确。
- 多 source 对 Search、Fetch、Preview 都返回 `409 d2_multisource_unsupported`，即使提交了显式 source ID 也不猜测；Fake 409 不能替代真实多源验收。
- Item 不存在、错误 source、非支持类型、Emby timeout、畸形响应、重定向和超大响应映射到稳定错误码。
- 候选 A Fetch 失败而候选 B 成功时，A 的失败不清空 Search 结果，B 仍可生成 Artifact；验证候选状态和尝试次数。
- timeout/429/connection reset 的请求次数最多为两次；候选 4xx、未分类 500、invalid content 不重复调用。
- 成功 Fetch 只写私有缓存，不产生媒体目录文件、不调用 Refresh/Save endpoint；Preview 只额外调用一次 `GetItem` 做绑定和 allowlist 检查，不调用 Provider Fetch。
- Candidate/Artifact 在映射仍存在但 TTL 已过期时返回一次 410，清理、重启、allowlist 变化或未知状态返回 404；错误认证上下文、错误 Item/source、重放和服务实例变化均拒绝；成功 Fetch 重放不重复打击上游。
- 8 KiB 请求体、1 MiB Search/Preview 响应、Provider 字段长度、4 MiB字幕、10,000 cue、500 cue 页、1 秒 `Retry-After` 和 30 秒 `WriteTimeout` 下的 25 秒 Fetch 端到端预算均有边界测试。
- 超大字幕、非法编码、HTML/JSON、压缩内容、cue/行长度超限和预览分页均正确处理。
- 应用日志、Fake Emby 记录、反向代理样例和响应中没有认证信息、原始 ID、URL、路径或字幕正文。

### 真实 Canary 与后续多源门禁

D2 代码和 Fake Emby 通过后，才可在独立授权下对服务端 allowlist 中批准的单源 Movie/Episode 做有界真实 Canary。必须分别记录静态检查、Fake Emby、Emby 直连和真实客户端结果；D2 只需要证明搜索、Fetch、校验和预览的只读链路，不把 Refresh 或客户端写入作为 D2 通过条件。

真实多 MediaSource 样本已出现；C92 的真实 API/source 对应和 D2 409 安全拒绝已记录在 [D2 多源真实 API Canary](d2-multisource-c92-canary-acceptance-20260825.md)。在浏览器 UI source 点击和多源正向实现/Canary 完成前，仍不能用本次安全拒绝、合成 Fake 或单源 Canary 宣称多源支持。

## 13. Knowledge Review

任务或阶段：D2-B1 后端、安全门禁和 Fake Emby 测试

验证范围：`AGENTS.md`、当前架构、ADR-003、ADR-005、维护经验、总体规划、文档索引、Knowledge Review 模板，以及 D2 `internal/config`、`internal/domain`、`internal/embyclient`、`internal/httpapi`、`internal/d2`、`internal/preview`、`internal/subtitle`、`internal/subtitleprovider`、`cmd/server` 和 Fake Emby 集成测试。

Knowledge Findings

- 新增约束　D2 对外区分 Search、Fetch、Preview；Canary 采用服务端 Item allowlist；Fetch 只生成短期 Artifact，Preview 只重读一次 Item/source 而不重新 Provider Fetch；三个接口在多源条件下统一 fail closed；启用 D2 必须使用显式稳定 `cache_dir`，启动时回收该目录中的旧 Artifact 文件。
- 隐蔽坑　私有路径安全检查必须双向判断，媒体祖先与媒体子目录都属于 overlap；启动清理前还要拒绝文件系统根和 cache_dir 本身/父链的 symlink/reparse point，避免只读 D2 通过 `chmod` 或清理误触媒体/系统路径。
- 隐蔽坑　Emby Bridge 的候选搜索和 Provider 选择是两个不同能力；一个候选的 500 可能来自候选自身失效，不能用通用 5xx 重试或使其他候选失效；缓存清理后也不能承诺永远返回 410。
- 新增约束　Emby 4.9.x 的版本组详情必须显式请求 `AlternateMediaSources`；客户端在 DTO 边界合并备用 source，并只抑制主、备用字段之间重复的非空 ID，同一字段内部的重复必须保留给源校验拒绝。C92 实测记录见 [`docs/d2-multisource-c92-sample.md`](d2-multisource-c92-sample.md)。
- 被证明错误的假设　D1 `GetItem` 只接受 Movie/Episode 会阻止 D2 对非支持类型返回稳定错误；D2 需要先保留详细 Item 类型，再由 D2 门禁判断。Provider 展示语言和绑定 canonical language 也不能共用一个语义。
- 建议沉淀项　候选级失败、短期 Token/Artifact、allowlist generation、4 MiB 字节上限、两次候选尝试、Search/Fetch/Preview 预算，以及稳定缓存目录的启动回收和路径安全门禁已进入本轮代码与测试；通用失败隔离经验与既有 `docs/lessons-learned.md` 去重后不重复追加。

证据

- 代码　D2 Service、Candidate/Artifact Store、Validator/Parser、Remote Subtitle Provider、固定只读 Emby 客户端、配置门禁和 HTTP 路由。
- 测试　D2 单元测试和 Fake Emby 集成测试覆盖开关关闭零上游请求、固定 query、候选失败隔离、Fetch 幂等、Preview 单次 GetItem、多源 409、请求体限制、无 Save/Refresh/媒体目录写入和日志脱敏；配置/Store 测试覆盖缺失 cache_dir、根目录、媒体双向 overlap、安全同级目录、symlink 防护和稳定目录重启回收；Compose schema 测试证明基础 D1 Compose 无 D2 依赖、overlay 合并后才有专用写挂载和 allowlist Secret。
- 官方文档　Emby Search、Fetch 和保存接口的官方 REST API 页面，已用于固定 query 参数和 D2 禁止调用的 POST 边界。
- 实际运行、日志或可复现结果　已运行 `scripts/verify.ps1`（gofmt、go vet、全包测试和构建）、`go test -count=3 -shuffle=on ./...`、`go build ./...`、`git diff --check`、项目 Markdown 检查和 Fake Emby httptest；Compose YAML/schema/invariant 测试通过。当前 Windows 没有 `docker`/`docker compose`，未执行真实 Compose merge/up；未启用真实远程搜索、未访问真实 Provider、未部署或重启。`go test -race ./...` 受当前 Windows 环境 `CGO_ENABLED=0` 且未提供 C 编译器限制，未宣称 race 通过。

去重检查

- 已搜索的文档和关键词　`AGENTS.md`、`docs/architecture.md`、`docs/lessons-learned.md`、`docs/adr/`、`D2`、`搜索预览`、`Fetch`、`PreviewArtifact`、`remote_search_enabled`、`MediaSource`、`候选`、`Token`。
- 是否更新已有结论　是。保留本文件作为 D2 契约唯一详细来源，并更新当前实现状态；实现评审细节见 [D2-B1 后端实现评审](d2-b1-backend-implementation-review.md)。

分流判断

- `docs/lessons-learned.md`　不需要更新
- `docs/architecture.md`　已更新 D2-B1 后端实现边界和证据链接
- `docs/adr/`　不需要新增 ADR；D2 入口和多源独立门禁已经由 ADR-005 决定
- `LOCAL_OPERATIONS.md`　不需要更新

未验证范围与残余风险

- 真实单源 Emby/Provider Search/Fetch/Preview 和真实客户端读取未在本轮运行；Fake Emby 证据不能替代真实 Canary。
- 4 MiB、TTL、并发和频率是本设计的初始安全预算，需在代码压力测试和授权 Canary 中按证据调整，不能当作当前运行时能力。
- 真实版本组样本已找到，API/source 对应和多源安全拒绝 Canary 已完成；在浏览器 UI source 点击以及多源正向实现/独立 Canary 完成前，多源搜索、Fetch 和预览继续不可用。
