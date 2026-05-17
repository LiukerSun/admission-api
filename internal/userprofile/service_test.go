package userprofile

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeStore is an in-memory implementation of Store used to unit test the
// service-layer validation rules without spinning up Postgres.
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
		ElectiveSubjects:    req.ElectiveSubjects,
		TotalScore:          req.TotalScore,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
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
		ElectiveSubjects:    []string{"biology", "chemistry"},
		TotalScore:          intPtr(620),
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
		t.Error("expected markCompleted=true when all 4 required fields are filled")
	}
}

func TestUpsertMyProfile_NormalizesElectiveSubjects(t *testing.T) {
	stub := &fakeStore{}
	svc := NewService(stub)
	req := validRequiredRequest()
	req.ElectiveSubjects = []string{"chemistry", "biology"} // 倒序
	_, err := svc.UpsertMyProfile(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// service 在调 store 前归一化为升序
	if got := stub.lastReq.ElectiveSubjects; len(got) != 2 || got[0] != "biology" || got[1] != "chemistry" {
		t.Errorf("expected normalized [biology chemistry], got %v", got)
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
		{"missing electives", func(r *UpsertRequest) { r.ElectiveSubjects = nil }},
		{"missing total", func(r *UpsertRequest) { r.TotalScore = nil }},
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
		{"score below 0", func(r *UpsertRequest) { r.TotalScore = intPtr(-1) }, ErrScoreOutOfRange},
		{"score above 750", func(r *UpsertRequest) { r.TotalScore = intPtr(751) }, ErrScoreOutOfRange},
		{"electives length=1", func(r *UpsertRequest) { r.ElectiveSubjects = []string{"biology"} }, ErrInvalidElectiveSet},
		{"electives length=3", func(r *UpsertRequest) {
			r.ElectiveSubjects = []string{"biology", "chemistry", "geography"}
		}, ErrInvalidElectiveSet},
		{"electives unknown code", func(r *UpsertRequest) { r.ElectiveSubjects = []string{"biology", "english"} }, ErrInvalidElectiveSet},
		{"electives duplicate", func(r *UpsertRequest) { r.ElectiveSubjects = []string{"biology", "biology"} }, ErrInvalidElectiveSet},
		{"electives physics in elective list", func(r *UpsertRequest) {
			r.ElectiveSubjects = []string{"physics", "biology"}
		}, ErrInvalidElectiveSet},
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
