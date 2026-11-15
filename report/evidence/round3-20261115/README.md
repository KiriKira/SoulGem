# 证据索引（2026-11-15 第三轮补测）

被测提交：SoulGem `6102a87`、Madoka `c67f380`（设备 sha 前 16 `bf649c57`）、Homura `ba8b087`（`0deacf46`）。容器镜像 revision `6102a875…`。

所有文件已脱敏：实验机地址统一记为 `<lab-host>`，不含 token、TURN 用户名/密码、配对码、session 哈希或数据库备份。

## C4 来源置顶

| 文件 | 说明 |
|---|---|
| `pp-C4-final2.json` | 外呼 `C4-final2`，`total_action_to_established_ms=3225`，`call_id=fd4b15c2-…`，两端 `selected_pair=relay/relay`。 |
| `call-C4-final2-1794722501.json` | 同一通的原始呼叫记录。 |
| `pp-C4-final.json` / `call-C4-final-…json` | 前一次复跑（3569 ms），用于交叉确认非偶然。 |
| `c4-turn-allocations.txt` | **决定性证据**：测试 coturn（3480）在 `06:00:07.157–06:00:08.014` 出现 4 次 allocation 递增/递减，时间与 `C4-final2` 的 ICE 阶段吻合；同一时间窗内 nodeA 内置 coturn 的 allocation 计数为 **0**。 |

配置前提（见报告正文）：两端节点来源全部关闭、`static-lab` 唯一置顶，故 `node_enabled=false`、`external_sources_selected=1`、`selected_turn_urls=1`。

## C5 来源内 URL 上限

| 文件 | 说明 |
|---|---|
| `pp-C5-32-out.json` / `call-C5-32-out-…json` | 32 个地址的来源被配置接受后的一次外呼。**注意**：该次运行在 TURN 凭据修复之前，因此 `sources=0` 且通话 `failed`；它只用于支撑「32 个地址被接受、无 `url_budget` 异常」，**不用于支撑通话接通**。 |
| `pp-C5-trunc-out2.json` / `call-C5-trunc-out2-…json` | 第二轮「40 个地址」那次。后续定位为该次输入被测试工具整串截断（实际只写入约 12 个地址），产品行为未被触及。 |

C5 的判定依据是代码事实而非通话结果：`RelaySources.select()` 对单来源以 `require(server.urls.size <= MAX_URLS) { "url_budget" }` 处理，33 个地址时表单保存即抛异常并被 `catch` 成通用提示、不落盘（对话框不关闭）。因此**不存在「超 32 截断」路径**；`relay_sources_truncated` 只可能来自多来源合并超预算。

## P2 非当前节点作为附加来源

拓扑：host 以 nodeB（无内置 TURN）为当前节点、nodeA（3478）为附加来源。

| 文件 | 说明 |
|---|---|
| `pp-P2-B-current.json` | 外呼 `P2-B-current`，`total_action_to_established_ms=2874`，`call_id=482b51f7-…`；host `relay_sources_resolved` 的 `sources=1`、`prep_ms=102`；两端 `selected_pair=relay/relay`。 |
| `call-P2-B-current-1794724890.json` | 同一通的原始呼叫记录。 |
| `gateway-mesh-B-result.json` | gateway 对 nodeB 的 `mesh_add` 结果，`added_gateway_id=87e00561-…`（新 gateway 行，未改任何 `install_id` 字段）。 |
| `host-B-before-mesh.json` | host 刚配对到 B、尚未 `mesh_add` A 时的 status。 |
| `host-mesh-A-result.json` | host 对 nodeA 的 `mesh_add` 结果，`added_node=fc8265a4-…`，`active_node` 仍为 B。 |
| `host-B-plus-A-status.json` | 双节点后的 status：`active_node=B`，`node_health` 中 B `active=true`、A `active=false`。 |
| `p2-host-nodes.xml` | 设备 `host-nodes.xml` 的 `registry_v1`：两项（B `4cf5b36c…` primary、A `fc8265a4…` 带 `node_scope`），`install_id=83488b1a-…` 与设备一致。 |
| `p2-turn-capture-summary.txt` | **承载方证据**：通话期抓包共 33378 包，`udp port 3478` 22278 包、中继端口段 `49160-49259` 11100 包；按目的端口直方图显示 3478 与 49242/49184 等中继端口为主。nodeB 无 TURN 部署，中继必由 nodeA 承载。 |
| `p2-db-final-state.txt` | 两节点最终 active 行：各 1 条 active gateway，`install_id` 均为 `83488b1a-…`；各 1 条 active client（nodeA `3c1d1208…`、nodeB `64b71807…`）。 |
| `p2-device-state.json` | 通话结束后的设备状态：gateway 空闲、host `phase=ENDED` 且 `active_node=B`。 |

## 未纳入本目录的产物

- 数据库备份（`pg_dump`）与逐条回滚 SQL：含 `access_hash`/`refresh_hash` 等敏感列，保留在实验机 `~/turn-acceptance/private/p2-exec/`（0600），不入库。
- `turn.pcap`（5.5 MB 原始抓包）：仅保留在实验机 `/tmp/p2run/`，本目录只放其统计摘要。
- 配对码文件：一次性凭据，不归档。
