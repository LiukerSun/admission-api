package redis

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRefreshTokenManager(t *testing.T) (*RefreshTokenManager, *Client) {
	t.Helper()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client, err := New(server.Addr())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
	})

	return NewRefreshTokenManager(client, time.Hour), client
}

func TestRefreshTokenManagerRotateSingleUseAllowsOnlyOneWinner(t *testing.T) {
	manager, _ := newTestRefreshTokenManager(t)
	ctx := context.Background()

	require.NoError(t, manager.Save(ctx, "old-hash", 7, "ios"))

	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, newHash := range []string{"new-hash-a", "new-hash-b"} {
		wg.Add(1)
		go func(newHash string) {
			defer wg.Done()
			ok, err := manager.RotateSingleUse(ctx, "old-hash", newHash, 7, "ios", &RotationReplay{
				AccessToken:  "access-" + newHash,
				RefreshToken: "refresh-" + newHash,
				ExpiresIn:    60,
			})
			results <- ok
			errs <- err
		}(newHash)
	}
	wg.Wait()
	close(results)
	close(errs)

	successCount := 0
	for ok := range results {
		if ok {
			successCount++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, 1, successCount)
}

func TestRefreshTokenManagerReturnsRecentRotationReplay(t *testing.T) {
	manager, client := newTestRefreshTokenManager(t)
	ctx := context.Background()

	require.NoError(t, manager.Save(ctx, "old-hash", 7, "ios"))

	ok, err := manager.RotateSingleUse(ctx, "old-hash", "new-hash", 7, "ios", &RotationReplay{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    60,
	})
	require.NoError(t, err)
	require.True(t, ok)

	replay, err := manager.GetRotationReplay(ctx, "old-hash")
	require.NoError(t, err)
	require.NotNil(t, replay)
	assert.Equal(t, "access-token", replay.AccessToken)
	assert.Equal(t, "refresh-token", replay.RefreshToken)

	ttl, err := client.TTL(ctx, rotationReplayKey("old-hash"))
	require.NoError(t, err)
	assert.Positive(t, ttl)
}

func TestRefreshTokenManagerRevokeUserSessionsDeletesTrackedTokens(t *testing.T) {
	manager, client := newTestRefreshTokenManager(t)
	ctx := context.Background()

	require.NoError(t, manager.Save(ctx, "hash-ios", 7, "ios"))
	require.NoError(t, manager.Save(ctx, "hash-web", 7, "web"))

	require.NoError(t, manager.RevokeUserSessions(ctx, 7))

	exists, err := client.Exists(ctx, refreshTokenKey("hash-ios"), refreshTokenKey("hash-web"), userRefreshSetKey(7), userDevicesKey(7), userPlatformRefreshKey(7, "ios"), userPlatformRefreshKey(7, "web"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}
