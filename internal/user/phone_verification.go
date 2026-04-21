package user

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"time"

	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/sms"
)

var mainlandPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

const (
	verificationCodeLength = 6
)

type PhoneVerificationService interface {
	SendPhoneVerificationCode(ctx context.Context, userID int64, phone string) error
	VerifyPhoneCode(ctx context.Context, userID int64, phone, code string) error
}

type PhoneVerificationConfig struct {
	CodeTTL      time.Duration
	SendCooldown time.Duration
	MaxAttempts  int
}

type phoneVerificationService struct {
	store  Store
	redis  *redis.Client
	sms    sms.Client
	config PhoneVerificationConfig
}

func NewPhoneVerificationService(store Store, redisClient *redis.Client, smsClient sms.Client, cfg PhoneVerificationConfig) PhoneVerificationService {
	return &phoneVerificationService{
		store:  store,
		redis:  redisClient,
		sms:    smsClient,
		config: cfg,
	}
}

func verificationCodeKey(phone string) string {
	return fmt.Sprintf("sms:code:%s", phone)
}

func verificationCooldownKey(phone string) string {
	return fmt.Sprintf("sms:cooldown:%s", phone)
}

func verificationAttemptsKey(phone string) string {
	return fmt.Sprintf("sms:attempts:%s", phone)
}

func normalizePhone(phone string) string {
	return phone
}

func validatePhone(phone string) error {
	if !mainlandPhonePattern.MatchString(phone) {
		return fmt.Errorf("invalid phone number")
	}
	return nil
}

func (s *phoneVerificationService) SendPhoneVerificationCode(ctx context.Context, userID int64, phone string) error {
	phone = normalizePhone(phone)
	if err := validatePhone(phone); err != nil {
		return err
	}

	if err := s.ensurePhoneAvailable(ctx, userID, phone); err != nil {
		return err
	}

	cooldownExists, err := s.redis.Exists(ctx, verificationCooldownKey(phone))
	if err != nil {
		return fmt.Errorf("check send cooldown: %w", err)
	}
	if cooldownExists > 0 {
		return fmt.Errorf("verification code sent too frequently")
	}

	code, err := generateNumericCode(verificationCodeLength)
	if err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}

	if err := s.sms.SendVerificationCode(ctx, phone, code); err != nil {
		return fmt.Errorf("send verification code: %w", err)
	}

	if err := s.redis.Set(ctx, verificationCodeKey(phone), code, s.config.CodeTTL); err != nil {
		return fmt.Errorf("save verification code: %w", err)
	}
	if err := s.redis.Set(ctx, verificationCooldownKey(phone), "1", s.config.SendCooldown); err != nil {
		return fmt.Errorf("save verification cooldown: %w", err)
	}
	if err := s.redis.Del(ctx, verificationAttemptsKey(phone)); err != nil {
		return fmt.Errorf("reset verification attempts: %w", err)
	}

	slog.Info("phone verification code sent", "user_id", userID, "phone", maskPhone(phone))
	return nil
}

func (s *phoneVerificationService) VerifyPhoneCode(ctx context.Context, userID int64, phone, code string) error {
	phone = normalizePhone(phone)
	if err := validatePhone(phone); err != nil {
		return err
	}

	if err := s.ensurePhoneAvailable(ctx, userID, phone); err != nil {
		return err
	}

	savedCode, err := s.redis.Get(ctx, verificationCodeKey(phone))
	if err != nil {
		return fmt.Errorf("verification code not found or expired")
	}

	if savedCode != code {
		attempts, incrErr := s.redis.Incr(ctx, verificationAttemptsKey(phone))
		if incrErr != nil {
			return fmt.Errorf("record verification attempt: %w", incrErr)
		}
		if attempts == 1 {
			if ttl, ttlErr := s.redis.TTL(ctx, verificationCodeKey(phone)); ttlErr == nil && ttl > 0 {
				_ = s.redis.Expire(ctx, verificationAttemptsKey(phone), ttl)
			}
		}
		if attempts >= int64(s.config.MaxAttempts) {
			_ = s.redis.Del(ctx, verificationCodeKey(phone), verificationAttemptsKey(phone))
			return fmt.Errorf("verification code attempts exceeded")
		}
		return fmt.Errorf("invalid verification code")
	}

	if err := s.store.UpdatePhone(ctx, userID, phone); err != nil {
		if err.Error() == "phone already exists" {
			return err
		}
		return fmt.Errorf("update phone: %w", err)
	}

	if err := s.redis.Del(ctx, verificationCodeKey(phone), verificationAttemptsKey(phone)); err != nil {
		return fmt.Errorf("clear verification code: %w", err)
	}

	return nil
}

func (s *phoneVerificationService) ensurePhoneAvailable(ctx context.Context, userID int64, phone string) error {
	u, err := s.store.GetByPhone(ctx, phone)
	if err != nil {
		if err.Error() == "user not found" {
			return nil
		}
		return fmt.Errorf("check phone uniqueness: %w", err)
	}
	if u.ID != userID {
		return fmt.Errorf("phone already exists")
	}
	return nil
}

func generateNumericCode(length int) (string, error) {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		result[i] = byte('0' + n.Int64())
	}
	return string(result), nil
}

func maskPhone(phone string) string {
	if len(phone) != 11 {
		return phone
	}
	return phone[:3] + "****" + phone[7:]
}
