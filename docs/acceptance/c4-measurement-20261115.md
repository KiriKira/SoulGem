# C4 实测记录（2026-11-15）

被测提交：Madoka `c67f380`（设备 sha 前 16 `bf649c57`）、Homura `ba8b087`（`0deacf46`）；nodeA 镜像 `ghcr.io/kirikira/gsm2sip:sha-6102a87`（revision `6102a875…`）。

## 用例目标

验证「优先使用」（置顶）的来源是否优先于节点下发的 TURN 承载通话。

## 前置条件

两端各自的中继来源列表状态（由 UI 树读取，非人工目测）：

| 设备 | 节点来源 | 附加来源 | 置顶 |
|---|---|---|---|
| Gateway（5554） | `节点 6e8586bb` OFF、`节点 c8eb78d7` OFF | `static-lab` ON | `static-lab`（唯一 checked） |
| Host（5556） | 两个节点来源均 OFF | `static-lab` ON | `static-lab`（唯一 checked） |

`static-lab` 配置：`turn:10.0.2.2:3480`，用户名 `<lab-turn-user>`（静态账号，测试 coturn 专用，非产品凭据）。

节点来源全部关闭 = 节点下发的 TURN 不进入 `ice_servers`，此时唯一可用中继来源就是置顶的 `static-lab`。

## 结果：通过

通话建立 **3225 ms**（另一次复跑 3569 ms），两端均为 `relay/relay`。

### 1. 配置层证据（两端诊断行）

Gateway：
```
event=relay_no_local_candidates reason=turn_candidates_unavailable force_relay=true
  local_candidates=0
  external_sources_configured=1  external_sources_enabled=1  external_sources_selected=1
  node_sources_configured=1      node_servers_configured=2    node_enabled=false
  selected_turn_urls=1
```
Host：
```
event=relay_sources_resolved elapsed_ms=490 sources=1
```
读法：`node_enabled=false` 证明节点 TURN 已被排除；`external_sources_selected=1` 且 `selected_turn_urls=1` 证明置顶来源是唯一进入配置的 TURN URL 集合。

### 2. 媒体层证据（两端 stats）

Gateway：`selected_pair=relay/relay candidate_types=relay tx_packets=4018 rx_packets=3909 pcm_capture=7942 pcm_decoded=7897`
Host：`selected_pair=relay/relay candidate_types=relay tx_packets=3968 rx_packets=4079 pcm_capture=7847 pcm_decoded=7821`

`candidate_types=relay` 说明本端只有 relay 候选，没有 host/srflx——即媒体路径完全依赖 TURN 中继，不存在直连或 srflx 旁路。

### 3. 承载方证据（决定性）

`turn-test` 容器（`coturn/coturn:latest`，`--network host`，3480，verbose）于 **05:59:32** 启动；通话期间 **06:00:07.157–06:00:08.014 产生 4 次 allocation**（`Global turn allocation count incremented, now 1→2→3→4`），随后随通话关闭释放。

同时，nodeA 内置 coturn（3478）在同一时段**没有任何 allocation 记录**。

因此这通 `relay/relay` 通话的中继流量确实由置顶的 `static-lab`（`turn:10.0.2.2:3480`）承载，不是节点下发 TURN，也不是任何未配置的旁路。

## 与上一轮结论的差异

上一轮 C4 记为「通过（带限定）」：置顶生效已确认，但该通 `relay_sources=current-node`，即媒体实际走节点下发的 TURN，「置顶来源优先承载」未被证明。

本轮补上了这个缺口：把节点来源全部关闭后，置顶来源成为唯一可用 TURN，通话依然以 `relay/relay` 建立并有双向媒体，且 allocation 计数落在测试 TURN 上。**「置顶来源优先承载」现已证明。**

本轮同时修正了上一轮 C5 的判定（详见 `c5-measurement-20261115.md`）：单来源不存在「超 32 截断」路径，32 通过 / 33 拒绝。

## 环境备注（验收范围之外，影响后续用例）

- 两端 `static-lab` 使用测试 coturn 的静态账号 `<lab-turn-user>`，该账号只存在于 `turn-test` 容器，不是产品凭据。
- nodeA 内置 coturn 用 `use-auth-secret` + `static-auth-secret`（TURN-REST）。服务端的原生签发入口是 `GET /v1/rtc/ice-servers`（`internal/httpapi/rtc_media.go:440`），`username = "<expiry>:<deviceID>"`，凭据由服务端 HMAC 生成，客户端缓存跨通话复用。手工用 secret 自行计算属于绕路，本轮已放弃该做法。
- **nodeA 的 install_id 已变为 `83488b1a-f665-4851-8712-0c7f50f61e9b`，nodeB 仍为 `dcd30923-030e-4018-ac9a-65fbc99f7717`。** 早前「两库 install_id 相同」的结论已被本轮只读核对推翻（期间 AVD 重启/重装改变了它）。这意味着 P2 中 `HostNodes.add` 拒绝第二服务器是**真实的环境不一致**，不能算产品缺陷。
