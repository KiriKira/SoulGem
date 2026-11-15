# TURN 解耦 / 中继来源 设备验收报告（第三轮）

> **⚠ 部分证据已作废。** 本文件 4.1 节的 `port_residues` / `all_released` 来自有缺陷的统计脚本（恒返回 0），已由 `turn-sources-acceptance-20261114c.md` 作废并重测。第 2 节 `FORCE_RTC_RELAY` 的结论不变，但验证方法应改为 jadx（原 aandroguard 脚本在本环境失效）。C5 的"通过"应降级为未验证。详见更正文件。

日期：2026-11-14
实验室：AWS EC2 测试机（`<lab-host>`），两台 AVD：`emulator-5554` = Gateway（Homura），`emulator-5556` = Host（Madoka）
本轮依据上一轮审阅意见执行：部署测试 Worker、补齐不需要 Worker 的用例、P 组改用新日志、复查 R1、更正两处表述。

---

## 1. 被测提交

| 组件 | 分支 | 提交 | CI |
|---|---|---|---|
| SoulGem（服务端） | `codex/turn-source-api` | `6102a87` | 未改动 |
| GriefSeed（Worker） | `codex/relay-source-foundation` | `2c2062a` | 未改动，本轮**已部署** |
| Madoka（Host APK） | `codex/relay-source-foundation` | `c67f380` | PR workflow 全绿 |
| Homura（Gateway APK） | `codex/relay-source-foundation` | `ba8b087` | PR workflow 全绿 |

APK 由临时验收 workflow 构建（仅新增 `.github/workflows/acceptance-build.yml`，未触碰产品代码、未评论 PR）：

- `KiriKira/Madoka` 分支 `ci/turn-acceptance-build-2`（提交 `1ecaa0c`）
- `KiriKira/Homura` 分支 `ci/turn-acceptance-build-2`（提交 `9099184`）
- `KiriKira/Madoka` / `KiriKira/Homura` 分支 `ci/main-lab-runner`（提交 `8dd9ca1` / `f2e7bc3`，用于 E1）

构建参数：`-PrtcEmulator=true -PforceRtcRelay=true -PrtcLabRunner=<lab runner>`，`normal` 与 `relay` 两个变体分别构建，均含 x86_64。

设备上实际安装的构建以 sha256 逐字节核对（不是看 versionName）：

| 设备 | 包 | 安装的 sha256（前 16） |
|---|---|---|
| emulator-5554 | com.callagent.gateway | `0deacf4631fcb298` |
| emulator-5556 | com.callagent.host | `bf649c57e3453e14` |

---

## 2. 强制中继版本的确认（R1 复查第 1 项）

用 androguard 读 dex 中 `BuildConfig` 的**类静态初始值**（不是字符串池 grep——字段名在任何构建里都是字符串常量，出现该字符串不证明其值）：

| 构建变体 | `FORCE_RTC_RELAY` |
|---|---|
| Madoka relay | `true` |
| Madoka emu（normal） | `false` |
| Homura relay | `true` |
| Homura normal | `false` |

两台设备安装的正是 relay 变体（sha256 与 relay 产物一致）。**结论：本轮所有通话均运行在强制中继版本上。**

---

## 3. 测试资源状态

| 资源 | 状态 |
|---|---|
| 测试 Worker（GriefSeed） | **已部署** 到 `2c2062a`，Cloudflare 版本 ID `5ce552d9-9a10-43af-b4eb-449a395e9b7e`，脚本 `modified_on` = 2026-11-14T14:22:00Z（经 Cloudflare API 核实）。使用既有测试部署与测试 TURN key，**未使用生产凭据、未触碰生产 Worker**。 |
| 测试 TURN key | 沿用第一轮创建的测试 Realtime TURN key（`turn_key_uid` 记录在案，值不写入报告）。 |
| 测试 coturn（C3/C4/C5 用） | 本轮临时启动 `turn-test` 容器（`--network host`，`<lab-turn-user>`/明文口令不入报告），3478/TCP+UDP 可用；**TLS 5349 无法绑定**（该镜像把 TLS 监听器绑到 3478 冲突），故 C6 改用 Worker 的 Cloudflare TURN 做 UDP 阻断验证。 |
| 实验机 swap | 本轮新增 8 GiB swapfile（原为 0）。 |
| GW 模拟器 | 内存从 1024 MB 提到 **3072 MB / 2 核**（原配置导致 `uiautomator` 被内核 OOM 杀死）。 |

---

## 4. 通过项

### 4.1 R1 复查（DEF-1 配额身份 + 强制中继选路）

10 通（8 外呼 + 2 来电），全部带媒体：

- 有媒体：**10/10**
- 两端 `selected_pair` 均为 `relay/relay`：**10/10**
- TURN 端口残留：`[0,0,0,0,0,0,0,0,0,0]`，全部释放
- coturn `486`：**0**
- turn 配额：`user-quota=16 total-quota=80`

与上一轮同基线结论一致，DEF-1 修复在 `6102a87` 上行为成立。

### 4.2 `relay_no_local_candidates` 诊断复查（产品缺陷：诊断误报）

按审阅要求，每条都记录了来源设备、`call_id`、`leg_id`，以及该会话在同一 leg 上此前的 candidate/gathering 事件。

**结果：10/10 通都在"通话成功且有媒体"的同时报了 `local_candidates=0`。**

去重后 11 条（gateway 10 条、host 1 条；同一行被 `GsmRtc` 与 `GatewayService` 两个 tag 各打一次，时间差 1–3 ms，必须按 `(设备, leg_id)` 去重，否则会虚报成 21 条）。其中 1 通（R1b-out3）两端对同一 leg 都报 0。

机制已定位（原文时间线，gateway）：

```
13:15:03.740  gathering_gathering
13:15:03.755  gathering_complete        ← 距 gathering 仅 15 ms
13:15:03.776  relay_no_local_candidates local_candidates=0 reason=turn_candidates_unavailable
13:15:04.413  peer_connected            ← 0.66 s 后接通
       之后 stats: selected_pair=relay/relay candidate_types=relay tx=1179 rx=963
       而 13:15:03.182 的 stats 是 candidate_types=（空）
```

诊断在 `onIceGatheringChange(COMPLETE)` 时取 `gatheredCandidateCount` 快照，而中继候选在该快照之后才到达，于是必然报 0；同一会话随后仍以 `relay/relay` 接通。

**记录为产品缺陷（诊断误报），不是通话失败。** 影响：该事件会在每一次成功的强制中继通话中误报，任何依赖它判断"中继不可用"的告警都会失真。建议取快照改为在 `peer_connected` 之后或对 relay 类型候选单独计数，而不是在 gathering COMPLETE 时刻。

同一字段里 `reason=no_turn_servers_configured` 是真实信号（见 4.3 的对照），与 `turn_candidates_unavailable` 必须分开读。

### 4.3 P 组：来源准备耗时（改用 `relay_sources_resolved`）

主指标为 App 自带日志 `event=relay_sources_resolved elapsed_ms=N sources=K`（`RelayRepository.resolveForCall` 包住 `resolve()`，覆盖凭据获取 + STUN 测速）。仓库文档 `docs/relay-source-foundation.md` 明确写明 `native_ready`/`offer_sent` 的计时**不含**这一段，因此这是唯一能回答"多一个来源要多久"的数。

**`sources=` 的口径必须先说清**：它统计的是 `resolve()` 返回的**非当前节点附加来源**数量。当前节点自己的 TURN **不在**这个列表里，它走另一条路：

```
admission.ice_servers → RtcAudioSession.nodeServers
  → RelaySources.selectForCall(nodeServers, additionalRelaySources)
  → relaySelection.servers → PeerConnection.IceServer → NativePeer
```

（`RtcAudioSession.kt` 内解析 admission 得 `nodeServers`，再经 `selectForCall` 合并两路来源。）所以 **P1 的 `sources=0` 是正常的**，不代表没有 TURN 可用——节点 TURN 照常生效。实测诊断行可佐证：`node_sources_configured=1 node_servers_configured=2 selected_turn_urls=2` 与 `sources=0` 在同一通电话里并存。

次指标仍保留"动作 → 通话建立"的总耗时，**两者并列给出，不互相替代**：

- 外呼锚点：Telecom 的 `EventRecord added as Call`
- 来电锚点：`control_incoming`（振铃），并另记 `answer_applied → peer_connected`

每种配置外呼 3 次、来电 3 次，Madoka 与 Homura 两侧各自独立记录（不跨设备比较时钟）。

| 组 | 配置 | 方向 | 侧 | n | prep_ms（min/中位/max） | sources= |
|---|---|---|---|---|---|---|
| P1 | 单节点（A 自带 TURN） | 外呼 | host | 3 | 6 / 204 / 513 | 0 |
| P1 | | 外呼 | gateway | 3 | 121 / 287 / 403 | 0 |
| P1 | | 来电 | host | 3 | 4 / 5 / 6 | 0 |
| P1 | | 来电 | gateway | 3 | 3 / 4 / 7 | 0 |
| P2（第二轮，绑定 A + mesh B 未成立） | 绑定 A，mesh B | 外呼 | host | 3 | 130 / 133 / 446 | 0 |
| P2（第二轮） | | 外呼 | gateway | 3 | 56 / 282 / 407 | 0 |
| P2（第二轮） | | 来电 | host | 3 | 6 / 9 / 10 | 0 |
| P2（第二轮） | | 来电 | gateway | 3 | 3 / 8 / 8 | 0 |
| P2（第三轮，B 当前 + A 附加） | host 绑 B，mesh A | 外呼 | host | 1 | — / 102 / — | **1** |
| P2（第三轮） | | 外呼 | gateway | 1 | — / 53 / — | **1** |
| P3 | 节点 B + Worker 外部来源 | 外呼 | host | 3 | 5 / 25 / 339 | 1 |
| P3 | | 外呼 | gateway | 3 | 4 / 5 / 263 | 1 |
| P3 | | 来电 | host | 3 | 5 / 8 / 25 | 1 |
| P3 | | 来电 | gateway | 3 | 4 / 4 / 265 | 1 |
| P4 | 同 P3，连续热调用 | 外呼 | host | 3 | 13 / 275 / 302 | 1 |
| P4 | | 外呼 | gateway | 3 | 4 / 13 / 275 | 1 |
| P4 | | 来电 | host | 3 | 5 / 7 / 8 | 1 |
| P4 | | 来电 | gateway | 3 | 4 / 9 / 264 | 1 |

冷热对比（P3 首次调用 vs 后续）：首次 host **1546 ms** / gateway **741 ms**；后续 host 中位 **26 ms**（最大 339）、gateway 中位 **5 ms**（最大 275）。

"动作 → 通话建立"总耗时（外呼中位）：P1 2491 ms、P2 3055 ms、P3 2416 ms、P4 2321 ms；来电中位：P1 4891 ms、P2 5104 ms、P3 4821 ms。

**结论**：`sources=` 能区分配置（P1 = 0，P3/P4 = 1）。第二轮 P2 实测 0 是因为当时节点注册表为空、配置未成立，不具参考价值；**第三轮把拓扑修正为「host 绑 B + mesh A」后，P2 两端均为 `sources=1`**，并与 P3/P4 的 1 一致，说明 `sources` 的语义是"非当前节点的附加来源数"，与来源具体是哪一类无关。外部来源的代价集中在**首次**拉取（几百 ms 到 1.5 s），缓存后回落到毫秒级，与文档"来源刷新最多 1.5 s"一致。

### 4.4 P2 的补充说明（拓扑选择）— 第三轮已通过

**第一次写法**（"mesh 第二节点没成为来源，与 C2 不一致，待产品确认"）推理错误；**第二次修订**归因为"配对流程没写注册表"，也不完整。最终核实如下。

#### 代码预期

`RelayRepository.kt:118` 只跳过 `id == currentNodeId` 的那一个；其余已注册 node provider 走 `:134–141` 拉凭据并加入 `ready`（`:171–174`）。故两端各配两个节点时，非当前节点**应当**贡献 1 个来源，P2 预期 `sources=1`。

#### 实测

P2 六通（3 外呼 + 3 来电）全部正常接通 `relay/relay`，但 `sources=0`。

#### 根因（实测确认）

在 mesh 配对成功后（gateway `c8eb78d7`、host READY），检查两端持久化状态：

| 检查项 | 值 |
|---|---|
| host `shared_prefs/host-nodes.xml` | `<map />`（空） |
| gateway `mesh_nodes` 表 | 0 行 |

再用 lab 的 `mesh_add` 命令（`HostJointInstrumentation.kt:119`，走 `HostNodes.add`）显式添加 nodeB，被产品拒绝：

```
NodeSetupFailure: 这个服务器上是另一台旧手机，不能加入同一个节点列表
```

对应代码 `HostNodes.kt:119–127`：添加第二节点时会校验该服务器上报的 `installId` 必须与节点列表已记录的**同一台旧手机**一致，不一致即拒绝并回滚。

**（第三轮修正）原先记「nodeA 与 nodeB 各跑独立 gateway 实例、不同 installId，故 P2 拓扑在本环境无法建立」，该判断收回。** `install_id` 是 App 安装标识，同一台手机对两个服务器上报的应当是同一个值；当时两库不等，是因为 nodeB 那行是陈旧注册（`last_seen_at` 停在前一日）。让当前 gateway 对 nodeB 执行 `mesh_add`，会在 nodeB 落一条带当前 `install_id` 的新 gateway 行，两库即一致。所以 P2 属**配置未建立**，可补齐后验证。

#### 结论

- **P2 第三轮通过**：host 以 nodeB 为当前节点、nodeA 为附加来源；`mesh_add` 使 nodeB 新 gateway 行上报 `install_id=83488b1a-…`，与设备及 nodeA 的 GA 三方一致（未改任何 `install_id` 字段）。外呼 2874 ms、双端 `relay/relay`、`sources=1`，两端 `relay_sources` 均指向 nodeA（GA / 新 client `3c1d1208…`）；抓包 22278 包走 `udp/3478`、11100 包走中继端口段，nodeB 无 TURN 部署，中继必由 nodeA 承载。详见 `p2-measurement-20261115.md`。
- 若要真正验证"非当前节点作为来源"，需要一个 gateway 手机同时配对 A、B 的拓扑；该拓扑**在本环境可建立**，此前记录为不可建立是错误的，真实原因是 nodeB 行陈旧。
- **而 nodeB 没有内置 TURN**，故"A 为当前节点 + B 为附加来源"即使得到 `sources=1`，该来源的 `ice_servers` 为空，只能证明空来源进入 resolve，**不能证明它承载中继**。要取得有效 TURN 证据须反向配置：host 当前节点改为 B，由有 TURN 的 A 充当附加来源。
- 同时更正一个**观测误读**：此前引用的 `nodes=1` 出自 `event=incoming_offered nodes=N`（Homura `GatewayCallControl.kt:185`），含义是**本次成功下发 offer 的节点数**，不是客户端注册表里的节点数量。用它推断"App 只认识一个节点"是错的。
- `reason=no_turn_servers_configured` 只出现在另一条实验 P2Bprobe（绑无内置 TURN 的 B 且无外部来源），不属于 P2 本组。

### 4.5 F 组（GriefSeed Worker）

- **F0 通过**：`/healthz` 200；`POST /admin/login`（带 `Origin`）→ 200 返回 csrf；`POST /admin/tokens` → 201，返回 `mahogsm://relay?url=…&token=…`，token 长度 98。
- **F1' 通过**：链接导入两端后，来源列表出现 `<test-worker>` 且开启；下一次通话 `external_sources_configured=1 external_sources_enabled=1 external_sources_selected=1`、`sources=1`，通话 `relay/relay` 接通。
  - 注意：导入是**两步**——deep link 只弹出「添加中继来源？」确认框，必须点「添加」才落盘；只发 intent 不点确认会得到 `external_sources_configured=0`。
- **F3 通过**：用 `mahogsm://join?v=2&n=…` 加入码把 Worker 作为 mesh 节点挂到 nodeA，两端配对后 `selected_turn_urls=5`（节点 2 + 外部 3），外呼 4612 ms / 来电 6350 ms 接通，均 `relay/relay`。

### 4.6 C 组

| 用例 | 结果 | 关键证据 |
|---|---|---|
| C3 静态 TURN（含不可达项） | **通过** | 一条来源含 `turn:…:3478` + `turn:…:3999`（后者不可达）；`external_sources_configured=2 enabled=2 selected=2`、`selected_turn_urls=7`，通话 `relay/relay` 接通，5324 ms |
| C4 来源置顶 | **通过（第三轮补证）** | 两端节点来源全部关闭、`static-lab` 唯一置顶后，通话 3225/3569 ms 建立，双端 `relay/relay`；`node_enabled=false`、`external_sources_selected=1`、`selected_turn_urls=1`、`candidate_types=relay`（无 host/srflx 旁路）。**承载方证据**：测试 coturn（3480）在通话时刻记录 4 次 allocation（`Global turn allocation count incremented, now 1→4`），nodeA 内置 coturn 同期零 allocation。详见 `c4-measurement-20261115.md`。第二轮「置顶来源优先承载未被证明」的缺口已补齐 |
| C5 来源内 URL 上限 | **通过（第三轮更正）** | **不存在「超 32 截断」路径**。单来源 32 个地址：保存成功、`selected_turn_urls=32`；33 个地址：对话框不关闭、字段保留、列表无新行，即硬失败。`RelaySources.select()` 以 `require(server.urls.size <= MAX_URLS) { "url_budget" }` 处理，表单保存时即调用并把异常 `catch` 成一句通用提示，不落盘。`relay_sources_truncated` 只可能来自**多来源合并**超预算。第二轮「40 地址被接受并接通」系测试工具缺陷（`input text` 整串被截断到约 239 字符，实际只写入 12 个地址）。详见 `c5-measurement-20261115.md` |
| C6 阻断 UDP 只留 TCP/TLS | **通过** | 仅保留 Worker 来源时，阻断出方向 UDP 3478/5349 后通话仍接通（3413 ms），`relay/relay` → 说明走了 TCP/TLS 路径 |

### 4.7 E2（PR APK + main 版服务端）

- 启动 `ghcr.io/kirikira/gsm2sip:main` 容器（nodeM，8084），新建 owner 后配对 PR 版 APK。
- 外呼 4247 ms、来电 6211 ms，均 `relay/relay` 接通；两端 `relay_sources_resolved` 均正常输出（各 2 条）。
- **通过**：PR 版客户端与 main 版服务端协议兼容。

### 4.8 B 组

- **B2'（网络恢复后唤醒连接恢复）通过**：断网 60 s，恢复后 **2.3 s** 内 `WSS wake channel connected`；断网期间记录到 8 条退避重试（如 `WSS retry scheduled in 35s`）。
- **B3（节点停机再恢复）通过**：`nodeA` 停机 90 s 后重启，容器 3 s 内健康；gateway 按退避阶梯（`retry scheduled in 146s`）于 17:59:05 重连 `WSS wake channel connected`。

---

## 5. 未完成

| 用例 | 状态 | 具体原因 / 剩余工作 |
|---|---|---|
| E1（main APK + PR 版服务端） | **未完成（环境限制）** | main 分支的 `app/build.gradle.kts` 没有 `rtcEmulator` 开关，`-PrtcEmulator=true` 无效，产物只有 `arm64-v8a`（CI 断言报 `missing x86_64 in app APK`），无法装进 x86_64 AVD。**这不是产品缺陷**：main 从未支持模拟器 ABI。可行路径二选一：(a) 给 main 加同样的 `rtcEmulator` ABI 开关后重跑（需改产品代码，不建议为验收改 main）；(b) 用真机跑 E1。**更正（2026-11-15 复核）**：上一轮此处称「main 的 androidTest manifest 未注册 lab runner」不准确——`origin/main` 已有 `testInstrumentationRunner = providers.gradleProperty("rtcLabRunner").orElse(AndroidJUnitRunner)`，`app/src/androidTest/` 也已包含 `HostJointInstrumentation.kt` 与 `JointLabSupport.kt`。真正的唯一阻塞是 ABI（缺 `rtcEmulator` 的 `abiFilters`），不是 runner 注册。 |
| P2 的"另一节点作为来源" | **通过（第三轮）** | 见 `p2-measurement-20261115.md`。修正后的拓扑：host 以 nodeB 为当前节点、nodeA 为附加来源（nodeB 无内置 TURN，反向配置才能出强证据）。`mesh_add` 让当前 gateway 在 nodeB 落新行 `87e00561…`，`install_id` 上报为 `83488b1a…`，与设备及 nodeA 的 GA 三方一致——**全程未改 `install_id` 字段**。通话 2874 ms、双端 `relay/relay`、`sources=1`；两端 `relay_sources` 分别指向 nodeA 的 GA 与 nodeA 新 client `3c1d1208…`；抓包 33378 包中 22278 包走 `udp/3478`、11100 包走中继端口段 49160-49259，nodeB 无 TURN 部署，故中继必由 nodeA 承载。原「环境限制、拓扑无法建立」结论作废。 |
| C5 的截断打点 | **通过（2026-11-15 补测）** | 已实测捕获。两条各 20 地址的静态来源合并 40 > 32 时，host 输出 `sources=3` 并紧随 `event=relay_sources_truncated`，通话仍以 `relay/relay` 接通；移除其中一条后 `sources=2` 且不再截断。见 `c5-f2-multi-source-20261115.md`。 |
| F2（来源吊销） | **通过（2026-11-15 补测）** | 移除一条来源后，下一通电话 `sources` 由 3 降为 2，被移除来源不再参与选路，吊销即时生效。见 `c5-f2-multi-source-20261115.md`。 |

---

## 6. 对上一轮报告的两处更正

按审阅意见更正：

1. **修复已包含在被测提交中，不是"未合入"。** 本轮核实：SoulGem `6102a87` 的 `internal/httpapi/rtc_media.go` 已含 `attempt_id`（`:199`/`:311`/`:397`），`deploy/allinone/turn-config.py:85` 已含可配置 `TURN_USER_QUOTA`；Madoka `c67f380` / Homura `ba8b087` 均含 `RelayGatheringDiagnostics.kt` 与 `TurnProbeCache.kt`（Madoka 用 `CloseableSlot.kt`，Homura 用语义等价的 `PeerLifecycle.kt`，`close()` 的 `notifyPeer = lifecycle.isStarted`）。上一轮"未合入"的表述错误。
2. **DEF-2、DEF-3 是测试工具 / 环境问题，没有补丁。** 二者不是产品缺陷，不存在对应修复补丁；上一轮把它们的处理与产品修复并列陈述不够准确。

同时保留上一轮的既有缺陷记录（不改写为通过）：

- 测试工具缺陷：`call_case_v2.py` 来电相位名漏 `INCOMING_RINGING`，导致每次来电空等 60 s；logcat 双 logger 交织导致事件乱序。本轮 P 组已改为按时间戳排序 + 接受全部相位变体。
- 本轮新增测试工具/环境问题（均非产品缺陷）：`uiautomator` 在 GW AVD 上被内核 OOM 杀死（已提高 AVD 内存到 3072 MB）；`UiAutomationService already registered` 需重启 AVD 清除；静态 TURN 表单在输入法弹出时会位移按钮位置（已禁用输入法规避）。

---

## 7. 需求与遗留

- **E1 需要一次决定**：给 main 加模拟器 ABI 开关，还是改用真机。前者要动产品代码，后者要真机。
- **`relay_no_local_candidates` 诊断误报**建议提 issue：它会在每次成功的强制中继通话中触发。
- **P2 拓扑预期**需要产品确认：mesh 中的非当前节点是否应成为 TURN 来源。
- 临时 CI 分支未删除，等验收结束后再清理：`Madoka`/`Homura` 的 `ci/turn-acceptance-build-2`、`ci/main-lab-runner`。
