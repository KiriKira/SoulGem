# C5 实测记录（2026-11-15）

被测提交：Madoka `c67f380`（设备 sha256 前16 `bf649c57`）、Homura `ba8b087`（`0deacf46`）。
拓扑：`emulator-5554` = Gateway（Homura），`emulator-5556` = Host（Madoka）；nodeA 单节点（owner `b01f6568`，gateway `c8eb78d7`）。
测量手段：关掉两个节点来源的 Switch（使 `nodeRelayPolicy.first=false`，节点 TURN 不进 ice_servers），只留被测静态来源，然后打一通强制中继外呼。此时 `RelayGatheringDiagnostics` 输出的 `selected_turn_urls=` 就等于该来源实际进入配置的 TURN 地址数。

## 结果

| 档位 | 表单提交 | 来源是否落盘 | 通话 | `selected_turn_urls` |
|---|---|---|---|---|
| 40 个地址（被截断的旧记录） | 接受 | 落盘，但实际只有 12 个 | 接通 `relay/relay` | 12 |
| 32 个地址 | 接受 | 落盘（`c5-32`） | 未建立（全部地址不可达，符合预期） | **32** |
| 33 个地址 | **拒绝**：对话框不关闭、字段保留、列表无新行 | 否 | 未测 | — |

## 关键代码事实（`git show c67f380:<path>`）

1. `rtcmedia/.../RelaySettingsActivity.kt:54`
   ```kotlin
   val urls=url.text.toString().split(',').map{it.trim()}
   val server=RelayIceServer(urls,user.text.toString(),password.text.toString())
   ...
   RelaySources.select(emptyList(),listOf(RelaySource(id,listOf(server))))
   ```
   **多个逗号分隔地址被存成一个 server 的多个 URL**，不是多个 server。

2. `rtcmedia/.../RelaySources.kt`
   ```kotlin
   require(server.urls.isNotEmpty() && server.urls.size <= MAX_URLS) { "url_budget" }
   ```
   `MAX_URLS = 32`。单来源超限是**硬失败**（抛 `IllegalArgumentException: url_budget`），**不是截断**。

3. 保存路径在**表单提交时**就调用 `select()` 做校验，异常被 `catch(_:Exception)` 吞掉，只弹通用提示：
   ```kotlin
   }catch(_:Exception){message("请检查 TURN 地址、用户名和密码")}
   ```
   所以 33 个地址的表现是「对话框不关、字段留着、没有任何具体原因」。**上限 32、URL 非法、凭据缺失三种情况给的是同一句话。**

4. `truncated` 只来自多来源合并超预算：
   ```kotlin
   if(groups>MAX_SERVERS||reserved.size>MAX_URLS)return select(node).truncated()
   ...
   if(proposedGroups>MAX_SERVERS||proposed.size>MAX_URLS){truncated=true;continue}
   ```
   对应事件在 `RtcAudioSession.kt:111`：`if(relaySelection.truncated)event("relay_sources_truncated")`。

## 结论

- C5 的真实行为是**单来源超过 32 个地址整条拒绝、不截断**。任务原本设想的「超 32 截断」在当前实现里不存在。
- `relay_sources_truncated` **不可能由单来源触发**，只能来自多来源合并时总预算不够而跳过某个来源。
- 因此此前两轮报告里把 C5 记为「通过（带限定）」和「表单接受但未持久化」都不准确，应改为：「32 通过 / 33 拒绝，边界确认；无截断路径」。

## 产品观察（非缺陷结论，供评审判断）

超限时只给一句「请检查 TURN 地址、用户名和密码」，不说明是数量上限。用户无法区分「地址格式错」「密码错」「地址太多」。是否要改进属产品决策，本轮不改代码。

## 测试工具问题（已修）

`lab/add_static.py` 原来把整串地址一次 `input text` 灌进去，几百字符会被 shell 截断——40 个地址实际只进去 12 个。这解释了两轮前「表单接受但未持久化」的假象：那次落盘的其实是 12 地址的来源。已改为按 160 字符分块输入。
