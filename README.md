<p align="center">
  <img src="icon.png" width="112" alt="SoulGem">
</p>
<h1 align="center">SoulGem</h1>
<p align="center"><b>一条 Docker 命令拉起你的私人通信中枢，串联主力机与 SIM 网关。</b></p>
<p align="center">
  <a href="#-快速部署">快速部署</a> ·
  <a href="#-本地开发">本地开发</a> ·
  <a href="docs/architecture.md">系统架构</a> ·
  <a href="docs/release-0.4.md">0.4 发布说明</a>
</p>
<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Docker-All--in--One-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/badge/Version-1.0.1-ff69b4?style=flat-square" alt="Version">
  <img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="MIT License">
</p>

**SoulGem** 是 MahoGSM 系列的核心服务端。它用 Go 和 PostgreSQL 管住所有设备的配对、短信收发队列和 WebRTC 通话信令，把 [Homura](https://github.com/KiriKira/Homura)（备用机网关端）和一台或多台 [Madoka](https://github.com/KiriKira/Madoka)（主力手机端）安全地连在一起。

## ✨ 核心能力

- **一条命令开箱即用：** 提供预构建一体镜像 `ghcr.io/kirikira/gsm2sip`（内置 PostgreSQL 与 TURN 中继），无需服务器编译，支持 amd64 / arm64。
- **一主多端，互不干扰：** 一个账号可配对多台主力设备与网关设备，各自拥有独立会话和设备标签；一次性配对码只用一次、15 分钟自动过期。
- **短信不重发、不漏同步：** 网关事件在数据库事务提交后才确认，主力机按游标安全拉取；即使从备份恢复，历史待发短信也不会被误触重发。
- **WebRTC 通话信令与抢接：** 来电同时呼叫所有在线的主力机，谁先接听由全局账本拍板，其余设备自动停振；已全面采用 WebRTC，彻底告别 Asterisk 和 SIP。
- **省电唤醒 + 多节点容灾：** 支持 Reception v2 按来电/短信分领域推送唤醒，也可多台 SoulGem（或搭配 [GriefSeed](https://github.com/KiriKira/GriefSeed)）组成多节点互相兜底。

## 🐳 快速部署

服务器上不用编译，直接跑一体镜像（内置 PostgreSQL 与 TURN，支持 amd64 / arm64）：

```sh
docker run -d --name gsm2sip --restart unless-stopped --network host \
  -v gsm2sip-data:/data ghcr.io/kirikira/gsm2sip:main
curl http://127.0.0.1:8080/readyz
```

生成一条两部手机都能扫的加入链接和二维码（15 分钟有效，没有账户时自动创建）：

```sh
docker exec gsm2sip mahogsm-admin join create --url https://你的服务器地址
```

用备用机（Homura）和主力机（Madoka）各扫一次，分别认领身份并确认 SIM 卡即可开始使用。

**部署要点：**

- **端口：** API 为 `8080`；内置 TURN 用 `3478`（UDP/TCP）与 `49160-49259/udp`，云安全组或路由器要放行。探测不到公网地址时设 `-e PUBLIC_IP=...`；不用 `--network host` 时的端口映射写法见 [部署指南](docs/deploy-images.md)。
- **HTTPS：** 公网部署在前面放一个反向代理（如 Caddy）转发到 `127.0.0.1:8080`；`http://` 只建议在局域网或 Tailscale 内使用（通话音频始终是 DTLS-SRTP 加密）。
- **密钥与备份：** 首次启动会在卷内 `/data/secrets/` 生成数据库密码和 `SECRETS_ENCRYPTION_KEY`，后者之后不能更换。备份整个数据卷，并另存一份 `docker exec gsm2sip cat /data/secrets/secrets_encryption_key`。
- **升级：** `docker pull ghcr.io/kirikira/gsm2sip:main`，再用同样参数删除并重建容器（数据在卷里）。也可用 `vX.Y.Z` 标签固定版本。
- **可选：** 已有外部 TURN 时设 `RTC_TURN_URLS` 与 `RTC_TURN_SHARED_SECRET`；多台节点互为备份时用 `join create --also`，见 [多节点 Mesh](docs/mesh.md)。分镜像 Compose 部署也见 [部署指南](docs/deploy-images.md)。

## 🛠️ 本地开发

本地开发需要 Go 1.24+ 与 PostgreSQL 17（也可 `cp .env.example .env` 后用仓库根目录的 `docker compose up --build -d` 起开发环境，它不是公网生产配置）：

```sh
export DATABASE_URL='postgresql://USER:PASSWORD@127.0.0.1:5432/gsm2sip?sslmode=disable'
go test ./...                     # 设 TEST_DATABASE_URL 时会跑 PostgreSQL 集成测试
go run ./cmd/api                  # 另有 ./cmd/worker 与 ./cmd/admin
```

更多文档：[三端架构总览](docs/architecture.md) · [运维与备份恢复](docs/operations.md#postgresql-backup-and-isolated-restore) · [RTC 呼叫控制](docs/rtc-call-control-v1.md) · [Reception v2](docs/reception-v2.md) · [OpenAPI](openapi/openapi.yaml) · [发布说明](docs/releases.md)

## 📄 许可证

[MIT](LICENSE)
