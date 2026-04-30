package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountBanned      = errors.New("account has been banned")
)

// Service defines the authentication business logic interface.
type Service interface {
	Register(ctx context.Context, email, password, userType string) (*User, error)
	Login(ctx context.Context, email, password, platform string) (*middleware.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*middleware.TokenPair, error)
	Me(ctx context.Context, userID int64) (*User, error)
	ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error
	ForgotPassword(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, token, newPassword string) error
}

// AuthService implements Service.
type AuthService struct {
	store        Store
	tokenManager *redis.RefreshTokenManager
	jwtConfig    *middleware.JWTConfig
	rdb          *redis.Client
}

func NewAuthService(store Store, tokenManager *redis.RefreshTokenManager, jwtConfig *middleware.JWTConfig, rdb *redis.Client) *AuthService {
	return &AuthService{
		store:        store,
		tokenManager: tokenManager,
		jwtConfig:    jwtConfig,
		rdb:          rdb,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password, userType string) (*User, error) {
	if userType != "parent" && userType != "student" {
		return nil, fmt.Errorf("invalid user type: must be 'parent' or 'student'")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := s.store.Create(ctx, email, string(hash), "user", userType)
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (s *AuthService) Login(ctx context.Context, email, password, platform string) (*middleware.TokenPair, error) {
	u, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if u.Status == "banned" {
		return nil, ErrAccountBanned
	}

	tokens, _, err := middleware.GenerateTokenPair(s.jwtConfig, u.ID, u.Role, u.UserType, platform)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	refreshHash := middleware.HashRefreshToken(tokens.RefreshToken)
	if err := s.tokenManager.Save(ctx, refreshHash, u.ID, platform); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*middleware.TokenPair, error) {
	claims, err := middleware.ParseRefreshToken(s.jwtConfig, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	oldHash := middleware.HashRefreshToken(refreshToken)
	tokens, _, err := middleware.GenerateTokenPair(s.jwtConfig, claims.UserID, claims.Role, claims.UserType, claims.Platform)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	newHash := middleware.HashRefreshToken(tokens.RefreshToken)
	rotated, err := s.tokenManager.RotateSingleUse(ctx, oldHash, newHash, claims.UserID, claims.Platform, &redis.RotationReplay{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    tokens.ExpiresIn,
	})
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}
	if !rotated {
		replay, replayErr := s.tokenManager.GetRotationReplay(ctx, oldHash)
		if replayErr != nil {
			return nil, fmt.Errorf("load rotation replay: %w", replayErr)
		}
		if replay != nil {
			return &middleware.TokenPair{
				AccessToken:  replay.AccessToken,
				RefreshToken: replay.RefreshToken,
				ExpiresIn:    replay.ExpiresIn,
			}, nil
		}
		wasRotated, usedErr := s.tokenManager.WasRotated(ctx, oldHash)
		if usedErr != nil {
			return nil, fmt.Errorf("check refresh rotation marker: %w", usedErr)
		}
		if wasRotated {
			return nil, fmt.Errorf("invalid or expired refresh token")
		}
		return nil, fmt.Errorf("invalid or expired refresh token")
	}

	return tokens, nil
}

func (s *AuthService) Me(ctx context.Context, userID int64) (*User, error) {
	return s.store.GetByID(ctx, userID)
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	u, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	if s.tokenManager != nil {
		if err := s.tokenManager.RevokeUserSessions(ctx, userID); err != nil {
			return fmt.Errorf("revoke user sessions: %w", err)
		}
	}

	return nil
}

const (
	resetTokenTTL = 15 * time.Minute
	resetTokenLen = 32
	resetTokenKey = "password_reset:%s"
)

func generateResetToken() (string, error) {
	b := make([]byte, resetTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate reset token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, email string) (string, error) {
	// Always return success to prevent email enumeration.
	u, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("get user by email: %w", err)
	}

	token, err := generateResetToken()
	if err != nil {
		return "", err
	}

	key := fmt.Sprintf(resetTokenKey, token)
	if err := s.rdb.Set(ctx, key, u.ID, resetTokenTTL); err != nil {
		return "", fmt.Errorf("store reset token: %w", err)
	}

	return token, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	key := fmt.Sprintf(resetTokenKey, token)
	userIDStr, err := s.rdb.Get(ctx, key)
	if err != nil {
		return ErrVerificationCodeExpired
	}

	var userID int64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil {
		return ErrVerificationCodeExpired
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.store.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Delete the token so it can't be reused.
	if err := s.rdb.Del(ctx, key); err != nil {
		return fmt.Errorf("delete reset token: %w", err)
	}

	if s.tokenManager != nil {
		if err := s.tokenManager.RevokeUserSessions(ctx, userID); err != nil {
			return fmt.Errorf("revoke user sessions: %w", err)
		}
	}

	return nil
}
