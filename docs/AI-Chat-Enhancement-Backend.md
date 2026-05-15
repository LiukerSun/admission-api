# AI 对话增强 · 后端改造总结

> 关联 Issue：[#21 - [PRD] AI 对话增强 - 后端改造](https://github.com/LiukerSun/admission-api/issues/21)
> 前端 PRD：[PRD-AI-Chat-Enhancement.md](../PRD-AI-Chat-Enhancement.md)

## 1. 背景

当前 `admission-api` 的 AI 对话存在三个核心问题：

1. **SSE 是伪流式**：`internal/ai/handler.go` 在 `agent.Run` 同步返回完整结果后，才把整段文本按 10 rune 切片以 `text_delta` 事件分次推送。首字延迟 = 整轮 LLM 生成时间（多轮工具调用场景下经常 10s+），用户感知不到"AI 正在思考/输出"的中间反馈。
2. **没有结构化展示通道**：`AgentResult` 已有 `Filter` / `Data` 字段但 SSE 协议没暴露，前端无法渲染图表/卡片；如果让 LLM 直接吐 echarts option JSON，会引入 XSS + 幻觉双风险。
3. **缺三个关键接口**：rollback（编辑历史强依赖）、regenerate、suggestions（底部推荐回复）。

## 2. 改造范围

| 维度 | 决策 |
|---|---|
| LLM Provider 真流式 | 仅 **OpenAI**（用户确认）。Anthropic 路径保留 ChatCompletion 接口，启动时打 warning 日志降级为伪流式 |
| 受影响端点 | stateless `/ai/chat` 与 conversation-scoped `/conversations/:id/ai-chat` **共用同一条流式管线** |
| 阶段 | A→B→C→D **一并交付** |
| 限流 | 4 个 AI 相关端点共用 `ratelimit:user:{id}` 桶 30/min |

**不在本次范围**：前端渲染、Markdown 安全策略、模型/Prompt 调优、Anthropic 真流式、历史 `tool_results → widgets` 数据回填。

## 3. 总体架构

```
HTTP Handler ──── SSE writer ──────┐
       │                           │
       ▼                           │
runAgentOnHistory(ctx, history) ◄── AgentCallbacks
       │
       ▼
Agent.RunStream(ctx, msgs, cb)
       │
       ▼
LLMProxy.ChatCompletionStream(ctx, msgs, tools) → <-chan StreamChunk
```

**三个关键设计**：

1. **流式管线统一收敛到 `runAgentOnHistory`** —— `Chat`、`ChatWithConversation`、`Regenerate` 全部调用它，杜绝三处流式代码漂移。
2. **`AgentCallbacks` 是 SSE 事件的唯一来源** —— agent 不知道 HTTP，handler 负责把回调翻译成 SSE 事件，方便单测。
3. **`render_chart` / `render_card` 在后端构造受控 widget payload** —— LLM 永远拿不到也产不出 echarts option JSON，规避 XSS 与幻觉。

## 4. Phase A · 真流式 + SSE 协议升级

### LLM 客户端层

- 在 `LLMProxy` 接口新增 `ChatCompletionStream(ctx, msgs, tools) (<-chan StreamChunk, error)`，保留原有 `ChatCompletion` 供非流式场景（如 suggestions）调用。
- `StreamChunk` 类型抽象掉 provider 差异：`text_delta` / `tool_call_done` / `done` / `error` 四种 chunk。
- **OpenAI 真流式实现要点**（`openai_client.go`）：
  - 请求体加 `"stream": true` + `"stream_options": {"include_usage": false}`
  - `bufio.Scanner` 按 `\n\n` 切 SSE 帧，每帧解析 `data: {...}`，遇 `data: [DONE]` 终止
  - **工具调用 arguments 跨帧累积**：OpenAI 把同一 `index` 的 `arguments` 分多个增量帧发送，客户端用 `pending map[int]*toolCallAcc` 持有部分状态，`finish_reason=="tool_calls"` 或 `[DONE]` 时统一 flush 出 `tool_call_done` chunk
  - HTTP body 在 goroutine 退出前关闭，配合 `ctx.Done()` 实现取消时清理上游连接
  - `bufio.Scanner` 缓冲区从默认 64KB 提升到 1MB，避免长 arguments payload 触发 `bufio.ErrTooLong`
- **Anthropic 降级实现**：把 `ChatCompletion` 的一次性返回包装成 channel（text → tool_calls → done）。本期不做真流式以控制范围。

### Agent 层

- 新增 `AgentCallbacks` 结构（`OnTextDelta` / `OnToolCallStart` / `OnToolCallEnd` / `OnWidget`），所有回调可选 nil（自动 noop）。
- `Agent.RunStream(ctx, msgs, cb) → *AgentResult` 取代旧 `Agent.Run` 的位置；旧 `Run` 内化为 `RunStream(ctx, msgs, AgentCallbacks{})` 的薄包装，**避免两份实现漂移**。
- 工具循环：在迭代中收到 `text_delta` 立即转 `OnTextDelta`；收到 `tool_call_done` 后调 `OnToolCallStart` → 执行工具 → 调 `OnToolCallEnd`（widget 工具同步触发 `OnWidget`）→ 工具结果回灌到下一轮 stream 调用。
- **`MaxWidgetsPerRun = 5` 硬上限**：在 agent 层强制，覆盖所有 widget 工具，防止 LLM 一轮内 spam chart。
- **取消传播修复**：`runOneIteration` 返回后必须检查 `ctx.Err()` —— 否则 stream 被 ctx 取消时 channel 提前关闭，agent 会把空 text 误判为"模型决定不调工具，正常结束"，吞掉取消信号（由测试 `TestAgentStopsWhenContextIsCancelled` 暴露）。

### Handler 层

- 抽出 `runAgentOnHistory(ctx, sw, history) → (*AgentResult, error)`：内部构造 `AgentCallbacks`，把每个回调翻译成对应 SSE 事件。
- 抽出 `streamConversationTurn(c, convID)`：`ChatWithConversation` 与 `Regenerate` 共用持久化 + SSE 路径。
- `newStreamWriter(c)` 封装 SSE header 初始化、`http.Flusher`、自动 `extendWriteDeadline`。
- **废弃**：旧的 10-rune chunking 循环；`step_start` / `step_finish` 的 `thinking` 用法（保留事件名向后兼容，新代码不主动写）。

### SSE 协议升级

| 事件 | 状态 | Payload |
|---|---|---|
| `text_delta` | 保留 | `{ content: string }` |
| `done` | 保留 | `{ data: AgentResult }` |
| `error` | 保留 | `{ content: string }` |
| `warning` | 保留 | `{ content: string }` |
| **`widget`** | 新增 | `{ id, kind: "chart" \| "card", payload: object }` |
| **`tool_call_start`** | 新增 | `{ call_id, tool_name }` |
| **`tool_call_end`** | 新增 | `{ call_id, success: bool, error?: string }` |
| `step_start` / `step_finish` | 已弃用语义 | 字段保留，新代码不写 |

### Anthropic 启动 warning

`cmd/api/main.go` 在 `LLMProvider=anthropic` 时打印：

```
anthropic provider falls back to non-streaming completion in this version
```

## 5. Phase B · widgets 列 + render 工具

### 数据库迁移

`conversation_messages.widgets` 列已并入 [`migration/001_init_schema.up.sql`](../migration/001_init_schema.up.sql)：

```sql
widgets JSONB NOT NULL DEFAULT '[]'::jsonb
```

- 旧消息默认 `'[]'`，前端兼容；**不做数据回填** — 旧 `tool_results` 不能自动转 widget，前端依赖 `widgets` 字段判断是否渲染。

### Message 模型 / Store / Service

- `conversation.Message` 增加 `Widgets []byte`（JSON-marshaled `[]Widget`）。
- `Store.AddMessage` 签名追加 `widgets []byte` 参数；INSERT 字段列表与 `ListMessages` SELECT 同步增加 widgets 列；Service 透传。
- `Handler.streamConversationTurn` 落库 assistant 消息时把 `AgentResult.Widgets` JSON-marshal 后传入。

### Widget 类型与安全 ID

`internal/ai/widget.go`：

```go
type Widget struct {
    ID      string         `json:"id"`      // crypto/rand 12 字节 → "wgt_<hex>"
    Kind    string         `json:"kind"`    // "chart" | "card"
    Payload map[string]any `json:"payload"` // 受控字段
}
```

ID 用于前端 dedupe 跨 SSE replay + 历史读，crypto/rand 仅为不可预测性（避免冲突），不承担安全语义。

### `render_chart` 工具

LLM 接收参数（只允许这些字段，没有任何途径传 raw echarts option）：

```json
{
  "chart_type": "bar | line | pie",
  "title": "string",
  "data_source": "inline | tool_result:<call_id>",
  "inline_data": [{ "x": ..., "y": ... }],
  "x_field": "string",
  "y_fields": ["string"]
}
```

**执行流程**：
1. 校验 `chart_type` 在 `{bar, line, pie}` 白名单
2. `data_source` 解析：`inline` 用 `inline_data`；`tool_result:<id>` 通过 `ToolExecContext.ResolveResult` 在同轮 toolResults 里查（同时兼容 `search_universities` 返回的 `{"top": [...]}` 形态）
3. **后端构造受控 echarts option** —— 只输出白名单 key：`title` / `tooltip` / `grid` / `xAxis` / `yAxis` / `legend` / `series`；**禁止 formatter 函数字符串**；series.data 强制 `numericValue()` 转 float64，object 自动变 0
4. 触发 `OnWidget(widget)` 推到 SSE 流 + 累积进 `AgentResult.Widgets`
5. 返回给 LLM 的 ToolResult content 是短文本（`"Chart rendered (N points)"`），不让 LLM 看到 widget payload

### `render_card` 工具

```json
{
  "title": "string",
  "description": "string?",
  "metrics": [{ "label": "...", "value": "...", "trend": "up|down|flat" }],
  "link": { "text": "...", "href": "string" }
}
```

- `link.href` 校验通过 `isAllowedCardLink(href, whitelist)`：
  - 相对路径 `/foo` 允许（同站点）
  - 协议相对 `//evil.com` 拒绝
  - `https://<whitelisted-host>/...` 允许（host 大小写不敏感）
  - 任何 `http://` / `javascript:` / `data:` / `file:` 拒绝
- 白名单通过环境变量 `CARD_LINK_WHITELIST="host1,host2"` 配置，注入 `ToolExecutor.SetCardLinkWhitelist(...)`
- 未知 `trend` 值（不在 `{up, down, flat}`）自动降级为 `""`，不抛错

### `ToolExecutor` 改造

- `Execute(ctx, call, execCtx ToolExecContext) → *ToolResult` 改签名，新增 per-run 能力包：
  - `EmitWidget func(Widget)` —— 同步触发流式 widget 事件
  - `ResolveResult func(callID string) (string, bool)` —— render_chart 引用同轮 prior tool result
  - `CardLinkWhitelist []string` —— per-call 覆盖
- 这样设计让 executor 保持**无状态**，可以安全跨并发 agent 运行共享。

## 6. Phase C · rollback + regenerate 接口

### `POST /api/v1/conversations/:id/rollback`

- **鉴权**：非本人会话 404（不是 403，避免泄露存在性）—— 复用 `canAccessConversation`
- **请求体**：`{ "message_id": int64, "inclusive": bool? }`，`inclusive` 缺省 `true`
- **响应**：`{ "deleted_count": int, "latest_message_id": int64 | null }`
- **SQL 关键点**：

```sql
DELETE FROM conversation_messages
WHERE conversation_id = $1
  AND (created_at, id) >= (
    SELECT created_at, id FROM conversation_messages
    WHERE id = $2 AND conversation_id = $1
  );
```

按 `(created_at, id)` **元组比较**而非单 id —— 同秒插入多条消息时，id 序与 created_at 序可能不一致，元组比较是全序的。`inclusive=false` 时把 `>=` 改成 `>`。

- 删除后若 `RowsAffected()=0`，会再 probe 一次锚点行是否存在：不存在则返回 `ErrConversationNotFound`（→ 404）；存在则返回 `(0, nil)`（合法的"没有后续消息可删"）。

### `POST /api/v1/conversations/:id/regenerate`

- 加载最后一条消息：
  - `role=assistant` → 用其 message_id 调 `Rollback(inclusive=true)`，连同 widgets 一起删
  - `role=user` → 不删（"再问一次"语义）
- 调 `streamConversationTurn(c, convID)` 复用流式管线，SSE 输出 + 重新持久化 assistant 消息

## 7. Phase D · suggestions 接口

### `GET /api/v1/conversations/:id/suggestions`

- **鉴权**：同上（非本人 404）
- **响应**：`{ "suggestions": string[] }`，长度 2-4，每条 ≤ 60 字符
- **流程**：
  1. 加载最近 N=10 条历史（跳过 tool/system 消息）
  2. 查 Redis 缓存 `conv_suggest:{convID}:{lastMessageID}`，命中直接返回
  3. 未命中 → 调 `LLMProxy.ChatCompletion`（**非流式**，无需 stream）配专用 system prompt，强制输出 JSON 数组
  4. 解析失败 / 元素数 < 2 → 返回 `{"suggestions": []}` **不抛 500**
  5. 写入 Redis 缓存，TTL=1h
- **降级容错**：`parseSuggestions` 兼容 ```` ```json ... ``` ```` 围栏、prose 中嵌入的 `[...]`、纯 garbage 一律返回空数组
- **LLM 调用 timeout**：8s（独立于 SSE 的 10min），避免慢上游拖垮 rate limit refill 窗口

### 独立 Handler 设计

`SuggestionsHandler` 与主 AI `Handler` 分离 —— suggestions 只依赖 `LLMProxy` + `conversation.Service` + `redis.Client`，不需要 Agent + 工具循环；主流式 handler 不需要 Redis。这样两个 handler 各自最小依赖。

## 8. 路由 + 限流

`cmd/api/main.go` 新增 3 个端点，全部接入同一 `RateLimitByUser` 桶（30/min/user）：

```go
authorized.POST("/conversations/:id/rollback",    middleware.RateLimitByUser(rdb, 30, 1*time.Minute), conversationHandler.Rollback)
authorized.POST("/conversations/:id/regenerate",  middleware.RateLimitByUser(rdb, 30, 1*time.Minute), aiHandler.Regenerate)
authorized.GET ("/conversations/:id/suggestions", middleware.RateLimitByUser(rdb, 30, 1*time.Minute), aiSuggestionsHandler.Suggestions)
```

共桶的原因：让"编辑组合"（rollback + 新 user 消息 + ai-chat 重跑）不能单独绕过 ai-chat 的限流。

## 9. 测试覆盖（TDD 补强）

新增 3 个测试文件，41 个 sub-test 全 pass（含 `-race`）：

| 文件 | 测试目标 | sub-test |
|---|---|---|
| `internal/ai/suggestions_test.go` | `parseSuggestions` 鲁棒性（fence 包裹、prose 嵌入、garbage、min/max count、长度截断） | 11 |
| `internal/ai/tools_widget_test.go` | `isAllowedCardLink` 白名单 + `executeRenderChart`/`Card` widget shape | 13 + 11 |
| `internal/ai/openai_stream_test.go` | `streamOpenAIBody` 工具调用 arguments 跨帧累积 / 并行 index / `[DONE]` 与 `finish_reason` 双终止 / 坏帧不中断 / ctx 取消关 body | 6 |

**测试发现的真实回归**：`TestAgentStopsWhenContextIsCancelled` 暴露了 `Agent.RunStream` 在 stream 被 ctx 取消时会把空 text 当作"模型决定不调工具，正常结束"返回 — 取消信号被吞。修复方案：`runOneIteration` 返回后增加 `ctx.Err()` 检查，统一翻译为 `agent context finished: %w` 错误。

**纯函数路径覆盖率**：

| 函数 | 覆盖率 |
|---|---|
| `parseSuggestions` | 94.1% |
| `sanitizeSuggestions` | 100% |
| `isAllowedCardLink` | 100% |
| `executeRenderChart` | 89.5% |
| `executeRenderCard` | 95.5% |
| `buildEchartsOption` | 100% |
| `streamOpenAIBody` | 86.8% |

整包覆盖 39.9%（受未测的 HTTP/SSE handler 拖累）。

## 10. 风险与对策

| 风险 | 对策 |
|---|---|
| OpenAI 流式工具调用 arguments 跨帧累积 bug | 6 个针对性单测覆盖（并行 index、缺 finish_reason、坏帧） |
| 历史 `tool_results` JSONB 不能自动转 widget | 明确"旧消息不展示 widget"，前端依赖 `widgets` 字段判断渲染 |
| suggestions 让单轮总 LLM 调用翻倍、成本压力 | Redis 缓存 key 含 `lastMessageID`，同一对话末尾 message_id 重复请求必命中；上线前压测命中率 |
| LLM 滥用 render_chart 导致 widget 泛滥 | `MaxWidgetsPerRun=5` 硬上限 + system prompt 约束"仅在用户明确要图/数据明显适合可视化时才用" |
| widget emitter 让 executor 变有状态 | 通过 `Execute(ctx, call, execCtx ToolExecContext)` 显式参数传递，executor 自身保持无状态 |
| SSE 客户端断连未清理上游 LLM HTTP 请求 | `ChatCompletionStream` 把 ctx 传给上游 HTTP，handler 用 `c.Request.Context()` 作 ctx 根；body 在 goroutine defer 中关闭 |
| ctx 取消时 stream 提前关闭导致取消信号被吞 | `runOneIteration` 返回后强制检查 `ctx.Err()`（由测试发现并修复） |

## 11. 验证清单

### 端到端

```bash
# 1. 跑迁移
go run ./cmd/api -migrate up

# 2. 启动 API
LLM_PROVIDER=openai LLM_API_KEY=sk-xxx go run ./cmd/api

# 3. 真流式延迟验证（首字 < 1s）
curl -N -H "Authorization: Bearer $JWT" \
  -d '{"message":"推荐几所黑龙江的985院校"}' \
  http://localhost:8080/api/v1/conversations/1/ai-chat

# 4. widget 输出验证（提问触发 render_chart）
curl -N -H "Authorization: Bearer $JWT" \
  -d '{"message":"画一张分数线对比图"}' \
  http://localhost:8080/api/v1/conversations/1/ai-chat

# 5. rollback
curl -X POST -H "Authorization: Bearer $JWT" \
  -d '{"message_id": 123, "inclusive": true}' \
  http://localhost:8080/api/v1/conversations/1/rollback

# 6. regenerate
curl -N -X POST -H "Authorization: Bearer $JWT" \
  http://localhost:8080/api/v1/conversations/1/regenerate

# 7. suggestions
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8080/api/v1/conversations/1/suggestions

# 8. Anthropic 降级 warning
LLM_PROVIDER=anthropic LLM_API_KEY=... go run ./cmd/api
# 应在启动日志看到: "anthropic provider falls back to non-streaming completion in this version"
```

### 单元 / 集成

```bash
go build ./...                                 # ✅
go vet ./...                                   # ✅
go test ./...                                  # ✅
go test -race -count=1 ./...                   # ✅ 无 race
go test -cover ./internal/ai/...               # 整包 39.9%，新增纯函数 90%+
```

## 12. 文件清单

**新增**（8 个）：
- `internal/ai/widget.go` — `Widget` 类型 + 安全 ID 生成
- `internal/ai/suggestions.go` — `SuggestionsHandler` + 缓存 + 解析
- `internal/ai/suggestions_test.go` — `parseSuggestions` 11 cases
- `internal/ai/tools_widget_test.go` — `isAllowedCardLink` / `render_chart` / `render_card` 共 24 cases
- `internal/ai/openai_stream_test.go` — `streamOpenAIBody` 6 cases
- `migration/001_init_schema.up.sql`（`widgets` 列已并入 baseline）
- `docs/AI-Chat-Enhancement-Backend.md`（本文件）

**修改**（15 个）：
- `internal/ai/llm.go` — 新增 `StreamChunk` + `ChatCompletionStream` 接口
- `internal/ai/openai_client.go` — 真流式实现（SSE 帧解析 + arguments 累积）
- `internal/ai/anthropic_client.go` — 降级实现
- `internal/ai/agent.go` — `RunStream` + `AgentCallbacks` + ctx 取消修复 + `AgentResult.Widgets`
- `internal/ai/handler.go` — `runAgentOnHistory` / `streamConversationTurn` / `Regenerate` / SSE 协议升级
- `internal/ai/tools.go` — `ToolExecContext` + `render_chart` / `render_card` + 白名单
- `internal/ai/agent_test.go` — stub 适配 `ChatCompletionStream` + `Execute` 新签名
- `internal/ai/handler_test.go` — stub 适配 `AddMessage(widgets)` + `ChatCompletionStream`
- `internal/conversation/model.go` — `Message.Widgets`
- `internal/conversation/store.go` — `AddMessage(widgets)` + `ListMessages` 加列 + `Rollback`
- `internal/conversation/service.go` — 签名透传 + `Rollback`
- `internal/conversation/handler.go` — `Rollback` handler
- `internal/conversation/handler_test.go` — stub 适配新签名
- `internal/platform/config/config.go` — `CardLinkWhitelist` 配置项
- `cmd/api/main.go` — Anthropic warning + `SuggestionsHandler` 实例化 + 3 个新路由

## 13. 遗留 TODO

- **`store.Rollback` 同秒 `(created_at, id)` 稳定性集成测试** —— 需要真实 PG 或 Testcontainers
- **HTTP+SSE handler 集成测试** —— `Chat` / `ChatWithConversation` / `Regenerate` / `Suggestions` 走 `httptest` 完整链路
- **`OpenAIClient.ChatCompletion` 非流式路径单测** —— 需要 HTTP server mock
- **Anthropic 真流式** —— 本期 OpenAI 优先，Anthropic 下个迭代再做
