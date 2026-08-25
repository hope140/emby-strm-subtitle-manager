# D2-C C92 可回滚部署前核对报告

- 日期：2026-08-25（香港时间）
- 范围：仅 C92；未连接、读取或修改 SH/上海服务器
- 结论：**阻断，未部署 D2，未重启，未执行真实 Search、Fetch、Preview 或 Provider 请求**

## 1. 执行边界

本轮只做本地源码/文档核对和 C92 只读预检：

- 保留工作区全部既有未提交改动；没有 reset、clean、覆盖无关文件、提交或推送。
- 没有修改 C92 的 Compose、配置、Secret、容器、镜像、媒体、字幕、Emby 元数据或播放状态。
- 没有合并 D2 overlay，没有创建 D2 cache/allowlist，也没有改变 `remote_search_enabled`。
- C92 只读查询没有输出 Token、API Key、Cookie、认证 URL、原始 Item/MediaSource/candidate ID、媒体标题或完整私有路径。
- 只请求了 C92 应用的存活、就绪、根页面、健康认证边界和静态资源；没有调用 D2 的三个 POST 路由或访问 Provider。

## 2. C92 已核对的实时事实

### 2.1 应用容器、镜像和运行安全边界

| 项目 | 实时结果 |
|---|---|
| 应用容器 | `emby-strm-subtitle-manager-app-1`，running/healthy |
| restart count | `0` |
| 当前镜像引用 | `emby-strm-subtitle-manager:d1.5-95782b4a8ca2` |
| 当前镜像 ID | `sha256:ed2fe89c56d69f1402cf3950688d112450b85118c6b3cfdbc5fb69d6c91b84f1` |
| OCI version | `0.1.0-d1.5` |
| OCI revision | `95782b4a8ca2fa40ffcfd5519315f48d171bb559` |
| 容器用户 | `10001:10001` |
| rootfs | `ReadonlyRootfs=true` |
| 网络 | `host`；运行时没有 Docker 端口映射 |
| 应用媒体挂载 | `/media` 为 bind、`RW=false` |
| 配置挂载 | 只读 |

`emby-server` 也处于 running/host-network；本轮只确认其媒体挂载存在，未读取媒体目录、标题或文件内容。应用容器与 Emby 容器的权限边界没有被本轮改变。

### 2.2 C92 只读 HTTP 和健康状态

| 检查 | 结果 |
|---|---|
| `/livez` | HTTP 200 |
| `/readyz` | HTTP 200 |
| 无 Bearer 的 `/v1/health` | HTTP 401 |
| 根页面 `/` | HTTP 200 |
| 受保护 health 的当前版本 | `0.1.0-d1.5` |
| 受保护 health 的 `write_enabled` | `false` |
| 受保护 health 的 `remote_search_enabled` | `false` |
| 实时 `app.js` 中 D2 Search/Fetch/Preview 路由标记 | 未发现 |
| 实时 `app.js` 中 `remote_search_enabled` 标记 | 未发现 |

健康接口使用了 C92 容器内已有的应用凭据进行本地内存内认证，凭据未输出、未写入文件，也未进入报告。

### 2.3 Compose 拓扑和 D1 Secret 角色

C92 当前 Compose 文件可由 Docker Compose `v2.39.1` 成功解析，Docker Server 为 `27.3.1`，服务只有 `app`，使用 host network、`10001:10001`、`read_only: true` 和只读 `/media`。当前 Compose 只声明三个 D1 file-source Secret：

- `emby_api_key`：服务端 Emby 访问凭据
- `app_identity_key`：应用身份/本地稳定标识用途
- `app_api_auth_token`：应用管理 API Bearer 用途

C92 宿主上三个 Secret 文件均存在，实时元数据为 owner `10001:10001`、mode `0400`；以应用用户执行的容器内只读预检显示三者均可读，配置文件也可读。Secret 内容没有被读取到报告中。

## 3. C92 D2 准备状态

| 门项 | 实时结果 | 判定 |
|---|---|---|
| D2 overlay | C92 部署目录未发现 `compose.d2-canary.yaml` 或同类 D2 overlay | 未准备 |
| D2 cache bind | 应用容器没有 `/var/lib/emby-strm-subtitle-manager/d2-preview-cache` 挂载 | 未准备 |
| `d2_canary_items` | C92 宿主 Secret 目录中不存在，容器内不可读/未挂载 | 未准备 |
| D2 cache 专用目录权限/与媒体 overlap | 因目录尚未准备，未进行伪造核对 | 无证据 |
| D2 runtime flags | 受保护 health 明确显示 `remote_search_enabled=false`；未看到 D2 Canary 运行时字段 | 保持关闭 |
| D1 写入开关 | 受保护 health 明确显示 `write_enabled=false` | 保持关闭 |

当前部署目录中可以看到旧 Compose、配置和 release 目录；没有把它们误认成 D2 overlay 或 D2 cache。默认 D1 Compose 的三份 Secret、只读 rootfs 和只读媒体挂载仍然保留。

## 4. 回滚证据

本轮没有执行任何需要回滚的 C92 变更。现有 D1 回滚点已只读核对：

| 回滚对象 | 实时结果 |
|---|---|
| 当前 D1.5 镜像 | tag `d1.5-95782b4a8ca2` 存在；ID 如第 2 节 |
| 上一个 D1 镜像 | tag `d1-a38b1f7ee391` 存在；ID `sha256:9f6cd5892126b092170f010a516166f3be21ea5b42808c903eb349bf5b6458dc` |
| 更早 D1 镜像 | tag `d1-d6ba61da5c6b` 存在；ID `sha256:72ee490926ef7614b854101615f0c1e10ef0339e33420ea97771b8ad01dd9fe7` |
| 当前 Compose | 可解析；SHA-256 `81f63feaf15bb3d8e947370a717efcad198a9de0ee733a519ecdbe78ca347e24` |
| 上一份 Compose 归档 | 可解析；SHA-256 `8e099966dca9aed3889ccd8788158f6b97f6262ac982087db0aeaf798fd57e30` |
| 更早 Compose 备份 | 可解析；SHA-256 `a72e4af5106c93c04523f46db10ccb7b49edbe36aa7f34337c9bd7eb3407f001` |
| 当前配置文件 | 已存在；SHA-256 `89a85889c21f53e0ad41192a331830e0f7d3ea134a737f5392d58c52006ceb21` |

当前和旧 Compose 解析结果均保留 host network、应用用户、rootfs 只读和媒体挂载只读边界。当前镜像没有 registry `RepoDigest`，但本地 image ID 可复核；这只能作为现有 D1 本地回滚点，不能作为 D2 发布凭据。

## 5. 本地 D2 源码与不可变引用核对

### 5.1 本地验证结果

当前 Windows 工作区完成了不写入源文件的验证：

- `go test -count=1 ./...`：通过，包含 D2、Fake Emby 集成和相关单元包。
- `go build ./...`：通过。
- `go vet ./...`：通过。
- `node --check internal/httpui/assets/app.js`：通过。
- `git diff --check`：通过；仅有现存的 LF/CRLF 转换警告，没有 whitespace failure。
- 本机 Docker：不可用，因此没有在本机执行 Docker build 或 Compose merge/up。

这些是本地静态、构建和 Fake 测试证据，不是 C92 D2 运行时或真实 Provider 证据。

### 5.2 dirty 状态和引用结论

- HEAD 为 `95782b4a8ca2fa40ffcfd5519315f48d171bb559`，提交主题是 D1.5 只读 UI；C92 当前镜像正是该 revision。
- 在本报告生成前，工作区共有 52 个既有 dirty 条目，其中 27 个 tracked 文件有未提交修改、25 个文件未跟踪；暂存区为 0。本报告是本轮新增的报告文件，未计入以下审计指纹。
- D2 核心新增路径和 D2 overlay 示例不在 HEAD 的 Git tree 中；它们主要以未跟踪文件存在，同时还有若干 tracked D1/D2 集成修改。
- 对全部 52 个 dirty 文件计算出的审计指纹为 `6e5e1182316c27eb4809765705afe636f73236cabb2e7b654df356c9f466e7ae`。该指纹包含用户的其他未提交改动，不能当作 D2 发布版本、Git commit 或镜像引用。

因此，当前本地 D2 代码**不能形成明确的不可变部署引用**。要得到可部署引用，至少还需要项目负责人审核后形成明确 commit，并通过受控构建/发布流程得到可验证的镜像 ID 或 registry digest。此过程可能需要 commit/push、CI 或镜像仓库权限，本轮没有擅自执行，也没有创建外部账户或凭据。

## 6. 阻断项、下一步和未做的外部变化

### 已确认的阻断项

1. C92 当前运行的是 D1.5，不是包含本地 dirty D2 代码的镜像。
2. 本地 D2 代码没有 commit 级不可变引用；当前 dirty 指纹不能替代 release revision。
3. C92 没有 D2 overlay、非空 allowlist 或专用 cache bind，不能安全地进入 D2 Canary 预检后的部署阶段。
4. 本轮没有取得或猜测 D2 发布所需的 commit/CI/registry 路径；因此不输出部署命令草案，也不宣称具备执行条件。

### 可执行的下一步

1. 主线程先审核当前 dirty D2 代码和本报告，决定正式的 commit、CI/镜像仓库和不可变镜像引用来源。
2. 在取得该引用后，仅在 C92 重新核对目标镜像 ID、OCI revision、Compose 合并结果、三份 D1 Secret 可读性、D2 allowlist 的 owner/mode/非空状态，以及专用 cache 的 owner/mode、symlink/reparse 防护和与媒体映射双向不重叠。
3. 保持 `remote_search_enabled=false` 和 `write_enabled=false`，直到另有独立 D2 Canary 窗口授权；任何配置切换和重启都由主线程在回滚点复核后另行批准。
4. 只有运行时 D2 门禁完整通过后，才可在固定单源 Movie/Episode allowlist 内开展有界真实 Canary；该 Canary 仍不能证明真实多源支持。

### 本轮未做的外部变化

- 未部署、未重建、未重启 C92。
- 未连接或修改 SH/上海服务器、FRP、OpenResty 或公网入口。
- 未创建/修改 C92 文件、Compose、配置、Secret、镜像标签或容器。
- 未调用 Search、Fetch、Preview、Provider、Save、Refresh、Add、Replace、Delete、Upload 或任何媒体写入接口。

## Knowledge Review

任务或阶段：D2-C C92 可回滚部署前核对

验证范围：`AGENTS.md`、`LOCAL_OPERATIONS.md`、D1.5 部署预检、D2 搜索预览契约、D2-C Canary 预检、当前架构、ADR-005、维护经验、Compose 基础/host-network/D2 overlay 示例、Dockerfile、本地 Git 状态、C92 Docker/Compose/容器/镜像/健康和 Secret 元数据。

### Knowledge Findings

- 新增约束：无。现有 D2 契约已规定 overlay 显式合并、非空 allowlist、稳定专用 cache、双向媒体路径隔离和默认关闭。
- 隐蔽坑：C92 的 Secret 文件不在 shared 目录而在独立 Secret 目录；应以 Docker Compose 实际解析结果和容器内应用用户可读性为准，不能从模板相对路径猜测当前布局。
- 被证明错误的假设：不能把当前 D1.5 镜像的 revision、健康探针或本地 dirty 工作区当作审核后的 D2 发布引用。
- 建议沉淀项：无。该结论是本次 C92 部署前事实核对，长期拓扑和凭据位置仍由 `LOCAL_OPERATIONS.md` 维护。

### 证据

- 代码：当前 dirty D2 代码存在，未修改代码；HEAD 不包含 D2 核心新增路径。
- 测试：`go test -count=1 ./...`、`go build ./...`、`go vet ./...`、前端语法检查和 `git diff --check` 通过。
- 实际运行、日志或可复现结果：C92 Docker/Compose 只读检查、容器 inspect、Secret 可读性预检、健康接口、D1.5 health flags、静态资源标记、当前/旧镜像 ID 和 Compose 解析/Hash 核对；未执行真实 D2 API 或 Provider。

### 去重检查

- 已搜索的文档和关键词：`D2`、`C92`、`Canary`、`remote_search_enabled`、`write_enabled`、`d2.canary.enabled`、`d2_canary_items`、`cache_dir`、`overlay`、`rollback`、`Compose`、`SH`、`MediaSource`。
- 是否更新已有结论：否；本报告保存本次 C92 实时预检事实，不改写架构、ADR 或本机长期拓扑。

### 分流判断

- `docs/lessons-learned.md`：不需要更新。
- `docs/architecture.md`：不需要更新；D2 仍未在 C92 运行。
- `docs/adr/`：不需要新增或更新 ADR。
- `LOCAL_OPERATIONS.md`：不需要更新；本轮没有新的长期连接、拓扑或恢复步骤。

### 未验证范围与残余风险

- 没有 D2 commit、CI 构建产物或 registry digest，不能进行 D2 部署。
- C92 尚未准备 D2 overlay、allowlist 和专用 cache；cache 与媒体路径的实际不重叠关系尚未有证据。
- 未验证真实 D2 Search、Fetch、Preview、Provider 候选失败隔离、真实客户端预览或 Canary 窗口关闭后的状态。
- 未验证真实多 MediaSource 样本；按 ADR-005，多源搜索/Fetch/Preview 仍必须保持安全拒绝。
