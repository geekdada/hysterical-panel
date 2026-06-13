# Hysterical Panel (backend)

轻量级 Hysteria 2 管理面板后端，基于 PocketBase（作为 Go 框架二次开发）。
只负责节点接口信息保存、自主轮询采集流量、用户管理与实时诊断。不部署节点、不做订阅计费。

## 模型

两层：
- **users** — 既是登录面板的人（`admin` 或 `user`），也是 Hysteria 认证的账号（`auth_string` 字段，与登录 email 独立）。`admin` 可管理全局资源；`user` 只能查看自己的账号诊断。`status`（`active`/`disabled`）控制启停：`disabled` 用户无法登录面板、也不再被采集器记账（但不会断开其 Hysteria 连接——面板不踢节点）。账号「可用」= `status=active` **且** `verified=true`：`verified` 是登录与 Hysteria 鉴权的附加门禁（admin 建号与邀请码注册者恒为 `verified=true`，仅开放无码注册者需先验证邮箱）。
- **nodes** — 一个 Hysteria 实例的接口信息（`api_url` + 加密的 `api_secret`）。

另有两个辅助 collection：**`invitations`**（通用邀请码：`code` 唯一、`max_uses`/`expires_at`/`revoked`/`used_count`）与单例 **`app_settings`**（注册开关 `invitations_enabled` / `open_registration` / `require_invite_for_open`，默认全关）。

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
| `PANEL_MASTER_KEY` | 是 | 节点 `api_secret` 的 AES-GCM 主密钥 |
| `PANEL_FRONTEND_URL_BASE` | 否 | 面板 UI 的 CORS 来源（`http://` 或 `https://`，不含路径）；未设置则允许 `*` |
| `PANEL_CORS_MAX_AGE` | 否 | 预检 `Access-Control-Max-Age`（秒），默认 `7200`；`0` 表示不发送 |
| `PB_DATA_DIR` | 否 | PocketBase 数据目录，默认 `./pb_data`；CLI `--dir` 优先级更高 |
| `MMDB_DIR` | 否 | IP 元数据 MMDB 目录，默认 `./mmdb`，需包含 `Country-asn.mmdb` 与 `Country-without-asn.mmdb` |
| `PB_ENCRYPTION_KEY` | 否 | PocketBase 设置库加密密钥，须为 **32 字符**；未设置则设置库明文存储 |

`PANEL_MASTER_KEY` 由 `internal/config`（[caarlos0/env](https://github.com/caarlos0/env)）解析，未设置则拒绝启动；`PB_DATA_DIR` / `PB_ENCRYPTION_KEY` 在 `main.go` 注入 `pocketbase.NewWithConfig`。`MMDB_DIR` 用于实时诊断的 Top domains IP 元数据；缺失或损坏会让服务启动失败，避免静默丢失 ASN / 国家信息。

首次启动按提示创建 superuser（PocketBase 后台 `/_/`）。迁移在 `./migrations` 中以代码形式提交，启动时自动应用。

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

## 采集器

后台 goroutine 每 5s 调度，按各节点 `poll_interval` 轮询 `GET /traffic`：
- counter-to-delta：处理 Hysteria 重启导致的计数器归零
- 累加到 `users.used_tx/rx` 与 `traffic_hourly` / `traffic_daily`
- 失败写 `node.last_error` 且不更新 cursor（漏采一轮不丢量）

`/online` 与 `/dump/streams` 不进采集循环，由 live 接口实时穿透拉取。live 的 Top domains 只对已经是 IP 字面量的目标做本地 MMDB 查询，补充 ASN/网络标签、国家信息与 IPv4 的 `ipinfo.io` 快速链接；不会做 DNS 解析，也不会入库。

## 接口（前缀 `/api/panel/`，需登录；除标注外需 admin）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/nodes` | 列表（含 health，不含 secret） |
| POST | `/nodes` | 新建（secret 加密存储） |
| PATCH | `/nodes/{id}` | 修改（secret 缺省=不变，传空=报错） |
| DELETE | `/nodes/{id}` | 删除 |
| POST | `/nodes/{id}/test` | 立即验证连通性 |
| GET | `/traffic` | 全局流量汇总（admin）；必填 `from` / `to`（UTC 日桶，含首尾，与 series 同格式） |
| GET | `/users` | 列表 |
| POST | `/users` | 新建（email+password+auth_string） |
| GET | `/users/{id}` | 详情（admin 或本人） |
| PATCH/DELETE | `/users/{id}` | 改/删 |
| GET | `/users/{id}/traffic/summary` | 当日（UTC）用量，按节点拆分（admin 或本人） |
| GET | `/users/{id}/traffic/series` | 趋势 `?granularity=hourly\|daily&from=&to=&node=`（admin 或本人；`from`/`to`/`bucket` 均为 **UTC**） |
| GET | `/users/{id}/live` | 实时诊断（admin；在线设备、活跃流、域名榜、设备维度） |
| GET | `/settings` | 读取注册/邀请开关 |
| PATCH | `/settings` | 改开关（`require_invite_for_open=true` 需 `invitations_enabled=true`，否则 400） |
| GET | `/invitations` | 邀请码列表（含有效性、`link`） |
| POST | `/invitations` | 新建邀请码（可选 `email`/`max_uses`/`expires_in_hours`/`note`/`send_email`；`invitations_enabled=false` 时 400） |
| DELETE | `/invitations/{id}` | 删除邀请码 |

所有返回 node 的接口都已剥除 `api_secret`。

**时间**：数据库存储与 API 中的 datetime 一律 **UTC**（流量按 UTC 小时/日分桶）。前端自行换算为本地时区展示；查询 `traffic/series` 时 `from`/`to` 也传 UTC。

## 公共接口（无需登录）

### 自助注册 `POST /api/panel/register`

请求体 `{ email, password, code? }`。访问与「是否需邀请码」由 `app_settings` 实时决定（见 `registrationDecision`）：

- 关闭（`open_registration` 与 `invitations_enabled` 均 false）→ 403。
- 仅邀请（`open_registration=false` 且 `invitations_enabled=true`）→ 必须带有效 `code`。
- 开放（`open_registration=true`）→ `require_invite_for_open` 决定是否仍需 `code`。

新用户固定 `role=user`、`status=active`、`auth_string` 由系统随机生成（客户端无法指定 role/auth_string/status）。`verified := 是否经邀请码`：

- 经邀请码 → `verified=true`，响应含 `token`+`record`（自动登录）。
- 开放且无码 → `verified=false`，发邮箱验证信（链接指向前端 `/verify?token=`，由其调 PocketBase 内置 `POST /api/collections/users/confirm-verification` 完成），响应 `{requires_verification:true}`，**不**自动登录；该路径依赖 SMTP，未配置 SMTP 时直接返回 503。

注册端点限流（按 IP），不进 `openapi.json` 的 secured 范围（标记为无需鉴权）。

### 找回密码（PocketBase 内置端点，无自定义后端代码）

前端 `/forgot-password` 直接调 PocketBase 内置 `POST /api/collections/users/request-password-reset`（请求体 `{email}`），无论邮箱是否存在都返回 `204`（内置反枚举 + 单账号 2 分钟重发节流）。重置确认页 `/reset-password?token=` 调内置 `POST /api/collections/users/confirm-password-reset`（请求体 `{token,password,passwordConfirm}`）；成功后若 token 内邮箱与记录一致还会顺带置 `verified=true`。这两个内置端点不触发 `OnRecordAuthRequest`，故不受 `status`/`verified` 门禁阻挡（`disabled` 用户可重置但仍无法登录）。

**部署需做一次配置**：默认重置邮件链接指向 PocketBase 后台（`{APP_URL}/_/#/auth/confirm-password-reset/{TOKEN}`）。要让链接落到前端重置页，请在 `/_/` 后台把 **Settings → Application URL** 设为前端域名，并把 users collection 的 **Reset password** 邮件模板链接改成 `{APP_URL}/reset-password?token={TOKEN}`。同样依赖 SMTP；未配置 SMTP 则找回密码不可用。

### Hysteria 回调 `POST /api/hysteria/auth`（供 Hysteria 节点调用）

给 Hysteria 2 节点的 `auth.type: http` 用，每次客户端连接时由节点回调。请求体形如 `{"addr":"1.2.3.4:5678","auth":"<client-auth-key>","tx":1000000}`；后端按 `auth` 在 `users.auth_string` 里查匹配，命中且 `status=active` 且 `verified=true` 时返回 `200 {"ok":true,"id":"<auth_string>"}`，其它返回非 200。返回的 `id` 故意回填为 `auth_string`，让节点后续 `/traffic` 上报的 key 与采集器查询用的字段一致。

| 状态码 | 含义 |
|---|---|
| 200 | 验证通过，body 含 `ok` 与 `id` |
| 400 | 请求体缺失 `auth` 或不是合法 JSON |
| 401 | `auth` 在 users 中查无此人 |
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

> 邮件（邀请信、邮箱验证信）走 PocketBase 内置 SMTP（在 `/_/` 后台 Settings → Mail 配置），无新增环境变量。未配置 SMTP 时邀请接口仍返回 `link` 供手动分享，但开放无码注册因依赖验证邮件而不可用。找回密码邮件同样走内置 SMTP，但用的是 PocketBase 自带的 **Reset password** 模板（需按上文把链接改指向前端 `/reset-password`），不经 `mailer.go`。

## 结构

```
main.go                     启动：migration + 采集器 + 路由
migrations/                 collection 定义（users 扩展 / nodes / traffic_* / passkeys / invitations / app_settings）
internal/config/            环境变量（caarlos0/env）
internal/cryptobox/         AES-GCM 加解密（主密钥来自 config）
internal/token/             URL-safe 随机 token（邀请码 / auth_string）
internal/hysteria/          Traffic Stats API 客户端
internal/collector/         counter-to-delta 采集核心
internal/api/               /api/panel 路由：nodes / users / traffic / live / settings / invitations / register
```
