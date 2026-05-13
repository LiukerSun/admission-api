package volunteerplan

import "errors"

var (
	ErrDraftNotFound  = errors.New("draft not found")
	ErrDraftNotReady  = errors.New("draft not ready")
	ErrPlanNotFound   = errors.New("plan not found")
)

