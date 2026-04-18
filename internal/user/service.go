package user

import (
	"context"
	"fmt"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"golang.org/x/crypto/bcrypt"
)

// Service defines the authentication business logic interface.
type Service interface {
	Register(ctx context.Context, email, password string) (*User, error)
	Login(ctx context.Context, email, password, platform string) (*middleware.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*middleware.TokenPair, error)
	Me(ctx context.Context, userID int64) (*User, error)
}

// AuthService implements Service.
type AuthService struct {
	store         Store
	tokenManager  *redis.RefreshTokenManager
	jwtConfig     *middleware.JWTConfig
}

func NewAuthService(store Store, tokenManager *redis.RefreshTokenManager, jwtConfig *middleware.JWTConfig) *AuthService {
	return &AuthService{
		store:         store,
		tokenManager:  tokenManager,
		jwtConfig:     jwtConfig,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := s.store.Create(ctx, email, string(hash), "user")
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (s *AuthService) Login(ctx context.Context, email, password, platform string) (*middleware.TokenPair, error) {
	u, err := s.store.GetByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	tokens, rawRefresh, err := middleware.GenerateTokenPair(s.jwtConfig, u.ID, u.Role, platform)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	refreshHash := middleware.HashRefreshToken(rawRefresh)
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
	valid, err := s.tokenManager.Verify(ctx, oldHash, claims.UserID, claims.Platform)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid or expired refresh token")
	}

	tokens, rawRefresh, err := middleware.GenerateTokenPair(s.jwtConfig, claims.UserID, claims.Role, claims.Platform)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	newHash := middleware.HashRefreshToken(rawRefresh)
	if err := s.tokenManager.Rotate(ctx, oldHash, newHash, claims.UserID, claims.Platform); err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	return tokens, nil
}

func (s *AuthService) Me(ctx context.Context, userID int64) (*User, error) {
	return s.store.GetByID(ctx, userID)
}
