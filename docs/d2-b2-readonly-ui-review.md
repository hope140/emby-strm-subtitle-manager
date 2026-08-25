# D2-B2 内嵌只读 UI Knowledge Review

任务或阶段：D2-B2 搜索、候选 Fetch 与纯文本预览 UI

补充说明（2026-08-25）：本报告记录的 Bearer-only UI 是 D2-B2 当时的历史证据。发布版当前改为管理员用户名/密码登录和 HttpOnly 会话，自动化 Bearer 不再出现在 UI 输入框；最新认证契约和验证见 [D2.5 管理员认证](d2.5-admin-auth.md)。

验证范围：`AGENTS.md`、`docs/architecture.md`、D2 搜索预览契约、ADR-003、ADR-005、维护经验、现有 D1.5 UI、`internal/httpui`、本地 Fake Emby 夹具，以及 Playwright CLI 浏览器流程。

## Knowledge Findings

- 新增约束　UI 只能由服务端 `/v1/health` 的开关状态展示 D2 控件；按钮存在不构成启用，服务端开关、Canary allowlist 和单源门禁仍是唯一授权边界。
- 新增约束（Bearer-only 历史版本）　Bearer、Candidate Token 与 Artifact Token 只保留在 JavaScript 内存；不写 browser storage、Cookie、URL、DOM attribute 或控制台。刷新、退出、切换媒体库都会清除相关 UI 状态。当前管理员会话的 HttpOnly 边界见 D2.5 报告。
- 隐蔽坑　候选失败必须局部渲染，不能清空其他候选；预览错误后的下一次成功渲染也必须清除错误样式，避免旧失败状态误导用户。
- 被证明错误的假设　只做静态字符串检查不足以证明 token 不落盘和刷新后登出；需要在真实浏览器中同时检查交互、存储、DOM 与控制台。
- 建议沉淀项　D2 UI 的浏览器验收要覆盖“默认关闭、候选 A 失败但 B 可用、预览分页、Artifact 过期、刷新回登录态”五条最小路径。

## 证据

- 代码　`internal/httpui` 增加同源 D2 状态、单源门禁展示、候选卡、Fetch、纯文本 cue 分页与稳定错误提示；没有 Save、Refresh、下载或其他写入入口。`cmd/d2-ui-fixture` 仅监听本地回环地址，使用运行时随机假候选 ID/假上游凭据，不能作为生产入口。
- 测试　`internal/httpui` 静态测试验证内嵌资源、CSP、无外部资源、无 persistent storage 标记、受限 D2 API surface 和无写入标签；`node --check` 验证 UI 与浏览器 E2E 脚本语法。
- 实际运行、日志或可复现结果（Bearer-only 历史版本）　`./scripts/d2-ui-e2e.ps1` 在本地 Fake Emby 与真实 Playwright 浏览器中通过：关闭时控件隐藏；启用时搜索、候选失败隔离、Fetch 成功、200/500 cue 分页和 Artifact 过期提示均正确；浏览器 localStorage、sessionStorage、Cookie 为空，DOM/控制台未出现 Token、原始候选 ID、上游凭据或媒体路径，刷新后返回登录页。管理员用户名/密码和 HttpOnly Cookie 的最新流程见 D2.5 报告。

## 去重检查

- 已搜索的文档和关键词　`D2`、`Search`、`Fetch`、`Preview`、`Token`、`MediaSource`、`remote_search_enabled`、`D1.5`、`Playwright`、`Save`、`Refresh`。
- 是否更新已有结论　是。当前 UI 实现事实写入架构、D2 契约、D1.5 UI 说明和文档索引；候选失败隔离和真实验收分层已在既有维护经验中，不重复添加。

## 分流判断

- `docs/lessons-learned.md`　不需要更新；本轮的通用结论已由既有候选隔离与浏览器/真实验收分层覆盖。
- `docs/architecture.md`　更新 D2 UI 的已实现边界与本地浏览器证据。
- `docs/adr/`　不需要新增或更新 ADR；D2 条件入口、默认关闭和真实多源门禁仍由 ADR-005 负责。
- `LOCAL_OPERATIONS.md`　不需要更新；没有新增持久拓扑、连接方式或恢复步骤。

## 未验证范围与残余风险

- 本轮只有本地 Fake Emby/Provider 和浏览器；没有启用真实远程搜索、访问真实 Provider、部署、重启或真实客户端验收。
- 真实多 MediaSource 样本仍缺失；UI 对多源保持不展示 D2 控件，不能宣称真实多源搜索支持。
- Playwright CLI 的会话守护进程只属于本地测试；脚本会按唯一会话名关闭并回收它，生产运行不依赖该工具。
