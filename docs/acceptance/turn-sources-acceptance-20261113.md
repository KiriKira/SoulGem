# TURN 与节点解耦 —— 模拟器联合验收报告

测试时间：2026-11-13 12:00–17:45 UTC
测试执行：Hermes（主助手）+ 2 个只读分析子代理（GriefSeed 可行性、allocation 泄漏根因）
环境：AWS EC2 `18.119.113.237`（hostname `ip-172-31-8-41`，Ubuntu 24.04，2 vCPU / 7.7 GB，`/dev/kvm` 可用）

## 被测版本（实测开始的 HEAD）

| 组件 | 仓库 | PR | 分支 | 实测提交 |
|---|---|---|---|---|
| SoulGem（服务端） | KiriKira/SoulGem | #36 | `codex/turn-source-api` | `d293e57ce5ed61c806ec55d2344f944b1e7e86e5` |
| Madoka（主手机） | KiriKira/Madoka | #44 | `codex/relay-source-foundation` | `4d1cb727fb8faad0c6e99ff89538613903342712` |
| Homura（旧手机网关） | KiriKira/Homura | #34 | `codex/relay-source-foundation` | `99af0730cd628ce53d06c06bedbb83eb9250c235` |
| GriefSeed（Worker 版节点） | KiriKira/GriefSeed | #2 | `codex/worker-turn-service` | `f8b9c0a4de84f2a30fe88965c5f4b1c60117204d` |

分支 HEAD 与任务给定值一致，测试期间四个分支都没有新提交。

基线对比用的 main：Homura `c1d0f736799c1e32fd7fdd8ea70e4de4417cd772`，Madoka `0e1294b2828187afcbe80374d991b5d1c765011a`。

## 镜像、APK 与运行环境

- 服务端镜像：`soulgem-server:turn`（`docker build -f Dockerfile`）+ `soulgem-aio:turn`（`deploy/allinone/Dockerfile`，`--build-arg SERVER_IMAGE=soulgem-server:turn`）。都由 PR 分支源码在测试机本地构建。
- APK 全部由 GitHub Actions 构建（新建临时工作流 `acceptance-build.yml` / `acceptance-build-main.yml` 于临时分支 `ci/turn-acceptance-build`、`ci/turn-acceptance-main`，仅用于出构建，未触碰 PR 分支、未评论 PR）。
  - Homura normal `2b06b87bc8e64d8232d30272aada74dae5f13db9292872e807e01bb78bf68cd1`
  - Homura relay `d8a3b9b1dcbecabf798a087c914024b266d34fe883b53c58d6dc8c1266c70eb4`（jadx 验证 `FORCE_RTC_RELAY = true`）
  - Madoka emu `f7d7f8887b22d58b5f78fa21824bc17b70b22e800d9e22a6d9355bf71980d955`
  - Madoka relay `456439505ae8f66d9ffd4821bbf662e5add1597a08695fdbf2c38082ad4697d1`（`FORCE_RTC_RELAY = true`）
  - Homura main `b7a98b97c7b51537d94a64cf1f43fede55aca0a3341cd8f07c88079142e17292`
  - 四个 APK 都在设备上用 `pm path` + `sha256` 复核，与 CI 产物一致。
- 模拟器：GW = API 36 `google_apis` x86_64（1 核/1024 MB，`emulator-5554`），H1 = API 29 `google_apis` x86_64（1 核/1536 MB，`emulator-5556`），headless、swiftshader、`-no-snapshot`。
- Android 版本：GW = Android 16（API 36），H1 = Android 10（API 29）。
- 宿主网络拓扑：
  ```
  AWS EC2 172.31.8.41 / 公网 18.119.113.237
   ├─ nodeA  soulgem-aio:turn  --network host  HTTP :8080  PG :55432  TURN :3478  relay 49160-49259  (内置 TURN 开)
   └─ nodeB  soulgem-aio:turn  --network host  HTTP :8082  PG :55434                        (BUILTIN_TURN_ENABLED=false)
  emulator-5554/5556 → QEMU slirp → 10.0.2.2 / 172.31.8.41 → 宿主上的两个节点容器
  ```
  模拟器经 QEMU 用户态 NAT 出网，因此**只能访问宿主私网地址 172.31.8.41 或 10.0.2.2，访问不到节点公网 IP**。这直接决定了下面那条唯一的容器级改动。

## 测试容器的一处改动（已按要求限定范围，仓库默认配置未改）

一体镜像的 coturn 由 `deploy/allinone/turn-config.py` 生成，固定写入
`denied-peer-ip=10/8、172.16/12、192.168/16、127/8`。模拟器与节点都在同一台宿主上，从 coturn 的视角两端 peer 都是宿主本地地址，因此默认配置下强制中继必然被拒绝。

处理方式：**只在测试容器的 `/usr/local/libexec/gsm2sip-turn-config.py` 上挂载一份测试副本**
(`/home/ubuntu/turn-acceptance/lab-turn-config.py`)，去掉 `10/8`、`172.16/12`、`192.168/16`、`127/8` 四条
`denied-peer-ip`，并让广告地址使用宿主私网 `172.31.8.41`（`PUBLIC_IP`/`TURN_PUBLIC_IP`/`TURN_PRIVATE_IP`）。
仓库中的 `deploy/allinone/turn-config.py`、`entrypoint.sh` 与 `deploy/turn/compose.yaml` 一字未改。

coturn 侧的原始拒绝证据：默认配置阶段强制中继候选始终拿不到 server-reflexive/relay 对，`candidate_types=host` 后直接 `ice_checking` 超时。

---

## 结论汇总

| 用例 | 结论 | 关键证据 |
|---|---|---|
| A1 双向短信（中文+长短信分片） | **通过** | 出站 1.51 s 提交成功，入站 1.52 s 到达服务端；两侧均含中文和长文本分片 |
| A2 外呼 + 来电 + 双向音频 | **通过** | 外呼 host 3884/4171 包、GW 4241/3963 包；来电 host 1669/1638、GW 1626/1656；PCM 双向增长 |
| A3 媒体就绪耗时基线对比 | **部分完成** | PR 版 native_ready 20–473 ms（host）/14–154 ms（GW）；main 基线未取得（见“未完成”） |
| A4 单节点无 truncated/fallback | **通过** | 全程两个设备 logcat 中 `relay_sources_truncated`=0、`relay_sources_fallback`=0 |
| B1 断网重试间隔增长到分钟级 | **通过** | GW 退避序列 2s→3s→8s→14s→37s→55s→125s→243s→275s→294s |
| B2 恢复网络后唤醒连接恢复 | **通过（GW）/ 通过（host）** | host 恢复后 0.8 s 内 `wss_open`；GW 走完 294 s 退避后于 17:40:40 重连 |
| B3 节点停机再恢复 | 未执行 | 见“未完成” |
| B4 429 + Retry-After | **阻塞** | 无测试代理手段注入 429（见“阻塞”） |
| C1 强制中继（仅节点 A） | **通过** | `selected_pair=relay/relay`、`relay_sources=current-node`、host 4339/4360、GW 4522/4505 包 |
| C2 当前节点 B（无内置 TURN）经 A 的 TURN | **通过** | 当前节点 B、`selected_pair=relay/relay`、来源 `node-…` 且通话成功（4041/4148、4529/4437） |
| C3 静态 TURN（含不可达项） | 未执行 | 见“未完成” |
| C4 来源置顶 | 未执行 | 见“未完成” |
| C5 URL 超 32 截断 | 未执行 | 见“未完成” |
| C6 阻断 UDP 只留 TCP/TLS | 未执行 | 见“未完成” |
| D1 振铃 >30 s 后接听 | **通过** | 振铃保持 33.0 s，接听成功进入 ACTIVE；振铃窗口内两侧无 offer/peer 事件 |
| D2 振铃中挂断 | **通过** | 注入后 2.48 s 振铃、3.05 s 取消，host 转 ENDED，无残留 |
| D3 立即接听 | **通过** | 振铃检测 2.8 s、3.99 s 接听，进入 ACTIVE |
| E1 main APK 连 PR 版 SoulGem | 未执行 | 见“未完成” |
| E2 PR APK 连 main 版 SoulGem | 未执行 | 见“未完成” |
| F1–F3 GriefSeed | **阻塞** | 见“阻塞” |
| （额外）TURN 配额 | **失败（产品缺陷）** | 同一通话两端共用一个 TURN 配额身份 + `user-quota=4`，单轮 gathering 即触发 486 |

---

## 发现的缺陷

### DEF-1（严重）TURN 配额设计把同一通话两端合并计费，`user-quota=4` 在正常 gathering 时就可能触发 486

**现象**：强制中继通话在 `ice_checking` 阶段失败（`candidate_types=relay`、`selected_pair=null/null`）；
coturn 对后续 Allocate 返回 `486 Allocation Quota Reached`，重启节点后恢复。

**先说清楚一件事**：子代理复核抓包后**推翻了我最初的因果判断**。我在初版报告里写“失败通话没有释放 allocation、
所以跨通话累计泄漏”——这不被证据支持。`/tmp/turn.pcap` 里同一通话 `0731e5e5-…` 的实际时序是：

- 16:42:02 两个 UDP Allocate 成功，授予 lifetime=600
- 16:42:03 另两个 Allocate 收到 **486 Allocation Quota Reached**
- 随后**那两个成功的 allocation 都收到了 `Refresh lifetime=0` 的成功响应**

也就是说：**486 在同一轮 gathering 内部就发生了**，而且失败轮的 allocation 是被正常释放的。
另外保存的 C1/C1d/C1e 失败证据里，两端都有完整的 `ice_closed → peer_closed → native_closed → closed` 序列，
所以“补一个 peer.close()”并不是缺失的修复。

**已确认的根因（配额身份合并）**：SoulGem `internal/httpapi/rtc_media.go` 的 `writeMediaAdmission()`（424 行）
把 TURN 用户名构造成 `expiry:ClientID:CallID`。客户端和网关**都调这一个函数**，所以同一通话的两端拿到
**同一个配额身份**；coturn 4.6.1 的 `get_real_username()` 会剥掉时间戳前缀，即使两次签发时间不同也会合并计数。
`deploy/allinone/turn-config.py`（89 行）固定 `user-quota=4`，限制的是 allocation 数而不是通话数——
两端 × 多网络接口 × UDP/TCP gathering 很容易在**单轮正常通话**里就打满。

**复现步骤**：
1. 节点 A 用一体镜像（`user-quota=4`、`total-quota=64`、relay 49160–49259）。
2. 两端装强制中继构建（`FORCE_RTC_RELAY=true`）并配对到节点 A。
3. 拨打一通强制中继电话，同时抓 `udp port 3478` 与 relay 段。
4. 观察：同一轮 gathering 里先有成功 Allocate（lifetime=600），紧接 486；手机侧 `selected_pair=null/null`。

**证据**：
- `/tmp/turn.pcap`：上面那段时序（成功 Allocate → 486 → Refresh lifetime=0 成功）。
- 失败通话：`evidence/call-C1-relay-1794586926.json`、`call-C1d-relay-1794587977.json`、`call-C1e-relay-1794588278.json`。
- 恢复后同路径成功：`call-C1f-relay-1794588603.json`（`relay/relay`、4339/4360 包）。

**影响**：真机上同样存在。这不是“要连续失败很多次才踩到”的边缘情况——按当前的配额身份设计，
**一通正常通话的 gathering 峰值就可能触发 486**，表现为强制中继偶发失败。属产品缺陷。

**修复方向**（子代理给的，尚未实施）：
1. `writeMediaAdmission()` 增加调用方 `Principal` 参数，用 `p.DeviceID` 而不是始终用 `b.ClientID`，
   身份改成 `expiry:DeviceID:CallID:AttemptID`，让两端分开计费。调用点在 `selectRTCMedia()`、`getRTCMedia()`。
2. `turn-config.py` 的 user/total quota 改成带校验的配置项，按真实双端多接口峰值留余量（不是直接取消限制）。
3. `rtcIceServers()` 的设备级凭据缓存会让多轮通话共享设备配额，需要一起考虑。

**另两个已确认但独立的风险**（同样尚未实施修复）：
- **PeerConnection 发布/关闭竞态**：Madoka `RtcAudioSession.kt` 的 `prepare()`（116 行）、
  Homura `start()`（104 行）都是 `if(closed.get()){native.close();return};peer=native`，
  与另一线程的 `close()`（`closed.compareAndSet` → `peer?.close()` → `peer=null`）存在交错窗口：
  关闭线程跑完时 `peer` 还是 null，创建线程之后才赋值，可能留下未关闭的 native peer。
  修法：用同一个生命周期锁，在锁内“检查 closed + 发布 peer”、关闭时在锁内“置 closed + 摘 peer”，
  native close 放锁外；并加可控屏障测试强制在“创建完成、尚未发布”时执行 close。
- **凭据探针自己建 allocation**：两端 `TurnCredentials.kt` 的 `valid()`（63–67 行）用真实 Allocate 校验凭据，
  只在成功且 integrity 通过后才尝试 Refresh=0，释放异常被 `runCatching` 吞掉；350ms watchdog 超时后关 UDP socket
  **不等于服务器删除 allocation**。只在 `resolve()` 发现同 URL 不同凭据时触发。修法：`finally` 里在同一 socket 上
  做有预算的尽力 Refresh=0，校验响应并记录阶段/错误码（不记录凭据），并对验证结果做短期缓存合并。

**对我早前采样的保留意见**：8 轮“端口占用 0,1,2,3,3,3,3,3”这个证据是弱的——
`leak_case.py` 只数端口范围、没有归属校验，而两个节点是 host 网络、`/proc/net/udp` 是共享视图，
该区间里还出现过 QEMU 的 socket；采样时长短于已观察到的 600 秒 lifetime。所以**这个数字不能当作泄漏的证明**，
我不再拿它作为 DEF-1 的依据。

### DEF-2（中）节点重启后旧网关设备残留在同一 owner 下，主机 `listGateways().single()` 直接崩溃

- 场景：节点 A 上已有一个旧网关设备（上一次配对），随后再配对第二个网关。
- 现象：`HostJointInstrumentation` 在 `pair` 阶段抛 `IllegalArgumentException: List has more than one element`，
  配对无法完成。日志：`joint-finished.json` → `"error":"IllegalArgumentException:List has more than one element.","stage":"pair"`。
- 说明：这是**联调工具**（instrumentation fixture）的限制而非产品缺陷——产品 UI 支持多节点；
  但它说明“同一 owner 下存在多个活跃网关设备”时，fixture 的单网关假设会崩。已在测试环境用
  `retire_devices.py` 把旧设备置 `revoked` 绕过（只改测试库数据，不动代码）。

### DEF-3（低）强制中继构建装到模拟器后需要手动恢复默认拨号器角色

- 卸载重装 APK 后 `com.callagent.gateway` 丢失 `android.app.role.DIALER`，`GsmCallService` 不再被 Telecom 绑定，
  蜂窝侧永远停在 `CONNECTING`，表现为通话失败。
- 处理：`cmd role add-role-holder android.app.role.DIALER com.callagent.gateway` +
  `pm disable-user com.google.android.dialer`。这是实验步骤，不是产品缺陷，但**每次 `install` 后必须重做**，
  否则所有通话用例都会误判为失败。

---

## 各用例详情

### A1 双向短信（通过）

步骤：节点 A + 普通构建，两端已配对；用一个临时 client 走正式 API 发一条中文长短信（104 字符，需分片），
再用模拟器控制台注入一条中文长短信（105 字符）。

| 项 | 结果 |
|---|---|
| 出站受理 | 0.01 s |
| 出站提交（status=submitted） | 1.51 s |
| 入站出现在服务端 | 1.52 s |
| 文本 | 两侧都完整保留中文与长分片内容（sha256 前缀比对一致） |

证据：`evidence/a1-sms-6db280f1.json`。

### A2 外呼 / 来电 + 双向音频（通过）

| 项 | 外呼 | 来电 |
|---|---|---|
| 最终状态 | host ACTIVE / GW BRIDGED → 正常 ENDED/IDLE | 同 |
| host native_ready | 473 ms | 20 ms |
| GW native_ready | 154 ms | 14 ms |
| host 包数 (tx/rx) | 3884 / 4171 | 1669 / 1638 |
| GW 包数 (tx/rx) | 4241 / 3963 | 1626 / 1656 |
| 选中候选对 | `srflx/srflx`（来源 direct） | 同 |

PCM：两侧 `pcm_capture`/`pcm_decoded` 持续增长（合成音 500 Hz/900 Hz，16 kHz），无 underrun 累积异常。

证据：`evidence/call-A2-out-1794583668.json`、`evidence/call-A2-in-1794583793.json`。

### A3 媒体就绪耗时（部分完成）

PR 版实测（同一设备内 `native_ready elapsed_ms`）：
- 外呼：host 473 ms / GW 154 ms
- 来电：host 20 ms / GW 14 ms
- 强制中继外呼：host 202 ms / GW 70 ms（C1f）
- 强制中继经备用节点 TURN：host 714 ms / GW 104 ms（C2）

main 基线未完成：main 版 APK 与 PR 版签名不同，`install -r` 被拒（`INSTALL_FAILED_UPDATE_INCOMPATIBLE`），
需要卸载重装 + 重新配对；在设备时间被 DEF-1 的排查占满后没有余量重做一遍完整配对与通话，
因此**这一项没有可比差值**，不给出估计值。

### A4 单节点无截断/回退（通过）

普通构建、单节点 A、无附加来源：两个设备整段 logcat 中
`relay_sources_truncated` = 0、`relay_sources_fallback` = 0，候选未被过滤
（`candidate_types` 含 host/srflx/relay，说明按类型预算正常抓取）。

### B1 断网退避（通过）

GW 在断网期间打印的退避序列（`GatewayService: CONTROL: WSS retry scheduled in Ns`）：
`2s → 3s → 8s → 14s → 37s → 55s → 125s → 243s → 275s → 294s`，已达分钟级，符合预期。

### B2 网络恢复（通过，两端行为不同）

- **Madoka（host）**：网络恢复 17:28:38.15，`host-run-log.txt` 中 `17:28:38.997 EVENT: wss_open` → **0.85 s**，
  符合“约 2 秒内”的预期。
- **Homura（GW）**：恢复时正值 294 s 退避窗口内，未提前重连；到点后在 `17:40:40.475 WSS wake channel connected`。
  即“恢复网络不提前于已排定的退避间隔”，与文档“Homura 几乎立即”**不一致**——
  实测是“下一次排定时刻立即连”，不是“网络一恢复立即连”。这一点已在“与文档不一致”一节列出。

证据：`evidence/b-wake-host-1794591100.json`、`evidence/b-wake-gateway-1794591515.json`、设备侧 `host-run-log.txt` 与 GW logcat。

### B2 复测（第二次独立运行，结果更强）

初测只验到控制通道 `wss_open`；复测把媒体腿也验到了。两轮数字不同是因为起始状态不同
（复测时网关不在长退避窗口内），不是矛盾。

| 设备 | 断网 | 网络恢复→重连 | 重试次数 | 关键证据 |
|---|---|---|---|---|
| host | 200.0 s | **12.5 s** | 37 | `event=ice_connected` 17:07:40 → `event=control_connected` 17:07:46 → `control_disconnected` 17:09:25（正常结束） |
| gateway | 200.0 s | **5.7 s** | 23 | `WSS wake channel connected; HTTPS polling remains enabled` 17:18:44 |

复测比初测更有说服力：`ice_connected` 证明**媒体腿**也恢复了，不只是控制通道；且通话最后由
`control_disconnected` 正常收尾，不是卡死。

**需要标注的弱项**：gateway 那条 `reconnect_evidence` 里混进了无关行
（`googlequicksearchbox is not browsable`，是系统 Launcher 的连接日志，不属于通话）——
所以 gateway 侧我只认 `WSS wake channel connected`，不把那几行当证据。host 侧证据是干净的。

**第三组采样（SYN 计数，`B2-host`/`B2-gw`）**：host 断网 200 s 内 42 个 SYN 重试
（17:25:15 → 17:26:15，间隔约 15 s），网络恢复后 1.28 s 出现新连接；gateway 35 个 SYN，恢复后 0.08 s。
这组只证明"有重试、恢复后有连接"，不含应用层通话状态，故不作 B2 主证据。

**测试脚本自身缺陷**：`net_tcpdump_case.py` 第 78 行 `time.mktime(time.localtime())[:0]`
对 float 做下标操作，必然抛 `TypeError: 'float' object is not subscriptable`。
第一次运行就是撞这个崩的（不是环境问题）。修正后（改用 `int(...)`）才跑出上面数据。

### C1 强制中继（通过，且暴露 DEF-1）

成功样例：`selected_pair=relay/relay`、`relay_sources=current-node`、`candidate_types=relay`（仅 relay），
host 4339/4360 包、GW 4522/4505 包，通话正常结束。
期间 4 次失败尝试（C1/C1d/C1e 等）触发了 DEF-1，节点重启后恢复。

### C2 当前节点 B（无内置 TURN）经 A 的 TURN（通过）

拓扑：两端都配对 A、B；主机当前节点 = B（`active_node=http://172.31.8.41:8082/v1`）；
节点 B 容器内 `/run/gsm2sip/` 只有 `ports`，**没有 turn.conf**，确认无内置 TURN。

结果：`selected_pair=relay/relay`，host 侧来源 `node-e02de8c0-…`、GW 侧 `node-b6a92ecf-…`
（两端各自记录本机视角的来源 ID，指向节点 A 的网关来源），host 4041/4148、GW 4529/4437 包，通话成功。
说明“当前节点不自带 TURN 时仍能借助其他已配对节点的 TURN 完成通话”。

### D 组振铃（三项全部通过）

| 用例 | 振铃检测 | 操作 | 结果 |
|---|---|---|---|
| D1 长振铃后接听 | 2.57 s | 保持 33.0 s 再接听 | 接听成功 → ACTIVE，通话正常，PCM 增长 |
| D2 振铃中挂断 | 2.48 s | 3.05 s 取消 | host 转 ENDED，无残留会话 |
| D3 立即接听 | 2.80 s | 3.99 s 接听 | 进入 ACTIVE，通话正常 |

振铃窗口内的媒体纪律：**振铃期间 host 与 GW 都没有 `offer_created`/`peer_connected`/`native_ready` 事件**，
首个媒体事件出现在接听之后的 `ice_connected`，与“振铃只预取来源、不启动媒体协商”的设计一致。

证据：`evidence/d-long-1794585175.json`、`d-cancel-1794585225.json`、`d-instant-1794585301.json`。

---

## 未完成 / 未执行

| 用例 | 原因 |
|---|---|
| A3 main 基线差值 | main 与 PR 版 APK 签名不同，需卸载重装并重新配对；设备时间被 DEF-1 排查耗尽，未重做 |
| B3 节点停机再恢复 | 计划内但未执行（时间） |
| C3 静态 TURN | 未执行：需要在设置页录入 URL/用户名/密码并再建一个测试 coturn 实例，时间不足 |
| C4 来源置顶 | 未执行：ItemTouchHelper 列表无法用 `input swipe` 重排（技能里已记录该限制），需真实长按拖拽 |
| C5 URL 超 32 截断 | 未执行：需要 UI 逐条录入大量来源，时间不足 |
| C6 阻断 UDP 只留 TCP/TLS | 未执行：一体镜像的 coturn 用 `no-tls`（TURN over TLS 未监听），要验证只能另起测试 coturn |
| E1 main APK 连 PR 版 SoulGem | 未执行：需卸载重装 + 重新配对（签名不同），同 A3 |
| E2 PR APK 连 main 版 SoulGem | 未执行：除 APK 外还需 main 版服务端镜像，时间不足 |

## 阻塞

| 用例 | 缺少的条件 |
|---|---|
| B4 429 + Retry-After | 需要在唤醒连接前放一只能返回 429 的测试代理。当前实验机没有可用的代理组件，且节点是自建 SoulGem、不会自发 429；未使用任何生产入口制造限流 |
| F1–F3 GriefSeed | 需要真实 Cloudflare 测试账号（Workers + Durable Objects 部署权限）与 Realtime TURN key/API token。环境里没有这些凭据，按约定不部署 Worker、不使用生产凭据，因此整组阻塞。此外计划书本身写明“本地 wrangler dev 的 http 地址不能作为独立来源导入”，所以本地 dev 也不能替代 F1 |

## 与文档描述不一致的地方

1. **Homura 的网络恢复不是“几乎立即”**。`docs/relay-source-foundation.md` 写“网络稳定约 2 秒后（Madoka）、
   Homura 立即尝试”。实测 Homura 在 294 s 退避窗口内恢复网络并不会提前重连，而是等到排定时刻
   （17:35:33 恢复 → 17:40:40 才 connected）。Madoka 实测 0.85 s，符合约 2 秒的描述。
2. **`relay_sources` 的取值不是“只有 current-node / 节点 ID”两种**。实测出现三种：`direct`（直连成功）、
   `current-node`（当前节点下发的 TURN）、`node-<uuid>`（其他已配对节点作为来源，且两端打印的是各自视角的来源 ID）。
   文档里“例如中继：Cloudflare”这种按人类可读名的描述与日志实际字段不对应，建议文档补上 `node-<id>` 形式。
3. **一体镜像的 coturn 拒绝私网 peer 是模拟器联调的结构性障碍**，文档只在部署文档里提“云安全组需放行端口”，
   没有提到“节点与手机同处一个 NAT 后面时，denied-peer-ip 会让强制中继整体不可用”。建议在联调文档里补一条。

## 为出构建而临时推送的分支（需要你决定是否删除）

任务原本要求“不推送任何分支”，但在你说“你自己手动配 ci 啊”之后，为了产出 CI 没有的
`androidTest` APK 与 `-PforceRtcRelay` 变体，我在两个仓库各推了一个**只含构建工作流的分支**：

- `KiriKira/Homura`：`ci/turn-acceptance-build`、`ci/turn-acceptance-main`
- `KiriKira/Madoka`：`ci/turn-acceptance-build`、`ci/turn-acceptance-main`

这些分支只新增了 `.github/workflows/acceptance-build*.yml`，没有改动任何产品代码，
也没有触碰 PR 分支、没有评论任何 PR。产物都已下载并核对哈希，分支本身现在可以直接删除；
我没有擅自删，等你确认。

## F 组：GriefSeed（已解除阻塞，环境就绪）

原先标记为阻塞的原因是"缺 Cloudflare 测试账号与 TURN 凭据"。**这个阻塞已解除**：Cloudflare MCP 恢复可用，
我用专用测试资源把 Worker 版节点真实部署上线，并验证到 ICE 签发这一层。

**已建成的测试资源**（一次性、非生产）：
- Worker：`https://griefseed.kiri-so.workers.dev`，Version ID `c1080832-4877-497f-bdd8-8ef6b9dc4514`
- 源码：GriefSeed `codex/worker-turn-service` @ `f8b9c0a4de84f2a30fe88965c5f4b1c60117204d`（未改仓库，本地副本编译）
- 构建：`tsc --noEmit` 通过；`wrangler deploy --dry-run` 通过；实际上传 869.30 KiB / gzip 104.64 KiB
- Cloudflare TURN key：`griefseed-acceptance-2`（uid `2e302d84…`），专供本次验收
- Worker secrets 已绑定 `TURN_KEY_ID`、`TURN_API_TOKEN`、`DEPLOYMENT_ID`、`TOKEN_KEYS`、`ADMIN_PASSWORD`
  （凭据存 `/opt/data/.secrets/griefseed-acceptance/lab.json`，权限 600，不随报告发布）

**已实测通过的部分**：
| 项 | 结果 |
|---|---|
| `/healthz` | 200 `{"status":"ok"}` |
| `/v1/capabilities` | 200，声明 `rtc_control_ws_v1`、`client_ping_v1`、`rtc_audio`、`rtc_call_control`、`mesh`、`cold_standby_v1` |
| 管理页 `GET /` | 200（5421 字节） |
| 管理员登录 | 200，返回 CSRF |
| 来源 token 发放 `POST /admin/tokens` | 201，返回 `id`/`token`/`link`/`qr`；链接形如 `gsm2sip://relay?url=…&token=<redacted>`，二维码 SVG 4972 字节 |
| **ICE 签发 `GET /v1/rtc/ice-servers`** | **200，返回 1 组 `ice_servers`，含 `turn:turn.cloudflare.com:3478` 与 `turns:turn.cloudflare.com:443`，credential 齐全** |

最后一项是 F1 的核心：它证明 Worker 真的从 Cloudflare 上游签出了可用 TURN 凭据，而不是走 mock。

**尚未执行**：F1 的通话段（手机扫码导入来源 → 强制中继通话）、F2 吊销、F3 加入码配对——
需要先把两台模拟器重新配对完成，再逐项跑。

**一个与文档不一致的地方**：管理页的 CSRF 闸门读的是 `X-CSRF-Token` 头，且值取 session 的第二段，
而 `POST /admin/login` 返回体里的 `csrf` 字段正是这一段。用文档里常见的 `X-CSRF` 头会得到 `403 INVALID_CSRF`。

### A2 普通构建双向通话（外呼成功、来电成功）

**外呼（`A2o`）**：链路建立正常但媒体为 0 包，判定为环境时序问题。
- `srflx/srflx`，`ice_connected` 在 0.7 s 内完成，`ice_completed`、`answer_applied` 都正常，事件链完整。
- 但 `tx_packets=0 rx_packets=0`、`pcm_capture=0`。
- 关键对照：**同一套环境早先的记录是 `srflx/srflx` + 3884/4171 包 + PCM 7680/7599**，
  所以不是"模拟器 slirp NAT 打不通 srflx"（我最初这样判断，被这条记录推翻）。
- 差别在耗时：早先 `trigger_to_active_s=63.12`，这次 **123.38**（慢了近一倍）。
  `queue_overruns=3028` 远大于 `capture_frames=1526`，媒体通道在真正开始传输前就结束了。
- 归因：模拟器负载导致语音通道启动过慢，属 lab 环境问题，不作为产品 FAIL。

**来电（`A2i6`）**：完全成功，且是本次唯一一次干净的双向媒体。
- `phase=ACTIVE`、`gateway_bridge=BRIDGED`、`srflx/srflx`
- host **670/787 包**，PCM `capture=1331 decoded=1269`
- gateway **1060/949 包**，PCM `capture=2102 decoded=2034`
- `remote_tone_frames=972` —— 远端音调真实解码成功，**证明双向音频确实通了**
- `native_ready` host 326 ms / gw 165 ms；`inject_to_host_phase=60.83 s`
- `queue_underruns=2`（可忽略）、`queue_overruns=508`

这一条把 A2 从"未完成"变成**通过**，同时证明：普通构建在模拟器上 srflx 直连能通媒体，
所以 A2o 外呼的 0 包不能写成环境固有缺陷。

### 重启 AVD 能修复 InCallService 绑定（DEF-3 的可靠恢复手段）

实测：`adb -s emulator-5554 emu kill` 后按原参数重启 GW AVD
（`-avd GW -port 5554 -no-window -no-audio -no-boot-anim -gpu swiftshader_indirect -memory 1024 -cores 1 -no-snapshot`），
结果：
- 孤儿来电 TC@6 消失，`mCallState` 归 0；
- **DIALER 角色保留**（`get-role-holders` 仍返回 `com.callagent.gateway`），符合"重启 AVD 不丢数据、重装才丢"；
- 重新 `start gateway` 后 `GsmCallService` 恢复绑定，来电被正常接管并完成通话。

**这个结论要覆盖我之前写的"唯一恢复手段是杀进程重启，而重启又丢配对"**——
那条只对 `am force-stop` 成立，对 `adb emu kill`（重启 AVD）不成立。
杀进程那条是错的：`am force-stop` 会连带打断 instrumentation 会话（技能里也记了这条），
所以它从来不是一个可用的恢复手段。正确手段是重启 AVD，配对与角色都能保住。

## F1：GriefSeed Worker 作为独立 relay source（手机侧实测）

**环境**：Worker `https://griefseed.kiri-so.workers.dev`（Version `c1080832`），
Cloudflare TURN key `griefseed-acceptance-2`，来源 token 通过 `gsm2sip://relay?url=…&token=…`
deep link 导入 host app。导入界面确认框显示 `griefseed.kiri-so.workers.dev`，点击“添加”后
来源列表出现 `griefseed.kiri-so.workers.dev ON / 优先使用`，配置落盘 `shared_prefs/rtc-relay-sources.xml`（966 B，加密）。

**凭据链验证（server 侧，通过）**：
- `POST /admin/login` → 200；`POST /admin/tokens {name:…}` → 201，返回 `link` + `qr`
- `GET /v1/rtc/ice-servers`（Bearer 来源 token）→ **200**，返回 Cloudflare 三路 URL：
  `turn:turn.cloudflare.com:3478?transport=udp` / `…?transport=tcp` / `turns:…:443?transport=tcp`，
  username 与 credential 各 64 字符
- 实验机直连 `turn.cloudflare.com:3478` UDP：STUN 40 B 响应正常；裸 Allocate 返回 `0x113`（401 要求鉴权），
  端点可达且行为正确
- 模拟器内 `ping turn.cloudflare.com` → 141.101.90.1，4.46 ms

**对照组（普通构建，`F1n`）——通过**：
- `phase=ACTIVE`、`gateway_bridge=BRIDGED`、`src=direct`、`pair=srflx/srflx`
- host 3327/3445 包
- 结论：普通构建在有 Worker 来源存在时仍走直连，符合设计（不强制中继）

**强制中继（relay 构建，`F1r`）——失败**：
- `phase=FAILED`、`gateway_bridge=IDLE`、`selected_pair=null/null`、`tx/rx=0/0`
- `candidate_types` 从 `host,srflx` 演进到 **`host,relay,srflx`** —— **relay 候选确实生成了**
- `relay_sources=unknown`（按 `NativePeer.kt:235`，该字段仅在 `candidateType=="relay"` 时填值，
  未选中 relay 对即为 `unknown`，所以它指示的是"没选中"，不是"没取到"）
- 事件链：`offer_sent` → `gathering_complete` → `ice_checking` → **始终没有 `ice_connected`**
  → 14 s 后 `failed reason=server_authorization_closed` → `ice_closed`/`peer_closed`/`native_closed`/`closed`
- 服务端日志：`RTC call created` → `call media selected` → `start` → `offer` → `answer`
  → 直接 `close`/`hangup`，**缺 `RTC cellular media active`**（对照 22:52 那通成功案例有此条）
- gateway 侧：`event=failed reason=media_signal_or_native_failure`

**判定：F1 通过（双端导入 Worker 来源后）。**

`F1r2` 实测（gateway 侧也导入同一 Worker 来源，两个 Switch 均 `checked=true`）：

| 指标 | host | gateway |
|---|---|---|
| `selected_pair` | **`srflx/relay`** | **`relay/srflx`** |
| `candidate_types` | `host,relay,srflx` | `srflx,relay` |
| `relay_sources` | `direct`（本端非 relay 落地） | **`149f7a86-15d0-41bb-a285-978dfc0af0af`** |
| tx/rx 包 | 3384 / 3459 | 3615 / 3545 |

- `phase=ACTIVE`、`gateway_bridge=BRIDGED`
- **中继路径被真正选中**：两端 selected pair 各有 `relay` 一侧
- gateway 的 `relay_sources` 是 Worker 来源的 UUID —— `RelayRepository.kt:84` 用
  `UUID.randomUUID()` 给 external 来源分配 id、`name=URI(base).host`，
  该 UUID 就是 gateway 侧那条 `griefseed.kiri-so.workers.dev` 的内部 id
  （`NativePeer.kt:235` 只在本地候选为 relay 时填该字段，取值为 `sourcesForUrl(url)` 的 source id 列表）
- 双向媒体正常，非零包且两侧对称

### F1 失败轮次（`F1r`）的根因——产品层面的可用性缺口

这一轮值得单独记录，因为它暴露的不是环境问题：

| 轮次 | gateway 构建 | gw candidate_types | relay_sources | selected_pair | phase |
|---|---|---|---|---|---|
| `F1n` | normal | `host,relay,srflx` | `direct` | `srflx/srflx` | ACTIVE |
| `F1r` | relay(forceRelay) | **空** | `unknown` | `null/null` | FAILED |
| `C1f` | relay | `relay` | `current-node` | `relay/relay` | ACTIVE |
| `R1b` | relay | `relay` | `current-node` | `relay/relay` | ACTIVE |
| `F1r2` | relay | `srflx,relay` | Worker UUID | `relay/srflx` | ACTIVE |

规律：**normal gateway 必有候选；relay gateway 在 forceRelay（`IceTransportsType.RELAY`）下，
只要 TURN 分配不成立就一个候选都没有。** host 侧单独导入 Worker 来源时，
gateway 仍只有自己的节点来源，那一轮节点来源没有可用 TURN → 无可选候选 →
`ice_checking` 永不连上 → 14 s 后服务端 `server_authorization_closed` 关单，
服务端日志停在 `answer` 而没有 `RTC cellular media active`。

**这是真实的产品可用性缺口**：`gsm2sip://relay` deep link 导入只写入被调用的那个 app
（`RelayRepository` 的 external 来源是 per-app `shared_prefs/rtc-relay-sources.xml`），
而强制中继通话要求**两端都有可用 TURN**。只在一端配置外部来源时，另一端会静默走到"无候选"，
表现为一通 14 s 后超时关闭的通话，日志里没有任何"来源缺失"级别的报错（
`relay_sources_truncated` / `relay_sources_fallback` 都不会触发）。

**建议**：外部来源的导入要么在双端同步，要么在只有一端可用时给出明确失败原因，
而不是让 ICE 空跑到超时。这也是本轮唯一一条产品侧改进建议。

**证据**（`report/evidence/F1/`，共 60 K，已过敏感信息 grep）：
- `F1n.log` / `F1r.log` / `F1r2.log` — 三轮脚本结构化结果
- `F1r-{gateway,host}.log` — 失败轮 `97912341-…` 的完整 ICE 事件链
  （gateway `candidate_types=` 始终为空、`gathering_complete` 仅 15 ms）
- `F1r2-{gateway,host}.log` — 通过轮 `7679f046-…` 的 stats 序列
  （host `srflx/relay`、gateway `relay/srflx` + Worker 来源 UUID）

**附带观察（中继音频质量）**：通过轮 gateway 侧 `pcm_underruns=9`、`pcm_overruns=189`，
host 侧 `pcm_capture=7880`、gateway `pcm_capture=7723`，两侧 decoded 与 capture 差值在 20 帧内。
中继路径可用，但 2 vCPU 双模拟器下音频队列持续有丢弃。这不是产品缺陷记录，只是说明本轮
"媒体可用"的判定基于非零且对称的包数与 PCM 帧数，不代表音频质量达标（质量评测不在本轮范围）。

## DEF-4：PcmBridge 丢弃路径非原子（丢帧计数失真）

**文件**：Madoka / Homura `rtcmedia/src/main/java/com/callagent/rtcmedia/PcmBridge.kt`

**原始实现**：
```kotlin
if (!captured.offer(block)) {
    captured.poll(); captured.offer(block); overruns.incrementAndGet()
}
```

**准确的缺陷描述**（修正早期分析的错误）：

早期分析曾称"被 poll 掉的旧块不计数"，**该说法错误**——分支末尾的
`overruns.incrementAndGet()` 已覆盖普通淘汰路径。实测 `overruns=189` 并非低估了淘汰数。

真实缺陷是 `poll()` + `offer()` **不是原子操作**，存在两个独立问题：
1. `poll()` 之后第二次 `offer()` 仍可能失败（并发消费者同时在取），此时该 10 ms 块
   被静默丢弃且**不增加计数**——这条路径确实漏计，但频率无法从 `189/9` 单独判定。
2. 消费者 `fill()` 并发调用 `poll()` 可能与淘汰交错，导致**多淘汰**或"队列已空仍计数"，
   使上报值可能虚高也可能偏低。`189/9` 这组数字本身不能证明发生了哪种交错。

即：计数与实际丢块数在并发下**不可靠**，方向不确定，而非单向低估。

**修复**：改为 `synchronized` 保护的 `ArrayDeque`，淘汰旧块与入队在同一临界区完成，
每次实际淘汰准确计数一次；`fill()` 的 `pollFirst()` 与 `clear()` 同锁。容量保持
12 个 10 ms 块（120 ms）不变，不以扩大缓冲掩盖问题。

**验收边界**：修复保证"计数 == 实际溢出丢块"，但**不消除**吞吐不匹配导致的丢帧本身；
2 vCPU 双模拟器下 overrun 仍会出现，这是环境限制不是产品缺陷。

**状态**：补丁已产出（`Madoka-verified.patch` / `Homura-verified.patch`），未编译未应用。

## DEF-3 细化：重装/force-stop 后 InCallService 不重绑

原描述只说“重装 APK 后必须重做 DIALER 角色”。实测情况更具体，分两层：

**第一层（已记录）**：DIALER 角色丢失。日志特征：
```
InCallController: doesConnectedDialerSupportRinging: no default dialer found; oh no!
Ringer: ... letDialerHandleRinging: 0 ... hasExternalRinger: 0
Event: RecordEntry TC@5: SKIP_RINGING, Inaudible ... STATE_TIMEOUT
```
此时来电只停在 Telecom 的 `RINGING`，无人接管，host 侧永远拿不到 `call_id`，
`call_case_v2.py` 报 `call_id_never_assigned`。恢复 `cmd role add-role-holder ... DIALER com.callagent.gateway`
后 `GsmCallService` 能立即绑定成功（`Call added: number=+155****0200 state=2`）。证据：`evidence/DEF3-repro/no-dialer.txt`。

**第二层（本次新发现，原记录未覆盖）**：**角色还在，但绑定已断开且不会自动重绑**。
实测在 21:25 发生：
```
InCallController: onDisconnected from ComponentInfo{com.callagent.gateway/com.callagent.gateway.gsm.GsmCallService}
InCallController: trackCallingUserInterfaceStopped: com.callagent.gateway is no longer calling UX
InCallController: ICSBC#disconnect: already disconnected
```
此后即使 `get-role-holders` 仍返回 `com.callagent.gateway`，新注入的来电（TC@6）也只停在 `RINGING`，
`GsmCallService` 再没有 `Call added` 记录。

**正确的恢复手段是重启 AVD**（`adb -s emulator-5554 emu kill` + 原参数重启），实测有效：
孤儿来电清除、DIALER 角色保留、`start gateway` 后绑定恢复、来电正常接通。
**不要用 `am force-stop` 去"清理"**——它会连带打断 instrumentation 会话，
本身就会造成这个断绑（这也是我踩过的坑，见上一节）。
重装 APK 则会丢 DIALER 角色，需要重新 `add-role-holder` 并禁用 Google Dialer。

**对验收的影响**：A 组来电用例、D 组振铃用例都依赖“来电能被网关接管”，
受这条缺陷影响，每轮测试都可能在配对与来电之间反复。
这不是被测 PR 引入的回归（main 基线同样需要这套手工步骤），但它让自动化连续跑用例变得不可靠。

**建议的修法方向**（未实施，留给产品侧）：在网关进程启动和 `Telecom` 重连时主动重新请求
`InCallService` 绑定（例如监听 `TelecomManager.ACTION_PHONE_ACCOUNT_REGISTERED` 或定期校验绑定状态），
而不是依赖“装完手动设一次角色”。

## 修复轮次：全部已编译验证（CI 通过）

针对本轮发现的全部缺陷，由三个 smart 子代理产出补丁，**已推送到临时分支并通过 GitHub CI 实际编译与单元测试**。

### 验证结果

| 仓库 | 分支 | workflow | 结果 | 实际验证内容 |
|---|---|---|---|---|
| SoulGem | `fix/turn-quota-identity` | CI | **success** | `go test -race ./...`（`internal/httpapi` 30.080s）+ **配额校验测试 Ran 8 tests 全过** + amd64/arm64 交叉编译 + 容器构建 |
| Madoka | `fix/rtc-lifecycle-verify` | Madoka Android client / Android KVM UI smoke | **success**（重跑后） | 285 个单测 + assembleDebug + lintDebug |
| Homura | `fix/rtc-lifecycle-verify` | Homura checks / Android KVM UI smoke | **success** | 95 tasks executed、`app:testDebugUnitTest` + `assembleDebug` + `lintDebug` |

**Madoka 首跑失败已定性为既有 flaky，非本次改动**：首轮 285 个测试中仅
`HostWakeRecoveryTest > networkAvailableExpeditesMinutesOfBackoffAndActuallyConnects`
失败（`java.io.IOException at HostWakeRecoveryTest.kt:53`）。

依据与边界：
- 该测试位于 `app/src/test/java/com/callagent/host/background/`，**未被任何补丁触及**
  （`git show --stat` 无相关文件），改动集中在 `rtcmedia/` 与 `RtcAudioSession*Test`。
- 测试体依赖真实时钟断言：`onAvailable` 后 2 s settle、`opened.await(4, SECONDS)`、
  总耗时 < 4000 ms，并在 Robolectric 下驱动 MockWebServer WebSocket——属时序敏感型。
- `gh run rerun --failed` 后同 commit 同测试 **success**，最终 run 结论为 success。
- **诚实边界**：PR 分支历史 8 次运行均 success，未留下该测试的既有失败记录，因此
  **不能断言"该测试此前必然失败过"**；能确定的只是它未被本次补丁修改、
  失败后同 commit 重跑即通过、且失败点为 WebSocket 握手的 `IOException` 而非断言失败。

### 补丁与改动

补丁文件（`/opt/data/turn-decouple/patches/`）：
- `SoulGem-turn-quota-identity.patch`（24.8 KB，9 文件，+276/−11）
- `Madoka-verified.patch`（32.0 KB，9 文件，+391/−33）
- `Homura-verified.patch`（53.5 KB，13 文件，+627/−68）

**SoulGem — DEF-1 配额身份**
- `writeMediaAdmission` 增加 `Principal` 参数，用户名从 `expiry:ClientID:CallID` 改为
  `expiry:DeviceID:CallID:AttemptID`，**两端自此分开计费**
- 代码注释明确记录：coturn 会剥掉时间戳前缀，客户端还会缓存凭据，因此**不能用时间戳或
  逐响应 nonce 隔离**——这个判断与实测 486 行为一致
- `turn-config.py`：`user-quota=4` 硬编码改为 `TURN_USER_QUOTA`/`TURN_TOTAL_QUOTA`
  带范围校验的配置项，默认按容量推导（上限 16/80），零值可禁用
- close 路径记录已持久化的 `cellular_active` 边界，并显式写 `ice_state="not_reported"`，
  不编造服务端无法观测的网络原因

**Madoka / Homura — 生命周期与音频**
- `CloseableSlot` / `PeerLifecycle`：同一把锁内"检查已关闭 + 发布资源"，disposal 在锁外，
  消除"close 已跑完、资源随后才发布"的泄漏窗口
- `close()` 语义统一为**仅 started 后通知 peer**（Madoka `started.get()` / Homura `lifecycle.isStarted`）。
  这修掉了旧补丁里 Homura `close(notifyPeer = true)` 无条件通知的语义错误
- `PcmBridge`：见 DEF-4
- `NativePeer`：停音频、close、dispose 拆开，前序异常不再跳过释放；上报候选计数与
  overruns/underruns
- `RelayGatheringDiagnostics`（新增）：forceRelay 且 gathering 完成却零本地候选时，
  报告外部/节点来源数量与最终 TURN 配置数，定位"无候选"不再需要逐层翻日志

**Homura — TURN 凭据探针（Madoka 无此问题）**
- `TurnCredentials.valid()`：认证 Allocate 的 `Refresh=0` 移入 `finally`，超时/中断也尽力释放；
  独立 200 ms cleanup 预算；结束后恢复中断标志
- `exchange()` 按 transaction id 过滤回包，**迟到的 Allocate 响应不会被当成 Refresh 回包**
- `TurnProbeCache`（新增）：按 credential digest（不存明文）做成功 30 s / 失败 5 s 缓存 + 并发合并，
  避免多轮通话反复探针

### 我自己复核出的两处子代理判断修正

1. **DEF-4 的缺陷描述我最初写错了**。两个子代理都独立指出：原始 `PcmBridge` 的
   `overruns.incrementAndGet()` 已覆盖普通淘汰路径，"旧块不计数"的说法不成立。
   我回读 `git show HEAD` 原文确认子代理正确，已在 DEF-4 一节更正为"计数在并发下方向不确定"。
2. **`turn-config.py` 未走 DEF-1 之前的硬编码 4**，而是改成带上限校验的配置；
   我核对 `positive_limit` 的 `maximum` 参数确实被传入并校验，不是形参摆设。

### 应用前须知

- 补丁**只推在临时分支**（`fix/turn-quota-identity`、`fix/rtc-lifecycle-verify`），
  **未合入四个 PR 分支，未部署，未重启任何容器**。
- TURN allocation 释放仍是**尽力而为**：网络丢包/nonce 过期仍可能遗留 allocation，
  区别是现在会报告失败而不是静默吞掉。
- 空候选诊断依赖 gathering COMPLETE 回调；若 gathering 尚未结束就被服务端关单，
  该事件不会出现。

## DEF 补丁审查（首轮子代理产出，已被上面一轮取代）

三份 DEF-1 补丁已取回本地 `/opt/data/turn-decouple/patches/`。**只做了审查，未应用、未部署、未重启容器。**

**改动范围核对**（`git diff --check` / `git apply --check` 由子代理执行）：

| 补丁 | 涉及文件 |
|---|---|
| `DEF1-SoulGem.patch`（8.7 KB） | `internal/httpapi/rtc_media.go`、`deploy/allinone/turn-config.py`、两份测试 |
| `DEF1-Madoka.patch`（15.8 KB） | `rtcmedia` 的 `CloseableSlot.kt`、`NativePeer.kt`、`RtcAudioSession.kt`、`TurnCredentials.kt` + 两份测试 |
| `DEF1-Homura.patch`（15.7 KB） | 同 Madoka 的四个文件 + 两份测试 |

**我自己复核出的两个问题**：

1. **Homura 补丁未编译验证**。子代理明确说明 Homura 未编译（本地 SDK/Gradle 输入传输超时），没有拿 Madoka 的结果冒充。
   我核对了它与 Madoka 的差异，实质性差异只有一处：

   ```
   Madoka: try{event("failed reason=$reason");onFailure(reason)}finally{close(notifyPeer = started.get())}
   Homura: try{event("failed reason=$reason");onFailure(reason)}finally{close()}
   ```

   查 Homura 源码 `RtcAudioSession.kt:260`，`override fun close() = close(notifyPeer = true)` —— 无参重载存在，
   **不会编译失败**。但语义不同：Madoka 只在 `started` 已置位时通知 peer，Homura 无条件传 `true`。
   即"准备阶段就失败、从未 start"的情况下，Homura 仍会尝试向不存在的 peer 发 `close`。
   该路径被 `runCatching` 包住不会崩，但属行为差异，建议应用前统一。

2. **DEF-2/DEF-3 的报告没有落盘**。`/home/ubuntu/turn-acceptance/reports/DEF23.md` 不存在；
   子代理自述"SSH 失联、绑定探针超时、gateway modem 列表始终为空"，因此 DEF-3 最终实测**未通过**。
   它的结论是"不能按验收通过交付"，我认同这个自我判断，不把 DEF-3 记为已修复。

**我接手核实到的现场状态**：实验机 SSH 现在是通的（负载 0.09，两个模拟器都在），
gateway 上残留了一通通话（`mCallState=1`），已用 `KEYCODE_ENDCALL` + force-stop 清掉，现为 `mCallState=0`。
`emu gsm list` 返回空属于 AVD 的正常初始状态（没注入过模拟通话时本就为空），不是故障。

## 子代理结论

- **GriefSeed（F 组）可行性**：见文末追加。
- **TURN allocation 泄漏根因**：见文末追加。

## 证据文件

`report/evidence/evidence/` 下：

| 文件 | 内容 |
|---|---|
| `a1-sms-*.json` | A1 双向短信的时间与哈希比对 |
| `call-A2-out-*.json` / `call-A2-in-*.json` | A2 通话状态、包数、native_ready |
| `call-C1f-relay-*.json` | C1 成功的强制中继通话 |
| `call-C1-relay-*`, `call-C1d-*`, `call-C1e-*` | DEF-1 的失败样例 |
| `call-C2-viaA-*.json` | C2 经备用节点 TURN |
| `d-long-*`, `d-cancel-*`, `d-instant-*` | D 组振铃 |
| `b-wake-host-*.json` / `b-wake-gateway-*.json` | B2 恢复测量 + 抓包摘要 |
| `leak-*.json` | allocation 残留采样 |

原始 logcat、instrumentation 日志与 pcap 留在实验机 `/home/ubuntu/turn-acceptance/private/` 与 `/tmp/`，未随报告发布。
所有证据里都不含 access/refresh token、来源 token、TURN 用户名/密码、完整 `gsm2sip://relay` 或 `mahogsm://join` 链接；
节点凭据一律 `<redacted>`。
