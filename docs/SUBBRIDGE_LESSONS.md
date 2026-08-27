# 从 SubBridge 保留的经验

这里只保留 SubSteward 仍直接使用的事实；完整旧实现和记录留在 `main` 与稳定基线 `3acdf27047338f81438fa611aed314533e170371`。

## Emby 与 STRM

- `Item`、`MediaSource`、`MediaStream` 是独立事实层；多 Source 不能猜测第一个 source。
- 版本组详情需要完整 source 信息；目标 Plugin API 的取数方式仍须 M0 实测。
- 不读取 `.strm` 内容、远端 URL 或其指向视频。STRM 外挂字幕以 `Item.Path` 为 sidecar 锚点，远程 `MediaSource.Path` 不是可写路径。
- MultiSource STRM V1 可扫描、判断、搜索与预览，但不自动写入。

## Provider 与 Refresh

- Search 候选不等于可 Fetch；上游 4xx、内容或格式错误按候选隔离，临时网络错误最多重试一次。
- 自动流程的候选尝试数有上限，V1 初始值为 3。
- 不依赖 Meiam、Thunder 或 ASSRT 的私有类型；M0 必须验证目标 Emby 的公开 Plugin API。
- 文件存在或 Refresh 成功都不足以证明可用；还要确认 MediaStreams 与字幕流，客户端播放属于更高层验收。
- Emby 的一次公开字幕 Download 可能产生多条新的外置 MediaStream；M1 必须按最终 MediaStream 和文件名核对结果，不能假设一次 Download 对应一条流。
- Provider 的 `Language=zh` 元数据不能证明正文是中文；在安装前必须 Fetch 并做正文语言门禁，至少拒绝没有中文字符的候选。错误候选应移出媒体目录并保留可恢复副本。
- Search 返回顺序也不能当作质量排序：本次 Emby 第一条是 Ghibli 合集且 Fetch 正文含鲁邦，第二条标称中英双语但正文为英语，第三条标题匹配候选才与目标字幕对应。应把候选名称/原名或 Hash 匹配作为人工选择前的明确状态，并对无匹配候选 fail closed。

## 字幕质量与数据边界

- Health 只处理影响观看的明确问题；双语、格式和特效属于 Preference。
- 自动 Repair 仅做确定性编码、BOM、换行、SRT 序号、明确坏标签和非法控制字符处理。
- API Key、Token、Cookie、候选原始 ID、私有路径和带认证参数的 URL 不进入代码、夹具、日志或文档。
- 公开质量 fixture 必须是脱敏、可公开保存的真实样本；M0 不将旧上传预览夹具冒充质量数据。
