# Emby STRM 字幕管理器

本项目为 Emby 与 STRM 媒体库提供中文字幕浏览、搜索、预览和安全管理能力。CMS 继续负责媒体整理，Emby 继续负责媒体索引与播放，第一阶段通过 MeiamSub 获取 Thunder 和 ASSRT 字幕候选。

## 当前状态

[Gate 0 实测](GATE0_REPORT.md)已经正式通过。真实环境已经验证 Emby API Key 搜索与 Fetch、STRM 网络边界、外部字幕写入、Emby 直连读取和受限范围内的缓存行为。

V1 已决定使用 Emby Remote Subtitle Bridge。Native Provider 暂缓，单个失效候选按候选级错误处理。

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。ADR-002 已接受，选择新建轻量 Go 后端并选择性复用 ChineseSubFinder。

D1 只读代码切片已经在本地建立，当前包含 Go 服务、Emby 只读客户端、MediaContext、跨平台 PathMapper、字幕 Inventory 和只读 HTTP API。相关单元测试、`go vet`、构建和 `scripts/verify.ps1` 已通过；这只代表本地代码与自动化检查通过，不代表 Docker 镜像、Docker Compose 或真实服务器已经验收。下一步是制作并验证安全默认的部署产物，再在私网或 SSH 隧道中完成真实 Movie、Episode 和多媒体源 Canary；在 D1 自动和真实验收都通过前不进入 D2。详见 [ADR-002](docs/adr/002-project-codebase-route.md)、[ADR-003](docs/adr/003-phase2-milestones-and-deployment.md)、[Phase 2 只读 Canary](docs/phase2-readonly-canary.md)、[Phase 1 基线报告](BASELINE.md) 和 [Phase 1 基线检查表](docs/phase1-baseline-checklist.md)。

当前服务公开 7 个 GET 路由：3 个运维路由（`/livez`、`/readyz`、`/v1/health`）和 4 个业务路由（媒体库、媒体分页、单个媒体、字幕清单）。`/livez` 与只返回极小状态的 `/readyz` 公开；所有 `/v1/*` 要求独立 Bearer Token，Token 不接受 query 参数。`/readyz` 会实际探测 Emby，不能只根据进程存活返回就绪。应用凭据、`security.identity_key_file` 与 `security.api_auth_token_file` 三者分离，后两者不能复用 Emby API Key。

部署入口包括根目录 [Dockerfile](Dockerfile)、通用 bridge 示例 [deploy/compose.example.yaml](deploy/compose.example.yaml)，以及 Emby 已使用 host 网络时的 [deploy/compose.host-network.example.yaml](deploy/compose.host-network.example.yaml)。所有示例默认保持写能力和远程搜索关闭，实际部署前必须替换占位路径并重新核对只读挂载。

Compose 的 `IMAGE_TAG`、`BUILD_VERSION`、`BUILD_COMMIT`、`BUILD_TIME` 和 `BUILD_SOURCE` 用于固定镜像版本并写入 OCI 构建溯源标签。正式部署应使用不可变标签或摘要，并保留上一份已验收引用以便回滚。

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
