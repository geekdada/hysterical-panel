# Hysterical Panel (backend)

轻量级 Hysteria 2 管理面板后端，基于 PocketBase（作为 Go 框架二次开发）。
只负责节点接口信息保存、自主轮询采集流量、用户管理与实时诊断。不部署节点、不做订阅计费。

## 模型

两层：
- **users** — 既是登录面板的人（admin），也是 Hysteria 认证的账号（`auth_string` 字段，与登录 email 独立）。
- **nodes** — 一个 Hysteria 实例的接口信息（`api_url` + 加密的 `api_secret`）。

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
| `PB_DATA_DIR` | 否 | PocketBase 数据目录，默认 `./pb_data` |
| `PB_ENCRYPTION_KEY` | 否 | PocketBase 设置库加密（见官方文档） |

配置在 `internal/config` 中通过 [caarlos0/env](https://github.com/caarlos0/env) 解析；未设置必填项时进程会拒绝启动。

首次启动按提示创建 superuser（PocketBase 后台 `/_/`）。迁移在 `./migrations` 中以代码形式提交，启动时自动应用。

## 采集器

后台 goroutine 每 5s 调度，按各节点 `poll_interval` 轮询 `GET /traffic`：
- counter-to-delta：处理 Hysteria 重启导致的计数器归零
- 累加到 `users.used_tx/rx` 与 `traffic_hourly` / `traffic_daily`
- 失败写 `node.last_error` 且不更新 cursor（漏采一轮不丢量）

`/online` 与 `/dump/streams` 不进采集循环，由 live 接口实时穿透拉取。

## 接口（前缀 `/api/panel/`，需登录 + admin）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/nodes` | 列表（含 health，不含 secret） |
| POST | `/nodes` | 新建（secret 加密存储） |
| PATCH | `/nodes/{id}` | 修改（secret 缺省=不变，传空=报错） |
| DELETE | `/nodes/{id}` | 删除 |
| POST | `/nodes/{id}/test` | 立即验证连通性 |
| GET | `/users` | 列表 |
| POST | `/users` | 新建（email+password+auth_string） |
| GET/PATCH/DELETE | `/users/{id}` | 详情/改/删 |
| GET | `/users/{id}/traffic/summary` | 累计用量，按节点拆分 |
| GET | `/users/{id}/traffic/series` | 趋势 `?granularity=hourly\|daily&from=&to=&node=`（`from`/`to`/`bucket` 均为 **UTC**） |
| GET | `/users/{id}/live` | 实时诊断（在线设备、活跃流、域名榜、设备维度） |

所有返回 node 的接口都已剥除 `api_secret`。

**时间**：数据库存储与 API 中的 datetime 一律 **UTC**（流量按 UTC 小时/日分桶）。前端自行换算为本地时区展示；查询 `traffic/series` 时 `from`/`to` 也传 UTC。

## 结构

```
main.go                     启动：migration + 采集器 + 路由
migrations/                 6 个 collection 定义
internal/config/            环境变量（caarlos0/env）
internal/cryptobox/         AES-GCM 加解密（主密钥来自 config）
internal/hysteria/          Traffic Stats API 客户端
internal/collector/         counter-to-delta 采集核心
internal/api/               /api/panel 路由：nodes / users / traffic / live
```
