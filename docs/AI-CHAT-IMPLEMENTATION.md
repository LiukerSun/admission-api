# AI 对话子系统实现总结

> 关联 Issue：[#21 AI Chat Enhancement](https://github.com/LiukerSun/admission-api/issues/21)
> 实现日期：2026-05-11
> 分支：`API/REDESIGN`

---

## 一、交付范围

本次实现覆盖 PRD 所定义的 **Phase 0 + A + B + C + D** 全部阶段，一次性交付完整 AI 对话子系统后端。

| Phase | 名称 | 内容 |
|-------|------|------|
| 0 | 基础设施 | 数据库迁移、`conversation` 包、配置扩展、限流改造 |
| A | 对话管理 CRUD | 创建 / 列表 / 详情 / 删除 / 重命名对话 |
| B | AI 流式对话 | SSE 真流式输出、Tool-Use 循环、持久化 |
| C | 高级操作 | Rollback（消息截断）、Regenerate（重新生成）、Suggestions（推荐问题） |
| D | 工具集成 | render_chart、render_card、QueryAdmissionPlan、QueryEmployment |

---

## 二、新增文件清单

### 数据库迁移

| 文件 | 说明 |
|------|------|
| `migration/011_ai_conversations.up.sql` | 创建 `ai_conversations` 和 `ai_conversation_messages` 表及索引 |
| `migration/011_ai_conversations.down.sql` | 回滚迁移 |

### `internal/conversation/` 包（对话管理域）

| 文件 | 说明 |
|------|------|
| `model.go` | `Conversation`、`Message`、`CreateMessageInput` struct；角色常量 |
| `errors.go` | `ErrNotFound`、`ErrInvalidArgument` sentinel error |
| `store.go` | PostgreSQL 数据访问层，含 `DeleteMessagesFrom`（CTE 稳定截断） |
| `service.go` | 业务逻辑层，`GetForUser` / `DeleteForUser` 所有权校验 |
| `handler.go` | Gin HTTP handler：Create / List / Get / Delete / Rename |
| `validate.go` | `ValidateMessageContent`：空值 + 8000 字符上限（rune 计数，多字节公平） |
| `service_test.go` | 跨用户隔离、所有权强制、空标题拒绝 |
| `validate_test.go` | TDD：8 个边界用例（空/空白/单字符/多字节/边界值） |

### `internal/ai/` 包（AI 核心域）

| 文件 | 说明 |
|------|------|
| `llm.go` | `LLMProxy` 接口；`StreamChunk` 类型；`FunctionDef` 结构（vendor 无关） |
| `openai_client.go` | OpenAI / 兼容 API 客户端；流式 delta 累积；工具调用 delta 合并 |
| `agent.go` | `AgentCallbacks`、`AgentResult`、`RunStream`；Tool-Use 循环（最多 6 轮） |
| `tools.go` | `Tool` 接口；`ToolRegistry`；`CallContext`；`Invoke` panic-recover |
| `tool_render_chart.go` | 后端构造 ECharts option 白名单（防 XSS），200 数据点 / 10 系列上限 |
| `tool_render_card.go` | 信息卡片渲染；href 白名单（仅 https + 允许 host）；HTML 转义 |
| `tool_admission.go` | `QueryAdmissionPlanTool`、`QueryEmploymentTool`（调用 analysis 服务） |
| `sse.go` | SSE 写入器；事件类型：`text_delta` / `tool_call_start` / `tool_call_end` / `widget` / `done` / `error` / `warning` |
| `handler.go` | `Chat` / `Regenerate` / `Rollback` / `Suggestions` HTTP handler |
| `ids.go` | `newWidgetID()`（crypto/rand，无额外依赖） |
| `widget.go` | `Widget` struct（前端渲染描述符） |
| `agent_test.go` | 纯文本流、工具循环（分片 args）、maxTurns 上限 |
| `tools_test.go` | render_chart inline/tool_result/错误数据源；render_card XSS 防护；suggestions 容错解析 |

### 平台层改动

| 文件 | 改动 |
|------|------|
| `internal/platform/middleware/ratelimit.go` | 新增 `UserRateLimitMiddleware`（user-based，fallback IP）；`RateLimitWithKey` 通用函数 |
| `internal/platform/middleware/rbac.go` | 新增 `RequireUserType(allowed ...string)` |
| `internal/platform/middleware/ratelimit_user_test.go` | 同用户共享桶、不同用户独立桶、匿名 IP fallback 测试 |
| `internal/platform/config/config.go` | 新增：`OpenAIAPIKey`、`OpenAIBaseURL`、`OpenAIModel`、`OpenAITimeoutSeconds`、`AICardLinkHosts`、`AIChatRateLimitPerMin` |
| `cmd/api/main.go` | 初始化 conversation / AI handler；注册路由；`splitCSV` 辅助函数 |

---

## 三、API 端点一览

### 对话管理（所有已认证用户）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/conversations` | 创建对话 |
| `GET` | `/api/v1/conversations` | 分页列表 |
| `GET` | `/api/v1/conversations/:id` | 获取对话详情（含消息） |
| `DELETE` | `/api/v1/conversations/:id` | 删除对话 |
| `PUT` | `/api/v1/conversations/:id/title` | 重命名 |

### AI 功能（仅 `user_type=student` 考生，30次/分/用户 限流）

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/conversations/:id/ai-chat` | 发送消息，SSE 流式返回 |
| `POST` | `/api/v1/conversations/:id/regenerate` | 重新生成最后一条 AI 回复 |
| `POST` | `/api/v1/conversations/:id/rollback` | 截断到指定消息，返回删除数量和最新消息 ID |
| `GET` | `/api/v1/conversations/:id/suggestions` | 获取推荐问题（Redis 缓存 1h） |

---

## 四、SSE 事件格式

```
event: text_delta
data: {"delta":"内容片段"}

event: tool_call_start
data: {"call_id":"xxx","name":"render_chart","index":0}

event: tool_call_end
data: {"call_id":"xxx","name":"render_chart","args":{"chart_type":"bar",...}}

event: widget
data: {"id":"wid_xxx","type":"chart","spec":{...}}

event: done
data: {"conversation_id":1,"message_id":42}

event: error
data: {"code":"rate_limit","message":"请求过于频繁，请稍后重试"}

event: warning
data: {"message":"已达到最大工具调用轮数，对话被截断"}
```

---

## 五、安全设计要点

1. **render_chart XSS 防护**：LLM 不直接传入 ECharts option，后端白名单字段重新构造（`title/tooltip/legend/grid/xAxis/yAxis/series`）
2. **render_card href 白名单**：scheme 必须 `https`，host 必须在配置的 `AICardLinkHosts` 列表中；相对路径（`/` 开头）允许；所有文本字段 `html.EscapeString`
3. **所有权隔离**：`GetForUser` / `DeleteForUser` 跨用户访问统一返回 `ErrNotFound`，防止 ID 枚举
4. **限流分层**：全局 IP 限流 + 用户级限流，无认证时 fallback 到 IP

---

## 六、新增配置项

在 `.env` 中配置以下变量：

```env
OPENAI_API_KEY=sk-...          # 必填，留空则禁用 AI 端点
OPENAI_BASE_URL=               # 可选，留空使用 api.openai.com（兼容 DeepSeek/Tongyi 等）
OPENAI_MODEL=gpt-4o            # 默认模型
OPENAI_TIMEOUT_SECONDS=120     # 单次请求超时（秒）
AI_CARD_LINK_HOSTS=example.com,yoursite.com  # render_card href 白名单，逗号分隔
AI_CHAT_RATE_LIMIT_PER_MIN=30  # 每用户每分钟请求上限
```

---

## 七、测试覆盖

| 测试文件 | 覆盖场景 |
|----------|---------|
| `conversation/validate_test.go` | 空内容、纯空白、单字符、中文多字节、边界值（8000 rune）、超限 |
| `conversation/service_test.go` | 跨用户访问返回 404、非所有者删除失败、空标题拒绝 |
| `ai/agent_test.go` | 纯文本流、工具调用循环（分片 args 累积）、maxTurns 上限截断 |
| `ai/tools_test.go` | render_chart：inline/从 tool_result 解析/错误数据源；render_card：XSS 防护/href 白名单；suggestions：容错 JSON 解析 |
| `platform/middleware/ratelimit_user_test.go` | 同用户共享令牌桶、不同用户桶隔离、匿名 fallback IP |

运行测试：

```bash
go test -race ./internal/ai/... ./internal/conversation/... ./internal/platform/middleware/...
```

---

## 八、下一步改进建议

以下功能未在本次 issue 范围内，但建议作为后续迭代考虑：

### 高优先级

1. **WriteTimeout 延长**  
   当前 HTTP server `WriteTimeout = 15s`，会截断长时 SSE 流。建议对 `/ai-chat` 路由单独设置更长超时（如 3~5 分钟），或使用 Gin 的 `context.WithTimeout` 方案在 handler 内部控制，而非依赖 server 全局超时。

2. **流式中断恢复**  
   客户端断连后当前直接丢弃剩余 token。可在数据库中记录 `streaming` 状态，断连后继续后台生成并将结果持久化，客户端重连时返回已生成内容。

3. **对话标题自动生成**  
   创建对话时 title 由调用方传入，建议在首条 AI 回复后用 LLM 自动生成简短标题并回写，提升 UX。

### 中优先级

4. **Token 用量统计**  
   在 `ai_conversation_messages` 表新增 `prompt_tokens`、`completion_tokens` 字段，每次调用后从 OpenAI response 取 `usage` 回写，支持成本追踪和用量配额。

5. **多模型支持**  
   当前模型在创建对话时固定，建议允许每条消息指定模型，或为对话存储 `model_name` 并在 AI 请求时使用对话自身的 model 而非全局默认。

6. **会话记忆压缩**  
   当历史消息过长时（接近模型上下文窗口），目前只截取最近 N 条。可引入"滚动摘要"策略：先用 LLM 压缩旧消息，再拼接最新历史，避免上下文丢失。

7. **Suggestions 缓存失效策略**  
   当前 suggestions 缓存 key 含 `lastMessageID`，新消息产生时自然失效。可进一步在 `Rollback` 后主动删除相关 cache key，避免旧建议残留。

### 低优先级

8. **集成测试（AI Handler）**  
   当前 AI handler 测试以单元测试（mock LLM）为主，建议补充端到端集成测试：用 miniredis + mock OpenAI server 覆盖 Chat / Regenerate / Rollback / Suggestions 完整流程。

9. **OpenTelemetry 追踪**  
   为 `agent.RunStream` 和 `Invoke` 注入 span，方便排查 LLM 延迟、工具调用耗时等问题。

10. **工具扩展**  
    可按业务需求继续实现 Tool 接口添加更多工具，如：院校 / 专业对比、分数线预测、志愿推荐等；已有 `ToolRegistry` 支持热插拔注册。
