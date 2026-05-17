package snapshot

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"admission-api/internal/lookup"
	"admission-api/internal/userprofile"
)

// fakeProfile 直接返回固定 ProfileResponse；err != nil 时优先返回错误。
type fakeProfile struct {
	resp *userprofile.ProfileResponse
	err  error
}

func (f *fakeProfile) GetMyProfile(_ context.Context, userID int64) (*userprofile.ProfileResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.resp == nil {
		// 模仿真实 service：未填写 → 返回空 profile，不 404
		return &userprofile.ProfileResponse{Profile: userprofile.Profile{UserID: userID}}, nil
	}
	r := *f.resp
	r.UserID = userID
	return &r, nil
}

// fakeLookup 给 LookupRank / LookupPlanSize 返回预设值。
type fakeLookup struct {
	rankRes   lookup.RankResult
	rankErr   error
	planSize  int
	planErr   error
	rankCalls []rankCall
	planCalls []planCall
}

type rankCall struct {
	year       int
	region     string
	subject    string
	totalScore int
}

type planCall struct {
	year    int
	region  string
	subject string
}

func (f *fakeLookup) LookupRank(_ context.Context, year int, region, subject string, score int) (lookup.RankResult, error) {
	f.rankCalls = append(f.rankCalls, rankCall{year, region, subject, score})
	return f.rankRes, f.rankErr
}

func (f *fakeLookup) LookupPlanSize(_ context.Context, year int, region, subject string) (int, error) {
	f.planCalls = append(f.planCalls, planCall{year, region, subject})
	return f.planSize, f.planErr
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func validProfile() *userprofile.ProfileResponse {
	return &userprofile.ProfileResponse{
		Profile: userprofile.Profile{
			RegionCode:          strPtr("230000"),
			SubjectCategoryCode: strPtr("physics"),
			ElectiveSubjects:    []string{"biology", "chemistry"},
			TotalScore:          intPtr(620),
		},
		Completed: true,
	}
}

func fixedYear(y int) YearProvider {
	return func(_ context.Context) (int, error) { return y, nil }
}

func TestBuildSnapshot_HappyPath(t *testing.T) {
	prof := &fakeProfile{resp: validProfile()}
	look := &fakeLookup{
		rankRes: lookup.RankResult{
			Rank:         3304,
			YearUsed:     2025,
			MatchedScore: 620,
			Source:       lookup.RankSourceExact,
		},
		planSize: 40,
	}
	svc := NewService(prof, look, fixedYear(2025))

	snap, err := svc.BuildRecommendationSnapshot(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.RegionCode != "230000" {
		t.Errorf("RegionCode: got %q", snap.RegionCode)
	}
	if snap.SubjectCategoryCode != "physics" {
		t.Errorf("SubjectCategoryCode: got %q", snap.SubjectCategoryCode)
	}
	if !reflect.DeepEqual(snap.ElectiveSubjects, []string{"biology", "chemistry"}) {
		t.Errorf("ElectiveSubjects: got %v", snap.ElectiveSubjects)
	}
	if snap.TotalScore != 620 {
		t.Errorf("TotalScore: got %d", snap.TotalScore)
	}
	if snap.ProvincialRank != 3304 {
		t.Errorf("ProvincialRank: got %d", snap.ProvincialRank)
	}
	if snap.PlanSize != 40 {
		t.Errorf("PlanSize: got %d", snap.PlanSize)
	}
	if snap.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d", snap.YearUsed)
	}
	if snap.RankSource != lookup.RankSourceExact {
		t.Errorf("RankSource: got %s", snap.RankSource)
	}

	// lookup 应该被恰好调用一次，且使用 YearProvider 给的年份
	if len(look.rankCalls) != 1 || look.rankCalls[0].year != 2025 || look.rankCalls[0].totalScore != 620 {
		t.Errorf("rankCalls unexpected: %+v", look.rankCalls)
	}
	if len(look.planCalls) != 1 || look.planCalls[0].year != 2025 {
		t.Errorf("planCalls unexpected: %+v", look.planCalls)
	}
}

func TestBuildSnapshot_IncompleteProfile(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *userprofile.Profile)
	}{
		{"missing region", func(p *userprofile.Profile) { p.RegionCode = nil }},
		{"empty region", func(p *userprofile.Profile) { p.RegionCode = strPtr("   ") }},
		{"missing subject", func(p *userprofile.Profile) { p.SubjectCategoryCode = nil }},
		{"missing total_score", func(p *userprofile.Profile) { p.TotalScore = nil }},
		{"missing electives", func(p *userprofile.Profile) { p.ElectiveSubjects = nil }},
		{"only 1 elective", func(p *userprofile.Profile) { p.ElectiveSubjects = []string{"biology"} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := validProfile()
			c.mutate(&resp.Profile)
			prof := &fakeProfile{resp: resp}
			look := &fakeLookup{} // 不应被调用
			svc := NewService(prof, look, fixedYear(2025))

			_, err := svc.BuildRecommendationSnapshot(context.Background(), 1)
			if !errors.Is(err, ErrProfileIncomplete) {
				t.Fatalf("want ErrProfileIncomplete, got %v", err)
			}
			if len(look.rankCalls) > 0 {
				t.Error("lookup should not be called when profile is incomplete")
			}
		})
	}
}

func TestBuildSnapshot_TranslatesRankNotAvailable(t *testing.T) {
	prof := &fakeProfile{resp: validProfile()}
	look := &fakeLookup{rankErr: lookup.ErrRankNotAvailable}
	svc := NewService(prof, look, fixedYear(2025))

	_, err := svc.BuildRecommendationSnapshot(context.Background(), 1)
	if !errors.Is(err, ErrRankDataMissing) {
		t.Fatalf("want ErrRankDataMissing, got %v", err)
	}
}

func TestBuildSnapshot_BubblesUpUnexpectedRankError(t *testing.T) {
	dbErr := errors.New("connection refused")
	prof := &fakeProfile{resp: validProfile()}
	look := &fakeLookup{rankErr: dbErr}
	svc := NewService(prof, look, fixedYear(2025))

	_, err := svc.BuildRecommendationSnapshot(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrRankDataMissing) {
		t.Fatal("non-ErrRankNotAvailable must not be translated to ErrRankDataMissing")
	}
	if errors.Is(err, ErrProfileIncomplete) {
		t.Fatal("non-ErrRankNotAvailable must not be translated to ErrProfileIncomplete")
	}
}

func TestBuildSnapshot_YearProviderError(t *testing.T) {
	yp := func(_ context.Context) (int, error) { return 0, errors.New("config missing") }
	svc := NewService(&fakeProfile{resp: validProfile()}, &fakeLookup{}, yp)

	_, err := svc.BuildRecommendationSnapshot(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildSnapshot_PrevYearSourcePropagated(t *testing.T) {
	// 当 year-walk 命中前一年时，snapshot 应保留 YearUsed=year-1 + Source=prev_year。
	prof := &fakeProfile{resp: validProfile()}
	look := &fakeLookup{
		rankRes: lookup.RankResult{
			Rank:         3304,
			YearUsed:     2025,
			MatchedScore: 620,
			Source:       lookup.RankSourcePrevYear,
		},
		planSize: 40,
	}
	svc := NewService(prof, look, fixedYear(2026))

	snap, err := svc.BuildRecommendationSnapshot(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.YearUsed != 2025 {
		t.Errorf("YearUsed: got %d, want 2025", snap.YearUsed)
	}
	if snap.RankSource != lookup.RankSourcePrevYear {
		t.Errorf("RankSource: got %s, want prev_year", snap.RankSource)
	}
}
