# 节点作为独立中继来源

节点公开声明 `ice_servers_v1` 后，已配对的客户端和网关均可用自己的 access token 请求：

```http
GET /v1/rtc/ice-servers
Authorization: Bearer <access token>
```

响应为 `{"ice_servers":[...],"expires_at":"...Z"}`，并设置 `Cache-Control: no-store`。
使用现有 STUN/TURN 配置、保留地址顺序及自定义端口；coturn 凭据按设备签发，最多一小时，
响应截止时间不晚于凭据截止时间。不创建呼叫、不选择媒体，也不开始 30 秒协商计时。

旧客户端继续使用媒体准入里的 `ice_servers`。来源接口支持现有会话刷新及设备撤销：失效会话返回 401；
RTC 未启动时能力为 false、接口返回 503。没有配置 TURN 的纯信令节点可返回空数组。
撤销会话会阻止后续签发；已签发的 coturn 凭据沿用其原有效期，此接口不声称能够主动撤销 coturn allocation。

一体镜像新增 `BUILTIN_TURN_ENABLED=false`：不启动内置 coturn，不要求公网地址探测成功，API 与 worker 继续运行。
默认仍为 true；配置 `RTC_TURN_URLS` 时始终使用外部中继。关闭内置中继后，手机可使用其他节点、独立来源或静态 TURN。

验证覆盖两种设备角色、会话撤销、不创建呼叫、HMAC/截止时间、自定义端口；真实一体镜像测试覆盖默认、关闭和外部 TURN 三种方式。
运行 `python scripts/test_builtin_turn.py` 前构建一体镜像，并以 `ALLINONE_TEST_IMAGE` 指定本地镜像标签。
强制中继媒体通话和不同手机网络下的选路见 [整体验收记录](turn-sources-implementation.md)。
