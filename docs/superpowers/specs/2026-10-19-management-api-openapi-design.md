# Management API OpenAPI 设计

## 为什么做

Management API 的文档现在有三份，都靠手写：`backend/README.md` 的接口表、`frontend/src/routes/settings/management-api.tsx` 里写死的 curl 和 JSON 示例，以及 `api.go` / `management_api*.go` 里真正的路由和 handler。三份之间没有任何检查，改了接口忘了改文档也不会有提示。

主要调用方是 admin 本人：在本地开发集成功能时，需要一份和正在运行的后端一致的接口说明。最好的拿法是用手上的 mgmt token 直接请求一个地址。

## 目标

- 用 mgmt token 就能拿到当前后端的 Management API OpenAPI spec。
- 前端 `/settings/management-api` 页面根据同一份 spec 渲染，不再写死接口内容。
- spec 只在 Go 里维护一份。schema 复用 `dto.go` 和现有的生成逻辑，接口规则写进 operation 的 description。
- 新增或修改 mgmt 路由却没更新 spec 时，`make test` 失败。

## 不做

- 不把 `/api/mgmt/*` 放进公开的 `/api/openapi.json`。决策 #8 不变，前端 `schema.d.ts` 里不出现 mgmt 类型。
- 接口说明不做多语言。summary 和 description 只有英文。
- 不嵌入 Scalar、Redoc、Swagger UI 这类第三方文档 UI。
- 页面不提供在线调用（try it out）。

## 失败的样子

- 页面显示的接口和后端实际行为对不上。这正是现在的问题，换个形式再出现就等于没做。
- mgmt 功能关闭时，`/api/mgmt/openapi.json` 返回 404 以外的结果，等于泄露了 Management API 的存在。
- spec 或页面里出现真实的 token、`api_secret` 或 auth string。

## 后端

### Spec 构建

- 新增 `BuildManagementOpenAPISpec()`，返回一份独立的 OpenAPI 3.1 文档。
  - `info.title` 为 `Hysterical Panel Management API`，`info.version` 用 `version.Version`。
  - 安全方案是 `bearerAuth`（HTTP bearer），全局生效。
  - `components.schemas` 只放 mgmt 用到的 schema，例如 `Node`、`PanelUser`、`ErrorResponse`、创建用户的请求和响应、节点创建/更新请求、测试结果、删除结果。生成方式和 `BuildOpenAPISpec()` 一样（每个 type 一个独立 generator，enum 照现有模式补）。
  - 覆盖 `api.go` 里 `/api/mgmt` group 下的 8 条路由，外加新的 `GET /api/mgmt/openapi.json`。
- 现在写在 README 和前端页面里的规则全部挪进对应 operation 的 `description`，至少包括：
  - `GET /users`：`id`、`email`、`auth_string` 三选一，同时给多个返回 400；只匹配 Current Auth String；Connected App 的 `sub` 就是 User ID。
  - `POST /users`：自动生成密码和 auth string，不返回；状态码 201；用户需要走找回密码拿到登录凭证。
  - `GET /nodes`：未删除节点，按创建时间倒序；不含 `api_secret`。
  - `POST /nodes`：`name` / `api_url` / `api_secret` 必填，`poll_interval` 默认 30，`enabled` 默认 true，状态码 201。
  - `PATCH /nodes/{id}`：开启和关闭就是改 `enabled`，副作用和面板一致；`api_url` 与存储值不同时必须同时传 `api_secret`，否则 400（ADR-0011）；`api_secret` 传空字符串也是 400。
  - `DELETE /nodes/{id}`：软删除，已删除再删返回 404，不提供恢复。
  - `POST /nodes/{id}/test`：使用已存储的地址和 secret，已关闭的节点也能测；恒为 200，结果是 `{ok:true, latency_ms}` 或 `{ok:false, error}`。
  - 文档级 description 写清鉴权方式：带 `Authorization: Bearer <token>`，功能未启用时一律 404，token 缺失或错误时 401。
- description 写成纯文本，不用 Markdown：短段落，规则多的时候每行以 `- ` 开头，前端保留换行原样显示。（我的判断。前端没有 Markdown 库，为这几段文字引入一个不值得。）
- 每个 operation 的 `responses` 里列出它会返回的错误状态码（400 / 401 / 404 等），带一句说明，引用 `ErrorResponse`。前端靠这些显示各接口的错误情况。
- 每个 operation 的请求体和成功响应都要带 `example`，取代前端页面里写死的 JSON 示例。示例值一律是假数据（`rec_...`、`user@example.com`、`tokyo-1.example.com` 这类）。
- `servers` 在运行时填 `PANEL_BACKEND_URL_BASE`；没有配置时省略这个字段。

### 接口

| 路由 | 守卫 | 用途 |
|---|---|---|
| `GET /api/mgmt/openapi.json` | `requireMgmtToken`（未启用 404，token 错 401） | 外部调用方用 token 拿 spec |
| `GET /api/panel/management-api/openapi.json` | `requireAdmin` | 前端文档页使用。mgmt 功能关闭时同样可以访问，admin 可以先读文档再决定是否开启 |

两个接口返回同一个 `BuildManagementOpenAPISpec()` 的结果。panel 接口要进公开 spec（`openapi.go`），响应 schema 写成通用 object 就行，前端不依赖它的具体类型。

### 防漂移

- `make test` 必须能发现「注册了 mgmt 路由但 spec 里没有」和「spec 里有但没注册」两种情况，method 和 path 都要对上。
- 测试还要用 kin-openapi 校验 spec 里每个 `example` 是否符合它引用的 schema。DTO 加了必填字段或改了类型而示例没跟上时，测试失败。
- 不在 spec 测试里把示例请求真的发给 handler 跑一遍。状态码由 `management_api_nodes_test.go` 这类现有 handler 测试负责。
- 路由比对的实现方式交给 builder。一种做法是用一张路由表同时驱动注册和 spec，从结构上避免漂移；另一种是测试里把实际注册的 mgmt 路由和 spec 的 paths 逐条比对。

## 前端

- `/settings/management-api` 保留现在的位置、`requireAdmin` 守卫和页面外框（标题、简介、鉴权说明、错误格式说明）。外框文案继续走 Paraglide，保持双语。
- 接口部分改成通过 react-query 请求 `GET /api/panel/management-api/openapi.json`，再渲染 spec：
  - 按 tag 分组（Users、Nodes）。分组标题用 spec 里的 tag 名，只有英文。
  - 每个 operation 显示 method、path、summary、description（保留换行）、参数表（名字、位置、是否必填、类型、说明）、请求体字段表、请求和成功响应的示例代码，以及错误状态码列表（状态码加说明）。
  - 响应只展示示例，不画字段表。`PanelUser` 这类大 schema 有嵌套数组和对象，示例 JSON 比递归展开的表更好读。（我的判断。）请求体是平铺的，字段表只解析一层 `$ref`。
  - curl 示例由 method、path、示例请求体和 `usePanelApiOrigin()` 拼出来，token 位置写 `YOUR_TOKEN`，样式沿用现有的 `CopyableCode`。
  - 视觉沿用当前页面的 `Endpoint` / `CodeBlock` 样式和 `globals.css` 的 token。
- 页面顶部加一段说明并给出可复制的 curl：用 token 请求 `/api/mgmt/openapi.json` 就能拿到这份 spec。文案走 Paraglide。
- 加载中、请求失败的状态按面板其他页面的惯例处理。
- 删除写死的 JSON 常量和各接口专用的 Paraglide 键（`mgmt_api_get_summary`、`mgmt_api_nodes_*` 等），两个 locale 同步删，`pnpm i18n:check` 通过。
- spec 用 `$ref` 引用 components。渲染时怎么解析 `$ref`、OpenAPI 类型从哪里来（手写最小类型或引入 `openapi-types` 一类的纯类型包），交给 builder。

## 文档更新

- `backend/README.md`：删掉 Management API 的接口表和逐条说明。保留启用、轮换 token 的说明，再加一句「接口 spec 用 token 请求 `GET /api/mgmt/openapi.json`」。（保留启用说明是我的判断，这部分讲的是 panel 侧怎么开启，spec 里不会有。）
- `AGENTS.md`：
  - 决策 #8：说明 mgmt 有自己独立的 spec，由 `BuildManagementOpenAPISpec()` 生成，不进 `/api/openapi.json`。
  - 决策 #10：去掉「前端文档页与 backend/README.md 需同步维护」，改成「改 mgmt 接口时同步更新 mgmt spec，测试会检查路由与 spec 一致」。
  - 公开接口一节中「契约见 backend/README.md」改成指向 `/api/mgmt/openapi.json`。
  - 目录结构里补上新增的文件。
- 改完后端契约后跑 `make openapi` 和 `pnpm api:sync`，因为新增的 panel 接口要进公开 spec。

## 交给 builder 的部分

- mgmt spec 的代码放在新文件里还是 `openapi.go` 里，以及和 `BuildOpenAPISpec()` 共享 schema 生成辅助函数的方式。
- 防漂移用哪种实现。
- `make openapi` 要不要顺带把 mgmt spec 也落成文件。没有消费方的话可以不做。
- 前端渲染组件的拆分和 `$ref` 解析方式。
