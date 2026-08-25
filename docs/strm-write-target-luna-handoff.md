# SubBridge 单源 STRM 写入目标修复交接

## 交给 Luna 的任务

请在当前 SubBridge 工作区完成单源 STRM 写入目标修复、相应测试和架构文档更新。先审查现状，再做最小必要修改，最后完成本地验证。

本任务只允许修改当前仓库内的代码、测试和文档。不要部署、重启、修改 C92、修改 Emby、调整媒体目录权限、提交、推送或创建 PR。保留用户已有改动，不执行 reset、clean 或无关重构。

## 当前代码基线

- 公开 main 基线为 `947d847bb8ee620fc0362081fdff981069472081`
- 当前分支为 `codex/core-ab-c92-acceptance-20260825`
- 当前 HEAD 为 `b61b21f83170c01a0d64efde05c4e90d2b3a3412`
- `b61b21f` 只记录 C92 阻断和 closed 恢复，没有修改代码
- 完整现场报告见 [Core A/B C92 综合部署验收](core-ab-c92-acceptance.md)

开始前必须重新执行 `git status --short --untracked-files=all`，核对分支和 HEAD。若状态与以上记录不同，以当前工作区为准，保留所有已有改动。

## 已确认的真实环境事实

C92 媒体库中的 Movie 和 Episode 使用 STRM。服务端详情返回的路径具有下面的含义。

- `Item.Path` 指向 Emby 媒体库中的本地 `.strm` 文件
- 外挂字幕应写入该 `.strm` 文件所在目录
- 字幕文件名应使用 `.strm` 文件的 basename
- `MediaSource.Path` 在真实样本中是远程播放 URL
- 远程 `MediaSource.Path` 不能进入 PathMapper，也不能作为本地文件名依据
- 写入完成后仍需通过 Emby Refresh、所选 source 的 MediaStreams、官方字幕流和实际客户端核验

当前 `ResolveWriteTarget` 要求所选 `MediaSource.Path` 可以映射为本地路径，因此所有真实 STRM 写入都在媒体操作前被拒绝。该拒绝保护了旧的 source-bound 契约，但契约不适合当前 STRM 部署。

旧的单源 D3 实现在提交 `a184cf1` 中使用 `mediaCtx.LocalPath` 和 `mediaCtx.LocalDirectory`。STRM 的这两个字段来自 `Item.Path`。该实现已经完成真实 C92 Add、Hash、Refresh、MediaStreams、字幕流、Emby Web 和手机端读取验收。Core A/B 改用 `ResolveWriteTarget(MediaSource.Path)` 后产生了本次回归。

## 已确认的代码问题

### 单源 STRM 写入目标错误

`internal/media/media.go` 中的 `Build()` 已正确使用 `Item.Path` 建立 STRM 的本地路径、目录和 Inventory。`ResolveWriteTarget()` 却无条件要求所选 source 自身具有本地可映射路径。

`internal/d3/service.go` 的 Add 和 `internal/d3/recovery.go` 的 Replace、Delete、Restore 都依赖这套写入目标解析，所以它们在 C92 上一起阻断。

### 多源 STRM 共享 sidecar 被按 source 管理

`internal/inventory/inventory.go` 会扫描 Item 目录和选中 source 目录。Item 目录中的同一物理字幕会使用当前 `MediaSourceID` 生成不同 opaque ID。多源 STRM 若直接启用 `Item.Path` 写入，同一个共享字幕可能分别显示为多个 source 的可管理字幕，随后被某一个 source 的 Replace 或 Delete 修改。

在确认 Emby 对同一 STRM Item 多 source 的字幕关联规则以前，多源 STRM 写入必须保持关闭。

### 测试夹具没有模拟 C92

当前 Core A/B Fake Emby 把 STRM Item 的 source path 构造成可映射的 `version-A.mkv` 和 `version-B.mkv`。这些文件甚至不需要真实存在，测试仍能通过。该模型没有覆盖 C92 的远程播放 URL，因此现有绿色测试无法发现真实阻断。

## 目标行为

### 单源 STRM

单源 STRM 应支持完整 D3 操作。

1. 请求继续强制提交非空 `media_source_id`
2. 服务端重新读取 Item
3. 校验 Item 为 Movie 或 Episode
4. 校验 Item 只有一个有效 source
5. 校验请求中的 source ID 与该 source 精确匹配
6. 根据 `Item.Path` 判断 STRM
7. 通过 PathMapper 映射 `Item.Path`
8. 通过 PathGuard 验证目录 containment
9. 确认映射后的 `.strm` 是现存普通文件，并拒绝 symlink、目录、设备文件和其他非普通对象
10. 写入目录使用 `.strm` 所在目录
11. 写入 basename 使用 `.strm` 的 basename
12. Add、Replace、Delete、Restore 共用该目标语义
13. Refresh 和 MediaStreams 核验继续使用请求绑定的 source ID

显式 source 绑定仍然保留。source ID 用于确认请求对象和验证 Emby 结果，不用于推导 STRM 的本地目录或字幕文件名。

### 普通本地视频

普通本地视频继续使用所选 `MediaSource.Path`。

- source path 必须是本地路径
- source path 必须通过 PathMapper 和 PathGuard
- 建议同样确认映射后的媒体锚点为现存普通文件
- 远程 source path 必须拒绝
- 不允许使用 `Item.Path` 作为远程 source 的 fallback

### 多源 STRM

多源 STRM 的 D3 写操作暂不支持。

- Add、Replace、Delete、Restore 应在接触 Artifact 内容和媒体文件写入以前返回稳定 409
- 建议错误码使用 `strm_multisource_write_unsupported`
- Search、Fetch、Preview 和 Upload 仍可保持显式 source 绑定，它们不直接写媒体目录
- Inventory 不应把共享 Item sidecar 表示为每个 source 独立可管理
- UI 应得到明确的不可写状态，不要让用户执行到最后才收到通用 `media_path_unsafe`

不要用远程 URL basename 生成字幕名。不要让多个 source 写入同一个 basename。不要在缺少真实 Emby 关联证据时设计 source-specific STRM sidecar 命名规则。

## 建议的代码结构

优先把媒体类别差异集中在 `internal/media`，避免 D3 四种操作各自判断。

可以让 `ResolveWriteTarget()` 返回明确的目标类别，例如 `item` 或 `source`，并遵循以下分支。

```text
STRM 且只有一个 source
    校验显式 source ID
    目标来自 Item.Path

STRM 且有多个 source
    返回稳定的多源 STRM 不支持错误

非 STRM
    目标来自选中的本地 MediaSource.Path
    远程或缺失 source path 安全拒绝
```

不要把远程 source fallback 混入 `Build()`。`Build()` 当前对 STRM Inventory 使用 `Item.Path` 的行为应保留。

## Inventory 和恢复记录

单源 STRM 的 Item 目录 sidecar 可以继续管理。多源 STRM 的共享 Item sidecar 只能只读展示或标记为不可管理，直到独立设计获得真实 Emby 证据。

新产生的单源 STRM Replace 和 Delete history 应记录 `OriginalLocation=item`。普通本地视频继续使用 `source`。

必须检查已有 version 2 history 的兼容性。历史记录若把 STRM 保存为 `OriginalLocation=source`，新代码不能把这个旧类别静默解释为 Item 目录。当前公开 Core A/B 没有完成真实 C92 写入，但实现仍应明确选择一种安全策略。

- 无法证明原目录时拒绝 Restore，并返回稳定错误
- 或者升级 history schema，保存足以验证语义但不泄露绝对路径的目标类别

任何方案都不能信任 history 中的路径，也不能把媒体绝对路径写入响应、日志或持久 history。

## 必须补充的测试

### `internal/media`

- 单源 STRM 配合远程 `MediaSource.Path` 时，写入目标来自 `Item.Path`
- 请求 source ID 缺失、错误、重复或变化时安全拒绝
- 单源 STRM 的 Item.Path 未映射、含控制字符、文件缺失、symlink 或非普通文件时安全拒绝
- 普通本地视频继续使用所选 source path
- 普通视频的远程 source 不允许回退到 Item.Path
- 多源 STRM 返回明确的不支持错误

### `internal/d3`

- 单源 STRM 配合远程 source 的 Add
- 同一模型下的 Replace、Delete 和 Restore
- Add 和 Replace 使用 `.strm` basename 生成版本化字幕名
- Replace 和 Delete 的 history 记录 `OriginalLocation=item`
- Restore 重新读取 Item 和 source 后恢复到 Item 目录
- 多源 STRM 的 Add、Replace、Delete、Restore 全部在写入前返回 409
- 旧 `OriginalLocation=source` 的 STRM history 不会被恢复到错误目录

### `internal/inventory`

- 单源 STRM 的 Item sidecar 可管理
- 多源 STRM 的共享 Item sidecar 不会成为 source-specific 可写对象
- 远程 source path 不会进入目录扫描
- Inventory、resolver 和 history 对 `item`、`source` 类别保持一致

### `internal/httpapi`

- Fake Emby 的单源 STRM source path 使用真实形态的远程 URL
- Search、Fetch、Preview、Add 完整走通
- Upload、Replace、Delete、Restore 使用相同真实形态
- 多源 STRM 写路由返回稳定 409 和稳定错误码
- 普通本地视频回归测试继续通过
- 响应和日志不包含 Item.Path、MediaSource.Path、STRM 内容、字幕正文、Token 或上传原文件名

现有多源 fixture 可以保留用于普通本地视频。不要继续用虚构的本地 `version-A.mkv` 和 `version-B.mkv` 证明多源 STRM 已支持。

## 架构文档要求

新增 ADR，建议编号为 ADR-009。它应说明以下事实和决定。

- STRM 与普通本地媒体使用不同的写入锚点
- 单源 STRM 使用 `Item.Path`
- 普通本地媒体使用所选 `MediaSource.Path`
- source ID 始终由请求显式绑定并由服务端重读校验
- 多源 STRM 写入暂不支持
- 新决定修订 ADR-008 中所有 Add 都使用 source path 的规则
- ADR-004 关于 STRM 远程 source 只是播放定位符的决定继续有效
- history 目标类别和旧记录兼容策略
- 本地测试、Fake Emby、C92 Canary 和真实客户端验收是四种不同证据

同步更新下面的长期文档，避免继续保留互相冲突的说法。

- `docs/architecture.md`
- `docs/lessons-learned.md`
- `docs/adr/README.md`
- `docs/adr/008-core-ab-daily-source-bound-recovery.md`
- `docs/core-ab-implementation-plan.md`
- `docs/core-ab-implementation-review.md`
- `docs/current-status-and-roadmap.md`

保留 `docs/core-ab-c92-acceptance.md` 作为当时阻断和 closed 恢复的历史报告，不要把它改写成已通过。

## CSF 参考边界

ChineseSubFinder 固定提交 `3335a9c95eec8e1664b7ab29368c34ce10f13575` 的 `SaveSubHelper.WriteSubFile2VideoPath()` 使用传入媒体路径的 `filepath.Dir`，再由媒体 basename 生成相邻字幕名。它能支持字幕跟随本地媒体入口目录这一经验。

不要复制 CSF 的旧保存流程。它在某些 default 字幕场景会先删除旧文件，再直接写入新文件，也没有 SubBridge 当前需要的 source 绑定、Item 锁、非覆盖原子提交、Hash、Refresh、history、archive、trash、quarantine 和回滚验证。

CSF 只作为路径和命名经验。C92 对 STRM sidecar 的真实识别能力由 Gate 0 和旧 D3 Canary 证据支撑。

## 本地验证要求

实现完成后至少执行以下检查。

```powershell
go test -count=1 ./...
go vet ./...
go build -trimpath ./cmd/server
git diff --check
git status --short --untracked-files=all
```

若仓库的 `scripts/verify.ps1` 可在当前环境运行，也要执行并记录结果。沙箱、临时目录、CGO、Docker 或浏览器环境造成的阻断要单独说明，不得写成代码已经通过。

不需要执行 C92、Docker、真实 Provider 或浏览器客户端验收。本任务完成条件只覆盖本地代码、测试、文档和静态审查。

## Luna 交付时必须说明

- 修改了哪些文件
- 单源 STRM 的目标选择规则
- 多源 STRM 在哪一层返回什么错误
- Inventory 如何避免共享 sidecar 被按 source 修改
- history 兼容策略
- 实际运行过的测试、静态检查和构建结果
- 尚未验证的 C92、MediaStreams、字幕流和客户端范围
- Knowledge Review 检查结果
- 工作区是否仍有任务前就存在的改动

完成后不要提交或推送。把代码差异、测试结果和未验证范围交回主会话审核，再决定是否进入新的 C92 app-only Canary。
