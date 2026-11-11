# TURN 与节点解耦、Cloudflare 支持与 Worker 版节点：目标与计划

更新：2026-11-11。状态：**目标已定，尚未实施**。本文是三端（SoulGem、Madoka、Homura）以及 Worker 版新仓库
[GriefSeed](https://github.com/KiriKira/GriefSeed) 共同遵守的目标文档；协议细节在对应里程碑实施时写进各自的协议文档。

## 背景

现在每台 SoulGem 节点都把 TURN 和自己绑在一起：通话时，手机只能拿到**当前节点**在媒体准入
（`GET/POST /v1/calls/{call_id}/media`）里下发的 `ice_servers`，凭据由节点用 coturn 共享密钥算出
（`internal/httpapi/rtc_media.go` 的 `writeMediaAdmission`）。由此带来三个问题：

1. 当前节点的 TURN 不通，或者离某部手机很远时，没有别的中继可选，即使其他节点的 TURN 都是好的。
2. 用不了不走共享密钥方案的 TURN。Cloudflare Realtime TURN 的凭据只能调它的 API 申请。
3. 没有服务器的人无法使用这套系统：信令和 TURN 都要求自己有一台公网主机。

## 已核实的前提（2026-11-11）

| 事实 | 依据 |
|---|---|
| libwebrtc 给每个 TURN 服务器一个互不相同的相对优先级，**列表越靠前优先级越高**；选路先比连通状态，再比网络开销和候选优先级，RTT 只在优先级完全相同时才起作用。因此多个 TURN 一起下发时，结果是“排在最前、并且能打通的那个”，不是“最快的那个” | libwebrtc `p2p/client/basic_port_allocator.cc`（`relative_priority--`）、`p2p/base/basic_ice_controller.cc`（`SortAndSwitchConnection`） |
| 两部手机的 `ice_servers` 解析只认 `urls`、`username`、`credential`，可以有多项；本地候选上限 128 个 | `rtcmedia/.../RtcAudioSession.kt`、`NativePeer.kt:113` |
| Cloudflare TURN：`POST https://rtc.live.cloudflare.com/v1/turn/keys/{key_id}/credentials/generate-ice-servers`，请求体 `{"ttl":秒}`，返回现成的 `iceServers`，可以吊销；端口 53 的地址建议过滤掉 | [Generate credentials](https://developers.cloudflare.com/realtime/turn/generate-credentials/) |
| Cloudflare TURN 计费：每月前 1000 GB 免费（和 SFU 共用），超出每 GB 0.05 美元 | [Realtime pricing](https://developers.cloudflare.com/realtime/pricing/) |
| Workers 免费版每天 10 万次请求；建立 WebSocket 计 1 次请求；Worker 向外发起的 fetch 不计费；付费版每月 5 美元含 1000 万次请求 | [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/) |
| Durable Objects 免费版可用（只能用 SQLite 存储）：每天 10 万次请求、13,000 GB-s、写 10 万行；付费版每月只含 100 万次请求。收到的 WebSocket 消息按 20 条计 1 次请求，**协议层 ping 不计费**，休眠期间不计时长；`setWebSocketAutoResponse` 文档只说不计时长，没说是否计请求数 | [Durable Objects pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) |
| “Deploy to Cloudflare”按钮会把公开仓库**复制**到用户自己的 GitHub/GitLab 账号，再由 Workers Builds 构建部署；支持 Durable Objects、D1、secrets（在 `.dev.vars.example` 里声明）；对 monorepo 支持不完整 | [Deploy buttons](https://developers.cloudflare.com/workers/platform/deploy-buttons/) |
| 有报告称 `workers.dev` 在中国大陆被 SNI 阻断，通常做法是绑自定义域名，但没有官方保证大陆可达 | 社区报告，需要实测 |

## 目标

### G1（首要）：TURN 与节点解耦

TURN 变成手机上独立配置的“**中继来源**”，和信令节点是多对多的关系。节点自带 TURN 仍然是默认、最常见的配法，
但不再是唯一的配法。

- 来源类型：
  - **节点下发**：现状，当前节点准入里的 `ice_servers`，行为不变。
  - **节点作为中继来源**：任意一个已配对的 SoulGem 节点（不必是当前节点）通过新接口提供 TURN 凭据。
    这样平时只用一个主节点，其他节点的 coturn 也能在通话时被用上，**平时不用连它们、不用探测**。
  - **静态 TURN**：手工填写 URL、用户名、密码（长期凭据），适合自建、不带 SoulGem 的 coturn。
  - **Cloudflare TURN**：凭据从 Cloudflare 申请。可以由手机直接用自己的 TURN key 申请，也可以经由 G3 的中转 Worker 申请。
  - **通用中继来源**：任何实现了下面“中继来源协议”的 HTTPS 服务，G3 的 Worker 就是其中之一。
- 两部手机**各自**配置、各自选择。ICE 允许两侧使用不同的中继，所以旧手机可以用离自己近的 TURN，
  主手机用 Cloudflare。
- 已有节点**不需要改配置**。新接口靠升级 SoulGem 版本获得，不改任何环境变量。

### G2：多个中继自动择优

- 在 PeerConnection 建立前把所有来源合并：按 URL 去重、过滤端口 53、每个来源最多保留 UDP 3478、TCP 3478、
  TLS 443 这几类，总数控制在本地候选上限 128 以内并留余量。
- **由手机排序**，因为 libwebrtc 只看列表顺序：向每个来源的 STUN 端口并行发 binding 请求，按 RTT 升序排列；
  不响应的排到最后，但仍保留，ICE 会自己跳过打不通的。RTT 按网络缓存，网络变化后重新测。
  用户可以把某个来源固定在最前。
- 能直连时，ICE 本来就会优先选直连，排序只决定中继之间的先后。
- 日志和通话界面记录最终选中的是哪个来源（例如“中继：Cloudflare”），沿用现有 `GsmRtc` 日志，不记录凭据。

### G3：Cloudflare 一键部署（独立仓库 GriefSeed）

Worker 版拆到独立仓库 [GriefSeed](https://github.com/KiriKira/GriefSeed)，README 放“Deploy to Cloudflare”按钮，
具体范围见它的 [PLAN.md](https://github.com/KiriKira/GriefSeed/blob/main/PLAN.md)。两个阶段放在同一个仓库、同一个 Worker 里，
用户只需要部署一次。分两个阶段：

- **G3a 中继来源 Worker**（小，先做）：实现中继来源协议，Cloudflare TURN key 和 API token 作为 Worker secret 保存，
  按用户 token 发放凭据。这样可以把 Cloudflare TURN 分享给别人，而不用把 API token 交给他们的手机。
- **G3b Worker 版节点**（大，后做）：用 Durable Objects（SQLite）实现手机实际用到的 SoulGem 协议子集，
  让没有服务器的人可以把它当唯一节点，有服务器的人可以把它当冷备节点。

平台只选 Cloudflare，不做 Vercel 版，也不两者都做：凭据来源发的本来就是 Cloudflare TURN 凭据；节点需要的串行状态、
可休眠长连接和精确定时，Durable Objects 都自带，Vercel Hobby 没有自带数据库、WebSocket 单个连接最长 300 秒、
cron 每天最多一次。GriefSeed 的协议逻辑写成和平台无关的模块，以后需要时再加别的平台适配器。

## 中继来源协议 v1（草案）

SoulGem 节点（G1）和中继来源 Worker（G3a）使用同一个线格式，手机只实现一份客户端：

```
GET {base}/v1/rtc/ice-servers
Authorization: Bearer <SoulGem 会话 access token | 来源 token>

200 {"ice_servers":[{"urls":["turn:host:3478?transport=udp", ...],"username":"...","credential":"..."}],
     "expires_at":"2026-11-11T12:00:00Z"}
```

- SoulGem 在公开的 `GET /v1/capabilities` 里加 `"ice_servers_v1": true`（和 `reception_v2`、`global_decline_v1` 同样的写法）；旧节点没有这一项，手机直接跳过。
- 凭据和具体通话无关，手机按 `expires_at` 缓存。剩余有效期不足（暂定 30 分钟）就在下一次通话准备前刷新。
- 通话准备时并行请求所有需要刷新的来源，总预算暂定 1.5 秒；超时或失败的来源本次跳过并记录日志，**绝不阻塞通话**。
  Madoka 来电时在振铃阶段准备媒体，预算算在这段时间里。
- 节点准入里的 `ice_servers` 保持不变，旧客户端不受影响。

## 不做的事

- 不在 Worker 里实现 TURN 中继本身：Workers 不能中转 UDP，中继只用 Cloudflare Realtime TURN 或 coturn。
- 不改 RTC 呼叫控制 v1 和信令流程，不在节点之间转发信令。
- 不要求修改任何现有节点的配置；不改变现有部署的默认行为。
- 不保证中国大陆可达。Cloudflare 相关功能都是可选来源，自建 TURN 始终保留。

## 里程碑

| 阶段 | 内容 | 涉及仓库 | 完成门槛 |
|---|---|---|---|
| T1 来源抽象 | `rtcmedia` 里引入“中继来源”抽象，支持合并、去重、过滤和上限；先接入“节点下发”和“静态 TURN”；记录选中的来源 | Madoka、Homura | 单元测试覆盖合并规则；只配节点下发时行为与现在逐字节一致；KVM 模拟器里用静态 TURN、强制中继完成一次通话 |
| T2 自动择优 | STUN RTT 测量与缓存、排序、手动置顶；通话界面显示选中的来源 | Madoka、Homura | 两个 TURN 中一个故意不通、一个高延迟时，选中符合预期；测量不拖慢来电振铃 |
| T3 节点作为中继来源 | SoulGem 新增 `GET /v1/rtc/ice-servers` 和 `ice_servers_v1` 能力声明；一体镜像新增可选开关关闭内置 coturn（默认仍开启），用于纯信令节点；手机把其他已配对节点作为来源 | SoulGem、Madoka、Homura | 旧客户端、旧节点混合运行正常；主节点 TURN 关闭时通过备用节点的 TURN 完成通话 |
| T4 Cloudflare 来源 | 手机直接用 TURN key 申请凭据（个人自用） | Madoka、Homura | 真机经 Cloudflare TURN、强制中继完成通话；凭据和 token 加密保存，日志不出现凭据 |
| T5 中继来源 Worker（G3a） | GriefSeed：中继来源协议、用户 token 发放和吊销、Deploy 按钮、升级说明 | GriefSeed | 从按钮全新部署到手机可用，不需要命令行；按请求预算实测一天的用量 |
| T6 Worker 版节点（G3b） | 另起设计文档：协议子集、DO 数据模型、冷备模式、短信端到端加密、请求预算 | GriefSeed、Madoka、Homura | 另行制定 |

T1、T2 不依赖服务器改动，可以先做；T3 和 T1 可以并行。T4 可以在 T5 之前单独交付。

## G3b 的已知约束（留给 T6 设计）

- **请求预算**：现有频率是给主节点用的。主手机每 15 秒探测一次节点（`NodeMonitor.kt`），通话中每 250ms 和 750ms 轮询呼叫控制，
  一通 5 分钟的电话约 1,600 次请求。Worker 版节点必须改为：冷备时主手机不探测；旧手机只保持一条可休眠的 WebSocket，
  用协议层 ping 保活；通话控制走 WebSocket；用推送唤醒。目标是冷备时每个用户每天一百次左右。
- **多人共用**：如果一个人部署的 Worker 给多个人用，短信会存在部署者的账号里，需要旧手机和主手机之间的端到端加密，
  Worker 只转发密文。
- **升级**：按钮创建的是复制出来的仓库，不是 fork，不会自动跟随上游更新，需要写清楚升级方法或者提供同步用的 Action。
- **和 Kyubey 的关系**：不合并。Kyubey 里放的是 App 的 Firebase 推送凭据，只能由维护者运营；GriefSeed 由每个用户部署到自己的账号。

## 待决定的问题

1. T4 是否允许把 Cloudflare API token 放在手机上（只限个人自用），还是只走 T5 的 Worker。token 泄露的后果是别人可以消耗你的 TURN 额度。
2. 两部手机的中继来源是各自手动配置，还是在配对时通过二维码或节点下发一份默认配置，用户再修改。
3. 使用 Cloudflare Realtime 是否必须绑定付款方式（文档未说明，需要实际注册确认）。
