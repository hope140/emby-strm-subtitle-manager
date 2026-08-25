# D2-B1 后端实现 Knowledge Review

任务或阶段：D2-B1 后端、安全门禁和 Fake Emby 测试

验证范围：`AGENTS.md`、`docs/architecture.md`、D2 搜索预览契约、ADR-003、ADR-005、`docs/lessons-learned.md`、D2 配置、Emby Remote Subtitle 客户端、Provider、Candidate/Artifact Store、字幕解析器、D2 Service、HTTP API、Fake Emby 集成测试及既有 D1 测试。

## Knowledge Findings

- 新增约束　D2 必须在服务端开关、Canary 和 Item allowlist 均满足后才接触 Emby；Candidate/Artifact 只保存 Token 摘要和服务端映射，allowlist generation 变化会使旧状态失效。
- 新增约束　启用 D2 时必须配置显式、绝对、稳定且位于媒体映射之外的专用 `d2.cache_dir`；缺失时 fail closed，启动同一目录会清理旧 Artifact 文件，不再创建临时缓存目录。
- 隐蔽坑　私有路径 overlap 必须双向判断，媒体祖先、媒体子目录和相等路径都拒绝；文件系统/卷根以及 cache_dir 本身或父链的 symlink/reparse point 也不能交给只读 D2 做 `chmod` 或清理。
- 部署约束　D2 cache bind 和 allowlist file-source 只能通过显式 Canary overlay 注入；两份基础 D1 Compose 不依赖 D2 目录或 Secret，rootfs 与媒体挂载的只读边界保持不变。
- 隐蔽坑　Provider 搜索返回的语言展示值与服务端绑定使用的 canonical language 是两个字段；Preview 必须重新读取一次 Item，但不能重新触发 Provider Fetch。限流滚动窗口必须淘汰过期事件，不能淘汰刚发生的事件。
- 被证明错误的假设　D1 的 `GetItem` 只接受 Movie/Episode 会阻止 D2 对非支持类型返回稳定的 `unsupported_media_type`；D2 需要保留详细 Item 类型后再由 D2 门禁判断。
- 建议沉淀项　候选级失败隔离、固定 Remote Search/Fetch GET 边界、一次性 410 后的 404、私有缓存目录权限和真实 Canary 与 Fake Emby 证据分层继续作为 D2 运行验收清单。

## 证据

- 代码　`internal/d2`、`internal/preview`、`internal/subtitle`、`internal/subtitleprovider`、`internal/embyclient`、`internal/httpapi`、`internal/config` 和 `cmd/server` 已形成后端闭环；D2 不提供 Save、Refresh 或媒体目录写入接口。
- 测试　全包 Go 单元测试覆盖配置、门禁、限流、Token/Artifact 生命周期、稳定缓存目录重启回收、根目录/双向媒体 overlap/symlink 防护、UTF-8/UTF-16、SRT/ASS、Provider 重试和绑定；Fake Emby 集成测试覆盖关闭时零上游请求、固定 query、候选失败隔离、Fetch 幂等、Preview 单次 GetItem、多源 409、请求体限制、无 Save/Refresh 和日志脱敏；Compose schema 测试证明 base 无 D2 依赖、overlay 合并后才有专用写挂载和 allowlist Secret。
- 实际运行、日志或可复现结果　已运行 `scripts/verify.ps1`（包含 gofmt、go vet、全包测试和构建）、`go test -count=3 -shuffle=on ./...`、`go build ./...`、`git diff --check` 和项目 Markdown 检查；Fake Emby 只使用本地 httptest，Compose YAML/schema/invariant 测试通过。当前 Windows 没有 `docker`/`docker compose`，未执行真实 Compose merge/up、Provider Canary、真实客户端验收、部署或重启。`go test -race ./...` 受当前 Windows `CGO_ENABLED=0` 且未提供 C 编译器限制，未宣称 race 通过。

## 去重检查

- 已搜索的文档和关键词　`D2`、`Search`、`Fetch`、`Preview`、`Candidate`、`Artifact`、`MediaSource`、`allowlist`、`Save`、`Refresh`、`remote_search_enabled`、`docs/adr/`。
- 是否更新已有结论　是。当前实现事实补充到架构和 D2 契约状态；候选失败隔离等既有维护经验不重复改写。

## 分流判断

- `docs/lessons-learned.md`　不需要新增；已有候选失败隔离和真实验收分层结论足够覆盖本轮通用经验。
- `docs/architecture.md`　更新 D2-B1 已实现边界和证据链接。
- `docs/adr/`　不需要新增或更新 ADR；单源条件入口和真实多源独立门禁仍由 ADR-005 负责。
- `LOCAL_OPERATIONS.md`　不需要更新；本轮没有新的本机拓扑、连接方法或恢复步骤。

## 未验证范围与残余风险

- 真实 Emby/Provider Canary、真实客户端字幕读取和部署启动尚未执行。
- 真实多 MediaSource 样本仍缺失；Fake Emby 的 409 只能证明 fail-closed 逻辑，不能宣称真实多源支持。
- D1.5 UI 尚未接入 D2 Search/Fetch/Preview，本轮未修改 UI。
