# Email Sendout 设计

名词见 `CONTEXT.md` 的 **Email Sendout** / **Sendout Recipient**。取舍记录在 `docs/adr/0008-browser-composed-email-sendouts.md`。

## 范围

- admin 发起的事务邮件，不能退订，只有一个固定模板。
- 收件人是一个可用 User，或者全部可用 User。可用 = `status=active && verified=true`，包括 admin。
- 发信通道是 PocketBase 配置的 SMTP 和发件人（`meta.senderName` / `meta.senderAddress`），不加 Reply-To。
- 不做：收件人个性化、订阅/退订、定时发送、自动重试、User 自己查看收件记录、图片。

## 写信

- 在 `@react-email/editor` 里写，用它自带的 bubble menu 和 slash commands，不挂 Inspector 侧栏。
- 可用格式：段落、H1–H3、粗体/斜体/下划线/删除线、链接、有序/无序列表、分隔线、按钮。slash command 的标题走 Paraglide。
- 编辑画布始终按邮件外观显示（浅色）；浮层菜单跟随面板主题。
- 固定模板通过自定义 `BaseTemplate` 包裹正文。每个 Sendout 选语言（`en` / `zh-cn`），外框文案随语言切换。页眉放 PocketBase `meta.appName`；页脚写明这是和账号相关的服务通知、不能退订，并附面板链接（`frontend_url`）。
- 没有收件箱预览文字。
- 浏览器用 `composeReactEmail` 生成 `unformattedHtml` 和 `text`，连同 subject、语言、收件范围、editor JSON 一起提交。后端只校验和存储，发送时原样使用。

## 后端数据

- `email_sendouts`：subject、language、audience（`single`/`all`）、html、text、content（editor JSON）、created_by + created_by_email 快照、cancelled_at、created。状态不落库，从 Recipient 计数推导：有 cancelled_at 就是 `cancelled`；还有 pending/sending 行就是 `sending`；否则 `completed`。
- `email_sendout_recipients`：sendout（级联删除）、user（User 删除后置空）、email、status、reason、attempts、queued_at、last_attempt_at、sent_at。
  - status：`pending` / `sending` / `sent` / `failed` / `skipped` / `cancelled`
  - reason：`delivery_failed` / `smtp_disabled` / `interrupted` / `user_ineligible` / `user_deleted`
  - `email` 在创建时写入，每次发送前改成 User 当前的邮箱，所以它记录的是这一行最后一次投递的地址。
- `app_settings.email_sendout_rate_per_minute`：取值 1–600，默认 30。
- 历史永久保留。

## 队列语义

- 创建时在同一个事务里写 Sendout 和全部 Recipient 行（`pending`，queued_at = 创建时间）。「全部」按创建那一刻展开，之后新增的 User 不补发。
- 单个进程内 worker，按 `queued_at, id` 的 FIFO 顺序逐封发送；两封之间间隔 `60s / rate`；队列空时每 5 秒轮询一次，创建或重发时立即唤醒。
- 发一封的步骤：
  1. 用带条件的 UPDATE 把 `pending` 抢成 `sending`，attempts +1；
  2. User 已删除 → `skipped` / `user_deleted`；
  3. User 不可用 → `skipped` / `user_ineligible`；
  4. SMTP 未启用 → `failed` / `smtp_disabled`；
  5. 发送失败 → `failed` / `delivery_failed`；
  6. 成功 → `sent` 并记 sent_at。
- 只发一次，不自动重试。进程启动时，把遗留的 `sending` 行改成 `failed` / `interrupted`。
- 取消：只把 `pending` 行改成 `cancelled`，已在发送中的那一封照常完成。没有 pending/sending 行时拒绝取消。已取消的 Sendout 不能再取消。
- 重发：只把 `failed` 行改回 `pending`，清空 reason，queued_at 设为当前时间（排到队尾）。可以指定行，不指定就是这个 Sendout 下所有失败的行。已取消的 Sendout 不能重发。
- SMTP 未启用时，创建和重发都返回 503。

## 接口（admin，进 OpenAPI）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/email-sendouts/context` | app_name、frontend_url、smtp_enabled、eligible_recipient_count、rate_per_minute |
| GET | `/email-sendouts/eligible-recipients?search=` | 最多 20 个可用 User（id、email），按邮箱子串匹配 |
| GET | `/email-sendouts` | 全部 Sendout，按创建时间倒序，带各状态计数 |
| POST | `/email-sendouts` | 创建；校验失败 400，SMTP 未启用 503 |
| GET | `/email-sendouts/{id}` | Sendout 加 html、text |
| GET | `/email-sendouts/{id}/recipients?status=` | Recipient 行，按 email 排序 |
| POST | `/email-sendouts/{id}/cancel` | 取消剩余的 pending 行 |
| POST | `/email-sendouts/{id}/resend` | body `{recipient_ids?}`，返回 `{requeued}` |
| PATCH | `/settings` | 新增 `email_sendout_rate_per_minute`（1–600） |

校验规则：
- subject：trim 后 1–200 个字符，不能含控制字符；
- language 只能是 `en` / `zh-cn`；
- audience 为 `single` 时必须带 `user_id`，为 `all` 时不能带；
- html、text 都不能为空，上限分别是 1,000,000 和 256,000 字节；
- content 必填，序列化后不超过 1 MiB。

## 前端页面（Settings 下）

- `/settings/emails`：Sendout 列表（主题、语言、收件范围、已发/失败/共计、状态、发起人、时间）和发送速率设置。SMTP 未启用时整页提示，并禁用新建。
- `/settings/emails/new`：editor、主题、语言、收件人（单个 User 选择器或「全部可用 User」）。「检查并发送」打开确认框，里面是 sandbox iframe 预览、收件范围和人数。
- `/settings/emails/$id`：sandbox iframe 只读预览，加 Recipient 状态表（按状态筛选、单行/批量重发、取消 Sendout）。
