package userprofile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Validation limits. Keep arrays small and entries short — they all flow
// into the LLM prompt context, and a runaway profile could blow past the
// agent's iteration budget.
const (
	MaxArrayEntries      = 16
	MaxArrayEntryLength  = 32
	MaxFreeTextLength    = 500
	MaxHollandCodeLength = 6
	ScoreMin             = 0
	ScoreMax             = 750
	SubjectScoreMax      = 150
	RankMin              = 0
	RankMax              = 500000
	PlanSizeMin          = 1
	PlanSizeMax          = 96
	BudgetMin            = 0
	BudgetMax            = 1000000 // 100w/yr — generous upper bound
)

var (
	// region_code is a 6-digit GB/T 2260 administrative division code.
	regionCodeRe = regexp.MustCompile(`^\d{6}$`)
	// holland_code is a 1-6 char subset of RIASEC, e.g. "RIA".
	hollandCodeRe = regexp.MustCompile(`^[RIASECriasec]{1,6}$`)
)

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
// and writes the row.
func (s *service) UpsertMyProfile(ctx context.Context, userID int64, req *UpsertRequest) (*ProfileResponse, error) {
	if err := s.validate(req); err != nil {
		return nil, err
	}
	markCompleted := allRequiredPresent(req)
	p, err := s.store.Upsert(ctx, userID, req, markCompleted)
	if err != nil {
		return nil, fmt.Errorf("upsert my profile: %w", err)
	}
	resp := ToResponse(p)
	return &resp, nil
}

// allRequiredPresent reports whether the 4 business-required fields are filled.
// plan_size is intentionally optional here — agent.go has its own default 40.
func allRequiredPresent(req *UpsertRequest) bool {
	if req.RegionCode == nil || strings.TrimSpace(*req.RegionCode) == "" {
		return false
	}
	if req.SubjectCategoryCode == nil || strings.TrimSpace(*req.SubjectCategoryCode) == "" {
		return false
	}
	if req.TotalScore == nil {
		return false
	}
	if req.ProvincialRank == nil {
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
	if req.PriorityStrategy != nil {
		ps := strings.TrimSpace(*req.PriorityStrategy)
		if ps != "" {
			switch ps {
			case StrategyAuto, StrategySchool, StrategyMajor:
			default:
				return ErrInvalidStrategy
			}
		}
	}
	if req.TotalScore != nil {
		if *req.TotalScore < ScoreMin || *req.TotalScore > ScoreMax {
			return ErrScoreOutOfRange
		}
	}
	if req.ProvincialRank != nil {
		if *req.ProvincialRank < RankMin || *req.ProvincialRank > RankMax {
			return ErrRankOutOfRange
		}
	}
	if req.PlanSize != nil {
		if *req.PlanSize < PlanSizeMin || *req.PlanSize > PlanSizeMax {
			return ErrPlanSizeOutOfRange
		}
	}
	for _, v := range []*int{req.MathScore, req.PhysicsScore, req.ChineseScore, req.EnglishScore} {
		if v == nil {
			continue
		}
		if *v < ScoreMin || *v > SubjectScoreMax {
			return ErrSubjectScoreInvalid
		}
	}
	if req.Preferences != nil {
		if err := validatePreferences(req.Preferences); err != nil {
			return err
		}
	}
	return nil
}

func validatePreferences(p *Preferences) error {
	groups := [][]string{
		p.RequiredMajors,
		p.PreferredMajors,
		p.ExcludedMajors,
		p.ExcludedKeywords,
		p.PreferredCities,
		p.ExcludedCities,
		p.PreferredProvinces,
		p.ExcludedProvinces,
	}
	for _, g := range groups {
		if err := checkStringArray(g); err != nil {
			return err
		}
	}
	if p.HollandCode != "" {
		if utf8.RuneCountInString(p.HollandCode) > MaxHollandCodeLength {
			return ErrInvalidHollandCode
		}
		if !hollandCodeRe.MatchString(p.HollandCode) {
			return ErrInvalidHollandCode
		}
	}
	if err := checkFreeText(p.FamilyResources); err != nil {
		return err
	}
	if err := checkFreeText(p.FamilyEconomy); err != nil {
		return err
	}
	if err := checkFreeText(p.CareerPlans); err != nil {
		return err
	}
	if p.BudgetTuitionMax != nil {
		if *p.BudgetTuitionMax < BudgetMin || *p.BudgetTuitionMax > BudgetMax {
			return ErrInvalidBudget
		}
	}
	return nil
}

func checkStringArray(items []string) error {
	if len(items) > MaxArrayEntries {
		return ErrPreferenceTooLong
	}
	for _, s := range items {
		if utf8.RuneCountInString(s) > MaxArrayEntryLength {
			return ErrPreferenceTooLong
		}
	}
	return nil
}

func checkFreeText(s string) error {
	if utf8.RuneCountInString(s) > MaxFreeTextLength {
		return ErrPreferenceTooLong
	}
	return nil
}
