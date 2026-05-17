package lookup

import (
	"context"
	"errors"
	"testing"
)

// fakeStore drives the year-walk and below-min fallback flows.
// 每个 (year, region, subject) 切片用一份预设回答；如果切片缺失则 ErrNotFound。
type fakeStore struct {
	// rankFloor: key=year, returns matchedScore + rank for the given userScore.
	// 用闭包是为了让单测描述更接近真实「向下取整」语义。
	rankFloor    map[int]func(userScore int) (int, int, error)
	minScoreRank map[int]func() (int, int, error)
	planSize     map[int]int // key=year → size; 缺失则 ErrNotFound
}

func (f *fakeStore) RankFloor(_ context.Context, year int, _, _ string, userScore int) (matchedScore, rank int, err error) {
	fn, ok := f.rankFloor[year]
	if !ok {
		return 0, 0, ErrNotFound
	}
	return fn(userScore)
}

func (f *fakeStore) MinScoreRank(_ context.Context, year int, _, _ string) (minScore, rank int, err error) {
	fn, ok := f.minScoreRank[year]
	if !ok {
		return 0, 0, ErrNotFound
	}
	return fn()
}

func (f *fakeStore) PlanSize(_ context.Context, year int, _, _ string) (int, error) {
	v, ok := f.planSize[year]
	if !ok {
		return 0, ErrNotFound
	}
	return v, nil
}

func TestLookupRank_ExactMatch(t *testing.T) {
	store := &fakeStore{
		rankFloor: map[int]func(int) (int, int, error){
			2025: func(userScore int) (int, int, error) {
				// 600 精确命中 → matchedScore=600, rank=5997
				if userScore == 600 {
					return 600, 5997, nil
				}
				return 0, 0, ErrNotFound
			},
		},
	}
	svc := NewService(store)

	res, err := svc.LookupRank(context.Background(), 2025, "230000", "physics", 600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Rank != 5997 {
		t.Errorf("Rank: got %d, want 5997", res.Rank)
	}
	if res.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d, want 2025", res.YearUsed)
	}
	if res.MatchedScore != 600 {
		t.Errorf("MatchedScore: got %d, want 600", res.MatchedScore)
	}
	if res.Source != RankSourceExact {
		t.Errorf("Source: got %s, want exact", res.Source)
	}
}

func TestLookupRank_FlooredMatch(t *testing.T) {
	// 用户考 605.5 → 表中没有 605.5；返回 score=605 的累计位次
	// 模拟：rankFloor 返回 matchedScore != userScore
	store := &fakeStore{
		rankFloor: map[int]func(int) (int, int, error){
			2025: func(_ int) (int, int, error) { return 605, 5206, nil },
		},
	}
	svc := NewService(store)

	res, err := svc.LookupRank(context.Background(), 2025, "230000", "physics", 606)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Source != RankSourceFloored {
		t.Errorf("Source: got %s, want floored", res.Source)
	}
	if res.MatchedScore != 605 {
		t.Errorf("MatchedScore: got %d, want 605", res.MatchedScore)
	}
	if res.Rank != 5206 {
		t.Errorf("Rank: got %d, want 5206", res.Rank)
	}
}

func TestLookupRank_BelowMin(t *testing.T) {
	// 用户考 100，表中最低分 130，rankFloor 返回 ErrNotFound；
	// MinScoreRank 命中 → 给出 minScore=130, rank=117407
	store := &fakeStore{
		rankFloor: map[int]func(int) (int, int, error){
			2025: func(_ int) (int, int, error) { return 0, 0, ErrNotFound },
		},
		minScoreRank: map[int]func() (int, int, error){
			2025: func() (int, int, error) { return 130, 117407, nil },
		},
	}
	svc := NewService(store)

	res, err := svc.LookupRank(context.Background(), 2025, "230000", "physics", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Source != RankSourceBelowMin {
		t.Errorf("Source: got %s, want below_min", res.Source)
	}
	if res.MatchedScore != 130 {
		t.Errorf("MatchedScore: got %d, want 130", res.MatchedScore)
	}
	if res.Rank != 117407 {
		t.Errorf("Rank: got %d, want 117407", res.Rank)
	}
	if res.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d, want 2025", res.YearUsed)
	}
}

func TestLookupRank_YearWalkOnePrev(t *testing.T) {
	// 当年 2026 数据完全缺失（rankFloor 和 MinScoreRank 都 ErrNotFound）；
	// 回退到 2025 命中。Source 必须是 prev_year（即使是 floored 命中也覆盖标 prev_year）。
	store := &fakeStore{
		rankFloor: map[int]func(int) (int, int, error){
			2025: func(_ int) (int, int, error) { return 600, 5997, nil },
		},
	}
	svc := NewService(store)

	res, err := svc.LookupRank(context.Background(), 2026, "230000", "physics", 600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Source != RankSourcePrevYear {
		t.Errorf("Source: got %s, want prev_year", res.Source)
	}
	if res.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d, want 2025", res.YearUsed)
	}
	if res.Rank != 5997 {
		t.Errorf("Rank: got %d, want 5997", res.Rank)
	}
}

func TestLookupRank_YearWalkAndBelowMin(t *testing.T) {
	// 当年完全缺失，回退到 prev 年 below_min 兜底。
	// review 共识：标 prev_year，因为前端最该提示「不是本年数据」。
	store := &fakeStore{
		minScoreRank: map[int]func() (int, int, error){
			2025: func() (int, int, error) { return 130, 117407, nil },
		},
	}
	svc := NewService(store)

	res, err := svc.LookupRank(context.Background(), 2026, "230000", "physics", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Source != RankSourcePrevYear {
		t.Errorf("Source: got %s, want prev_year", res.Source)
	}
	if res.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d, want 2025", res.YearUsed)
	}
}

func TestLookupRank_NoDataAfterWalkback(t *testing.T) {
	// 当年 + 前一年都完全空。
	store := &fakeStore{}
	svc := NewService(store)

	_, err := svc.LookupRank(context.Background(), 2026, "230000", "physics", 600)
	if !errors.Is(err, ErrRankNotAvailable) {
		t.Fatalf("err: got %v, want ErrRankNotAvailable", err)
	}
}

func TestLookupRank_StoreErrorBubblesUp(t *testing.T) {
	// 非 ErrNotFound 的错误必须直接返回，不能被 fallback 掉。
	dbErr := errors.New("connection refused")
	store := &fakeStore{
		rankFloor: map[int]func(int) (int, int, error){
			2026: func(_ int) (int, int, error) { return 0, 0, dbErr },
		},
	}
	svc := NewService(store)

	_, err := svc.LookupRank(context.Background(), 2026, "230000", "physics", 600)
	if err == nil {
		t.Fatal("expected DB error to bubble up, got nil")
	}
	if errors.Is(err, ErrRankNotAvailable) {
		t.Fatal("DB error must not be masked as ErrRankNotAvailable")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected wrapped dbErr, got %v", err)
	}
}

func TestLookupPlanSize_HitAndMiss(t *testing.T) {
	store := &fakeStore{planSize: map[int]int{2025: 40}}
	svc := NewService(store)

	// 命中
	size, err := svc.LookupPlanSize(context.Background(), 2025, "230000", "physics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 40 {
		t.Errorf("size: got %d, want 40", size)
	}

	// 缺失 → 默认值
	size, err = svc.LookupPlanSize(context.Background(), 2099, "230000", "physics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != DefaultPlanSize {
		t.Errorf("size: got %d, want DefaultPlanSize=%d", size, DefaultPlanSize)
	}
}

func TestLookupPlanSize_DBError(t *testing.T) {
	// 非 ErrNotFound 错误必须返回，不能默默吞成默认值。
	dbErr := errors.New("connection refused")
	store := &errPlanSizeStore{err: dbErr}
	svc := NewService(store)

	_, err := svc.LookupPlanSize(context.Background(), 2025, "230000", "physics")
	if err == nil {
		t.Fatal("expected DB error, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected wrapped dbErr, got %v", err)
	}
}

// errPlanSizeStore returns the supplied error on PlanSize calls; other methods
// behave as the empty store. Used to test that non-ErrNotFound errors are
// not swallowed by the default fallback.
type errPlanSizeStore struct {
	err error
}

func (s *errPlanSizeStore) RankFloor(_ context.Context, _ int, _, _ string, _ int) (matchedScore, rank int, err error) {
	return 0, 0, ErrNotFound
}

func (s *errPlanSizeStore) MinScoreRank(_ context.Context, _ int, _, _ string) (minScore, rank int, err error) {
	return 0, 0, ErrNotFound
}

func (s *errPlanSizeStore) PlanSize(_ context.Context, _ int, _, _ string) (int, error) {
	return 0, s.err
}
