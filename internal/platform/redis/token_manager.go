package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type RefreshTokenManager struct {
	client    *Client
	ttl       time.Duration
	replayTTL time.Duration
	usedTTL   time.Duration
}

func NewRefreshTokenManager(client *Client, ttl time.Duration) *RefreshTokenManager {
	return &RefreshTokenManager{
		client:    client,
		ttl:       ttl,
		replayTTL: 5 * time.Second,
		usedTTL:   time.Minute,
	}
}

type RotationReplay struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (m *RefreshTokenManager) Save(ctx context.Context, tokenHash string, userID int64, platform string) error {
	value := refreshTokenValue(userID, platform)
	_, err := m.client.Eval(ctx, saveRefreshTokenScript, []string{
		refreshTokenKey(tokenHash),
		userDevicesKey(userID),
		userRefreshSetKey(userID),
		userPlatformRefreshKey(userID, platform),
	}, value, m.ttl.Milliseconds(), platform, tokenHash)
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

func (m *RefreshTokenManager) Verify(ctx context.Context, tokenHash string, userID int64, platform string) (bool, error) {
	value, err := m.client.Get(ctx, refreshTokenKey(tokenHash))
	if err != nil {
		return false, nil
	}

	expected := refreshTokenValue(userID, platform)
	return value == expected, nil
}

func (m *RefreshTokenManager) Delete(ctx context.Context, tokenHash string) error {
	return m.client.Del(ctx, refreshTokenKey(tokenHash))
}

func (m *RefreshTokenManager) Rotate(ctx context.Context, oldHash, newHash string, userID int64, platform string) error {
	ok, err := m.RotateSingleUse(ctx, oldHash, newHash, userID, platform, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("refresh token is invalid or already rotated")
	}
	return nil
}

func (m *RefreshTokenManager) RotateSingleUse(ctx context.Context, oldHash, newHash string, userID int64, platform string, replay *RotationReplay) (bool, error) {
	expected := refreshTokenValue(userID, platform)
	replayPayload := ""
	if replay != nil {
		payloadBytes, err := json.Marshal(replay)
		if err != nil {
			return false, fmt.Errorf("marshal rotation replay: %w", err)
		}
		replayPayload = string(payloadBytes)
	}
	result, err := m.client.Eval(ctx, rotateRefreshTokenScript, []string{
		refreshTokenKey(oldHash),
		refreshTokenKey(newHash),
		userDevicesKey(userID),
		userRefreshSetKey(userID),
		userPlatformRefreshKey(userID, platform),
		rotationReplayKey(oldHash),
		rotationUsedKey(oldHash),
	}, expected, platform, expected, m.ttl.Milliseconds(), oldHash, newHash, replayPayload, m.replayTTL.Milliseconds(), m.usedTTL.Milliseconds())
	if err != nil {
		return false, fmt.Errorf("rotate refresh token: %w", err)
	}

	rotated, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("rotate refresh token: unexpected result %T", result)
	}
	return rotated == 1, nil
}

func (m *RefreshTokenManager) GetRotationReplay(ctx context.Context, oldHash string) (*RotationReplay, error) {
	payload, err := m.client.Get(ctx, rotationReplayKey(oldHash))
	if err != nil {
		return nil, nil
	}

	var replay RotationReplay
	if err := json.Unmarshal([]byte(payload), &replay); err != nil {
		return nil, fmt.Errorf("decode rotation replay: %w", err)
	}
	return &replay, nil
}

func (m *RefreshTokenManager) WasRotated(ctx context.Context, oldHash string) (bool, error) {
	exists, err := m.client.Exists(ctx, rotationUsedKey(oldHash))
	if err != nil {
		return false, fmt.Errorf("check rotation-used marker: %w", err)
	}
	return exists > 0, nil
}

func (m *RefreshTokenManager) RevokeUserSessions(ctx context.Context, userID int64) error {
	sessionHashes, err := m.client.SMembers(ctx, userRefreshSetKey(userID))
	if err != nil {
		return fmt.Errorf("get refresh token hashes: %w", err)
	}

	platforms, err := m.client.SMembers(ctx, userDevicesKey(userID))
	if err != nil {
		return fmt.Errorf("get user devices: %w", err)
	}

	keys := make([]string, 0, len(sessionHashes)+len(platforms)+2)
	for _, hash := range sessionHashes {
		keys = append(keys, refreshTokenKey(hash))
	}
	for _, platform := range platforms {
		keys = append(keys, userPlatformRefreshKey(userID, platform))
	}
	keys = append(keys, userRefreshSetKey(userID), userDevicesKey(userID))

	if err := m.client.Del(ctx, keys...); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}

	return nil
}

func refreshTokenKey(tokenHash string) string {
	return fmt.Sprintf("refresh:%s", tokenHash)
}

func userDevicesKey(userID int64) string {
	return fmt.Sprintf("user:%d:devices", userID)
}

func userRefreshSetKey(userID int64) string {
	return fmt.Sprintf("user:%d:refresh_tokens", userID)
}

func userPlatformRefreshKey(userID int64, platform string) string {
	return fmt.Sprintf("user:%d:platform:%s:refresh", userID, platform)
}

func rotationReplayKey(oldHash string) string {
	return fmt.Sprintf("refresh_rotation:%s", oldHash)
}

func rotationUsedKey(oldHash string) string {
	return fmt.Sprintf("refresh_used:%s", oldHash)
}

func refreshTokenValue(userID int64, platform string) string {
	return fmt.Sprintf("%d:%s", userID, platform)
}

const saveRefreshTokenScript = `
local existingHash = redis.call("GET", KEYS[4])
if existingHash and existingHash ~= ARGV[4] then
	redis.call("DEL", "refresh:" .. existingHash)
	redis.call("SREM", KEYS[3], existingHash)
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
redis.call("SADD", KEYS[2], ARGV[3])
redis.call("SADD", KEYS[3], ARGV[4])
redis.call("SET", KEYS[4], ARGV[4], "PX", ARGV[2])
return 1
`

const rotateRefreshTokenScript = `
local current = redis.call("GET", KEYS[1])
if current ~= ARGV[1] then
	return 0
end
redis.call("DEL", KEYS[1])
redis.call("SREM", KEYS[4], ARGV[5])
redis.call("SET", KEYS[2], ARGV[3], "PX", ARGV[4])
redis.call("SADD", KEYS[3], ARGV[2])
redis.call("SADD", KEYS[4], ARGV[6])
redis.call("SET", KEYS[5], ARGV[6], "PX", ARGV[4])
if ARGV[7] ~= "" then
	redis.call("SET", KEYS[6], ARGV[7], "PX", ARGV[8])
end
redis.call("SET", KEYS[7], "1", "PX", ARGV[9])
return 1
`
