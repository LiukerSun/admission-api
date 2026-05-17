package ai

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"admission-api/internal/admission"
	"admission-api/internal/volunteerplan"
)

// TestSameRecommendationInput 守护 reuse 路径的核心判等逻辑：
// 旧 input_json 经过 PG JSONB 往返后键序可能与当次 Marshal(req) 不同；
// 切片成员顺序变化算同一意图（LLM 在多轮间可能改写顺序）。
func TestSameRecommendationInput(t *testing.T) {
	base := admission.RecommendationRequest{
		RegionCode:          "230000",
		SubjectCategoryCode: "physics",
		TotalScore:          550,
		ProvincialRank:      16806,
		PlanSize:            40,
		RequiredMajors:      []string{"计算机", "金融"},
		ExcludedProvinces:   []string{"640000", "150000"},
	}
	baseJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal base: %v", err)
	}

	t.Run("identical inputs match", func(t *testing.T) {
		if !sameRecommendationInput(baseJSON, baseJSON) {
			t.Fatalf("identical bytes must compare equal")
		}
	})

	t.Run("key-reordered JSONB still matches", func(t *testing.T) {
		// 模拟 PG JSONB 重排键顺序：手工写一个键顺序完全打乱的等价 JSON。
		reordered := []byte(`{
			"provincial_rank": 16806,
			"plan_size": 40,
			"excluded_provinces": ["640000", "150000"],
			"required_majors": ["计算机", "金融"],
			"subject_category_code": "physics",
			"total_score": 550,
			"region_code": "230000"
		}`)
		if !sameRecommendationInput(reordered, baseJSON) {
			t.Fatalf("key-reordered equivalent JSON should match")
		}
	})

	t.Run("plan_size change triggers drift", func(t *testing.T) {
		changed := base
		changed.PlanSize = 60
		changedJSON, _ := json.Marshal(changed)
		if sameRecommendationInput(baseJSON, changedJSON) {
			t.Fatalf("changed plan_size should NOT match")
		}
	})

	t.Run("required_majors set change triggers drift", func(t *testing.T) {
		// 真实漂移：用户加了"会计"——集合发生变化。
		changed := base
		changed.RequiredMajors = []string{"计算机", "金融", "会计"}
		changedJSON, _ := json.Marshal(changed)
		if sameRecommendationInput(baseJSON, changedJSON) {
			t.Fatalf("required_majors set change should NOT match")
		}
	})

	t.Run("only_provinces drift", func(t *testing.T) {
		// 对话 19 真实场景：用户从"上海+云南"切到"黑龙江"。
		oldReq := base
		oldReq.OnlyProvinces = []string{"310000", "530000"}
		newReq := base
		newReq.OnlyProvinces = []string{"230000"}
		oldJSON, _ := json.Marshal(oldReq)
		newJSON, _ := json.Marshal(newReq)
		if sameRecommendationInput(oldJSON, newJSON) {
			t.Fatalf("only_provinces drift should NOT match")
		}
	})

	t.Run("empty inputs never match", func(t *testing.T) {
		if sameRecommendationInput(nil, baseJSON) {
			t.Fatalf("nil old should not match")
		}
		if sameRecommendationInput(baseJSON, nil) {
			t.Fatalf("nil new should not match")
		}
	})
}

// ---------- fakes for execute path ----------------------------------

type fakeDraftStore struct {
	mu          sync.Mutex
	drafts      map[int64]*volunteerplan.Draft
	nextID      int64
	supersedeOK bool
}

func newFakeDraftStore() *fakeDraftStore {
	return &fakeDraftStore{drafts: map[int64]*volunteerplan.Draft{}, nextID: 1}
}

func (s *fakeDraftStore) GetByID(_ context.Context, userID, id int64) (*volunteerplan.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok || d.UserID != userID {
		return nil, volunteerplan.ErrDraftNotFound
	}
	return d, nil
}

func (s *fakeDraftStore) ListByConversation(_ context.Context, userID, convID int64) ([]*volunteerplan.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*volunteerplan.Draft, 0)
	for _, d := range s.drafts {
		if d.UserID == userID && d.ConversationID == convID {
			out = append(out, d)
		}
	}
	// 按 id desc 模拟 created_at desc
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID > out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (s *fakeDraftStore) Create(_ context.Context, userID, convID int64, inputJSON []byte, algoVer string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.drafts[id] = &volunteerplan.Draft{
		ID:               id,
		UserID:           userID,
		ConversationID:   convID,
		Status:           volunteerplan.DraftStatusGenerating,
		InputJSON:        append([]byte(nil), inputJSON...),
		AlgorithmVersion: algoVer,
	}
	return id, nil
}

func (s *fakeDraftStore) MarkReady(_ context.Context, userID, id int64, planJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusReady
	d.PlanJSON = append([]byte(nil), planJSON...)
	return nil
}

func (s *fakeDraftStore) MarkFailed(_ context.Context, userID, id int64, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusFailed
	d.Error = errMsg
	return nil
}

func (s *fakeDraftStore) MarkAdopted(_ context.Context, userID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	d.Status = volunteerplan.DraftStatusAdopted
	return nil
}

func (s *fakeDraftStore) MarkSuperseded(_ context.Context, userID, id int64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok || d.UserID != userID {
		return volunteerplan.ErrDraftNotFound
	}
	if d.Status != volunteerplan.DraftStatusReady {
		// 模拟真 store 的状态机：只允许 ready -> superseded
		return volunteerplan.ErrDraftNotInExpectedState
	}
	d.Status = volunteerplan.DraftStatusSuperseded
	d.Error = reason
	s.supersedeOK = true
	return nil
}

type fakeRecService struct {
	recommendCount int
	previewCount   int
	resp           *admission.RecommendationResponse
}

func newFakeRecService() *fakeRecService {
	plan := map[string]any{
		"groups": []any{
			map[string]any{
				"orderNo":          1,
				"universityCode":   "10001",
				"universityName":   "北京大学",
				"groupCode":        "01",
				"groupName":        "01",
				"isObeyAdjustment": true,
				"remark":           "冲",
				"majors": []any{
					map[string]any{"majorOrder": 1, "majorCode": "080901", "majorName": "计算机科学与技术"},
				},
			},
		},
		"stats": map[string]any{"schoolCount": 1, "groupCount": 1, "recordCount": 1},
	}
	planJSON, _ := json.Marshal(plan)
	return &fakeRecService{
		resp: &admission.RecommendationResponse{
			Strategy:      "major",
			RushCount:     1,
			MatchCount:    0,
			SafeCount:     0,
			VolunteerPlan: planJSON,
		},
	}
}

func (s *fakeRecService) Recommend(_ context.Context, _ *admission.RecommendationRequest) (*admission.RecommendationResponse, error) {
	s.recommendCount++
	return s.resp, nil
}

func (s *fakeRecService) Preview(_ context.Context, _ *admission.RecommendationRequest) (*admission.PreviewResponse, error) {
	s.previewCount++
	return &admission.PreviewResponse{PoolSize: 10, PlanSize: 40}, nil
}

func newDraftToolCall(dryRun bool, overrides map[string]any) ToolCall {
	args := map[string]any{
		"dry_run":               dryRun,
		"region_code":           "230000",
		"subject_category_code": "physics",
		"total_score":           550,
		"provincial_rank":       16806,
		"plan_size":             40,
		"only_provinces":        []string{"230000"},
		"required_majors":       []string{"计算机", "金融"},
	}
	for k, v := range overrides {
		args[k] = v
	}
	raw, _ := json.Marshal(args)
	var tc ToolCall
	tc.ID = "call-test-1"
	tc.Type = "function"
	tc.Function.Name = "generate_volunteer_plan_draft"
	tc.Function.Arguments = string(raw)
	return tc
}

func parseToolResult(t *testing.T, content string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, content)
	}
	return m
}

// TestExecuteGenerateVolunteerPlanDraftReusesOnIdenticalInput 守护"输入没变就复用"路径。
func TestExecuteGenerateVolunteerPlanDraftReusesOnIdenticalInput(t *testing.T) {
	ds := newFakeDraftStore()
	rec := newFakeRecService()
	exe := NewToolExecutor(nil, nil, rec, ds, nil, nil)
	ctx := context.Background()
	execCtx := ToolExecContext{UserID: 1, ConversationID: 10}

	first, err := exe.executeGenerateVolunteerPlanDraft(ctx, newDraftToolCall(false, nil), execCtx)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	firstOut := parseToolResult(t, first.Content)
	if firstOut["reused"] != false {
		t.Fatalf("first call reused must be false; got %v", firstOut["reused"])
	}
	firstID := firstOut["draft_id"]
	if firstID == nil || rec.recommendCount != 1 {
		t.Fatalf("first call should run Recommend exactly once; got count=%d, out=%v", rec.recommendCount, firstOut)
	}

	second, err := exe.executeGenerateVolunteerPlanDraft(ctx, newDraftToolCall(false, nil), execCtx)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	secondOut := parseToolResult(t, second.Content)
	if secondOut["reused"] != true {
		t.Fatalf("second call should reuse; got %v", secondOut)
	}
	if secondOut["reused_stale"] != false {
		t.Fatalf("second call should have reused_stale=false; got %v", secondOut)
	}
	if secondOut["draft_id"] != firstID {
		t.Fatalf("second call should reuse same draft_id; first=%v second=%v", firstID, secondOut["draft_id"])
	}
	if rec.recommendCount != 1 {
		t.Fatalf("second call must NOT trigger Recommend again; count=%d", rec.recommendCount)
	}
}

// TestExecuteGenerateVolunteerPlanDraftSupersedesOnInputDrift 是本次 bug 的回归测试。
// 真实场景：第一次按偏好 A 落盘，用户后续改了偏好得到 B，再次落盘必须重新跑算法
// 并把旧草稿标记为 superseded —— 而不是悄悄复用旧 draft_id。
func TestExecuteGenerateVolunteerPlanDraftSupersedesOnInputDrift(t *testing.T) {
	ds := newFakeDraftStore()
	rec := newFakeRecService()
	exe := NewToolExecutor(nil, nil, rec, ds, nil, nil)
	ctx := context.Background()
	execCtx := ToolExecContext{UserID: 1, ConversationID: 10}

	first, err := exe.executeGenerateVolunteerPlanDraft(ctx, newDraftToolCall(false, map[string]any{
		"only_provinces": []string{"310000", "530000"},
	}), execCtx)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	firstOut := parseToolResult(t, first.Content)
	firstID := int64(firstOut["draft_id"].(float64))

	// 第二次：only_provinces 变了——模拟用户改主意。
	second, err := exe.executeGenerateVolunteerPlanDraft(ctx, newDraftToolCall(false, map[string]any{
		"only_provinces": []string{"230000"},
	}), execCtx)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	secondOut := parseToolResult(t, second.Content)

	if secondOut["reused"] != false {
		t.Fatalf("drifted input must NOT reuse; got %v", secondOut)
	}
	if secondOut["reused_stale"] != false {
		t.Fatalf("drifted input should produce a fresh draft (reused_stale=false); got %v", secondOut)
	}
	secondID := int64(secondOut["draft_id"].(float64))
	if secondID == firstID {
		t.Fatalf("drift must produce a NEW draft_id; got same %d", firstID)
	}

	// Recommend 必须被再次调用——这正是修复要点：复用不能掩盖真实算法重跑。
	if rec.recommendCount != 2 {
		t.Fatalf("Recommend must run twice (once per non-drifted+drifted); got %d", rec.recommendCount)
	}

	// 旧草稿必须被作废，不留 ready 残留——避免下次 reuse 误命中。
	oldDraft, err := ds.GetByID(ctx, execCtx.UserID, firstID)
	if err != nil {
		t.Fatalf("get old draft: %v", err)
	}
	if oldDraft.Status != volunteerplan.DraftStatusSuperseded {
		t.Fatalf("old draft should be superseded; got status=%s", oldDraft.Status)
	}
	if !strings.Contains(oldDraft.Error, "preferences changed") {
		t.Fatalf("superseded reason should reference preference change; got %q", oldDraft.Error)
	}

	// 新草稿是 ready，且 input 反映本次最新偏好。
	newDraft, err := ds.GetByID(ctx, execCtx.UserID, secondID)
	if err != nil {
		t.Fatalf("get new draft: %v", err)
	}
	if newDraft.Status != volunteerplan.DraftStatusReady {
		t.Fatalf("new draft should be ready; got %s", newDraft.Status)
	}
	if !strings.Contains(string(newDraft.InputJSON), `"only_provinces":["230000"]`) {
		t.Fatalf("new draft input should contain 230000; got %s", string(newDraft.InputJSON))
	}
}

// TestExecuteGenerateVolunteerPlanDraftDryRunNoStateChange 防御性测试：
// dry_run 路径不能触发 ListByConversation / Create / 任何状态突变。
func TestExecuteGenerateVolunteerPlanDraftDryRunNoStateChange(t *testing.T) {
	ds := newFakeDraftStore()
	rec := newFakeRecService()
	exe := NewToolExecutor(nil, nil, rec, ds, nil, nil)
	ctx := context.Background()
	execCtx := ToolExecContext{UserID: 1, ConversationID: 10}

	res, err := exe.executeGenerateVolunteerPlanDraft(ctx, newDraftToolCall(true, nil), execCtx)
	if err != nil {
		t.Fatalf("dry-run call: %v", err)
	}
	out := parseToolResult(t, res.Content)
	if out["status"] != "preview" {
		t.Fatalf("expected status=preview; got %v", out)
	}
	if rec.previewCount != 1 {
		t.Fatalf("Preview must run once; got %d", rec.previewCount)
	}
	if rec.recommendCount != 0 {
		t.Fatalf("Recommend must NOT run during dry_run; got %d", rec.recommendCount)
	}
	if len(ds.drafts) != 0 {
		t.Fatalf("dry_run must not create any draft; got %d", len(ds.drafts))
	}
}

// Compile-time guard: fakeDraftStore must satisfy the interface (otherwise
// new methods on DraftStore silently break tests).
var _ volunteerplan.DraftStore = (*fakeDraftStore)(nil)
var _ admission.RecommendationService = (*fakeRecService)(nil)
