# AGENTS.md — hysterical-panel

面向 AI 协作者的项目概要与开发注意事项。动手前先读完本文件。

## 这是什么

一个**轻量级 Hysteria 2 管理面板**。当前只有后端（`./backend`），前端由人类单独开发。

后端职责边界（务必遵守，不要擅自扩张）：
- 保存 Hysteria 节点的**接口信息**（API 地址 + secret），不部署节点、不管理服务器。
- 自主轮询采集各节点流量，做用户级 / 节点级聚合。
- 用户管理 + 实时诊断面板。
- **不做**：订阅、用量计费、账号踢出、节点部署。这些是明确排除项，需求方已确认。

## 核心架构决策（不要推翻，除非需求方明确要求）

1. **两层模型，没有 Account 抽象。**
   `users` 既是登录面板的人，也是 Hysteria 认证的账号。Hysteria 的 auth key 存在 `users.auth_string`，与登录用的 `email` **完全独立**——改 Hysteria 账号名不应影响登录。不要把 email 当 auth key 用。

2. **节点对用户的可见性全员开放。**
   所有 `enabled` 节点默认对所有用户生效。这个逻辑收口在 `internal/api/api.go` 的 `nodesForUser(userID)` 函数里——目前返回全部 enabled 节点。**将来要做用户组，只改这一个函数**，不要在 handler 或采集器里散落过滤逻辑。

3. **采集走 counter 模式，不用 `/traffic?clear=1`。**
   多节点对账场景下，丢量比逻辑复杂更可怕。counter 模式漏采一轮不丢量。详见下方采集器说明。

4. **节点接口走自定义 Go 路由，不裸用 PocketBase 自动 API。**
   纯粹为了在返回前剥掉 `api_secret`。PocketBase 自动 collection API 会把字段全返回，因此 nodes/users 一律走 `/api/panel/*`。

5. **角色当前只有 `admin` / `user`。**
   `admin` 可管理节点和用户；`user` 只能查看自己的账号详情、用量和实时诊断。新增管理接口默认走 `requireAdmin`；新增用户自查接口才走 admin-or-self 守卫。

6. **`status` 是用户启停的单一来源，且真正生效。**
   `active`/`disabled` 两态。落地在两处：登录鉴权 `bindAuthGate`（`OnRecordAuthRequest("users")`，非 active 一律 403 `Account is disabled`，覆盖登录与 token 刷新）；采集器 `pollNode` 里非 active 用户**仍推进 cursor 但不计量**（避免重新启用时把停用期间的 counter 一次性灌进单个 bucket）。注意：面板不踢 Hysteria 连接，`disabled` 只挡面板登录 + 停止记账，不会断流。写 `status` 经 `validUserStatus` 校验。

## 技术栈与版本

- Go（go.mod 锁 1.26.2，但语言特性按 1.21+ 写）。
- **PocketBase v0.23.12**，以 **framework 方式**引入（不是当二进制用）。
  - 用的是 v0.23 的 API：`core.App`、`app.OnServe().BindFunc`、`core.RequestEvent`、`core.NewBaseCollection`、`se.Router.Group(...).Bind(...)`、`hook.Handler[*core.RequestEvent]`。
  - 升级 PocketBase 大版本前务必查 API 变更，0.22→0.23 改动很大。
- SQLite（PocketBase 自带 modernc 驱动，纯 Go，无 CGO）。

## 目录结构

```
hysterical-panel/
├── .AGENTS.md                  本文件
└── backend/
    ├── main.go                 启动：注册 migration + 采集器 + 自定义路由
    ├── go.mod / go.sum         module 名为 hysterical-panel
    ├── README.md               面向人类的运行说明 + 接口表
    ├── migrations/             代码式迁移，启动自动应用
    │   ├── 1730000001_extend_users.go      扩展内置 users
    │   ├── 1730000002_create_nodes.go
    │   ├── 1730000003_create_traffic_cursor.go
    │   ├── 1730000004_create_traffic_hourly.go
    │   ├── 1730000005_create_traffic_daily.go
    │   ├── 1730000006_drop_users_enabled.go 移除冗余 users.enabled
    │   └── 1730000007_add_user_role.go     新增普通 user 角色
    ├── .env.example / .envrc   环境变量模板 + direnv（见 README）
    └── internal/
        ├── config/             环境变量（caarlos0/env）
        ├── cryptobox/          AES-GCM 加解密节点 secret
        ├── hysteria/           Traffic Stats API 客户端
        ├── collector/          counter-to-delta 采集核心
        └── api/                /api/panel 路由
            ├── api.go          路由注册 + 鉴权中间件 + 脱敏辅助 + nodesForUser
            ├── nodes.go        节点 CRUD + 连通性测试
            ├── users.go        用户 CRUD
            ├── traffic.go      用量 summary / series
            └── live.go         实时诊断（重点）
```

## 数据模型

`users`（扩展 PocketBase 内置 auth collection）：
- `auth_string` (text, unique, required) — Hysteria auth key，= /traffic 返回的 key
- `role` (select [admin, user])、`status` (select [active, disabled]) — `status` 是用户启停的单一来源（active = 启用）
- `quota_bytes`、`used_tx`、`used_rx` (number, int64) — quota 当前不计费，仅留字段

`nodes`：
- `name`、`api_url` (url)、`api_secret` (text, **AES-GCM 加密存储**)
- `poll_interval` (number, 秒, 默认 30)、`enabled` (bool)
- `last_polled_at` (date)、`last_error` (text) — 用于 health 判断

`traffic_cursor` (user+node 唯一)：`last_tx`、`last_rx` —— counter-to-delta 的游标
`traffic_hourly` / `traffic_daily` (user+node+bucket 唯一)：`bucket` (date, **UTC**)、`tx`、`rx`

> 字节一律 int64，禁止 float。

### 时间（UTC）

- **库内一律 UTC**：`bucket`、`last_polled_at` 及所有 date 字段写入时用 `time.Now().UTC()`；`traffic_hourly` 按 **UTC 小时**、`traffic_daily` 按 **UTC 日** 分桶（与服务器本地时区无关）。
- **API 返回**：datetime 保持 UTC（PocketBase 常见为带 `Z` 的 ISO 字符串）；`traffic/series` 的 `from` / `to` 查询参数也传 **UTC**，与 `points[].bucket` 同格式。
- **前端**：解析 API 时间为 UTC，图表/列表用浏览器或用户偏好时区 **仅做展示**；向 `series` 发范围前先把本地起止时间换算成 UTC。不要要求后端按用户时区重算 bucket。

## 采集器（internal/collector）

- `main.go` 在 `OnServe` 里启动一个后台 goroutine，`OnTerminate` 时 cancel。
- 每 5s 统一 tick，按各节点 `poll_interval` 判断是否到点（不是每节点一个 ticker，便于增删节点）。最小采集粒度因此是 5s。
- 每个节点每轮：`GET /traffic` → 对每个 auth_string 查 `users` → counter-to-delta → 累加 `users.used_*` + upsert hourly/daily → 更新 cursor。
- **counter reset 处理**（关键，别动）：
  ```go
  func delta(cur, last int64) int64 {
      if cur >= last { return cur - last } // 正常累加
      return cur                           // Hysteria 重启 counter 归零 → 当前值即增量
  }
  ```
- 失败时写 `node.last_error` 且**不更新 cursor**，下一轮自然补回（counter 模式特性）。
- `/online` 和 `/dump/streams` **不进采集循环**，由 live 接口实时拉。

## 接口（前缀 /api/panel/，需登录；除用户自查接口外需 admin）

详见 backend/README.md。要点：
- 凡是返回 node 的响应**必须经过 `publicNode()` 剥除 api_secret**。新增 node 相关接口时务必走这个函数。
- `PATCH /nodes/{id}` 的 `api_secret`：**缺省=不变，传空字符串=报错**（防止误清空）。
- `GET /users/{id}`、`GET /users/{id}/traffic/*`、`GET /users/{id}/live` 允许 admin 或本人访问；用户列表、创建、修改、删除仍仅 admin。
- `live` 接口（`GET /users/{id}/live`）是实时诊断核心：并发拉所有可见节点的 `/dump/streams` + `/online`（5s 超时），按 `auth_string` 过滤，聚合出 `online_devices` / `active_streams` / `by_node` / `top_domains`（按 hooked_req_addr 域名聚合）/ `by_connection`（按设备分组）。单节点失败在 `by_node` 标 `error`，不阻塞整体。**不缓存、不入库。**

## 运行与安全

本地推荐：`cp .env.example .env` → 编辑 `PANEL_MASTER_KEY` → `direnv allow` → `make serve`（详见 `backend/README.md`）。PocketBase 不读 `.env`；变量须进入进程环境。`internal/config` 用 caarlos0/env 解析，`PANEL_MASTER_KEY` 未设置则拒绝启动。

- `PANEL_MASTER_KEY` 经 SHA-256 派生 256 位 AES key，用于加解密节点 secret。**换了这个 key，已存的节点 secret 全部解不开**——迁移环境时要带着原 key。
- `api_secret` 绝不能明文返回给前端，也不要写进日志。
- Hysteria API 调用都带 `Authorization: <secret>` header。

## 开发约定

- 新增或写入 datetime 字段时默认 **UTC**；勿用 `time.Now()` 无 `.UTC()` 落库。
- 改动后至少跑 `go build ./...` 和 `go vet ./...`，确保零告警。
- 验证启动：带 `PANEL_MASTER_KEY` 跑 `serve`，确认 6 张 collection 建出、未授权访问 `/api/panel/*` 返回 401。
- 不写测试是当前状态，但欢迎补充——优先覆盖 `collector.delta`（reset 边界）和 live 聚合逻辑。
- 字段名、collection 名一旦定下，前端会依赖，改动需同步通知前端开发者并更新 README。
