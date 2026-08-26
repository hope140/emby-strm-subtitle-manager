# SubBridge（SB，字幕桥）— Gate0 实测报告

日期：2026-08-24

## 结论

Gate0 **正式通过**。

核心链路已经在真实登录的 Emby Web 页面、独立 API Key 请求和 C92 实例上跑通：搜索、候选 Fetch、写入外置字幕、文件系统核验、Emby 直连读取、既有媒体代理链路核验，以及同路径替换后的缓存复测均有结果。

Gate0.1 已完成三个收口项：

1. 计划中的 API Key 在不携带浏览器会话的独立请求中具备 Search 和 Fetch 权限，V1 后端采用服务端 API Key 模型。
2. Thunder HTTP 500 已定位为单个候选的上游字幕地址 HTTP 404，Provider 将其包装成 `NotFound`/`InvalidDataException` 后由 Emby 返回 500；不是网络超时、429 或格式校验失败。
3. 缓存结论已收窄为本次 `no-cache` 复测范围；V1 继续使用“新版本文件名写入、验证成功后归档旧文件”的安全策略，不恢复默认同路径覆盖。

本次未执行进程级 per-file `open()` 追踪。STRM stub 是否被读取计算 CID/hash 只作为现有 Meiam 兼容行为记录，不能写成“本次实时打开已证明”。atime 未变化也因 `relatime` 规则而不具备判定力。

## Gate0.1 补充实测

### API Key 独立验证

从项目目录中的 `embyapi` 文件读取 Key，仅通过 `X-Emby-Token` 请求头调用公网 Emby 接口；请求使用 curl，不携带浏览器 Cookie 或 Web 会话。

- Search：HTTP 200，返回 JSON 数组，共 20 个候选。
- 响应候选字段已确认包含 `Id`、`ProviderName`、`Name`、`Format`、`Author`、`Comment`、`IsHashMatch`、`Language`；候选值和 ID 未写入报告。
- Fetch：使用同一次 Search 的前三个 Thunder 候选，每个只请求一次，不重试。
- 第 1 个：HTTP 500，根因见下方日志定位。
- 第 2、3 个：HTTP 200，均解析为有效 UTF-8 SRT，各 603 个 cue。

因此，Gate0 范围内 API Key 权限足够，V1 后端采用 API Key；API Key 只留在服务端配置，不进入前端、普通日志或候选 Token。

### Thunder HTTP 500 定位

本次复测时间为 2026-08-24 10:48:48（中国标准时间），Emby 日志与复测时间对应：

| 阶段 | 结果 |
|---|---|
| Search | Thunder 上游 HTTP 200，返回 20 个候选 |
| 第 1 个候选 Fetch | 上游字幕地址 HTTP 404，耗时约 214ms；Thunder 记录 `NotFound`，Emby 最终返回 HTTP 500 |
| 第 2 个候选 Fetch | 上游 HTTP 200，Thunder 记录 `Response -> OK`，有效 SRT |
| 第 3 个候选 Fetch | 上游 HTTP 200，Thunder 记录 `Response -> OK`，有效 SRT |

日志中的异常类型为 `System.IO.InvalidDataException: MeiamSub.Thunder subtitle download failed`，调用栈位于 `ThunderProvider.DownloadSubAsync`。根因是候选对应的上游资源已失效或不存在，属于候选失效/上游 404；没有观察到网络超时、429 或成功响应后的格式校验失败。

### V1 候选失败策略

- 任意候选失败只标记该候选，不让整个 Search 结果报废。
- 超时、429、连接重置等临时网络错误最多重试一次。
- 上游 4xx（包括本次 404）、内容无效和格式校验失败直接标记失败，不重试。
- 手动模式保留其余候选供用户选择。
- 自动模式最多按结果顺序尝试前三个候选。
- 本次结果支持继续沿用 Emby Bridge，不因单个失效候选切换 Native Provider。

## 环境与样本

| 项目 | 实测值 |
|---|---|
| Emby 实例 | C92 |
| Emby Server | 4.9.5.0 |
| MeiamSub.Assrt | 1.0.16.0，已加载 |
| MeiamSub.Thunder | 1.0.16.0，已加载 |
| 样本电影 | Item `63632`，“骗骗”喜欢你，Item.Path 为 `.strm` |
| 样本剧集 | Item `155302`，“权力的游戏前传：龙族” S02E02，Item.Path 为 `.strm` |
| 登录入口 | `<PRIVATE_EMBY_BASE_URL>` |

未在报告、日志或其他项目文件中记录 API Key、Emby Token、候选 opaque ID 或带认证参数的 URL；Key 仅保留在用户提供的本地 `embyapi` 文件中，该文件已加入 `.gitignore`。

## Gate0 验收矩阵

| 编号 | 验收项 | 结果 | 证据摘要 |
|---:|---|---|---|
| 1 | 固定版本 | PASS | Server 4.9.5.0；Assrt/Thunder 当前加载版本均为 1.0.16.0。 |
| 2 | 选择电影与剧集 STRM | PASS | Item `63632` 与 `155302` 的 API Path 均以 `.strm` 结尾。 |
| 3 | Emby 搜索与权限 | PASS | 电影搜索 HTTP 200、19 个候选；剧集搜索 HTTP 200、20 个候选。 |
| 4 | Fetch 实际字幕文本 | PASS | 电影候选 Fetch HTTP 200，UTF-16LE ASS，285 个 cue；Gate0.1 中 Thunder 前三个候选为 500/200/200，其中 500 已定位为上游 404，后两个为有效 UTF-8 SRT，各 603 个 cue。 |
| 5 | STRM 内部 URL 无请求 | PASS | 在电影搜索+Fetch 的受控操作期间，对已知媒体代理端点 `<MEDIA_PROXY_ENDPOINT>` 抓包，捕获 0 个数据包。 |
| 6 | Meiam 是否读取 STRM stub 算 CID/hash | NOTE | 记录为“当前兼容行为会读取 stub 计算 CID/hash”；本次没有进程级文件打开追踪，atime 观察未变化且不可判定。 |
| 7 | Provider 与 keyword 请求边界 | PASS | 增加 `ProviderName` 与 `SearchTerm` 后仍 HTTP 200、仍 20 个候选，候选可见字段与基线一致，说明这些额外参数被忽略，API 没有提供本次请求级筛选能力。 |
| 8 | 隔离字幕写入与读取 | PASS | 使用 Thunder 成功候选写入剧集；Emby 识别出 1 条外部 SRT。目标文件 43,614 bytes、UTF-8 BOM+CRLF、603 个 cue；Emby 直连 Stream.srt HTTP 200，返回内容与文件内容归一化后相同。 |
| 9 | 同路径缓存复测 | PASS（范围限定） | 在浏览器缓存禁用且请求带 `Cache-Control: no-cache` 时，同路径替换后读取到新标记，恢复后读取到原 603 cue；这不能覆盖普通客户端/代理的历史缓存命中，因此 V1 保留新版本文件名策略。 |

## 关键链路记录

### 搜索与 Fetch

- 电影候选来自 `MeiamSub.Assrt`，Fetch 成功并解析为 ASS。
- 剧集候选来自 `MeiamSub.Thunder`。搜索返回 20 个候选；前三个 Fetch 结果为 500/200/200。
- 第一个候选的 HTTP 500 是上游资源 404 被 Provider 包装后的结果；其余两个候选成功并解析为有效 SRT。
- 这证明“搜索返回候选”不等于“每个候选均可 Fetch”，V1 必须采用候选级失败处理。

### STRM 网络边界

受控测试只执行电影字幕搜索和候选 Fetch，并对媒体代理端口抓包。期间无到该端口的数据包，因此没有观察到候选 Fetch 触发 STRM 内部媒体 URL 请求。

### 写入、直连与文件系统

写入后 Emby 将剧集识别为包含一条外部 SRT。文件系统中的实际字幕为 UTF-8 BOM、CRLF 换行，603 个 cue；通过 Emby 字幕流接口读取时 HTTP 200，归一化后的内容一致。

本次使用的 Emby 字幕流形态为：

`/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}`

### 同路径缓存

测试前先备份原文件，写入短标记字幕并刷新 Item。禁用浏览器缓存并带 `Cache-Control: no-cache` 请求时得到标记文本；随后恢复原文件、再次刷新并请求，得到原始 603 cue 内容。远端临时备份已清理。

该结果只证明本次显式绕过缓存的请求没有读到旧内容，不能证明普通 Emby 客户端、反向代理或媒体缓存永远不会命中旧内容。因此 V1 默认先写入带版本的新文件名，确认新流可用后再归档旧文件，不采用默认同名覆盖。

## 遗留事项

- 通过 Add/下载链路产生的外部 SRT 条目仍保留在样本剧集上，作为 Gate0 写入验收产物；实际文件内容已恢复为原内容。未执行删除外部字幕条目，以免在未确认的情况下改变 Emby 库状态。
- 后续开发可继续收集 Thunder 候选失效样本；Gate0.1 已按单候选失败策略收口，不再作为 Gate0 阻塞项。
- 若要把第 6 项升级为实时证据，需要在允许的观测方式下单独设计文件打开监控；当前报告不把它伪装成已完成的 live trace。
