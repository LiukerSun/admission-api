package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DATABASE_URL", "postgres://db")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "JWT_SECRET is required")
}

func TestLoadUsesDatabaseURLAndRedisAddrOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("DATABASE_URL", "postgres://override")
	t.Setenv("REDIS_ADDR", "redis.example:6379")
	t.Setenv("MOCK_CALLBACK_SECRET", "mock-secret")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "postgres://override", cfg.DatabaseURL)
	assert.Equal(t, "redis.example:6379", cfg.RedisAddr)
	assert.Equal(t, "mock-secret", cfg.MockCallbackSecret)
}

func TestLoadBuildsFallbackURLsFromComponents(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("POSTGRES_HOST", "postgres.internal")
	t.Setenv("POSTGRES_USER", "appuser")
	t.Setenv("POSTGRES_PASSWORD", "apppass")
	t.Setenv("POSTGRES_DB", "admission_test")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("REDIS_PORT", "6381")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "postgres://appuser:apppass@postgres.internal:5433/admission_test?sslmode=disable", cfg.DatabaseURL)
	assert.Equal(t, "localhost:6381", cfg.RedisAddr)
}
