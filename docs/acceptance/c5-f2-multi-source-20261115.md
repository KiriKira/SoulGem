# C5 多来源截断与 F2 吊销实测（2026-11-15）

补上 `c5-measurement-20261115.md` 留下的两个空缺：该文件确认了**单来源**不存在截断路径，
但「`relay_sources_truncated` 实际能否被触发」和「来源被吊销后是否立即失效」都还是未观察状态。
本文件用同一个来源列表把两者都测了。

## 被测与环境

- 拓扑：`emulator-5554` = Gateway（Homura），`emulator-5556` = Host（Madoka）；
  当前节点 nodeA（`http://172.31.8.41:8080`，owner `b01f6568`），gateway `590b95c1`，
  client `76741434` 重确认后 `mapping_revision=2`。
- 来源列表（host 侧 `RelaySettingsActivity`）：节点来源 2 条 + 静态来源 `c5-a`、`c5-b`，
  每条静态来源 20 个地址（`turn:h1:3478` … `turn:h20:3478` / `turn:g1:3478` … `turn:g20:3478`），
  合计 40 个地址 > `MAX_URLS=32`。
- 通话均为强制中继外呼（`call_case_v2.py`，hold 15 s）。

## 结果

| 用例 | 来源状态 | host `sources=` | `relay_sources_truncated` | 通话 |
|---|---|---|---|---|
| C5-trunc | `c5-a` + `c5-b`（20+20） | **3** | **出现（首次实测捕获）** | 接通 `relay/relay`，`ENDED` |
| F2-revoke | 移除 `c5-b`，仅 `c5-a` | **2** | 不再出现 | 接通 `relay/relay`，`ENDED` |

- C5-trunc `call_id=693d4c53-9cc0-4d93-9a0e-82382981e6b4`，
  证据 `evidence/call-C5-trunc-out-1794770376.json`；
  host 事件 `event=relay_sources_resolved elapsed_ms=90 sources=3`，
  紧随 `role=host event=relay_sources_truncated`；gateway `sources=0`。
- F2-revoke `call_id=5b552344-c305-4211-94f1-dfe633db62b1`，
  证据 `evidence/call-F2-revoke-out-1794770679.json`；
  host `event=relay_sources_resolved elapsed_ms=293 sources=2`，无 truncated 事件。

## 结论

1. **C5（多来源合并超预算）通过**：`relay_sources_truncated` 确实由「多来源合并总量超 32」触发，
   与 `c5-measurement` 的代码推断一致；截断只跳过超额来源，**不影响可通话性**——
   该次通话仍以 `relay/relay` 接通并完成 3519 帧解码。
2. **F2（来源吊销）通过**：移除一个来源后，下一次通话的 `sources` 由 3 降为 2，
   被移除来源不再参与选路，且不再触发截断。吊销在**下一通电话即生效**。
3. 原先「C5 截断打点未观察到」这一项可以关闭：事件本身存在且语义正确，
   只是单来源路径永远不会走到它。

## 录入工具的两个坑（都已绕过，不影响结论）

1. `input text` 会吞掉 `&` 和 `=`：`turn:…?transport=udp&ttl=1,turn:…` 落盘成
   `turn:…?transport=udppsport=udp:…`。带查询参数的地址无法用这个工具录入，
   因此改用无特殊字符的短地址（`turn:h1:3478`）来精确控制地址**数量**——
   本用例只关心条数是否超过 32，不关心地址是否可达。
2. 对话框里 BACK 会连锁关闭对话框和 Activity（键盘已收起时）。核验字段内容要用
   `uiautomator dump` 读 `text=`，不要靠返回键。
