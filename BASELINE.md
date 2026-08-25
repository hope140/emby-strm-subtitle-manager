# Phase 1-A 基线报告

状态：已完成基线取证和 Phase 1 文档收口；存在环境阻断和未验证项；ADR-002 已接受。

日期：2026-08-24

本报告只覆盖 Phase 1-A 的上游快照、工具链、构建验证和代码耦合检查。没有把 ChineseSubFinder 源码复制到当前仓库，没有编写 MediaContext、Inventory 或 Installer，也没有修改 Emby 服务。

Phase 1 当前结论：项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。

## 1. 快照范围与固定版本

上游仓库：<https://github.com/ChineseSubFinder/ChineseSubFinder.git>

| 项目 | 本次固定值 | 依据或状态 |
| --- | --- | --- |
| 分支 | `master` | 取快照时的远端分支 |
| 上游 commit | `3335a9c95eec8e1664b7ab29368c34ce10f13575` | `git ls-remote` 与快照 `rev-parse HEAD` 一致 |
| 上一个 commit | `4d55136b74907ffd636db599fe0ec5e3f0a25806` | 固定 commit 的父提交 |
| Go | `1.17.13 windows/amd64` | `go.mod` 与上游 CI 使用 Go 1.17；本次使用同主版本的可复现补丁版本 |
| Node.js | `v16.20.2 windows/x64` | `frontend/Dockerfile` 使用 Node 16，上游前端 engines 要求 Node >=16 |
| npm | `8.19.4` | 随 Node 16.20.2 提供；使用 `frontend/package-lock.json` lockfileVersion 2 |
| 包管理器 | npm，`npm ci` | 上游 Dockerfile 明确执行 `npm ci`；没有改用 pnpm 或 yarn |
| Docker CLI/Engine | 未取得 | 本机没有 `docker`、`docker-compose`、Podman、nerdctl 或 buildah；未安装 Docker Desktop、WSL 或 Hyper-V |

快照位于当前仓库之外的隔离临时目录，构建缓存和前端依赖也位于仓库之外。快照初始及验证后的 Git 状态均为 `master...origin/master`，没有把构建产物带回当前仓库。

本次下载工具包的 SHA-256：

```text
go1.17.13.windows-amd64.zip
6CEA8E199C8034995F3A691EF4564E0CC6645EE1649D7EF268A836387F1A5DFA

node-v16.20.2-win-x64.zip
F8BB35F6C08DC7BF14AC753509C06ED1A7EBF5B390CD3FBDC8F8C1AEDD020EC3
```

上游根目录包含 MIT License，未发现 `NOTICE` 文件。本报告没有复制许可证文本；若后续复制代码或文件，需要保留原许可证声明。

## 2. 可复现验证命令

以下命令中的 `<CSF_REFERENCE>`、`<GO>` 和 `<CACHE>` 代表仓库外的隔离路径，不代表当前项目目录：

```powershell
Set-Location <CSF_REFERENCE>\ChineseSubFinder

& <GO> test -mod=readonly ./internal/pkg/gss/
& <GO> test -mod=readonly ./pkg/gss/
& <GO> build -mod=readonly -ldflags='-s -w' `
  -o <CACHE>\artifacts\chinesesubfinder.exe `
  ./cmd/chinesesubfinder

Set-Location frontend
npm ci --cache <CACHE>\npm
npm run build

Set-Location ..
docker build --file frontend/Dockerfile --tag csf-phase1-frontend:<COMMIT> frontend
& <GO> run -mod=readonly ./cmd/chinesesubfinder -litemode=true
```

后端正式构建和最小启动使用 `CGO_ENABLED=1`。Go module、Go build cache、npm cache、产物和日志均定向到隔离目录。

## 3. 验证结果

| 顺序 | 验证 | 结果 | 证据与边界 |
| --- | --- | --- | --- |
| 1 | 上游 CI 中的后端测试 `go test -mod=readonly ./internal/pkg/gss/` | 未通过 | 快照没有 `internal/pkg/gss`，Go 报 `directory not found`；这是上游 CI 路径漂移，不是测试断言失败 |
| 2 | 修正到实际目录 `go test -mod=readonly ./pkg/gss/` | 通过 | `ok github.com/ChineseSubFinder/ChineseSubFinder/pkg/gss 0.305s` |
| 3 | 后端构建，前端构建前 | 未通过 | Go embed 找不到 `frontend/dist/spa/css`；说明 Go 正式构建依赖先生成前端 dist |
| 4 | `npm ci` | 通过 | 安装 1275 个包，审计 1276 个包；npm 报 72 个漏洞，其中 low 9、moderate 17、high 38、critical 8。本次没有执行 `npm audit fix`，没有升级依赖 |
| 5 | `npm run build` | 通过 | Quasar 2.11.5、`@quasar/app` 3.3.3、Webpack 5 编译成功，约 68.9 秒，生成 `frontend/dist/spa`；仅有 browserslist/caniuse-lite 过期提示 |
| 6 | 后端构建，前端 dist 已生成 | 未通过 | `github.com/mattn/go-sqlite3` 和 `github.com/baabaaox/go-webrtcvad` 都报告 `C compiler "gcc" not found`；源代码没有完成正式可执行文件编译 |
| 7 | `CGO_ENABLED=0` 诊断性构建 | 未通过 | `go-webrtcvad` 的 build constraints 排除了全部 Go 文件；不能作为正式构建替代方案 |
| 8 | Docker 版本与 Docker 构建 | 未执行 | `docker` 命令不存在，无法读取 Engine 版本、构建镜像或判断镜像能否启动；本次没有安装系统级容器运行时 |
| 9 | 最小启动 `go run ... -litemode=true` | 未通过 | 在编译阶段再次被缺少 GCC 阻断；没有监听默认端口 19035，没有执行 HTTP 或 Emby 请求 |
| 10 | ASS/SRT/Hub Parser 测试 | 未形成通过结论 | 测试引用快照外的 `ChineseSubFinder-TestData`，共在打开 fixture 时失败；日志初始化还报告创建 symlink 需要额外 Windows 权限。不能据此判定 Parser 逻辑正确或错误 |

### 3.1 后端验证的含义

`pkg/gss` 的单包测试已通过，但这不等于整个后端测试套件通过。正式后端构建和最小启动目前是环境阻断，缺少 GCC/MinGW 是可行动的下一步条件。上游 README 也明确要求 Windows 构建具备 CGO 和 MinGW 条件。

前端构建已经证明锁定的 Node/npm 与当前快照可以完成生产资源生成。首次在受限执行环境中运行 Node 16 时出现对用户目录 `lstat` 的 `EPERM`，在同一隔离快照中用受控权限重跑后通过；这属于执行环境权限差异，不是前端编译错误。

Docker 基线仍不完整。除本机没有 Docker 外，快照中的 `frontend/Dockerfile` 使用可变的 `library/node:16-alpine` 和 `nginx:alpine` 标签，完整镜像还引用 `allanpk716/chinesesubfinder-base:latest`。即使取得 Docker Engine，后续也应把基础镜像固定到 digest 后再形成可发布基线。

## 4. 当前仓库与上游的边界

当前仓库继续作为 SubBridge（SB，字幕桥）的规划和证据仓库。ChineseSubFinder 只作为仓库外参考快照使用。Phase 1-A 没有引入其源码、依赖、Docker 文件或运行产物，也没有对 Emby 做部署、重启、配置写入或客户端验收。

Emby 真实 API、真实客户端和远端字幕读取均未在本阶段新增验证。已有 Gate 0 证据仍是当前项目的事实起点；本报告只补充上游代码取证和本地构建结果。

## 5. Knowledge Review

本阶段新增且有源码或命令证据支持的知识如下：

1. 上游 CI 的 `internal/pkg/gss` 路径已经与快照目录结构不一致，实际可运行包为 `pkg/gss`。
2. Go 正式构建依赖先生成 `frontend/dist/spa`，并且依赖 CGO 编译器；仅切换 `CGO_ENABLED=0` 不能绕过 `go-webrtcvad`。
3. Parser 单测夹具位于快照之外，当前快照无法独立复现这部分测试；后续若要把 Parser 纳入复用范围，应先补齐受控、可分发的最小 fixture。
4. Library、旧扫描器、Cloud 和 Provider Hub 之间存在启动期和运行期的交叉依赖，详见 [CSF_REUSE_MATRIX.md](CSF_REUSE_MATRIX.md)。

这些结果足以支持路线判断，但不把环境阻断伪装成全仓通过。本次已将稳定的路线事实分流到 [docs/architecture.md](docs/architecture.md) 和已接受的 [ADR-002](docs/adr/002-project-codebase-route.md)；没有向 `docs/lessons-learned.md` 追加一次性基线流水账。

## 6. 结论

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。

上游快照和前端构建链可复现，但后端正式构建、Docker 构建和最小启动尚未形成全绿基线。代码耦合检查见 [CSF_REUSE_MATRIX.md](CSF_REUSE_MATRIX.md)，路线决策见已接受的 [ADR-002](docs/adr/002-project-codebase-route.md)。Phase 1 文档收口完成；项目保持在 Phase 2 入口之前，不进入 MediaContext、Inventory、Installer 或 Emby 服务修改。
