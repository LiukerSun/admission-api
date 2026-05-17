package volunteerplan

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"admission-api/internal/conversation"
)

type Service interface {
	GetDraft(ctx context.Context, userID, draftID int64) (*Draft, error)
	ListDraftsByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error)
	AdoptDraft(ctx context.Context, userID, draftID int64, title string) (*UserVolunteerPlan, error)
	// ListPlans 返回轻量摘要（不含 plan_json）。前端用 summary 渲染左侧列表，
	// 选中后再通过 GetPlan 拉详情。
	ListPlans(ctx context.Context, userID int64) ([]*UserVolunteerPlanSummary, error)
	GetPlan(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error)
	// UpdatePlanMeta 部分更新 title / description。任一字段 nil 表示不动。
	// 用 title="" 表示清空名字（service 层会拦掉这种没意义的情况）。
	UpdatePlanMeta(ctx context.Context, userID, planID int64, title, description *string) (*UserVolunteerPlan, error)
	// DeletePlan 软删除。再次调用已删方案返回 ErrPlanNotFound。
	DeletePlan(ctx context.Context, userID, planID int64) error
}

type service struct {
	drafts        DraftStore
	plans         PlanStore
	conversations conversation.Service
}

func NewService(drafts DraftStore, plans PlanStore, conversations conversation.Service) Service {
	return &service{drafts: drafts, plans: plans, conversations: conversations}
}

func (s *service) GetDraft(ctx context.Context, userID, draftID int64) (*Draft, error) {
	return s.drafts.GetByID(ctx, userID, draftID)
}

// ListDraftsByConversation gates the listing on conversation ownership.
// Drafts are joined to a conversation via conversation_id, so listing
// without checking the conversation owner would let a user enumerate
// another user's draft history simply by guessing conversation ids.
// Returning ErrConversationNotFound for both "no such conversation"
// and "not owned by caller" avoids leaking conversation existence.
func (s *service) ListDraftsByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error) {
	conv, err := s.conversations.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.UserID == nil || *conv.UserID != userID {
		return nil, conversation.ErrConversationNotFound
	}
	return s.drafts.ListByConversation(ctx, userID, conversationID)
}

func (s *service) ListPlans(ctx context.Context, userID int64) ([]*UserVolunteerPlanSummary, error) {
	return s.plans.ListSummariesByUser(ctx, userID)
}

func (s *service) GetPlan(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error) {
	p, err := s.plans.GetByID(ctx, userID, planID)
	if err != nil {
		return nil, err
	}
	p.PlanJSON = normalizeLegacyPlanJSON(p.PlanJSON)
	return p, nil
}

func (s *service) DeletePlan(ctx context.Context, userID, planID int64) error {
	return s.plans.SoftDelete(ctx, userID, planID)
}

// UpdatePlanMeta 校验后转交 store。title 不允许写空字符串（业务上方案必须有名字）；
// description 允许空（用户主动清空备注是合法操作）。
func (s *service) UpdatePlanMeta(ctx context.Context, userID, planID int64, title, description *string) (*UserVolunteerPlan, error) {
	if title != nil {
		trimmed := strings.TrimSpace(*title)
		if trimmed == "" {
			return nil, ErrInvalidPlanTitle
		}
		title = &trimmed
	}
	p, err := s.plans.UpdateMeta(ctx, userID, planID, title, description)
	if err != nil {
		return nil, err
	}
	p.PlanJSON = normalizeLegacyPlanJSON(p.PlanJSON)
	return p, nil
}

// normalizeLegacyPlanJSON 是给旧版方案 plan_json 的运行时兼容层。
// 历史上 recommendation_service 一度把 {"items":[...]} 直接当作 plan_json
// 落库，与前端期望的 {"groups":[...],"stats":{...}} 完全不兼容，导致
// 采纳后的方案在 V2 plans 页面渲染为空。算法侧已经修了写路径，但
// 已落库的老方案仍然是 items 形态——挨个迁移既要 SQL UPDATE 又要
// 重启风险，不如在读路径上做一次本地折叠，对前端透明。
//
// 判定逻辑：plan_json 里没有 groups 字段但有 items 数组 → 触发转换。
// 已经是新格式 (有 groups) 或者 plan_json 为空 → 原样返回。
//
// 字段映射（与 admission.buildVolunteerPlanFromItems 保持一致）：
//
//	it.university_code → group.universityCode
//	it.university_name → group.universityName
//	it.group_code      → group.groupCode / group.groupName(兜底)
//	it.order           → group.orderNo
//	it.tier            → group.remark
//	it.local_major_*   → group.majors[0].major{Code,Name,Order=1}
func normalizeLegacyPlanJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw // 不是 object，无法兼容；让上层照旧处理
	}
	if _, hasGroups := obj["groups"]; hasGroups {
		return raw // 已经是新格式
	}
	itemsRaw, hasItems := obj["items"]
	if !hasItems {
		return raw
	}
	var items []map[string]any
	if err := json.Unmarshal(itemsRaw, &items); err != nil || len(items) == 0 {
		return raw
	}

	type majorView struct {
		MajorOrder int    `json:"majorOrder"`
		MajorCode  string `json:"majorCode"`
		MajorName  string `json:"majorName"`
	}
	type groupView struct {
		OrderNo          int         `json:"orderNo"`
		UniversityCode   string      `json:"universityCode"`
		UniversityName   string      `json:"universityName"`
		GroupCode        string      `json:"groupCode"`
		GroupName        string      `json:"groupName"`
		IsObeyAdjustment bool        `json:"isObeyAdjustment"`
		Remark           string      `json:"remark"`
		Majors           []majorView `json:"majors"`
	}
	type statsView struct {
		SchoolCount int `json:"schoolCount"`
		GroupCount  int `json:"groupCount"`
		RecordCount int `json:"recordCount"`
	}

	getString := func(m map[string]any, key string) string {
		if v, ok := m[key].(string); ok {
			return v
		}
		return ""
	}
	getInt := func(m map[string]any, key string) int {
		switch v := m[key].(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
		return 0
	}

	groups := make([]groupView, 0, len(items))
	indexByKey := make(map[string]int, len(items))
	uniqUniversities := make(map[string]struct{})

	// tier 字段是算法用的英文 (rush/match/safe)，必须翻译成中文 (冲/稳/保)
	// 才能落到 remark 给用户看，与 admission.tierToChinese 保持一致。未知值
	// 原样保留以便排查异常。
	tierZh := func(t string) string {
		switch t {
		case "rush":
			return "冲"
		case "match":
			return "稳"
		case "safe":
			return "保"
		default:
			return t
		}
	}

	for _, it := range items {
		uc := getString(it, "university_code")
		gc := getString(it, "group_code")
		key := uc + "\x1F" + gc
		idx, exists := indexByKey[key]
		if !exists {
			groups = append(groups, groupView{
				OrderNo:          getInt(it, "order"),
				UniversityCode:   uc,
				UniversityName:   getString(it, "university_name"),
				GroupCode:        gc,
				GroupName:        gc,
				IsObeyAdjustment: true,
				Remark:           tierZh(getString(it, "tier")),
				Majors:           []majorView{},
			})
			idx = len(groups) - 1
			indexByKey[key] = idx
		}
		groups[idx].Majors = append(groups[idx].Majors, majorView{
			MajorOrder: len(groups[idx].Majors) + 1,
			MajorCode:  getString(it, "local_major_code"),
			MajorName:  getString(it, "local_major_name"),
		})
		if uc != "" {
			uniqUniversities[uc] = struct{}{}
		}
	}

	recordCount := 0
	for _, g := range groups {
		recordCount += len(g.Majors)
	}

	out := map[string]any{
		"groups": groups,
		"stats": statsView{
			SchoolCount: len(uniqUniversities),
			GroupCount:  len(groups),
			RecordCount: recordCount,
		},
		"items": itemsRaw, // 保留原始 items 给可能的调试/分析路径
	}
	normalized, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return normalized
}

// AdoptDraft performs the three-step adopt flow: insert the user plan
// from the draft snapshot, mark the draft adopted (state-machine
// guarded), and archive the originating conversation so the chat UI
// flips into read-only mode.
//
// Transactionality: the three stores each hold their own pgxpool and do
// not currently share a pgx.Tx, so we cannot wrap all three writes in a
// single atomic transaction without an invasive store refactor. Instead
// we accept "best-effort with detailed logging" — the plan insert is
// idempotent (ON CONFLICT (user_id, source_draft_id) DO NOTHING then
// re-select), MarkAdopted is state-machine-guarded so a double click
// won't double-archive, and downstream failures are logged but do not
// roll back the user-visible plan row. This matches the user expectation
// that "if the plan was created, the user has succeeded; downstream
// bookkeeping failures are an ops concern, not a user one".
func (s *service) AdoptDraft(ctx context.Context, userID, draftID int64, title string) (*UserVolunteerPlan, error) {
	draft, err := s.drafts.GetByID(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}
	if draft.Status == DraftStatusAdopted {
		return nil, ErrDraftAlreadyAdopted
	}
	if draft.Status != DraftStatusReady {
		return nil, ErrDraftNotReady
	}
	if len(draft.PlanJSON) == 0 || string(draft.PlanJSON) == "null" {
		return nil, ErrDraftCorrupted
	}
	if !json.Valid(draft.PlanJSON) {
		return nil, ErrDraftCorrupted
	}

	normalizedTitle := strings.TrimSpace(title)
	if normalizedTitle == "" {
		normalizedTitle = "志愿方案"
	}

	plan, err := s.plans.CreateFromDraft(ctx, userID, draftID, normalizedTitle, draft.PlanJSON)
	if err != nil {
		return nil, err
	}

	// Step 2: mark the draft adopted. The state-machine guard means a
	// second adopt call on the same draft cleanly returns
	// ErrDraftNotInExpectedState (which we mask here as a no-op log
	// because the plan was already produced on the prior call).
	if markErr := s.drafts.MarkAdopted(ctx, userID, draftID); markErr != nil {
		if errors.Is(markErr, ErrDraftNotInExpectedState) {
			// Re-adoption of an already-adopted draft. The plan row is
			// either the one we just created (ON CONFLICT path) or
			// pre-existed; either way the user-visible answer is the
			// same returned plan. Log for diagnosability.
			slog.Warn("adopt step failed",
				"step", "mark_adopted",
				"draftID", draftID,
				"userID", userID,
				"reason", "draft already left ready state",
			)
		} else {
			slog.Error("adopt step failed",
				"step", "mark_adopted",
				"draftID", draftID,
				"userID", userID,
				"error", markErr,
			)
		}
	}

	// Step 3: archive the originating conversation so the chat UI flips
	// to read-only. Failure here doesn't undo the adopt; ops must
	// reconcile via the logs.
	if archErr := s.conversations.ArchiveConversation(ctx, draft.ConversationID); archErr != nil {
		slog.Error("adopt step failed",
			"step", "archive_conversation",
			"draftID", draftID,
			"userID", userID,
			"conversationID", draft.ConversationID,
			"error", archErr,
		)
	}

	return plan, nil
}
