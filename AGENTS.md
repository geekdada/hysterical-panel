# AGENTS.md — hysterical-panel

面向 AI 协作者的项目概要与开发注意事项。动手前先读完本文件。

> `CLAUDE.md` 是指向本文件的符号链接——两者是同一份内容，编辑 `AGENTS.md` 即可。

## 这是什么

一个**轻量级 Hysteria 2 管理面板**，分前后端两部分：

- `./backend` — Go + **PocketBase（framework 方式）** 后端。所有业务逻辑、采集、鉴权都在这里。
- `./frontend` — **TanStack Start (React 19) + HeroUI v3** 前端，通过后端 OpenAPI 生成的类型化 client 调用 `/api/panel/*`。

后端职责边界（务必遵守，不要擅自扩张）：

- 保存 Hysteria 节点的**接口信息**（API 地址 + secret），不部署节点、不管理服务器。
- 自主轮询采集各节点流量，做用户级 / 节点级聚合 + admin 全局看板。
- 用户管理（分页/排序/筛选列表）+ 实时诊断面板。
- 作为 Hysteria 2 / anytls 节点的 `auth.type: http` 回调端点，按 `auth_string`（anytls 按其 hash）鉴权客户端连接。
- 面板登录支持密码 + passkey（WebAuthn）；可选开启 Management API（Bearer token）供外部系统建号 / 查号。
- **不做**：订阅、用量计费、节点部署。这些是明确排除项，需求方已确认。

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
| `pnpm i18n:check` | 校验每个 message key 在所有 locale（`en` / `zh-cn`）都有非空翻译（`scripts/i18n-check.mjs`，CI 门禁） |
| `pnpm api:sync` | 重生成后端 `openapi.json` 再生成 `src/api/schema.d.ts`（= `api:schema` + `api:types`） |

- **改了后端 DTO / 路由后必须 `pnpm api:sync`**，否则前端类型与后端契约漂移。
- `src/routeTree.gen.ts`（已提交）、`src/api/schema.d.ts` 与 `src/paraglide/`（后两者 gitignore）是**生成产物，勿手改**。
- **i18n 由 Paraglide 经 vite 插件编译进 `src/paraglide/`**：全新 checkout 直接 `pnpm typecheck` 会因该目录不存在而失败，先跑一次 `pnpm dev` / `pnpm build` 生成它再 typecheck。改了 `messages/*.json` 务必两个 locale 同步加键，并 `pnpm i18n:check`。

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
   `admin` 可管理节点和用户；`user` 只能查看自己的账号详情、用量和实时诊断。新增管理接口默认走 `requireAdmin`；新增用户自查接口才走 admin-or-self 守卫。后端守卫共四个（`internal/api/api.go`）：`requireAdmin`、`requireAdminOrSelf`，以及对 passkey 等敏感自助操作额外要求 `status=active` 的 `requireActiveAdminOrSelf` / `requireActiveSelf`。前端 `src/api/guards.ts` 的 `requireAdmin` / `requireAdminOrSelf` 与后端守卫对应。

6. **`status` 是用户启停的单一来源，且真正生效；`verified` 是附加门禁。**
   `active`/`disabled` 两态。落地在三处：登录鉴权 `bindAuthGate`（`OnRecordAuthRequest("users")`，非 active 一律 403 `account is disabled`，覆盖登录与 token 刷新）；Hysteria 回调 `hysteriaAuth`（非 active 返回 403，拒绝客户端新连接）；采集器 `pollNode` 里非 active 用户**仍推进 cursor 但不计量**（避免重新启用时把停用期间的 counter 一次性灌进单个 bucket）。`disabled` 同时挡新连接 + 面板登录 + 停止记账；写 `status` 经 `validUserStatus` 校验。
   **停用时（status `active`→`disabled`）会触发一次 best-effort `/kick` 扇出**：`updateUser` 检测到该转换后，起一个**后台 goroutine**（**不阻塞 PATCH 响应**），以 **3 并发**（信号量 `kickConcurrency`）对 `nodesForUser(userID)` 返回的每个节点 `POST /kick [auth_string]`（5s/节点超时，整体 30s 上限）。失败只记日志（`[kick] ...`）、单个失败不影响其余、`nodesForUser` 返回空也直接结束。落地在 `internal/api/kick.go` 的 `kickUser` / `fanOutKicks`，Hysteria client 在 `internal/hysteria/client.go` 的 `Kick`。**只清存量会话**：`node_client_auth.go` 的 403 已挡客户端重连，`/kick` 只是把当前已建立的连接断掉。其他状态转换（建号 / 删除 / 改 auth_string）不触发 `/kick`，靠后续 401/403 自然清退。
   **账号「可用」= `status=active` 且 `verified=true`**：`bindAuthGate` 与 `hysteriaAuth` 都在 status 检查后再判 `verified`（非 verified → 403 `email not verified`）。admin 建号与邀请码注册者恒 `verified=true`，新门禁只挡「开放注册且无邀请码」的未验证用户，直到其点开验证邮件。

7. **注册访问由 `app_settings` 三开关 + `registrationDecision` 收口，三开关是严格层级。**
   公开 `POST /api/panel/register`（见 `register.go`）。三开关层级：`open_registration`（总开关）> `invitations_enabled`（依赖总开关）> `require_invite_for_open`（依赖邀请系统）。判定：`open_registration=false` → 403（无论邀请系统开否）；`open_registration=true` → `require_invite_for_open` 决定是否要码。`PATCH /settings` 强制校验该嵌套（`invitations_enabled` 需 `open_registration`；`require_invite_for_open` 需 `invitations_enabled`，违反 400）；前端关父开关时在同一 PATCH 里级联把子开关置 false。`verified := codeRequired`（经码即验证并自动登录；开放无码 → `verified=false`、发验证邮件、依赖 SMTP、未配则 503）。新用户固定 `role=user`/`status=active`、`auth_string` 由 `internal/token` 随机生成并查重，**不信任客户端传入的 role/auth_string/status**。`registrationDecision` / `invalidInviteReason` 有单测（`register_test.go`），改判定逻辑务必同步测试。邀请码是通用码（`max_uses`/`expires_at`/`revoked`）。存量「仅邀请」部署（`invitations_enabled=true && open_registration=false`）由迁移 `1730000017` 提升为三开关全开以保持行为不变。

8. **后端是 OpenAPI 契约的唯一来源。**
   `internal/api/dto.go` 的结构体 + `internal/api/openapi.go` 生成 `/api/openapi.json`（也可 `make openapi` 落地成文件），前端据此生成 TS 类型。**新增/改动 `/api/panel/*` 接口时同步更新 DTO 并重生成 schema**。`/api/hysteria/auth`、`/api/anytls/auth`、`/api/mgmt/*` 故意不进 schema（它们由节点 / 外部系统调用，不是前端 client 的一部分）。

9. **Passkey（WebAuthn）是密码之外的第二登录方式，不替代密码。**
   基于 `go-webauthn/webauthn`，落在 `internal/api/passkeys.go` + 两张 collection：`passkey_credentials`（每凭据一行，私有 `credential` JSON `Hidden`）、`passkey_sessions`（challenge 暂存，`passkeyChallengeTTL` 5min，用后即焚）。登录走公开的 `POST /api/panel/passkeys/login/{options,finish}`（finish 成功签发与密码登录同形的 token+record，同样过 `bindAuthGate` 的 status/verified 门禁）；注册/列举/删除挂在 `/users/{id}/passkeys/*`，受 `requireActiveSelf` / `requireActiveAdminOrSelf` 守。**RP ID 与允许的 origin 由 `PANEL_WEBAUTHN_RP_ID` / `PANEL_WEBAUTHN_ORIGINS` 配置；未配（且无静态面板 origin）则 passkey 整体禁用**，`/config` 的 `passkeys` 标志据此告诉前端是否渲染入口。

10. **Management API 是「默认不存在」的外部建号通道。**
    `/api/mgmt/*`（`management_api.go`）公开无面板登录，唯一门禁是 `Authorization: Bearer <token>`。**功能未启用时一律返回 404（不泄露其存在）**，启用但 token 错返回 401。开关与 token **哈希**存在 `app_settings`（`management_api_enabled` / `management_api_token_hash`，只存 `sha256` hex，明文 token 仅在启用/轮换响应里回显一次）：admin 经 `PATCH /settings` 首次启用时自动生成，或 `POST /management-api/rotate` 轮换。当前仅 `GET /api/mgmt/users`（按 email 或 auth_string 精确查）与 `POST /api/mgmt/users`（建号，自动生成密码与 auth_string、均不返回）。详见 backend/README.md。

## 技术栈与版本

- Go（go.mod 锁 1.26.2，但语言特性按 1.21+ 写）。
- **PocketBase v0.39.0**，以 **framework 方式**引入（不是当二进制用）。
  - 用的是 v0.23+ 的 API：`core.App`、`app.OnServe().BindFunc`、`core.RequestEvent`、`core.NewBaseCollection`、`se.Router.Group(...).Bind(...)`、`hook.Handler[*core.RequestEvent]`。
  - 升级 PocketBase 大版本前务必查 [CHANGELOG](https://github.com/pocketbase/pocketbase/blob/master/CHANGELOG.md)；0.22→0.23 改动很大，0.23→0.39 对本项目主要是依赖与 `pb_data` 系统迁移。
- SQLite（PocketBase 自带 modernc 驱动，纯 Go，无 CGO）。
- 关键依赖：`caarlos0/env/v11`（环境变量）、`oschwald/maxminddb-golang/v2`（IP 元数据 MMDB）、`getkin/kin-openapi`（OpenAPI 生成）、`go-webauthn/webauthn`（passkey）、`spf13/cobra`（PocketBase 自带，`main.go` 加了 `openapi-schema` 子命令）。
- 前端：TanStack Start / Router（React 19，SSR，文件式路由）、HeroUI v3（beta，基于 React Aria，**Tailwind v4**，无 Provider）、`@tanstack/react-query`（数据获取，`src/api/queries.ts` + `query-provider.tsx`）、`@tanstack/react-table`（用户列表）、`@tanstack/react-form`、recharts、`openapi-fetch` + `openapi-typescript`、`@simplewebauthn/browser`（passkey）、`@inlang/paraglide-js`（i18n，`en` / `zh-cn`）。

## 目录结构

```text
hysterical-panel/
├── AGENTS.md / CLAUDE.md       本文件（CLAUDE.md 是指向 AGENTS.md 的符号链接）
├── PRODUCT.md                  设计语言 + 产品定位唯一来源（曾在 frontend/，已移到根）
├── VERSION                     全应用版本号（frontend/package.json 必须与之一致）
├── RELEASING.md                发布流程；scripts/release.sh 配套
├── .github/workflows/          release.yml：push `v*` tag 后构建并推 GHCR 镜像，再建 draft release
├── backend/
│   ├── main.go                 启动：migration + openapi-schema 子命令 + ipmeta + 采集器 + 路由
│   ├── Makefile / README.md    命令与人类向运行说明 + 接口表
│   ├── Dockerfile / .dockerignore
│   ├── go.mod / go.sum         module 名为 hysterical-panel
│   ├── mmdb/                    ipinfo_lite.mmdb（ipmeta 读取）
│   ├── migrations/             代码式迁移，启动自动应用（1730000001..17）
│   └── internal/
│       ├── config/             环境变量（caarlos0/env）+ test
│       ├── cryptobox/          AES-GCM 加解密节点 secret
│       ├── token/              URL-safe 随机 token（邀请码 / auth_string）+ test
│       ├── hysteria/           Traffic Stats API 客户端
│       ├── ipmeta/             IP 字面量 → ASN/国家（MMDB）+ test
│       ├── collector/          counter-to-delta 采集核心
│       └── api/                /api/panel 路由（+ /api/mgmt、公开回调）
│           ├── api.go          路由注册 + 4 个鉴权守卫（含 verified / active 门禁）+ 脱敏辅助 + nodesForUser
│           ├── config.go       公开 GET /api/panel/config（静态字段 + 实时注册开关 + passkeys 标志）
│           ├── nodes.go        节点 CRUD + 连通性测试 + 软删除（deleted_at）+ reset-api-secret
│           ├── users.go        用户 CRUD + reset-auth-string
│           ├── users_list.go   分页 / 排序 / 筛选的用户列表 + /users/stats
│           ├── recent_connections.go 用户最近连接 IP 的派生与脱敏序列化
│           ├── passkeys.go     WebAuthn 注册 / 登录 / 列举 / 删除（go-webauthn）
│           ├── settings.go     app_settings 读写（注册开关 + management API token）
│           ├── management_api.go 公开 /api/mgmt/* + requireMgmtToken（未启用 404）
│           ├── database.go     GET /database/stats + POST /database/prune（30 天留存裁剪）
│           ├── invitations.go  邀请码 CRUD + inviteValid
│           ├── ignored_connection_ips.go 全局忽略 IP CRUD
│           ├── register.go     公开自助注册 + registrationDecision + auth_string 生成
│           ├── mailer.go       邀请信 / 验证信（PocketBase SMTP，含 link 兜底）
│           ├── traffic_panel.go admin 全局用量看板（GET /traffic、/traffic/series、/nodes/traffic/summary）
│           ├── traffic.go      用户用量 summary / series
│           ├── node_traffic.go 节点维度用量 summary / series
│           ├── live.go         用户实时诊断（重点）
│           ├── node_live.go    节点维度实时诊断
│           ├── node_client_auth.go 节点 HTTP 鉴权共用契约（handleNodeClientAuth）
│           ├── hysteria_auth.go  公开 /api/hysteria/auth 回调
│           ├── anytls_auth.go    公开 /api/anytls/auth 回调
│           ├── kick.go         停用时 best-effort /kick 扇出（kickUser / fanOutKicks，3 并发）
│           ├── dto.go          OpenAPI 用的响应/请求结构体
│           ├── openapi.go      生成 OpenAPI 3.1 spec
│           └── *_test.go       register / kick / database / users_list / recent_connections / auth_string_anytls_hash / live 等单测
└── frontend/
    ├── vite.config.ts          TanStack Start + react + tailwind + paraglide 插件；从 ../VERSION 注入 __APP_VERSION__
    ├── tsconfig.json           路径别名 ~/* → src/*
    ├── project.inlang/         Paraglide i18n 配置（baseLocale en，locales en/zh-cn）
    ├── messages/               i18n 文案（en.json / zh-cn.json）；编译产物落 src/paraglide/（gitignore）
    ├── scripts/                i18n-check.mjs（校验翻译完整）等
    └── src/
        ├── api/                client.ts(openapi-fetch) / auth.ts(login/register/passkey/密码找回) / queries.ts + query-provider.tsx(react-query) / session.ts / cookie.ts / guards.ts / panel-config.ts / schema.d.ts(生成)
        ├── routes/             文件式路由（index / login / register / verify / forgot-password / reset-password / analytics / settings / invitations / nodes / users）
        ├── components/         traffic.tsx / traffic-range-picker.tsx / ui.tsx / breadcrumbs.tsx / theme-toggle.tsx / locale-toggle.tsx / user-menu.tsx
        ├── paraglide/          Paraglide 编译产物（生成，gitignore，勿手改）
        ├── lib/                展示与工具 helper（format / theme / locale / timezone / cn / use-* hooks 等）
        └── styles/globals.css  设计 token（覆盖 HeroUI v3 默认）
```

## 数据模型

`users`（扩展 PocketBase 内置 auth collection）：

- `auth_string` (text, unique, required) — Hysteria auth key，= /traffic 返回的 key；自助注册时由系统随机生成
- `auth_string_anytls_hash` (text, unique, required) — `hex(sha256(auth_string))`，64 位小写十六进制，供 anytls 回调按客户端发来的哈希匹配用户。**由 `users` 集合的 `OnRecordCreate`/`OnRecordUpdate` 钩子（`bindUserAnytlsHashSync`）在每次保存时自动从 `auth_string` 派生**，禁止手动设置；存量数据由迁移 `1730000016` 回填
- `role` (select [admin, user])、`status` (select [active, disabled]) — `status` 是用户启停的单一来源（active = 启用）
- `verified` (PocketBase 内置 auth 字段) — 账号可用的附加门禁；admin 建号与邀请注册者恒 true，仅开放无码注册者初始 false
- `quota_bytes`、`used_tx`、`used_rx` (number, int64) — quota 当前不计费，仅留字段
- `last_connected_at` (date)、`recent_connections` (json) — Hysteria 鉴权成功后更新；`recent_connections` 只保留最近 10 个唯一客户端 IP（不含端口），MMDB ASN / 国家等信息在 API 序列化时临时补充，不落库

`nodes`：

- `name`、`api_url` (url)、`api_secret` (text, **AES-GCM 加密存储**)
- `poll_interval` (number, 秒, 默认 30)、`enabled` (bool)
- `last_polled_at` (date)、`last_error` (text) — 用于 health 判断
- `current_tx_speed`、`current_rx_speed` (number, int64, B/s) — 采集器每轮按相邻两次 counter 差除以间隔算出的瞬时速率，禁用/删除/采集失败置 0
- `deleted_at` (date) — **软删除**：`DELETE /nodes/{id}` 只写此字段，不真删；`nodesForUser` 与采集器都用 `deleted_at = '' && enabled = true` 过滤，保留历史流量归属

`traffic_cursor` (user+node 唯一)：`last_tx`、`last_rx` —— counter-to-delta 的游标
`traffic_hourly` / `traffic_daily` (user+node+bucket 唯一)：`bucket` (date, **UTC**)、`tx`、`rx`

`invitations`：`code` (text, unique) — 通用邀请码；`email`（可选，仅记录/发信，不绑定）、`max_uses`（0=不限）、`used_count`、`expires_at` (date, 空=永不)、`revoked` (bool)、`note`、`created_by` (relation→users)、`last_used_at`。

`ignored_connection_ips`：`ip` (text, unique, required) — 全局忽略的客户端 IP；命中后不再写入 `users.recent_connections`（`last_connected_at` 仍更新），API 返回时过滤历史记录。admin 通过 `GET|POST /ignored-connection-ips`、`DELETE /ignored-connection-ips/{id}` 管理。

`passkey_credentials`：每个已注册 passkey 一行。`user` (relation→users)、`credential_id`、`user_handle`、`rp_id`、`name`、`credential` (json, **`Hidden`，私有凭据**)、`transports`、`sign_count`、`backup_eligible`/`backup_state`/`clone_warning` (bool)、`last_used_at`。
`passkey_sessions`：WebAuthn challenge 暂存（注册 / 登录两 `kind`）。`challenge_id`、`user` (relation)、`session_data` (json, **`Hidden`**)、`expires_at`（TTL 5min）、`consumed_at`（用后即焚）。

`app_settings`（单例，迁移时 seed 一条全 false 记录）：`invitations_enabled`、`open_registration`、`require_invite_for_open` (bool)；`management_api_enabled` (bool) + `management_api_token_hash` (text, 仅存 token 的 sha256 hex，**绝不存明文**)。运行期可变，注册与 `/config` 实时读。

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
- `GET /users/{id}`、`GET /users/{id}/traffic/*` 允许 admin 或本人访问；`GET /users/{id}/live` 仅 admin。用户列表、创建、修改、删除仍仅 admin。`PATCH /users/{id}` 在状态从 `active`→`disabled` 时，**异步**对 `nodesForUser(userID)` 返回的每个节点扇出 `POST /kick [auth_string]`（3 并发，5s/节点，best-effort，失败只记日志，不阻塞响应）；详见核心决策 #6。
- `GET /users`（仅 admin）是**分页 / 排序 / 筛选**列表（`users_list.go`：`page`、`perPage`∈{25,50,100}、`search`、`sort` 白名单），`GET /users/stats` 给看板汇总。`POST /users/{id}/reset-auth-string`、`POST /nodes/{id}/reset-api-secret` 重新随机生成对应凭据并返回（节点 secret 仍脱敏）。
- **admin 全局看板**：`GET /traffic`、`GET /traffic/series`（`from`/`to` 必填，UTC）、`GET /nodes/traffic/summary` 是跨用户跨节点的全局聚合（`traffic_panel.go`），与下面单节点维度接口区分。
- `GET /database/stats`、`POST /database/prune`（仅 admin，`database.go`）：查看库体量并按 **30 天 UTC 留存**裁剪 `traffic_hourly`/`traffic_daily` 历史。
- passkey：`GET /users/{id}/passkeys`、`DELETE /users/{id}/passkeys/{passkeyId}`（`requireActiveAdminOrSelf`）；注册 `POST /users/{id}/passkeys/registration/{options,finish}`（`requireActiveSelf`）。详见核心决策 #9。
- 节点维度接口 `GET /nodes/{id}/traffic/summary|series`、`GET /nodes/{id}/live` 是**单节点跨用户**视角，仅 admin。
- `GET|PATCH /settings`、`POST /management-api/rotate`、`GET|POST /invitations`、`DELETE /invitations/{id}`、`GET|POST /ignored-connection-ips`、`DELETE /ignored-connection-ips/{id}` 均 admin。`PATCH /settings` 校验注册开关层级（`invitations_enabled` 依赖 `open_registration`；`require_invite_for_open` 依赖 `invitations_enabled`），并在首次置 `management_api_enabled=true` 时生成明文 token 回显一次（见决策 #10）；`POST /invitations` 在 `invitations_enabled=false` 时 400。邀请响应含 `link`（`frontend_url + /register?code=`，未设前端域名则相对路径）。
- `GET /api/panel/config`（公开）回静态字段（`api_url` 来自 `PANEL_BACKEND_URL_BASE`、`frontend_url`、`version`、`passkeys_enabled`）+ **实时**读 `app_settings` 的 `registration_open` / `registration_require_invite` / `invitations_enabled`，供 `/login`、`/register` 渲染入口。
- `live` 接口（用户：`GET /users/{id}/live`；节点：`GET /nodes/{id}/live`）是实时诊断核心：并发拉可见节点的 `/dump/streams` + `/online`（5s 超时），按 `auth_string` 过滤/聚合出 `online_devices` / `active_streams` / `by_node` / `top_domains`（按 hooked_req_addr 域名聚合）/ `by_connection`（按设备分组）。单节点失败在 `by_node` 标 `error`，不阻塞整体。**不缓存、不入库。** Top domains 只对已是 IP 字面量的目标做本地 MMDB 查询（`internal/ipmeta`），补 ASN / 国家与 IPv4 的 ipinfo.io 链接，**不做 DNS 解析**。

### OpenAPI

- `GET /api/openapi.json`（无需登录，不含 secret）实时返回 spec；`make openapi` 落地成 `backend/openapi.json` 供前端 `pnpm api:types` 消费。
- spec 由 `dto.go` 结构体 + `openapi.go` 生成。`openapi.go` 对每个 type 用独立 generator，避免 enum 在共享 schema 间串味——加字段枚举时照此模式。

### 公开接口（无需登录）

`POST /api/panel/register` — 自助注册（进 OpenAPI，标记无需鉴权）。访问与是否要码由 `app_settings` + `registrationDecision` 决定；`verified := codeRequired`；经码自动登录（返回 token+record），开放无码发验证邮件并返回 `{requires_verification:true}`（依赖 SMTP，未配 503）。强制 `role=user`/`status=active`，`auth_string` 系统生成。按 IP 限流。邮箱验证由前端 `/verify` 调 PocketBase 内置 `POST /api/collections/users/confirm-verification` 完成（非本项目自建端点）。

`POST /api/panel/passkeys/login/{options,finish}` — passkey 无密码登录（公开，无需先登录）。`options` 发 challenge，`finish` 校验断言成功后签发与密码登录同形的 token+record（仍过 `bindAuthGate` 的 status/verified 门禁）。passkey 未配置时（见决策 #9）这两个端点不可用。

`POST /api/mgmt/users`、`GET /api/mgmt/users` — Management API（公开但 Bearer 鉴权，未启用返回 404；见决策 #10）。供外部系统按 email/auth_string 查号或建号，**不进 OpenAPI**，契约见 backend/README.md。

**找回密码**（无自建后端代码，纯走 PocketBase 内置）：前端 `/forgot-password` 调内置 `POST /api/collections/users/request-password-reset`（`{email}`，恒 204，内置反枚举 + 2 分钟重发节流）；`/reset-password?token=` 调内置 `POST /api/collections/users/confirm-password-reset`（`{token,password,passwordConfirm}`，成功且 token 邮箱匹配会顺带置 `verified=true`）。两端点不触发 `OnRecordAuthRequest`，不受 `bindAuthGate` 阻挡（`disabled` 用户可重置但仍无法登录）。前端函数在 `src/api/auth.ts` 的 `requestPasswordReset`/`confirmPasswordReset`。**部署须一次性配置**：把 PocketBase `Settings → Application URL` 设为前端域名，并把 users collection 的 **Reset password** 邮件模板链接由默认的 `{APP_URL}/_/#/auth/confirm-password-reset/{TOKEN}` 改成 `{APP_URL}/reset-password?token={TOKEN}`，否则邮件链接落到 PocketBase 后台而非前端重置页。该模板邮件走内置 SMTP，不经 `mailer.go`。

`POST /api/hysteria/auth` — Hysteria 2 节点 `auth.type: http` 回调，每次客户端连接时触发。按请求体 `auth` 在 `users.auth_string` 查匹配，命中且 `status=active` 且 `verified=true` → `200 {"ok":true,"id":"<auth_string>"}`；查无此人 401；存在但 disabled 或未验证 403；缺 `auth`/非法 JSON 400。返回的 `id` **故意回填为 `auth_string`**，让节点后续 `/traffic` 上报的 key 与采集器查询字段一致（见 `node_client_auth.go` / `hysteria_auth.go` 注释）。成功鉴权会异步更新 `users.last_connected_at`，并从请求体 `addr` 提取客户端 IP 写入 `users.recent_connections`（最近 10 个唯一 IP，重复 IP 更新 `last_seen_at`；只存 IP，不存端口；ASN / 国家 / ipinfo 链接由 API 返回用户记录时用 MMDB 临时补充）。**绝不记录 `auth` 值本身**（凭据），拒绝日志只记 addr 与拒绝原因。该路由不进 OpenAPI。

`POST /api/anytls/auth` — [anytls fork](https://github.com/geekdada/anytls-go/tree/feat/stats-and-http-auth) 节点 `auth.type: http` 回调。与 hysteria 回调共用同一实现核心（`node_client_auth.go` 的 `handleNodeClientAuth`），契约、状态码、`{"ok","id"}` 响应、连接元数据更新完全一致；**唯一区别**：anytls 客户端发送 `hex(sha256(password))`（64 位小写十六进制）而非原始密码，故后端按 `auth`（小写化后）查 `users.auth_string_anytls_hash` 而非 `auth_string`（见 `anytls_auth.go`）。返回的 `id` **仍回填 `auth_string`**，使 anytls 的 `/traffic` key 与采集器一致——采集器 / live / kick 因此零改动。用户的 anytls 密码即其 `auth_string`（两协议共用同一凭据）。同样不进 OpenAPI。

> 邮件走 PocketBase 内置 SMTP（`/_/` 后台配置，无新增 env）。`mailer.go` 在 `SMTP.Enabled=false` 时不发信：邀请接口仍返回 `link` 供手动分享，开放无码注册因依赖验证邮件而不可用。

## 前端（./frontend）

- 设计语言以**仓库根** `PRODUCT.md` 的 **Design Language** 为唯一来源（Linear 风、默认 follow `prefers-color-scheme`，**但提供 Light / Dark / System 手动切换**——`theme-toggle.tsx` 写 localStorage `hp:theme`，`__root.tsx` 内联 head 脚本做 SSR 防闪 + 跟随系统切换）。token 落在 `src/styles/globals.css`（覆盖 HeroUI v3 默认），改设计就改那里。`PRODUCT.md` 的 “Avoid” 列了要躲开的 AI-dashboard 套路（hero metric 卡片、卡片网格、渐变文字、玻璃拟态、UI 文案里的破折号等）。
- **类型化 API**：`src/api/client.ts` 用 `openapi-fetch` + 生成的 `schema.d.ts`（`paths`）。不要手写请求/响应类型；改后端契约后跑 `pnpm api:sync`。
- **数据获取走 TanStack Query**：query/mutation 集中在 `src/api/queries.ts`，`query-provider.tsx` 挂 client。组件别直接调 `client.ts`，复用既有 hooks。
- **i18n 走 Paraglide**：UI 文案全部来自 `~/paraglide/messages.js`（如 `m.theme_light()`），源在 `messages/{en,zh-cn}.json`。**新增文案必须两个 locale 同步加键**，否则 `pnpm i18n:check` / CI 失败；locale 由 `locale-toggle.tsx` 切换。不要在组件里硬编码可见文案。
- **鉴权**：登录直接打 PocketBase 内置 `/api/collections/users/auth-with-password`（`src/api/auth.ts`），token+record 存 cookie；`client.ts` 的 middleware **每请求**从 cookie 读 token 塞 `Authorization`（无共享模块状态，防跨请求泄漏）。SSR 安全靠 `createIsomorphicFn`：服务端读 request cookie、客户端读 document.cookie。
- **路由守卫**：`src/api/guards.ts`，在路由 `beforeLoad` 里用，与后端守卫对齐（非 admin 访问 admin 页 → 跳自己的账号页）。
- 路径别名 `~/*` → `src/*`；`VITE_API_BASE_URL` 指向后端（dev 默认 `http://localhost:8090`，见 `.env.local`）；`__APP_VERSION__` 由 vite 从根 `VERSION` 注入。

## 运行与安全

本地推荐：`cp .env.example .env` → 编辑 `PANEL_MASTER_KEY` → `direnv allow` → `make serve`（详见 `backend/README.md`）。PocketBase 不读 `.env`；变量须进入进程环境。`internal/config` 用 caarlos0/env 解析。

| 变量 | 必填 | 说明 |
|---|---|---|
| `PANEL_MASTER_KEY` | 是 | 经 SHA-256 派生 256 位 AES key，加解密节点 secret。**换 key → 已存 secret 全部解不开**，迁移环境要带原 key。生成 schema 时可传占位（`make openapi` 用 `PANEL_MASTER_KEY=skip`） |
| `PANEL_FRONTEND_URL_BASE` | 否 | 面板 UI 的 CORS 来源（`http://` / `https://`，无路径）；未设置则 `*`。写入 `apis.ServeConfig.AllowedOrigins` |
| `PANEL_BACKEND_URL_BASE` | 否 | 公开 API 来源，经 `GET /api/panel/config` 的 `api_url` 回给前端；未设置则省略该字段，前端回退同域 / 构建期配置 |
| `PANEL_SSR_API_BASE_URL` | 否 | 前端 Nitro SSR 进程访问后端的运行期地址；Docker Compose 默认 `http://backend:8090` |
| `PANEL_CORS_MAX_AGE` | 否 | 预检缓存 `Access-Control-Max-Age`（秒），默认 `7200`；`0` 关闭 |
| `PANEL_WEBAUTHN_RP_ID` | 否 | passkey 的稳定 Relying Party ID（通常是面板域名，无端口/协议）|
| `PANEL_WEBAUTHN_ORIGINS` | 否 | passkey 允许的精确前端 origin，逗号分隔。**与 `PANEL_WEBAUTHN_RP_ID` 都未设、且无静态面板 origin 时 passkey 整体禁用** |
| `PB_DATA_DIR` | 否 | PocketBase 数据目录，默认 `./pb_data`；CLI `--dir` 优先级更高 |
| `MMDB_DIR` | 否 | IP 元数据 MMDB 目录，默认 `./mmdb`，需含 `ipinfo_lite.mmdb`。缺失/损坏会让服务**启动失败**（避免静默丢 ASN/国家信息） |
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
- 已有测试覆盖 `internal/config`、`internal/ipmeta`，及 `internal/api` 的 live 聚合 / register / kick / database / users_list / recent_connections / auth_string_anytls_hash；继续补测优先 `collector.delta`（reset 边界）和 live 聚合逻辑。
- **改前端可见文案务必 `messages/en.json` 与 `messages/zh-cn.json` 同步加键**，并 `pnpm i18n:check`；新增组件不要硬编码文案。
- 字段名、collection 名、API 契约一旦定下前端会依赖，改动需同步更新 `dto.go` / OpenAPI / README 并通知前端。
