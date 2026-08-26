# D1 部署验收报告

状态：D1 部署、公网 HTTPS 与 Movie/Episode STRM Canary 已通过；按 ADR-005 已满足有条件进入单源 D2 的门禁，真实多媒体源支持门禁尚未完成。

本报告记录 D1 只读切片在已授权 C92 环境的部署和真实 STRM Canary 结果。报告只保留可复核的摘要，不记录私有路径、Item 标识、媒体标题、Secret、认证参数或私有 URL。

## 版本与归档

- 验收代码提交：`95746a29b1466184e6db842c3412fefea4f379aa`
- 归档 SHA256：`ACCB131ADC9B0E17C86CA1DE11A3738A44D2CAE61E114B087B984C5F595F4AB3`
- 运行镜像 ID：`sha256:4641b78bb18a70ea4e7e27d60144fed755a036f5e052308717ae03d6b766c0b5`
- GitHub Actions：[CI run 32702979650](https://github.com/hope140/emby-strm-subtitle-manager/actions/runs/32702979650)，格式化、Vet、Test 和 Build 全部通过。

## 已通过

- Docker Compose schema、镜像构建和 host-network 变体通过。
- 镜像 OCI `version`、`revision`、`created` 和 `source` 与公开 GitHub 提交及构建记录一致；历史重写前的旧镜像只保留为短期回滚件，不再作为发布验收镜像。
- 容器 UID 10001、只读 root、媒体只读挂载和三份 Secret 权限检查通过。
- `/readyz` readiness 检查、Bearer 认证错误时的 401 行为和版本溯源标签检查通过。
- FRP 新代理启用单代理 payload 加密，原有 9 条代理与新增代理均保持 running；共享 FRPC 的全局 TLS 配置未变更。
- SH loopback remote port 的 `/readyz` 返回 200；主机防火墙对公网应用 remote port 有显式 IPv4/IPv6 DENY，独立外部探针失败且 DROP 计数增加。
- 公网 HTTPS 证书严格校验通过；`/readyz` 返回 200，HTTP 跳转 HTTPS，无、错误或 query Bearer 返回 401，正确 Bearer 返回 200。
- Linux 全包测试通过且无 skip。
- 真实库浏览中的 Movie 与 Episode STRM 均为 mapped、inventory complete、present，且无 warning。
- `write_enabled=false`、`remote_search_enabled=false` 保持关闭；本次没有搜索或写入行为。

## 尚未完成

- 2026-08-24 的真实多 MediaSource 门禁复查仍未找到样本。第一轮对 11 个媒体库逐库检查 `DateCreated` 最新页，最多 1,200 个 Item，实际检查 1,026 个 Item、12 个 GET 请求；第二轮对 11 个媒体库按 `SortName` 检查首段、中段和尾段，每页 30 个，最多 1,000 个唯一 Item，实际检查 938 个 Item、33 页、34 个 GET 请求；两轮均为 0 个包含多个 `MediaSource` 的 Movie/Episode。
- 真实 Emby 的总 Movie/Episode 计数为 7,317，本轮是有界取样，不是全库无多源证明。扫描只使用 `GET /Library/MediaFolders` 和带 `Fields=MediaSources` 的 `GET /Items`，请求方法均为 GET；没有调用字幕、RemoteSearch、Fetch、Refresh、Playback 或写入接口。
- 自动化的 409 和显式 `media_source_id` 选择测试已通过，但不能替代真实多源 Item 验收。因本轮没有真实候选，未对真实 Item 发起详情/字幕清单 source 对应验收，也未以合成样本替代该门禁。
- 当前个人部署保留面板默认 access log 配置；验收期间从未通过 query 发送真实 Token。面向发布的统一安全日志与反代模板见 [OpenResty 公网入口基线](../../guides/openresty-public-entry.md)，它不会自动修改现有服务器配置。

## 结论与下一步

D1 的代码、自动化、C92 部署、公网 HTTPS 和 Movie/Episode STRM 只读 Canary 已具备可复核证据。本轮有界真实 Emby 扫描仍未补齐多媒体源样本；按 [ADR-005](../../decisions/adr/005-conditional-d2-entry-without-live-multisource.md)，这不再阻断 D2 契约、实现和单源 Canary，但继续阻断真实多源搜索的支持声明与启用。`write_enabled=false` 和 `remote_search_enabled=false` 继续保持默认关闭，实际搜索需另行完成 D2 专项授权和验收。

真实多源后续方案：

1. 由环境负责人提供或在 Emby 管理界面确认一个已知的多版本 Movie/Episode，随后只对该 Item 做一次脱敏、只读的 API 与 D1.5 浏览器验收。
2. 在获得明确授权后扩大扫描上限或改用更密集的分层取样；扩大范围前仍需保留请求数、Item 数和终止条件。
3. 若必须构造专用多源样本，需要另行明确 Emby/媒体变更授权；本轮不通过修改 Emby 或媒体来制造样本。

## 2026-08-24 D1.5 表面检查

- 当前 UI 根页面和 `assets/app.js` 的无凭据 GET 表面检查返回 200；`/v1/health` 缺少 Bearer 时返回 401。
- 前端代码包含 409 source-selection 分支，选择按钮以 `media_source_id` 重新请求详情和字幕清单；当前 `scripts/verify.ps1` 已通过，包含 Fake Emby 的多源选择测试。
- 由于真实扫描没有命中多源 Item，本轮没有宣称真实 API/UI 的 source 一致性通过；该项保留为多源搜索支持的独立门禁，不再阻断单源 D2。

## Knowledge Review

任务或阶段：D1.5 后真实多 MediaSource 只读门禁复查

验证范围：真实 Emby 两轮有界 GET 扫描、当前 D1.5 UI 无凭据表面、Go 格式/Vet/单测/构建、现有 Fake Emby 多源 API 测试和文档去重检查。

Knowledge Findings

- 新增约束：有界取样未命中不能改写为“全库不存在多源”；真实多源 Item 仍是多源搜索支持声明和启用的前置门禁，但不再阻断单源 D2。
- 隐蔽坑：仅检查最新排序页可能遗漏旧媒体库条目，因此本轮增加首段/中段/尾段取样；该策略仍只产生样本证据。
- 被证明错误的假设：无。没有把 Fake Emby 或 UI 静态表面当作真实多源验收。
- 建议沉淀项：保留本报告中的上限、取样策略、实际请求数和实际 Item 数，下一次扩大范围时先比较覆盖差异。

证据

- 代码：`internal/media`、`internal/httpapi` 和 `internal/httpui/assets/app.js` 的显式 source 选择实现。
- 测试：`scripts/verify.ps1`，格式检查、`go vet ./...`、`go test -count=1 ./...`、服务构建全部通过。
- 实际运行：真实 Emby 两轮脱敏扫描均为 0 个多源 Movie/Episode；UI 根页面/脚本 GET 200，未认证 `/v1/health` 为 401。

去重检查

- 已搜索：`AGENTS.md`、`docs/architecture.md`、`docs/lessons-learned.md`、`docs/adr/README.md`、ADR-003、ADR-004、Phase 2 只读 Canary、D1/D1.5 验收文档、相关 API/UI 测试。
- 是否更新已有结论：更新本报告的实时扫描证据，并通过 ADR-005 将单源 D2 入口与真实多源支持门禁拆分。

分流判断

- `docs/lessons-learned.md`：新增本次“有界取样不能证明全库无多源”的通用边界。
- `docs/architecture.md`：同步 D2 条件入口和真实多源支持边界。
- `docs/adr/`：新增 ADR-005，记录门禁拆分及 fail-closed 要求。
- `LOCAL_OPERATIONS.md`：同步更新本机长期快照，不写入媒体名称、Item/MediaSource ID、路径或凭据。

未验证范围与残余风险：真实多源 Item 尚未找到，因而真实详情与字幕清单对应所选 source、真实浏览器选择流程和全库覆盖均未验证；单源 D2 可以继续，多源搜索必须保持安全拒绝且不能宣称已支持。
