package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"admission-api/internal/admission"
)

const recommendationTunerPrompt = `你是一名资深高考志愿规划师，负责对算法生成的志愿草表做最后的人工审阅。
输入：学生完整画像 + 一张冲/稳/保志愿草表（每条带历史位次、概率、综合得分、推荐理由）。
任务：
1. 找出明显不合理的条目（同质化严重、与学生明确偏好冲突、忽视专业壁垒、保底不稳等），列出问题。
2. 给出 3 条以内的总体调整建议（不要重复志愿表里已有的字段）。
3. 不要发明新的院校或专业，只能基于草表里已有的条目做评论。
4. 最终输出严格 JSON：{"summary":"...","suggested_swaps":[{"from_order":1,"reason":"..."}]}
`

// RecommendationTuner is the LLM-backed implementation of admission.RecommendationTuner.
// It writes a short Chinese summary into resp.LLMSummary and (best-effort) tweaks the items
// based on suggested swaps. It never invents new candidates — only re-orders or annotates.
type RecommendationTuner struct {
	llm LLMProxy
}

// NewRecommendationTuner returns a tuner backed by the given LLM proxy.
func NewRecommendationTuner(llm LLMProxy) *RecommendationTuner {
	return &RecommendationTuner{llm: llm}
}

// Tune asks the LLM to review the draft plan. Failures degrade gracefully: the original
// response is returned unchanged, since the algorithm output is already deterministically valid.
func (t *RecommendationTuner) Tune(ctx context.Context, req *admission.RecommendationRequest, resp *admission.RecommendationResponse) (*admission.RecommendationResponse, error) {
	if t == nil || t.llm == nil || resp == nil {
		return resp, nil
	}
	user := buildTunerUserMessage(req, resp)
	out, err := t.llm.ChatCompletion(ctx, []Message{
		{Role: "system", Content: recommendationTunerPrompt},
		{Role: "user", Content: user},
	}, nil)
	if err != nil {
		slog.Warn("recommendation tuner llm call failed", "error", err)
		resp.Notes = append(resp.Notes, "AI 复核暂不可用，仅返回算法结果")
		return resp, nil
	}
	parsed := parseTunerOutput(out.Content)
	if parsed.Summary != "" {
		resp.LLMSummary = parsed.Summary
	}
	// swap 建议挂到对应 order 的条目上——from_order 是合并后整张表的 1-based 编号
	for _, sw := range parsed.SuggestedSwaps {
		if sw.FromOrder <= 0 || sw.FromOrder > len(resp.Items) {
			continue
		}
		idx := sw.FromOrder - 1
		if sw.Reason != "" {
			resp.Items[idx].Warnings = append(resp.Items[idx].Warnings, "AI 建议关注: "+sw.Reason)
		}
	}
	return resp, nil
}

func buildTunerUserMessage(req *admission.RecommendationRequest, resp *admission.RecommendationResponse) string {
	profile, _ := json.MarshalIndent(req, "", "  ")
	rows := make([]map[string]any, 0, len(resp.Items))
	for i := range resp.Items {
		it := &resp.Items[i]
		rows = append(rows, map[string]any{
			"order":          it.Order,
			"tier":           it.Tier,
			"probability":    it.Probability,
			"composite":      it.CompositeScore,
			"university":     it.UniversityName,
			"city":           it.City,
			"is_985":         it.Is985,
			"is_211":         it.Is211,
			"major":          it.LocalMajorName,
			"discipline":     it.DisciplineCategory,
			"min_rank":       it.HistoricalMinRank,
			"admitted_count": it.AdmittedCount,
			"reason":         it.Reason,
		})
	}
	plan, _ := json.MarshalIndent(rows, "", "  ")
	return fmt.Sprintf("学生画像:\n%s\n\n志愿草表:\n%s", string(profile), string(plan))
}

type tunerOutput struct {
	Summary        string `json:"summary"`
	SuggestedSwaps []struct {
		FromOrder int    `json:"from_order"`
		Reason    string `json:"reason"`
	} `json:"suggested_swaps"`
}

// parseTunerOutput is lenient: it extracts the first {...} block from the LLM response,
// because some providers wrap JSON in markdown fences or add prose before it.
func parseTunerOutput(raw string) tunerOutput {
	var out tunerOutput
	if raw == "" {
		return out
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		out.Summary = strings.TrimSpace(raw)
		return out
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &out); err != nil {
		out.Summary = strings.TrimSpace(raw)
	}
	return out
}
