# SubBridge Core A/B 连续实施计划

- 状态　local implementation completed；真实 C92 综合验收待独立授权
- 日期　2026-08-25
- 基线　`main` 的 `b675758`（包含 SubBridge 改名收口）
- 目标　优先完成字幕管理核心能力；沿用当前 UI，只增加必要入口；Core A/B 全部完成后再统一审核和部署验收

## 1. 最终交付

本任务连续完成两组能力，中间不拆发布版本、不部署 C92，也不开始 UI 信息架构重构。

Core A：

- 将 D2/D3 从“必须使用专用 Item Canary”的一次性验收模式扩展为管理员可日常启用的受控模式。
- 单源 Movie/Episode 继续支持 Search→Fetch→Preview→Add。
- 正向支持一个 Item 含多个 `MediaSource`，所有操作必须绑定管理员明确选择的 source。
- 多个独立 Item 被 Emby 合并展示时，仍按各自 Item/source 精确操作，不依赖标题或分组猜测。

Core B：

- Replace：验证新字幕成功后归档旧字幕，失败时旧字幕继续可用。
- Upload：接收本地字幕、校验并生成普通 PreviewArtifact，再复用 Add/Replace。
- Delete：把可管理字幕移动到媒体库外回收目录，不提供即时永久删除。
- Restore：通过服务端操作记录恢复被 Replace 归档或 Delete 回收的字幕。

任务完成的含义是源码、自动化、Fake Emby 和最小 UI E2E 全部通过；不代表已经部署或完成真实 C92 验收。

## 2. 不做的内容

- 不重构首页、媒体库、电视剧、季、集和版本的页面层级。
- 不开发完整设置页、日志页、Provider 状态页或视觉主题。
- 不实现批量任务、自动下载、定时扫描、评分自动安装或永久清理。
- 不改变 Emby、CMS、SH、FRP、OpenResty 或 C92 现有运行资源。
- 不读取 STRM 内容，不接受前端路径，不调用 Emby Remote Subtitle Save API。

## 3. 日常模式与 Canary 兼容

保留 `features.remote_search_enabled` 和 `features.write_enabled`，默认仍为 `false`。功能开关关闭时必须在访问 Emby、Provider 或文件系统写路径前返回稳定 403。

调整现有 Canary 语义：

1. `canary.enabled=true` 时继续要求非空 Item allowlist，并把 generation 绑定到 Candidate、Artifact 和写操作。
2. `canary.enabled=false` 时允许管理员启用日常模式；不再因为缺少 Canary allowlist 拒绝启动。
3. 日常模式不等于匿名全库写入。它仍要求管理员会话或对应 Bearer scope、显式功能开关、Movie/Episode、有效 source、PathMapper、PathGuard、可写 overlay 和私有状态目录。
4. 为 Canary 与日常模式提供统一的 Item gate 接口。日常 gate 允许有效 Item，并提供稳定 generation；不能到处使用 `nil` 或特殊数字绕过 Token 绑定。
5. allowlist 或模式变化后，旧 Candidate/Artifact 失效；服务重启后内存状态继续失效。

新增面向正常使用的 Compose 写入 overlay，例如 `deploy/compose.write.example.yaml`。它只提供精确的 `/media:rw`、preview、history、quarantine、archive 和 trash 挂载，不修改 rootfs 只读、UID、认证或网络边界。现有 D2/D3 Canary overlay 保留给有界真实验收。

## 4. Core A　日常 Add 与正向多源

### 4.1 source 选择

- 每次 Search 都重新读取包含 `AlternateMediaSources` 的 Item 详情。
- Search 在一个 source 时允许省略 `media_source_id`，也允许提交精确匹配的 ID；Fetch/Preview 只从已绑定 Token 取得 source。
- Upload 与所有 D3 写入在单源和多源时都必须提交非空且精确匹配的 `media_source_id`；缺失或错误 ID 安全拒绝，不自动选择默认 source。
- Fetch 和 Preview 从 Candidate/Artifact 绑定中取得 source，重新读取 Item 后确认该 source 仍存在，不选择默认 source。
- Add 请求中的 source 必须同时匹配 Artifact 绑定和最新 Item；Item/source 变化后旧 Artifact 不可写入。
- source 的播放路径、URL、完整本地路径和原始 Provider ID不进入响应或日志。

删除现有 D2/D3 对“多个 source 一律 409 unsupported”的正向限制，同时保留零 source、重复 ID、多个 default、source 结构无效和 source 变化的 fail-closed 行为。

### 4.2 Add

继续复用现有 Add 的 Item 锁、幂等 `operation_id`、同目录临时文件、非覆盖原子提交、Hash 回读、Emby Refresh/轮询、history 和 quarantine。

多源 Add 的目标 basename 必须来自选中 source 的安全本地媒体路径。两个版本位于同目录时，文件名仍须由各自媒体 basename 区分；禁止使用标题、默认 source 或列表顺序生成目标名。

### 4.3 最小 UI

- 保留现有媒体列表和详情布局。
- 多源 Item 必须先选择版本，Search/Fetch/Preview/Add 才可用。
- 明确显示当前选中的 source 展示名和操作状态。
- 不增加媒体库层级、季/集导航或完整设置界面。

## 5. Core B　Replace、Upload、Delete 与 Restore

### 5.1 服务端字幕解析

所有修改现有字幕的请求只接受 Inventory 返回的安全 `subtitle_id`，不接受路径或文件名。每次操作前重新读取 Item/source 和 Inventory，并确认字幕：

- 属于当前 Item 和明确 source。
- `manageable=true`，为可解析的外挂文本字幕。
- 当前仍存在于 PathGuard 允许的媒体目录。
- 没有重复、路径冲突或 Inventory 不完整警告。

在 Inventory 或 D3 内增加服务端 resolver，把安全 ID解析为本次请求内使用的路径事实；resolver 结果不序列化到 HTTP、日志或 history 公共字段。

### 5.2 Replace API

```text
POST /v1/media/{item_id}/subtitles/{subtitle_id}/replace
```

请求体：

```json
{
  "artifact_token": "短期 Artifact Token",
  "media_source_id": "明确 source ID",
  "operation_id": "幂等操作 ID"
}
```

顺序：

1. 取得 Item 锁，重新解析 Item/source、Artifact 和目标字幕。
2. 以 Add 规则创建并核对新版本文件，不覆盖旧文件。
3. Refresh 并确认新字幕流可读取。
4. 将旧字幕以 Hash 和受限元数据归档到媒体库外 `archive_dir`。
5. 再次 Refresh，确认新字幕仍存在且旧文件不再作为当前 sidecar。
6. 写入操作 history 后返回成功。

任何一步失败都必须恢复到“旧字幕仍可用”。若旧文件已经离开媒体目录，则先恢复旧文件、Refresh，再把新文件送入 quarantine。跨文件系统归档必须采用复制到临时文件、fsync、Hash 核对、原子提交、再删除源文件的流程，不能假定 `os.Rename` 一定成功。

### 5.3 Upload API

```text
POST /v1/media/{item_id}/subtitles/upload
Content-Type: multipart/form-data
```

字段仅允许 `file`、`media_source_id` 和 `language`。总请求上限应在现有 4 MiB 字幕上限之外只保留小幅 multipart 余量。忽略客户端文件名和 MIME 类型，由内容 Validator 判断 SRT/ASS/SSA、编码、canonical UTF-8、语言和安全预算。

Upload 成功只生成普通 PreviewArtifact，并返回与 Fetch 相同形状的安全摘要；不直接写媒体目录。之后由现有 Add 或 Replace 消费该 Artifact，因此 Upload 不建立第二套写入实现。

### 5.4 Delete API

```text
POST /v1/media/{item_id}/subtitles/{subtitle_id}/delete
```

请求体只包含 `media_source_id` 和 `operation_id`。服务端重新解析目标后，将文件复制并核对到媒体库外 `trash_dir`，再删除媒体目录原文件，Refresh 并确认对应字幕流消失，最后写 history。

失败时必须恢复原文件；没有永久删除 API。trash 的保留期清理不属于 Core B。

### 5.5 History 与 Restore API

history 记录 `add`、`replace`、`delete`、`restore` 类型，并保存恢复需要的服务端私有事实。Upload 只生成短期 Artifact，不写持久 history。公共响应只包含操作 ID、类型、Item/source 安全绑定、字幕 ID、安全 basename、Hash、状态和时间；History 查询使用有界 limit。

至少提供：

```text
GET  /v1/subtitle-operations?item_id={item_id}
POST /v1/subtitle-operations/{operation_id}/restore
```

Restore 必须重新读取当前 Item/source、取得同一个 Item 锁、检查目标文件冲突、从 archive/trash 核对 Hash 后恢复、Refresh/轮询并写入新的 history。已有同名但不同内容的文件不得覆盖。

### 5.6 最小 UI

- 增加本地文件选择并显示校验/预览结果。
- `manageable=true` 的字幕显示 Replace 和 Delete；不可管理字幕只显示原因。
- 增加最近操作的最小列表和 Restore 按钮。
- 只补足功能验收，不做页面层级和视觉重构。

## 6. 认证、请求与日志

- Search 继续要求 `subtitle:search`；Fetch、Preview、Upload 要求 `subtitle:preview`；Add、Replace、Delete、Restore 要求 `subtitle:write`。
- 浏览器写请求继续要求管理员会话、同源和 CSRF；Bearer 写请求不使用 CSRF，但必须有写 scope。
- JSON 请求体继续限制 8 KiB；仅 multipart Upload 使用独立小上限。
- 所有写操作共用 Item 锁，锁粒度为规范化 Item ID；不同操作不能绕过彼此并发控制。
- `operation_id` 必须校验长度和字符集，并绑定操作类型、Item、source、目标字幕和内容 Hash。相同 ID 不同参数返回冲突。
- 响应和日志不得包含本地路径、STRM 内容、字幕正文、上传原文件名、Provider 原始 ID/URL、Token、Cookie 或凭据。
- 新错误码必须稳定、简短、可测试；不得把 `os`、Emby 或 Provider 原始错误直接回传。

## 7. 配置与目录

扩展 D3 配置：

```yaml
d3:
  history_dir: /var/lib/subbridge/d3-history
  quarantine_dir: /var/lib/subbridge/d3-quarantine
  archive_dir: /var/lib/subbridge/d3-archive
  trash_dir: /var/lib/subbridge/d3-trash
```

四个目录都必须是绝对、稳定、私有、位于媒体映射之外，不能互相重叠，不能是文件系统根、临时目录或 symlink/reparse point。启用写能力时必须全部可由 UID 10001 读写；默认只读部署不依赖这些目录。

不得改动管理员 environment、Secret 名称、API Key 读取方式或现有 C92 私有文件。

## 8. 主要代码范围

| 模块 | 工作 |
|---|---|
| `internal/config` | 日常/Canary gate 语义、archive/trash、目录隔离和 Compose invariant |
| `internal/preview` | 统一 Item gate/generation；支持 Upload 创建受绑定 Artifact |
| `internal/d2` | 显式多源选择、Candidate/Artifact source 重校验、稳定错误码 |
| `internal/inventory` | 安全 subtitle ID 的服务端 resolver，不向外公开路径 |
| `internal/d3` | 扩展 Add，新增 Replace/Delete/Restore、归档/回收、幂等和恢复事务 |
| `internal/httpapi` | 新路由、multipart 上限、scope/CSRF、稳定 envelope |
| `internal/httpui` | 最小多源选择、Upload、Replace、Delete、历史/Restore 控件 |
| `cmd/server` | gate、目录、Service 和清理生命周期组装 |
| `deploy` | 正常写入 overlay、新目录示例和安全预检 |

为速度考虑，不在本任务把 `internal/d3` 重命名为 Installer，也不引入数据库、前端框架或新的大型依赖。

## 9. 必须通过的自动化

Core A：

- 单源 Search 省略/显式 source 均通过；Upload 和所有写入缺失 source 均安全拒绝。
- 多源显式选择每一个 source 均可 Search、Fetch、Preview、Add；省略、错误、重复和变化 source 安全拒绝。
- Candidate/Artifact 不能跨 Item、source、认证上下文、gate generation 或服务实例使用。
- 两种多版本组织方式都覆盖：一个 Item 多 source、多个独立 Item。
- Add 对同目录不同媒体 basename 不串写，幂等和并发保持原行为。

Core B：

- Replace 成功、旧字幕归档、Refresh 两阶段核验和幂等重放。
- Replace 在新文件创建、首次 Refresh、归档复制、旧文件删除、第二次 Refresh、history 任一点失败时均恢复旧字幕并隔离新版本；Delete/Restore 同样执行补偿。补偿后必须重新核对 Hash 和 Emby MediaStreams，恢复、移除、quarantine、Refresh 或 history 补偿失败统一返回 `subtitle_rollback_failed`，保留 archive/trash/quarantine 供人工恢复。
- Upload 的 SRT/ASS/SSA、UTF-8/UTF-16、BOM、空文件、HTML/JSON、二进制、超限和恶意文件名。
- Delete 成功进入 trash；失败恢复；重复操作不产生第二份；内嵌或不可管理字幕拒绝。
- Restore 成功、目标冲突、Hash 不符、Item/source 变化和重复恢复。
- Item 锁覆盖 Add/Replace/Delete/Restore 交叉并发。
- 所有路由的管理员会话、CSRF、Bearer scope、请求体、日志脱敏和错误码。
- Fake Emby 核对 Refresh 次数、MediaStreams 变化、无 Remote Save 调用和无错误 source 写入。
- 最小浏览器 E2E 覆盖多源选择、Upload→Preview→Add、Replace、Delete、Restore；浏览器存储继续为空。

最终统一运行：

```text
scripts/verify.ps1
go test -count=3 -shuffle=on ./...
go vet ./...
go build -trimpath ./cmd/server
node --check internal/httpui/assets/app.js
git diff --check
```

若当前环境仍无法运行 race，只记录原因；在可用 Linux CI 中增加 `go test -race ./...`，但不能通过降低路径或权限安全检查换取本机通过。

## 10. 实施与审核方式

1. 从包含本文档的干净 `main` 创建 `codex/core-ab` 分支。
2. 先完成统一 Item gate 和 Core A，多源自动化全绿后继续 Core B；不在 Core A 后部署或请求真实写入。
3. Core B 完成后执行全套验证和 Knowledge Review，更新架构、路线图、契约/实现评审和维护经验；没有新知识时明确说明。
4. 在分支内形成可审核提交，不合并、不推送 `main`、不部署、不重启、不修改 C92/SH/Emby/媒体。
5. 主会话审核代码、测试、敏感信息和恢复边界后，再决定合并与推送。
6. 合并后只开一个部署验收任务，使用 Movie、Episode、单源、多源和专用可恢复字幕完成一次综合 C92 Canary；结束时恢复 closed 和只读边界。

## 11. 完成与中断条件

完成条件：Core A/B 代码、最小 UI、自动化、文档和 Knowledge Review 全部完成，工作树范围清楚，无 P1/P2 审查问题。

可以提前中断的条件仅限：

- 发现现有 Item/source/Inventory 模型无法在不改变公开契约的情况下安全定位目标。
- Windows/Linux 文件系统无法满足已定义的原子恢复语义，需要用户决定支持范围。
- 必须新增数据库、外部依赖或改变部署授权范围才能继续。
- 发现真实凭据、用户改动冲突或无法恢复的仓库状态。

普通测试失败、实现复杂或需要补充内部类型不属于提前中断理由，应先在本地修复并继续。
