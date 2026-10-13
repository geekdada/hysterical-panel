# Hysterical Panel (backend)

轻量级 Hysteria 2 管理面板后端，基于 PocketBase（作为 Go 框架二次开发）。
只负责节点接口信息保存、自主轮询采集流量与最新在线设备数、用户管理、订阅门禁与实时诊断。不部署节点、不处理支付或账单。管理员可配置 Monitor（节点离线 / 高流量，以及用户流量不足 / 订阅即将到期）驱动 Alert 生命周期及一次性自动 Notification，也可管理加密的 Notification Channel；不提供重试队列、提醒、确认或审计日志。管理员可经 PocketBase SMTP 发送 Email Sendout（事务邮件，不可退订），由持久化队列按速率逐封投递、只发一次、失败可手动重发。

## 模型

两层：
- **users** — 既是登录面板的人（`admin` 或 `user`），也是节点认证主体。节点凭据独立保存在 `user_auth_strings`：每个 User 只有一个 Current Auth String 可认证，Retired 历史仅用于旧 Node Client ID 归属。`admin` 可管理全局资源；`user` 只能查看自己的账号诊断。`status`（`active`/`disabled`）控制启停：`disabled` 用户无法登录面板、也不再被采集器记账，并会触发一次 best-effort `/kick [user.id]` 扇出（3 并发、异步、失败仅记日志）。轮换 Auth String（`PATCH /users/{id}` 改 `auth_string` 或 `POST /users/{id}/reset-auth-string`）同样触发该扇出，断开仍用旧凭据的会话。账号「可用」= `status=active` **且** `verified=true`。成功节点鉴权会更新 `last_connected_at` 与 `recent_connections`。
- **nodes** — 一个 Hysteria 实例的接口信息（`api_url` + 加密的 `api_secret`），并保存最近一次成功 `/online` 的 Node 总设备数与观测时间。
- **online_device_counts** — 每个 User/Node 的最新正数在线客户端实例投影；不保留历史。User 总数只汇总 Enabled Node 且跨 Node 不去重，Node 总数包含未知 Node Client ID。
- **subscription_types / user_subscriptions** — 管理员定义额度与 30/360 天重置周期，授予 360 天订阅；节点鉴权要求当前窗口仍有额度。升级前的用户在首次授予前豁免，新用户（包括 admin）须由管理员授予。详见 [`docs/subscriptions.md`](../docs/subscriptions.md)。

另有两个辅助 collection：**`invitations`**（通用邀请码：`code` 唯一、`max_uses`/`expires_at`/`revoked`/`used_count`）与单例 **`app_settings`**（注册开关 `invitations_enabled` / `open_registration` / `require_invite_for_open`，默认全关；`email_sendout_rate_per_minute`，1–600，默认 30）。

所有 enabled 节点默认对所有用户生效（`nodesForUser()` 是唯一选择点，将来加用户组只改这里）。

## 运行

环境变量由进程环境提供（PocketBase **不会**自动读 `.env`）。推荐用 [direnv](https://direnv.net/) + `.env` 本地文件：

```bash
cp .env.example .env
# 编辑 .env，设置 PANEL_MASTER_KEY

direnv allow   # 进入 backend/ 目录时自动加载 .env
make serve     # 或: go run . serve
```

未安装 direnv 时，可 `export $(grep -v '^#' .env | xargs)`，或依赖 Makefile 对 `.env` 的 `include`（仅 `make` 目标生效）。

| 变量 | 必填 | 说明 |
|------|------|------|
| `PANEL_MASTER_KEY` | 是 | 节点 `api_secret` 与 OIDC 签名私钥的 AES-GCM 主密钥 |
| `PANEL_FRONTEND_URL_BASE` | 否 | 面板 UI 的 CORS 来源（`http://` 或 `https://`，不含路径）；未设置则允许 `*` |
| `PANEL_BACKEND_URL_BASE` | 否 | 后端公开地址，经 `GET /api/panel/config` 回给前端；OIDC issuer 也由它派生 |
| `PANEL_CORS_MAX_AGE` | 否 | 预检 `Access-Control-Max-Age`（秒），默认 `7200`；`0` 表示不发送 |
| `PB_DATA_DIR` | 否 | PocketBase 数据目录，默认 `./pb_data`；CLI `--dir` 优先级更高 |
| `MMDB_DIR` | 否 | IP 元数据 MMDB 目录，默认 `./mmdb`，需包含 `ipinfo_lite.mmdb` |
| `PB_ENCRYPTION_KEY` | 否 | PocketBase 设置库加密密钥，须为 **32 字符**；未设置则设置库明文存储 |

`PANEL_MASTER_KEY` 由 `internal/config`（[caarlos0/env](https://github.com/caarlos0/env)）解析，未设置则拒绝启动；`PB_DATA_DIR` / `PB_ENCRYPTION_KEY` 在 `main.go` 注入 `pocketbase.NewWithConfig`。`MMDB_DIR` 用于实时诊断的 Top domains IP 元数据；缺失或损坏会让服务启动失败，避免静默丢失 ASN / 国家信息。

首次启动按提示创建 superuser，然后进入 PocketBase 后台 `/_/` 的 `users` collection 创建首个面板用户：设置 email/password、`role=admin`、`status=active`、`verified=true`。User create request hook 会在同一事务中自动生成其 Current Auth String；凭据创建失败时 User 也会回滚。此入口仅用于后台引导，客户端不能指定 Auth String。迁移在 `./migrations` 中以代码形式提交，启动时自动应用。

## Docker

发布后的后端镜像位于 GHCR：

```bash
docker run --rm \
  -p 8090:8090 \
  -e PANEL_MASTER_KEY=change-me \
  -v hysterical-panel-data:/app/pb_data \
  ghcr.io/geekdada/hysterical-panel-backend:1.2.3
```

镜像监听 `0.0.0.0:8090`，数据目录为 `/app/pb_data`。`PANEL_MASTER_KEY`
仍为必填；`PB_ENCRYPTION_KEY` 可按需传入。镜像只会在 GitHub Release
发布后由 CI 构建并推送，普通提交、PR 和 tag push 都不会触发发布镜像。

放在反向代理后面时，要在 `/_/` 的 Settings → Application → User IP proxy
headers 里填代理设置的头（仓库自带的 `deploy/nginx/default.conf` 用 `X-Real-IP`）。
PocketBase 默认不信任这些头，此时所有请求的来源 IP 都是代理地址。注册、passkey
和 `/api/oidc/*` 的按 IP 限流就变成所有人共用一份额度，一个人刷满后别人也会收到
429。直接对外暴露、前面没有代理时不要填，否则客户端可以伪造这个头绕过限流。

## 采集器

后台 goroutine 每 5s 调度，按各节点 `poll_interval` 并发轮询 `GET /traffic` 与 `GET /online`：
- counter-to-delta：处理 Hysteria 重启导致的计数器归零
- 累加到 `users.used_tx/rx` 与 `traffic_hourly` / `traffic_daily`
- 同一事务内按发送和接收之和结算当前订阅窗口；耗尽后拒绝新连接并异步 Kick
- 失败写 `node.last_error` 且不更新 cursor（漏采一轮不丢量）
- `/online` 成功时事务替换该 Node 的最新用户计数并更新 Node 总数；失败保留旧值，不改变 Traffic health

`/traffic` 与 `/online` 独立提交，任一失败不阻止另一项成功结果落库。`/dump/streams` 不进采集循环，由 live 接口实时穿透拉取；live 不再请求或返回在线设备数。live 的 Top domains 只对已经是 IP 字面量的目标做本地 MMDB 查询，补充 ASN/网络标签、国家信息与 IPv4 的 `ipinfo.io` 快速链接；不会做 DNS 解析，也不会入库。

## 接口（前缀 `/api/panel/`，需登录；除标注外需 admin）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/nodes` | 列表（含 health，不含 secret） |
| POST | `/nodes` | 新建（secret 加密存储） |
| PATCH | `/nodes/{id}` | 修改（secret 缺省=不变，传空=报错） |
| DELETE | `/nodes/{id}` | 删除 |
| POST | `/nodes/{id}/test` | 立即验证连通性 |
| GET | `/nodes/{id}/current-speed` | 节点当前速度与速度最高的 10 个用户（只列非零项，按 tx+rx 降序） |
| GET | `/traffic` | 全局流量汇总（admin）；必填 `from` / `to`（UTC 日桶，含首尾，与 series 同格式） |
| GET | `/users` | 分页列表；`?page=1&per_page=25&search=&sort=created`（search 对 email/remark/role/status 子串匹配，auth_string 仅完整相等）→ `{ items, total, page, per_page }`；每个 item 附 `current_subscription`（覆盖当前时刻的授予，无则 `null`），终身 `used_tx` / `used_rx` 只对未授予过订阅的旧豁免用户返回（其余为 `null`）。`sort=[-]subscription_used|subscription_tx|subscription_rx` 按当前窗口用量排序，无当前订阅的用户无论升降序都排在最后（按创建时间） |
| GET | `/users/stats` | 用户总数与 active 数 `{ total, active }` |
| POST | `/users` | 新建（email+password+auth_string） |
| GET | `/users/{id}` | 详情（admin 或本人） |
| GET | `/subscription-types` | 管理员列出订阅类型 |
| POST / PATCH / DELETE | `/subscription-types`、`/subscription-types/{id}` | 管理员创建、修改、删除尚未授予的类型；已授予过的类型只可隐藏或修改额度 |
| GET | `/users/{id}/subscriptions` | 授予记录；当前授予附本窗口用量 `used_bytes` = `used_tx_bytes` + `used_rx_bytes`，每份授予附整期用量 `grant_tx_bytes` / `grant_rx_bytes`（admin 或本人） |
| POST | `/users/{id}/subscriptions` | 管理员授予一份订阅，当前已有一份时排队到其到期时间 |
| POST / DELETE | `/users/{id}/subscriptions/{subscriptionId}/top-up`、`/users/{id}/subscriptions/{subscriptionId}` | 管理员给当前窗口增加 `allowance_bytes` 字节，或终止当前/排队订阅 |
| POST | `/users/{id}/subscriptions/{subscriptionId}/reschedule` | 管理员改期当前订阅：`{ starts_at }`（UTC，不晚于 now，改期后仍覆盖 now，不早于上一份自然到期订阅的 `ends_at`，可与已终止的订阅重叠），排队订阅同步平移；本窗口用量与加额随之保留；返回全部授予记录 |
| PATCH/DELETE | `/users/{id}` | 改/删；停用或修改 `auth_string` 时异步 Kick |
| POST | `/users/{id}/reset-auth-string` | 随机生成新的 Current Auth String 并返回，旧值转 Retired，异步 Kick（active 的 admin 或本人） |
| GET | `/users/{id}/traffic/summary` | 当日（UTC）用量，按节点拆分（admin 或本人） |
| GET | `/users/{id}/traffic/series` | 趋势 `?granularity=hourly\|daily&from=&to=&node=`（admin 或本人；`from`/`to`/`bucket` 均为 **UTC**） |
| GET | `/users/{id}/live` | 实时 streams 诊断（admin；活跃流、域名榜、客户端连接维度） |
| GET / DELETE | `/users/{id}/oauth-links`、`/users/{id}/oauth-links/{linkId}` | 列出 / 取消该用户的 OAuth2 社交账号关联（active 的 admin 或本人） |
| GET | `/users/{id}/current-speed` | 当前速度（最近一个采集周期的平均 B/s），按可见节点拆分，只列非零项（admin 或本人） |
| GET | `/settings` | 读取注册/邀请开关 |
| PATCH | `/settings` | 改开关（层级校验：`invitations_enabled=true` 需 `open_registration=true`；`require_invite_for_open=true` 需 `invitations_enabled=true`，否则 400）；也改 `email_sendout_rate_per_minute`（1–600）与 `oidc_enabled`（见下方 OIDC Provider） |
| GET/POST | `/oidc-clients` | 列出 Connected App / 新建（`client_secret` 只在响应里出现一次） |
| PATCH/DELETE | `/oidc-clients/{id}` | 修改名称、redirect URI、scope、refresh、跳过同意页、启停 / 删除（连同它的授权、code 与 token） |
| POST | `/oidc-clients/{id}/rotate-secret` | 轮换 client secret，新明文只返回一次。已签发的 access token 在 10 分钟内仍有效，刷新须用新 secret；secret 泄露时要立即切断请停用该应用 |
| GET | `/oidc/requests/{handle}` | 同意页读取待处理的授权请求（任意已登录且可用的 User；提供方关闭时 404） |
| POST | `/oidc/requests/{handle}/approve` | 同意页同意，返回跳回应用的 `redirect_to`（任意已登录且可用的 User） |
| POST | `/oidc/requests/{handle}/deny` | 同意页拒绝，返回带 `access_denied` 的 `redirect_to`（只要求已登录） |
| POST | `/oidc/requests/{handle}/abort` | **公开**。未登录的 User 结束授权请求：`prompt=none` 返回 `login_required`，其余返回 `access_denied`。持有 handle 即可调用，与 `/authorize` 共用每 IP 每分钟 60 次额度 |
| GET / DELETE | `/users/{id}/oidc-grants`、`/users/{id}/oidc-grants/{grantId}` | 列出 / 撤销该用户已授权的 Connected App（active 的 admin 或本人） |
| GET/POST | `/notification-channels` | 管理员列出非秘密 Channel 元数据 / 新建单 URL Channel（默认 disabled） |
| PATCH/DELETE | `/notification-channels/{id}` | 修改名称、启停或替换 URL / 永久删除 Channel |
| POST | `/notification-channels/{id}/test` | 发送固定测试消息（禁用 Channel 也可；结果只返回安全状态） |
| POST | `/notification-channels/{id}/reveal/options`、`/reveal/finish` | 经新鲜 passkey assertion reveal 一个已保存 URL |
| GET | `/invitations` | 邀请码列表（含有效性、`link`） |
| POST | `/invitations` | 新建邀请码（可选 `email`/`max_uses`/`expires_in_hours`/`note`/`send_email`；`invitations_enabled=false` 时 400） |
| DELETE | `/invitations/{id}` | 删除邀请码 |
| GET | `/ignored-connection-ips` | 全局忽略的客户端 IP 列表（用于最近连接统计） |
| POST | `/ignored-connection-ips` | 忽略一个 IP（body `{ "ip": "1.2.3.4" }`；已存在则幂等返回） |
| DELETE | `/ignored-connection-ips/{id}` | 取消忽略 |
| GET | `/email-sendouts/context` | 写信页所需：`app_name`、`frontend_url`、`smtp_enabled`、可用 User 数、发送速率 |
| GET | `/email-sendouts/eligible-recipients?search=` | 按邮箱子串查可用 User（`status=active && verified=true`），最多 20 条 |
| GET | `/email-sendouts` | Email Sendout 列表（新到旧，含各状态计数与推导出的 `status`） |
| POST | `/email-sendouts` | 创建 Sendout 并入队（SMTP 未启用 503） |
| GET | `/email-sendouts/{id}` | Sendout 详情，含原样存储的 `html` / `text` |
| GET | `/email-sendouts/{id}/recipients?status=` | Sendout Recipient 列表 |
| POST | `/email-sendouts/{id}/cancel` | 取消尚未发出的收件人 |
| POST | `/email-sendouts/{id}/resend` | 把失败的收件人重新排到队尾（body `{ "recipient_ids": [] }`，空 = 全部失败项；SMTP 未启用 503） |

所有返回 node 的接口都已剥除 `api_secret`。

### 通知 Channel

通知 Channel 仅由 admin 管理；每条记录只保存一个完整 Shoutrrr URL，使用 `PANEL_MASTER_KEY` AES-GCM 加密后落库。普通列表、创建和更新响应均不返回 URL 或密文。新建默认 disabled，替换 URL 会清空上一次测试结果；disabled Channel 仍可发送显式的固定测试消息。没有通知规则、后台发送、重试、队列或历史日志。

服务白名单与 [Beszel 通知指南](https://beszel.dev/zh/guide/notifications/) 对齐：Generic、Bark、Discord、Gotify、Google Chat、IFTTT、Join、Lark、Mattermost、Matrix、MQTT、ntfy、OpsGenie、Pushbullet、Pushover、Rocket.Chat、Signal、Slack、Teams、Telegram、Twilio、WeCom、Zulip。依赖固定为 `github.com/nicholas-fedor/shoutrrr v0.16.1`；自托管私网目标允许。测试超时或投递失败只记录 `timed_out` / `delivery_failed`，绝不回显 URL、凭据、headers、response body 或原始 provider error。

已保存 URL 的 reveal 需要当前 admin 已注册 passkey，并针对该 Channel 完成一枚新鲜、5 分钟有效、用后即焚的 WebAuthn assertion。其 session 使用可复用的 `sensitive_field_reveal` kind，并以隐藏 `scope=<channel id>` 绑定目标；断言只可 reveal 该一条 URL。未配置 passkey 时 admin 仍可替换、测试、启停和删除 Channel。

### Email Sendout

邮件 HTML 与纯文本由前端 `@react-email/editor` 生成（含固定模板外框），后端只校验大小并原样存储、原样发送，没有后端模板（ADR 0008）。创建时在同一事务里为每个可用 User 写一行 `email_sendout_recipients`；「全部」按创建瞬间展开。进程内单 worker 按 `queued_at` FIFO 逐封发送，间隔 `60s / app_settings.email_sendout_rate_per_minute`（1–600，默认 30）。每行只尝试一次：发送前重查资格（不可用 → `skipped`），用 User 当时的邮箱；SMTP 失败或未启用 → `failed`。进程重启时遗留的 `sending` 行转 `failed`（`interrupted`），不会自动重发。取消只影响 `pending` 行；重发只影响 `failed` 行。历史永久保留。若数据库在一行被 claim 后、或在写最终状态时失败，该行会停留在 `sending`，只在下次重启时才被标记为 `failed`（`interrupted`）。PocketBase 的 SMTP client 没有发送超时，一个接受连接后卡住不响应的 relay 会阻塞这个单一 worker 直到重启。

**时间**：数据库存储与 API 中的 datetime 一律 **UTC**（流量按 UTC 小时/日分桶）。前端自行换算为本地时区展示；查询 `traffic/series` 时 `from`/`to` 也传 UTC。

## 公共接口（无需登录）

### 自助注册 `POST /api/panel/register`

请求体 `{ email, password, code? }`。访问与「是否需邀请码」由 `app_settings` 实时决定（见 `registrationDecision`）。三开关是严格层级：`open_registration` 是总开关，`invitations_enabled` 依赖它，`require_invite_for_open` 依赖 `invitations_enabled`：

- 关闭（`open_registration=false`）→ 403（无论邀请系统是否开启）。
- 开放（`open_registration=true`）→ `require_invite_for_open` 决定是否需 `code`（要求 `code` 时须 `invitations_enabled=true`）。

新用户固定 `role=user`、`status=active`、`auth_string` 由系统随机生成（客户端无法指定 role/auth_string/status）。`verified := 是否经邀请码`：

- 经邀请码 → `verified=true`，响应含 `token`+`record`（自动登录）。
- 开放且无码 → `verified=false`，发邮箱验证信（链接指向前端 `/verify?token=`，由其调 PocketBase 内置 `POST /api/collections/users/confirm-verification` 完成），响应 `{requires_verification:true}`，**不**自动登录；该路径依赖 SMTP，未配置 SMTP 时直接返回 503。

注册端点限流（按 IP），不进 `openapi.json` 的 secured 范围（标记为无需鉴权）。

### 找回密码（PocketBase 内置端点，无自定义后端代码）

前端 `/forgot-password` 直接调 PocketBase 内置 `POST /api/collections/users/request-password-reset`（请求体 `{email}`），无论邮箱是否存在都返回 `204`（内置反枚举 + 单账号 2 分钟重发节流）。重置确认页 `/reset-password?token=` 调内置 `POST /api/collections/users/confirm-password-reset`（请求体 `{token,password,passwordConfirm}`）；成功后若 token 内邮箱与记录一致还会顺带置 `verified=true`。这两个内置端点不触发 `OnRecordAuthRequest`，故不受 `status`/`verified` 门禁阻挡（`disabled` 用户可重置但仍无法登录）。

**部署需做一次配置**：默认重置邮件链接指向 PocketBase 后台（`{APP_URL}/_/#/auth/confirm-password-reset/{TOKEN}`）。要让链接落到前端重置页，请在 `/_/` 后台把 **Settings → Application URL** 设为前端域名，并把 users collection 的 **Reset password** 邮件模板链接改成 `{APP_URL}/reset-password?token={TOKEN}`。同样依赖 SMTP；未配置 SMTP 则找回密码不可用。

### 邮箱验证码登录（PocketBase OTP，免密码）

在 `/_/` 后台 users collection 的 **Options → One-time password (OTP)** 启用，面板不新增环境变量。这里是单独使用 OTP 登录，不是 MFA；users collection 开着 MFA 时 OTP 会变成第二因素，登录页的验证码入口无法单独完成登录。前端登录页读内置 `GET /api/collections/users/auth-methods` 的 `otp.enabled` 决定是否显示「通过邮箱验证码登录」，流程分两步：内置 `POST /api/collections/users/request-otp`（`{email}`）返回 `otpId` 并发信，再调内置 `POST /api/collections/users/auth-with-otp`（`{otpId,password:<验证码>}`）换取与密码登录同形的 token+record。

- 验证码长度、有效期与邮件模板都在同一处 OTP 选项里配置。PocketBase 对每个账号限制 180 秒内最多 5 次验证，验证码用过即删。
- `OnRecordRequestOTPRequest("users")`（`otp_login.go`）：SMTP 未启用时返回 503 `email sign-in is unavailable: SMTP is not configured`，因为 sendmail 兜底会静默丢信；`status` 非 active 的用户与不存在的邮箱一样拿到假 `otpId`、不发信，响应不暴露账号是否存在。
- 登录仍过 `bindAuthGate` 的 `status` / `verified` 门禁。未验证用户可以收到验证码，用验证码登录时 PocketBase 会置 `verified=true` 并重置其密码（防预注册劫持），之后可经找回密码重新设置。
- 有 passkey 的账号也能用验证码登录（密码登录对其关闭）。

### OAuth2 登录（只登录不注册）

provider 在 `/_/` 后台 users collection 的 **Options → OAuth2** 里启用和配置，面板不新增环境变量。前端登录页调内置 `GET /api/collections/users/auth-methods` 列出已启用的 provider，点击后跳转到 provider 授权页。provider 回跳后端公开的 `GET|POST /api/panel/oauth2-redirect`，它只把 `code` / `state` / `error` 以 303 转发到 `PANEL_FRONTEND_URL_BASE + /oauth-callback`（未配置则为相对路径 `/oauth-callback`，只适用于前后端同域）。前端校验 `state` 后调内置 `POST /api/collections/users/auth-with-oauth2`（带 PKCE `codeVerifier`）换取与密码登录同形的 token+record。

- **只登录已有账号**：`OnRecordAuthWithOAuth2Request("users")`（`oauth2_login.go`）拒绝新建用户，返回 403 `no account matches this sign-in`。PocketBase 先按已绑定的 OAuth2 身份匹配，再按 provider 返回的邮箱匹配；首次按邮箱匹配成功后自动绑定。新用户仍只能经面板注册策略、admin 建号或 Management API 产生。
- 仍过 `bindAuthGate` 的 `status` / `verified` 门禁。未验证用户经邮箱匹配登录时，PocketBase 会置 `verified=true` 并重置其密码。
- **部署需做一次配置**：在每个 provider 的开发者后台把回调地址登记为 `<后端公开地址>/api/panel/oauth2-redirect`（后端公开地址即 `PANEL_BACKEND_URL_BASE`，同域反代时就是面板域名）。不要用 PocketBase 后台提示的 `/api/oauth2-redirect`，它只服务 JS SDK 的 realtime 弹窗流程。前后端不同源时必须设置 `PANEL_FRONTEND_URL_BASE`，且用户要从这个前端地址发起登录，因为 PKCE verifier 存在该 origin 的 sessionStorage 里。
- Apple 以 `response_mode=form_post` 回跳，由转发路由的 POST 分支接收。
- **主动关联**：设置页对每个尚未关联的已启用 provider 显示「关联」按钮。流程与登录相同，区别是前端交换 code 时带上当前 session token。PocketBase 先按已绑定身份匹配，没有命中就直接用 token 对应的用户，不再按邮箱查找，所以邮箱不同也能关联。该身份已绑定到其他用户时，`oauth2_login.go` 返回 409 `this social account is linked to another user`，避免 PocketBase 切换成那个账号登录。前端在交换前刷新 token，token 已失效则不发请求。每个用户每个 provider 最多一条关联（PocketBase 唯一索引）。
- 设置页（本人）与用户详情页（admin）的「社交账号登录」区域经 `GET|DELETE /api/panel/users/{id}/oauth-links` 列出 / 取消关联（`oauth2_links.go`，`requireActiveAdminOrSelf`）。不用内置 `_externalAuths` record API，因为它的默认规则只允许本人访问。取消关联后，如果该社交账号邮箱与面板邮箱相同，再次用它登录会重新按邮箱关联。PocketBase 未启用 OAuth2 或没有已启用的 provider 时，两处区域整体隐藏，也不请求关联列表。已停用 provider 的旧关联无法登录，需要清理时在 `/_/` 删除。
- 账号匹配依赖 provider 返回的邮箱可信，只启用会校验邮箱所有权的 provider。

users collection 的 List / View / Create / Update / Delete API 规则由迁移 `1730000030` 锁为仅 superuser。PocketBase 默认规则允许匿名建号、允许用户改自己的 `role` / `status` / 用量字段；锁定后 OAuth2 也无法经通用建号接口创建用户。面板只走 `/api/panel/*` 与内置 auth 端点，不依赖这些规则。

### Hysteria 回调 `POST /api/hysteria/auth`（供 Hysteria 节点调用）

给 Hysteria 2 节点的 `auth.type: http` 用，每次客户端连接时由节点回调。请求体形如 `{"addr":"1.2.3.4:5678","auth":"<current-auth-string>","tx":1000000}`；后端只按 `user_auth_strings` 中的 Current Auth String 匹配，命中且 `status=active` 且 `verified=true` 时返回 `200 {"ok":true,"id":"<user.id>"}`，其它返回非 200。稳定 User ID 会用于节点后续 `/traffic`、`/online`、`/dump/streams` 与 `/kick`；Retired Auth String 不能认证，但仍可把迁移前连接的统计归属回 User。成功鉴权会异步更新连接元数据。

| 状态码 | 含义 |
|---|---|
| 200 | 验证通过，body 含 `ok` 与 `id` |
| 400 | 请求体缺失 `auth` 或不是合法 JSON |
| 401 | `auth` 不匹配任何 Current Auth String |
| 403 | 用户存在但 `status=disabled` 或 `verified=false` |

节点侧 `server.yaml` 示例：
```yaml
auth:
  type: http
  http:
    url: http://<panel-base-url>/api/hysteria/auth
    insecure: false
```

该路由**不**进 `openapi.json`：它由 Hysteria 节点调用，不是前端 client 的一部分。

### anytls 回调 `POST /api/anytls/auth`（供 anytls 节点调用）

给 [anytls fork](https://github.com/geekdada/anytls-go/tree/feat/stats-and-http-auth) 的 `auth.type: http` 用。契约与 `/api/hysteria/auth` 完全一致，唯一区别是 AnyTLS 客户端发送 `hex(sha256(password))`；后端只匹配 Current `user_auth_strings.auth_string_anytls_hash`。成功同样返回 `user.id`，Retired hash 不再认证。原始 Auth String 与派生 hash 都保存在内部 `user_auth_strings` history 中，普通 API 只返回 Current 值。

节点侧 `server.yaml` 示例：
```yaml
auth:
  type: http
  http:
    url: http://<panel-base-url>/api/anytls/auth
    insecure: false
```

该路由同样**不**进 `openapi.json`。

### 管理接口（Management API）`/api/mgmt/*`（供外部系统调用）

供**外部系统**（非面板前端）通过共享 bearer token 查询与创建用户。**默认关闭**，需管理员在 `/settings` 页面启用。启用时由服务器自动生成 token（明文仅显示一次，需立即复制保存），token 经 SHA-256 哈希存储，无法回读；管理员可随时 rotate。与 `/api/hysteria/auth` 一样不进 `openapi.json`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/mgmt/users?id=`、`?email=` 或 `?auth_string=` | 按 User ID、email 或 Current `auth_string` 精确查询单个用户（三选一，不能同时给）；Retired 值不匹配。Connected App 拿到的 `sub` 就是 User ID，可直接用 `?id=` 查。返回 `PanelUser` 形状（含 Current `auth_string`） |
| POST | `/api/mgmt/users` | 请求体 `{ email }`，自动生成密码与 `auth_string`（不返回）。返回 `{ id, email, status }`，状态码 **201**。创建后用户需走找回密码流程拿到登录凭证 |

所有 `/api/mgmt/*` 请求必须带 `Authorization: Bearer <token>`。功能未启用时返回 **404**（避免泄露表面存在）；token 缺失或不匹配返回 **401**。

启用方式：`PATCH /api/panel/settings`，置 `management_api_enabled=true`；首次启用时服务器自动生成 token 并在响应 `management_api_token` 字段返回明文（仅此一次）。响应中 `management_api_token_set` 标识当前是否已配置 token。需要更换 token 时调 `POST /api/panel/management-api/rotate`，返回新的明文 token（同样仅一次）。

> 邮件（邀请信、邮箱验证信）走 PocketBase 内置 SMTP（在 `/_/` 后台 Settings → Mail 配置），无新增环境变量。面板自己发的邮件（邀请信、验证信、Email Sendout）发件人名取 **Settings → Application Name**，为空才回退到 Mail 设置里的 Sender name；发件地址仍是 Sender address。PocketBase 内置模板邮件（找回密码、OTP 验证码）照旧用 Sender name。未配置 SMTP 时邀请接口仍返回 `link` 供手动分享，但开放无码注册因依赖验证邮件而不可用。找回密码邮件同样走内置 SMTP，但用的是 PocketBase 自带的 **Reset password** 模板（需按上文把链接改指向前端 `/reset-password`），不经 `mailer.go`。

## OIDC Provider（Connected App 登录）

面板可以作为 OpenID Connect 提供方，让 admin 登记的 Connected App 用面板账号登录。Connected App 只面向自己运营的站点（ADR-0010），不要登记第三方应用。决策与取舍见 [ADR-0010](../docs/adr/0010-oidc-provider.md)，conformance 结果见 [`docs/oidc-conformance.md`](../docs/oidc-conformance.md)。

只支持一种流程：Authorization Code + PKCE `S256`，客户端必须是持有 secret 的 confidential client（`client_secret_basic` 或 `client_secret_post`）。不支持 public client、implicit、`client_credentials`、动态注册、`request` / `request_uri`、introspection。

### 启用

1. 设置 `PANEL_BACKEND_URL_BASE` 与 `PANEL_FRONTEND_URL_BASE`。issuer 是 `PANEL_BACKEND_URL_BASE + "/api/oidc"`，必须是 https；只有 `localhost`、`127.0.0.1`、`::1` 这类 loopback 地址允许 http，供本地开发。同意页由前端渲染，所以前端地址也必须配置。启用时只检查后端地址的 https，前端地址同样应该用 https，因为同意页带着面板登录态。
2. admin 在面板「设置」里打开 OIDC 开关（`PATCH /settings` 置 `oidc_enabled=true`）。打开时生成签名密钥并固定当前 issuer。
3. 在「设置 → Connected Apps」登记应用，填精确的 redirect URI，保存后复制只显示一次的 client secret。

关闭开关后 `/api/oidc/*` 全部返回 404。issuer 固定后如果 `PANEL_BACKEND_URL_BASE` 变了，提供方也会停止应答，设置页显示 `issuer_changed`。确认要换地址时先关再开，Connected App 也要改用新 issuer。

### 端点（前缀 `/api/oidc`，不进 OpenAPI）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/.well-known/openid-configuration` | Discovery |
| GET | `/jwks` | 签名公钥（RS256） |
| GET / POST | `/authorize` | 校验请求后 303 到前端同意页 `/oidc/authorize?request=...`，错误按 RFC 6749 跳回应用或跳到前端 `/oidc/error` |
| POST | `/token` | `authorization_code` 与 `refresh_token` |
| GET / POST | `/userinfo` | access token 只从 `Authorization: Bearer` 头读取，返回 `sub`、`email`、`email_verified` |
| POST | `/revoke` | RFC 7009，撤销 access 或 refresh token |

限流按 IP 计。`/authorize` 每分钟 60 次，超出时跳到前端 `/oidc/error?reason=rate_limited`。`/token`、`/userinfo`、`/revoke` 共用每分钟 300 次，超出返回 429。client 认证失败另有每分钟 10 次的额度，用完后错误的 secret 收到 429，正确的 secret 仍放行，但仍受 300 次总额度限制。放在反向代理后面时按上文「Docker」一节配置 User IP proxy headers。

### 应用侧配置

- scope：`openid`（必需）、`email`、`offline_access`。未知 scope（如 `profile`）直接忽略、不报错，实际授予的 scope 见 token 响应的 `scope`。
- `sub` 是 User ID。需要 Current Auth String 或账号状态时，应用服务端用 Management API `GET /api/mgmt/users?id=<sub>` 查。Management API token 是全局的，能读任意 User 的 Current Auth String，也能建号。只把它交给你自己控制的服务端，不要给第三方，也不要放进浏览器。
- 只有 `status=active` 且 `verified=true` 的 User 能登录，授权、换 token、刷新和 userinfo 每一步都会重新检查。User 被停用或取消验证时，保存后会尽力撤销它的全部授权，`/_/` 后台的修改同样生效。撤销失败只记日志、不阻止保存；停用期间所有 token 照样被拒。改密码或删 passkey 不撤销授权。
- 同意按 User + 应用 + scope 记住，scope 变多时再问。应用可以设置为跳过同意页，默认关闭；请求带 `prompt=consent` 或 `prompt=select_account` 时仍显示同意页。用户在「设置 → 已授权的应用」里撤销。
- `prompt=login` 和 `max_age` 会让用户回到面板登录页重新登录，任何登录方式都算；`auth_time` 取自这次真实登录的时间。`prompt=none` 未登录时返回 `login_required`，需要同意时返回 `consent_required`。
- 回调参数带 RFC 9207 的 `iss`。
- refresh token 只在应用开启 refresh 且请求带 `offline_access` 时签发，每次使用都轮换；重放已轮换的 refresh token 或已用过的 code 会撤销同一授权链上的所有 token。

固定有效期：授权请求 10 分钟，code 60 秒，access token 与 ID token 10 分钟，refresh token 闲置 30 天、最长 90 天。每 30 分钟一次的 cron 清理：过期的授权请求与登录时间记录立即删除；过期的 code 与 token 再保留 7 天，让迟到的重放仍能找到记录并撤销整条 family；退役的签名密钥在退役 24 小时后删除。已撤销的授权记录不清理。

### 签名密钥

私钥用 `PANEL_MASTER_KEY` 加密存储。面板里没有手动轮换入口，用 CLI。命令必须带服务端相同的 `PANEL_MASTER_KEY` 和数据目录（`PB_DATA_DIR` 或 `--dir`），否则会在另一个空库上「成功」：

```bash
./hysterical-panel oidc rotate-signing-key
# Docker 镜像里：docker exec <容器> hysterical-panel oidc rotate-signing-key
```

新签发的 ID token 立即使用新 `kid`。`/jwks` 带 `Cache-Control: max-age=300`，遇到未知 `kid` 不重新拉取 JWKS 的应用最多会失败 5 分钟。旧公钥在 JWKS 里保留 24 小时，之前签发的 ID token 仍能验证。`--drop-previous` 立即移除所有旧公钥，包括刚被替换的那把。

换了 `PANEL_MASTER_KEY` 后不需要手动轮换：服务端在下次签名或请求 JWKS 时发现旧私钥解不开，会自动退役它并生成新密钥。CLI 遇到解不开的密钥会拒绝执行，这通常说明当前 shell 的 `PANEL_MASTER_KEY` 或数据目录与服务端不一致。先核对，确认无误再考虑 `--force`。

## 结构

```
main.go                     启动：migration + 采集器 + 路由
migrations/                 collection 定义（users 扩展 / nodes / traffic_* / passkeys / invitations / app_settings）
internal/config/            环境变量（caarlos0/env）
internal/authstrings/       Current/Retired Auth String 与 Node Client ID 解析
internal/cryptobox/         AES-GCM 加解密（主密钥来自 config）
internal/token/             URL-safe 随机 token（邀请码 / auth_string）
internal/hysteria/          Traffic Stats API 客户端
internal/notifications/     Beszel-compatible Shoutrrr URL 校验与单 Channel 测试投递
internal/collector/         counter-to-delta 采集核心
internal/oidc/              OIDC Provider：授权请求、同意、code / token、签名密钥、清理、`oidc rotate-signing-key` CLI
internal/api/               /api/panel 路由：nodes / users / traffic / live / settings / invitations / notification-channels / register
```
