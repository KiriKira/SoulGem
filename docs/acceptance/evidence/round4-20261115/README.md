# 证据索引（2026-11-15 第四轮：诊断修复验证 + C5 多来源 / F2）

被测提交：Homura `9b18435`（CI 构建 `06b2fe4`）、Madoka `25f58c0`（CI 构建 `427afc7`）。
设备实际 APK sha256 前 16：`emulator-5554` = `5a8fa94eb4ed6624`，`emulator-5556` = `45fbec0fc71bcb04`。
所有文件已脱敏：不含实验机地址、TURN 凭据、token、配对码或 session 哈希。

## 诊断修复：`relay_no_local_candidates` 只在通话失败时报告

### D-success（nodeA 当前节点，内置 TURN）

| 文件 | 说明 |
|---|---|
| `pp-D1-ok.json` / `call-D1-ok-1794767102.json` | 外呼，3393 ms，双端 `relay/relay`；`relay_no_local_candidates` 出现 0 次。 |
| `pp-D2-ok.json` / `call-D2-ok-1794767217.json` | 来电，6899 ms，双端 `relay/relay`；该事件 0 次。 |
| `pp-D3-ok.json` / `call-D3-ok-1794767343.json` | 外呼，2251 ms，双端 `relay/relay`；该事件 0 次。 |

三通按 `call_id` 检索两端 logcat 共 307 行相关日志，成功通话中该事件出现 **0 次**。

### D-negative（nodeB 当前节点，无 TURN）

| 文件 | 说明 |
|---|---|
| `call-D4-noturn-1794769204.json` | 外呼失败，`post_host_phase=FAILED`。gateway 输出 `event=relay_no_local_candidates reason=no_turn_servers_configured force_relay=true local_candidates=0 node_servers_configured=0 selected_turn_urls=0`；host 为 `reason=server_authorization_closed`。 |

## C5 多来源合并截断 / F2 来源吊销

| 文件 | 说明 |
|---|---|
| `call-C5-trunc-out-1794770376.json` | 两条各 20 地址的静态来源（合并 40 > 32）。host `relay_sources_resolved sources=3`，紧随 `event=relay_sources_truncated`；通话仍 `relay/relay` 接通（3519 帧解码）。**`relay_sources_truncated` 首次实测捕获。** |
| `call-F2-revoke-out-1794770679.json` | 移除其中一条来源后：host `sources=2`，无 truncated 事件，通话仍 `relay/relay` 接通。吊销即时生效。 |

两通的 `gateway_stats`/`host_stats` 均为 `selected_pair=relay/relay`、`relay_sources=current-node`。

判定说明与录入工具的坑见 `docs/acceptance/c5-f2-multi-source-20261115.md`；
诊断修复的完整判据与恢复步骤见 `docs/acceptance/relay-diagnostics-fix-20261115.md`。

## 未在本轮重做的项

E1（main APK + PR 版服务端）仍受 ABI 限制：`origin/main` 的 `abiFilters` 只有 `arm64-v8a`，
无 `rtcEmulator` 开关，装不进 x86_64 AVD。**已核实并更正**：main 并非缺少 lab runner 注册
（`rtcLabRunner` 与 `androidTest/` 下的 runner 源码均已存在），唯一阻塞是 ABI。
B2′（断网恢复）沿用第三轮结论，本轮未重跑。
