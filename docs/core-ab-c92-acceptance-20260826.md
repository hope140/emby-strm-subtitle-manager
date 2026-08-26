# Core A/B C92 单源 STRM 正式验收

- 日期：2026-08-26（中国标准时间）
- 结论：**通过，但只覆盖本报告列出的单源 STRM 服务端闭环。**
- 候选源码：`5deaf519f69ba1226840836516c07124965a4afc`
- 部署范围：仅 C92 上的 SubBridge 应用；未修改 Emby、SH、FRP、OpenResty 或媒体库全局权限。

## 与历史报告的关系

[2026-08-25 Core A/B C92 综合部署验收](core-ab-c92-acceptance.md) 如实记录了当时 source-bound 规则下的阻断，不能改写为通过。本报告记录随后采用 ADR-009 单源 STRM 写入锚点的独立、app-only 验收；两份报告共同构成时间顺序上的证据。

## 验收范围与结果

本次只选择已授权的单源 STRM 样本。请求仍显式绑定 `media_source_id`，服务端每一步重新读取并核验当前 Item/source；本地写入目标由 `Item.Path` 映射出的现存 `.strm` 文件目录和 basename 决定，远程播放 URL 不参与本地写入定位。

| 项目 | 结果 | 说明 |
|---|---|---|
| 不可变候选与启动边界 | 通过 | 运行中的版本和 OCI revision 与候选源码一致；容器使用 UID 10001、只读根文件系统，公开健康端点、认证边界和 Bearer 健康接口均通过。 |
| daily 受控窗口 | 通过 | `write_enabled` 与 `remote_search_enabled` 仅在短时窗口启用；日常 gate 与精确 Item allowlist 生效。 |
| 精确目录权限预检 | 通过 | 初始 Add 因样本目录对 UID 10001 不可写而安全返回 `subtitle_write_failed`；仅对该 `.strm` 所在目录临时授予写权限，未递归修改媒体库，结束后恢复原属主和模式。 |
| Upload → Preview → Add | 通过 | 两份受控本地测试字幕均先生成 PreviewArtifact；Add 完成原子写入、Hash 回读、Emby Refresh 和可见性核验。 |
| Replace → Restore | 通过 | 新版本经 Refresh/MediaStreams 核验后归档旧版本；Restore 成功恢复原版本并再次核验。 |
| Delete → Restore → 最终 Delete | 通过 | Delete 使用可恢复 trash；Restore 成功；最终 Delete 后以新鲜服务端 Inventory 确认测试 sidecar 不存在。 |
| MediaStreams 与官方字幕流 | 通过 | 每次成功 Add/Replace/Restore 后均在 Emby MediaStreams 中可见；官方字幕流可读。应用输出会补一个终止换行，故以去除末尾换行后的语义内容与受控测试字幕比对，而非错误宣称原始字节完全相同。 |
| history 与 source 绑定 | 通过 | 有界 history 查询只返回当前显式 source 的操作记录；Replace/Delete 的可恢复记录均可按绑定 source 执行。 |
| 管理 UI 可见性 | 通过 | 管理员登录后，受控单源 STRM 页面显示远程搜索、文件选择、上传并预览、操作历史和 Restore 入口。未在浏览器中提交上传或写入请求，因此不把此项表述为 UI 写入端到端验收。 |

## 不在本次通过结论内

- 未执行真实 Provider 的 Search、Fetch 或 Preview；本次写入使用受控本地 Upload→PreviewArtifact。
- 未在本次窗口重新执行 Emby Web、手机端或其他实际客户端播放。历史 D3.1 客户端证据仍只属于其原报告，不能视为本次单源 STRM 回归的新增客户端证据。
- 普通本地媒体的真实 C92 正向流程未执行。
- 多源 STRM 的真实 C92 409 未执行；当前仅有本地与 Fake Emby 路由级回归。多源 STRM 继续只读展示，D3 写入必须拒绝。
- 未进行发布、推送、正式镜像分发、批量、自动下载或永久删除验收。

## 关闭与回滚状态

验收结束后，应用已重新创建为 closed 状态：`write_enabled=false`、`remote_search_enabled=false`、`/media:ro`；容器 healthy、UID 10001、根文件系统只读、重启计数为 0。最终复核 `/readyz` 成功、未认证健康请求被拒绝。测试生成物已通过最终 Delete 清除，临时工作材料已移除。没有部署或重启非应用服务。

## Knowledge Review

任务或阶段　Core A/B 的 ADR-009 单源 STRM 写入目标修复后，C92 app-only 受控服务端验收与 closed 收尾。

验证范围　候选提交与 OCI revision、C92 app-only 启动/认证/关闭边界、单源 STRM 的 Upload→Preview→Add、Replace/Restore、Delete/Restore/最终 Delete、Hash、Refresh、MediaStreams、官方字幕流、source-specific history、精确目录权限和管理 UI 写入入口可见性。

### Knowledge Findings

- 新增约束　单源 STRM 的 `Item.Path` 写入模型可在真实 C92 完成服务端闭环，但通过结论必须逐项限定为单样本、单源和 API 操作，不能扩展为普通本地媒体、多源 STRM、真实 Provider、管理 UI 写入提交或实际客户端播放。
- 隐蔽坑　UI 的历史记录或媒体流展示不能单独证明测试 sidecar 仍存在或已清理；最终 Delete 必须配合新鲜服务器端 Inventory 和 closed-state 复核。
- 被证明错误的假设　`/media` 的 RW 挂载并不表示指定样本目录已对容器 UID 可写；真实窗口应先做精确目录权限预检，且只能临时调整授权样本目录。
- 建议沉淀项　正式验收报告保留历史阻断报告，并用独立的后续报告记录修复后的范围；不要回写历史结论。

### 证据

- 代码　候选提交 `5deaf519f69ba1226840836516c07124965a4afc` 采用 ADR-009 的单源 STRM `Item.Path` 写入目标与显式 source 绑定；普通本地媒体和多源 STRM 的边界未在本次运行中改变。
- 测试　候选提交的全包 Go、race、vet、build、验证脚本和本地浏览器 E2E 已在部署前通过；本报告不把这些本地结果替代为真实 C92 证据。
- 实际运行、日志或可复现结果　C92 上完成本报告表格列出的单源 STRM 操作、MediaStreams、官方字幕流和 closed 收尾；管理 UI 仅核对入口可见，未提交浏览器写入请求。

### 去重检查

- 已搜索的文档和关键词　`AGENTS.md`、`architecture.md`、`current-status-and-roadmap.md`、`lessons-learned.md`、`adr/`、`Core A/B`、`STRM`、`Item.Path`、`MediaSource`、`Upload`、`Replace`、`Delete`、`Restore`、`MediaStreams`、`Inventory`、`closed`、`C92`。
- 是否更新已有结论　是；保留 [2026-08-25 阻断报告](core-ab-c92-acceptance.md) 的当时结论，更新当前架构、路线图、实现评审和文档索引，并新增本报告记录修复后的受控范围。

### 分流判断

- `docs/lessons-learned.md`　更新；记录最终清理应以服务器端 Inventory 和 closed 收尾为准。
- `docs/architecture.md`　更新；记录单源 STRM 的真实服务端闭环及其明确边界。
- `docs/adr/`　不需要；本次遵循 ADR-008 和 ADR-009，未改变既有决策。
- `LOCAL_OPERATIONS.md`　不需要；没有新增长期拓扑、连接方法或恢复步骤。

### 未验证范围与残余风险

本次是 app-only、单样本、单源 STRM 的受控验收。以后任何重新打开写入窗口都仍需要独立授权、实时 image/config/gate/权限预检，并以完成后 closed/只读恢复作为必需收尾证据。
