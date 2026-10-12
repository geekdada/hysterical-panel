# 调研：为 hysterical-panel 增加 OAuth 2.0 / OIDC Provider

日期：2026-10-12。面板代码未做任何改动。第 7.2 节记录了需求方的决策，对应 `docs/adr/0010-oidc-provider.md`。

## 1. 结论摘要

推荐做法：在后端进程内自行实现一个窄 profile 的 OIDC Provider，用 PocketBase collection 存状态，用 `go-jose/v4` 签 ID Token。上线前跑一遍 OpenID Foundation conformance suite 的 Basic OP 测试计划。

窄 profile 只包含以下内容：

- 只有 Authorization Code + PKCE，且只接受 `S256`。
- 第一版只支持 confidential client，认证方式为 `client_secret_basic` 与 `client_secret_post`。
- `redirect_uri` 逐字符精确匹配。
- 授权码一次性，60 秒过期。重放时吊销该授权码派生的全部 token。
- Access token 与 refresh token 都是不透明随机串，库里只存 SHA-256。refresh token 每次使用都轮换，旧值重放则吊销整个授权。
- ID Token 用 RS256 签名。提供 discovery、JWKS、userinfo、RFC 7009 revocation，授权响应带 RFC 9207 的 `iss` 参数。
- 功能开关放在 `app_settings`，默认关闭。关闭时所有协议端点返回 404，与 Management API 的处理一致。

不选现成库的理由如下：

- 没有一个 Go 库同时满足「维护活跃、能只开 S256、支持 RFC 9207、客户端存数据库、能挂进 PocketBase 路由」。fosite 独立仓库已停止开发，代码已并入 ory/hydra。zitadel/oidc v3 维护活跃，但不能关闭 `plain` PKCE，也没有 RFC 9207，还自带 chi 路由与 CORS。luikyv/go-oidc 已通过认证，但仍是 v0.x，客户端只能静态配置或走动态注册。细节见第 4 节。
- 窄 profile 的协议逻辑以状态检查为主，签名交给 go-jose。Gitea 走的也是这条路：其 OAuth2 provider 自己实现协议逻辑，签名用 `golang-jwt/jwt/v5`，见 `services/oauth2_provider/jwtsigningkey.go` 的 import 与 [go-gitea/gitea services/oauth2_provider](https://github.com/go-gitea/gitea/tree/main/services/oauth2_provider)。
- 如果将来要 PAR、DPoP、动态注册或设备流，再整体换到 zitadel/oidc v3 或 luikyv/go-oidc。

PocketBase v0.39 只是 OAuth2 client，自身不提供任何 provider 能力。它的会话 token 是 HS256、无 scope、无 audience，**不能**当作 OAuth access token 发给第三方。详见第 3 节。

预估工作量约 15 到 18 人日。这项功能超出了 AGENTS.md 写明的后端职责边界，需求方已同意并写入 ADR-0010。各项决策见第 7.2 节。

## 2. 规范要求

### 2.1 2026 年「基础但安全」的最低要求

规范基线是 [RFC 9700 OAuth 2.0 Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html) 与 OAuth 2.1 草案。OAuth 2.1 最新版是 [draft-ietf-oauth-v2-1-16](https://datatracker.ietf.org/doc/draft-ietf-oauth-v2-1/)，2026-09-03 发布，状态仍是 I-D，未进入 IESG 审议。

| 要求 | 规范依据 | 本项目做法 |
|---|---|---|
| 授权服务器必须支持 PKCE，公共客户端必须用 PKCE，confidential client 推荐用 | [RFC 9700 §2.1.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1.1) | 所有客户端强制 PKCE |
| `S256` 是唯一不在请求中暴露 verifier 的方法；能用 S256 的客户端必须用 | [RFC 9700 §2.1.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1.1)，[RFC 7636 §4.2](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.2)，[§7.2](https://www.rfc-editor.org/rfc/rfc7636.html#section-7.2) | 拒绝 `plain` |
| 授权服务器必须把 `code_challenge` 与授权码绑定，校验失败返回 `invalid_grant` | [RFC 7636 §4.4](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.4)，[§4.6](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.6) | 授权码记录里存 challenge |
| verifier 长度 43 到 128 字符 | [RFC 7636 §4.1](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.1) | 校验长度与字符集 |
| `redirect_uri` 必须与注册值完全相等，仅原生应用的 loopback 地址允许端口可变 | [RFC 9700 §2.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)，[§4.1.3](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.1.3)；OAuth 2.1 §2.3.1 | 逐字符比较，第一版不做 loopback 例外 |
| `redirect_uri` 无效时不得重定向，要直接告知用户 | [RFC 6749 §4.1.2.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.2.1) | 前端显示错误页 |
| 不应使用 implicit grant；OAuth 2.1 已删除它 | [RFC 9700 §2.1.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1.2)，OAuth 2.1 §10.1 | 只接受 `response_type=code` |
| 不得使用 ROPC（密码模式） | [RFC 9700 §2.4](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.4) | 不实现 |
| 授权码必须短期有效，建议不超过 10 分钟；重复使用必须拒绝，且应吊销据此签发的 token | [RFC 6749 §4.1.2](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.2) | 60 秒，重放即吊销 grant |
| 授权码在 token 端点首次使用后必须失效 | [RFC 9700 §4.2.4](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.2.4) | `used_at` 原子写入 |
| 公共客户端的 refresh token 必须 sender-constrained 或轮换；必须检测重放；必须绑定所授权的 scope | [RFC 9700 §2.2.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.2.2)，[§4.14.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14.2) | 对所有客户端都轮换 |
| 客户端与多个授权服务器交互时必须有 mix-up 防御，推荐 RFC 9207 的 `iss` | [RFC 9700 §2.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)，[§4.4.2.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.4.2.1) | 授权响应带 `iss` |
| 用 `state` 防 CSRF | [RFC 6749 §4.1.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.1) 指向 §10.12；[RFC 9700 §4.7](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.7) | 原样回传 `state`；客户端用 PKCE 后也可依赖 PKCE |
| 授权服务器必须防点击劫持，用 `X-Frame-Options` 与 CSP `frame-ancestors` | [RFC 9700 §4.16](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.16) | 同意页加这两个头 |
| 不得做开放重定向；只对可信的 redirect URI 自动跳转 | [RFC 9700 §4.11.2](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.11.2) | 只跳已注册 URI |
| 可能带凭据的重定向不得用 307，应用 303 | [RFC 9700 §4.12](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.12) | 一律 303 |
| 授权完成后的页面不应包含第三方资源或外链 | [RFC 9700 §4.2.4](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.2.4) | 同意页 `Referrer-Policy: no-referrer` |
| 客户端认证推荐非对称方式 | [RFC 9700 §2.5](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.5) | 第一版用高熵 secret，`private_key_jwt` 留作以后 |
| access token 应限定 audience | [RFC 9700 §2.3](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.3) | 唯一的资源服务器是本面板的 userinfo，token 记录绑定 client |

client secret 的熵要求：用于 HMAC 时 `client_secret` 必须有足够熵，见 [OIDC Core §16.19](https://openid.net/specs/openid-connect-core-1_0.html#SymmetricKeyEntropy)。本项目用 32 字节随机数，与 Management API token 相同，见 `backend/internal/api/settings.go:166-168`。高熵随机 secret 用单次 SHA-256 存储即可，不需要 bcrypt 一类慢哈希。这一点是工程判断，没有规范原文。

token 寿命：OIDC Core 建议 access token 很短或一次性，长期访问靠 refresh token，见 [OIDC Core §16.18](https://openid.net/specs/openid-connect-core-1_0.html#TokenLifetime)。建议值：授权请求 10 分钟，授权码 60 秒，access token 10 分钟，ID token 10 分钟，refresh token 滑动 30 天、绝对上限 90 天。这些数字是建议，规范只给上限或方向。

token 端点响应要带 `Cache-Control: no-store`，见 [RFC 6749 §5.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.1)。access token 不接受 URI query 传递，OAuth 2.1 已删除这种方式，原定义见 [RFC 6750 §2.3](https://www.rfc-editor.org/rfc/rfc6750.html#section-2.3)。

### 2.2 「用面板账号登录」需要的 OIDC 部分

**ID Token。** 必填 claim 为 `iss`、`sub`、`aud`、`exp`、`iat`。`sub` 在 issuer 内唯一且永不重新分配，不超过 255 个 ASCII 字符。`aud` 必须包含 client_id。请求带 `nonce` 时 ID Token 必须原样带回。请求带 `max_age` 时必须返回 `auth_time`。以上见 [OIDC Core §2](https://openid.net/specs/openid-connect-core-1_0.html#IDToken)。Code flow 下 `nonce` 与 `at_hash` 都是 OPTIONAL，见 [§3.1.2.1](https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest) 与 [§3.1.3.6](https://openid.net/specs/openid-connect-core-1_0.html#CodeIDToken)。建议仍然返回 `at_hash`，成本很低。

**Scope。** `openid` 必需。`email` scope 对应 `email` 与 `email_verified`。`profile` 对应 `name`、`picture` 等资料字段。见 [OIDC Core §5.4](https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims)。只有 `iss` + `sub` 组合是稳定标识，email 不得用作唯一标识，见 [§5.7](https://openid.net/specs/openid-connect-core-1_0.html#ClaimStability)。

**UserInfo。** 必须接受 Bearer access token，必须走 TLS，响应必须含 `sub`。RP 必须核对它与 ID Token 的 `sub` 一致。见 [OIDC Core §5.3](https://openid.net/specs/openid-connect-core-1_0.html#UserInfo)。

**所有 OP 必须实现的特性。** 见 [OIDC Core §15.1](https://openid.net/specs/openid-connect-core-1_0.html#ServerMTI)：

- 支持 RS256 签名 ID Token。
- 支持 `prompt` 参数，包括 `none` 与 `login`。
- `display`、`ui_locales`、`claims_locales`、`acr_values` 至少不能报错。
- 被请求时返回 `auth_time`，并执行 `max_age`。

`max_age` 与 `auth_time` 在本项目里有实际困难，见 3.3 节与 5.12 节。

**Discovery。** 必填元数据为 `issuer`、`authorization_endpoint`、`token_endpoint`、`jwks_uri`、`response_types_supported`、`subject_types_supported`、`id_token_signing_alg_values_supported`。`userinfo_endpoint`、`scopes_supported`、`claims_supported` 为推荐。见 [OIDC Discovery §3](https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata)。issuer 带路径时，配置地址是在路径后面追加 `/.well-known/openid-configuration`，见 [§4](https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfig)。返回的 `issuer` 必须与请求所用 issuer 完全相同，也必须等于 ID Token 的 `iss`，见 [§4.3](https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfigurationValidation)。`code_challenge_methods_supported` 不在 OIDC Discovery 里，出自 [RFC 8414 §2](https://www.rfc-editor.org/rfc/rfc8414.html#section-2)。可以同时声明，值只写 `["S256"]`。

**RFC 9207。** 支持该规范的服务器必须在授权响应中带 `iss`，错误响应也要带。元数据用 `authorization_response_iss_parameter_supported: true` 声明，省略时默认 false。见 [RFC 9207 §2](https://www.rfc-editor.org/rfc/rfc9207.html#section-2) 与 [§3](https://www.rfc-editor.org/rfc/rfc9207.html#section-3)。

**签名密钥。**

- RS256 是 §15.1 强制项，所以第一版只发 RS256。
- RSA 密钥至少 2048 位，见 [RFC 7518 §3.3](https://www.rfc-editor.org/rfc/rfc7518.html#section-3.3)。
- ES256 体积更小，可作为第二个算法后加。
- EdDSA 这个多态标识已被 [RFC 9864](https://datatracker.ietf.org/doc/html/rfc9864) 标为 Deprecated，替代标识是 `Ed25519`。RP 端支持面也窄，不建议采用。
- 轮换：每个 JWS 头带 `kid`。新密钥先加入 JWKS 再启用，刚退役的密钥在 JWKS 里保留一段时间。RP 遇到陌生 `kid` 会重新拉取 JWKS。见 [OIDC Core §10.1.1](https://openid.net/specs/openid-connect-core-1_0.html#RotateSigKeys)。
- 验证端要固定算法，防 alg confusion，见 [RFC 8725 §3.1](https://www.rfc-editor.org/rfc/rfc8725.html#section-3.1)。这条主要约束 RP。我们自己解析 token 的地方只有 revocation，而那里用的是不透明 token，不涉及 JWT 解析。

**Revocation 与 introspection。**

- RFC 7009：必须支持吊销 refresh token，应支持吊销 access token。吊销 refresh token 时，应同时作废同一授权下的 access token。对无效 token 也返回 200。见 [RFC 7009 §2](https://www.rfc-editor.org/rfc/rfc7009.html#section-2)、[§2.1](https://www.rfc-editor.org/rfc/rfc7009.html#section-2.1)、[§2.2](https://www.rfc-editor.org/rfc/rfc7009.html#section-2.2)。第一版实现它。
- Introspection（[RFC 7662](https://www.rfc-editor.org/rfc/rfc7662.html)）用于让外部资源服务器校验不透明 token。本方案里唯一接受 access token 的资源服务器是面板自己的 userinfo，所以第一版不需要。

### 2.3 有意不做的部分

| 特性 | 不做的理由 |
|---|---|
| 动态客户端注册 [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591.html) | 需求写明只有 admin 能登记客户端。不开放注册，也就不存在 `jwks_uri`、`sector_identifier_uri` 带来的 SSRF |
| 设备授权 [RFC 8628](https://www.rfc-editor.org/rfc/rfc8628.html) | 「用面板账号登录」用不到 |
| `client_credentials` | 没有用户参与，与「登录」无关。Management API 已经覆盖机器对机器的场景 |
| PAR [RFC 9126](https://www.rfc-editor.org/rfc/rfc9126.html)、JAR [RFC 9101](https://www.rfc-editor.org/rfc/rfc9101.html) | 保护授权请求的完整性与机密性，是 FAPI 级别的要求。基础 profile 下 PKCE + 精确 redirect 已足够。以后可加 |
| DPoP [RFC 9449](https://www.rfc-editor.org/rfc/rfc9449.html) | RFC 9700 对 confidential client 不要求 sender-constraining。第一版又不开放公共客户端，refresh 轮换已满足 §2.2.2 |
| JWT access token [RFC 9068](https://www.rfc-editor.org/rfc/rfc9068.html) | 没有外部资源服务器需要离线验签。不透明 token 可以随时在库里吊销，用户停用后马上失效 |
| implicit、hybrid、ROPC | RFC 9700 不推荐或禁止 |
| `request_uri`、`claims` 请求参数 | 不做，避免 SSRF 与额外解析面 |

## 3. PocketBase 现状

以下内容来自本地模块缓存里的 `pocketbase@v0.39.0` 源码，行号也对应该版本。

### 3.1 PocketBase 只是 OAuth2 client

- 路由只挂了 settings、collection、record CRUD、record auth、logs、backup、cron、file、batch、realtime、health、SQL 这些组，没有 authorize、token、userinfo、JWKS 或 `.well-known`。见 [apis/base.go#L39-L51](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/base.go#L39)。
- auth 路由只有 `auth-with-password`、`auth-with-oauth2`、`auth-refresh`、OTP、密码重置、验证、改邮箱和 superuser 的 `impersonate`，见 [apis/record_auth.go#L12-L68](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/record_auth.go#L12)。
- `tools/auth` 的 `Provider` 接口只有 `BuildAuthURL`、`FetchToken` 等客户端操作，见 [tools/auth/auth.go#L34](https://github.com/pocketbase/pocketbase/blob/v0.39.0/tools/auth/auth.go#L34)。其中唯一的 JWKS 代码用于校验外部 IdP 发来的 id_token，见 [tools/auth/oidc.go#L179](https://github.com/pocketbase/pocketbase/blob/v0.39.0/tools/auth/oidc.go#L179)。
- 维护者在 2022 年写过 v1 之前不打算让 PocketBase 当 OAuth2 provider，见 [discussion #1144](https://github.com/pocketbase/pocketbase/discussions/1144#discussioncomment-4271922)。2026-10-08 他又表示 token 签名方式「for now」不会改，仍是 HMAC，见 [discussion #7857](https://github.com/pocketbase/pocketbase/discussions/7857#discussioncomment-18816388)。

### 3.2 会话 token 不能复用为 OAuth token

- 签名密钥是 `record.TokenKey() + collection.AuthToken.Secret`，算法 HS256。claim 只有 `type`、`id`、`collectionId`、`refreshable`，再加 `exp`。没有 `iss`、`aud`、`scope`、`jti`、`iat`。见 [core/record_tokens.go#L51-L73](https://github.com/pocketbase/pocketbase/blob/v0.39.0/core/record_tokens.go#L56) 与 [tools/security/jwt.go#L46-L57](https://github.com/pocketbase/pocketbase/blob/v0.39.0/tools/security/jwt.go#L57)。
- 全局中间件 `loadAuthToken` 对每个路由都接受任何 `type=auth` 的 token，`NewStaticAuthToken` 签出的也算，见 [apis/middlewares.go#L184-L209](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/middlewares.go#L184)。
- 单个 token 不能吊销。唯一手段是轮换 `tokenKey`，而这会让该用户所有会话下线，见 [core/record_model_auth.go#L46](https://github.com/pocketbase/pocketbase/blob/v0.39.0/core/record_model_auth.go#L46)。`auth-refresh` 也不作废旧 token，见 [apis/record_auth_refresh.go#L25](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/record_auth_refresh.go#L25)。

所以，把 PocketBase token 发给第三方等于交出该用户的完整面板会话：没有 scope 限制，没有 audience，也撤不回来。第三方离线也验不了 HMAC。OAuth 的 token 必须另起格式与存储，和面板会话完全分开。

反过来也要防：OAuth access token 绝不能被 `loadAuthToken` 当成面板会话。不透明随机串不是 JWT，`FindAuthRecordByToken` 解析会失败，请求会以未登录状态继续，见 [apis/middlewares.go#L184-L209](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/middlewares.go#L184)。这正是选不透明 token 的另一个理由。

### 3.3 对授权流程的影响

- `getAuthTokenFromRequest` 只读 `Authorization` 头，不读 cookie，见 [apis/middlewares.go#L211-L222](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/middlewares.go#L212)。面板前端把 token 存在前端域的非 HttpOnly cookie `hp_auth` 里，每个请求由 `client.ts` 的 middleware 塞进 `Authorization`，见 `frontend/src/api/cookie.ts:4,75-84` 与 `frontend/src/api/client.ts:66-83`。所以后端的 `/authorize` 拿不到浏览器的登录态，前后端不同源时更是如此。登录与同意必须交给前端页面，见 5.4 节。
- PocketBase token 没有 `iat`，也没有认证时间，前端无法据此判断「用户何时输入了凭据」。前端 `auth.ts` 里已有一个 `sessionStorage` 标记 `markFreshPasswordLogin` / `consumeFreshPasswordLogin`（`frontend/src/api/auth.ts:115-126`），但后端无法验证它。已决定的做法见 5.12 节。
- 关于限流：PocketBase 内置限流器默认关闭，见 [core/settings_model.go#L177-L183](https://github.com/pocketbase/pocketbase/blob/v0.39.0/core/settings_model.go#L177)。它对自定义路由只能按路径或前缀匹配，同一前缀下所有路径共用一个桶，见 [apis/middlewares_rate_limit.go#L28-L50](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/middlewares_rate_limit.go#L28)。项目已经有按 IP 计数的 `passkeyRateLimiter`（`backend/internal/api/passkeys.go:62-113`），直接复用它即可。
- 安全头：PocketBase 默认给所有响应加 `X-Frame-Options: SAMEORIGIN`，但没有 CSP，见 [apis/middlewares.go#L288-L303](https://github.com/pocketbase/pocketbase/blob/v0.39.0/apis/middlewares.go#L288)。同意页在前端 Nitro 上，不受它影响。仓库里目前没有任何地方设置 `frame-ancestors` 或 `X-Frame-Options`，`frontend/`、`deploy/nginx/default.conf` 和 `backend/internal` 都搜过。
- Hook：`OnRecordAfterUpdateSuccess("users")` 在事务提交后才触发，回滚时不触发，见 [core/app.go#L878-L884](https://github.com/pocketbase/pocketbase/blob/v0.39.0/core/app.go#L881)。它能覆盖 `/api/panel/users/{id}` 的 PATCH，也能覆盖 superuser 在 `/_/` 里直接改 `status`。现有的 kick 由 `updateUser`（`backend/internal/api/users.go:264-266`）与订阅相关代码（`subscriptions.go:126,375`，`api.go:244`）调用，在 `/_/` 里直接改 `status` 不会触发任何一处。
- 定时清理：可用 `app.Cron().MustAdd(...)`，见 [tools/cron/cron.go#L81](https://github.com/pocketbase/pocketbase/blob/v0.39.0/tools/cron/cron.go#L81)。

### 3.4 社区插件

唯一真正做 provider 的是 [benjamesfleming/pocketbase-ext-oauth2](https://github.com/benjamesfleming/pocketbase-ext-oauth2)。MIT 许可，6 star，2026-02 创建，最后提交 2026-07-13，基于 fosite v0.49.0。读 HEAD 代码发现两个问题：

- README 示例配置是 `enforce_pkce = "none"`。
- `oauth2_auth.go:61-62` 从表单参数 `pb_token` 取 PocketBase token，调用 `FindAuthRecordByToken(token)` 时不限定 token 类型，因此 file、verification、passwordReset 等类型的 token 也会被接受。

另有一个 0 star 的 fork [DragonsWho/pocketbase-ext-oauth2](https://github.com/DragonsWho/pocketbase-ext-oauth2)，加了读 `pb_auth` cookie 跳过同意的「silent SSO」。其余搜到的项目都是 OAuth client。结论：没有成熟可用的社区方案，不建议引入。

## 4. 库选型对比

数据快照日期 2026-10-12，来源为 `gh api` 与各仓库源码。

| | ory/fosite | zitadel/oidc v3 `pkg/op` | go-oauth2/oauth2 v4 | luikyv/go-oidc | 自研 + go-jose v4 |
|---|---|---|---|---|---|
| 最新版本 | v0.49.0，2024-12-12 | v3.51.14，2026-10-09 | v4.6.0，2026-09-01 | v0.25.0，2026-07-23 | v4.1.5，2026-09-03 |
| 许可 | Apache-2.0 | Apache-2.0 | MIT | MIT | Apache-2.0 |
| 维护状态 | 独立仓库最后提交 2025-07-03。2025-10-31 并入 [ory/hydra](https://github.com/ory/hydra/commit/2c3ba1311e196d226dc2904ed7507481d9479cdd) | 活跃，每周多个补丁。v4 仍是 prerelease | 活跃度一般，2023-02 到 2025-04 没有发版 | 活跃，但几乎由一人维护，API 变化快，README 与代码已对不上 | go-jose 活跃 |
| OIDC | id_token 有；userinfo、discovery、JWKS 要自己写 | 全套 | **无** | 全套 | 自己写 |
| 只允许 S256 | 可配置 | 代码不能关闭 `plain`，只对 `none` 认证的客户端强制 PKCE，见 [code_challenge.go#L25-L33](https://github.com/zitadel/oidc/blob/5ac478eb670627266989c502eb3035e4e3298a4f/pkg/oidc/code_challenge.go#L25-L33) 与 [token_code.go#L107-L111](https://github.com/zitadel/oidc/blob/5ac478eb670627266989c502eb3035e4e3298a4f/pkg/op/token_code.go#L107-L111) | 默认允许 plain | `WithPKCE(S256)` + `WithPKCERequired()` | 自己控制 |
| RFC 9207 `iss` | 无 | 无 | 无 | 有 | 一行代码 |
| Revocation / introspection | 有 / 有 | 有 / 有 | 无 / 无 | 有 / 有 | 只做 revocation |
| OpenID 认证 | 未列出 | OP **未**认证，README 说只有 RP 通过认证 | 不适用 | Basic OP，FAPI 1/2 | 需要自己跑 conformance suite |
| 存储接口负担 | 约 25 个方法，加上自写的 discovery、JWKS、userinfo | `op.Storage` 23 个方法，加上 `op.Client` 16 个、`op.AuthRequest` 15 个，见 [storage.go](https://github.com/zitadel/oidc/blob/5ac478eb670627266989c502eb3035e4e3298a4f/pkg/op/storage.go#L14) | 8 个 | 小，但客户端只能来自静态配置、Federation 或动态注册，见 [internal/client/util.go#L16-L31](https://github.com/luikyv/go-oidc/blob/6aeac93f370044ca9a59a556b9230cd10bd96868/internal/client/util.go#L16) | 直接读写 collection |
| 路由 | 只接 `*http.Request` / `http.ResponseWriter`，可放进 PocketBase handler | 自带 chi 路由与 rs/cors，要用 `apis.WrapStdHandler` 整体挂载，CORS 会与 PocketBase 的重叠 | 任意路由 | `http.Handler`，整体挂载 | 原生 PocketBase handler |
| 直接依赖数 | 25，另有 55 个间接依赖；仍依赖 go-jose v3.0.3，该版本落在 GHSA-c6gw-w398-hv78 范围内 | 17 | 7 | 3 | 0 |
| 近三年安全通告 | 独立仓库无 | [GHSA-j8gq-92xf-382c](https://github.com/zitadel/oidc/security/advisories/GHSA-j8gq-92xf-382c)，2026-08，不透明 token 可伪造 subject，v3.47.0 修复 | 未发现 | 未发现 | go-jose 有 4 条，均已修复 |

另外两个 fosite 的维护 fork：

- [authelia/oauth2-provider](https://github.com/authelia/oauth2-provider)：要求 Go 1.27，本项目是 1.26.2。README 标注不提供 UserInfo。
- [pocket-id/fosite](https://github.com/pocket-id/fosite)：README 明说 API 随 Pocket ID 的需求变化。

[openshift/osin](https://github.com/openshift/osin) 自 2022 年起没有提交，也没有 OIDC。

### 4.1 推荐与理由

**推荐自研窄 profile。** 理由如下：

1. 无论选哪个库，最大的工作量都是把它的存储接口接到 PocketBase collection 上。zitadel 要实现约 54 个方法，fosite 约 25 个，还要另写 discovery、JWKS、userinfo。而窄 profile 自研的核心状态机只有四件事：校验授权请求、签发授权码、换 token、轮换 refresh token。代码量与写适配层相当。
2. 两个关键安全要求，库要么做不到，要么得在适配层里补：只允许 S256，以及所有客户端都强制 PKCE。zitadel v3 两条都要在 `CreateAuthRequest` 存储方法里手工拒绝，RFC 9207 也要自己补。
3. 自研可以直接复用仓库已有的模式：`passkey_sessions` 式的一次性短期记录、Management API 的 secret 只回显一次与 SHA-256 存储、`cryptobox` 加密私钥、`passkeyRateLimiter` 限流。也不会引入第二套路由和 CORS。
4. 风险在于自己写出协议漏洞。对策有两条：一是用 OpenID Foundation 的开源 [conformance suite](https://openid.net/certification/about-conformance-suite/) 跑 Basic OP 测试计划；二是第 6 节的威胁清单逐条写成单元测试。Gitea 也采用了自己实现的方案，见 [go-gitea/gitea services/oauth2_provider](https://github.com/go-gitea/gitea/tree/main/services/oauth2_provider)。

**备选：zitadel/oidc v3。** 如果维护者不想自己承担协议正确性，就选它。它维护活跃，Apache-2.0，功能齐全，login URL 回调模式也与本方案的前端同意页契合。代价是：

- 要写约 54 个方法的适配层。
- 要在适配层里拒绝 `plain` 并对所有客户端强制 PKCE。
- 要去掉或协调它的 rs/cors。
- 没有 RFC 9207。
- 它的 OP 没有经过认证。

**不推荐：**

- fosite 独立仓库：已停止开发。
- go-oauth2：没有 OIDC。
- luikyv/go-oidc：v0.x，单人维护，客户端模型与「admin 在数据库里管理客户端」不匹配。
- osin：不再维护。

**外部 IdP 对照。** Dex、Authelia、Kanidm、ZITADEL、Keycloak、authentik、Pocket ID 都能作为独立 OP 运行。但它们要么有自己的用户库，要么只能通过 LDAP 一类连接器联合外部用户。见 [Dex connectors](https://github.com/dexidp/dex#connectors) 与 [Kanidm](https://github.com/kanidm/kanidm)。没有一个能直接读本项目的 `users` collection。要用面板账号登录，要么让面板先成为 OIDC provider（又回到本方案），要么同步用户。后者还会让 `status` / `verified` 门禁分散到两个系统。对本项目来说，外部 IdP 不能省掉工作，只会多一个要部署的服务。

## 5. 本仓库集成设计

### 5.1 与现有规则的冲突，需要先处理

- AGENTS.md「后端职责边界……不要擅自扩张」里没有「身份提供方」。本功能需要需求方明确同意，并新增一份 ADR，例如 `docs/adr/0010-oidc-provider.md`，同时更新 AGENTS.md 的职责列表。
- 命名冲突，有两处：
  - 仓库已有 `oauth2_login.go`、`/api/panel/oauth2-redirect`、前端 `/oauth-callback`、`/users/{id}/oauth-links`，这些都是「面板作为 OAuth client」。provider 一侧建议统一用 `oidc` 前缀：后端文件 `oidc_*.go`，协议路由 `/api/oidc/*`，collection `oidc_*`，避免混淆。
  - `CONTEXT.md` 把 "Client" 用在 Node Client 语义上，且列为 User 的避免用词（`CONTEXT.md:9`）。第三方应用在术语表里建议叫 **Connected App**，代码标识符用 `oidc_client`，并在术语表里写清楚它与 Node Client 的区别。
- 核心决策 #8：协议端点由第三方调用，与 `/api/mgmt/*` 一样不进 OpenAPI。admin 的客户端管理接口、同意页用到的面板接口都进 OpenAPI。
- 核心决策 #4 与迁移 `1730000030`：新 collection 的 List/View/Create/Update/Delete 规则全部设为 `nil`，只有 superuser 能访问，杜绝 PocketBase 自动 API 泄露。
- 所有日期字段写 `time.Now().UTC()`。

### 5.2 配置与发现

- issuer 定为 `PANEL_BACKEND_URL_BASE + "/api/oidc"`。按 OIDC Discovery §4，配置地址是 `/api/oidc/.well-known/openid-configuration`，正好落在 nginx 的 `/api/` 反代规则里（`deploy/nginx/default.conf:7-14`），不用改 nginx。
- RFC 8414 形式的 `/.well-known/oauth-authorization-server/api/oidc` 位于根路径，会被 nginx 转发给前端。第一版不提供它，纯 OIDC RP 不需要。
- 开启功能的前提：`PANEL_BACKEND_URL_BASE` 为 https（localhost 例外），且 `PANEL_FRONTEND_URL_BASE` 已设置，见 `backend/internal/config/config.go:19-25` 与 `config.go:86-87` 的 `PanelBackendURL()`。条件不满足时 `PATCH /settings` 返回 400。issuer 一旦对外公布就不能改，否则所有 RP 失效，这一点要写进 README。

### 5.3 Collection 与迁移

建议一个迁移 `1730000032_create_oidc_provider.go`。

| Collection | 字段 | 说明 |
|---|---|---|
| `oidc_clients` | `client_id`（unique，随机 16 字节）、`name`、`secret_hash`（hidden，SHA-256 hex）、`redirect_uris`（json 字符串数组）、`allowed_scopes`（json）、`refresh_enabled`（bool）、`skip_consent`（bool，默认 false）、`enabled`（bool）、`created_by`（relation→users）、`last_used_at` | secret 只在创建与轮换时回显一次，与 Management API token 一致（`settings.go:124-161`）。第一版只有 confidential 类型，`token_endpoint_auth_method` 固定 |
| `oidc_auth_requests` | `request_id`（unique，32 字节）、`client`、`redirect_uri`、`scope`、`state`、`nonce`、`code_challenge`、`prompt`、`max_age`、`expires_at`（10 分钟）、`consumed_at` | 后端校验完参数后落库，前端只拿 `request_id`。用后即焚，与 `consumePasskeySession` 同一模式（`passkeys.go:481-508`） |
| `oidc_codes` | `code_hash`（unique，hidden）、`grant`、`redirect_uri`、`nonce`、`code_challenge`、`auth_time`、`expires_at`（60 秒）、`used_at` | 兑换时在事务里检查并写入 `used_at`。已被使用则吊销对应 grant |
| `oidc_grants` | `user`（relation，cascade delete）、`client`（relation，cascade delete）、`scopes`、`created`、`revoked_at`、`revoke_reason`、`last_used_at` | 一个用户对一个应用的一次授权，也是同意记录与 refresh token family。吊销 grant 等于吊销其下所有 token |
| `oidc_tokens` | `token_hash`（unique，hidden）、`kind`（access/refresh）、`grant`、`expires_at`、`revoked_at`、`rotated_at` | refresh 重放判定：命中的记录 `rotated_at` 非空，即视为重放，吊销整个 grant |
| `oidc_signing_keys` | `kid`、`alg`（RS256）、`private_key`（hidden，`cryptobox` 加密的 PKCS#8）、`public_jwk`（json）、`state`（active/retired）、`retired_at` | 首次开启功能时生成 RSA-2048。如果 `PANEL_MASTER_KEY` 变了导致解不开私钥，就生成新 key；RP 会按新 `kid` 重新拉 JWKS，只是已签发的 ID Token 会验不过 |
| `oidc_session_auth_times` | `token_hash`（unique，hidden）、`user`（relation，cascade delete）、`auth_time`、`expires_at` | 面板会话 token 的真实登录时间，刷新时随 token 转移。见 5.12 节 |

定时清理：在 `app.Cron()` 加一个任务，每 30 分钟删除过期的 auth request 与 code，以及过期超过 7 天的 token。

### 5.4 端点与 `/authorize` 跨域流程

协议端点。公开，不进 OpenAPI，统一挂一个 `requireOIDCEnabled` 中间件，功能关闭时返回 404：

| 方法 路径 | 作用 |
|---|---|
| `GET /api/oidc/.well-known/openid-configuration` | discovery |
| `GET /api/oidc/jwks` | 公钥，active 加未过保留期的 retired |
| `GET /api/oidc/authorize` | 校验参数并落库，然后 303 到前端 |
| `POST /api/oidc/authorize/{request_id}/abort` | 不需要登录。仅用于 `prompt=none` 且未登录时，返回带 `login_required` 的错误重定向地址。只能把错误发到已注册的 redirect_uri |
| `POST /api/oidc/token` | `authorization_code` 与 `refresh_token` 两种 grant |
| `GET`/`POST /api/oidc/userinfo` | Bearer access token |
| `POST /api/oidc/revoke` | RFC 7009，需要客户端认证 |

面板端点。进 OpenAPI，走 `/api/panel` 现有的 `RequireAuth("users")`：

| 方法 路径 | 守卫 |
|---|---|
| `GET /api/panel/oidc/requests/{request_id}` | 任何已登录且可用的用户。返回应用名、请求的 scope、redirect 的 host |
| `POST /api/panel/oidc/requests/{request_id}/approve` / `deny` | 同上。返回 `{ redirect_to }` |
| `GET`/`POST /api/panel/oidc-clients`，`GET`/`PATCH`/`DELETE /api/panel/oidc-clients/{id}`，`POST /api/panel/oidc-clients/{id}/rotate-secret` | `requireAdmin` |
| `GET /api/panel/users/{id}/oidc-grants`，`DELETE /api/panel/users/{id}/oidc-grants/{grantId}` | `requireActiveAdminOrSelf`，与 oauth-links 一致 |

流程：

1. RP 把浏览器带到 `GET /api/oidc/authorize?...`。后端按以下顺序校验：
   - 客户端存在且 enabled。
   - `redirect_uri` 与注册值逐字符相等。
   - 这两项不通过时，**不重定向**，303 到前端错误页 `/oidc/error?reason=...`，见 RFC 6749 §4.1.2.1。
   - 其余参数：`response_type=code`；`code_challenge_method=S256`；`code_challenge` 存在；scope 含 `openid` 且都在 `allowed_scopes` 内。不通过时带 `error`、`state`、`iss` 303 回 `redirect_uri`。
2. 通过后写入 `oidc_auth_requests`，303 到 `PANEL_FRONTEND_URL_BASE + "/oidc/authorize?request_id=..."`。
3. 前端路由 `/oidc/authorize` 在 `beforeLoad` 里判断登录状态：
   - 未登录时跳 `/login?redirect=<本页>`。`/login` 目前没有回跳参数，见 `frontend/src/routes/login.tsx:13-18`，需要新增。新增时只接受以单个 `/` 开头的站内路径，拒绝 `//` 和带 scheme 的值，否则会造出新的开放重定向。
   - `prompt=none` 且未登录时，调用 abort 端点后跳回 RP。
4. 前端用 `GET /api/panel/oidc/requests/{id}` 渲染同意页，展示应用名、scope 和回跳 host。用户点允许，前端发 `POST .../approve`，后端按顺序做：
   - 用 `e.Auth` 绑定用户，检查 `status=active && verified=true`。
   - 原子地消费 request。
   - 已有覆盖这些 scope 的未吊销 grant 时复用，否则新建。
   - 签发授权码，返回 `redirect_uri?code=...&state=...&iss=...`。
5. 前端执行 `window.location.assign(redirect_to)`。

这个设计对 CSRF 有天然防护。后端只认 `Authorization` 头，不认 cookie，跨站页面无法替用户带上 token 发 approve 请求，见 3.3 节与 [RFC 9700 §4.7](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.7)。`request_id` 为 32 字节随机数且一次性，攻击者拿到也只能让受害者看到一个真实的同意页。

点击劫持：前端 `/oidc/authorize` 的响应要带 `Content-Security-Policy: frame-ancestors 'none'`、`X-Frame-Options: DENY` 和 `Referrer-Policy: no-referrer`。可以在 Nitro 的 route rules 里加，也可以在 nginx 里按路径加。具体用哪种方式这次没有验证。

### 5.5 Token 端点

- 客户端认证：`client_secret_basic` 与 `client_secret_post` 二选一，同时出现返回 `invalid_request`。按 SHA-256 后用 `subtle.ConstantTimeCompare` 比对，复用 `verifyManagementAPIToken` 的写法（`settings.go:178-184`）。
- `authorization_code` 在一个事务里完成：
  - 查 code_hash。
  - `used_at` 非空时吊销 grant，返回 `invalid_grant`。
  - 检查未过期、client 一致、`redirect_uri` 一致（[RFC 6749 §4.1.3](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.3)），以及 PKCE。
  - 写 `used_at`，签发 access token、可选的 refresh token 和 ID Token。
- `refresh_token`：
  - 查 token_hash。
  - `rotated_at` 非空时吊销 grant，返回 `invalid_grant`。
  - 否则标记旧值并签发新的 refresh token 与 access token。
  - 请求的 scope 只能缩小。
- 用户门禁：每次兑换都重新读用户，要求 `status=active && verified=true`，否则返回 `invalid_grant`。门禁逻辑抽成一个函数，例如 `oidcUserUsable(client, user)`。将来要按应用限制用户，例如只允许 admin 或只允许有订阅的用户，只改这一个函数，做法与 `nodesForUser` 相同（`api.go:344-349`）。
- ID Token：
  - claim：`iss`、`sub=user.id`、`aud=client_id`、`exp`、`iat`、`auth_time`、`nonce`（请求里有时）、`at_hash`。
  - 头部：`kid` 与 `alg=RS256`。
- 响应带 `Cache-Control: no-store`。

### 5.6 用户停用与传播

- 在 `OnRecordAfterUpdateSuccess("users")` 里处理：
  - `status` 由 active 变为其他值，或 `verified` 由 true 变 false 时，吊销该用户全部 grant，`revoke_reason=user_disabled`。
  - 用 record hook 而不是在 `updateUser` 里处理，是为了同时覆盖 `/_/` 里的修改。
  - 是否在改密码时也吊销，作为待决问题。PocketBase 改密码会轮换 `tokenKey`（[core/record_model.go#L1437-L1441](https://github.com/pocketbase/pocketbase/blob/v0.39.0/core/record_model.go#L1437)），但它管不到 OIDC token。
- 用户删除时，`oidc_grants.user` 的 cascade delete 一并清掉。
- 吊销之外还有兜底：token、refresh、userinfo 每次请求都重新检查门禁。所以即使 hook 失败，停用也会在下一次请求时生效。
- 对已签出的 ID Token 无法追回。它们 10 分钟过期，RP 的会话长度由 RP 决定，这一点要写进文档。不做 back-channel logout，它不在基础范围内。

### 5.7 暴露的用户 claim

| scope | claim | 说明 |
|---|---|---|
| `openid` | `sub` | `user.id`，PocketBase 随机生成的 15 位 ID。PocketBase 没有显式保证删除后不再生成同值，只是碰撞概率可忽略；这一点本次未查源码确认。不用 email 当 `sub`，见 OIDC Core §5.7 |
| `email` | `email`、`email_verified` | 门禁保证 `verified=true`，所以 `email_verified` 恒为 true |

第一版不支持 `profile` scope，也不暴露 `role`、`auth_string`、`used_tx`/`used_rx`、`recent_connections`、`remark`、订阅信息。这个范围按首个接入方 help-center-next 的实际用法确定，见 5.11 节。

### 5.8 限流、日志、审计

- 限流：复用 `newPasskeyRateLimiter`，按 `e.RealIP()` 计数。建议 `/authorize` 每分钟 60 次，`/token` 与 `/revoke` 每分钟 30 次。`/token` 客户端认证失败另按 client_id 计数，防止撞 secret。
- 日志：绝不记录 code、token、client secret、`code_verifier`。PocketBase 请求日志会记 URL。`/authorize` 的 query 里只有 `state`、`nonce`、`code_challenge`，不含秘密。带 code 的 `Location` 头不进日志。仍需人工确认 activity log 不记录 POST body。这一点这次没有核对源码。
- 审计：本项目对 Monitoring 明确不做审计日志（AGENTS.md）。建议保持一致，只在 `oidc_grants` 记 `created`、`last_used_at`、`revoked_at`、`revoke_reason`，在 `oidc_clients` 记 `last_used_at`，让 admin 和用户能看到「哪个应用、何时授权、何时吊销」。

### 5.9 前端

- 新路由：`/oidc/authorize`（同意页）、`/oidc/error`。
- 新页面：`/settings/oidc-clients` 的列表、新建、编辑，以及新建与轮换后只显示一次 secret 的对话框，可参照 Management API token 的现有交互。另外在设置页加「已授权的应用」列表与吊销按钮。
- `/login` 加安全的 `redirect` 参数，passkey 与 OAuth2 社交登录完成后也要走这个回跳。
- 表单用 TanStack Form，数据走 `queries.ts`，文案在 `en` 与 `zh-cn` 同步加键，按 AGENTS.md 前端约定执行。

### 5.10 测试

后端单测按威胁逐条写：

- redirect_uri 精确匹配：尾斜杠、大小写、query、fragment、userinfo 都要覆盖。
- `plain` PKCE 被拒，缺少 challenge 被拒，verifier 错误时返回 `invalid_grant`。
- code 过期、重放、重放后 grant 被吊销。
- refresh 轮换，旧值重放后整个 family 被吊销。
- client secret 错误，basic 与 post 同时出现。
- 停用、未验证用户在 approve、token、refresh、userinfo 各处都被拒。
- `/_/` 改 `status` 触发吊销。
- 功能关闭时所有端点返回 404。
- ID Token 的 claim 与签名，`nonce` 原样回传，`at_hash` 正确。
- discovery 的 `issuer` 与 ID Token 的 `iss` 一致。
- 授权响应带 `iss`。

最后用 conformance suite 的 Basic OP 计划做一次端到端验证。

### 5.11 首个接入方：help-center-next

`/Users/yihang/Development/teehee-network/help-center-next`（commit `187f4e0`）目前用 next-auth 4 的 Auth0 provider 登录，再用 Management API 按 email 查面板账号。它用到的数据：

| 数据 | 来源 | 用途 | 接入后 |
|---|---|---|---|
| `sub` | Auth0 profile | 登录回调要求非空（`pages/api/auth/[...nextauth].ts:30`） | 面板 `user.id` |
| `email` | Auth0 profile | 按 email 查面板账号；Drift 客服挂件的 `userId`（`components/base-layout/index.tsx:43-50`） | `email` scope |
| `email_verified` | Auth0 profile | 未验证时调 Auth0 Management API 改成已验证（`[...nextauth].ts:44-49`） | 面板恒为 true，这段逻辑可删 |
| `name` | Auth0 profile | 头部显示名与头像首字母，缺省回退 `U`（`components/header/index.tsx:110,182`）；Drift 属性 | 面板不维护 `name`，不提供。头部需改为回退到 email |
| `image` | Auth0 profile | 头部头像（`components/header/index.tsx:68`） | 不提供 |
| `roles` | Auth0 Management API | Tab 和 API 的按角色过滤（`components/header/index.tsx:84`，`middlewares/auth.ts:20-26`） | 当前没有任何 Tab 或 API 设置 `role`，机制未使用。不提供 |
| `auth_string` | 面板 Management API | 写进 `config.teehee.cloud/embed/*` iframe 的 query（`libs/client-utils/patch-embed-iframes.ts:4-16`） | 继续走 Management API |
| `status=active` | 面板 Management API | 每次 SSR 和 API 请求都重查，停用即登出（`libs/client-utils/ssp-wrapper.ts:37-52`，`middlewares/auth.ts:33-43`） | 继续走 Management API |

`auth_string` 是节点凭据，不进 ID Token 或 userinfo。help center 的会话比 access token 长，本来就要按请求重查账号状态，而 Management API 已经同时返回 `status` 与 `auth_string`。所以只把登录换成 OIDC，账号查询保持不变。

面板侧需要的配套改动：`GET /api/mgmt/users` 增加按 `id` 精确查询，三选一。这样 help center 用 `sub` 查，用户改 email 后不会断开。

help center 侧的接入改动：next-auth 换成通用 OIDC provider（`wellKnown` 指向面板 discovery，`checks: ["pkce", "state", "nonce"]`），删掉 Auth0 Management API 依赖，查询键从 email 换成 `sub`。

### 5.12 `prompt=login` 与 `max_age`

问题：PocketBase 会话 token 只有 `exp`，没有签发时间或认证时间（`core/record_tokens.go` 的 `newAuthToken` 只写 `type`、`id`、`collectionId`、`refreshable`）。而且前端会调用 `auth-refresh` 续期（`frontend/src/api/session.ts:92`），所以也不能用 `exp` 反推登录时间。

方案：面板自己记录每个会话的真实登录时间。

- `OnRecordAuthRequest("users")` 对每次登录都会触发，包括 token 刷新。PocketBase 刷新时传入的 `AuthMethod` 是空字符串（`apis/record_auth_refresh.go:33`）。密码、OAuth2、OTP 分别传 `password`、`oauth2`、`otp`（`apis/record_auth_with_*.go`），本项目的 passkey 登录传 `passkey`（`backend/internal/api/passkeys.go:199`）。
- 新增 collection `oidc_session_auth_times`：`token_hash`（unique，hidden）、`user`（cascade delete）、`auth_time`、`expires_at`。
- 在现有 hook（`backend/internal/api/api.go:326`）里：
  - `AuthMethod` 非空时，写入 `sha256(e.Token)` 与当前 UTC 时间。
  - `AuthMethod` 为空（刷新）时，用请求头里旧 token 的 hash 查出 `auth_time`，复制到新 token 名下。
  - cron 清理与授权码清理放在同一任务里，删除已过期的记录。
- approve 时用请求的 `Authorization` 头算 hash，查出本会话的 `auth_time`。查不到（功能上线前签发的 token）就视为未知。

授权行为：

| 请求 | 行为 |
|---|---|
| 无 `prompt`、无 `max_age` | 用现有会话。已记住同意或应用标为跳过同意时，前端直接 approve，用户只看到一次跳转。ID Token 带已知的 `auth_time` |
| `max_age=N` | `auth_time` 已知且距今不超过 N 秒时同上；否则按 `prompt=login` 处理 |
| `prompt=login` | 同意页提示「该应用要求重新验证身份」，按钮跳到 `/login?reauth=1&redirect=<同意页>`。用户可用任意登录方式，包括社交登录。回来后 approve 要求 `auth_time` 晚于授权请求创建时间 |
| `prompt=consent` | 总是显示同意页，即使已记住同意或应用标为跳过 |
| `prompt=none` | 未登录返回 `login_required`；需要重新认证返回 `login_required`；需要同意返回 `consent_required`。不显示任何页面 |
| `prompt=select_account` | 与 `prompt=consent` 相同，总是显示同意页。同意页一直显示当前账号 email 与「切换账号」链接，用户可以在这里换账号。总是带这个值的 RP 也能正常登录 |

选这个方案的理由：

- 所有登录方式都能完成重新认证。通过 Management API 建的账号，用户不知道自动生成的密码（`backend/internal/api/management_api.go:81`），只靠「输入密码」做重新认证会把这些人挡在外面。
- `auth_time` 按会话记录，另一台设备上的新登录不会让旧会话满足 `max_age`。
- 重新认证就是一次普通登录，面板会话随之更新。不需要额外的 passkey challenge 类型。
- 前端要改两处。第一，`/login` 的 `beforeLoad` 遇到已登录会直接跳到 `/`（`frontend/src/routes/login.tsx:14-18`），`reauth=1` 时要跳过这个判断，显示登录表单。第二，密码、passkey、社交登录成功后都跳到固定地址（`login.tsx:33,39`，`oauth-callback.tsx:79`），三条路径都要改为跳回 `redirect` 参数指定的站内路径。

## 6. 威胁清单

| 威胁 | 防御 | 依据 |
|---|---|---|
| 开放重定向（授权服务器侧） | redirect_uri 精确匹配；校验失败不重定向；先认证再跳转 | RFC 9700 §4.1、§4.11.2；RFC 6749 §4.1.2.1 |
| 开放重定向（面板 `/login?redirect=`） | 只接受站内路径 | RFC 9700 §4.11 |
| 授权码截获或注入 | 强制 S256 PKCE；code 60 秒、一次性、hash 存储；兑换时核对 client 与 redirect_uri | RFC 9700 §4.5；RFC 7636 |
| PKCE 降级 | 授权请求没带 challenge 就拒绝，而不是放行 | RFC 9700 §4.8 |
| authorize / consent 的 CSRF | 面板接口只认 `Authorization` 头；`request_id` 一次性；`state` 原样回传 | RFC 9700 §4.7 |
| Mix-up | 授权响应带 `iss`，元数据声明支持；RP 端也要校验 | RFC 9207；RFC 9700 §4.4 |
| 经 Referer 或浏览器历史泄露 | 同意页 `no-referrer`、不加载第三方资源；code 短期且一次性；token 只出现在后端对后端的响应里 | RFC 9700 §4.2、§4.3 |
| 经日志泄露 | 不记录 code、token、secret；secret 与 token 只存 hash | 工程约定 |
| 公共客户端冒充 | 第一版不开放公共客户端；以后开放时强制 PKCE 与 refresh 轮换，同意页不允许跳过 | RFC 9700 §2.2.2、§4.14 |
| 跳过同意 | `skip_consent` 默认 false，只有 admin 能开；`prompt=consent` 始终显示同意页；scope 扩大必须重新同意 | OIDC Core §3.1.2.1 |
| 会话固定 | 不在后端建任何浏览器会话；授权请求在 approve 时才绑定用户，且只能消费一次 | 设计如此 |
| 点击劫持 | 同意页 `frame-ancestors 'none'` 与 `X-Frame-Options: DENY` | RFC 9700 §4.16 |
| 307 带凭据重放 | 全部用 303 | RFC 9700 §4.12 |
| SSRF | 不做动态注册，不接受 `request_uri`、`jwks_uri`、`sector_identifier_uri`；后端不主动向 RP 发请求 | 设计如此 |
| refresh token 被盗 | 轮换加重放检测；绝对寿命上限；用户停用时吊销 | RFC 9700 §4.14.2 |
| 签名私钥泄露 | 私钥用 `cryptobox` 加密，`PANEL_MASTER_KEY` 不入库；泄露后轮换 key 并立即从 JWKS 移除旧 key，已签出的 ID Token 随即失效 | OIDC Core §10.1.1 |
| client secret 泄露 | 只存 hash；admin 可一键轮换；停用客户端立即拒绝该客户端的全部 token | OIDC Core §16.19 |
| 面板会话与 OAuth token 混用 | 两套 token 格式与存储完全分离；OAuth access token 不是 JWT，不会被 `loadAuthToken` 识别 | 第 3.2 节 |
| 爆破 `/token` | 按 IP 与 client_id 限流 | 工程约定 |

## 7. 工作量与待决问题

### 7.1 工作量估算（人日）

| 部分 | 内容 | 估算 |
|---|---|---|
| 后端 | 迁移与 collection | 0.5 |
| | 签名 key、JWKS、discovery | 0.5 |
| | `/authorize`、请求记录、abort | 1 |
| | approve / deny 面板接口 | 0.5 |
| | `/token`（客户端认证、code、refresh 轮换、ID Token） | 2 |
| | userinfo、revoke | 0.5 |
| | admin 客户端 CRUD、轮换 secret、设置开关、DTO / OpenAPI | 1 |
| | 吊销 hook、cron 清理、限流 | 0.5 |
| 前端 | 同意页、错误页、`/login` 回跳 | 1.5 |
| | 客户端管理页、secret 一次性展示 | 1.5 |
| | 设置开关、已授权应用列表、i18n | 1 |
| 测试 | 后端单测（第 5.10 节） | 2 |
| | conformance suite 搭建与修复 | 1 到 2 |
| 文档 | ADR、AGENTS.md、README、CONTEXT.md 术语 | 0.5 到 1 |
| 合计 | | 约 15 到 18，其中约 1 人日来自 5.11 与 5.12 节 |

如果改用 zitadel/oidc v3，后端协议部分大约省 2 天，但适配层与 CORS 协调要多花 1 到 2 天。总量差不多。

### 7.2 决策（2026-10-12）

已记录在 `docs/adr/0010-oidc-provider.md`。

| # | 问题 | 决定 |
|---|---|---|
| 1 | 是否纳入后端职责边界 | 接受，写 ADR-0010。实现时同步更新 AGENTS.md 职责列表与 `CONTEXT.md` 术语 |
| 2 | 自研还是 zitadel/oidc v3 | 自研窄 profile |
| 3 | 同意是否按「用户 + 应用 + scope」记住 | 记住，scope 扩大时再问 |
| 4 | admin 能否把应用标为跳过同意 | 允许，默认关闭 |
| 5 | 是否支持公共客户端 | 不支持 |
| 6 | 是否签发 refresh token | 签发。客户端 `refresh_enabled` 为真且请求含 `offline_access` 时才发 |
| 7 | 暴露哪些 claim | `sub`、`email`、`email_verified`。依据见 5.11 节 |
| 8 | 谁能登录 | 所有 `status=active` 且 `verified=true` 的用户 |
| 9 | `prompt=login` 与 `max_age` | 回到面板登录页重新登录，按会话记录 `auth_time`。见 5.12 节 |
| 10 | token 寿命 | 写死为 2.1 节的建议值 |
| 11 | 改密码、删 passkey 是否吊销 grant | 不吊销 |
| 12 | 签名 key 轮换 | 只提供 CLI 子命令 |

## 8. 参考来源

规范：

- RFC 6749 OAuth 2.0：https://www.rfc-editor.org/rfc/rfc6749.html
- RFC 6750 Bearer Token：https://www.rfc-editor.org/rfc/rfc6750.html
- RFC 7009 Token Revocation：https://www.rfc-editor.org/rfc/rfc7009.html
- RFC 7518 JWA：https://www.rfc-editor.org/rfc/rfc7518.html
- RFC 7636 PKCE：https://www.rfc-editor.org/rfc/rfc7636.html
- RFC 7662 Token Introspection：https://www.rfc-editor.org/rfc/rfc7662.html
- RFC 8414 AS Metadata：https://www.rfc-editor.org/rfc/rfc8414.html
- RFC 8725 JWT BCP：https://www.rfc-editor.org/rfc/rfc8725.html
- RFC 9207 Issuer Identification：https://www.rfc-editor.org/rfc/rfc9207.html
- RFC 9700 OAuth 2.0 Security BCP：https://www.rfc-editor.org/rfc/rfc9700.html
- RFC 9864 Fully-Specified Algorithms：https://datatracker.ietf.org/doc/html/rfc9864
- OAuth 2.1 draft-16：https://datatracker.ietf.org/doc/draft-ietf-oauth-v2-1/
- OpenID Connect Core 1.0：https://openid.net/specs/openid-connect-core-1_0.html
- OpenID Connect Discovery 1.0：https://openid.net/specs/openid-connect-discovery-1_0.html
- OpenID 认证名单：https://openid.net/certification/certified-openid-connect-implementations/
- OpenID conformance suite：https://openid.net/certification/about-conformance-suite/

核对说明：RFC 9700、RFC 6749 §4.1.2 / §4.1.2.1 / §4.1.3、RFC 7636、RFC 7009、RFC 9207、OIDC Core §2 / §3.1.2.1 / §3.1.3.6 / §5.3 / §5.4 / §5.7 / §10.1.1 / §15.1 / §16.18 / §16.19、OIDC Discovery §3 / §4、OAuth 2.1 draft-16 状态，本次都抓取原文核对过。RFC 6749 §5.1、RFC 6750 §2.3、RFC 7518 §3.3、RFC 8414 §2、RFC 8725 §3.1、OIDC Core §11 本次没有重新抓取原文，内容依据既有了解，引用前建议再核对一次。OAuth 2.1 草案中 refresh token 轮换的具体章节（§4.3.3）正文本次未能读到。

PocketBase（v0.39.0）：

- 源码：https://github.com/pocketbase/pocketbase/tree/v0.39.0 ，文中链接均固定到该 tag
- discussions：[#1144](https://github.com/pocketbase/pocketbase/discussions/1144)、[#3740](https://github.com/pocketbase/pocketbase/discussions/3740)、[#7451](https://github.com/pocketbase/pocketbase/discussions/7451)、[#7857](https://github.com/pocketbase/pocketbase/discussions/7857)
- 社区插件：https://github.com/benjamesfleming/pocketbase-ext-oauth2

Go 库：

- ory/fosite：https://github.com/ory/fosite ，并入 hydra 的提交：https://github.com/ory/hydra/commit/2c3ba1311e196d226dc2904ed7507481d9479cdd
- zitadel/oidc：https://github.com/zitadel/oidc ，通告 GHSA-j8gq-92xf-382c
- go-oauth2/oauth2：https://github.com/go-oauth2/oauth2
- luikyv/go-oidc：https://github.com/luikyv/go-oidc
- openshift/osin：https://github.com/openshift/osin
- authelia/oauth2-provider：https://github.com/authelia/oauth2-provider
- pocket-id/fosite：https://github.com/pocket-id/fosite
- go-jose：https://github.com/go-jose/go-jose
- Gitea 的自研 provider：https://github.com/go-gitea/gitea/tree/main/services/oauth2_provider

外部 IdP：Dex https://github.com/dexidp/dex ，Kanidm https://github.com/kanidm/kanidm ，ZITADEL https://github.com/zitadel/zitadel ，Authelia https://github.com/authelia/authelia ，authentik https://github.com/goauthentik/authentik ，Pocket ID https://github.com/pocket-id/pocket-id ，Keycloak https://github.com/keycloak/keycloak 。

本仓库文件：`backend/internal/api/{api.go,settings.go,management_api.go,passkeys.go,users.go,oauth2_login.go}`、`backend/internal/panelserve/cors.go`、`backend/internal/cryptobox/cryptobox.go`、`backend/internal/config/config.go`、`frontend/src/api/{cookie.ts,client.ts,auth.ts}`、`frontend/src/routes/login.tsx`、`deploy/nginx/default.conf`、`CONTEXT.md`。
