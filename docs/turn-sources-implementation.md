# TURN 计划实现与待验收项

2026-11-12：本批实现覆盖 T1–T6，代码通过本地构建、协议/状态机、浏览器和容器验证；尚未作为正式版本发布。

| 阶段 | 实现 | 自动化证据 |
|---|---|---|
| T1 | 两端来源模型、静态 TURN、精确去重、有界鉴权回退、候选分类预算 | node-only 原样兼容；同地址第一凭据失效、第二凭据有效的真实 UDP TURN 协议应答；为 host/reflexive 各留 32、relay 留 64 个候选 |
| T2 | 并行 STUN、按网络缓存、RTT 提示排序、单来源置顶、选中来源界面 | STUN 事务匹配、UDP 超时、TLS 校验、排序及来源映射测试 |
| T3 | 节点来源 API、能力声明、可关闭内置 TURN | PostgreSQL 集成测试、Go race/vet、三种真实容器启动方式 |
| T4 | 加密来源列表、HTTPS 配对链接、来源设置 | 链接边界、AES/GCM 存储、鉴权拒绝清缓存、账户隔离、7 来源共用 1.5 秒截止时间 |
| T5 | 签名 token、SQLite DO、Cloudflare 适配、撤销队列、管理页 | Miniflare/workerd 真实 SQLite；升级重启、吊销交错、重试、零 DO 无效签名预检；Chromium 管理页 |
| T6 | 单 owner 节点、短信账本/分片、RTC 租约、休眠控制 WebSocket、推送、冷备、恢复隔离 | 冻结契约响应校验；多主机 CAS/赢家、长振铃无提前媒体、失联 unknown 占用、会话刷新、限额结束预算、事件批事务、分页、备份恢复及无推送冷备激活 |

Worker 设计、协议扩展、额度计算及开发命令在 GriefSeed 的 `docs/`。两端 PR 中提供构建和测试结果。

## 仍需设备或真实平台的验收

本机没有 `/dev/kvm`。以下结果未由单元测试或模拟的 Cloudflare 上游替代：

- 固定 WebRTC SDK 的强制中继通话、最终 candidate URL 到来源的实际归属；弱网、双网卡、IPv4/IPv6 下的候选与排序。
- 一个 TURN 不通、高延迟、UDP 不通但 TLS 可用；当前节点关闭 TURN 时切换备用来源；同 Cloudflare 地址两组凭据的媒体通话。
- 振铃超过 30 秒后接听、立即接听、预取中取消；双主机抢接/全局拒接和蜂窝 END 的设备竞态。
- Android Doze、网络切换、进程重启、快速断连、会话刷新；验证控制连接复用与旧节点 HTTP 回退。
- 用部署者的 Cloudflare 账号验证 Deploy 按钮、真实 key 权限、升级、吊销后新 Allocate/已有 allocation 停止时间、一天真实用量。
  当前环境没有 Cloudflare 凭据；本地测试使用假密钥和模拟上游，不产生 TURN 流量，也没有部署生产 Worker。

本地压缩的“24 小时空轮询”验证的是客户端不会反复发 RPC，并不是平台一天用量实测。
