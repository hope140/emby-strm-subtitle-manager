# Core A/B C92 综合部署验收

日期：2026-08-25（Asia/Hong_Kong）
范围：SubBridge Core A/B，C92 app-only 部署；SH、FRP、OpenResty、Emby 重启或配置修改不在授权范围内。

## 结论

本轮真实 C92 综合验收在媒体操作前的 source-bound 前置门禁处阻断，不能标记为通过。精确提交 `947d847bb8ee620fc0362081fdff981069472081` 已构建并短时部署，启动、健康和运行开关均通过；对 C92 Movie/Episode 的有界真实查询没有找到一个 `MediaSource.Path` 位于应用 `/media` 映射下的可写 source。

根据 [ADR-008](adr/008-core-ab-daily-source-bound-recovery.md) 和当前 `ResolveWriteTarget` 契约，不能用可映射的 Item.Path 冒充选中 source 的路径。因而本轮没有执行 Search→Fetch→Preview→Add、Upload、Replace、Delete、Restore、Emby Refresh 或媒体目录写权限变更。

验收结束后已完成 app-only 回滚，C92 恢复为 healthy、closed、`write_enabled=false`、`remote_search_enabled=false` 和 `/media:ro`。本报告记录的是一次阻断后的可复核结果，不是 Core A/B 真实支持声明。

## 发布候选与回滚

- 公开 `main` 和本地 HEAD 均为 `947d847bb8ee620fc0362081fdff981069472081`；只读 `git ls-remote origin refs/heads/main` 返回同一提交。
- 源码归档来自该精确提交，归档 SHA-256 为 `89e3ce6bc84be3d333b22ee7b09930609851b11d6cb3aca9bf1337b38bcf0d8f`，C92 上传后 hash 一致并完成解包。
- C92 候选镜像标签为 `emby-strm-subtitle-manager:core-ab-947d847`，镜像 ID 为 `sha256:fa93e2e8fe5e66a97803e6562ba7a317b97aeec852d06fc05cd91089a1bbc65c`。
- OCI `version` 为 `0.4.0-core-ab.1`，`revision` 为完整 `947d847bb8ee620fc0362081fdff981069472081`，`source` 为 `https://github.com/hope140/subbridge`，构建时间为 `2026-08-25T13:13:16Z`。
- C92 原有 Compose project、目录、FRP proxy、旧 Compose/config、旧镜像和旧回滚材料均保留。新增候选的 closed/daily Compose、closed/daily config、源码归档、启动诊断和 app-only 回滚材料均在 C92 私有 release 材料中；报告不记录其私有路径。

第一次 daily 启动被应用按设计拒绝，原因是复制出的 daily config 没有四个必需的 D3 私有目录字段。只修正了版本化 daily config，补齐目录绑定后重新做 Compose parse、UID 10001 可读性和 Canary false 检查，未修改源码。

## 部署前后安全边界

部署前 C92 应用为原有 D3 closed 镜像，healthy、restart=0、UID `10001:10001`、rootfs 只读、`/media` 只读，两个功能开关均为 false。原有 app-only closed Compose/config 和回滚镜像已在部署前保留。

daily 窗口启动成功后核对到：

- 候选容器 healthy、restart=0，运行 UID 为 `10001:10001`，rootfs 仍只读、host network 保持不变。
- `write_enabled=true`、`remote_search_enabled=true`，D2 和 D3 Canary 均为 false，daily gate 已启用。
- `/media` 临时为 RW；preview、history、quarantine、archive、trash 五个私有挂载均为 RW，宿主目录均为 `0700`、属主 `10001:10001`。
- 非凭据 config 由 UID 10001 实际 `test -r` 通过；closed 和 daily Compose 均解析通过。
- `/livez` 和 `/readyz` 均返回 200。

阻断后核对到：

- 应用已恢复原有 closed 镜像，healthy、restart=0、rootfs 只读，`/media` 为 RW=false。
- 认证健康接口报告 `write_enabled=false`、`remote_search_enabled=false`；公开 `/livez`、`/readyz` 均为 200。
- 本机和既有 HTTPS 入口的 `/readyz` 均返回 200；无认证 `/v1/health` 和 query token 请求均返回 401，正确 Bearer 只读健康接口仍报告两个开关为 false。
- C92 `frpc-sh` 和 `emby-server` 的容器 ID、restart count 与部署前快照一致。
- 本轮未连接 SH，未向 SH、FRP 服务端或 OpenResty 发出命令；公网代理和端口未作为本轮修改对象。

## 真实 source-bound 预检

使用 C92 Emby API 的有界详情查询覆盖 37 页、7,321 个实际 Movie/Episode Item（查询槽位上限为 7,400）。结果为：

- 真实样本的 Item.Path 可以映射到应用 `/media`，这只能证明 Item 目录事实可读。
- 有界样本中没有一个候选满足“选中的 MediaSource.Path 也位于 `/media` 映射下”；实际抽查到的 source path 是远程播放 URL。
- 没有把默认 source、Item.Path、标题或 source 排序当作写入依据。
- 没有对媒体目录执行 chown/chmod；用于记录样本的临时状态文件已移除，媒体目录未进入写入测试。

这不是可以通过扩大权限解决的权限问题。当前实现的 `ResolveWriteTarget` 要求选中 source 自身可安全映射，远程 source 必须拒绝；将 Item.Path 作为 fallback 会破坏多 source 版本隔离和 ADR-008 的恢复语义。因此本轮在真实写操作前停止。

## 综合验收矩阵

| 项目 | 结果 | 说明 |
|---|---|---|
| Movie Search→Fetch→Preview→Add | 未执行 | source-bound 前置门禁阻断 |
| Episode Search→Fetch→Preview→Add | 未执行 | 同上 |
| 多 MediaSource 显式选择每个 source | 未执行 | 没有可写的本地 source 样本 |
| Upload→PreviewArtifact→Add | 未执行 | 没有进入写窗口 |
| Upload→PreviewArtifact→Replace→Restore | 未执行 | 没有进入写窗口 |
| Delete→Restore | 未执行 | 没有进入写窗口 |
| History 默认/显式 limit、source 绑定 | 未执行 | 没有产生本轮 history |
| `operation_id` 幂等重放 | 未执行 | 没有产生本轮 operation |
| 文件 Hash、MediaStreams、直连字幕流、Emby Web 读取 | 未执行 | 没有新增或替换字幕 |
| 日志、响应、DOM 脱敏 | 部分 | 只完成健康/只读预检；未进入候选、上传或写入页面 |

管理 UI 只观察到未登录的登录 DOM，未输入管理员凭据，未执行浏览器写操作。因真实 source-bound 门禁已阻断，不能用 UI 登录或本地浏览器证据替代 C92 写入验收。

## 保留与清理

- C92 私有源码归档、候选镜像、版本化 Compose/config、app-only 回滚点、启动失败诊断和 source-bound 阻断摘要保留。
- preview、history、quarantine、archive、trash 目录保留为可恢复材料；本轮没有产生媒体副本、history、archive、trash 或 quarantine 文件。
- 没有永久删除任何字幕或 recovery 材料，没有递归 chown/chmod，也没有修改 Emby、SH、FRP、OpenResty 或公网端口。
- C92 已恢复 closed/只读并通过健康探针和容器边界复核。

## 后续门禁

重新进入真实 Core A/B 前必须满足其一：提供一个经授权、选中 MediaSource.Path 可安全映射到 `/media` 的 Movie/Episode 测试样本；或先由代码/ADR 评审决定是否改变 source-bound 契约。不得在现场把远程 source 当作本地 source，也不得用 Item.Path fallback 绕过当前拒绝规则。

## Knowledge Review

任务或阶段：Core A/B 精确提交 C92 综合部署尝试与 closed 恢复。

验证范围：公开 main、精确源码归档、OCI 构建标签、版本化 Compose/config、app-only 回滚、C92 容器/挂载/健康/功能开关、C92 Emby 有界 Movie/Episode source 查询、source-bound 前置门禁和恢复后的只读边界。

### Knowledge Findings

- 新增约束　启用 daily write 前，config 必须同时提供 history、quarantine、archive、trash 四个 D3 私有目录；Compose 的 RW 挂载不能替代配置字段校验。
- 隐蔽坑　C92 的 Item.Path 可以映射到 `/media`，不代表选中的 MediaSource.Path 也能映射；STRM/远程播放 source 必须在写入前单独验证。
- 被证明错误的假设　“存在可管理的 Item 就存在可写的 source”不成立；写入样本选择必须把 source path 作为独立门禁。
- 建议沉淀项　把“至少一个本地 source path、精确目录存在、目录权限可恢复”放在任何媒体权限变更和写窗口之前，并在无样本时保持 closed。

### 证据

- 代码　当前提交的 `internal/media`、`internal/d3`、`internal/config` 和 `internal/httpapi`。
- 测试　受控升级环境执行 `.\scripts\verify.ps1 -GoPath .tools\go1.26.7\go\bin\go.exe` 通过；沙箱内同命令因 Windows 临时目录权限门禁失败，未将其误记为代码失败。
- 实际运行、日志或可复现结果　C92 镜像 label、Compose parse、UID config readability、daily 启动日志、健康接口、37 页/7,321 Item 的 source-bound 查询和 closed 恢复探针。

### 去重检查与分流

- 已搜索 `AGENTS.md`、`LOCAL_OPERATIONS.md`、`docs/architecture.md`、`docs/lessons-learned.md`、`docs/adr/`、Core A/B 实现评审和 D3 C92 报告，以及 `MediaSource`、`Item.Path`、`ResolveWriteTarget`、`history`、`archive`、`trash`、`quarantine`、`closed`、`remote_search_enabled`、`write_enabled` 关键词。
- `docs/lessons-learned.md`　新增 daily config 私有目录和 source-path 独立门禁。
- `docs/architecture.md`　更新 C92 综合验收的真实状态和 source-bound 证据边界。
- `docs/adr/`　不更新；ADR-008 的 source-bound 规则仍然有效，没有形成新的架构决策。
- `LOCAL_OPERATIONS.md`　更新本机长期的 C92 当前 closed 状态、候选回滚边界和重新进入门禁；不记录私有路径、凭据或标识。

### 未验证范围与残余风险

真实 C92 的 Movie、Episode、多 source 正向 Search/Fetch/Preview/Add、Upload、Replace、Delete、Restore、History、Hash、MediaStreams、直连字幕流、Emby Web 客户端和写入 DOM 均未通过。本轮没有代码修改；下一次必须先解决可写 source 样本或完成独立契约评审。
