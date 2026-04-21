package sms

import "context"

// Client sends SMS verification codes.
type Client interface {
	SendVerificationCode(ctx context.Context, phone string, code string) error
}
