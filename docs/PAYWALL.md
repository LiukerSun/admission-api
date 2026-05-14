# 付费弹窗 + 后端权限校验

> 一次实现，长期演进：把"哪些接口收费"从代码硬决策中解耦出来，让产品/运营可以一行代码完成切换。

---

## 1. 设计哲学

### 1.1 核心目标

- **后端**：声明式付费保护——把路由放进 `premium` 路由组就受保护，移出就免费
- **前端**：功能无关的拦截层——只看后端返回的统一 response 就触发弹窗，不关心接口路径
- **结果**：未来切换某接口为付费或免费，只需修改 `cmd/api/main.go` 一行

### 1.2 关键不变量

| 不变量 | 落实位置 |
|--------|---------|
| HTTP 状态码 `403` + 响应体 `code === 1010` ⇔ 付费墙 | 后端 [middleware/membership.go](../internal/platform/middleware/membership.go) / 前端 [services/paywall.ts](../../admission-frontend/src/services/paywall.ts) |
| 弹窗触发是单点函数 `triggerPaywallIfMatch` | 前端 [services/paywall.ts](../../admission-frontend/src/services/paywall.ts) |
| 弹窗触发后自动 `refreshMembership`（无须调用方关心） | 同上 |

---

## 2. 架构概览

```
┌──────────────────────────── 后端 admission-api ─────────────────────────────┐
│                                                                            │
│   premium := authorized.Group("")                                          │
│   premium.Use(RequireActiveMembership(membershipService))                  │
│   premium.POST("/ai/chat", ...)         ← 付费接口                          │
│                                                                            │
│   ┌───────────────────────────────┐                                        │
│   │ RequireActiveMembership       │                                        │
│   │  └─ HasActiveMembership(uid)  │                                        │
│   │      └─ 否 → 403 + code:1010  │ ┐                                      │
│   └───────────────────────────────┘ │                                      │
│                                     │                                      │
│           ┌─ 统一响应体 ─────────────┘                                      │
│           ▼                                                                │
│   {                                                                        │
│     "code": 1010,                                                          │
│     "message": "active membership required",                               │
│     "data": {                                                              │
│       "reason": "membership_required",                                     │
│       "required_level": "premium",                                         │
│       "recommended_plan": "quarterly",                                     │
│       "checkout_url": "/membership"                                        │
│     }                                                                      │
│   }                                                                        │
└────────────────────────────────────────────────────────────────────────────┘
                          │
                          │ HTTP 403 response
                          ▼
┌──────────────────────── 前端 admission-frontend ───────────────────────────┐
│                                                                            │
│  ┌─ axios interceptor ────────────────┐  ┌─ SSE fetch path ──────────────┐ │
│  │ if (status === 403) {              │  │ if (!response.ok) {           │ │
│  │   triggerPaywallIfMatch(data)      │  │   if (status === 403) {       │ │
│  │ }                                  │  │     paywall=tryParsePaywall.. │ │
│  └────────────────┬───────────────────┘  │     triggerPaywallIfMatch(..) │ │
│                   │                      │   }                           │ │
│                   ▼                      └────────────┬──────────────────┘ │
│   ┌─ services/paywall.ts ─────────────────────────────▼─────────────────┐  │
│   │ triggerPaywallIfMatch(payload)                                      │  │
│   │  └─ if isPaywallResponse(payload):                                  │  │
│   │       └─ paywallStore.openPaywall({ recommendedPlan, ... })         │  │
│   │       └─ authStore.refreshMembership()  ← 内置副作用                 │  │
│   └─────────────────────────┬───────────────────────────────────────────┘  │
│                             │                                              │
│                             ▼                                              │
│   ┌─ paywallStore (zustand) ────────────────────────────────────────────┐  │
│   │ { open, recommendedPlan, featureName, trigger, openPaywall, ... }   │  │
│   └─────────────────────────┬───────────────────────────────────────────┘  │
│                             │ subscription                                 │
│                             ▼                                              │
│   ┌─ <PaywallModal /> (mounted in BasicLayout) ─────────────────────────┐  │
│   │ dark luxury 弹窗 — 季卡金缎 — CTA → /membership?plan=quarterly       │  │
│   └─────────────────────────┬───────────────────────────────────────────┘  │
│                             │ navigate                                     │
│                             ▼                                              │
│   ┌─ /membership 页 ─ selectHighlightedPlan(plans, ?plan=) ─────────────┐  │
│   │ 自动 scrollIntoView + 金色高亮目标卡片                                │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 后端

### 3.1 路由组

[cmd/api/main.go](../cmd/api/main.go)：

```go
authorized := api.Group("")
authorized.Use(middleware.JWTMiddleware(jwtConfig))
authorized.Use(middleware.AuthStatusMiddleware(...))
// authorized 路由：登录用户可访问

premium := authorized.Group("")
premium.Use(middleware.RequireActiveMembership(membershipService))
// premium 路由：登录 + 会员有效才可访问
```

**切换某路由为付费/免费**：把声明从 `authorized.POST(...)` 改为 `premium.POST(...)` 或反向（一行）。

当前 `premium` 内的路由：

| 路由 | 方法 |
|------|------|
| `/api/v1/ai/chat` | POST |
| `/api/v1/conversations/:id/ai-chat` | POST |
| `/api/v1/conversations/:id/regenerate` | POST |
| `/api/v1/conversations/:id/suggestions` | GET |

### 3.2 统一响应体

[internal/platform/web/error.go](../internal/platform/web/error.go) 定义错误码：

```go
ErrCodeMembershipRequired = 1010  // 永久占用：paywall
```

[internal/platform/web/paywall_reason.go](../internal/platform/web/paywall_reason.go) 枚举原因：

```go
PaywallReasonMembershipRequired = "membership_required"
PaywallReasonQuotaExceeded      = "quota_exceeded"  // 预留：配额墙
```

[internal/platform/middleware/membership.go](../internal/platform/middleware/membership.go) 返回：

```go
c.JSON(http.StatusForbidden, web.Response{
    Code:    web.ErrCodeMembershipRequired,
    Message: "active membership required",
    Data: gin.H{
        "reason":           web.PaywallReasonMembershipRequired,
        "required_level":   "premium",
        "recommended_plan": "quarterly",
        "checkout_url":     "/membership",
    },
})
```

### 3.3 复用的现有能力

- `membership.Service.HasActiveMembership(ctx, userID)` — [internal/membership/service.go:68](../internal/membership/service.go#L68)
- `RequireActiveMembership(checker)` middleware — [internal/platform/middleware/membership.go:14](../internal/platform/middleware/membership.go#L14)
- `web.Response` 统一信封 — [internal/platform/web/response.go](../internal/platform/web/response.go)

---

## 4. 前端

### 4.1 模块职责

| 模块 | 职责 |
|------|------|
| [stores/paywallStore.ts](../../admission-frontend/src/stores/paywallStore.ts) | 全局弹窗开关状态（zustand 单例） |
| [services/paywall.ts](../../admission-frontend/src/services/paywall.ts) | 统一判定函数 + 触发副作用（**唯一判定入口**） |
| [services/api.ts](../../admission-frontend/src/services/api.ts) | axios 拦截器，403 → 触发弹窗 |
| [services/ai.ts](../../admission-frontend/src/services/ai.ts) | SSE 路径专属，非 200 时识别 1010 |
| [components/paywall/PaywallModal.tsx](../../admission-frontend/src/components/paywall/PaywallModal.tsx) | dark luxury 弹窗 UI |
| [layouts/BasicLayout.tsx](../../admission-frontend/src/layouts/BasicLayout.tsx) | 弹窗全局挂载点 |
| [stores/authStore.ts](../../admission-frontend/src/stores/authStore.ts) | 缓存 `membership` 用于预判 + `refreshMembership` |
| [utils/membershipHighlight.ts](../../admission-frontend/src/utils/membershipHighlight.ts) | `/membership?plan=X` 高亮目标套餐 |

### 4.2 统一判定函数

[services/paywall.ts](../../admission-frontend/src/services/paywall.ts)：

```ts
export const PAYWALL_CODE = 1010 as const

export function isPaywallResponse(value: unknown): value is PaywallResponse
export function tryParsePaywallText(text: string): PaywallResponse | null
export function triggerPaywallIfMatch(value: unknown, opts?): boolean
```

- `isPaywallResponse`：类型守卫，仅检查 `code === 1010`
- `tryParsePaywallText`：把原始字符串 body 安全解析为付费 payload（SSE 路径专用）
- `triggerPaywallIfMatch`：唯一触发入口，命中后：
  1. 调用 `paywallStore.openPaywall(...)` 弹弹窗
  2. 异步调用 `authStore.refreshMembership()` 刷新缓存（无需调用方关心）
  3. 返回 `true` 表示已处理

### 4.3 触发链路

| 触发点 | 文件 | 行为 |
|--------|------|------|
| 用户在 AI 页发送消息 / 重新生成 | [pages/admission-ai/index.tsx](../../admission-frontend/src/pages/admission-ai/index.tsx) | 先读 `authStore.hasActiveMembership`，未开通直接 `openPaywall`，**不发请求** |
| axios 任意接口返回 403 | [services/api.ts](../../admission-frontend/src/services/api.ts) | response interceptor 内 `triggerPaywallIfMatch(error.response.data)` |
| AI SSE 流首包 403 | [services/ai.ts](../../admission-frontend/src/services/ai.ts) | `tryParsePaywallText(text)` → `triggerPaywallIfMatch(payload)` |

### 4.4 弹窗视觉

- 风格：**dark luxury**（深色渐变 + 金色 + 玻璃磨砂）
- 容器：Antd `Modal`，宽 560，body 自定义
- 背景：`linear-gradient(135deg, #0F172A 0%, #1E293B 50%, #0F172A 100%)` + 噪点纹理
- 推荐套餐：金色边框 + 顶部 "最划算" 缎带
- CTA：金色填充按钮 → `navigate('/membership?plan=' + selectedCode)`
- 关闭：✕ 按钮或点遮罩，ESC 不立即关（防误触）

---

## 5. 常见运维场景

### 5.1 把一个接口改为付费

**单文件单行改动**：

```diff
- authorized.POST("/some-endpoint", handler)
+ premium.POST("/some-endpoint", handler)
```

重启后端即生效。前端零改动——拦截器/SSE handler 自动处理。

### 5.2 把一个接口改回免费

反向操作：

```diff
- premium.POST("/some-endpoint", handler)
+ authorized.POST("/some-endpoint", handler)
```

### 5.3 调整推荐套餐

修改 [middleware/membership.go](../internal/platform/middleware/membership.go) 中 `"recommended_plan": "quarterly"` 为目标 plan_code，重启即可。

> **未来优化（已记 TODO）**：将该字段从 `plan_store` 中带 `is_recommended` 列的套餐读取，避免改 Go 代码。

### 5.4 新增一种付费拦截原因（如配额墙）

复用同一个错误码 1010，仅 `data.reason` 不同：

```go
// 配额超限处理
c.JSON(http.StatusForbidden, web.Response{
    Code: web.ErrCodeMembershipRequired,
    Data: gin.H{
        "reason": web.PaywallReasonQuotaExceeded,  // 不同 reason
        ...
    },
})
```

前端弹窗组件可读 `paywallStore.reason` 切换文案，弹窗本身和触发链路零改动。

### 5.5 排查"用户付完费仍被拦截"

1. 浏览器 DevTools → Network → `/api/v1/membership` 是否返回 `active: true`
2. 控制台执行 `useAuthStore.getState()` 查看 `hasActiveMembership`
3. 若不一致，调用 `useAuthStore.getState().refreshMembership()`
4. 如确认 DB 中 `user_memberships.ends_at < now()`，说明后端权益发放未完成；查 `payment_orders` 表的 `entitlement_status`

---

## 6. 开发与维护

### 6.1 添加测试

| 模块 | 测试位置 | 类型 |
|------|---------|------|
| `paywallStore` | `src/stores/paywallStore.test.ts` | 单元 |
| `paywall.ts` | `src/services/paywall.test.ts` | 单元 |
| `membershipHighlight` | `src/utils/membershipHighlight.test.ts` | 单元 |
| `RequireActiveMembership` middleware | `internal/platform/middleware/membership_test.go` | 单元 |

### 6.2 运行测试

```bash
# 前端
cd admission-frontend
npx vitest run                       # 全部 38 用例
npx tsc --noEmit                     # 类型检查

# 后端
cd admission-api
go test ./...                        # 全部
go test ./internal/platform/middleware/... -v -run TestRequireActiveMembership
```

### 6.3 端到端手工验证

```bash
# 1. 启动服务
go run -C admission-api ./cmd/api &
cd admission-frontend && npx vite &

# 2. 注册无会员用户、调 AI 应当 403/1010
EMAIL="test$(date +%s)@example.com"
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"testpass123\"}"

TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"testpass123\"}" \
  | jq -r .data.access_token)

curl -X POST http://localhost:8080/api/v1/ai/chat \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"hi"}]}'
# 期望：HTTP 403 + {"code":1010,"data":{"reason":"membership_required",...}}

# 3. 浏览器 → http://localhost:5173 登录 → AI 页发送消息 → 弹 dark luxury 弹窗
```

---

## 7. 已知边界与 TODO

| 项 | 当前状态 | 计划 |
|----|---------|------|
| `recommended_plan` 硬编码 | 写死 "quarterly" 在 middleware 内 | 加 `plans.is_recommended` 列 |
| SSE 流中途会员失效 | 仅请求开始时校验一次 | 定期心跳校验 + 中断 stream |
| 推荐接口 `/admission/recommendations` | 后端未实现 | 实现后按需放进 `premium` |
| 配额墙 (`quota_exceeded` reason) | 常量已预留 | 接入实际配额服务时使用 |
| Playwright E2E | 未补 | 覆盖 注册 → 弹窗 → 支付 → 解锁 全流程 |
| `coverage` 工具 | `@vitest/coverage-v8` 未装 | 需要时 `npm i -D @vitest/coverage-v8` |

---

## 8. 测试覆盖

### 前端 8 套件 / 38 用例

| 套件 | 用例数 | 覆盖 |
|------|-------|------|
| `paywallStore.test.ts` | 5 | open/close/no-op/metadata |
| `paywall.test.ts` | 17 | `isPaywallResponse`, `tryParsePaywallText`, `triggerPaywallIfMatch` + refreshMembership 副作用 |
| `membershipHighlight.test.ts` | 5 | `?plan=` query 解析 |
| `authStore.test.ts` | 2 | 含 membership 状态恢复 |
| 其余 | 9 | 既有测试（adminUsers / nextActions / accountCenter / phone） |

### 后端

- `internal/platform/middleware/membership_test.go` — 3 用例，含 1010 + data 字段断言
- `internal/platform/middleware/access_semantics_test.go` — 4 用例

---

## 9. 致谢

本特性遵循以下设计纪律：

- **实事求是**（arming-thought）：调研先于判断
- **api-design**：HTTP 语义、错误码、统一信封
- **frontend-design**：dark luxury 视觉方向、避免通用 SaaS 弹窗
- **TDD 工作流**：RED → GREEN → 重构，38 用例护栏
- **批评与自我批评**：写完检视，识别 `?plan=` 不被识别的 bug 并修复

