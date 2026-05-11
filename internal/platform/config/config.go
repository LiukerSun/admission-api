package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const defaultJWTSecret = "your-super-secret-jwt-key-change-in-production"

// Config holds all application configuration.
type Config struct {
	Port                     string
	DatabaseURL              string
	RedisAddr                string
	JWTSecret                string
	JWTAccessTTLMinutes      int
	JWTRefreshTTLHours       int
	Env                      string
	MockCallbackSecret       string
	AliyunSMSAccessKeyID     string
	AliyunSMSAccessKeySecret string
	AliyunSMSEndpoint        string
	AliyunSMSSignName        string
	AliyunSMSTemplateCode    string
	SMSCodeTTLMinutes        int
	SMSSendCooldownSeconds   int
	SMSDailyLimit            int
	SMSMaxVerifyAttempts     int

	// OpenAI / compatible LLM configuration
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	OpenAIModel          string
	OpenAITimeoutSeconds int
	AICardLinkHosts      string // comma-separated allowlist for render_card link hosts
	AIChatRateLimitPerMin int
}

// Load reads configuration from environment variables.
// It attempts to load .env file if present (ignored in production).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                     getEnv("PORT", "8080"),
		DatabaseURL:              buildDatabaseURL(),
		RedisAddr:                buildRedisAddr(),
		JWTSecret:                getEnv("JWT_SECRET", ""),
		JWTAccessTTLMinutes:      getIntEnv("JWT_ACCESS_TTL_MINUTES", 15),
		JWTRefreshTTLHours:       getIntEnv("JWT_REFRESH_TTL_HOURS", 168),
		Env:                      getEnv("ENV", "development"),
		MockCallbackSecret:       getEnv("MOCK_CALLBACK_SECRET", ""),
		AliyunSMSAccessKeyID:     getEnv("ALIYUN_SMS_ACCESS_KEY_ID", ""),
		AliyunSMSAccessKeySecret: getEnv("ALIYUN_SMS_ACCESS_KEY_SECRET", ""),
		AliyunSMSEndpoint:        getEnv("ALIYUN_SMS_ENDPOINT", "dysmsapi.aliyuncs.com"),
		AliyunSMSSignName:        getEnv("ALIYUN_SMS_SIGN_NAME", ""),
		AliyunSMSTemplateCode:    getEnv("ALIYUN_SMS_TEMPLATE_CODE", ""),
		SMSCodeTTLMinutes:        getIntEnv("SMS_CODE_TTL_MINUTES", 5),
		SMSSendCooldownSeconds:   getIntEnv("SMS_SEND_COOLDOWN_SECONDS", 60),
		SMSDailyLimit:            getIntEnv("SMS_DAILY_LIMIT", 10),
		SMSMaxVerifyAttempts:     getIntEnv("SMS_MAX_VERIFY_ATTEMPTS", 5),
		OpenAIAPIKey:             getEnv("OPENAI_API_KEY", ""),
		OpenAIBaseURL:            getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:              getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAITimeoutSeconds:     getIntEnv("OPENAI_TIMEOUT_SECONDS", 120),
		AICardLinkHosts:          getEnv("AI_CARD_LINK_HOSTS", ""),
		AIChatRateLimitPerMin:    getIntEnv("AI_CHAT_RATE_LIMIT_PER_MIN", 30),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if strings.EqualFold(cfg.Env, "production") && cfg.JWTSecret == defaultJWTSecret {
		return nil, fmt.Errorf("JWT_SECRET must be changed in production")
	}

	return cfg, nil
}

func buildDatabaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	host := getEnv("POSTGRES_HOST", "localhost")
	user := getEnv("POSTGRES_USER", "app")
	password := getEnv("POSTGRES_PASSWORD", "app")
	db := getEnv("POSTGRES_DB", "admission")
	port := getEnv("POSTGRES_PORT", "5432")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, db)
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

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
