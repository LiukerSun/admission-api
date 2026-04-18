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
	RedisAddr           string
	JWTSecret           string
	JWTAccessTTLMinutes int
	JWTRefreshTTLHours  int
	Env                 string
}

// Load reads configuration from environment variables.
// It attempts to load .env file if present (ignored in production).
// Missing required fields will panic.
func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         requireEnv("DATABASE_URL"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		JWTSecret:           requireEnv("JWT_SECRET"),
		JWTAccessTTLMinutes: getIntEnv("JWT_ACCESS_TTL_MINUTES", 15),
		JWTRefreshTTLHours:  getIntEnv("JWT_REFRESH_TTL_HOURS", 168),
		Env:                 getEnv("ENV", "development"),
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required environment variable: %s", key))
	}
	return v
}

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
