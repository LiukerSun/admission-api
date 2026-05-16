package userprofile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeStore is an in-memory implementation of Store used to unit test the
// service-layer validation rules without spinning up Postgres. The store
// surface is small enough that this is cheaper to maintain than mockgen.
type fakeStore struct {
	getProfile *Profile
	getErr     error

	upsertCalled    bool
	lastReq         *UpsertRequest
	lastMarkDone    bool
	upsertReturn    *Profile
	upsertReturnErr error
}

func (f *fakeStore) GetByUserID(_ context.Context, userID int64) (*Profile, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getProfile == nil {
		return nil, ErrProfileNotFound
	}
	p := *f.getProfile
	p.UserID = userID
	return &p, nil
}

func (f *fakeStore) Upsert(_ context.Context, userID int64, req *UpsertRequest, markCompleted bool) (*Profile, error) {
	f.upsertCalled = true
	f.lastReq = req
	f.lastMarkDone = markCompleted
	if f.upsertReturnErr != nil {
		return nil, f.upsertReturnErr
	}
	if f.upsertReturn != nil {
		p := *f.upsertReturn
		p.UserID = userID
		return &p, nil
	}
	// Default: echo back what we got with a fresh timestamp.
	p := &Profile{
		UserID:              userID,
		RegionCode:          req.RegionCode,
		SubjectCategoryCode: req.SubjectCategoryCode,
		TotalScore:          req.TotalScore,
		ProvincialRank:      req.ProvincialRank,
		PlanSize:            req.PlanSize,
		PriorityStrategy:    req.PriorityStrategy,
		MathScore:           req.MathScore,
		PhysicsScore:        req.PhysicsScore,
		ChineseScore:        req.ChineseScore,
		EnglishScore:        req.EnglishScore,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	if req.Preferences != nil {
		p.Preferences = *req.Preferences
	}
	if markCompleted {
		now := time.Now()
		p.CompletedAt = &now
	}
	return p, nil
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func validRequiredRequest() *UpsertRequest {
	return &UpsertRequest{
		RegionCode:          strPtr("230000"),
		SubjectCategoryCode: strPtr(SubjectPhysics),
		TotalScore:          intPtr(620),
		ProvincialRank:      intPtr(4500),
		PlanSize:            intPtr(40),
	}
}

func TestGetMyProfile_EmptyWhenAbsent(t *testing.T) {
	svc := NewService(&fakeStore{})
	got, err := svc.GetMyProfile(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Completed {
		t.Errorf("expected completed=false for empty profile, got true")
	}
	if got.UserID != 42 {
		t.Errorf("expected user_id=42, got %d", got.UserID)
	}
}

func TestGetMyProfile_PropagatesStoreError(t *testing.T) {
	stub := &fakeStore{getErr: errors.New("db down")}
	svc := NewService(stub)
	_, err := svc.GetMyProfile(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpsertMyProfile_MarksCompletedWhenAllRequiredPresent(t *testing.T) {
	stub := &fakeStore{}
	svc := NewService(stub)
	_, err := svc.UpsertMyProfile(context.Background(), 1, validRequiredRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stub.lastMarkDone {
		t.Error("expected markCompleted=true when all 4 required scalars are filled")
	}
}

func TestUpsertMyProfile_DoesNotMarkCompletedWhenMissingScalar(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(r *UpsertRequest)
	}{
		{"missing region", func(r *UpsertRequest) { r.RegionCode = nil }},
		{"empty region", func(r *UpsertRequest) { r.RegionCode = strPtr("   ") }},
		{"missing subject", func(r *UpsertRequest) { r.SubjectCategoryCode = nil }},
		{"missing total", func(r *UpsertRequest) { r.TotalScore = nil }},
		{"missing rank", func(r *UpsertRequest) { r.ProvincialRank = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &fakeStore{}
			svc := NewService(stub)
			req := validRequiredRequest()
			c.mutate(req)
			_, err := svc.UpsertMyProfile(context.Background(), 1, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stub.lastMarkDone {
				t.Errorf("expected markCompleted=false for case %q, got true", c.name)
			}
		})
	}
}

func TestUpsertMyProfile_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(r *UpsertRequest)
		wantErr error
	}{
		{"bad region format", func(r *UpsertRequest) { r.RegionCode = strPtr("12345") }, ErrInvalidRegion},
		{"region not numeric", func(r *UpsertRequest) { r.RegionCode = strPtr("ABC123") }, ErrInvalidRegion},
		{"bad subject", func(r *UpsertRequest) { r.SubjectCategoryCode = strPtr("comprehensive") }, ErrInvalidSubject},
		{"bad strategy", func(r *UpsertRequest) { r.PriorityStrategy = strPtr("random") }, ErrInvalidStrategy},
		{"score below 0", func(r *UpsertRequest) { r.TotalScore = intPtr(-1) }, ErrScoreOutOfRange},
		{"score above 750", func(r *UpsertRequest) { r.TotalScore = intPtr(751) }, ErrScoreOutOfRange},
		{"rank below 0", func(r *UpsertRequest) { r.ProvincialRank = intPtr(-1) }, ErrRankOutOfRange},
		{"rank above 500000", func(r *UpsertRequest) { r.ProvincialRank = intPtr(500001) }, ErrRankOutOfRange},
		{"plan size 0", func(r *UpsertRequest) { r.PlanSize = intPtr(0) }, ErrPlanSizeOutOfRange},
		{"plan size 97", func(r *UpsertRequest) { r.PlanSize = intPtr(97) }, ErrPlanSizeOutOfRange},
		{"subject math 200", func(r *UpsertRequest) { r.MathScore = intPtr(200) }, ErrSubjectScoreInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeStore{})
			req := validRequiredRequest()
			tt.mutate(req)
			_, err := svc.UpsertMyProfile(context.Background(), 1, req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestUpsertMyProfile_PreferenceArrayLimits(t *testing.T) {
	tests := []struct {
		name string
		make func() *Preferences
	}{
		{
			"too many entries",
			func() *Preferences {
				return &Preferences{RequiredMajors: makeStringSlice(MaxArrayEntries + 1, "x")}
			},
		},
		{
			"entry too long",
			func() *Preferences {
				return &Preferences{PreferredMajors: []string{strings.Repeat("a", MaxArrayEntryLength+1)}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeStore{})
			req := validRequiredRequest()
			req.Preferences = tt.make()
			_, err := svc.UpsertMyProfile(context.Background(), 1, req)
			if !errors.Is(err, ErrPreferenceTooLong) {
				t.Fatalf("expected ErrPreferenceTooLong, got %v", err)
			}
		})
	}
}

func TestUpsertMyProfile_HollandCodeValidation(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr error
	}{
		{"valid uppercase", "RIA", nil},
		{"valid mixed case", "RiAs", nil},
		{"valid full", "RIASEC", nil},
		{"invalid char", "RX", ErrInvalidHollandCode},
		{"too long", "RIASECR", ErrInvalidHollandCode},
		{"digits", "R1A", ErrInvalidHollandCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeStore{})
			req := validRequiredRequest()
			req.Preferences = &Preferences{HollandCode: tt.code}
			_, err := svc.UpsertMyProfile(context.Background(), 1, req)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestUpsertMyProfile_FreeTextLimit(t *testing.T) {
	svc := NewService(&fakeStore{})
	req := validRequiredRequest()
	req.Preferences = &Preferences{CareerPlans: strings.Repeat("a", MaxFreeTextLength+1)}
	_, err := svc.UpsertMyProfile(context.Background(), 1, req)
	if !errors.Is(err, ErrPreferenceTooLong) {
		t.Fatalf("expected ErrPreferenceTooLong for long free text, got %v", err)
	}
}

func TestUpsertMyProfile_BudgetRange(t *testing.T) {
	svc := NewService(&fakeStore{})
	req := validRequiredRequest()
	req.Preferences = &Preferences{BudgetTuitionMax: intPtr(BudgetMax + 1)}
	_, err := svc.UpsertMyProfile(context.Background(), 1, req)
	if !errors.Is(err, ErrInvalidBudget) {
		t.Fatalf("expected ErrInvalidBudget, got %v", err)
	}
}

func TestUpsertMyProfile_AcceptsPartialOptionals(t *testing.T) {
	// Only required filled; all optionals nil — must succeed and mark completed.
	stub := &fakeStore{}
	svc := NewService(stub)
	_, err := svc.UpsertMyProfile(context.Background(), 1, validRequiredRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stub.upsertCalled {
		t.Error("store.Upsert should have been called")
	}
}

func makeStringSlice(n int, val string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = val
	}
	return out
}
