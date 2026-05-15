package sms

import (
	"context"
	"log/slog"
)

// MockClient is a no-op SMS sender for local development and tests. The code
// is logged so a developer can copy it out of the server log during manual
// testing — the real Aliyun client (aliyun.go) never logs codes.
type MockClient struct{}

func NewMockClient() Client {
	return &MockClient{}
}

func (c *MockClient) SendVerificationCode(ctx context.Context, phone, code string) error {
	slog.Info("[MockSMS] verification code generated", "phone", phone, "code", code)
	return nil
}
