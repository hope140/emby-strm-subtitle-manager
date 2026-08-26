# ADR-011　默认启用日常字幕管理

- 状态　accepted
- 日期　2026-08-26
- 相关组件　发布 Compose、部署配置、认证 scope、D2、D3、安装文档

## 背景

SubBridge 已具备搜索、预览和可恢复字幕管理能力，但早期 Canary 策略仍让标准发行模板以只读和关闭功能开关启动。这使正式版安装后只能浏览，和产品的日常管理定位不一致。

## 最终选择

标准 bridge、host-network 与 release Compose 均默认挂载日常运行所需的可写媒体与私有目录；标准配置默认启用 `remote_search_enabled`、`write_enabled` 和 `subtitle:write`。管理员会话与 Bearer 仍受认证和 scope 约束。

Canary overlay 只保留给一次性、按 Item allowlist 缩小范围的生产验证。多版本 STRM 写入必须明确选择 source；本地写入仍锚定 Item.Path，Emby 将结果绑定到所选版本。

## 后续影响

部署者必须在首次启动前替换六个宿主挂载路径，并确保容器 UID `10001` 对媒体根和四个恢复目录具备所需权限。服务端继续执行 Item/source 重读、PathMapper、PathGuard、Artifact 绑定、原子版本化写入、Hash、Refresh、quarantine 与 Restore；本决策不允许绕过这些边界。

本 ADR 替代 ADR-003、ADR-008 中关于标准部署默认关闭和只读媒体挂载的部分，不改变其余阶段或数据模型决定。
