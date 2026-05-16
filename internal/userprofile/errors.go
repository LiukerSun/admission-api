package userprofile

import "errors"

// Domain-level errors. Handler layer maps these to HTTP statuses + error codes.
var (
	ErrProfileNotFound     = errors.New("user profile not found")
	ErrInvalidRegion       = errors.New("invalid region_code (must be 6 digits)")
	ErrInvalidSubject      = errors.New("invalid subject_category_code")
	ErrInvalidStrategy     = errors.New("invalid priority_strategy")
	ErrScoreOutOfRange     = errors.New("score out of range")
	ErrRankOutOfRange      = errors.New("provincial_rank out of range")
	ErrPlanSizeOutOfRange  = errors.New("plan_size out of range")
	ErrSubjectScoreInvalid = errors.New("subject score out of range")
	ErrPreferenceTooLong   = errors.New("preference field exceeds size limit")
	ErrInvalidHollandCode  = errors.New("invalid holland_code")
	ErrInvalidBudget       = errors.New("invalid budget_tuition_max")
)
