package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	Port                     string
	DatabaseURL              string
	RedisAddr                string
	JWTSecret                string
	JWTAccessTTLMinutes      int
	JWTRefreshTTLHours       int
	Env                      string
	AliyunSMSAccessKeyID     string
	AliyunSMSAccessKeySecret string
	AliyunSMSEndpoint        string
	AliyunSMSSignName        string
	AliyunSMSTemplateCode    string
	SMSCodeTTLMinutes        int
	SMSSendCooldownSeconds   int
	SMSDailyLimit            int
	SMSMaxVerifyAttempts     int
}

// Load reads configuration from environment variables.
// It attempts to load .env file if present (ignored in production).
// Missing required fields will panic.
func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                     getEnv("PORT", "8080"),
		DatabaseURL:              buildDatabaseURL(),
		RedisAddr:                buildRedisAddr(),
		JWTSecret:                requireEnv("JWT_SECRET"),
		JWTAccessTTLMinutes:      getIntEnv("JWT_ACCESS_TTL_MINUTES", 15),
		JWTRefreshTTLHours:       getIntEnv("JWT_REFRESH_TTL_HOURS", 168),
		Env:                      getEnv("ENV", "development"),
		AliyunSMSAccessKeyID:     getEnv("ALIYUN_SMS_ACCESS_KEY_ID", ""),
		AliyunSMSAccessKeySecret: getEnv("ALIYUN_SMS_ACCESS_KEY_SECRET", ""),
		AliyunSMSEndpoint:        getEnv("ALIYUN_SMS_ENDPOINT", "dysmsapi.aliyuncs.com"),
		AliyunSMSSignName:        getEnv("ALIYUN_SMS_SIGN_NAME", ""),
		AliyunSMSTemplateCode:    getEnv("ALIYUN_SMS_TEMPLATE_CODE", ""),
		SMSCodeTTLMinutes:        getIntEnv("SMS_CODE_TTL_MINUTES", 5),
		SMSSendCooldownSeconds:   getIntEnv("SMS_SEND_COOLDOWN_SECONDS", 60),
		SMSDailyLimit:            getIntEnv("SMS_DAILY_LIMIT", 10),
		SMSMaxVerifyAttempts:     getIntEnv("SMS_MAX_VERIFY_ATTEMPTS", 5),
	}

	return cfg
}

func buildDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	user := getEnv("POSTGRES_USER", "app")
	password := getEnv("POSTGRES_PASSWORD", "app")
	db := getEnv("POSTGRES_DB", "admission")
	port := getEnv("POSTGRES_PORT", "5432")
	return fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", user, password, port, db)
}

func buildRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	port := getEnv("REDIS_PORT", "6379")
	return fmt.Sprintf("localhost:%s", port)
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
