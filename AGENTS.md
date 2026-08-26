# 项目协作规则

## 适用范围

本文件适用于 SubBridge（SB，字幕桥）仓库中的代码、测试、文档和本地验证。当前阶段以 [总体规划](docs/planning/master-plan.md)和 [Gate 0 实测报告](docs/records/acceptance/gate0-report.md)为事实起点。

## 开始任务前的读取顺序

1. 阅读本文件。
2. 阅读 [当前架构](docs/reference/architecture.md)。
3. 按任务关键词搜索 [维护经验](docs/reference/lessons-learned.md)和 [ADR](docs/decisions/adr/index.md)。
4. 涉及阶段范围时阅读总体规划和对应阶段检查表。
5. 涉及本机连接、路径和恢复时读取本地 `LOCAL_OPERATIONS.md`，不得在输出中回显敏感内容。

## 文档职责

- `AGENTS.md` 保存稳定的协作规则和验收门禁。
- `docs/reference/architecture.md` 保存当前已经实现或实测确认的架构事实。
- `docs/reference/lessons-learned.md` 保存隐蔽、可复用并且有证据的经验。
- `docs/decisions/adr/` 保存跨模块、长期有效且需要解释取舍的决策。
- 阶段报告保存一次任务的完整过程和证据，不直接充当长期知识库。
- `LOCAL_OPERATIONS.md` 保存本机拓扑、连接方法、路径映射和恢复步骤。

同一事实只保留一个正式来源，其他文档使用链接。版本、提交、进程和服务状态需要实时检查，不能从旧报告推断当前状态。

## 当前已接受的项目决策

- V1 使用 Emby Remote Subtitle Bridge。
- Provider 标签只筛选和排列已返回的结果。
- Native Provider 留作 Emby Bridge 无法满足要求时的替代方案。
- Provider 失败按候选隔离，不能让一个失效候选使整次搜索失败。
- STRM 内部媒体访问继续交给 Emby 播放链。
- 替换字幕默认写入新版本文件，验证成功后归档旧文件。
- 当前只支持单应用实例，多实例需要共享状态和分布式锁。

决策发生变化时新增或更新 ADR，不能只改代码或聊天结论。

## 变更原则

- 先检查现有文件、测试和未提交改动。
- 修改范围只覆盖当前阶段，不提前实现后续功能。
- 保留用户已有改动，不重置、不清理，也不覆盖无关文件。
- API Key、Token、Cookie、候选原始 ID 和带认证参数的 URL 不能进入代码、测试夹具、日志或提交历史。
- 外部字幕写操作必须通过服务端 ItemID 重新解析路径，不能信任前端路径。
- 任何删除默认采用可恢复方式，永久清理由独立保留期任务负责。

## 阶段门禁

Gate 0 已正式通过。下一阶段为 Phase 1 构建基线与代码路线决策。

Phase 1 只允许构建、检查和文档化。完成 `docs/planning/phase1-baseline.md`、`docs/planning/chinesesubfinder-reuse-matrix.md` 和项目路线 ADR 前，不开始 MediaContext、Inventory 或 Installer 实现。

Phase 2 只做 Emby Item、MediaContext、PathMapper 与字幕清单的只读切片。真实电影、剧集和多源自动化验收通过后，可按 [ADR-005](docs/decisions/adr/005-conditional-d2-entry-without-live-multisource.md) 有条件进入单源搜索预览；真实多媒体源 Item 验收前，多源搜索必须安全拒绝且不得宣称支持，`remote_search_enabled` 继续默认关闭。

Installer 必须等只读模型和路径映射准确后再开始。部署、重启和外部发布始终需要用户明确授权。

## 验证要求

- 按[风险分级验收矩阵](docs/reference/acceptance-matrix.md)选择本次改动的最低充分检查集；不确定时上调档位。
- 文档修改检查 Markdown 链接、UTF-8、代码围栏、尾随空白和 `git diff --check`；局部代码至少运行受影响测试，合并候选由 CI 覆盖全量格式、vet、测试与构建。
- Emby API、Provider、部署、路径映射、鉴权和写入语义变更，才按矩阵触发真实实例 Canary；本地/Fake 结果不能替代被触发的真实验收。
- 文件写入能力首次启用、外部边界变更或正式发布时，仍需核对文件 Hash、Emby MediaStreams、直连字幕流和实际客户端读取。
- 代理或客户端行为只能由对应路径的真实请求证明，但纯本地改动不自动使既有真实证据失效。

## Knowledge Review

Knowledge Review 的深度按[风险分级验收矩阵](docs/reference/acceptance-matrix.md)执行。只有 C 类及以上且产生可复用长期知识时才填写完整[模板](docs/reference/knowledge-review-template.md)；A/B 类在交付说明中记录检查范围和“无新增长期知识”即可。

主任务负责核验证据、搜索重复内容和决定文档分流。只有代码、测试、日志、可复现运行结果，或官方文档与当前实现的交叉证据能够进入正式知识文档。

没有新增知识时说明检查范围和无新增的原因。Knowledge Review 不是自动钩子，也不代替测试和验收，更不重复矩阵已经覆盖的证据。

## 本地操作文档

`LOCAL_OPERATIONS.md` 只保存长期有用的本机信息。任务进度、当前版本、临时故障和一次性命令留在阶段报告或实时检查中。

本地文档不得包含明文凭据。需要凭据时只记录凭据文件或 Secret 的位置、权限要求和轮换步骤。
