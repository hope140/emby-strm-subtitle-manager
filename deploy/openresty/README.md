# OpenResty 公网入口基线

本目录提供面向所有部署者的安全反代基线，不会自动修改现有 OpenResty、1Panel 或 FRP 配置。

公网入口采用以下边界：

```text
客户端 --HTTPS--> OpenResty
OpenResty loopback --HTTP--> FRPS remote port
FRPS --FRP proxy encryption--> FRPC
FRPC loopback --HTTP--> 应用
```

这套拓扑保证 Bearer 不跨主机明文传输，但 FRP 的 `transport.useEncryption` 不提供基于 CA 的服务端身份验证，不能描述为每一跳均为 TLS。

## 安装顺序

1. 将 [http-safe-log.conf.example](http-safe-log.conf.example) 加载到 nginx 的 `http` 上下文，并确保它在站点配置之前解析。
2. 在已经配置可信 HTTPS 证书的站点 `server` 中使用 [proxy-location.conf.example](proxy-location.conf.example)。
3. nginx 配置测试通过后再 reload，并用 `nginx -T` 确认最终生效配置引用 `emby_strm_subtitle_safe`。
4. 如果当前面板无法持久化自定义 `log_format`，代理 location 应临时使用 `access_log off`，不能退回包含完整请求行的默认格式。

安全日志只记录粗粒度路由名、方法、协议、状态码、响应大小和耗时。不得加入 `$request`、`$request_uri`、`$args`、`$query_string`、`$http_authorization`、`$http_referer`、原始 Emby Item ID 或候选 ID。

## 网络门禁

- 公网只开放 HTTPS 入口；FRPS 的应用 remote port 必须由主机和云防火墙拒绝公网入站。
- OpenResty 所在主机应能通过 loopback 访问 FRPS remote port。
- FRPC 的该代理单独启用 `transport.useEncryption = true`，不因本应用改变共享 FRPC 的全局 TLS 设置。
- `/livez` 与 `/readyz` 可以公开；所有 `/v1/*` 只接受 `Authorization: Bearer`，不接受 query token。

验收必须同时覆盖：证书严格校验、HTTP 跳转 HTTPS、公开探针 200、无或错误 Bearer 401、正确 Bearer 200、应用 remote port 的独立公网探测失败，以及反代日志中不存在认证参数和 Authorization 内容。面板重新生成站点配置后必须重复检查。
