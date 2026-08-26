# Phase 1 构建基线与代码路线检查表

## 目标

Phase 1 只回答两个问题。

1. 当前 ChineseSubFinder 快照能否在固定工具链下稳定构建。
2. 本项目应继续整仓渐进改造，还是新建轻量 Go 后端并选择性复用代码。

本阶段不实现 MediaContext、Inventory、搜索 UI 或 Installer。

## 当前状态

Phase 1 已完成路线决策和文档收口。项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。Docker 构建、CSF 完整后端构建和最小启动仍未形成通过结论，详见 [BASELINE.md](phase1-baseline.md)。

## 开始前检查

- 当前文档和 Gate 报告已保存
- 工作区状态和已有改动已记录
- 上游仓库地址、目标分支和 commit 已确认
- 没有把 `embyapi`、Token 或本机操作文档加入 Git
- 不升级 Go、Node、Quasar 或主要依赖

## 任务一　固定上游快照

- 在自己的 Fork 中固定 ChineseSubFinder commit
- 新建项目开发分支
- 记录上游许可证和保留要求
- 记录 Go、Node、包管理器和 Docker 版本
- 保存锁文件和实际构建入口

## 任务二　建立构建基线

依次执行并记录完整结果。

- 后端测试或最小编译
- 后端正式构建
- 前端依赖安装与构建
- Docker 构建
- 最小启动检查

网络、依赖或平台问题要区分为代码失败和环境阻塞。不能用 Docker 构建成功代替前后端独立结果。

## 任务三　检查复用范围

对下面模块记录位置、直接依赖、外部服务依赖、现有测试和建议处理方式。

| 模块 | 重点检查 |
|---|---|
| Emby API | 认证、Item 分页、MediaSource、Refresh |
| Library 页面 | Store、路由、旧任务和云端接口耦合 |
| 字幕搜索 Dialog | Provider DTO、下载动作、预览组件 |
| ASS、SSA、SRT Parser | 输入编码、错误行为、测试覆盖 |
| Path 和保存逻辑 | 物理视频假设、STRM 行为、原子写入 |
| 旧扫描器 | 是否能够完全绕开 |
| CSF Cloud 与 SubtitleBest | 启动、页面和任务系统中的运行时依赖 |
| Provider Hub | 是否侵入 UI、任务和配置核心 |

结果写入 `CSF_REUSE_MATRIX.md`。每项使用 `reuse`、`adapt`、`replace` 或 `remove`，并附证据。

## 任务四　选择项目路线

满足下面条件时可以继续整仓改造。

- Library 与 Emby API 能独立运行
- Cloud 和旧扫描器能通过明确边界停用
- Parser 与预览组件无需携带旧 Provider Hub
- 构建和测试基线可重复

出现下面情况时优先新建轻量后端。

- 启动依赖旧 Cloud 或扫描任务
- 去除旧功能需要持续修改多个核心包
- 前端 Store 与旧 API 无法在小范围内解耦
- 旧构建链无法形成稳定基线

最终决定已写入 [`docs/adr/002-project-codebase-route.md`](../decisions/adr/002-project-codebase-route.md)，状态为 `accepted`，选择方案 B：新建轻量 Go 后端，选择性复用 ChineseSubFinder。

## 必须交付

- `BASELINE.md`
- `CSF_REUSE_MATRIX.md`
- `docs/adr/002-project-codebase-route.md`
- 实际执行的构建和测试命令
- 失败清单、未验证范围和残余风险
- 本阶段 Knowledge Review

## 退出条件

Phase 1 在复用矩阵具备代码证据、项目路线 ADR 已接受、构建失败与未验证范围已记录并完成文档收口后结束。CSF 整仓的完整后端构建、Docker 构建和最小启动仍存在环境阻断或未验证项；因方案 B 不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。

结束后暂停并报告结果。未经确认不进入 Phase 2，不部署、不重启 Emby，也不修改 Gate 0 样本字幕。
