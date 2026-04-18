package redis

import (
	"context"
	"fmt"
	"time"
)

type RefreshTokenManager struct {
	client *Client
	ttl    time.Duration
}

func NewRefreshTokenManager(client *Client, ttl time.Duration) *RefreshTokenManager {
	return &RefreshTokenManager{
		client: client,
		ttl:    ttl,
	}
}

func (m *RefreshTokenManager) Save(ctx context.Context, tokenHash string, userID int64, platform string) error {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	value := fmt.Sprintf("%d:%s", userID, platform)

	if err := m.client.Set(ctx, key, value, m.ttl); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}

	deviceKey := fmt.Sprintf("user:%d:devices", userID)
	_ = m.client.SAdd(ctx, deviceKey, platform)

	return nil
}

func (m *RefreshTokenManager) Verify(ctx context.Context, tokenHash string, userID int64, platform string) (bool, error) {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	value, err := m.client.Get(ctx, key)
	if err != nil {
		return false, nil
	}

	expected := fmt.Sprintf("%d:%s", userID, platform)
	return value == expected, nil
}

func (m *RefreshTokenManager) Delete(ctx context.Context, tokenHash string) error {
	key := fmt.Sprintf("refresh:%s", tokenHash)
	return m.client.Del(ctx, key)
}

func (m *RefreshTokenManager) Rotate(ctx context.Context, oldHash, newHash string, userID int64, platform string) error {
	if err := m.Save(ctx, newHash, userID, platform); err != nil {
		return fmt.Errorf("save new refresh token: %w", err)
	}

	if err := m.Delete(ctx, oldHash); err != nil {
		return fmt.Errorf("delete old refresh token: %w", err)
	}

	return nil
}
