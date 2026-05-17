package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"admission-api/internal/admission"
	"admission-api/internal/conversation"
	"admission-api/internal/volunteerplan"
)

// ToolResult is the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ToolExecContext is the per-run capability bundle handed to tool
// executions. Keeping it as a value (not a field on ToolExecutor) means
// the executor itself stays stateless and can safely be shared across
// concurrent agent runs.
//
//   - EmitWidget pushes a widget into the streaming/persistence pipeline.
//     Tools call it when they produce a structured render unit.
//   - ResolveResult looks up the textual content of a prior tool result
//     from the same run by call_id. render_chart uses it to back a chart
//     with data produced by an earlier search_universities call without
//     duplicating that payload through the LLM.
//   - CardLinkWhitelist restricts hrefs in render_card. An empty slice
//     means no external links are allowed; tools should still permit
//     relative ("/...") hrefs to the same site.
type ToolExecContext struct {
	UserID            int64
	ConversationID    int64
	EmitWidget        func(Widget)
	ResolveResult     func(callID string) (string, bool)
	CardLinkWhitelist []string
}

// ToolExecutor executes tool calls from the LLM.
type ToolExecutor struct {
	admissionLineStore  admission.AdmissionLineStore
	aggregateStore      admission.AggregateStore
	recommendations     admission.RecommendationService
	draftStore          volunteerplan.DraftStore
	conversationService conversation.Service
	cardLinkWhitelist   []string

	// cityOptionStore 提供"按省份分组的城市枚举"——给 render_form 的
	// preferred_cities / excluded_cities 字段动态填充选项。可空：
	// 测试或脱机环境下传 nil 时，render_form 退化为静态硬编码列表。
	cityOptionStore admission.CityOptionStore
	// cityOptionsCache 缓存一次 LoadCityOptions 的结果。城市枚举在
	// 一年内基本不会变（除非新增院校 profile），所以进程级缓存合适——
	// 避免每次表单调 DB。失败时 cached 仍为 nil，下次调用会重试。
	cityOptionsCache    []formFieldOption
	cityOptionsCacheErr error
	cityOptionsOnce     sync.Once
	cityOptionsMu       sync.Mutex
}

// NewToolExecutor creates a new tool executor.
//
// conversationService is used by generate_volunteer_plan_draft to refuse
// writes against an archived conversation; it may be nil in
// unit tests that do not exercise that path (the tool falls back to the
// previous behaviour of skipping the status check when nil).
//
// cityOptionStore 可为 nil；nil 时 render_form 的城市字段用 form_fields.go
// 中的静态硬编码列表兜底，保证脱 DB 环境（单测）仍能跑。
func NewToolExecutor(
	admissionLineStore admission.AdmissionLineStore,
	aggregateStore admission.AggregateStore,
	recommendations admission.RecommendationService,
	draftStore volunteerplan.DraftStore,
	conversationService conversation.Service,
	cityOptionStore admission.CityOptionStore,
) *ToolExecutor {
	return &ToolExecutor{
		admissionLineStore:  admissionLineStore,
		aggregateStore:      aggregateStore,
		recommendations:     recommendations,
		draftStore:          draftStore,
		conversationService: conversationService,
		cityOptionStore:     cityOptionStore,
	}
}

// loadCityOptions 把 DB 拉的 CityOption 转成 formFieldOption（按省份分组）。
// 第一次调用打 DB，之后命中 sync.Once 缓存；如果首次失败会重试一次再放弃，
// 因为这种"枚举数据加载"通常是部署期 DB 还没就绪的临时态。
func (e *ToolExecutor) loadCityOptions(ctx context.Context) ([]formFieldOption, error) {
	if e.cityOptionStore == nil {
		return nil, nil
	}
	e.cityOptionsOnce.Do(func() {
		opts, err := e.cityOptionStore.ListCityOptions(ctx)
		if err != nil {
			e.cityOptionsCacheErr = err
			return
		}
		// DB 已按 (province, univ_count desc) 排好；这里只做 province →
		// formFieldOption 的字段映射。GroupCode 填 region_code，前端整省
		// 勾选时直接拿它当 PreferredProvinces / OnlyProvinces 等字段的值。
		out := make([]formFieldOption, 0, len(opts))
		for _, o := range opts {
			out = append(out, formFieldOption{
				Value:     o.City,
				Label:     o.City,
				Group:     o.ProvinceName,
				GroupCode: o.ProvinceCode,
			})
		}
		// 按 group 稳定排序，让相同省份连续——下游 antd Select 的 OptGroup
		// 渲染会按出现顺序聚合。
		sort.SliceStable(out, func(i, j int) bool { return out[i].Group < out[j].Group })
		e.cityOptionsCache = out
	})
	e.cityOptionsMu.Lock()
	defer e.cityOptionsMu.Unlock()
	// 首次失败时清掉 once 让下一轮可重试。代价是并发首调可能多打一次 DB，
	// 但城市枚举体量小（~300 行）可接受。
	if e.cityOptionsCacheErr != nil {
		err := e.cityOptionsCacheErr
		e.cityOptionsCacheErr = nil
		e.cityOptionsOnce = sync.Once{}
		return nil, err
	}
	return e.cityOptionsCache, nil
}

// SetCardLinkWhitelist configures the host whitelist used by render_card.
// Entries are matched case-insensitively against the URL host.
func (e *ToolExecutor) SetCardLinkWhitelist(hosts []string) {
	cleaned := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			cleaned = append(cleaned, h)
		}
	}
	e.cardLinkWhitelist = cleaned
}

// Execute runs a tool call and returns the result. The ToolExecContext
// carries per-run capabilities (widget emission, prior result lookup);
// pass an empty struct in tests that don't exercise those features.
func (e *ToolExecutor) Execute(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	switch call.Function.Name {
	case "apply_filter":
		return e.executeApplyFilter(ctx, call)
	case "search_universities":
		return e.executeSearchUniversities(ctx, call)
	case "aggregate_data":
		return e.executeAggregateData(ctx, call)
	case "render_chart":
		return e.executeRenderChart(ctx, call, execCtx)
	case "render_card":
		return e.executeRenderCard(ctx, call, execCtx)
	case "render_form":
		return e.executeRenderForm(ctx, call, execCtx)
	case "generate_volunteer_plan_draft":
		return e.executeGenerateVolunteerPlanDraft(ctx, call, execCtx)
	default:
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Unknown tool: %s", call.Function.Name)}, nil
	}
}

// previewItemCap 限制 dry_run 返回给模型的预览条目数量。
// 太多会撑大 prompt context；模型只需要知道「冲/稳/保各一两条样例」即可叙述。
const previewItemCap = 2

func (e *ToolExecutor) executeGenerateVolunteerPlanDraft(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	if execCtx.UserID <= 0 || execCtx.ConversationID <= 0 {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"generate_volunteer_plan_draft requires conversation context"}`}, nil
	}

	// 先抽取 dry_run 标志位；剩余字段反序列化为 RecommendationRequest（未知字段会被忽略）。
	var meta struct {
		DryRun bool `json:"dry_run"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &meta)

	var req admission.RecommendationRequest
	if err := json.Unmarshal([]byte(call.Function.Arguments), &req); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"status":"failed","error":"invalid arguments: %v"}`, err)}, nil
	}

	if req.RegionCode == "" || req.SubjectCategoryCode == "" || req.TotalScore == 0 || req.ProvincialRank == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"missing required fields: region_code, subject_category_code, total_score, provincial_rank"}`}, nil
	}
	if req.RegionCode != "230000" {
		return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"only supports region_code=230000"}`}, nil
	}

	// dry_run：跑一次算法但不落库，把过滤后真实候选池规模 + 三档真实分布
	// + 少量预览条目返回给模型，让它据此判断是否继续追问偏好或进入正式落盘。
	// 注意：dry_run 走 Preview 路径，不做 plan_size 截断、不做同校多样性约束，
	// 所以 pool_size 会随用户偏好的硬过滤真实变化（漏斗效果）。
	if meta.DryRun {
		resp, err := e.recommendations.Preview(ctx, &req)
		if err != nil {
			payload := map[string]any{"status": "preview_failed", "error": err.Error()}
			content, _ := json.Marshal(payload)
			return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
		}
		hard, soft, unused := classifyRequestFields(&req)
		out := map[string]any{
			"status":              "preview",
			"strategy":            resp.Strategy,
			"plan_size":           resp.PlanSize,
			"pool_size":           resp.PoolSize,
			"pool_rush_count":     resp.PoolRushCnt,
			"pool_match_count":    resp.PoolMatchCnt,
			"pool_safe_count":     resp.PoolSafeCnt,
			"rank_window":         resp.RankWindow,
			"notes":               resp.Notes,
			"sample_items":        trimPreviewItems(resp.SampleItems, previewItemCap),
			"active_hard_filters": hard,
			"active_soft_scoring": soft,
			"unused_fields":       unused,
		}
		content, _ := json.Marshal(out)
		return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
	}

	// 已归档对话拒绝落盘：用户已经采纳过这次会话产出的方案（adopt 会把
	// conversation 翻成 archived）；继续生成新草稿会让聊天 UI 出现"已归档
	// 状态下却又冒出新方案卡片"的撕裂态。dry_run 预览不受此约束 —— 让
	// 模型可以在归档对话里继续做问答和预览，只是不能再写盘。
	if e.conversationService != nil {
		conv, err := e.conversationService.GetConversation(ctx, execCtx.ConversationID)
		if err != nil {
			if errors.Is(err, conversation.ErrConversationNotFound) {
				return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"conversation not found"}`}, nil
			}
			return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"status":"failed","error":"load conversation failed: %v"}`, err)}, nil
		}
		if conv.Status != "active" {
			return &ToolResult{ToolCallID: call.ID, Content: `{"status":"failed","error":"conversation is archived, cannot create draft"}`}, nil
		}
	}

	// 查重：一次会话最多保留一个 ready 状态的草稿。若已经存在 ready 且未被采纳的草稿，
	// 直接复用它的 draft_id —— 避免模型在临门一脚阶段重复触发昂贵的 Recommend 调用，
	// 并防止前端出现多张"可保存"卡片导致用户困惑。
	// 若存在 generating 状态的草稿（罕见：上一次落盘还在跑），返回错误信号告知模型先稍候。
	if existing, listErr := e.draftStore.ListByConversation(ctx, execCtx.UserID, execCtx.ConversationID); listErr == nil {
		for _, d := range existing {
			if d == nil {
				continue
			}
			switch d.Status {
			case volunteerplan.DraftStatusGenerating:
				payload := map[string]any{
					"status": "generating",
					"error":  "previous draft is still being generated; ask the user to wait a moment and retry",
				}
				content, _ := json.Marshal(payload)
				return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
			case volunteerplan.DraftStatusReady:
				out := map[string]any{
					"draft_id": d.ID,
					"status":   "ready",
					"reused":   true,
					"note":     "an existing ready draft for this conversation was reused; no new recommendation was generated",
				}
				content, _ := json.Marshal(out)
				return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
			}
		}
	}

	inputJSON, _ := json.Marshal(req)
	draftID, err := e.draftStore.Create(ctx, execCtx.UserID, execCtx.ConversationID, inputJSON, "recommendations_v2")
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"status":"failed","error":"create draft failed: %v"}`, err)}, nil
	}

	resp, err := e.recommendations.Recommend(ctx, &req)
	if err != nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, err.Error())
		payload := map[string]any{"draft_id": draftID, "status": "failed", "error": err.Error()}
		content, _ := json.Marshal(payload)
		return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
	}
	if resp == nil || resp.VolunteerPlan == nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, "recommendation response missing volunteer_plan")
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf(`{"draft_id":%d,"status":"failed","error":"recommendation response missing volunteer_plan"}`, draftID)}, nil
	}

	planJSON, _ := json.Marshal(resp.VolunteerPlan)
	if err := e.draftStore.MarkReady(ctx, execCtx.UserID, draftID, planJSON); err != nil {
		_ = e.draftStore.MarkFailed(ctx, execCtx.UserID, draftID, err.Error())
		payload := map[string]any{"draft_id": draftID, "status": "failed", "error": fmt.Sprintf("persist draft failed: %v", err)}
		content, _ := json.Marshal(payload)
		return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
	}

	out := map[string]any{
		"draft_id":    draftID,
		"status":      "ready",
		"strategy":    resp.Strategy,
		"rush_count":  resp.RushCount,
		"match_count": resp.MatchCount,
		"safe_count":  resp.SafeCount,
		"rank_window": resp.RankWindow,
	}
	content, _ := json.Marshal(out)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

// classifyRequestFields 按当前推荐算法的实际行为，把请求里非空的字段拆成三类：
//   - hard：会真正剔除候选的硬过滤（SQL where 或 in-memory filter）
//   - soft：仅影响排序加权、不剔除候选的软加分
//   - unused：当前算法实现没有读取的字段（保留作为 future-proof，模型若收集了也应告知用户暂未生效）
//
// 这个映射必须随算法演化同步维护——单元测试 tools_test.go 应该断言所有
// RecommendationRequest 公开字段都被分类，避免漏项。
func classifyRequestFields(req *admission.RecommendationRequest) (hard, soft, unused []string) {
	// 硬过滤组（SQL 层 + filterByPreference 层）
	if req.RegionCode != "" {
		hard = append(hard, "region_code")
	}
	if req.SubjectCategoryCode != "" {
		hard = append(hard, "subject_category_code")
	}
	if req.SubjectRequirementCode != "" {
		hard = append(hard, "subject_requirement_code")
	}
	if req.AdmissionYear != nil {
		hard = append(hard, "admission_year")
	}
	if req.BudgetTuitionMax != nil {
		hard = append(hard, "budget_tuition_max")
	}
	if len(req.ExcludedProvinces) > 0 {
		hard = append(hard, "excluded_provinces")
	}
	if len(req.ExcludedCities) > 0 {
		hard = append(hard, "excluded_cities")
	}
	if len(req.OnlyCities) > 0 {
		hard = append(hard, "only_cities")
	}
	if len(req.OnlyProvinces) > 0 {
		hard = append(hard, "only_provinces")
	}
	if len(req.ExcludedMajors) > 0 {
		hard = append(hard, "excluded_majors")
	}
	if len(req.ExcludedKeywords) > 0 {
		hard = append(hard, "excluded_keywords")
	}
	if len(req.RequiredMajors) > 0 {
		hard = append(hard, "required_majors")
	}
	if req.MathScore != nil {
		hard = append(hard, "math_score")
	}
	if req.PhysicsScore != nil {
		hard = append(hard, "physics_score")
	}

	// 软加分组（影响 composite_score 排序，不剔除）
	if len(req.PreferredCities) > 0 {
		soft = append(soft, "preferred_cities")
	}
	if len(req.PreferredProvinces) > 0 {
		soft = append(soft, "preferred_provinces")
	}
	if len(req.PreferredMajors) > 0 {
		soft = append(soft, "preferred_majors")
	}
	if len(req.FamilyResources) > 0 {
		soft = append(soft, "family_resources")
	}
	if req.FamilyEconomy != "" {
		soft = append(soft, "family_economy")
	}
	if req.HollandCode != "" {
		soft = append(soft, "holland_code")
	}
	if len(req.CareerPlans) > 0 {
		soft = append(soft, "career_plans")
	}

	// 未被算法读取的字段（如有）— 当前没有"已收集但完全不读"的字段，
	// 保留切片留作算法演化时的对账位。
	if req.Gender != "" {
		unused = append(unused, "gender")
	}
	if req.Language != "" {
		unused = append(unused, "language")
	}
	if len(req.Health) > 0 {
		unused = append(unused, "health")
	}
	return hard, soft, unused
}

// trimPreviewItems 把推荐结果按三档各取若干条返回给模型，仅保留最少必要字段，
// 避免 dry_run 在多轮对话里把 prompt context 撑爆。
func trimPreviewItems(items []admission.RecommendationItem, perTier int) []map[string]any {
	if perTier <= 0 || len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	out := make([]map[string]any, 0, perTier*3)
	// 用索引迭代避免每次复制 RecommendationItem（结构体较大，gocritic 会告警）。
	for i := range items {
		it := &items[i]
		if counts[it.Tier] >= perTier {
			continue
		}
		counts[it.Tier]++
		out = append(out, map[string]any{
			"tier":             it.Tier,
			"university_name":  it.UniversityName,
			"group_code":       it.GroupCode,
			"local_major_name": it.LocalMajorName,
			"probability":      it.Probability,
		})
	}
	return out
}

func (e *ToolExecutor) executeApplyFilter(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		FilterType string          `json:"filter_type"`
		FilterData json.RawMessage `json:"filter_data"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter arguments: %v", err)}, nil
	}

	// For MVP, we just validate the filter data can be parsed
	var filter admission.AdmissionLineFilter
	if err := decodeAdmissionLineFilter(params.FilterData, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter data: %v", err)}, nil
	}

	result := map[string]any{
		"filter_type": params.FilterType,
		"valid":       true,
		"filter":      filter,
	}
	content, _ := json.Marshal(result)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func (e *ToolExecutor) executeSearchUniversities(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		Filter json.RawMessage `json:"filter"`
		Limit  int             `json:"limit"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid search arguments: %v", err)}, nil
	}
	if params.Limit <= 0 || params.Limit > 20 {
		params.Limit = 5
	}

	var filter admission.AdmissionLineFilter
	if err := decodeAdmissionLineFilter(params.Filter, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter: %v", err)}, nil
	}

	lines, err := e.admissionLineStore.ListAdmissionLines(ctx, &filter)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Search error: %v", err)}, nil
	}

	count := len(lines)
	top := lines
	if len(top) > params.Limit {
		top = top[:params.Limit]
	}

	result := map[string]any{
		"count": count,
		"top":   top,
	}
	content, _ := json.Marshal(result)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func (e *ToolExecutor) executeAggregateData(ctx context.Context, call ToolCall) (*ToolResult, error) {
	var params struct {
		Filter json.RawMessage `json:"filter"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid aggregate arguments: %v", err)}, nil
	}

	var filter admission.AggregateFilter
	if err := decodeAggregateFilter(params.Filter, &filter); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid filter: %v", err)}, nil
	}

	resp, err := e.aggregateStore.Aggregate(ctx, &filter)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Aggregate error: %v", err)}, nil
	}

	content, _ := json.Marshal(resp)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

// Allowed chart types — we control the rendered echarts options, but
// still limit which kinds the model can pick so unsupported shapes
// fail fast instead of producing empty visuals client-side.
var allowedChartTypes = map[string]bool{
	"bar":  true,
	"line": true,
	"pie":  true,
}

// chartParams is what the LLM is allowed to send. Notice there is NO
// way for the model to pass raw echarts option JSON — the server-side
// builder takes these high-level intent fields and constructs a
// whitelisted echarts option from them.
type chartParams struct {
	ChartType  string           `json:"chart_type"`
	Title      string           `json:"title"`
	DataSource string           `json:"data_source"` // "tool_result:<call_id>" or "inline"
	InlineData []map[string]any `json:"inline_data"`
	XField     string           `json:"x_field"`
	YFields    []string         `json:"y_fields"`
}

func (e *ToolExecutor) executeRenderChart(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	var p chartParams
	if err := json.Unmarshal([]byte(call.Function.Arguments), &p); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid render_chart arguments: %v", err)}, nil
	}
	if !allowedChartTypes[p.ChartType] {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Unsupported chart_type: %s", p.ChartType)}, nil
	}

	rows, err := resolveChartData(&p, execCtx)
	if err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_chart: %v", err)}, nil
	}
	if len(rows) == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: no data points"}, nil
	}
	if p.XField == "" {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: x_field is required"}, nil
	}
	if p.ChartType != "pie" && len(p.YFields) == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: "render_chart: y_fields is required for bar/line"}, nil
	}

	option := buildEchartsOption(&p, rows)
	widget := Widget{
		ID:   NewWidgetID(),
		Kind: "chart",
		Payload: map[string]any{
			"title":  p.Title,
			"type":   p.ChartType,
			"option": option,
		},
	}
	if execCtx.EmitWidget != nil {
		execCtx.EmitWidget(widget)
	}
	return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Chart rendered (%d points)", len(rows))}, nil
}

// resolveChartData picks data rows either from a prior tool result or
// from the inline_data field. When pulling from a prior tool result, we
// expect that result's content to be JSON with either {"top": [...]}
// (the search_universities shape) or a top-level array.
func resolveChartData(p *chartParams, execCtx ToolExecContext) ([]map[string]any, error) {
	if p.DataSource == "" || p.DataSource == "inline" {
		return p.InlineData, nil
	}
	if !strings.HasPrefix(p.DataSource, "tool_result:") {
		return nil, fmt.Errorf("data_source must be \"inline\" or \"tool_result:<id>\"")
	}
	if execCtx.ResolveResult == nil {
		return nil, fmt.Errorf("no resolver for tool_result references")
	}
	id := strings.TrimPrefix(p.DataSource, "tool_result:")
	content, ok := execCtx.ResolveResult(id)
	if !ok {
		return nil, fmt.Errorf("tool_result %s not found in this run", id)
	}
	// Try the search_universities shape first; fall back to aggregate_data shape, then bare array.
	var wrapper struct {
		Top []map[string]any `json:"top"`
	}
	if err := json.Unmarshal([]byte(content), &wrapper); err == nil && len(wrapper.Top) > 0 {
		return wrapper.Top, nil
	}
	var aggWrapper struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &aggWrapper); err == nil && len(aggWrapper.Items) > 0 {
		return aggWrapper.Items, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("tool_result %s is not chartable", id)
}

// buildEchartsOption assembles a strictly-whitelisted echarts option
// from chart params and rows. ONLY the fields we add here end up in the
// payload, so the LLM cannot smuggle formatter strings, JS expressions,
// or unrelated keys into the rendered chart.
func buildEchartsOption(p *chartParams, rows []map[string]any) map[string]any {
	option := map[string]any{
		"title":   map[string]any{"text": sanitizeString(p.Title)},
		"tooltip": map[string]any{"trigger": "axis"},
		"grid":    map[string]any{"left": "10%", "right": "5%", "bottom": "10%"},
	}

	if p.ChartType == "pie" {
		// Pie uses a single y-field as the value channel; default to the
		// first available y_field, else the second column of the row.
		valueField := ""
		if len(p.YFields) > 0 {
			valueField = p.YFields[0]
		}
		data := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			name, _ := r[p.XField].(string)
			value := numericValue(r, valueField)
			data = append(data, map[string]any{
				"name":  sanitizeString(name),
				"value": value,
			})
		}
		option["series"] = []map[string]any{{
			"type": "pie",
			"data": data,
		}}
		option["legend"] = map[string]any{"orient": "horizontal", "bottom": 0}
		return option
	}

	// bar / line: shared xAxis / yAxis / series structure
	xCategories := make([]string, 0, len(rows))
	for _, r := range rows {
		x, _ := r[p.XField].(string)
		xCategories = append(xCategories, sanitizeString(x))
	}
	option["xAxis"] = map[string]any{
		"type": "category",
		"data": xCategories,
	}
	option["yAxis"] = map[string]any{"type": "value"}

	series := make([]map[string]any, 0, len(p.YFields))
	legend := make([]string, 0, len(p.YFields))
	for _, yf := range p.YFields {
		values := make([]float64, 0, len(rows))
		for _, r := range rows {
			values = append(values, numericValue(r, yf))
		}
		series = append(series, map[string]any{
			"name": sanitizeString(yf),
			"type": p.ChartType,
			"data": values,
		})
		legend = append(legend, sanitizeString(yf))
	}
	option["series"] = series
	option["legend"] = map[string]any{"data": legend}
	return option
}

// numericValue extracts a float from a row's field, coercing
// number-typed JSON values to float64. Anything non-numeric becomes 0
// rather than crashing — bad input becomes a visible zero on the chart,
// not a server error.
func numericValue(row map[string]any, field string) float64 {
	v, ok := row[field]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

// sanitizeString trims values used as text labels in chart options.
// We do not strip HTML — the frontend renders echarts options through
// the echarts library which treats labels as text — but we cap length
// so a runaway model cannot ship a megabyte of label text.
func sanitizeString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// cardParams is the LLM-facing shape for render_card.
type cardParams struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Metrics     []struct {
		Label string `json:"label"`
		Value string `json:"value"`
		Trend string `json:"trend"`
	} `json:"metrics"`
	Link struct {
		Text string `json:"text"`
		Href string `json:"href"`
	} `json:"link"`
}

var allowedCardTrends = map[string]bool{
	"":     true, // omitted is fine
	"up":   true,
	"down": true,
	"flat": true,
}

func (e *ToolExecutor) executeRenderCard(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	var p cardParams
	if err := json.Unmarshal([]byte(call.Function.Arguments), &p); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid render_card arguments: %v", err)}, nil
	}
	if strings.TrimSpace(p.Title) == "" {
		return &ToolResult{ToolCallID: call.ID, Content: "render_card: title is required"}, nil
	}

	metrics := make([]map[string]any, 0, len(p.Metrics))
	for _, m := range p.Metrics {
		if !allowedCardTrends[m.Trend] {
			m.Trend = ""
		}
		metrics = append(metrics, map[string]any{
			"label": sanitizeString(m.Label),
			"value": sanitizeString(m.Value),
			"trend": m.Trend,
		})
	}

	payload := map[string]any{
		"title":       sanitizeString(p.Title),
		"description": sanitizeString(p.Description),
		"metrics":     metrics,
	}
	whitelist := execCtx.CardLinkWhitelist
	if whitelist == nil {
		whitelist = e.cardLinkWhitelist
	}
	if href := strings.TrimSpace(p.Link.Href); href != "" {
		if !isAllowedCardLink(href, whitelist) {
			return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_card: link %s not allowed", href)}, nil
		}
		payload["link"] = map[string]any{
			"text": sanitizeString(p.Link.Text),
			"href": href,
		}
	}

	widget := Widget{ID: NewWidgetID(), Kind: "card", Payload: payload}
	if execCtx.EmitWidget != nil {
		execCtx.EmitWidget(widget)
	}
	return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Card rendered: %s", payload["title"])}, nil
}

// isAllowedCardLink returns true if href is a relative same-site link
// (starts with "/") or points at a host in the configured whitelist.
// We reject any scheme other than https for absolute URLs so the model
// can't slip data: or javascript: payloads past the frontend.
func isAllowedCardLink(href string, whitelist []string) bool {
	if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
		return true
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range whitelist {
		if host == allowed {
			return true
		}
	}
	return false
}

// formParams 是 LLM 调 render_form 时允许的入参。fields 是字段 key 列表，
// 解析时按 formFieldRegistry 查表；title/intro/submit_label 控制表单展示。
type formParams struct {
	Title       string   `json:"title"`
	Intro       string   `json:"intro"`
	Fields      []string `json:"fields"`
	SubmitLabel string   `json:"submit_label"`
}

// maxFormFieldsPerCall 控制单次表单的最大字段数。设计上一张表单覆盖
// 一轮"批量收偏好"——超过 6 个用户会眼花，应拆成多张。
const maxFormFieldsPerCall = 6

func (e *ToolExecutor) executeRenderForm(ctx context.Context, call ToolCall, execCtx ToolExecContext) (*ToolResult, error) {
	_ = ctx
	var p formParams
	if err := json.Unmarshal([]byte(call.Function.Arguments), &p); err != nil {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Invalid render_form arguments: %v", err)}, nil
	}
	if strings.TrimSpace(p.Title) == "" {
		return &ToolResult{ToolCallID: call.ID, Content: "render_form: title is required"}, nil
	}
	if len(p.Fields) == 0 {
		return &ToolResult{ToolCallID: call.ID, Content: "render_form: at least one field is required"}, nil
	}
	if len(p.Fields) > maxFormFieldsPerCall {
		return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_form: at most %d fields per form", maxFormFieldsPerCall)}, nil
	}

	defs := make([]formFieldDef, 0, len(p.Fields))
	seen := map[string]bool{}
	for _, key := range p.Fields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if seen[key] {
			continue
		}
		def, ok := formFieldRegistry[key]
		if !ok {
			return &ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("render_form: unknown field %q", key)}, nil
		}
		// 城市字段从 DB 动态加载，避免硬编码列表与真实院校分布漂移
		// （例如黑龙江本来有 13 个地市，硬编码里只列了 3 个）。加载失败
		// 时退回 def 自带的静态兜底列表——表单仍可用，只是覆盖面窄。
		if def.Type == formFieldMultiSelect && (def.Key == "preferred_cities" || def.Key == "excluded_cities" || def.Key == "only_cities") {
			loaded, err := e.loadCityOptions(ctx)
			if err == nil && len(loaded) > 0 {
				def.Options = loaded
			}
		}
		seen[key] = true
		defs = append(defs, def)
	}

	submitLabel := sanitizeString(p.SubmitLabel)
	if submitLabel == "" {
		submitLabel = "提交并继续"
	}

	payload := map[string]any{
		"title":        sanitizeString(p.Title),
		"intro":        sanitizeString(p.Intro),
		"fields":       defs,
		"submit_label": submitLabel,
	}
	widget := Widget{ID: NewWidgetID(), Kind: "form", Payload: payload}
	if execCtx.EmitWidget != nil {
		execCtx.EmitWidget(widget)
	}
	// 返回结构化结果而非自由文本，方便模型在下一轮看到"自己刚弹了
	// 哪些字段"——避免它重复发同一张表单。
	out := map[string]any{
		"status":      "form_rendered",
		"widget_id":   widget.ID,
		"field_keys":  collectFieldKeys(defs),
		"awaiting":    "user_submission",
		"description": "表单已发给用户，等待用户提交（form_submission 代码块）后再继续",
	}
	content, _ := json.Marshal(out)
	return &ToolResult{ToolCallID: call.ID, Content: string(content)}, nil
}

func collectFieldKeys(defs []formFieldDef) []string {
	keys := make([]string, 0, len(defs))
	for _, d := range defs {
		keys = append(keys, d.Key)
	}
	return keys
}

// FormFieldKeys 暴露当前白名单的字段 key 列表，供 system prompt 拼装时
// 使用——把可选 key 内嵌到 prompt 里，模型不必猜。
func FormFieldKeys() []string {
	keys := make([]string, 0, len(formFieldRegistry))
	for k := range formFieldRegistry {
		keys = append(keys, k)
	}
	// 排序，保证 prompt 稳定，cache 命中率不会因 map 遍历乱序而抖动。
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// DefaultTools returns the default set of tool definitions for the admission agent.
func DefaultTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "apply_filter",
				Description: "对当前的院校查询施加筛选条件。filter_type 语义：replace=用 filter_data 整体替换当前筛选（用户切换关注点时最常用，放宽/降级层次也走这个）；add=在不冲突的维度上追加；remove=单点移除一个条件；reset=清空所有筛选。filter_data 必须使用 snake_case 字段名，例如 region_code、subject_category_code、exclude_provinces、is_985、min_score_from、min_score_to、tag_query。注意：志愿表主流程不依赖此工具，仅在用户进行探索性查询时使用。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter_type": map[string]any{"type": "string", "enum": []string{"add", "remove", "replace", "reset"}, "description": "如何修改当前筛选条件"},
						"filter_data": map[string]any{"type": "object", "description": "筛选字段，JSON 对象"},
					},
					Required: []string{"filter_type", "filter_data"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "search_universities",
				Description: "按筛选条件查询院校列表，返回命中数量和前若干条结果。filter 必须使用 snake_case 字段名（region_code、subject_category_code、exclude_provinces、is_985、min_score_from、min_score_to、tag_query 等）。用于用户的点查问题（『XX 大学今年位次』『某分数能上哪些 211』），志愿表生成流程不依赖此工具。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter": map[string]any{"type": "object", "description": "AdmissionLineFilter，JSON 对象"},
						"limit":  map[string]any{"type": "integer", "description": "返回结果上限（默认 5，最大 20）"},
					},
					Required: []string{"filter"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "aggregate_data",
				Description: "按维度（省份、城市、院校层次等）对录取数据做聚合统计。filter 字段使用 snake_case，包含 group_by 和 metrics。用于回答『各省份招生人数分布』这类聚合问题。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"filter": map[string]any{"type": "object", "description": "AggregateFilter，JSON 对象，含 group_by 和 metrics"},
					},
					Required: []string{"filter"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "generate_volunteer_plan_draft",
				Description: "运行志愿推荐算法，根据 dry_run 切换两种模式：\n• dry_run=true（预览模式）：跑算法但不写库，返回 pool_size / pool_rush_count / pool_match_count / pool_safe_count / plan_size / rank_window / sample_items。多轮对话中每收到一项新偏好就调一次，用于把当前候选规模和分布告诉用户，决定是否继续追问。每次都要传【累计的完整参数集合】，不传增量。\n• dry_run=false（落盘模式）：把推荐结果写入草稿表，返回 draft_id。一次会话只在临门一脚时调用一次——当 plan_size ≤ pool_size ≤ plan_size × 1.5 且三档非空时，可以 dry_run=false 落盘；用户主动要求保存方案时也可落盘。\n必填字段：region_code（仅 230000）、subject_category_code（physics 或 history）、total_score、provincial_rank。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"dry_run":                  map[string]any{"type": "boolean", "description": "true=预览（不写库）；false 或省略=正式落盘并返回 draft_id"},
						"region_code":              map[string]any{"type": "string"},
						"subject_category_code":    map[string]any{"type": "string", "enum": []string{"physics", "history"}},
						"subject_requirement_code": map[string]any{"type": "string"},
						"selected_subjects":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"total_score":              map[string]any{"type": "integer"},
						"provincial_rank":          map[string]any{"type": "integer"},
						"preferred_majors":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "软偏好——仅影响排序加权，候选不会被剔除。用户语气是\"喜欢/感兴趣/倾向\"时填这里"},
						"required_majors":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬白名单——候选必须命中其中任一关键词，否则被剔除。仅当用户明确说\"我想学 X / 只想学 X / 必须是 X\"时才填；含糊时先放 preferred_majors 并追问"},
						"excluded_majors":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬过滤——含有这些关键词的候选会被剔除。用户说\"不想学/不要 X\"时填这里"},
						"excluded_keywords":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"preferred_cities":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "软偏好：命中加分排序，不剔除"},
						"excluded_cities":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬过滤：命中的城市候选剔除"},
						"preferred_provinces":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "软偏好：region_code 列表，命中加分"},
						"excluded_provinces":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬过滤：region_code 列表，命中的省份候选剔除"},
						"only_cities":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬白名单：候选必须在这些城市里。与 only_provinces 是 OR 关系"},
						"only_provinces":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "硬白名单：region_code 列表，候选必须在这些省份里。与 only_cities 是 OR 关系"},
						"priority_strategy":        map[string]any{"type": "string", "enum": []string{"auto", "school", "major"}},
						"plan_size":                map[string]any{"type": "integer"},
						// 画像类软偏好——不收窄候选池，但影响排序质量。
						// 之前 tool schema 漏了这些字段，导致 agent 只在硬过滤维度
						// （地域 / 专业）反复挖深，无法切到家庭背景 / 职业规划等
						// 新角度来询问学生。新对话务必在 2-3 轮硬过滤后弹这些。
						"family_resources":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "家庭行业资源（软偏好）：[公检法,金融,医疗,教育,电网,商业,普通]。影响相关行业相关专业排序"},
						"family_economy":     map[string]any{"type": "string", "enum": []string{"充裕", "中等", "普通", "紧张"}, "description": "家庭经济（软偏好）：紧张时算法降低高学费 / 民办院校权重"},
						"holland_code":       map[string]any{"type": "string", "description": "霍兰德 RIASEC 兴趣类型字符串，例如 'RIA' / 'SEC'。影响匹配兴趣方向的专业排序"},
						"career_plans":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "职业规划（软偏好）：[考公,从医,电网,考研,留学]。影响相关方向加分"},
						"math_score":         map[string]any{"type": "integer", "description": "数学单科分（硬门槛）：用于剔除强依赖数学的专业当分数明显不够时"},
						"physics_score":      map[string]any{"type": "integer", "description": "物理单科分（硬门槛）：同上"},
						"budget_tuition_max": map[string]any{"type": "integer", "description": "学费上限（硬过滤，元/年）：超过的专业剔除"},
					},
					Required: []string{"region_code", "subject_category_code", "total_score", "provincial_rank"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "render_chart",
				Description: "在聊天界面渲染一张图表。仅当用户明确要求可视化、或多组数值并排比较确实比文字更直观时才使用。数据可以通过 inline_data 直接传入，也可以通过 \"tool_result:<call_id>\" 引用本轮之前某次工具调用的输出。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"chart_type":  map[string]any{"type": "string", "enum": []string{"bar", "line", "pie"}, "description": "图表类型"},
						"title":       map[string]any{"type": "string", "description": "图表标题"},
						"data_source": map[string]any{"type": "string", "description": "\"inline\" 表示用 inline_data；\"tool_result:<call_id>\" 表示引用本轮之前某次工具调用的结果"},
						"inline_data": map[string]any{"type": "array", "description": "data_source=inline 时使用的行数据数组；每行是包含 x_field 与 y_fields 键的对象"},
						"x_field":     map[string]any{"type": "string", "description": "用作 x 轴分类的字段名"},
						"y_fields":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "作为数值序列的字段（每个字段一条 series）"},
					},
					Required: []string{"chart_type", "x_field"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "render_form",
				Description: "在聊天界面弹出一张结构化偏好表单，让用户一次勾选多项偏好，代替逐条文字追问。fields 列出本表单包含的字段 key，必须从白名单中选取：preferred_cities / excluded_cities / only_cities / preferred_majors / required_majors / excluded_majors / family_economy / family_resources / career_plans / holland_code / priority_strategy / budget_tuition_max / plan_size。每张表单 1-6 个字段；用户提交后会以 form_submission 代码块形式回灌到对话，agent 不应在弹出表单的同一回合再追问相同字段。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"title":        map[string]any{"type": "string", "description": "表单标题，例如『地域和专业方向偏好』"},
						"intro":        map[string]any{"type": "string", "description": "一句话说明，告诉用户为什么要勾这几项"},
						"fields":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "字段 key 列表（白名单见说明）"},
						"submit_label": map[string]any{"type": "string", "description": "提交按钮文案，默认『提交并继续』"},
					},
					Required: []string{"title", "fields"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Parameters  ToolParameter `json:"parameters"`
			}{
				Name:        "render_card",
				Description: "在聊天界面渲染一张结构化信息卡片（例如一所院校、一个专业的关键指标汇总）。适合用几个关键指标概括单个实体，不适合大段文字。",
				Parameters: ToolParameter{
					Type: "object",
					Properties: map[string]any{
						"title":       map[string]any{"type": "string", "description": "卡片标题"},
						"description": map[string]any{"type": "string", "description": "一行副标题"},
						"metrics": map[string]any{
							"type":        "array",
							"description": "卡片上展示的指标块（建议不超过 4 个）",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{"type": "string"},
									"value": map[string]any{"type": "string"},
									"trend": map[string]any{"type": "string", "enum": []string{"up", "down", "flat"}},
								},
								"required": []string{"label", "value"},
							},
						},
						"link": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
								"href": map[string]any{"type": "string", "description": "站内相对路径或白名单内的 https URL"},
							},
						},
					},
					Required: []string{"title"},
				},
			},
		},
	}
}
