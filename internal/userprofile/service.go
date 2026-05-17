package userprofile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"admission-api/internal/admission"
)

// Validation limits for the 4 core fields.
// Other validation rules (preferences / single-subject scores / strategy /
// holland / budget) were removed in migration 008.
const (
	ScoreMin = 0
	ScoreMax = 750
)

// region_code is a 6-digit GB/T 2260 administrative division code.
var regionCodeRe = regexp.MustCompile(`^\d{6}$`)

// Service is the userprofile business layer.
type Service interface {
	GetMyProfile(ctx context.Context, userID int64) (*ProfileResponse, error)
	UpsertMyProfile(ctx context.Context, userID int64, req *UpsertRequest) (*ProfileResponse, error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

// GetMyProfile returns the saved profile or an empty zero-state when the
// user has never filled the questionnaire. Returning an empty profile rather
// than 404 keeps the frontend free of branch logic.
func (s *service) GetMyProfile(ctx context.Context, userID int64) (*ProfileResponse, error) {
	p, err := s.store.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrProfileNotFound) {
			resp := ToResponse(EmptyProfileFor(userID))
			return &resp, nil
		}
		return nil, fmt.Errorf("get my profile: %w", err)
	}
	resp := ToResponse(p)
	return &resp, nil
}

// UpsertMyProfile validates the payload, decides whether to mark completed,
// and writes the row. ElectiveSubjects 在写入前归一化（升序去重），保证
// [biology,chemistry] 和 [chemistry,biology] 视为同值。
func (s *service) UpsertMyProfile(ctx context.Context, userID int64, req *UpsertRequest) (*ProfileResponse, error) {
	if err := s.validate(req); err != nil {
		return nil, err
	}
	req.ElectiveSubjects = admission.NormalizeElectives(req.ElectiveSubjects)
	markCompleted := allRequiredPresent(req)
	p, err := s.store.Upsert(ctx, userID, req, markCompleted)
	if err != nil {
		return nil, fmt.Errorf("upsert my profile: %w", err)
	}
	resp := ToResponse(p)
	return &resp, nil
}

// allRequiredPresent reports whether the 4 business-required fields are filled:
//
//	region + subject + elective_subjects(=2) + total_score
//
// 与 migration 008 后的数据模型一致。其余偏好由 AI 在对话中收集，不影响 completed 标记。
func allRequiredPresent(req *UpsertRequest) bool {
	if req.RegionCode == nil || strings.TrimSpace(*req.RegionCode) == "" {
		return false
	}
	if req.SubjectCategoryCode == nil || strings.TrimSpace(*req.SubjectCategoryCode) == "" {
		return false
	}
	if len(req.ElectiveSubjects) != 2 {
		return false
	}
	if req.TotalScore == nil {
		return false
	}
	return true
}

func (s *service) validate(req *UpsertRequest) error {
	// Treat whitespace-only strings as "not filled" — matches the
	// allRequiredPresent contract, so a user submitting "   " for region
	// stays in the "incomplete profile" state rather than getting a 400.
	if req.RegionCode != nil {
		rc := strings.TrimSpace(*req.RegionCode)
		if rc != "" && !regionCodeRe.MatchString(rc) {
			return ErrInvalidRegion
		}
	}
	if req.SubjectCategoryCode != nil {
		sc := strings.TrimSpace(*req.SubjectCategoryCode)
		if sc != "" {
			switch sc {
			case SubjectPhysics, SubjectHistory:
			default:
				return ErrInvalidSubject
			}
		}
	}
	if len(req.ElectiveSubjects) > 0 {
		if err := validateElectiveSubjects(req.ElectiveSubjects); err != nil {
			return err
		}
	}
	if req.TotalScore != nil {
		if *req.TotalScore < ScoreMin || *req.TotalScore > ScoreMax {
			return ErrScoreOutOfRange
		}
	}
	return nil
}

// validateElectiveSubjects 校验再选科目：长度=2 + 元素 ∈ 枚举 + 不重复。
// 归一化排序由 service 层在写入前调用 admission.NormalizeElectives 完成。
func validateElectiveSubjects(es []string) error {
	if len(es) != 2 {
		return ErrInvalidElectiveSet
	}
	seen := make(map[string]struct{}, 2)
	for _, code := range es {
		if !admission.IsValidElectiveCode(code) {
			return ErrInvalidElectiveSet
		}
		if _, dup := seen[code]; dup {
			return ErrInvalidElectiveSet
		}
		seen[code] = struct{}{}
	}
	return nil
}
