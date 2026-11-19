<p align="center">
  <img src="icon.png" width="112" alt="SoulGem">
</p>
<h1 align="center">SoulGem</h1>
<p align="center"><b>一条 Docker 命令拉起你的私人通信中枢，串联主力机与 SIM 网关。</b></p>
<p align="center">
  <a href="#-快速部署">快速部署</a> ·
  <a href="#-本地开发与更多文档">本地开发</a> ·
  <a href="docs/architecture.md">系统架构</a> ·
  <a href="docs/release-0.4.md">0.4 发布说明</a>
</p>
<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Docker-All--in--One-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/badge/Version-1.0.1-ff69b4?style=flat-square" alt="Version">
</p>

**SoulGem** 是 MahoGSM 系列的核心服务端。它用 Go 和 PostgreSQL 管住所有设备的配对、短信收发队列和 WebRTC 通话信令，把 [Homura](https://github.com/KiriKira/Homura)（备用机网关端）和一台或多台 [Madoka](https://github.com/KiriKira/Madoka)（主力手机端）安全地连在一起。

## ✨ 核心能力

- **一条命令开箱即用：** 提供预构建一体镜像 `ghcr.io/kirikira/gsm2sip`（内置 PostgreSQL 与 TURN 中继），无需服务器编译，支持 amd64 / arm64。
- **一主多端，互不干扰：** 一个账号可配对多台主力设备与网关设备，各自拥有独立会话和设备标签；一次性配对码只用一次、15 分钟自动过期。
- **短信不重发、不漏同步：** 网关事件在数据库事务提交后才确认，主力机按游标安全拉取；即使从备份恢复，历史待发短信也不会被误触重发。
- **WebRTC 通话信令与抢接：** 来电同时呼叫所有在线的主力机，谁先接听由全局账本拍板，其余设备自动停振；已全面采用 WebRTC，彻底告别 Asterisk 和 SIP。
- **省电唤醒 + 多节点容灾：** 支持 Reception v2 按来电/短信分领域推送唤醒，也可多台 SoulGem（或搭配 [GriefSeed](https://github.com/KiriKira/GriefSeed)）组成多节点互相兜底。

## 🐳 快速部署

推荐使用预构建一体镜像或 Docker Compose（自带 Caddy 反代与内部 PostgreSQL）：

```sh
cp .env.example .env
# 按 docs/operations.md 生成并填写加密密钥与本地数据库密码
docker compose up -d
```

服务启动后，生成一条两部手机都能扫的加入链接和二维码：

```sh
docker compose run --rm --no-deps --entrypoint /usr/local/bin/mahogsm-admin api \
  join create --url https://你的服务器地址
```

用备用机（Homura）和主力机（Madoka）各扫一次，分别认领身份并确认 SIM 卡即可开始使用。公网部署建议配合 Caddy 或反向代理启用 HTTPS / WSS。

<details id="-本地开发与更多文档">
<summary>🛠️ 本地 Go 开发、备份与架构文档</summary>

本地开发需要 Go 1.24+ 与 PostgreSQL 17：

```sh
export DATABASE_URL='postgresql://USER:PASSWORD@127.0.0.1:5432/gsm2sip?sslmode=disable'
go test ./...
go run ./cmd/api
```

- [预构建镜像部署指南](docs/deploy-images.md) · [PostgreSQL 备份与隔离恢复](docs/operations.md#postgresql-backup-and-isolated-restore)
- [三端架构总览](docs/architecture.md) · [多节点 Mesh](docs/mesh.md) · [RTC 呼叫控制](docs/rtc-call-control-v1.md) · [Reception v2](docs/reception-v2.md)
- [0.4 发布说明](docs/release-0.4.md) · [0.4 三端联合测试计划](docs/plan-0.4-joint-test.md)

</details>
