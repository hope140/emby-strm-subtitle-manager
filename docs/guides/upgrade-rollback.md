# 升级与回滚

本指南只处理应用镜像和应用配置的升级/回滚，不迁移 Emby、不修改反向代理，也不删除媒体文件。

## 升级前冻结

记录当前运行镜像的完整引用（优先 digest）、应用 Compose 文件、非敏感配置版本和持久化目录清单。保留已经通过验收的旧镜像引用，直到新版本完成下述 closed 验收。

不要把凭据值、Token、Cookie、媒体绝对路径或 Item ID 写入升级记录。只记录它们的文件存在性、权限检查结果和引用位置。

升级前必须确认：

- 新镜像引用是发行清单给出的不可变 digest。
- OCI version、revision、created 和 source 与发行记录一致。
- `docker compose config` 通过，且没有意外合并写入/Canary overlay。
- `/media` 仍为只读，`write_enabled` 与 `remote_search_enabled` 都是 false。
- 当前 Compose、配置和已验收镜像引用可用于回滚。

## 升级步骤

1. 将私有 Compose 中的 `IMAGE_REF` 改为新的完整 digest；不要修改凭据或媒体权限。
2. 先运行 Compose 配置合成检查，再按现有部署方式更新 app 服务。
3. 只核对 app 服务的健康、镜像 OCI 标签、`/livez`、`/readyz`、认证边界和管理员登录。
4. 登录后确认健康摘要仍是 closed，完成一次只读媒体/字幕清单检查。
5. 把验证结论、镜像 digest 和回滚引用记录在发行记录中，但不记录敏感值。

如果任一项失败，停止升级。不要通过临时开放公网端口、关闭 rootfs 只读、放宽媒体权限或开启功能开关来“修复”升级。

## 回滚步骤

1. 先保留失败版本的 Compose、容器日志和持久化目录，不做删除或清理。
2. 将 `IMAGE_REF` 恢复为升级前已验收的完整 digest，并恢复与之兼容的非敏感配置。
3. 仅重建/重启 app 服务，不联动 Emby、反向代理或其他无关服务。
4. 再次确认健康、认证边界、默认 closed 开关、只读 rootfs 与只读媒体挂载。
5. 使用管理员登录完成只读浏览检查。

历史、缓存、quarantine、archive 和 trash 目录是可恢复性证据的一部分。升级和回滚都不得将它们当作临时文件删除。永久清理必须由独立的保留期任务处理。

## 写入边界

升级或回滚本身不授权打开远程搜索或写入。若另行批准受控写入，应按对应的 Item/source、路径、Hash、Refresh、MediaStreams、字幕流和客户端读取门禁执行，并在结束时恢复 closed。现有真实证据的适用范围见[当前状态与后续路线图](../planning/current-status-and-roadmap.md)。
