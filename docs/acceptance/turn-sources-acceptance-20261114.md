# TURN 与节点解耦 —— 模拟器联合验收报告（第二轮）

- 日期：2026-11-14
- 角色：纯测试与记录。**本轮未修改任何产品代码**，缺陷只记录复现方式。
- 上一轮报告：`turn-sources-acceptance-20261113.md`（旧提交基线）

## 被测版本（本轮实测的 HEAD）

| 仓库 | PR | 分支 | 本轮实测提交 | 说明 |
|---|---|---|---|---|
| SoulGem（服务端） | #36 | `codex/turn-source-api` | `6102a87` | 含 DEF-1 修复：TURN 用户名改为 `设备:通话:尝试` 三段，user/total quota 可配置，默认 16/80 |
| Madoka（主手机） | #44 | `codex/relay-source-foundation` | `e81044d` | 发布竞态修复、PCM 计数修复、无候选诊断、TURN 探测分配释放与缓存 |
| Homura（旧手机网关） | #34 | `codex/relay-source-foundation` | `9f0d59b` | 与 Madoka 共享文件已统一 |
| GriefSeed（Worker） | #2 | `codex/worker-turn-service` | `2c2062a` | 来源链接改为 `mahogsm://relay` |

上一轮结果（SoulGem `d293e57` / Madoka `4d1cb72` / Homura main `c1d0f73` / GriefSeed `f8b9c0a`）
**仅在下文明确标注处引用，不冒充本轮数据**。

## 环境

- 实验机：Linux x86_64，2 vCPU，`<lab-host>`
- 模拟器：`emulator-5554` = gateway（Homura，Android 14 / API 36）；
  `emulator-5556` = host（Madoka，Android 10 / API 29）；均 1080x1920 x86_64
- 节点 A：`8080` / PG `55432` / TURN `3478` / relay 端口段 `49160-49259` / `BUILTIN_TURN_ENABLED=true`
- 节点 B：`8082` / PG `55434`，**无内置 TURN**（已确认无 turnserver 进程）
- 镜像：`ghcr.io/kirikira/gsm2sip:sha-6102a87`（与 `:turnverify-6102a87` 同 digest）
- APK：由 CI（`ci/turn-acceptance-build`）产出，四套变体（Madoka emu/relay、Homura normal/relay），
  均为 x86_64，`sha256` 与 CI 产物一致
- **所有编译均在 CI 或 Hermes 容器内完成，实验机未执行任何 Go/Gradle/npm 构建**

### 编译策略（沿用上一轮结论）

- SoulGem 镜像：`workflow_dispatch` 触发 `images.yml`（`PUSH=true`）推 ghcr，实验机只 `docker pull`
- APK：CI 构建后下载产物
- GriefSeed：在 Hermes 容器内 `npm ci` / `tsc` / `wrangler deploy`
- 实验机唯一允许的二进制操作是 `apksigner` 重签（秒级、不占 CPU）

### 测试容器的 TURN 配置改动（范围已限定）

测试版 `lab-turn-config.py` 基于新提交 `deploy/allinone/turn-config.py` **重新生成**，
只删除 `denied-peer-ip` 的私网条目，其余（含 DEF-1 的 quota 逻辑）保持原样。
旧副本硬编码 `user-quota=4`，若复用会覆盖 DEF-1 修复，故已弃用。该脚本仅挂载到测试容器，
**仓库默认配置未改**。

## 结论汇总

| 用例 | 结果 | 关键数据 |
|---|---|---|
| R1 配额身份 / 486 | **通过** | 10/10 通有媒体，`486`=0，端口残留全 0，`user-quota=16 total-quota=80` |
| R2 快速起止（外呼） | **通过** | 10/10，hold 1.17–2.88 s，全部 `idle_after=True` |
| R2 快速起止（来电） | **通过** | 10/10，`ring_wait` 1.17–2.79 s，进程 PID 未变 |
| R3 无候选诊断 | **通过** | 双方向均 `reason=no_turn_servers_configured`，无媒体，`selected_pair=null/null` |
| R4 普通构建双向通话 | **通过** | 双向媒体正常，`pcm_overruns`/`pcm_underruns` 字段有值 |
| P1 单节点来源准备耗时 | **完成** | 外呼 787–1940 ms；来电 2802–3163 ms；`selected→offer_sent` 76–810 ms |
| P2/P3 多来源耗时 | **未执行** | 需 GriefSeed Worker 签发来源链接，本轮按要求未部署 |
| P4 缓存生效 | **未执行** | 依赖 P3 |
| B/C/E/F 组 | **未在本轮重跑** | 上一轮结果见旧报告，基线不同不复用 |

一句话结论：**R 组（配额身份、快速起止、无候选诊断、普通构建）在本轮新基线上全部通过，
DEF-1 的修复在真机行为上成立**。P 组只完成了单节点基线 P1，多来源对比因不部署 Worker 而未执行。

## 本轮发现的缺陷

### DEF-5（测试工具，非产品缺陷）来电相位名不匹配导致每次空等 60 秒

这是**测试工具**的缺陷，不是产品问题，但影响过测量结论，必须记录。

- 现象：`call_case_v2.py` 的来电分支等待相位 `("RINGING", "INCOMING", "ANSWERING", "ACTIVE")`，
  而产品实际上报的振铃相位是 **`INCOMING_RINGING`**。条件永不成立，于是每轮都跑满 60 s 超时后才接听。
- 后果：一度把 `control_incoming → selected` 的 60.3 s 记录成"来源准备耗时"，属**方法学错误**。
  修正相位列表后，同一用例的接听等待降到秒级。
- 复现：`call_case_v2.py:127`，对一通来电打印 host 相位即可看到 `INCOMING_RINGING` 不在列表内。
- 处理：仅修测试脚本（`cp` 备份后 `sed` 一行），**未触碰产品代码**。
- 关联：同一相位名问题上一轮已在 R2 的来电段出现过一次（当时修的是 `r2_rapid.py`），
  这次暴露在共享 harness 里。

### DEF-6（测试工具）logcat 双 logger 交织导致事件乱序

- 现象：同一 `call_id` 的事件在 logcat 缓冲区里出现顺序 ≠ 实际发生顺序
  （`selected` 有时排在 `control_early` 之后）。
- 后果：未排序时把 `selected` 自身当作起点，算出 `0 ms`。
- 处理：按行首时间戳排序后再取首事件。**产品代码无问题**，App 自身的时间戳是准的。

## 方法学修正：P 组该测什么、不该测什么

这一节是上一轮 P 组未收敛的直接原因，也是本轮最实质的产出。

从 `rtcmedia/RtcAudioSession.kt` 读到的结构：

```kotlin
fun prepare() {
    preparedAt = System.nanoTime()
    event("selected")                    // 来源集合在此刻已确定
    if (relaySelection.truncated) event("relay_sources_truncated")
    relaySelection.fallbackReason?.let { event("relay_sources_fallback reason=$it") }
    worker.execute {
        val native = NativePeer(context, callId, servers, pcm, ::event, onDigit,
                                 forceRelay, relaySelection::sourcesForUrl,
                                 relaySelection.hasAdditionalSources)
        event("native_ready elapsed_ms=" + now - preparedAt)
        pendingOffer = native.offer()
        event("offer_created elapsed_ms=" + ...)
        event("offer_sent prepared_ms=" + ...)
    }
}
```

而会话本身是**以来源解析结果为构造参数**建立的：

```kotlin
RtcAudioSession(..., additionalRelaySources = RelayRepository.resolve(...), ...)
```

由此得到两条硬结论：

1. **`native_ready elapsed_ms` / `offer_sent prepared_ms` 都不能用来衡量来源准备耗时。**
   它们的起点是 `preparedAt`，即 `prepare()` 进入之后，而来源解析在会话对象存在之前就已完成。
   这正是任务说明里"不要用 native_ready elapsed_ms"的技术依据。
2. **`selected` 是一通电话的第一个事件**，不是它的结论。因此从 App 日志内部无法自证
   "拨号动作 → 来源解析完成"这段有多长，必须引入动作侧锚点。

本轮的外呼锚点取 Telecom 建单行（`LogUtils: EventRecord added as Call: [TC@N`），
与 `event=selected` 在同一设备时钟上，只用于**定位**，区间本身仍由两个时间戳相减得到。

被否弃的两种做法（都实测过，都不可用）：

| 做法 | 实测问题 |
|---|---|
| `adb shell date` 在拨号前后夹逼 | `anchor_width_ms=1378`，adb 往返本身比被测区间还大；同一用例读出 2407 ms，而日志显示 181–519 ms |
| 匹配 `setCallState ... -> ACTIVE` | 该构建无 "Creating a new outgoing call" 行，`-> ACTIVE` 会匹配到**上一通**，得出 165 s |

## P1：单节点来源准备耗时（完成）

六通（外呼 3 / 来电 3），全部 `selected_pair = relay/relay`，来源 `current-node`，
即只有当前节点一个来源。证据：`report/evidence/P/pc-P1-*.json`。

| 用例 | 方向 | 动作 → `selected` | `selected` → `offer_sent` |
|---|---|---|---|
| P1-out1 | 外呼 | 1136 ms | 354 ms |
| P1-out2 | 外呼 | 1940 ms | 779 ms |
| P1-out3 | 外呼 | 787 ms | 810 ms |
| P1-in1 | 来电 | 2802 ms | 533 ms |
| P1-in2 | 来电 | 3115 ms | 273 ms |
| P1-in3 | 来电 | 3163 ms | 76 ms |

读法：

- **外呼** `action_to_selected` 含"建单到来源解析完成"，787–1940 ms，抖动主要由模拟器 2 vCPU 争抢带来。
- **来电** 2802–3163 ms，明显高于外呼，且三通非常集中。这与 Madoka 在振铃期做预取的设计一致：
  来电方向在接听前多经过一次振铃期准备。
- `selected → offer_sent`（App 内部建立 peer + 出 offer）76–810 ms，与外呼/来电无稳定相关性。

**未验证的推断**：来电比外呼多出的约 1.5 s 是否全部由"振铃期预取"贡献，未做分离实验（需在同一通话里
同时插桩振铃与接听两侧），故只作描述不作归因。

## P2 / P3 / P4：未执行

- **P2（节点作为来源）与 P3（节点 + 外部来源）**都需要至少两个来源。技术路径已确认：
  服务端**没有** relay-source 的 HTTP 路由，也没有 `mahogsm-admin relay-source` 子命令；
  来源保存在 App 本地 AES 加密的 SharedPreferences `rtc-relay-sources` 里，
  外部来源只能通过 GriefSeed Worker 签发的 `mahogsm://relay` 链接导入。
- 本轮按要求**没有部署 Worker**，因此 P2/P3 无法构造"多来源"前置条件。
- **P4（5 分钟内第二通应明显更快，验证 `TurnProbeCache`）**依赖 P3，一并未执行。
- 现状：P1 统计里 `external_sources_configured=0`、`node_sources_configured=1`、`relay_sources=current-node`，
  与"仅单节点来源"一致。

## R 组详情

### R1 配额身份与 486（通过）

- 10 通（外呼 8 / 来电 2），全部 `media_ok=True`
- `486` 累计 **0**
- 每通结束后 relay 端口段 `[49160,49259]` 残留绑定数 **全为 0**
- 容器内实测 `user-quota=16`、`total-quota=80`
- `selected_pair`：host `srflx/relay`、gateway `relay/srflx`

计量方式说明：不解析 coturn 日志、不读取任何用户名，改为统计容器 netns 内该端口段的已绑定端口数
（活跃分配占端口，通话结束后残留即未释放）。这样既不接触凭据，也能直接反映配额行为。

另需注意：本组每通**成功**通话的日志里都出现
`relay_no_local_candidates reason=turn_candidates_unavailable local_candidates=0
node_sources_configured=1 selected_turn_urls=2`。
**该 reason 不是失败信号**，对端中继仍可接通；它与 R3 断言的 `no_turn_servers_configured` 是两个不同取值，不可混判。

### R2 快速起止（通过）

- 外呼 10 轮：拨出后 1–3 秒挂断，hold 实测 1.17–2.88 s，全部 `idle_after=True`、`phase=ENDED`
- 来电 10 轮：振铃后拒接，`ring_wait` 1.17–2.79 s
- 进程存活：gateway `14625` / host `23801`，前后 PID 不变
- `crashes=[]`，close 之后无 `native_ready`/`offer_created`/`selected` 复活

首轮来电段（45 s 才拒接）因相位名问题方法学无效，已归档为 `R2-run1-invalid-methodology.json`，
不计入通过率；上表以修正后重跑为准。

### R3 无候选诊断（通过）

- 双端换到无内置 TURN 的节点 B，强制中继
- 外呼、来电各一次，均出现 `reason=no_turn_servers_configured`
- 无媒体，`selected_pair=null/null`
- 诊断字段齐全：`force_relay=true`、`local_candidates=0`、
  `external_sources_configured=0`、`node_sources_configured=0`

### R4 普通构建双向通话（通过）

| 方向 | host `native_ready` | gateway `native_ready` | PCM（capture / overruns / underruns） |
|---|---|---|---|
| 外呼 | 984 ms | 693 ms | 4408 / 1171 / 4 |
| 来电 | 19 ms | 28 ms | 4777 / 1235 / 4（另一条 1705 / 483 / 0） |

两侧 `event=stats` 行均含 `pcm_overruns`、`pcm_underruns`，DEF-4 的补丁使这两个计数可读。

## 上一轮缺陷的当前状态

| 缺陷 | 上一轮状态 | 本轮观测 |
|---|---|---|
| DEF-1 TURN 配额把同一通话两端合并计费 | 已定位，已有补丁 | **新基线上行为成立**：R1 10/10 有媒体、`486`=0、端口残留 0、quota 16/80 |
| DEF-2 节点重启后旧网关残留导致 `single()` 崩溃 | 已定位，已有补丁 | 本轮仍多次复现（详见下节），补丁未合入，属预期 |
| DEF-3 重装/force-stop 后 `InCallService` 不重绑 | 已定位，已有补丁 | 本轮未复现（未做 force-stop 场景） |
| DEF-4 `PcmBridge` 丢弃路径计数在并发下方向不确定 | 已更正描述 | R4 中计数有值且非零，与"计数可靠但方向不承诺"一致 |

## 环境侧反复踩到的配对问题（非本轮新增，记录以便复现）

1. **`IllegalArgumentException: List has more than one element`（`stage=pair`，本轮出现 3 次以上）**
   - 原因：节点 `devices` 表累积 29 行，其中 2 个 active gateway + 3 个 active client，
     host 侧 claim 匹配到多行
   - 处置：配对前置 `update devices set state='revoked' where state='active' and id <> '<本轮 gateway>'`
2. **join code 单次使用**：gateway claim 后即失效，host 不能复用，必须另发 `--role client` 的配对码
3. **`pairing-code create` 的 `--owner-id` 必填**，且无 `owner list` 子命令，需容器内只读查 PG
4. **公网 `8082` 从手机侧不可达**（安全组未开），一律用实验机私网地址（记为 `<lab-host>`）
5. **gateway 重装后未注册**：`status` 返回 `gateway_id=null` 而两个进程都活着，
   表现为拨号时报 `dial_disabled: 旧手机离线`；重跑一次配对即可恢复

## 未完成 / 未执行（明确列出，不含糊）

- P2 / P3 / P4：需多来源，本轮不部署 Worker
- B 组（断网退避、网络恢复）：上一轮通过，本轮未重跑
- C 组：C1/C2 上一轮通过；C3 需另起测试 coturn 容器、C6 需宿主 iptables 只阻断 UDP 3478，均未做
- E 组：E2 需 main 版 SoulGem 一体镜像，未构建
- F 组：F3 需用加入码把 GriefSeed 当节点配对，本轮不部署 Worker，未做
- A 组：上一轮通过，本轮未重跑

## 修复分支（已推送，未合入）

三个修复分支均已推送到远端并各自通过 CI。**未合入任何 PR 分支、未部署、未重启容器。**

| 仓库 | 分支 | 远端 HEAD | 改动规模 |
|---|---|---|---|
| SoulGem | `fix/turn-quota-identity` | `d6ce8d9` | 9 文件 +276/−11 |
| Madoka | `fix/rtc-lifecycle-verify` | `c5574b8` | 9 文件 +391/−33 |
| Homura | `fix/rtc-lifecycle-verify` | `b4fc604` | 13 文件 +627/−68 |

三份补丁另以文件形式存档于 `docs/acceptance/patches/`。补丁内容与上一轮报告"修复轮次"一节一致，
本轮**未新增或修改补丁**（本轮未改产品代码）。

## 安全边界

- 未向真实号码发短信、未拨真实计费电话（使用测试号码与模拟基站注入）
- 未使用生产凭据
- 未推送或评论任何 PR
- 本报告不含 token、TURN 用户名/密码、完整 `mahogsm://relay` 或 `mahogsm://join` 链接
- 本报告以 `<lab-host>` 代替测试机地址

## 测试资源清单（按要求保留，待你决定是否删除）

| 资源 | 状态 |
|---|---|
| 节点 A / 节点 B 容器（含测试版 TURN 配置） | 运行中，保留 |
| 测试 TURN key / 凭据（SoulGem 侧签发） | 保留 |
| 实验机上的四套 APK 与证据目录 | 保留 |
| 模拟器 `emulator-5554` / `emulator-5556` | 运行中，保留 |
| GriefSeed Worker | **本轮未部署**（上一轮部署的版本状态未知，未改动） |

## 证据文件

- `report/evidence/R/R1-summary.json`、`R1-out{1..8}.json`、`R1-in{1,2}.json`
- `report/evidence/R/R2-merged-summary.json`、`R2in-summary.json`、`R2-run1-invalid-methodology.json`
- `report/evidence/R/R3-out.json`、`R3-in.json`
- `report/evidence/R/call-R4-out-*.json`、`call-R4-in-*.json`
- `report/evidence/P/pc-P1-{out,in}{1..3}.json`
- 测试脚本：`lab/r1_quota.py`、`r2_rapid.py`、`r3_nocand.py`、`p_case.py`、`pair_clean.py`、`pair_to_node.py`
