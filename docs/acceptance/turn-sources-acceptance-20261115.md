# TURN 来源第四轮验收（2026-11-15）

第三轮报告（`turn-sources-acceptance-20261114b.md`）已审阅，其余结论保留。本文件只补第三轮遗留的五项，
并更正上一轮的两处表述。

## 1. 被测提交

| 仓库 | 分支 | 提交 | 变更 |
|---|---|---|---|
| Madoka（host） | `codex/relay-source-foundation` | `25f58c0` | `relay_no_local_candidates` 改为只在通话失败时输出 |
| Homura（gateway） | `codex/relay-source-foundation` | `9b18435` | 同上 |
| SoulGem（服务端） | `codex/turn-source-api` | `6102a87` | 未改动 |
| GriefSeed（Worker） | `codex/worker-turn-service` | `2c2062a` | 未改动 |

测试 APK 由临时 CI 分支产出，只在 PR head 上追加 workflow，**未改产品代码**：

| 仓库 | 构建分支 | 构建提交 | 产物 |
|---|---|---|---|
| Homura | `ci/turn-acceptance-build-3` | `06b2fe4` | normal + relay |
| Madoka | `ci/turn-acceptance-build-3` | `427afc7` | normal + relay |

设备上实际运行的 APK（`pm path` 取路径后 `exec-out cat` 算 sha256，前 16 位）：
`emulator-5554`（gateway）= `5a8fa94eb4ed6624`，`emulator-5556`（host）= `45fbec0fc71bcb04`。
`FORCE_RTC_RELAY` 用 build-tools `dexdump` 读 `BuildConfig` 确认：relay 变体为 `true`，normal 为 `false`。

## 2. 诊断修复验证（`relay_no_local_candidates`）

修复前该事件在**成功通话**中也会输出，属诊断误报。修复后要同时满足两条判据，缺一不可——
只验证「成功通话不出现」是不够的，把事件整个删掉也能通过。

### 2.1 成功通话不得误报

nodeA 作当前节点（内置 TURN，3478），两侧强制中继。

| 用例 | 方向 | 接通耗时 | 双端选路 | 该事件次数 |
|---|---|---|---|---|
| D1-ok | 外呼 | 3393 ms | `relay/relay` | **0** |
| D2-ok | 来电 | 6899 ms | `relay/relay` | **0** |
| D3-ok | 外呼 | 2251 ms | `relay/relay` | **0** |

按 `call_id` 逐条检索两端 logcat，三通共 307 行相关日志，该事件出现 **0 次**。

### 2.2 确实没有 TURN 时必须准确报出

TURN 地址来自节点容器自身的 `rtc.options`（`internal/httpapi/rtc_media.go` 在服务端组装
`ice_servers`），**不随客户端来源开关变化**，所以「关闭所有来源」无法让当前节点失去 TURN。
负例必须让无 TURN 的节点成为**当前节点**：把 gateway 与 host 都重新配对到 nodeB（`8082`，无 `turn.conf`），
并确认两端外部来源列表为空。

结果（`call_id=1ea5f2c4…`，`post_host_phase=FAILED`）：

```
gateway: event=relay_no_local_candidates reason=no_turn_servers_configured force_relay=true
         local_candidates=0 external_sources_configured=0 external_sources_enabled=0
         external_sources_selected=0 node_sources_configured=0 node_servers_configured=0
         node_enabled=true selected_turn_urls=0
host:    event=failed reason=server_authorization_closed
```

### 结论

两条判据同时满足，**修复通过**。成功通话不再误报，真正缺 TURN 时仍准确报出并给出区分性 `reason`
（`no_turn_servers_configured` 与 `media_signal_or_native_failure` 可区分）。
详见 `relay-diagnostics-fix-20261115.md`。

## 3. C5 多来源合并截断

第三轮已确认**单来源**不存在「超 32 截断」路径（32 通过 / 33 整条拒绝），并留下「`relay_sources_truncated`
未观察到」这一空缺。本轮补齐。

来源列表：节点来源 2 条 + 静态来源 `c5-a`、`c5-b`，每条 20 个地址（合计 40 > `MAX_URLS=32`）。

| 用例 | 来源状态 | host `sources=` | `relay_sources_truncated` | 通话 |
|---|---|---|---|---|
| C5-trunc | `c5-a` + `c5-b` | **3** | **出现** | 接通 `relay/relay`，3519 帧解码 |
| F2-revoke | 移除 `c5-b` | **2** | 不出现 | 接通 `relay/relay` |

`event=relay_sources_resolved sources=3` 之后紧随 `role=host event=relay_sources_truncated`。
**截断只跳过超额来源，不影响可通话性**——该次通话正常接通并完成音频解码。

结论：**通过**。截断打点确实存在、语义正确，只是单来源路径永远走不到它。
第三轮「未观察到」一项就此关闭。

## 4. F2 来源吊销

移除 `c5-b` 后打下一通电话：`sources` 由 3 降为 2，不再触发截断，通话仍 `relay/relay` 接通。
被移除的来源不再参与选路，**吊销在下一通电话即生效**。

结论：**通过**。详见 `c5-f2-multi-source-20261115.md`。

## 5. E1（main APK + PR 版服务端）— 按决定跳过

**本轮按 Kiri 决定不执行**（不新建 `ci/main-lab-runner` 临时分支、不改 `abiFilters`）。

记录上一轮的环境结论，并更正其中一处措辞：

- `origin/main` 的 `app/build.gradle.kts` 只有 `ndk { abiFilters += "arm64-v8a" }`，
  没有 `rtcEmulator` 开关，`-PrtcEmulator=true` 无效，产物装不进 x86_64 AVD。
  **这不是产品缺陷**：main 从未支持模拟器 ABI。
- **更正**：上一轮称「main 的 androidTest manifest 未注册 lab runner」不准确。
  `origin/main` 已有 `testInstrumentationRunner = providers.gradleProperty("rtcLabRunner")
  .orElse(AndroidJUnitRunner)`，`app/src/androidTest/` 也已有 `HostJointInstrumentation.kt`、
  `JointLabSupport.kt`。**唯一阻塞是 ABI，不是 runner 注册。**

将来若要补做，两条路径：(a) 在临时分支给 main 加 `rtcEmulator` 的 ABI 开关后重跑（不改 main）；
(b) 用真机跑 E1。

## 6. B2′ 的断网方式（上一轮用的是哪一种）

**上一轮用的是 Android 层断网，不是宿主机层。** 具体见 `lab/net_case.py:33–40`：

- 断网：`svc wifi disable` + `cmd connectivity airplane-mode enable`
- 恢复：反向执行上述两条

不涉及宿主机 `iptables`、不操作容器网络。本轮按要求只补说明，**不重测**。

B2 的原始语义与上一轮结果（沿用，未重跑）：

- **B1** 断网重试间隔增长到分钟级 → 通过，退避序列 `2s→3s→8s→14s→37s→55s→125s→243s→275s→294s`
- **B2** 网络恢复后连接自动恢复 → host 恢复后 0.85–2.3 s 内 `wss_open` / `WSS wake channel connected`；
  gateway 若恢复时正处退避窗口内，**不提前重连**，等到排定时刻才连
  （17:35:33 恢复 → 17:40:40 connected）
- **B3** 节点停机再恢复 → 第三轮通过；**B4** 429 + Retry-After → 阻塞（无法注入 429）

B2 仍是一处**文档与实测不一致**：`docs/relay-source-foundation.md` 写「Homura 立即尝试」，
实测是「到下一个排定时刻立即连」，而非「网络一恢复立即连」。

## 7. 本轮通过项汇总

| 用例 | 结果 | 依据 |
|---|---|---|
| 诊断修复（成功侧） | **通过** | 3 通成功通话，`relay_no_local_candidates` 0 次 |
| 诊断修复（失败侧） | **通过** | 无 TURN 时 `reason=no_turn_servers_configured` |
| C5 多来源合并截断 | **通过** | `sources=3` + `relay_sources_truncated`，通话仍接通 |
| F2 来源吊销 | **通过** | `sources` 3→2，下一通即生效 |
| B2′ 断网方式说明 | **已完成** | Android 层 `svc wifi disable` + airplane-mode |
| E1 | **按决定跳过** | 不建临时分支、不改 ABI |

## 8. 测试资源清单（保留，未删除）

| 资源 | 位置/标识 | 处置 |
|---|---|---|
| 测试节点 | nodeA（8080，含 TURN）、nodeB（8082，无 TURN）、nodeM（8084） | 保留 |
| 测试 coturn | `turn-test` 容器（3480，relay 49300-49399） | 保留 |
| 测试 Worker（GriefSeed） | `2c2062a`，Cloudflare 版本 `5ce552d9…` | 保留 |
| AVD | `emulator-5554`（gateway）、`emulator-5556`（host） | 保留 |
| CI 构建分支 | `ci/turn-acceptance-build-3`（Madoka/Homura） | 保留，未合并 main |
| 新增来源配置 | host 侧静态来源 `c5-a`（20 地址） | 保留 |
| 已吊销设备行 | nodeA `ef36719c`、nodeB `87e00561`/`45264c39` 等 superseded gateway 行 | 置 `revoked`，未删除历史 |

是否删除由 Kiri 决定。

## 9. 本期新增的测试方法学教训

非产品结论，供后续复现者参考：

1. **joint runner 卡在 `service_start` 多半是陈旧 stage 文件，不是 UiAutomation 被占用。**
   `driver.start_runner()` 删 ready/finished/command/result，**不删 `joint-stage.txt`**；
   runner 的 `stage="init"` 是字段初始化、不走 setter，所以上一次运行留下的 `service_start`
   会一直留在那里冒充当前卡点。诊断前先自行删除并断言其不存在，启动后再读回。
2. **协议文件读取必须分类。** 把 `EACCES`、文件不存在、JSON 非法、adb 失败都塌缩成 `None`，
   就是把所有真实原因都变成 `*_runner_ready_timeout`。应分别返回
   `MISSING`/`DENIED`/`INVALID`/`OK`，并先判 fresh `joint-finished.json` 再接受 `ready`。
3. **`am force-stop` 不是无害的 UI 交接动作**，它会杀掉正在运行的 instrumentation 与服务，
   并使包保持 stopped；交接应走协议的 `stop` 命令并等 fresh finished。
4. **指纹缺失的 AVD 上 SIM 永远是 `unverified`**，必须走 runner 的
   `reconfirm_retained_emulator_sim` 分支（`same_sim_verified=true`，保留原 SIM UUID）；
   靠 UI 点「确认所选 SIM」不足以稳定通过，每次 runner 重启都会把它带回 `unverified`。
5. **UI 录入 TURN 地址时 `input text` 会吞 `&` 和 `=`**：`turn:…?transport=udp&ttl=1`
   会落盘成 `…transport=udppsport=udp:…`。需要精确控制地址**条数**时应改用无特殊字符的短地址
   （本用例只关心是否超 32，不关心可达性）。
