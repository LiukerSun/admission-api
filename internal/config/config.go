package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	Port                string
	DatabaseURL         string
	JWTSecret           string
	JWTAccessTTLMinutes int
	JWTRefreshTTLHours  int
	Env                 string
}

// Load reads configuration from environment variables.
// It attempts to load .env file if present (ignored in production).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", ""),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		JWTAccessTTLMinutes: getIntEnv("JWT_ACCESS_TTL_MINUTES", 15),
		JWTRefreshTTLHours:  getIntEnv("JWT_REFRESH_TTL_HOURS", 168),
		Env:                 getEnv("ENV", "development"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
