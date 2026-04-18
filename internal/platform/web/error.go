package web

const (
	ErrCodeUnknown      = 0
	ErrCodeBadRequest   = 1001
	ErrCodeUnauthorized = 1002
	ErrCodeForbidden    = 1003
	ErrCodeNotFound     = 1004
	ErrCodeConflict     = 1005
	ErrCodeInternal     = 5000
)

// AppError represents an application-level error.
type AppError struct {
	Code    int
	Message string
}

func NewError(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

func (e *AppError) Error() string {
	return e.Message
}
