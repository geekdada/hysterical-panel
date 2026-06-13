# AGENTS.md — hysterical-panel

面向 AI 协作者的项目概要与开发注意事项。动手前先读完本文件。

> `CLAUDE.md` 是指向本文件的符号链接——两者是同一份内容，编辑 `AGENTS.md` 即可。

## 这是什么

一个**轻量级 Hysteria 2 管理面板**，分前后端两部分：

- `./backend` — Go + **PocketBase（framework 方式）** 后端。所有业务逻辑、采集、鉴权都在这里。
- `./frontend` — **TanStack Start (React 19) + HeroUI v3** 前端，通过后端 OpenAPI 生成的类型化 client 调用 `/api/panel/*`。

后端职责边界（务必遵守，不要擅自扩张）：

- 保存 Hysteria 节点的**接口信息**（API 地址 + secret），不部署节点、不管理服务器。
- 自主轮询采集各节点流量，做用户级 / 节点级聚合。
- 用户管理 + 实时诊断面板。
- 作为 Hysteria 节点的 `auth.type: http` 回调端点，按 `auth_string` 鉴权客户端连接。
- **不做**：订阅、用量计费、账号踢出、节点部署。这些是明确排除项，需求方已确认。

## 常用命令

### backend（`cd backend`，命令需 `PANEL_MASTER_KEY` 在进程环境）

| 命令 | 说明 |
|---|---|
| `make serve` | 启动 dev server（`go run . serve`），缺 `PANEL_MASTER_KEY` 会拒绝启动 |
| `make build` | `CGO_ENABLED=0` 构建到 `dist/` |
| `make test` | `go test ./...` |
| `make vet` / `make fmt` / `make tidy` | go vet / fmt / mod tidy |
| `make migrate` / `make migrate-create name=add_foo` | 应用 / 新建迁移 |
| `make openapi` | 生成 `openapi.json`（内部跑 `PANEL_MASTER_KEY=skip go run . openapi-schema -o openapi.json`，不启动 server） |

- 跑单个测试：`go test ./internal/api -run TestName`（包路径 + `-run` 正则）。
- **改动后至少跑 `go build ./...` 和 `go vet ./...`，确保零告警。**
- `make serve` 走 direnv/.env 注入环境变量（见下方“运行与安全”）；PocketBase **不读 `.env`**。

### frontend（`cd frontend`，用 **pnpm**）

| 命令 | 说明 |
|---|---|
| `pnpm dev` | Vite dev server（端口 3000）；连后端地址取自 `.env.local` 的 `VITE_API_BASE_URL`（默认 `http://localhost:8090`） |
| `pnpm build` | 生产构建 |
| `pnpm typecheck` | `tsc --noEmit`——**没有独立 lint/test，typecheck 是唯一门禁** |
| `pnpm api:sync` | 重生成后端 `openapi.json` 再生成 `src/api/schema.d.ts`（= `api:schema` + `api:types`） |

- **改了后端 DTO / 路由后必须 `pnpm api:sync`**，否则前端类型与后端契约漂移。
- `src/routeTree.gen.ts` 与 `src/api/schema.d.ts` 是**生成产物，勿手改**。

### release（仓库根）

`scripts/release.sh <version>`（如 `1.2.3` 或 `v1.2.3-rc.1`）：写 `VERSION` + `frontend/package.json`、跑后端 test/vet/build 与前端 typecheck/build、commit 并打 tag。详见 `RELEASING.md`。

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
   `admin` 可管理节点和用户；`user` 只能查看自己的账号详情、用量和实时诊断。新增管理接口默认走 `requireAdmin`；新增用户自查接口才走 admin-or-self 守卫。前端 `src/api/guards.ts` 的 `requireAdmin` / `requireAdminOrSelf` 与后端守卫一一对应。

6. **`status` 是用户启停的单一来源，且真正生效；`verified` 是附加门禁。**
   `active`/`disabled` 两态。落地在三处：登录鉴权 `bindAuthGate`（`OnRecordAuthRequest("users")`，非 active 一律 403 `account is disabled`，覆盖登录与 token 刷新）；Hysteria 回调 `hysteriaAuth`（非 active 返回 403，拒绝客户端新连接）；采集器 `pollNode` 里非 active 用户**仍推进 cursor 但不计量**（避免重新启用时把停用期间的 counter 一次性灌进单个 bucket）。注意：面板不踢 Hysteria 已建立的连接，`disabled` 只挡新连接 + 面板登录 + 停止记账。写 `status` 经 `validUserStatus` 校验。
   **账号「可用」= `status=active` 且 `verified=true`**：`bindAuthGate` 与 `hysteriaAuth` 都在 status 检查后再判 `verified`（非 verified → 403 `email not verified`）。admin 建号与邀请码注册者恒 `verified=true`，新门禁只挡「开放注册且无邀请码」的未验证用户，直到其点开验证邮件。

7. **注册访问由 `app_settings` 三开关 + `registrationDecision` 收口。**
   公开 `POST /api/panel/register`（见 `register.go`）。判定：`open_registration` → 看 `require_invite_for_open` 是否要码；否则 `invitations_enabled` → 仅邀请（必须有码）；都关 → 403。`verified := codeRequired`（经码即验证并自动登录；开放无码 → `verified=false`、发验证邮件、依赖 SMTP、未配则 503）。新用户固定 `role=user`/`status=active`、`auth_string` 由 `internal/token` 随机生成并查重，**不信任客户端传入的 role/auth_string/status**。`registrationDecision` / `invalidInviteReason` 有单测（`register_test.go`），改判定逻辑务必同步测试。邀请码是通用码（`max_uses`/`expires_at`/`revoked`）。

8. **后端是 OpenAPI 契约的唯一来源。**
   `internal/api/dto.go` 的结构体 + `internal/api/openapi.go` 生成 `/api/openapi.json`（也可 `make openapi` 落地成文件），前端据此生成 TS 类型。**新增/改动 `/api/panel/*` 接口时同步更新 DTO 并重生成 schema**。`/api/hysteria/auth` 故意不进 schema（它由 Hysteria 节点调用，不是前端 client 的一部分）。

## 技术栈与版本

- Go（go.mod 锁 1.26.2，但语言特性按 1.21+ 写）。
- **PocketBase v0.39.0**，以 **framework 方式**引入（不是当二进制用）。
  - 用的是 v0.23+ 的 API：`core.App`、`app.OnServe().BindFunc`、`core.RequestEvent`、`core.NewBaseCollection`、`se.Router.Group(...).Bind(...)`、`hook.Handler[*core.RequestEvent]`。
  - 升级 PocketBase 大版本前务必查 [CHANGELOG](https://github.com/pocketbase/pocketbase/blob/master/CHANGELOG.md)；0.22→0.23 改动很大，0.23→0.39 对本项目主要是依赖与 `pb_data` 系统迁移。
- SQLite（PocketBase 自带 modernc 驱动，纯 Go，无 CGO）。
- 关键依赖：`caarlos0/env/v11`（环境变量）、`oschwald/maxminddb-golang/v2`（IP 元数据 MMDB）、`getkin/kin-openapi`（OpenAPI 生成）、`spf13/cobra`（PocketBase 自带，`main.go` 加了 `openapi-schema` 子命令）。
- 前端：TanStack Start / Router（React 19，SSR，文件式路由）、HeroUI v3（beta，基于 React Aria，**Tailwind v4**，无 Provider）、recharts、`openapi-fetch` + `openapi-typescript`。

## 目录结构

```text
hysterical-panel/
├── AGENTS.md / CLAUDE.md       本文件（CLAUDE.md 是指向 AGENTS.md 的符号链接）
├── VERSION                     全应用版本号（frontend/package.json 必须与之一致）
├── RELEASING.md                发布流程；scripts/release.sh 配套
├── .github/workflows/          release.yml：push `v*` tag 后构建并推 GHCR 镜像，再建 draft release
├── backend/
│   ├── main.go                 启动：migration + openapi-schema 子命令 + ipmeta + 采集器 + 路由
│   ├── Makefile / README.md    命令与人类向运行说明 + 接口表
│   ├── Dockerfile / .dockerignore
│   ├── go.mod / go.sum         module 名为 hysterical-panel
│   ├── mmdb/                    Country-asn.mmdb / Country-without-asn.mmdb（ipmeta 读取）
│   ├── migrations/             代码式迁移，启动自动应用（1730000001..10）
│   └── internal/
│       ├── config/             环境变量（caarlos0/env）+ test
│       ├── cryptobox/          AES-GCM 加解密节点 secret
│       ├── token/              URL-safe 随机 token（邀请码 / auth_string）+ test
│       ├── hysteria/           Traffic Stats API 客户端
│       ├── ipmeta/             IP 字面量 → ASN/国家（MMDB）+ test
│       ├── collector/          counter-to-delta 采集核心
│       └── api/                /api/panel 路由
│           ├── api.go          路由注册 + 鉴权中间件（含 verified 门禁）+ 脱敏辅助 + nodesForUser
│           ├── nodes.go        节点 CRUD + 连通性测试
│           ├── users.go        用户 CRUD
│           ├── settings.go     app_settings 读写（注册开关）
│           ├── invitations.go  邀请码 CRUD + inviteValid
│           ├── register.go     公开自助注册 + registrationDecision + auth_string 生成
│           ├── mailer.go       邀请信 / 验证信（PocketBase SMTP，含 link 兜底）
│           ├── traffic.go      用户用量 summary / series
│           ├── node_traffic.go 节点维度用量 summary / series
│           ├── live.go         用户实时诊断（重点）
│           ├── node_live.go    节点维度实时诊断
│           ├── hysteria_auth.go 公开 /api/hysteria/auth 回调
│           ├── dto.go          OpenAPI 用的响应/请求结构体
│           ├── openapi.go      生成 OpenAPI 3.1 spec
│           ├── register_test.go registrationDecision / inviteValid 单测
│           └── live_test.go    live 聚合测试
└── frontend/
    ├── PRODUCT.md              设计语言唯一来源（见“前端”）
    ├── vite.config.ts          TanStack Start + react + tailwind；从 ../VERSION 注入 __APP_VERSION__
    ├── tsconfig.json           路径别名 ~/* → src/*
    └── src/
        ├── api/                client.ts(openapi-fetch) / auth.ts(含 register/confirmVerification) / cookie.ts / guards.ts / queries.ts / schema.d.ts(生成)
        ├── routes/             文件式路由（index / login / register / verify / settings / invitations / nodes / users）
        ├── components/         traffic.tsx（图表）/ ui.tsx / user-menu.tsx（admin 入口：settings / invitations）
        ├── lib/format.ts       展示格式化（字节 / 时间）
        └── styles/globals.css  设计 token（覆盖 HeroUI v3 默认）
```

## 数据模型

`users`（扩展 PocketBase 内置 auth collection）：

- `auth_string` (text, unique, required) — Hysteria auth key，= /traffic 返回的 key；自助注册时由系统随机生成
- `role` (select [admin, user])、`status` (select [active, disabled]) — `status` 是用户启停的单一来源（active = 启用）
- `verified` (PocketBase 内置 auth 字段) — 账号可用的附加门禁；admin 建号与邀请注册者恒 true，仅开放无码注册者初始 false
- `quota_bytes`、`used_tx`、`used_rx` (number, int64) — quota 当前不计费，仅留字段

`nodes`：

- `name`、`api_url` (url)、`api_secret` (text, **AES-GCM 加密存储**)
- `poll_interval` (number, 秒, 默认 30)、`enabled` (bool)
- `last_polled_at` (date)、`last_error` (text) — 用于 health 判断

`traffic_cursor` (user+node 唯一)：`last_tx`、`last_rx` —— counter-to-delta 的游标
`traffic_hourly` / `traffic_daily` (user+node+bucket 唯一)：`bucket` (date, **UTC**)、`tx`、`rx`

`invitations`：`code` (text, unique) — 通用邀请码；`email`（可选，仅记录/发信，不绑定）、`max_uses`（0=不限）、`used_count`、`expires_at` (date, 空=永不)、`revoked` (bool)、`note`、`created_by` (relation→users)、`last_used_at`。

`app_settings`（单例，迁移时 seed 一条全 false 记录）：`invitations_enabled`、`open_registration`、`require_invite_for_open` (bool)。运行期可变，注册与 `/config` 实时读。

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

## 接口

### 面板接口（前缀 `/api/panel/`，需登录；除标注外需 admin）

详见 backend/README.md。要点：

- 凡是返回 node 的响应**必须经过 `publicNode()` 剥除 api_secret**。新增 node 相关接口时务必走这个函数。
- `PATCH /nodes/{id}` 的 `api_secret`：**缺省=不变，传空字符串=报错**（防止误清空）。
- `GET /users/{id}`、`GET /users/{id}/traffic/*` 允许 admin 或本人访问；`GET /users/{id}/live` 仅 admin。用户列表、创建、修改、删除仍仅 admin。
- 节点维度接口 `GET /nodes/{id}/traffic/summary|series`、`GET /nodes/{id}/live` 是**全节点跨用户**视角，仅 admin。
- `GET|PATCH /settings`、`GET|POST /invitations`、`DELETE /invitations/{id}` 均 admin。`PATCH /settings` 校验 `require_invite_for_open` 依赖 `invitations_enabled`；`POST /invitations` 在 `invitations_enabled=false` 时 400。邀请响应含 `link`（`frontend_url + /register?code=`，未设前端域名则相对路径）。
- `GET /api/panel/config`（公开）现额外回 `registration_open` / `registration_require_invite` / `invitations_enabled`（从 `app_settings` 实时读），供 `/login`、`/register` 渲染入口。
- `live` 接口（用户：`GET /users/{id}/live`；节点：`GET /nodes/{id}/live`）是实时诊断核心：并发拉可见节点的 `/dump/streams` + `/online`（5s 超时），按 `auth_string` 过滤/聚合出 `online_devices` / `active_streams` / `by_node` / `top_domains`（按 hooked_req_addr 域名聚合）/ `by_connection`（按设备分组）。单节点失败在 `by_node` 标 `error`，不阻塞整体。**不缓存、不入库。** Top domains 只对已是 IP 字面量的目标做本地 MMDB 查询（`internal/ipmeta`），补 ASN / 国家与 IPv4 的 ipinfo.io 链接，**不做 DNS 解析**。

### OpenAPI

- `GET /api/openapi.json`（无需登录，不含 secret）实时返回 spec；`make openapi` 落地成 `backend/openapi.json` 供前端 `pnpm api:types` 消费。
- spec 由 `dto.go` 结构体 + `openapi.go` 生成。`openapi.go` 对每个 type 用独立 generator，避免 enum 在共享 schema 间串味——加字段枚举时照此模式。

### 公开接口（无需登录）

`POST /api/panel/register` — 自助注册（进 OpenAPI，标记无需鉴权）。访问与是否要码由 `app_settings` + `registrationDecision` 决定；`verified := codeRequired`；经码自动登录（返回 token+record），开放无码发验证邮件并返回 `{requires_verification:true}`（依赖 SMTP，未配 503）。强制 `role=user`/`status=active`，`auth_string` 系统生成。按 IP 限流。邮箱验证由前端 `/verify` 调 PocketBase 内置 `POST /api/collections/users/confirm-verification` 完成（非本项目自建端点）。

`POST /api/hysteria/auth` — Hysteria 2 节点 `auth.type: http` 回调，每次客户端连接时触发。按请求体 `auth` 在 `users.auth_string` 查匹配，命中且 `status=active` 且 `verified=true` → `200 {"ok":true,"id":"<auth_string>"}`；查无此人 401；存在但 disabled 或未验证 403；缺 `auth`/非法 JSON 400。返回的 `id` **故意回填为 `auth_string`**，让节点后续 `/traffic` 上报的 key 与采集器查询字段一致（见 `hysteria_auth.go` 注释）。**绝不记录 `auth` 值本身**（凭据），只记 addr 与拒绝原因。该路由不进 OpenAPI。

> 邮件走 PocketBase 内置 SMTP（`/_/` 后台配置，无新增 env）。`mailer.go` 在 `SMTP.Enabled=false` 时不发信：邀请接口仍返回 `link` 供手动分享，开放无码注册因依赖验证邮件而不可用。

## 前端（./frontend）

- 设计语言以 `frontend/PRODUCT.md` 的 **Design Language** 为唯一来源（Linear 风、系统明暗 follow `prefers-color-scheme`、无手动切换）。token 落在 `src/styles/globals.css`（覆盖 HeroUI v3 默认），改设计就改那里。`PRODUCT.md` 的 “Avoid” 列了要躲开的 AI-dashboard 套路（hero metric 卡片、卡片网格、渐变文字、玻璃拟态、UI 文案里的破折号等）。
- **类型化 API**：`src/api/client.ts` 用 `openapi-fetch` + 生成的 `schema.d.ts`（`paths`）。不要手写请求/响应类型；改后端契约后跑 `pnpm api:sync`。
- **鉴权**：登录直接打 PocketBase 内置 `/api/collections/users/auth-with-password`（`src/api/auth.ts`），token+record 存 cookie；`client.ts` 的 middleware **每请求**从 cookie 读 token 塞 `Authorization`（无共享模块状态，防跨请求泄漏）。SSR 安全靠 `createIsomorphicFn`：服务端读 request cookie、客户端读 document.cookie。
- **路由守卫**：`src/api/guards.ts`，在路由 `beforeLoad` 里用，与后端守卫对齐（非 admin 访问 admin 页 → 跳自己的账号页）。
- 路径别名 `~/*` → `src/*`；`VITE_API_BASE_URL` 指向后端（dev 默认 `http://localhost:8090`，见 `.env.local`）；`__APP_VERSION__` 由 vite 从根 `VERSION` 注入。

## 运行与安全

本地推荐：`cp .env.example .env` → 编辑 `PANEL_MASTER_KEY` → `direnv allow` → `make serve`（详见 `backend/README.md`）。PocketBase 不读 `.env`；变量须进入进程环境。`internal/config` 用 caarlos0/env 解析。

| 变量 | 必填 | 说明 |
|---|---|---|
| `PANEL_MASTER_KEY` | 是 | 经 SHA-256 派生 256 位 AES key，加解密节点 secret。**换 key → 已存 secret 全部解不开**，迁移环境要带原 key。生成 schema 时可传占位（`make openapi` 用 `PANEL_MASTER_KEY=skip`） |
| `PANEL_FRONTEND_URL_BASE` | 否 | 面板 UI 的 CORS 来源（`http://` / `https://`，无路径）；未设置则 `*`。写入 `apis.ServeConfig.AllowedOrigins` |
| `PANEL_CORS_MAX_AGE` | 否 | 预检缓存 `Access-Control-Max-Age`（秒），默认 `7200`；`0` 关闭 |
| `PB_DATA_DIR` | 否 | PocketBase 数据目录，默认 `./pb_data`；CLI `--dir` 优先级更高 |
| `MMDB_DIR` | 否 | IP 元数据 MMDB 目录，默认 `./mmdb`，需含 `Country-asn.mmdb` 与 `Country-without-asn.mmdb`。缺失/损坏会让服务**启动失败**（避免静默丢 ASN/国家信息） |
| `PB_ENCRYPTION_KEY` | 否 | PocketBase 设置库加密密钥，须 **32 字符**；未设则设置库明文存储 |

- `api_secret` 绝不能明文返回给前端，也不要写进日志。Hysteria API 调用都带 `Authorization: <secret>` header。
- 首次启动按提示创建 superuser（PocketBase 后台 `/_/`）。

### Docker / 发布

- `backend/Dockerfile` 多阶段构建（`CGO_ENABLED=0`，alpine，非 root `panel` 用户），监听 `0.0.0.0:8090`，数据卷 `/app/pb_data`，把 `mmdb/` 拷进镜像。`PANEL_MASTER_KEY` 仍必填。
- `frontend/Dockerfile` 多阶段构建（Go 生成 OpenAPI → pnpm build → Nitro `.output`，非 root `panel` 用户），监听 `0.0.0.0:3000`。构建上下文为仓库根目录；CI 默认空 `VITE_API_BASE_URL`（同域反代）。
- 镜像**只在 push `v*.*.*` tag 后**由 `.github/workflows/release.yml` 构建并推 GHCR（`ghcr.io/<repo>-backend` / `-frontend`，多架构 amd64+arm64）；同一个 workflow 在两个镜像都推成功后**创建 draft release**（正文含各镜像 `docker pull` 命令），由人工 review 后手动发布。普通提交、PR 不触发；发布 draft 不会重新构建（镜像在 tag 落地时已推）。CI 会校验 `VERSION` == `frontend/package.json` version == tag。
- 根目录 `docker-compose.yml` + `deploy/nginx/default.conf`：本地全栈（nginx 反代 `/api` 与 `/_/` 到后端，其余到前端）。

## 开发约定

- 新增或写入 datetime 字段时默认 **UTC**；勿用 `time.Now()` 无 `.UTC()` 落库。
- 改动后端后至少跑 `go build ./...` 和 `go vet ./...`，确保零告警；改了接口契约要 `make openapi` + 前端 `pnpm api:sync`。
- 验证启动：带 `PANEL_MASTER_KEY` 跑 `serve`，确认 collection 建出、未授权访问 `/api/panel/*` 返回 401。
- 已有测试覆盖 `internal/config`、`internal/ipmeta`、`internal/api`(live 聚合)；继续补测优先 `collector.delta`（reset 边界）和 live 聚合逻辑。
- 字段名、collection 名、API 契约一旦定下前端会依赖，改动需同步更新 `dto.go` / OpenAPI / README 并通知前端。
