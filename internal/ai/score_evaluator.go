package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"admission-api/internal/admission"
)

const scoreEvaluatorSystemPrompt = `你是高考志愿专业评估师。给你一对 (大学 × 专业)，请基于公开知识为该组合输出 5 个 0~2.0 的浮点数评分（1.0 表示中等水平），并给出每个维度 30 字以内的中文理由。

评分定义（注意：这里评的是"该专业在该校的客观水平"，与具体学生无关）：
- city_score: 这所大学所在城市的就业/实习/平台资源（不是城市规模）。
- school_score: 这所大学的整体声誉与平台。
- major_score: 该校该专业本身的学科水平、师资、行业认可度。
- ability_improvement_score: 该专业的本科培养能在多大程度上提升学生综合能力（思维训练、技能成长、抗压锻炼）。
- future_competitiveness_score: 该专业方向 5–10 年的就业 / 升学 / 创业前景与抗替代性（含 AI 替代风险）。

打分参考：
- 顶级名校王牌专业 ≈ 1.7–2.0
- 一流大学普通专业 ≈ 1.2–1.5
- 普通大学普通专业 ≈ 0.9–1.1
- 弱校弱专业或夕阳专业 ≈ 0.6–0.85

严格输出 JSON，不要 markdown 围栏，结构：
{
  "city_score": 1.x, "school_score": 1.x, "major_score": 1.x,
  "ability_improvement_score": 1.x, "future_competitiveness_score": 1.x,
  "city_reason": "...", "school_reason": "...", "major_reason": "...",
  "ability_improvement_reason": "...", "future_competitiveness_reason": "..."
}`

// LLMScoreEvaluator implements admission.ScoreEvaluator backed by an LLMProxy.
type LLMScoreEvaluator struct {
	llm     LLMProxy
	modelID string // identifier written to recommendation_precomputed_scores.evaluator_model
}

func NewLLMScoreEvaluator(llm LLMProxy, modelID string) *LLMScoreEvaluator {
	return &LLMScoreEvaluator{llm: llm, modelID: modelID}
}

func (e *LLMScoreEvaluator) Source() string { return "llm" }

func (e *LLMScoreEvaluator) Evaluate(ctx context.Context, row *admission.PrecomputedScoreRow) (*admission.ScoreEvaluation, error) {
	if e == nil || e.llm == nil {
		return nil, fmt.Errorf("llm evaluator not configured")
	}
	user := buildEvaluatorUserMessage(row)
	resp, err := e.llm.ChatCompletion(ctx, []Message{
		{Role: "system", Content: scoreEvaluatorSystemPrompt},
		{Role: "user", Content: user},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}
	parsed, err := parseEvaluatorOutput(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("parse llm output: %w (raw=%q)", err, resp.Content)
	}
	parsed.ModelID = e.modelID
	return parsed, nil
}

func buildEvaluatorUserMessage(row *admission.PrecomputedScoreRow) string {
	tierTag := ""
	switch {
	case row.Is985:
		tierTag = "985"
	case row.Is211:
		tierTag = "211"
	case row.IsDoubleClass:
		tierTag = "双一流"
	}
	if row.UniversityTier != "" {
		tierTag = row.UniversityTier + " " + tierTag
	}

	parts := []string{
		fmt.Sprintf("大学: %s", row.UniversityName),
		fmt.Sprintf("城市: %s (%s)", row.City, row.ProvinceCode),
		fmt.Sprintf("学校档次: %s", strings.TrimSpace(tierTag)),
		fmt.Sprintf("专业: %s (本地代码 %s)", row.LocalMajorName, row.LocalMajorCode),
	}
	if row.DisciplineCategory != "" {
		parts = append(parts, fmt.Sprintf("学科门类: %s", row.DisciplineCategory))
	}
	if row.FirstLevelDiscipline != "" {
		parts = append(parts, fmt.Sprintf("一级学科: %s", row.FirstLevelDiscipline))
	}
	if row.MajorIntro != "" {
		parts = append(parts, fmt.Sprintf("专业简介: %s", truncate(row.MajorIntro, 400)))
	}
	if row.EmploymentDirection != "" {
		parts = append(parts, fmt.Sprintf("就业方向: %s", truncate(row.EmploymentDirection, 200)))
	}
	if row.TagNames != "" {
		parts = append(parts, fmt.Sprintf("CHSI 标签: %s", row.TagNames))
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type evaluatorOutput struct {
	CityScore                   float64 `json:"city_score"`
	SchoolScore                 float64 `json:"school_score"`
	MajorScore                  float64 `json:"major_score"`
	AbilityImprovementScore     float64 `json:"ability_improvement_score"`
	FutureCompetitivenessScore  float64 `json:"future_competitiveness_score"`
	CityReason                  string  `json:"city_reason"`
	SchoolReason                string  `json:"school_reason"`
	MajorReason                 string  `json:"major_reason"`
	AbilityImprovementReason    string  `json:"ability_improvement_reason"`
	FutureCompetitivenessReason string  `json:"future_competitiveness_reason"`
}

func parseEvaluatorOutput(raw string) (*admission.ScoreEvaluation, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found")
	}
	var out evaluatorOutput
	if err := json.Unmarshal([]byte(raw[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &admission.ScoreEvaluation{
		CityScore:                   clampScore(out.CityScore),
		SchoolScore:                 clampScore(out.SchoolScore),
		MajorScore:                  clampScore(out.MajorScore),
		AbilityImprovementScore:     clampScore(out.AbilityImprovementScore),
		FutureCompetitivenessScore:  clampScore(out.FutureCompetitivenessScore),
		CityReason:                  out.CityReason,
		SchoolReason:                out.SchoolReason,
		MajorReason:                 out.MajorReason,
		AbilityImprovementReason:    out.AbilityImprovementReason,
		FutureCompetitivenessReason: out.FutureCompetitivenessReason,
	}, nil
}

// clampScore sanitizes the LLM output: out-of-range or zero → 1.0 fallback.
func clampScore(v float64) float64 {
	if v <= 0 || v > 3 {
		return 1.0
	}
	return v
}
