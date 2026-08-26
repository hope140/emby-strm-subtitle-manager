# ADR-007　SubBridge 品牌与旧部署标识兼容

- 状态　accepted
- 日期　2026-08-25
- 相关组件　GitHub 仓库、Go module、Docker 镜像、Compose、C92 部署与历史报告

## 背景

项目公开品牌已确定为 SubBridge（SB，字幕桥），GitHub 仓库也已在首个正式 V1 前改名。源码和新安装材料应使用一致的新名称，但 C92 已有多轮真实验收、版本化 Compose、镜像、目录、FRP proxy 和回滚材料使用 `emby-strm-subtitle-manager`。仅为改名迁移这些运行资源没有功能收益，还会增加容器并存、路径搬迁和回滚失配风险。

## 决策

1. 公开品牌、仓库 slug、Go module、内部 imports、构建二进制和新安装 Compose project/image 统一使用 `SubBridge`/`subbridge`。
2. 新安装的配置和状态路径使用 `/etc/subbridge` 与 `/var/lib/subbridge`；OCI source 默认指向 `https://github.com/hope140/subbridge`。
3. 已部署 C92 的 Compose project、镜像、容器、宿主目录和 FRP proxy 保留旧标识。只有后续获得包含实际功能收益的部署授权时，才可在核对回滚点后决定继续兼容旧名或执行显式迁移。
4. API 路径、配置字段、管理员 environment、Secret 名称、Item/source 绑定和媒体 sidecar 契约不因品牌改名改变。
5. 历史验收报告保留当时真实使用的仓库链接、镜像、容器和路径名称，不追溯改写为新名。

## 结果与代价

- 新用户看到的仓库、代码模块、镜像示例和页面品牌保持一致。
- 现有 C92 不需要一次无功能收益的重部署，旧回滚材料仍可按原名核对。
- 一段时间内公开材料与历史部署证据会同时出现新旧技术标识；当前架构、README 和本地操作手册负责解释边界。
- Go module 在 V1 前变更，不承诺旧 module path 的库级兼容；应用 HTTP/API 和配置契约保持不变。

## 验证依据

- 改名后的 GitHub HEAD 与本地公开 HEAD 一致。
- `scripts/verify.ps1` 已使用 `github.com/hope140/subbridge` 通过格式、Vet、全包测试和构建。
- Compose invariant 测试已随 `/etc/subbridge`、`/var/lib/subbridge` 和新 project/image 名更新。
- C92 未在本任务连接、重建或迁移，旧运行标识只在历史报告与本地操作手册中保留。
