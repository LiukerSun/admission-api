package user

import (
	"context"
	"errors"
	"fmt"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountBanned      = errors.New("account has been banned")
)

// Service defines the authentication business logic interface. All identifiers
// are phone numbers; email-based auth is no longer supported.
type Service interface {
	SendAuthCode(ctx context.Context, phone string, scene Scene) error
	RegisterByPhone(ctx context.Context, phone, code, password, platform string) (*User, *middleware.TokenPair, error)
	LoginByPassword(ctx context.Context, phone, password, platform string) (*middleware.TokenPair, error)
	LoginByCode(ctx context.Context, phone, code, platform string) (*middleware.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*middleware.TokenPair, error)
	Me(ctx context.Context, userID int64) (*User, error)
	ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error
}

// AuthService implements Service.
type AuthService struct {
	store        Store
	phoneAuth    PhoneAuthCodeService
	tokenManager *redis.RefreshTokenManager
	jwtConfig    *middleware.JWTConfig
}

func NewAuthService(store Store, phoneAuth PhoneAuthCodeService, tokenManager *redis.RefreshTokenManager, jwtConfig *middleware.JWTConfig) *AuthService {
	return &AuthService{
		store:        store,
		phoneAuth:    phoneAuth,
		tokenManager: tokenManager,
		jwtConfig:    jwtConfig,
	}
}

func (s *AuthService) SendAuthCode(ctx context.Context, phone string, scene Scene) error {
	if s.phoneAuth == nil {
		return errors.New("phone auth service not configured")
	}
	return s.phoneAuth.SendAuthCode(ctx, phone, scene)
}

func (s *AuthService) RegisterByPhone(ctx context.Context, phone, code, password, platform string) (*User, *middleware.TokenPair, error) {
	if s.phoneAuth == nil {
		return nil, nil, errors.New("phone auth service not configured")
	}

	phone = normalizePhone(phone)
	if err := validatePhone(phone); err != nil {
		return nil, nil, err
	}

	if err := s.phoneAuth.VerifyAuthCode(ctx, phone, code, SceneRegister); err != nil {
		return nil, nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := s.store.CreateWithPhone(ctx, phone, string(hash), "user")
	if err != nil {
		return nil, nil, err
	}

	tokens, err := s.issueTokens(ctx, u, platform)
	if err != nil {
		return nil, nil, err
	}

	return u, tokens, nil
}

func (s *AuthService) LoginByPassword(ctx context.Context, phone, password, platform string) (*middleware.TokenPair, error) {
	phone = normalizePhone(phone)
	if err := validatePhone(phone); err != nil {
		return nil, ErrInvalidCredentials
	}

	u, err := s.store.GetByPhone(ctx, phone)
	if err != nil {
		// Do not leak whether the phone is registered.
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if u.Status == "banned" {
		return nil, ErrAccountBanned
	}

	return s.issueTokens(ctx, u, platform)
}

func (s *AuthService) LoginByCode(ctx context.Context, phone, code, platform string) (*middleware.TokenPair, error) {
	if s.phoneAuth == nil {
		return nil, errors.New("phone auth service not configured")
	}

	phone = normalizePhone(phone)
	if err := validatePhone(phone); err != nil {
		return nil, err
	}

	if err := s.phoneAuth.VerifyAuthCode(ctx, phone, code, SceneLogin); err != nil {
		return nil, err
	}

	u, err := s.store.GetByPhone(ctx, phone)
	if err != nil {
		// Race: account was deleted between send-code and verify. Surface as
		// "not found" so the UI can prompt the user to register.
		return nil, ErrUserNotFound
	}

	if u.Status == "banned" {
		return nil, ErrAccountBanned
	}

	return s.issueTokens(ctx, u, platform)
}

func (s *AuthService) issueTokens(ctx context.Context, u *User, platform string) (*middleware.TokenPair, error) {
	tokens, _, err := middleware.GenerateTokenPair(s.jwtConfig, u.ID, u.Role, u.IsAdmin, platform)
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
	tokens, _, err := middleware.GenerateTokenPair(s.jwtConfig, claims.UserID, claims.Role, claims.IsAdmin, claims.Platform)
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
