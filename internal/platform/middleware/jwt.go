package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Secret          string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	RefreshTokenTTL time.Duration
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type Claims struct {
	UserID   int64  `json:"sub"`
	Role     string `json:"role"`
	Platform string `json:"platform"`
	Type     string `json:"typ"`
	jwt.RegisteredClaims
}

func GenerateTokenPair(cfg *JWTConfig, userID int64, role, platform string) (*TokenPair, string, error) {
	now := time.Now()

	accessClaims := Claims{
		UserID:   userID,
		Role:     role,
		Platform: platform,
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.AccessTTL)),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessSigned, err := accessToken.SignedString([]byte(cfg.Secret))
	if err != nil {
		return nil, "", fmt.Errorf("sign access token: %w", err)
	}

	refreshTokenRaw, err := generateRandomToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate refresh token: %w", err)
	}
	refreshClaims := Claims{
		UserID:   userID,
		Role:     role,
		Platform: platform,
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.RefreshTTL)),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshSigned, err := refreshToken.SignedString([]byte(cfg.Secret))
	if err != nil {
		return nil, "", fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessSigned,
		RefreshToken: refreshSigned,
		ExpiresIn:    int(cfg.AccessTTL.Seconds()),
	}, refreshTokenRaw, nil
}

func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func JWTMiddleware(cfg *JWTConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractBearerToken(r)
			if tokenString == "" {
				http.Error(w, `{"code":1002,"message":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(cfg.Secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"code":1002,"message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			if claims.Type != "access" {
				http.Error(w, `{"code":1002,"message":"invalid token type"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextUserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, ContextRoleKey, claims.Role)
			ctx = context.WithValue(ctx, ContextPlatformKey, claims.Platform)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ParseRefreshToken(cfg *JWTConfig, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	if claims.Type != "refresh" {
		return nil, fmt.Errorf("invalid token type")
	}
	return claims, nil
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
