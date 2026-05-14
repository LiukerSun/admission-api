package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// parseCSV splits a comma-separated env-var into a trimmed list,
// dropping empty segments so "" yields nil instead of [""].
func parseCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Config holds all application configuration.
type Config struct {
	Port                         string
	DatabaseURL                  string
	RedisAddr                    string
	JWTSecret                    string
	JWTAccessTTLMinutes          int
	JWTRefreshTTLHours           int
	Env                          string
	MockCallbackSecret           string
	AliyunSMSAccessKeyID         string
	AliyunSMSAccessKeySecret     string
	AliyunSMSEndpoint            string
	AliyunSMSSignName            string
	AliyunSMSTemplateCode        string
	AliyunSMSTemplateParamFormat string
	SMSCodeTTLMinutes            int
	SMSSendCooldownSeconds       int
	SMSDailyLimit                int
	SMSMaxVerifyAttempts         int
	LLMProvider                  string
	LLMAPIKey                    string
	LLMBaseURL                   string
	LLMModel                     string
	// CardLinkWhitelist enumerates hosts that the render_card tool is
	// allowed to link out to. Relative links ("/...") on the same site
	// are always allowed. Configured via env var as a comma-separated
	// list, e.g. "admission.example.com,knowledge.example.com".
	CardLinkWhitelist          []string
	VolunteerPlansFilePath     string
	AlipayAppID                string
	AlipayAppPrivateKey        string
	AlipayAppPrivateKeyPath    string
	AlipayAppPublicCertPath    string
	AlipayAlipayPublicCertPath string
	AlipayAlipayRootCertPath   string
	AlipayNotifyURL            string
	AlipayReturnURL            string
	AlipaySandbox              bool
	AlipayEncryptKey           string
	AlipayDecryptKey           string
}

// Load reads configuration from environment variables.
// It attempts to load .env file if present (ignored in production).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                         getEnv("PORT", "8080"),
		DatabaseURL:                  buildDatabaseURL(),
		RedisAddr:                    buildRedisAddr(),
		JWTSecret:                    getEnv("JWT_SECRET", ""),
		JWTAccessTTLMinutes:          getIntEnv("JWT_ACCESS_TTL_MINUTES", 15),
		JWTRefreshTTLHours:           getIntEnv("JWT_REFRESH_TTL_HOURS", 168),
		Env:                          getEnv("ENV", "development"),
		MockCallbackSecret:           getEnv("MOCK_CALLBACK_SECRET", ""),
		AliyunSMSAccessKeyID:         getEnv("ALIYUN_SMS_ACCESS_KEY_ID", ""),
		AliyunSMSAccessKeySecret:     getEnv("ALIYUN_SMS_ACCESS_KEY_SECRET", ""),
		AliyunSMSEndpoint:            getEnv("ALIYUN_SMS_ENDPOINT", "dysmsapi.aliyuncs.com"),
		AliyunSMSSignName:            getEnv("ALIYUN_SMS_SIGN_NAME", ""),
		AliyunSMSTemplateCode:        getEnv("ALIYUN_SMS_TEMPLATE_CODE", ""),
		AliyunSMSTemplateParamFormat: getEnv("ALIYUN_SMS_TEMPLATE_PARAM_FORMAT", "json"),
		SMSCodeTTLMinutes:            getIntEnv("SMS_CODE_TTL_MINUTES", 5),
		SMSSendCooldownSeconds:       getIntEnv("SMS_SEND_COOLDOWN_SECONDS", 60),
		SMSDailyLimit:                getIntEnv("SMS_DAILY_LIMIT", 10),
		SMSMaxVerifyAttempts:         getIntEnv("SMS_MAX_VERIFY_ATTEMPTS", 5),
		LLMProvider:                  getEnv("LLM_PROVIDER", "openai"),
		LLMAPIKey:                    getEnv("LLM_API_KEY", ""),
		LLMBaseURL:                   getEnv("LLM_BASE_URL", ""),
		LLMModel:                     getEnv("LLM_MODEL", ""),
		CardLinkWhitelist:            parseCSV(getEnv("CARD_LINK_WHITELIST", "")),
		VolunteerPlansFilePath:       getEnv("VOLUNTEER_PLANS_FILE_PATH", "../admission-frontend/plans.json"),
		AlipayAppID:                  getEnv("ALIPAY_APP_ID", ""),
		AlipayAppPrivateKey:          getEnv("ALIPAY_APP_PRIVATE_KEY", ""),
		AlipayAppPrivateKeyPath:      getEnv("ALIPAY_APP_PRIVATE_KEY_PATH", ""),
		AlipayAppPublicCertPath:      getEnv("ALIPAY_APP_PUBLIC_CERT_PATH", ""),
		AlipayAlipayPublicCertPath:   getEnv("ALIPAY_ALIPAY_PUBLIC_CERT_PATH", ""),
		AlipayAlipayRootCertPath:     getEnv("ALIPAY_ALIPAY_ROOT_CERT_PATH", ""),
		AlipayNotifyURL:              getEnv("ALIPAY_NOTIFY_URL", ""),
		AlipayReturnURL:              getEnv("ALIPAY_RETURN_URL", ""),
		AlipaySandbox:                getBoolEnv("ALIPAY_SANDBOX", true),
		AlipayEncryptKey:             getEnv("ALIPAY_ENCRYPT_KEY", ""),
		AlipayDecryptKey:             getEnv("ALIPAY_DECRYPT_KEY", ""),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
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

func getBoolEnv(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1"
	}
	return fallback
}
