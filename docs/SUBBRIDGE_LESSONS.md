# SubSteward 可复用经验索引

本文只保留旧 SubBridge 实测后仍适用于 SubSteward 的规则。它是经验和边界清单，不是旧项目功能迁移表。完整旧实现、部署记录和回退材料留在 `legacy/subbridge` 与稳定基线 `3acdf27047338f81438fa611aed314533e170371`。

## 先记住的六条规则

| 主题 | 当前可用规则 |
| --- | --- |
| 媒体身份 | 以 `Item + MediaSource` 作为处理单位，不猜第一个 Source |
| STRM 写入 | `.strm` sidecar 只锚定本地 `Item.Path`；远程 `MediaSource.Path` 不是文件路径 |
| 候选选择 | Search 顺序和 Provider 元数据都不可信，必须标题/Hash 绑定后再 Fetch |
| 内容门禁 | `Language=zh` 不能代替正文语言检查；Fetch 后再做格式、健康和中文字符校验 |
| 写入确认 | 文件存在或 Refresh 成功都不够，要核对 MediaStream，最终还要看真实客户端 |
| 复杂边界 | MultiSource STRM、未知状态和低置信度判断保持 `MANUAL`、`Unsupported` 或 `TODO` |

## 1. Emby 媒体模型与 STRM

- `Item`、`MediaSource`、`MediaStream` 是不同事实层，详情、搜索、写入和验收不能混用字段。
- 处理前先确认 Item 类型、Source 数量、Source ID、字幕流和本地锚点；版本组或多 Source 不能默认取第一条。
- 不读取 `.strm` 内容，不解析其中的远端 URL，也不探测 URL 指向的视频。
- STRM 外挂字幕使用 Emby `Item.Path` 对应的本地 `.strm` 文件目录作为 sidecar 锚点。
- 远程 `MediaSource.Path` 只用于播放定位，不能当成本地可写路径。
- MultiSource STRM 可以做受限读取、Presence、Search 和 Preview；在没有来源专属 sidecar 语义前不自动写入。
- 非 STRM 媒体也只有在 Source 是本地普通文件、目录安全且进程具备权限时才允许写入。

## 2. Provider 候选与错误隔离

- Search 候选不等于可安装 Artifact。Provider 的名称、语言、格式、分数和返回顺序都只是待验证元数据。
- 标题或原名匹配、Hash 匹配属于进入 Fetch 的绑定条件；没有绑定的候选保持拒绝，不能靠“第一条结果”补救。
- `ISubtitleManager.SearchSubtitles` 返回的候选必须通过短期 token 与同一个 Item/source 绑定；原始候选 ID 不进入响应日志或文档。
- Fetch 使用 `GetRemoteSubtitles` 取得字节后再 Validate。内容为空、格式损坏、读取失败、超过 16 MiB 或正文语言不符合时，只隔离当前候选。
- Provider 元数据里的 `Language=zh` 不能证明正文是中文。当前中文候选至少需要正文出现中文字符，并且继续满足 Item 标题/Hash 绑定。
- 临时网络错误最多做有限重试；不要因为重试或换候选而放宽身份、语言或格式门禁。
- 初期搜索结果最多展示 20 条；未来自动 Fetch 仍需单独限制尝试次数，不能把人工列表上限当成自动化授权。

历史实测中曾出现第一条候选是错误合集、第二条标称中英双语却实际为英语、第三条标题匹配候选才对应目标字幕的情况。这说明“搜索成功”只证明 Provider 返回了结果，不证明候选正确。

## 3. 字幕质量、语言与偏好

业务判断固定按以下顺序收敛：

```text
Presence → Health → Preference → Action
```

- Presence 只回答目标语言是否存在，内封和外挂都可提供 Presence 证据。
- Health 只处理影响观看的明确问题，例如编码不可读、SRT/ASS 结构损坏、空字幕、非法时间轴、替换字符和确定性坏标签。
- Preference 处理目标/第二语言覆盖、双语、格式顺序和特效倾向；“不是双语”或“不是 ASS”不应直接判定字幕损坏。
- `WARNING` 或低置信度语言/双语判断只能给人工建议，不能单独触发替换。
- 内封目标语言默认只参与 Presence，不提取正文、不 OCR、不深检、不重封装。
- 外置字幕深检必须有路径安全边界，只读取与本地媒体锚点同目录的普通文件，并设置大小和数量上限。
- 当前正文级语言证据对中文、英语和日语较明确；简繁仍主要依赖语言码、标题和安全文件名等外部证据，不能据此自动替换。

## 4. 对轴、写入与 Refresh

- 对轴只能使用管理员明确给出的整体偏移，不猜测音画偏移，不自动改写正文。
- 当前支持 SRT、ASS、SSA；累计偏移前后最多 10 分钟，ASS/SSA 使用 10ms 步进，生成 Artifact 后必须重新 Validate。
- 写入先计算安全 sidecar 目标，再写临时文件并原子移动到新的版本化文件名；不覆盖已有字幕。
- Install 必须重新读取 Item/source，写入后 Refresh，再按同一 Source 和文件名确认新的外置 MediaStream。
- 任何一步失败都要明确返回失败原因，并清理本次创建的正式文件；不能把“文件留下了”当成成功。
- Emby 一次公开字幕 Download 可能带来多条新的外置 MediaStream，必须按最终文件名、Source 和 MediaStream 对账。
- MediaStream 已出现仍不等于客户端可用。真实客户端不显示或不读取时，验收应判失败并继续查客户端链路。

## 5. 证据分层与状态写法

| 证据层 | 可以写成 | 不能写成 |
| --- | --- | --- |
| 本地 build/test | 当前工作树可编译、纯逻辑或测试通过 | 目标 Emby 已加载或 Provider 内容正确 |
| Fake/模拟 API | 结构、边界和错误处理通过 | 真实 Emby、真实挂载或客户端已通过 |
| C92 管理员 API | 指定时间、指定修订的路由可用 | 最新修订、字幕内容和客户端都已验收 |
| 文件/Refresh/MediaStream | 写入和索引链路完成 | 客户端一定显示 |
| 实际客户端 | 指定媒体、字幕和客户端当时可见可用 | 其他客户端、其他版本或后续部署仍通过 |

记录实测必须同时写日期、目标版本/修订、样本范围、通过项和未验证项。后续代码、配置、Provider、媒体挂载或客户端变化可能只使其中一层失效，不能用一句“之前测试过”覆盖。

## 6. 安全与可迁移边界

- API Key、Token、Cookie、候选原始 ID、私有路径和带认证参数的 URL 不进入代码、公开 fixture、日志、截图、Git 或产品文档。
- 本机 Emby/API 凭据按 `LOCAL_OPERATIONS.md` 中的 `embyapi` 条目读取，使用 `X-Emby-Token` 请求头。
- 公开质量 fixture 必须脱敏并可公开保存，不把旧上传预览或真实私有字幕冒充通用样本。
- 借鉴旧 SubBridge 时只迁移已验证的安全语义，例如 Source 绑定、Item.Path 锚定、版本化写入、Refresh 对账和 fail-closed；不自动带回 Go 服务、Docker、数据库、history、quarantine、Restore Center 或旧 Web UI。

## 7. 最小决策表

| 发现 | 动作 |
| --- | --- |
| 单 Source、目标字幕缺失、候选有标题/Hash 绑定且 Health PASS | 允许人工继续 Preview/Install |
| 多 Source STRM | 允许查看和人工判断，写入返回 `MANUAL`/`Unsupported` |
| 候选语言字段与正文冲突 | 拒绝该候选，不能降级为“可能可用” |
| 现有外置字幕路径不在锚点目录或是重解析点 | 不深检，报告 `UNKNOWN`/不安全 |
| 目标存在但 Health 未测量 | 保守返回 `MANUAL`，不能直接 KEEP 或替换 |
| Refresh 成功但目标 MediaStream 不出现 | 安装失败，保留证据并清理本次文件 |
| 客户端不显示已确认的 MediaStream | 客户端验收失败，不回写为成功 |
