# AI 对话子系统 — 联调更新（2026-05-11）

> 本文档记录 PR #22 合并 [feat/ai-chat-enhancement](https://github.com/LiukerSun/admission-api/tree/feat/ai-chat-enhancement) 后，结合 admission-frontend 进行端到端联调时发现的问题与调整。
> 后端代码未做改动，但前端有若干契约/解析层面的修正，本文档同步落档便于后续维护。

---

## 一、本次更新摘要

| 类别 | 内容 | 涉及位置 |
|------|------|----------|
| 联调验证 | 启动 Postgres + Redis + 后端，使用 student@test.com 走通对话 CRUD | 数据库迁移 011 已执行 |
| 契约对齐 | 前端 SSE 解析器修复（之前忽略 `event:` 行） | `admission-frontend/src/services/ai.ts` |
| 契约对齐 | 前端 `Conversation` 类型移除冗余 `status` 字段 | `admission-frontend/src/services/conversation.ts` |
| 契约对齐 | 前端 `list` 响应类型补充 `PaginatedList` 包装 | `admission-frontend/src/services/conversation.ts` |
| UX 优化 | AI 对话页建议区改为 4 卡片 grid，紧贴输入框上方 | `admission-frontend/src/pages/admission-ai/index.tsx` |
| 测试设施 | 新增 Playwright E2E 测试基线 | `admission-frontend/tests/e2e/ai-chat.spec.ts` |

---

## 二、联调发现的契约不一致

### 1. SSE 事件解析器（前端）

**问题：** 前端旧的 SSE 解析器只读 `data:` 行，期望 payload 自带 `{"type":"text_delta","content":"..."}`。
后端实际产出的是标准 SSE 双字段：

```
event: text_delta
data: {"delta":"内容片段"}
```

旧实现里 `event.type === 'text_delta'` 永远为 false，UI 不会展示任何流式文本。

**修复：** 前端解析器改为按"消息块"（`\n\n` 分隔）解析，同时收集 `event:` 与 `data:`，并在产出 `SSEEvent` 时：

- `type` 取自 `event:` 行
- `content` 字段做规范化：优先取 payload 里的 `delta`，回退到 `content`，保持上层 UI 代码不变

后端协议不需要任何改动。

### 2. `Conversation` 类型字段

**问题：** 前端类型里包含 `status: string`，但后端 `Conversation` 模型没有该字段（见 `internal/conversation/model.go`）。

**修复：** 前端删除 `status`，新增可选 `model_name?: string` 与后端一致。

### 3. 列表响应包装

**问题：** 前端 `conversationApi.list()` 旧类型写成 `Envelope<Conversation[]>`，对页面调用 `.map()` 时直接抛 `TypeError: conversations.map is not a function`，页面白屏。

后端实际返回的是分页包装：

```json
{
  "code": 0,
  "data": { "items": [...], "total": 1, "page": 1, "per_page": 20 }
}
```

**修复：** 引入 `PaginatedList<T>` 类型并把页面的取值改为 `res.data?.data?.items || []`。

---

## 三、AI 对话页 UI 调整

参考产品方提供的对照截图（4 卡片 + 输入框紧贴布局），把欢迎页底部分散的 chip 标签升级为 4 张并排卡片：

| 卡片 | 含义 |
|------|------|
| 院校录取 | 查看 985 院校在黑龙江的录取数据 |
| 地区偏好 | 排除不想去的城市后重新筛选 |
| 分数定位 | 根据分数和科类给出冲稳保 |
| 专业方向 | 根据兴趣推荐适合专业 |

布局要点：

- 4 列等宽 grid，紧贴输入框上方（不再随消息列表滚动）
- 每张卡片包含彩色图标徽章 + 标题 + 一行描述
- hover 时边框上色 + 浅投影，点击填入对应问句
- 一旦对话产生消息（`messages.length > 0`），整组卡片自动隐藏，让出空间给对话流

---

## 四、E2E 测试基线

`admission-frontend/tests/e2e/ai-chat.spec.ts` 覆盖以下场景：

- 学生账号登录后跳转 dashboard
- 进入 `/admission/ai`，验证侧栏 + Chat 区域可见
- 点击"新建对话"，URL 含 `?id=...`，侧栏出现新条目
- 输入问句并发送，验证不会白屏（OPENAI_API_KEY 未配置时显示错误气泡而非崩溃）

CI 集成留待后续接入（需准备测试种子数据 + headless 模式）。

---

## 五、本地联调指引

```bash
# 1) 启动依赖
cd admission-api
docker compose up -d                       # Postgres + Redis
go run ./cmd/api/main.go -migrate up       # 011 迁移

# 2) 启动后端（可选 OPENAI_API_KEY）
OPENAI_API_KEY=sk-... go run ./cmd/api/main.go
# 不配置 KEY 时，/api/v1/conversations 仍可用，/ai-chat 等流式端点会返回 404

# 3) 启动前端
cd ../admission-frontend
npm install
npm run dev                                # http://localhost:5173

# 4) 测试账号（开发库）
# 邮箱：student@test.com
# 密码：Test1234
# user_type=student（仅 student 可访问 /ai-chat）
```

---

## 六、仍需后续完成的事项

- 在 `OPENAI_API_KEY` 未配置时，前端友好提示"AI 服务暂未开放，请稍后再试"，而非透传 `HTTP 404: 404 page not found`
- HTTP server `WriteTimeout = 15s` 对长流仍是隐患，建议为 `/ai-chat` 单独放开或改用 handler 内 ctx 超时
- 加 OpenTelemetry / Slog 字段记录 LLM 调用耗时，便于排查慢请求
- 把 E2E 测试纳入 CI（含数据种子和清理）
