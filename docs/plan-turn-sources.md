# TURN 与节点解耦、Cloudflare 支持与 Worker 版节点：目标与计划

更新：2026-11-12。状态：**本批实现已完成，等待设备和真实 Cloudflare 验收**。详见 [实现与验收边界](turn-sources-implementation.md)。本文是三端（SoulGem、Madoka、Homura）以及 Worker 版新仓库
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
| libwebrtc 给每个 TURN 服务器一个互不相同的相对优先级，**列表越靠前优先级越高**；选路先比连通状态，再比网络开销和候选优先级，RTT 只在优先级完全相同时才起作用。列表顺序会影响选择，但不能据此保证任意网络下都选第一个可达来源或最快路径；需用固定 SDK 实测 | libwebrtc `p2p/client/basic_port_allocator.cc`（`relative_priority--`）、`p2p/base/basic_ice_controller.cc`（`SortAndSwitchConnection`） |
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
  - **独立中继来源**：任何实现了下面“中继来源协议”的 HTTPS 服务，不需要是节点。GriefSeed 就是其中之一，
    它发的是 Cloudflare TURN 凭据。
  - **静态 TURN**：手工填写 URL、用户名、密码（长期凭据），适合自建、不带 SoulGem 的 coturn。
- **Cloudflare API token 不放在手机上**（已决定）：Cloudflare TURN 只经由 GriefSeed 获得，token 只保存在 Worker 里。
- 添加方式（已决定）：
  - **添加节点时自动带上它的 TURN**：节点声明 `ice_servers_v1` 时，加节点就同时把它加为中继来源，复用这个节点的会话，
    不需要另外配对；用户可以单独关掉某个节点的中继。GriefSeed 作为节点时，自带的 TURN 就是 Cloudflare。
  - **单独配对一个中继来源**：扫描来源生成的链接或二维码（暂定 `mahogsm://relay?url=...&token=...`，和现有的
    `mahogsm://join` 同一种方式），不用手打。链接里含 token，只在来源的管理页上显示。
- 两部手机**各自**维护自己的来源列表、各自选择。ICE 允许两侧使用不同的中继，所以旧手机可以用离自己近的 TURN，
  主手机用 Cloudflare。
- 已有节点**不需要改配置**。新接口靠升级 SoulGem 版本获得，不改任何环境变量。

### G2：多个中继自动择优

- 在 PeerConnection 建立前合并来源，保留来源 ID、传输方式、凭据及有效期。只有 URL、用户名和凭据均相同的项才可合并，
  并保留其来源映射；有效期取保守值。同 URL 的不同凭据不能直接丢弃，必须定义有界尝试和失败回退，
  验收覆盖同一 Cloudflare 地址的一组凭据失效、另一组仍有效。确认所固定 libwebrtc 版本对重复地址的行为；
  若它不能同时尝试不同凭据，在手机来源层实现回退，不能只把重复 URL 塞进列表就宣称支持。
- 通用来源按传输类型限制数量，保留实际配置的端口，包括自定义 `TURN_LISTEN_PORT` 和 TLS 5349。
  端口 53 过滤及 UDP 3478、TCP 3478、TLS 443 的筛选只属于 Cloudflare 适配器，不能套用到自建节点或静态 TURN。
  仅配置节点下发时保留原有行为。来源数、服务器数及候选数分别设上限；一个 URL 可能产生多个候选，
  必须给多网卡、IPv4/IPv6 和直连候选留余量，不能把 URL 数当成 128 个候选的预算。
- **由手机排序**，列表顺序影响 libwebrtc 优先级。向支持探测的来源发送有超时的 STUN binding 请求，
  RTT 仅作为初始排序提示，不等于 TURN 鉴权成功或端到端媒体质量；UDP 探测失败不能否定 TCP/TLS 路径。
  不响应的来源保留有界回退，RTT 按网络缓存，网络变化后失效；用户可以把某个来源固定在最前。
  通过所固定 SDK 的实测确认排序和选路，不承诺单靠 STUN RTT 就能选到最快的媒体路径。
- 能直连时保留 ICE 的直连优先策略，不为来源排序强制全局 relay-only；强制中继仅用于验收。
- 日志和通话界面记录最终选中的是哪个来源（例如“中继：Cloudflare”），沿用现有 `GsmRtc` 日志，不记录凭据。

### G3：Cloudflare 一键部署（独立仓库 GriefSeed）

[GriefSeed](https://github.com/KiriKira/GriefSeed) 是 **Worker 版的 SoulGem**：同一套手机协议，跑在 Workers + Durable Objects 上，
README 放“Deploy to Cloudflare”按钮。SoulGem 自带 coturn，GriefSeed 自带的 TURN 就是 Cloudflare TURN，相关代码都放在 GriefSeed 里。
具体范围见它的 [PLAN.md](https://github.com/KiriKira/GriefSeed/blob/main/PLAN.md)。同一个仓库、同一个 Worker，用户只需要部署一次。
分两个阶段：

- **G3a 自带的 Cloudflare TURN**（小，先做）：实现中继来源协议，Cloudflare TURN key 和 API token 作为 Worker secret 保存，
  按用户 token 发放凭据；T5 首次交付即采用版本化 HMAC-SHA-256 签名来源 token，手机按不透明 Bearer 字符串使用。
  格式、签名输入和密钥轮换要求见 GriefSeed 计划；不改变 SoulGem 会话 token。T5 起用部署级 SQLite Durable Object 保存 token、签发记录和撤销任务；T6 单 owner 节点复用该对象及 schema。
  吊销 token 后拒绝新签发，并主动撤销其名下未过期的 username，失败任务持久化重试；已有中继的停止时间按实测描述。
- **G3b Worker 版节点**（大，后做）：用 Durable Objects（SQLite）实现手机实际用到的 SoulGem 协议子集，
  让没有服务器的人可以把它当唯一节点，有服务器的人可以把它当冷备节点。

平台只选 Cloudflare，不做 Vercel 版，也不两者都做：凭据来源发的本来就是 Cloudflare TURN 凭据；节点需要的串行状态、
可休眠长连接和持久化定时，Durable Objects 都自带（alarm 不作为精确或恰好一次执行保证），Vercel Hobby 没有自带数据库、WebSocket 单个连接最长 300 秒、
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
- 凭据和具体通话无关；响应使用 `Cache-Control: no-store`，手机在受控本地存储中按来源、身份及 `expires_at` 缓存。
  响应的有效期不得晚于其中任一凭据的有效期；剩余有效期不足（暂定 30 分钟）时触发刷新。
  来源移除、账户切换或鉴权被拒绝时失效缓存；不能跨身份复用。正式契约需明确错误码、刷新并发合并和时钟偏差余量。
- 通话准备时并行刷新来源，整个刷新过程（包括节点会话刷新和重试）共用暂定 1.5 秒预算，不随来源数累加。
  网络失败时可继续使用仍满足安全有效期的缓存；鉴权拒绝不得回退该来源缓存。无可用缓存的来源本次跳过，
  当前节点准入和其他可用来源继续工作；全部不可用且直连失败时明确报告媒体建立失败，不承诺所有情况下通话成功。
- Madoka 来电振铃阶段只预取凭据和测量来源，不调用媒体选择/准入接口、不创建 PeerConnection，也不启动媒体协商计时。
  当前实现接听后才建立媒体，避免 30 秒协商期限先于用户接听耗尽。接听时再次检查缓存有效期，
  取消来电时取消预取工作；验收覆盖振铃超过 30 秒后接听、立即接听及预取中取消。
- 节点准入里的 `ice_servers` 保持不变，旧客户端不受影响。

## 不做的事

- 不在 Worker 里实现 TURN 中继本身：Workers 不能中转 UDP，中继只用 Cloudflare Realtime TURN 或 coturn。
- T1–T5 不改 RTC 呼叫控制 v1 和信令流程，不在节点之间转发信令。T6 的 WebSocket 控制传输另行设计、能力协商和验收，不能隐式替换现有 wake-only WebSocket。
- 不要求修改任何现有节点的配置；不改变现有部署的默认行为。
- 不保证中国大陆可达。Cloudflare 相关功能都是可选来源，自建 TURN 始终保留。

## 里程碑

| 阶段 | 内容 | 涉及仓库 | 完成门槛 |
|---|---|---|---|
| T1 来源抽象 | `rtcmedia` 里引入“中继来源”抽象，支持合并、去重、过滤和上限；先接入“节点下发”和“静态 TURN”；记录选中的来源 | Madoka、Homura | 单元测试覆盖合并规则；只配节点下发时行为与现在逐字节一致；自定义端口、同 URL 不同凭据及候选上限通过；KVM 模拟器里用静态 TURN、强制中继完成一次通话 |
| T2 自动择优 | STUN RTT 测量与缓存、排序、手动置顶；通话界面显示选中的来源 | Madoka、Homura | 两个 TURN 中一个故意不通、一个高延迟时，选中符合预期；测量不拖慢来电振铃；UDP 不通但 TLS 可用、长振铃及刷新超时通过 |
| T3 节点作为中继来源 | SoulGem 新增 `GET /v1/rtc/ice-servers` 和 `ice_servers_v1` 能力声明；一体镜像新增可选开关关闭内置 coturn（默认仍开启），用于纯信令节点；手机加节点时自动把它加为来源，可单独关闭 | SoulGem、Madoka、Homura | 旧客户端、旧节点混合运行正常；主节点 TURN 关闭时通过备用节点的 TURN 完成通话 |
| T4 独立中继来源配对 | 手机支持扫 `mahogsm://relay` 链接添加独立来源，调用中继来源协议；来源 token 加密保存 | Madoka、Homura | 用 T5 的 GriefSeed 作为来源，真机经 Cloudflare TURN、强制中继完成通话；日志不出现凭据和 token |
| T5 GriefSeed 的 Cloudflare TURN（G3a） | GriefSeed：中继来源协议、版本化签名 token、SQLite DO 持久化 token 管理、主动撤销与竞态处理、Deploy 按钮、升级说明 | GriefSeed | 从按钮部署和升级后 token 状态及验签能力可恢复；伪造签名在 Worker 拒绝且不调用 DO，真实签名仍查 DO；吊销后拒绝新签发并撤销名下未过期凭据，失败可恢复重试；记录新 Allocate 拒绝及已有中继的停止时间；不需要命令行；按请求预算实测一天的用量 |
| T6 GriefSeed 节点本体（G3b） | 另起设计文档：协议子集、DO 数据模型、租约/恢复、冷备模式、请求预算；多 owner 前置端到端加密 | GriefSeed、Madoka、Homura | 先冻结设计与预算；实现后通过下述断线、租约和兼容验收 |

T1、T2 不依赖服务器改动，可以先做；T3 和 T1 可以并行。T4 和 T5 一起开发、一起验收。

## G3b 的已知约束（留给 T6 设计）

- **控制传输与租约**：现有呼叫轮询会刷新参与者的业务租约，20 秒未续期会触发失联处理；网关在线及 RTC 能力另有
  90 秒新鲜度检查。WebSocket 协议 ping 只保持连接，不刷新业务状态，不能替代应用层续租。
  T6 必须定义有身份和 call_id 约束的续租、撤销/令牌过期后的重新认证、断线检测、重连快照、幂等重试和 HTTP 回退。
  延续已拨出后进入 unknown 并保留占用、网关确认蜂窝结束后才释放的规则。通过新能力声明协商传输，旧端保持 HTTP 行为。
- **冷备与恢复**：主手机可暂停周期探测，但必须定义主节点失效后的激活触发、无推送时的恢复路径及激活后的同步顺序。
  DO 休眠/重启、alarm 延迟或重复触发不能延长已过期授权；在处理操作时检查持久化 deadline，而非只依赖 alarm。
  多主机抢接、续租中断超过 20 秒、网关信息超过 90 秒、断网重连、重复命令和旧端接入都属于 T6 必测场景。
- **弱网重连：修正 Homura，并验证两端**。当前 Homura `GatewayService.onOpen` 在连接成功时立即重置退避；
  Madoka `HostBackgroundService` 已在连接稳定 60 秒后才重置。Homura 改为稳定窗口达标后再重置，初始按 60 秒与 Madoka 对齐，
  保留指数退避和 jitter；验证两端在快速断连、网络切换、令牌刷新后都不会绕过退避反复握手。
  “全天每分钟重连一次约 1,440 次”仅是每设备、每节点的预算场景，在实施明确限频前不能称为上限；
  每次握手的 Workers/DO 请求以及鉴权、补拉同步、SQLite 写入须单独计入。明确连接阶段限频不会替代活动通话的业务租约。
- **请求预算**：现有主手机周期探测和通话轮询不能直接沿用。冷备 WebSocket、推送及控制传输优化是待验证方案，
  “每用户每天约一百次”只作为无通话、少量事件的冷备目标，不是保证或主节点容量指标。
  分别计算 Workers 请求、DO 请求/消息、唤醒时长、SQLite 读写、alarm、应用层续租、会话刷新、短信同步和重连风暴；
  分开实测冷备空闲、主节点空闲及通话场景。不得为凑预算放宽鉴权、租约或 durable ACK。
- **额度防护边界**：T5 就采用带签名来源 token，Worker 在访问 DO 前检查格式、版本/kid 并验签；
  无效签名不调用 DO，也不写 DO 审计/计数。签名通过后，登记状态、完整 token 哈希与吊销状态仍由 DO 权威核验。
  被窃取、重放或已吊销的真实签名 token 仍可能消耗 DO 请求；不能保证所有垃圾请求都不耗 DO 额度。
  自定义域名/WAF、关闭不需要的直达入口及适用路由的 fail closed 降低风险，Worker 内限流本身不免请求计费。
  T6 普通设备请求限额应返回 `Retry-After`，为挂断及蜂窝结束确认保留受限预算；平台总额度耗尽仍会阻断访问。
  验收 1027、限流与弱网下的客户端退避和状态收敛，不把每日 1,440 次重连场景当作已实现的上限。
- **先只支持一个 owner**（已决定）：一个部署只服务一个 owner，短信只存在自己的账号里。分享给别人时，T5 的 TURN token
  本来就支持多人；节点让每个人自己点一次部署。以后要支持多个 owner 时，必须先做旧手机和主手机之间的短信端到端加密，
  Worker 只转发密文。
- **升级**：按钮创建的是复制出来的仓库，不是 fork，不会自动跟随上游更新，需要写清楚升级方法或者提供同步用的 Action。
- **和 Kyubey 的关系**：不合并。Kyubey 里放的是 App 的 Firebase 推送凭据，只能由维护者运营；GriefSeed 由每个用户部署到自己的账号。

## 待决定的问题

已决定（2026-11-12）：Cloudflare API token 不放手机；加节点时自动带上它的 TURN，也可以单独配对中继来源；
GriefSeed 的来源 token 用 Worker 自带的管理页管理，T5 起采用版本化签名并由 DO 核验登记与吊销；Worker 版节点先只支持一个 owner。

1. 使用 Cloudflare Realtime 是否必须绑定付款方式（文档未说明，需要实际注册确认）。

## 后续事项（暂不排期）

### IPv6（2026-11-14 记录，现有功能完成后再做）

现状（按代码核对）：

- **HTTPS 信令**：API 监听 `:8080`，Linux 上同时接受 IPv4/IPv6；前置的 Docker 端口映射、反向代理和安全组也要放行 IPv6。用 `curl -6 https://<域名>/v1/capabilities` 验证。
- **内置 TURN 只支持 IPv4**：`deploy/allinone/turn-config.py` 写死 `listening-ip=0.0.0.0`、IPv4 `relay-ip`，下发的是 IPv4 字面量地址（不是域名），所以只给域名加 AAAA 不起作用。`deploy/turn` 同样只接受 IPv4。
- **手机直连**：两端 WebRTC 没有禁用 IPv6，双方都有公网 IPv6 时可能已经直连成功，但从未实测；当前日志只有候选类型，看不出地址族。
- **Cloudflare TURN**：下发域名 `turn.cloudflare.com`，大概率支持 IPv6，未核实。

要做的事：

1. 先确认直连：在通话统计里记录选中路径两端的地址族（v4/v6，不记录 IP），用两台真机在移动网络下实测。
2. 自建 TURN 支持 IPv6（一体镜像和 `deploy/turn` 都要改）：
   - coturn 同时监听 IPv4 和 IPv6，各配一个 `relay-ip`；需要能发现或配置服务器的 IPv6 地址。
   - 下发地址：加 IPv6 字面量地址，或改为下发同时有 A/AAAA 的域名。
   - **安全**：`denied-peer-ip` 补上 IPv4 映射地址 `::ffff:0:0/96` 和 NAT64 前缀 `64:ff9b::/96`，否则可绕过 IPv4 内网限制，把 TURN 当作访问服务器内网的跳板。
   - 实测 coturn 在双栈下给 IPv6 客户端分配的中继地址族，以及 IPv6 客户端经 IPv4 中继到对端是否正常。
   - 部署文档：推荐 host 网络；安全组放行 IPv6 的 TURN 监听端口和中继端口段。
3. 验收：当前模拟器环境基本没有 IPv6，需要有 IPv6 的测试机加真机，或者单独搭建双栈的测试网络。
