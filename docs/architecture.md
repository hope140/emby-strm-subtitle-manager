# 当前架构

本文只描述截至 2026 年 8 月 24 日已经由官方接口、当前源码、自动化检查和 Gate 0 真实运行确认的内容。D1 的 Go 后端只读切片已在本地实现；Docker 镜像、Compose 和真实服务器部署仍未验收。Installer、搜索预览和写入能力仍属于后续阶段。

## D1 本地实现

当前代码由 `cmd/server` 和以下内部模块组成：`config`（非密配置与 Secret 读取）、`domain`（Emby 领域 DTO）、`embyclient`（仅只读 Emby 调用）、`media`（MediaContext 与 MediaSource 选择）、`pathmap`（跨平台路径映射和目录安全边界）、`inventory`（字幕清单）以及 `httpapi`（只读 HTTP 层）。

服务公开 7 个 GET 路由，3 个运维路由和 4 个业务路由：

```text
运维：GET /livez
      GET /readyz
      GET /v1/health
业务：GET /v1/emby/libraries
      GET /v1/emby/items
      GET /v1/media/{itemId}
      GET /v1/media/{itemId}/subtitles
```

`/livez` 只表示进程存活；`/readyz` 会对 Emby 发起受超时和缓存控制的真实只读探测；`/v1/health` 返回版本、功能开关和当前 Emby readiness 状态。业务路由始终由服务端使用 ItemID 重新查询 Emby。响应只投影展示字段，不暴露 Emby 绝对路径、字幕正文、认证参数或 STRM 内部地址。

应用 API Key 与独立的 identity secret 分离。identity secret 由 Inventory 用于生成稳定、不可逆的本地字幕标识，不能替代或复用 Emby API Key，也不会进入响应和普通日志。

管理 API 使用独立的 `security.api_auth_token_file` Bearer Token。`/livez` 与只返回极小状态的 `/readyz` 保持公开；所有 `/v1/*` 路由均要求 `Authorization: Bearer <token>`，缺失或错误统一返回 401、`WWW-Authenticate: Bearer` 和脱敏错误 envelope。Token 不接受 query 参数，也不写入日志或响应，并且不能复用 Emby API Key 或 identity secret。

MediaContext 对单源自动选择，对多源要求显式 `media_source_id`，不会猜测列表第一项。PathMapper 支持 POSIX、Windows drive 和 UNC 形式，采用规范化、最长前缀匹配及目录 containment 检查；路径不安全、未映射或目录不可用时返回降级状态和稳定 warning。Inventory 只枚举受控目录、读取文件元数据并合并 Emby MediaStreams，绝不读取 STRM 内容、媒体正文或字幕正文。

本地 `scripts/verify.ps1` 已覆盖格式化、`go vet`、全包测试和构建。该结果仅证明源码和自动化测试，不证明 Docker 镜像、Compose 配置或真实 Emby Canary 已通过。

## 当前系统边界

```text
115 / CD2
   │
   ▼
CMS 整理目录和生成 STRM
   │
   ▼
Emby 4.9.5.0
   ├─ 媒体索引与播放
   ├─ Remote Subtitle API
   └─ 外部字幕识别与字幕流
          │
          ├─ MeiamSub.Assrt 1.0.16.0
          └─ MeiamSub.Thunder 1.0.16.0
```

版本号来自 Gate 0 的当日实测。后续任务必须重新检查当前版本，不能把本节当作实时状态。

## 已验证的远程字幕数据流

```text
服务端 API Key
   │
   ▼
GET /Items/{Id}/RemoteSearch/Subtitles/{Language}
   │
   ▼
RemoteSubtitleInfo[]
   │
   ▼
GET /Providers/Subtitles/Subtitles/{Id}
   │
   ▼
字幕字节流
```

独立 API Key 请求已经完成 Search 和 Fetch。请求不需要浏览器 Cookie 或登录会话。

搜索请求无法指定单次调用使用哪个 Provider，也无法传入自定义搜索词。额外的 `ProviderName` 和 `SearchTerm` 参数在 Gate 0 中没有改变候选结果。

## Provider 行为

MeiamSub.Assrt 和 MeiamSub.Thunder 都能通过 Emby 返回可用候选。Thunder 搜索结果中可能包含已经失效的上游下载地址。

Gate 0.1 的第一个 Thunder 候选在 Fetch 时遇到上游 HTTP 404。Meiam 将其包装为 `InvalidDataException`，Emby 对外返回 HTTP 500。同次搜索的第二和第三个候选均成功返回有效 SRT。

V1 因此采用候选级失败模型。上游 4xx 和内容无效直接标记候选失败，临时网络错误最多重试一次，其他候选继续可用。

## STRM 边界

Gate 0 在受控搜索与 Fetch 期间监测已知媒体代理端口，没有观察到访问 STRM 内部媒体 URL 的流量。

当前 Meiam Thunder 源码会对可读的 MediaPath 尝试计算 CID。Gate 0 没有执行进程级文件打开追踪，因此 STRM stub 读取属于源码确认的兼容行为，不属于本次 live trace 结果。

未来应用自身不得读取 STRM 内容或计算 CID。是否维护 Meiam `.strm` 跳过补丁留作兼容性决定。

## 外部字幕写入与读取

Gate 0 已经验证 Emby 能把成功 Fetch 的 Thunder 候选写入 STRM 同目录，并识别为外部 SRT。通过下面的字幕流接口读取时，内容与文件系统字幕归一化后一致。

```text
/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
```

本次同路径替换测试显式使用 `Cache-Control: no-cache`，只证明该请求范围内没有返回旧字幕。V1 继续采用新版本文件名写入、验证成功后归档旧文件的策略。

## V1 选择与代码路线

V1 通过 Emby Bridge 使用 Meiam Provider。Native Thunder 和 Native ASSRT 暂缓，详见 [ADR-001](adr/001-v1-uses-emby-remote-subtitle-bridge.md)。

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。ADR-002 已接受，选择新建轻量 Go 后端，选择性复用 ASS/SRT Parser 核心、语言与命名处理经验、相关测试思路、Emby HTTP 调用经验和少量无状态前端组件。

ChineseSubFinder 的旧扫描器、Cloud/SubtitleBest 下载链、Provider Hub、Cron/PreJob、旧任务队列、按视频物理路径保存和视频 Hash 逻辑不进入新运行时。搜索、Installer 和写入能力仍属于后续阶段。

Phase 2 的交付顺序和默认部署边界已记录在 [ADR-003](adr/003-phase2-milestones-and-deployment.md)：先做 D1 只读 Canary，再做 D2 搜索预览，最后对专用样本做 D3 Add。D1 的代码和自动化门禁已完成，Docker 产物和真实 Canary 仍待验收；具体 API、安全边界和门禁见 [只读 Canary 验收定义](phase2-readonly-canary.md)。

## 证据

- [Gate 0 实测报告](../GATE0_REPORT.md)
- [总体规划](../Emby_STRM_Subtitle_Manager_Master_Plan_Revised.md)
- [ADR-002：项目代码路线](adr/002-project-codebase-route.md)
- [Phase 1 基线报告](../BASELINE.md)
- [ChineseSubFinder 复用矩阵](../CSF_REUSE_MATRIX.md)
- [Emby SubtitleService API](https://dev.emby.media/reference/RestAPI/SubtitleService.html)
- [Meiam ThunderProvider](https://github.com/91270/MeiamSubtitles/blob/master/Emby.MeiamSub.Thunder/ThunderProvider.cs)
