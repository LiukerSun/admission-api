package integration

import (
	"context"
	"testing"
	"time"

	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func createIntegrationUser(t *testing.T, database *db.DB, email, userType string) int64 {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	var userID int64
	err = database.Pool().QueryRow(
		context.Background(),
		`INSERT INTO users (email, password_hash, role, user_type, status) VALUES ($1, $2, 'user', $3, 'active') RETURNING id`,
		email,
		string(passwordHash),
		userType,
	).Scan(&userID)
	require.NoError(t, err)

	return userID
}

func issueAccessToken(t *testing.T, cfg *config.Config, userID int64, role, userType string) string {
	t.Helper()

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  time.Duration(cfg.JWTAccessTTLMinutes) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshTTLHours) * time.Hour,
	}

	tokens, _, err := middleware.GenerateTokenPair(jwtConfig, userID, role, userType, "web")
	require.NoError(t, err)
	return tokens.AccessToken
}
