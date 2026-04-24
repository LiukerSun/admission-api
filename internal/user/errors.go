package user

import "errors"

var (
	ErrUserNotFound             = errors.New("user not found")
	ErrEmailAlreadyExists       = errors.New("email already exists")
	ErrPhoneAlreadyExists       = errors.New("phone already exists")
	ErrBindingNotFound          = errors.New("binding not found")
	ErrStudentNotFound          = errors.New("student not found")
	ErrCannotBindSelf           = errors.New("cannot bind yourself")
	ErrStudentAlreadyBound      = errors.New("student already bound to another parent")
	ErrPhoneInvalid             = errors.New("invalid phone number")
	ErrPhoneCodeTooFrequent     = errors.New("verification code sent too frequently")
	ErrPhoneCodeDailyLimit      = errors.New("verification code daily limit exceeded")
	ErrVerificationCodeInvalid  = errors.New("invalid verification code")
	ErrVerificationCodeExpired  = errors.New("verification code not found or expired")
	ErrVerificationCodeExceeded = errors.New("verification code attempts exceeded")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrUserNotFound)
}
