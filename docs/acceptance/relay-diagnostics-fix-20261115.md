# 诊断修复验证：`relay_no_local_candidates`（2026-11-15）

## 修复

| 仓库 | 修复提交 | CI 构建提交 |
|---|---|---|
| Homura（gateway） | `9b18435` fix(rtc): report missing relay candidates only when a call fails | `06b2fe4` |
| Madoka（host） | `25f58c0` 同上 | `427afc7` |

设备上实际运行的 APK（`pm path` + `exec-out cat` 后 sha256 前 16 位）：
`emulator-5554` = `5a8fa94eb4ed6624`，`emulator-5556` = `45fbec0fc71bcb04`。

## 判据

修复前：该事件在**成功通话**中也会输出，属诊断误报。
修复后应满足两条，缺一不可：

1. **成功通话不得出现** `relay_no_local_candidates`。
2. **确实没有可用 TURN 时**必须出现，且带 `reason=no_turn_servers_configured`。

只验证第 1 条不足以说明修复有效——把事件整个删掉也能通过。

## D-success（nodeA：内置 TURN，双端 `relay/relay`）

| 用例 | 方向 | 接通耗时 | 双端选路 | 两端含 call_id 日志行数 | `relay_no_local_candidates` |
|---|---|---|---|---|---|
| D1-ok | 外呼 | 3393 ms | `relay/relay` | 67 / 41 | **0** |
| D2-ok | 来电 | 6899 ms | `relay/relay` | 54 / 32 | **0** |
| D3-ok | 外呼 | 2251 ms | `relay/relay` | 71 / 42 | **0** |

按 call_id 逐条检索 logcat，三通共 307 行相关日志，该事件出现 **0 次**。
（`call_id`：`2f5c4071…`、`9a89ecbc…`、`df1b84fb…`；证据 `evidence/pp-D{1,2,3}-ok.json`。）

## D-negative（nodeB：无 TURN、无外部来源，强制中继应失败）

拓扑切换：gateway 与 host 都重新配对到 nodeB（`8082`，owner `53728ba1`，gateway `df049676`）。
前置核实：nodeB 容器 `/run/gsm2sip/turn.conf` 不存在；两端外部来源列表为空。

`call_id=1ea5f2c4-1e0c-49ed-8c15-5a1acb7ca52b`，`post_host_phase=FAILED`。
gateway 输出：

```
event=failed reason=media_signal_or_native_failure
event=relay_no_local_candidates reason=no_turn_servers_configured force_relay=true
  local_candidates=0 external_sources_configured=0 external_sources_enabled=0
  external_sources_selected=0 node_sources_configured=0 node_servers_configured=0
  node_enabled=true selected_turn_urls=0
```

host 侧为 `event=failed reason=server_authorization_closed`（服务端因媒体失败关闭通话）。

## 结论

两条判据同时满足，修复**通过**：
成功通话不再误报该诊断，真正缺少 TURN 时仍准确报出并给出区分性 reason。

## 环境副作用与恢复（供复现者注意）

1. 指纹缺失的 AVD 上 SIM 永远是 `unverified`，host 会以
   「SIM N 需在旧手机上确认后才能使用」禁用拨号。必须走 runner 的
   `reconfirm_retained_emulator_sim` 分支（`same_sim_verified=true`，保留原 SIM UUID），
   靠 UI 点「确认所选 SIM」不足以稳定通过——每次 runner 重启都会把服务端状态带回 `unverified`。
2. gateway 只能对**当前配对节点**心跳。用 mesh 把 gateway 挂到第二节点时，
   该节点上的 gateway 行不会 `online`，host 的节点探测因此判「旧手机不在线」，
   不能靠 mesh 实现「B 作当前节点」——必须真正重新配对两端。
3. 重配对会产生新的 gateway 行而旧行仍 `active`。清理时必须在同一事务里加
   目标行数与预期一致的 assert，再精确置 `revoked`，不要按角色批量更新
   （nodeA/nodeB 上残留过多条 active gateway 行，`devices.id` 为 uuid，
   与 text 比较会报 `operator does not exist`，需显式 `::uuid`）。
4. 来源配置用应用内 AES 加密（`rtc-relay-sources.xml`），外部无法写入；
   且重装/恢复备份会让加密密钥失配，界面显示「来源存储不可读：清除后重新添加」，
   必须清除后经 UI 重新添加。
