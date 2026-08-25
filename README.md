# SubBridge（SB，字幕桥）

SubBridge（SB，字幕桥）为 Emby 与 STRM 媒体库提供中文字幕浏览、搜索、预览和安全管理能力。CMS 继续负责媒体整理，Emby 继续负责媒体索引与播放，第一阶段通过 MeiamSub 获取 Thunder 和 ASSRT 字幕候选。

项目品牌、GitHub 仓库、Go module、构建产物和面向新安装的 Docker/Compose 示例统一使用 SubBridge；既有 C92 容器、镜像、目录、FRP proxy 和历史验收记录中的 `emby-strm-subtitle-manager` 继续作为兼容与追溯标识，不因品牌改名原地迁移。详细边界见 [ADR-007](docs/adr/007-subbridge-brand-and-legacy-deployment-identifiers.md)。

当前优先完成字幕管理核心能力。现有 UI 只作为功能测试和最小操作入口，不在核心功能完成前进行媒体库层级、设置页、日志页或整体视觉重构；压缩后的顺序见 [当前状态与后续路线图](docs/current-status-and-roadmap.md)。

## 当前状态

当前统一的完成度、缺口和建议顺序见 [当前状态与后续路线图](docs/current-status-and-roadmap.md)。简要结论是：D1、D2 单源后端、D2.5 和 D3.1 专用单源 Add 已完成真实验收；日常 Add、多源正向流程、Replace、Upload、Delete 和正式镜像发布仍待推进。

[Gate 0 实测](GATE0_REPORT.md)已经正式通过。真实环境已经验证 Emby API Key 搜索与 Fetch、STRM 网络边界、外部字幕写入、Emby 直连读取和受限范围内的缓存行为。

V1 已决定使用 Emby Remote Subtitle Bridge。Native Provider 暂缓，单个失效候选按候选级错误处理。

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。ADR-002 已接受，选择新建轻量 Go 后端并选择性复用 ChineseSubFinder。

D1 只读代码切片、Linux 全包自动化验证、C92 部署和 FRP 公网 HTTPS 验收已经完成：Docker Compose schema/build、host-network、UID 10001、只读 root、只读媒体、三份 Secret 权限、`/readyz`、Bearer 认证、版本溯源标签、公网 443 及应用 remote port 防火墙边界均已核对；真实库浏览中的 Movie 与 Episode STRM 也已确认 mapped、inventory complete、present 且无 warning。C92 已找到真实 Movie 版本组，详情请求必须包含 `AlternateMediaSources` 才能取得完整 source 列表；客户端字段修正、本地回归和两个真实 Item 的 source 对应核对已完成，应用对多源请求安全返回 409。[ADR-005](docs/adr/005-conditional-d2-entry-without-live-multisource.md) 允许继续 D2 契约、实现和单源 Canary，多源搜索在完整门禁前必须安全拒绝，也不得宣称支持。`write_enabled=false` 和 `remote_search_enabled=false` 仍保持默认关闭，实际启用搜索仍需 D2 专项授权和验收。详见 [D1 部署验收报告](docs/d1-deployment-acceptance.md)、[D2 搜索预览契约](docs/d2-search-preview-contract.md)、[D2 多版本 MediaSources 实测记录](docs/d2-multisource-c92-sample.md)、[OpenResty 公网入口基线](deploy/openresty/README.md)、[ADR-002](docs/adr/002-project-codebase-route.md)、[ADR-003](docs/adr/003-phase2-milestones-and-deployment.md)、[Phase 2 只读 Canary](docs/phase2-readonly-canary.md)、[Phase 1 基线报告](BASELINE.md) 和 [Phase 1 基线检查表](docs/phase1-baseline-checklist.md)。

D1.5 增加了同一 Go 服务内嵌的最小只读 Web UI。发布版 UI 使用私有 Compose environment 配置的管理员用户名和密码登录，服务端签发短期 HttpOnly 会话；密码不进入页面存储，面板不提供改密或注销。CLI、定时任务和 CI 继续使用独立 Bearer Token。它只浏览既有媒体库、Movie/Episode 分页、媒体详情和字幕清单，不增加搜索或写能力。使用方式和公网 HTTP/HTTPS 边界见 [D1.5 最小只读 Web UI](docs/d1.5-readonly-ui.md)，管理员认证实现见 [D2.5 管理员认证](docs/d2.5-admin-auth.md)，部署前检查见 [D1.5 部署前预检](docs/d1.5-deployment-preflight.md)。

D3 专用样本 Add 已完成本地实现和 C92 真实闭环验收，包含管理员会话 CSRF、独立 `subtitle:write` scope、D3 allowlist、Artifact 绑定、原子非覆盖版本文件、Emby Refresh/轮询、history/quarantine、字幕流回读，以及 Emby Web 和手机端实际客户端读取。验收结束后已恢复 closed 配置、`write_enabled=false`、`remote_search_enabled=false` 和 `/media:ro`。这只代表 D3.1 专用 Add 完成，不代表 Phase 4 的 Replace、Delete、Upload 和日常全库写入完成。详见 [D3 专用样本 Add 契约](docs/d3-dedicated-add-contract.md) 与 [D3 C92 Canary 验收](docs/d3-c92-canary-acceptance-20260825.md)。

当前服务还提供 `/` 与 `/assets/{asset}` 的内嵌 UI、D1 的 7 个 GET API/运维路由、默认关闭并受 Canary allowlist 保护的三个 D2 POST 路由（Search、Fetch、Preview），以及默认关闭的 D3 专用 Add 路由。UI 只在服务端 health 明确报告远程搜索已启用且当前为单源 Movie/Episode 时展示 D2 控件；D3 只有在写开关、专用 allowlist 和 Artifact 都满足时展示 Add。`/livez` 与只返回极小状态的 `/readyz` 公开；`/v1/auth/login` 使用 Compose environment 中的管理员凭据登录，其余 `/v1/*` 接受短期管理员会话或按路由检查 scope 的独立 Bearer，Bearer 不接受 query 参数。`/readyz` 会实际探测 Emby，不能只根据进程存活返回就绪。应用凭据、`security.identity_key_file`、`security.api_auth_token_file` 和管理员 environment 分离，管理员密码不能复用 Emby API Key。真实 C92 单源 Provider API Canary 已通过；尚缺的是 C92 管理 UI 中 Search→Fetch→Preview 的完整点击验收。D3 C92 真实闭环见 [D3 C92 Canary 验收](docs/d3-c92-canary-acceptance-20260825.md)。

部署入口包括根目录 [Dockerfile](Dockerfile)、通用 bridge 示例 [deploy/compose.example.yaml](deploy/compose.example.yaml)、Emby 已使用 host 网络时的 [deploy/compose.host-network.example.yaml](deploy/compose.host-network.example.yaml)，以及仅在独立授权后合并的 [D2 Canary Compose overlay](deploy/compose.d2-canary.example.yaml) 和 [D3 专用样本 Add overlay](deploy/compose.d3-canary.example.yaml)。默认 base Compose 不依赖 D2 cache、D3 history/quarantine 或 allowlist 文件；所有示例默认保持写能力和远程搜索关闭，实际部署前必须替换占位路径并重新核对只读挂载。

Compose 的 `IMAGE_TAG`、`BUILD_VERSION`、`BUILD_COMMIT`、`BUILD_TIME` 和 `BUILD_SOURCE` 用于固定镜像版本并写入 OCI 构建溯源标签。正式部署应使用不可变标签或摘要，并保留上一份已验收引用以便回滚。

## 文档入口

| 文档 | 用途 |
|---|---|
| [当前状态与后续路线图](docs/current-status-and-roadmap.md) | 已完成、部分完成、待做和建议推进顺序的统一入口 |
| [Core A/B 连续实施计划](docs/core-ab-implementation-plan.md) | 核心字幕管理能力的连续实现范围、测试矩阵和停止条件 |
| [总体规划](SubBridge_Master_Plan_Revised.md) | 产品范围、数据模型、阶段和验收条件 |
| [Gate 0 实测报告](GATE0_REPORT.md) | 真实环境验证结果和证据边界 |
| [D1 部署验收报告](docs/d1-deployment-acceptance.md) | C92 部署、真实 STRM Canary 和剩余门禁 |
| [D2 搜索预览契约](docs/d2-search-preview-contract.md) | D2 Search、Fetch、Preview 契约、安全边界和实现测试矩阵 |
| [D2.5 管理员认证](docs/d2.5-admin-auth.md) | Compose environment 管理员登录、HttpOnly 会话、Bearer 自动化分离和剩余门禁 |
| [D2-B2 UI 评审](docs/d2-b2-readonly-ui-review.md) | D2 UI、浏览器存储边界和本地 Fake Emby E2E 证据 |
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
- 不把已验收的专用 Add 扩展为未经过契约和真实验收的 Replace、Delete、Upload 或批量写入
- 未经明确要求不部署、不重启 Emby，也不发布外部版本
