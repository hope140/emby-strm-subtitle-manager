# Emby STRM 字幕管理器

本项目为 Emby 与 STRM 媒体库提供中文字幕浏览、搜索、预览和安全管理能力。CMS 继续负责媒体整理，Emby 继续负责媒体索引与播放，第一阶段通过 MeiamSub 获取 Thunder 和 ASSRT 字幕候选。

## 当前状态

[Gate 0 实测](GATE0_REPORT.md)已经正式通过。真实环境已经验证 Emby API Key 搜索与 Fetch、STRM 网络边界、外部字幕写入、Emby 直连读取和受限范围内的缓存行为。

V1 已决定使用 Emby Remote Subtitle Bridge。Native Provider 暂缓，单个失效候选按候选级错误处理。

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。ADR-002 已接受，选择新建轻量 Go 后端并选择性复用 ChineseSubFinder。

当前尚未建立正式应用代码。Phase 1 文档已收口，ADR-003 已接受并确定 D1 只读 Canary → D2 搜索预览 → D3 专用样本 Add 的顺序。下一步是实现并部署 D1 骨架，默认 Linux Docker Compose 单容器、媒体只读挂载、`write_enabled=false`，通过私网或 SSH 隧道进行实际测试；在 D1 自动和真实验收通过前不进入 D2。详见 [ADR-002](docs/adr/002-project-codebase-route.md)、[ADR-003](docs/adr/003-phase2-milestones-and-deployment.md)、[Phase 2 只读 Canary](docs/phase2-readonly-canary.md)、[Phase 1 基线报告](BASELINE.md) 和 [Phase 1 基线检查表](docs/phase1-baseline-checklist.md)。

## 文档入口

| 文档 | 用途 |
|---|---|
| [总体规划](Emby_STRM_Subtitle_Manager_Master_Plan_Revised.md) | 产品范围、数据模型、阶段和验收条件 |
| [Gate 0 实测报告](GATE0_REPORT.md) | 真实环境验证结果和证据边界 |
| [文档索引](docs/README.md) | 文档分层和读取顺序 |
| [当前架构](docs/architecture.md) | 当前已经验证的系统边界与数据流 |
| [维护经验](docs/lessons-learned.md) | 经过代码、日志或真实运行证明的高复用结论 |
| [ADR 索引](docs/adr/README.md) | 长期架构决策及取舍 |
| [Knowledge Review 模板](docs/knowledge-review-template.md) | 每次实质性任务结束后的知识复盘 |

本机长期操作信息写入 `LOCAL_OPERATIONS.md`。该文件由版本化 `.gitignore` 排除，不能提交。

## 当前开发边界

- 不读取 STRM 内部媒体地址
- 不管理 115、CD2 Cookie 或直链
- 不建立第二套媒体索引
- 不在 V1 开发 Native Thunder 或 Native ASSRT
- 不在只读模型准确以前编写 Installer
- 未经明确要求不部署、不重启 Emby，也不发布外部版本
