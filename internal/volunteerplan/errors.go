package volunteerplan

import "errors"

var (
	ErrDraftNotFound = errors.New("draft not found")
	ErrDraftNotReady = errors.New("draft not ready")
	// ErrDraftAlreadyAdopted is returned when adopt is attempted on a
	// draft that has already been adopted (idempotency / double-click guard).
	ErrDraftAlreadyAdopted = errors.New("draft already adopted")
	// ErrDraftCorrupted is returned when a draft's plan_json payload is
	// missing or fails json validation — usually a sign the generation
	// step failed mid-flight and the user should regenerate.
	ErrDraftCorrupted = errors.New("draft plan data is corrupted")
	// ErrDraftNotInExpectedState is the low-level sentinel returned by
	// the store's state-machine guards when an UPDATE WHERE status='...'
	// matches zero rows. The service layer maps it to the more specific
	// ErrDraftNotReady / ErrDraftAlreadyAdopted depending on the
	// transition that was attempted.
	ErrDraftNotInExpectedState = errors.New("draft not in expected state")
	ErrPlanNotFound            = errors.New("plan not found")
	// ErrInvalidPlanTitle: title 必须非空（trim 后）。description 没有这种限制，
	// 允许用户主动清空备注。
	ErrInvalidPlanTitle = errors.New("plan title cannot be empty")
)
