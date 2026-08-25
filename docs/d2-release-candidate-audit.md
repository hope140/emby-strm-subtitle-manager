# D2 发布候选审计

审计日期：2026-08-25（香港时间）

## 结论

当前候选仍不可发布，也不能作为 D2 的不可变提交或部署输入。静态检查、单元测试、CGO-disabled 构建，以及基于本地 Fake Emby 的真实浏览器流程均已通过；但当前 D2 实现仍处于脏工作区，D2 核心文件不在 `HEAD`，没有对应的不可变提交、镜像摘要或 CI 产物证据。

本轮已修复审计确认的两项代码级 P1，并补充了权限失败和 canonical language 回归测试。D2 仍应保持默认关闭，未进行真实 Search/Fetch/Preview/Provider 调用，也未进行任何部署或媒体写入。

## 审计边界

本次只检查当前项目文件和本地 Git 工作区，保持既有脏改动，不 stage、commit、reset、clean 或覆盖既有文件。未访问 SH/上海服务器，未修改或重启 C92，未开启 `remote_search_enabled`，未推送 GitHub，未创建公开 release。

## Git 与候选边界

- 分支为 `main`，`main` 与其跟踪远端无 ahead/behind 差异。
- 当前 `HEAD` 为 D1.5 只读 UI 提交；`HEAD` 没有对应 D2 tag。
- 审计开始时工作区有 27 个已跟踪修改文件和 26 个未跟踪文件；无 staged diff。新增本报告后，仅增加本地文档文件，不改变业务代码。
- D2 的 `internal/d2/`、`internal/preview/`、`internal/subtitle/`、`internal/subtitleprovider/`、`internal/pathsecurity/`、D2 集成测试、Fixture、E2E 脚本及 D2 Compose overlay 均不在 `HEAD` 的树中，当前候选因此不能被一个现有提交唯一引用。
- 普通 `git diff` 不包含未跟踪文件。本次审计同时检查了已跟踪 diff 和所有未跟踪文件内容，不能把普通 diff 为空误认为候选完整。

## 实际检查的文件集合

审计前纳入敏感扫描和文件状态核对的集合为 Git 已跟踪 62 个文件及未跟踪 26 个文件，共 88 个项目文件。除完整的已跟踪文件集外，当前 dirty set 和全部未跟踪文件如下。

已跟踪修改文件：

`.gitignore`、`AGENTS.md`、`Emby_STRM_Subtitle_Manager_Master_Plan_Revised.md`、`README.md`、`cmd/server/main.go`、`deploy/config.example.yaml`、`deploy/config.host-network.example.yaml`、`docs/README.md`、`docs/adr/003-phase2-milestones-and-deployment.md`、`docs/adr/README.md`、`docs/architecture.md`、`docs/d1-deployment-acceptance.md`、`docs/d1.5-deployment-preflight.md`、`docs/d1.5-readonly-ui.md`、`docs/lessons-learned.md`、`docs/phase2-readonly-canary.md`、`internal/config/config.go`、`internal/config/config_test.go`、`internal/domain/domain.go`、`internal/embyclient/client.go`、`internal/embyclient/dto.go`、`internal/httpapi/server.go`、`internal/httpui/assets/app.css`、`internal/httpui/assets/app.js`、`internal/httpui/assets/index.html`、`internal/httpui/handler.go`、`internal/httpui/handler_test.go`。

未跟踪文件：

`cmd/d2-ui-fixture/main.go`、`deploy/compose.d2-canary.example.yaml`、`docs/adr/005-conditional-d2-entry-without-live-multisource.md`、`docs/d2-b1-backend-implementation-review.md`、`docs/d2-b2-readonly-ui-review.md`、`docs/d2-c-c92-canary-preflight.md`、`docs/d2-c92-deployment-preflight.md`、`docs/d2-search-preview-contract.md`、`internal/d2/errors.go`、`internal/d2/limiter.go`、`internal/d2/limiter_test.go`、`internal/d2/service.go`、`internal/d2/service_test.go`、`internal/embyclient/d2_client_test.go`、`internal/httpapi/d2_integration_test.go`、`internal/pathsecurity/path.go`、`internal/preview/allowlist.go`、`internal/preview/artifact_store.go`、`internal/preview/candidate_store.go`、`internal/preview/store_test.go`、`internal/subtitle/subtitle.go`、`internal/subtitle/subtitle_test.go`、`internal/subtitleprovider/provider.go`、`internal/subtitleprovider/provider_test.go`、`scripts/d2-ui-e2e-browser.js`、`scripts/d2-ui-e2e.ps1`。

另外阅读了项目规则、架构、D2 Search/Fetch/Preview 合同、D2-B1/D2-B2 review、C92 preflight、ADR-003/ADR-005、维护经验、Dockerfile、CI、Go 模块配置、验证脚本及 Compose 配置。`LOCAL_OPERATIONS.md` 仅用于边界核对，未把其中的拓扑、凭据位置或私有路径写入本报告。

## 敏感信息与范围扫描

- 88 个项目文件中未发现可识别为真实凭据的内容；认证 URL 扫描结果为 0。
- 发现的 token/API key 字段名、占位符和认证值均属于测试夹具、契约示例或配置占位，不作为真实凭据使用；报告不回显任何值。
- 未发现需要写入报告的真实生产 Item/MediaSource/candidate ID、媒体标题或认证参数 URL。
- 绝对路径命中均为测试、示例、容器路径或占位文本；报告只使用相对项目路径。

## 已执行验证

通过：

- `scripts/verify.ps1`：gofmt、`go vet ./...`、`go test -count=1 ./...`、服务端构建。
- `go build ./...`。
- `go test -count=3 -shuffle=on ./...`。
- `CGO_ENABLED=0 go test -count=1 ./...` 及服务端 `CGO_ENABLED=0` 构建。
- 前端和 Playwright 浏览器脚本的 `node --check`。
- 本地 Fake Emby + Playwright CLI 的 D2 UI 流程，覆盖默认关闭、启用后的 Search、候选隔离、Fetch、Preview、刷新登出、过期和敏感值不进入页面检查。
- `git diff --check`，仅有既存的换行符提示，没有 whitespace error。
- P2 修复后重新运行 `gofmt`、`go test ./internal/httpui`、`scripts/verify.ps1`、`node --check internal/httpui/assets/app.js`、项目 Markdown 检查和 `git diff --check`，均通过。

未通过或未完成：

- `go test -race ./...` 受环境限制失败，原因是本机没有 `gcc`，不能记为 race 通过。
- 本机没有 Docker，因此没有真实 Docker build，也没有用 Compose 引擎合并验证 base 与 D2 overlay。
- 没有 C92 当前运行时、真实 Emby、真实 Provider 或真实客户端验收证据。
- 没有真实多媒体源样本；当前只能保留单源支持和多源安全拒绝结论。

## Findings

### P0

未发现 P0。审计期间没有真实部署、真实远端调用或媒体/Emby 写操作，因此没有发现正在发生的生产事故。

### P1

1. **P1：候选没有不可变发布边界。** D2 核心代码、测试和 overlay 均为未跟踪文件，当前 `HEAD` 不包含它们，也没有对应 tag、精确 CI 运行、镜像 digest 或可回滚发布引用。脏工作区不能充当发布版本。

2. **P1：缓存目录权限存在 fail-open 路径（已修复）。** `internal/preview/artifact_store.go` 现在把 `Chmod(0700)` 的任何错误都映射为 `ErrArtifactUnavailable`，无法确认私有权限时不会继续初始化 Artifact store；`TestArtifactStoreFailsClosedWhenPrivateDirectoryPermissionCannotBeConfirmed` 通过注入权限错误在各平台验证该门禁，不依赖真实生产路径。

3. **P1：Fetch/Preview 的 `language` 可能使用 Provider 展示语言，而非规范绑定语言（已修复）。** Artifact 语言现在由 canonical binding 派生，Fetch/Preview 从 `artifact.Binding.Language` 返回；Search 仍保留 Provider 展示值。Service 单元测试和 Fake Emby HTTP 测试均覆盖 Provider 返回 `zho`、Search 展示 `zho`、Fetch/Preview 返回 `zh-CN` 的契约。

4. **P1：真实发布验收证据缺失。** C92 preflight 文档是历史本地材料，不能证明当前 C92 运行时配置、overlay/cache/allowlist 权限、镜像版本、服务状态或客户端读取结果。真实 Provider、Emby 和客户端验收都尚未执行；在用户明确禁止远端和部署动作的本次任务中不能补齐这些证据。

### P2

1. **P2：UI 健康检查存在非合同字段 fallback（已修复）。** `internal/httpui/assets/app.js` 现在只读取 `/v1/health.features.remote_search_enabled`；`health` 为 `null` 或缺少 `features` 时使用空对象，并通过严格 `=== true` 保持 default-off。`TestD2UIHealthFeatureGateUsesContractFieldAndDefaultsOff` 锁定该响应形状并拒绝顶层同名字段读取，不依赖真实服务。

2. **P2：部分测试失败诊断可能包含合成认证值或请求标识。** 相关值当前来自本地测试夹具，不是真实凭据，但测试输出应继续遵守生产日志的脱敏边界，避免失败时直接 dump 请求对象或认证 header。

3. **P2：race、Docker/Compose 和真实客户端验证仍是环境或验收缺口。** 它们不等同于源码通过；在补齐工具和授权前，不能写成“全绿”或“已发布”。

## 尚缺的发布不变量

形成真正发布候选前，至少还需要：

- 明确本次 D2 文件集合；本轮两项代码级 P1 已修复并通过测试，仍需由负责人创建包含全部候选文件的不可变 commit。
- 对该精确 commit 取得 CI、构建产物和镜像/OCI revision digest，并保留可回滚引用。
- 用 Docker/Compose 引擎验证 base + D2 overlay 的最终配置，确认缓存和 allowlist 的路径、权限、挂载方向及不重叠约束。
- 在授权的 C92 canary 上确认 D2 默认关闭；若进入 canary，再以固定范围完成真实 Emby、Provider 和客户端的 Search/Fetch/Preview 验收。真实多源验收前继续拒绝多源，不宣称支持。
- 记录真实客户端读取、artifact 生命周期、过期清理、限流和故障隔离证据，并单独保留 rollback 方案。
- 权限测试当前通过注入 `Chmod` 错误验证跨平台 fail-closed 分支；本轮未在 Unix 上构造真实拒绝权限的文件系统夹具，也未单独验证 Windows ACL。运行时不因平台差异放宽语义，任何权限确认错误仍返回 `ErrArtifactUnavailable`。

## 下一步

1. 保留本轮 P1/P2 修复和回归证据；由负责人审定 dirty set，并为候选创建包含全部 D2 文件的不可变提交。
2. 形成包含 D2 文件的不可变提交并运行针对该提交的 CI/镜像验证。
3. 只有获得单独授权后，才进行 C92 本地部署前检查和 canary 验收；SH/上海服务器不在本任务范围内。
4. 多源支持另走 ADR-005 规定的独立门禁。

## Knowledge Review

本次新增的是一次性发布候选审计报告；本轮在其基础上修复了两项代码级 P1 和一项 UI health shape P2，并把可复核的权限门禁、canonical language 与嵌套 health feature gate 测试证据回填到本报告，没有把未完成的远端、部署或客户端推断写入架构事实，也没有新增长期 ADR。审计依据为当前源代码、测试、构建结果、本地 Fake Emby 浏览器结果、Git 状态/diff 和项目合同文档；真实 canary 完成后仍应把新的证据分别回填到对应阶段报告或长期知识文档。本次没有新增 `docs/lessons-learned.md`、`docs/architecture.md`、`docs/adr/` 或 `LOCAL_OPERATIONS.md` 的长期结论。

## 未执行的远端与部署动作

本次未访问 SH/上海服务器，未连接或修改 C92，未部署、重启或查看远端服务，未开启 `remote_search_enabled`，未调用真实 Search/Fetch/Preview/Provider，未写媒体文件或 Emby 元数据，未推送 GitHub，未创建公开 release。
