package sms

import "context"

// MockClient is a no-op SMS sender for local development and tests.
type MockClient struct{}

func NewMockClient() Client {
	return &MockClient{}
}

func (c *MockClient) SendVerificationCode(ctx context.Context, phone string, code string) error {
	return nil
}
