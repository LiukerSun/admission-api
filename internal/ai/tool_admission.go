package ai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"admission-api/internal/analysis"
)

// QueryAdmissionPlanTool exposes the historical enrollment-plan dataset to the
// LLM. It returns a tabular envelope { "items": [...] } so render_chart can
// pick it up via data_source = "tool_result:<call_id>".
type QueryAdmissionPlanTool struct {
	svc analysis.Service
}

// NewQueryAdmissionPlanTool constructs the tool with an analysis service.
func NewQueryAdmissionPlanTool(svc analysis.Service) Tool {
	return &QueryAdmissionPlanTool{svc: svc}
}

func (t *QueryAdmissionPlanTool) Name() string { return "query_admission_plan" }

func (t *QueryAdmissionPlanTool) Schema() FunctionDef {
	return FunctionDef{
		Name:        "query_admission_plan",
		Description: "查询某省份某年某高校的招生计划数据。返回 items 数组，可作为 render_chart 的 data_source。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"year":         map[string]any{"type": "integer", "description": "招生年份 (例如 2024)"},
				"province":     map[string]any{"type": "string", "description": "考生所在省份名称"},
				"school_name":  map[string]any{"type": "string", "description": "目标高校名称（可选，留空查询全省所有高校）"},
				"major_name":   map[string]any{"type": "string", "description": "目标专业名称（可选）"},
				"per_page":     map[string]any{"type": "integer", "description": "返回条数上限 (默认 30，最大 100)"},
			},
			"required": []string{"province"},
		},
	}
}

type queryAdmissionPlanArgs struct {
	Year       int    `json:"year"`
	Province   string `json:"province"`
	SchoolName string `json:"school_name"`
	MajorName  string `json:"major_name"`
	PerPage    int    `json:"per_page"`
}

func (t *QueryAdmissionPlanTool) Execute(cc *CallContext, raw json.RawMessage) ToolResult {
	var args queryAdmissionPlanArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return failTool("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(args.Province) == "" {
		return failTool("province is required")
	}
	perPage := args.PerPage
	if perPage <= 0 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}

	q := &analysis.EnrollmentPlanQuery{
		Province:   args.Province,
		SchoolName: args.SchoolName,
		MajorName:  args.MajorName,
		PageQuery:  analysis.PageQuery{Page: 1, PerPage: perPage},
	}
	if args.Year > 0 {
		q.Year = strconv.Itoa(args.Year)
	}

	resp, err := t.svc.GetEnrollmentPlans(cc.Ctx, q)
	if err != nil {
		return failTool("query failed: " + err.Error())
	}

	items := make([]map[string]any, 0, len(resp.Items))
	for _, p := range resp.Items {
		items = append(items, map[string]any{
			"school_name": p.SchoolName,
			"major_name":  p.MajorName,
			"plan_year":   p.PlanYear,
			"province":    p.Province,
			"plan_count":  derefIntPtr(p.PlanCount),
			"tuition_fee": derefFloatPtr(p.TuitionFee),
			"batch":       p.Batch,
		})
	}
	body, err := json.Marshal(map[string]any{
		"items": items,
		"total": resp.Total,
		"query": q,
	})
	if err != nil {
		return failTool("encode response: " + err.Error())
	}
	return ToolResult{Content: string(body)}
}

// QueryEmploymentTool wraps the employment-data endpoint.
type QueryEmploymentTool struct {
	svc analysis.Service
}

// NewQueryEmploymentTool constructs the tool.
func NewQueryEmploymentTool(svc analysis.Service) Tool {
	return &QueryEmploymentTool{svc: svc}
}

func (t *QueryEmploymentTool) Name() string { return "query_employment" }

func (t *QueryEmploymentTool) Schema() FunctionDef {
	return FunctionDef{
		Name:        "query_employment",
		Description: "查询某专业的就业数据（薪资、就业率等）。返回 items 数组，可作为 render_chart 的 data_source。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"major_name": map[string]any{"type": "string", "description": "目标专业名称"},
				"province":   map[string]any{"type": "string", "description": "省份（可选）"},
				"year":       map[string]any{"type": "integer", "description": "年份（可选）"},
				"industry":   map[string]any{"type": "string", "description": "行业（可选）"},
				"per_page":   map[string]any{"type": "integer", "description": "返回条数上限 (默认 30)"},
			},
			"required": []string{"major_name"},
		},
	}
}

type queryEmploymentArgs struct {
	MajorName string `json:"major_name"`
	Province  string `json:"province"`
	Year      int    `json:"year"`
	Industry  string `json:"industry"`
	PerPage   int    `json:"per_page"`
}

func (t *QueryEmploymentTool) Execute(cc *CallContext, raw json.RawMessage) ToolResult {
	var args queryEmploymentArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return failTool("invalid arguments: " + err.Error())
	}
	if strings.TrimSpace(args.MajorName) == "" {
		return failTool("major_name is required")
	}
	perPage := args.PerPage
	if perPage <= 0 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}

	resp, err := t.svc.GetEmploymentData(cc.Ctx, &analysis.EmploymentDataQuery{
		MajorName: args.MajorName,
		Province:  args.Province,
		Year:      args.Year,
		Industry:  args.Industry,
		Page:      1,
		PerPage:   perPage,
	})
	if err != nil {
		return failTool("query failed: " + err.Error())
	}
	items := make([]map[string]any, 0, len(resp.Data))
	for _, d := range resp.Data {
		items = append(items, map[string]any{
			"major_name":       d.MajorName,
			"province":         d.Province,
			"year":             d.Year,
			"graduates_count":  d.GraduatesCount,
			"employment_rate":  d.EmploymentRate,
			"average_salary":   d.AverageSalary,
			"industry":         d.Industry,
			"further_study":    d.FurtherStudyRate,
		})
	}
	body, err := json.Marshal(map[string]any{
		"items": items,
		"total": resp.Total,
	})
	if err != nil {
		return failTool("encode response: " + err.Error())
	}
	return ToolResult{Content: string(body)}
}

func failTool(msg string) ToolResult {
	return ToolResult{
		Content: fmt.Sprintf(`{"ok":false,"error":%q}`, msg),
		Error:   msg,
		IsError: true,
	}
}

func derefIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func derefFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
