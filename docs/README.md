# SubBridge 文档索引

这套文档把当前事实、长期决策、维护经验和本机信息分开保存。开发前按需要读取，任务结束后通过 Knowledge Review 决定是否更新。

当前统一状态见 [当前状态与后续路线图](current-status-and-roadmap.md)。截至 2026-08-26，D1、D2 单源后端、D2.5 和 D3.1 专用单源 Add 已完成相应自动化与真实验收，D3.1 又补充了手机端实际客户端读取确认。Core A/B 已完成本地源码、Fake Emby、浏览器 E2E 和受控 C92 单源 STRM 管理 UI 闭环：Search、Fetch、Preview、Add、Upload、Replace、Delete、Restore 与实际播放器字幕显示均已确认，随后恢复 closed。普通本地媒体和多源 STRM 仍是独立边界；正式镜像发布和 UI/V1 产品收口见 [发布收口与 UI/V1 计划](release-and-ui-v1-plan.md)。历史阶段报告继续保存当时的范围和证据，不再各自充当“当前进度”的正式来源。

## 正式文档

| 文档 | 保存内容 | 不保存的内容 |
|---|---|---|
| [当前状态与后续路线图](current-status-and-roadmap.md) | 当前完成度、剩余缺口、推进优先级和运行边界 | 一次性执行命令、凭据和实时部署状态 |
| [发布收口与 UI/V1 计划](release-and-ui-v1-plan.md) | Core 测试版发布收口、UI/V1 里程碑、独立边界验收和非目标 | 运行中的部署状态、凭据和一次性命令 |
| [安装指南](install.md) | 全新环境的只读安装、首次登录与 closed 验证 | 真实凭据、媒体路径和开启写入 |
| [升级与回滚](upgrade-rollback.md) | 不可变镜像、升级/回滚顺序和保留约束 | 任意媒体清理、无授权写入 |
| [故障排查](troubleshooting.md) | 启动、认证、路径、STRM 和开关的安全排查 | 密码、Token、Cookie、媒体绝对路径 |
| [风险分级验收矩阵](acceptance-matrix.md) | 日常开发、合并候选、Canary 与正式发布的最低充分验证和证据复用规则 | 实际运行状态、凭据和一次性操作记录 |
| [Core A/B 连续实施计划](core-ab-implementation-plan.md) | 日常 Add、多源、Replace、Upload、Delete、Restore 的实现范围、测试和审核方式 | C92 实际部署、凭据和 UI 重构 |
| [Core A/B 实现评审](core-ab-implementation-review.md) | 本地实现、测试证据、Knowledge Review 和真实验收边界 | C92 实际部署、凭据和真实客户端结论 |
| [Core A/B C92 综合部署验收](core-ab-c92-acceptance.md) | 精确提交的 C92 app-only 部署、source-bound 阻断、恢复状态和 Knowledge Review | Item/source 标识、凭据、私有路径和未执行的写入结论 |
| [Core A/B C92 单源 STRM 正式验收](core-ab-c92-acceptance-20260826.md) | 修复后候选的单源 STRM Upload/Add/Replace/Delete/Restore、MediaStreams、官方字幕流和 closed 回滚 | 普通本地媒体、多源 STRM、真实 Provider、UI 写入提交和新客户端播放 |
| [Core A/B C92 综合验收现场清单](core-ab-c92-combined-acceptance-checklist.md) | 单源 STRM、普通本地媒体、多源 STRM 的单窗口门禁、执行顺序、证据与 closed 收尾 | 凭据、样本标识、私有路径和实际部署命令 |
| [当前架构](architecture.md) | 已实现或实测确认的组件、数据流和边界 | 未来愿景和未验证设计 |
| [维护经验](lessons-learned.md) | 隐蔽、高复用、有证据的规则 | 一次性排错流水账 |
| [ADR](adr/README.md) | 长期架构选择、原因和代价 | 局部实现细节 |
| [Phase 1 基线检查表](phase1-baseline-checklist.md) | Phase 1 当前收口状态、基线任务和下一阶段门禁 | Phase 2 实现 |
| [Phase 2 只读 Canary](phase2-readonly-canary.md) | D1 已实现切片的 API、配置、安全边界和自动/真实验收门禁 | D2 搜索预览和 D3 写入实现 |
| [D2 搜索预览契约](d2-search-preview-contract.md) | D2 Search、Fetch、Preview、错误码、Token/Artifact 生命周期、安全预算、完整 MediaSources 读取和测试矩阵 | 真实 Canary、部署和写入能力 |
| [D2-B1 后端实现评审](d2-b1-backend-implementation-review.md) | D2-B1 代码、单元测试、Fake Emby 证据和残余门禁 | 真实 Canary、部署和 UI 验收 |
| [D2-B2 UI 评审](d2-b2-readonly-ui-review.md) | D2 UI、内存 Token 边界与本地 Fake Emby 浏览器 E2E | C92 管理 UI 真实点击验收、部署和写入能力 |
| [D2-C C92 预检](d2-c-c92-canary-preflight.md) | C92 有界只读 Item 选择、实时版本核对和 Canary 阻断证据 | 真实 D2 Search、Fetch、Preview Canary |
| [D2-C C92 部署前核对](d2-c92-deployment-preflight.md) | C92 现网 D1 安全边界、回滚点和 D2 发布阻断证据 | D2 部署、重启和真实 Canary |
| [D2 发布候选审计](d2-release-candidate-audit.md) | 本地 Git、敏感信息、测试构建证据与发布阻断项 | C92 部署、重启和真实 canary |
| [D2-C C92 单源真实 Canary 验收](d2-c92-canary-acceptance.md) | C92 可回滚 D2 部署、真实 Search/Fetch/Preview、候选失败隔离和关闭后的状态 | 多源支持、真实客户端、公网 UI 和 D3 写入 |
| [D2 多版本 MediaSources 实测记录](d2-multisource-c92-sample.md) | C92 真实版本组、`AlternateMediaSources` 字段证据和客户端修正入口 | 多源正向 Search/Fetch/Preview、写入能力 |
| [D2 多源真实 API Canary](d2-multisource-c92-canary-acceptance-20260825.md) | C92 真实多源 API/source 对应、D2 409 安全拒绝和关闭后的状态 | 多源正向支持、真实浏览器 UI 和写入能力 |
| [D3 专用样本 Add 契约](d3-dedicated-add-contract.md) | D3 Add 请求、CSRF/写 scope、原子非覆盖写入、Refresh/轮询、history/quarantine 和恢复边界 | Replace/Delete/Upload/批量能力 |
| [D3 Add 实现评审](d3-add-implementation-review.md) | D3 本地实现、测试证据、C92 真实闭环和宿主权限门禁 | 后续写能力 |
| [D3 C92 Canary 验收](d3-c92-canary-acceptance-20260825.md) | 单源 Search→Fetch→Preview→Add、Hash、Refresh/MediaStreams、字幕流、客户端读取、幂等和 closed 回滚 | Replace/Delete/Upload/批量能力 |
| [ADR-006 管理员会话与自动化凭据](adr/006-admin-session-and-automation-credentials.md) | 发布镜像的管理员登录、Compose environment 配置、自动化 Token 分离和 D2.5 排期 | D3 写入和多用户系统 |
| [ADR-008 日常模式与可恢复字幕操作](adr/008-core-ab-daily-source-bound-recovery.md) | Core A/B Item gate、显式 source、多源、可恢复写入和默认 closed 边界 | 部署授权与真实 C92 综合验收结论 |
| [D2.5 管理员认证](d2.5-admin-auth.md) | D2.5-A/B/C 当前实现、会话与 scope 契约、Compose environment、测试证据和 D3 认证边界 | 后续写能力与多用户系统 |
| [D2.5 目标环境迁移预检](d2.5-target-migration-preflight-20260825.md) | 本轮本地发布核对、C92 只读状态、暂停原因和恢复顺序 | Secret 内容、部署操作和 D3 写入 |
| [D1 部署验收报告](d1-deployment-acceptance.md) | 已验证的部署、真实 STRM Canary 和剩余门禁 | 私有部署细节、凭据和一次性操作记录 |
| [D1.5 最小只读 Web UI](d1.5-readonly-ui.md) | 内嵌 UI、Token 内存边界和三种访问方式 | 真实部署验收和私有环境细节 |
| [D1.5 部署前预检](d1.5-deployment-preflight.md) | 发布不变量、三种访问方式、目标主机预检、验收顺序和回滚准备 | 实际 Secret、私有路径和一次性部署操作 |
| [Phase 1 基线报告](../BASELINE.md) | 上游快照、构建结果、失败清单和未验证范围 | 后续阶段实现 |
| [OpenResty 公网入口基线](../deploy/openresty/README.md) | 面向所有部署者的安全反代日志、FRP 和公网端口边界 | 自动修改现有服务器配置 |
| [ChineseSubFinder 复用矩阵](../CSF_REUSE_MATRIX.md) | 上游模块耦合证据和方案 B 的复用边界 | 新运行时实现 |
| [Knowledge Review 模板](knowledge-review-template.md) | 任务结束后的知识检查格式 | 实际任务结论 |

总体规划与 Gate 0 报告暂时保留在仓库根目录。总体规划描述目标状态，Gate 报告保存真实验证过程。架构文档只吸收其中已经成立且仍然有效的事实。

## 本地文档

`LOCAL_OPERATIONS.md` 位于仓库根目录，由版本化 `.gitignore` 排除。

适合写入的内容包括服务器别名、连接方式、容器或服务名称、持久路径、PathMapper 对照、备份位置和恢复步骤。

不适合写入的内容包括明文凭据、候选 ID、带认证参数的 URL、当前进程状态和每天的操作记录。

## 维护方式

1. 任务开始前搜索现有文档，避免重复结论。
2. 任务结束时填写 Knowledge Review。
3. 先核验证据，再更新对应正式文档。
4. 架构选择写 ADR，已被替代的 ADR 保留历史并修改状态。
5. 当前状态依靠实时命令检查，文档只保存长期规则。
