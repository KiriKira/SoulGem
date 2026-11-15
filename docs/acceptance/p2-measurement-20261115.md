# P2 实测记录：非当前节点的 TURN 作为附加来源（2026-11-15）

被测提交：Madoka `c67f380`（设备 sha 前 16 `bf649c57`）、Homura `ba8b087`（`0deacf46`）；nodeA/nodeB 镜像 `ghcr.io/kirikira/gsm2sip:sha-6102a87`（revision `6102a875…`）。

## 用例目标

验证「**非当前节点**的 TURN」能作为附加来源被选中并承载媒体，而不是只有节点下发的 TURN 能用。

## 为什么必须用这个拓扑

nodeB **没有内置 TURN**（只有 HTTP 8082 + PG 55434）。因此「A 为当前节点 + B 为附加来源」即使得到 `sources=1`，该来源的 `ice_servers` 为空，只能证明空来源进入了 resolve，**不能证明它承载中继**。

本轮采用唯一能出强证据的拓扑：

| 角色 | 当前节点（本通讯令） | 附加来源（中继） |
|---|---|---|
| Host（Madoka，5556） | nodeB（无 TURN） | nodeA（有 TURN 3478） |
| Gateway（Homura，5554） | 经 nodeB 的 mesh 会话接收本通 | nodeA（主配对 A） |

## 前置修复：install_id 对齐

进入本例前两库 `install_id` 不一致：nodeB 那行是陈旧注册（`dcd30923-…`，`last_seen_at` 停在 2026-11-14 17:04），当前设备与 nodeA 是 `83488b1a-f665-4851-8712-0c7f50f61e9b`。

**未使用 `UPDATE gateways.install_id` 伪造一致。** 做法是让当前 gateway 真正 `mesh_add` 到 nodeB：

- 在 nodeB 签发 `--role gateway --ttl 10m` 的角色码
- gateway 执行 `mesh_add {api_url: http://<lab-host>:8082, pairing_code: <一次性码>}`
- 得到**新** gateway 行 `87e00561-aac1-49c4-aa7d-2df34d6c49f5`，等待 heartbeat 后其 `install_id` 上报为 `83488b1a-…`，`last_seen_at` 刷新

随后按显式 UUID 清单精确吊销旧 active 行（带 `all_targets_match` 断言与逐条回滚 SQL，回滚文件保存在实验机私有目录）。吊销后每节点恰好 1 条 active gateway、0 条多余 active client。

## Host 切换为以 B 为当前节点

1. 用产品 UI「设置 → 解除此手机配对 → 确认」正常解除 host 与 nodeA 的配对。**服务端效果已核实**：nodeA 上该 client（`9dbb81ad-…`）的 `sessions` 行被删除，仅余 `devices.state=active` 而无凭据。
2. host 用 nodeB 签发的 `--role client` 码正常重新配对到 B（`reuse=False`），runner READY。
3. host 再 `mesh_add` nodeA（nodeA 签发 `--role client` 码），得到新 client `3c1d1208-b607-4941-8296-e99cc0fb750d`。

设备 `host-nodes.xml` 的 `registry_v1` 随后含两项，`install_id=83488b1a-…`（与设备一致）：

- `4cf5b36c-…` → `http://<lab-host>:8082/v1`（B，primary）
- `fc8265a4-…` → `http://<lab-host>:8080/v1`（A，`node_scope=node-fc8265a4`）

`status.active_node = http://<lab-host>:8082/v1`（= B），`node_health` 中 B `active=true`、A `active=false`（A 是附加来源，非当前）。

## 来源配置

两端 `static-lab` 及其他静态/外部来源全部关闭、无置顶；节点来源开启。这与 C4 的「节点全关 + static-lab 唯一置顶」互为反向对照。

## 结果：通过

外呼建立 **2874 ms**，双端 `relay/relay`，`host_sources=1`、`gateway_sources=1`，`call_id=482b51f7-ba0a-4191-b944-6ae8c037df28`。

### 1. 来源归属（关键）

| 端 | `relay_sources` | 含义 |
|---|---|---|
| Gateway | `node-c8eb78d7-553b-4169-991a-ab3e97536edf` | nodeA 的 gateway（本通当前节点是 B，故 A 为附加来源） |
| Host | `node-3c1d1208-b607-4941-8296-e99cc0fb750d` | nodeA 上新签发的 client（即 host 对 A 的附加来源） |

两端 `candidate_types=relay`，无 host/srflx 旁路。

### 2. 媒体证据

- Gateway：`tx_packets=4012 rx_packets=3886 pcm_capture=7924 pcm_decoded=7877 pcm_underruns=0`
- Host：`tx_packets=3937 rx_packets=4062 pcm_capture=7786 pcm_decoded=7743 pcm_underruns=0`

### 3. 承载方证据（决定性，抓包）

通话期间在实验机对 TURN 相关流量抓包，共 **33378 包**：

- `udp port 3478`：**22278 包**（TURN 控制/通道）
- `udp portrange 49160-49259`：**11100 包**（nodeA coturn 的中继端口段）

nodeB 无 TURN 部署，故这些中继流量只能来自 nodeA。两端 `relay/relay` 且 `candidate_types=relay`，说明媒体确实经 nodeA 的 TURN 中继，而非直连或节点下发。

### 4. 服务端最终状态

| 节点 | active gateway | install_id | active client |
|---|---|---|---|
| nodeA | `c8eb78d7-553b-4169-991a-ab3e97536edf` | `83488b1a-f665-4851-8712-0c7f50f61e9b` | `3c1d1208-b607-4941-8296-e99cc0fb750d` |
| nodeB | `87e00561-aac1-49c4-aa7d-2df34d6c49f5` | `83488b1a-f665-4851-8712-0c7f50f61e9b` | `64b71807-4759-4483-aa8f-5cb84569d55a` |

两节点 gateway 的 `install_id` 与设备真实值三方一致。

## 与旧结论的差异

- 旧报告记「P2 属环境限制、拓扑无法建立、`sources=0`」。**该结论作废**：真实原因是 nodeB 的 gateway 行陈旧，`mesh_add` 对齐 `install_id` 后即成立。
- 旧报告记 P2 六通全部 `sources=0`。本轮为 `sources=1`（两端），且 `relay_sources` 明确指向 nodeA。

## 副作用与偏差

- **副作用**：host 被解除配对后重新配对到 nodeB；旧 host session 已被撤销（`sessions` 行删除），同步的短信/网关缓存被清空。原 `C4` 阶段的 host-on-A 配对不再存在。
- **偏差**：未执行手册 4.3.1 的「移除旧 B mesh 项」——执行时设备 `mesh_nodes` 表为空（UI 上的节点行只是中继来源预置项，不是 mesh 注册），无可移除项。旧 active 行的清理改由第 6 节的显式 UUID 清单完成，而非全表清理器。
- 未重启 AVD、未重装 APK、未在实验机编译、未修改任何容器配置。

## 环境遗留（供后续决定）

- 当前拓扑为「host 以 B 为当前节点 + A 附加」。若后续用例需要 host 回到 A，须重新配对或走 `mesh_add`，不能假设自动恢复。
- 测试用 `turn-test` 容器（3480）仍在运行，属 C4 用测试设施。
- 数据库备份与本轮回滚文件保存在实验机 `~/turn-acceptance/private/p2-exec/`（0600）。
