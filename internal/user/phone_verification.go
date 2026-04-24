package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
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
	DailyLimit   int
	MaxAttempts  int
	Now          func() time.Time
}

type verificationRedis interface {
	Get(ctx context.Context, key string) (string, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Incr(ctx context.Context, key string) (int64, error)
	Decr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

type phoneVerificationService struct {
	store  Store
	redis  verificationRedis
	sms    sms.Client
	config PhoneVerificationConfig
	now    func() time.Time
}

func NewPhoneVerificationService(store Store, redisClient *redis.Client, smsClient sms.Client, cfg PhoneVerificationConfig) PhoneVerificationService {
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &phoneVerificationService{
		store:  store,
		redis:  redisClient,
		sms:    smsClient,
		config: cfg,
		now:    nowFn,
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

func verificationDailyLimitKey(phone string, now time.Time) string {
	return fmt.Sprintf("sms:daily:%s:%s", phone, now.Format("20060102"))
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	phone = replacer.Replace(phone)
	phone = strings.TrimPrefix(phone, "+86")
	if strings.HasPrefix(phone, "86") && len(phone) == 13 {
		phone = strings.TrimPrefix(phone, "86")
	}
	return phone
}

func validatePhone(phone string) error {
	if !mainlandPhonePattern.MatchString(phone) {
		return ErrPhoneInvalid
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
		return ErrPhoneCodeTooFrequent
	}

	code, err := generateNumericCode(verificationCodeLength)
	if err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}

	releaseDailySlot, err := s.reserveDailySendSlot(ctx, phone)
	if err != nil {
		return err
	}

	if err := s.redis.Set(ctx, verificationCodeKey(phone), code, s.config.CodeTTL); err != nil {
		releaseDailySlot()
		return fmt.Errorf("save verification code: %w", err)
	}
	if err := s.redis.Set(ctx, verificationCooldownKey(phone), "1", s.config.SendCooldown); err != nil {
		_ = s.redis.Del(ctx, verificationCodeKey(phone))
		releaseDailySlot()
		return fmt.Errorf("save verification cooldown: %w", err)
	}
	if err := s.redis.Del(ctx, verificationAttemptsKey(phone)); err != nil {
		_ = s.redis.Del(ctx, verificationCodeKey(phone), verificationCooldownKey(phone))
		releaseDailySlot()
		return fmt.Errorf("reset verification attempts: %w", err)
	}

	if err := s.sms.SendVerificationCode(ctx, phone, code); err != nil {
		_ = s.redis.Del(ctx, verificationCodeKey(phone), verificationCooldownKey(phone), verificationAttemptsKey(phone))
		releaseDailySlot()
		return fmt.Errorf("send verification code: %w", err)
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
		return ErrVerificationCodeExpired
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
			return ErrVerificationCodeExceeded
		}
		return ErrVerificationCodeInvalid
	}

	if err := s.store.UpdatePhone(ctx, userID, phone); err != nil {
		if errors.Is(err, ErrPhoneAlreadyExists) {
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
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return fmt.Errorf("check phone uniqueness: %w", err)
	}
	if u.ID != userID {
		return ErrPhoneAlreadyExists
	}
	return nil
}

func (s *phoneVerificationService) reserveDailySendSlot(ctx context.Context, phone string) (func(), error) {
	if s.config.DailyLimit <= 0 {
		return func() {}, nil
	}

	key := verificationDailyLimitKey(phone, s.now())
	count, err := s.redis.Incr(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("record daily verification send count: %w", err)
	}

	if count == 1 {
		if err := s.redis.Expire(ctx, key, durationUntilNextDay(s.now())); err != nil {
			return nil, fmt.Errorf("set daily verification send count expiry: %w", err)
		}
	}

	release := func() {
		if _, decrErr := s.redis.Decr(ctx, key); decrErr != nil {
			slog.Warn("failed to release sms daily limit slot", "phone", maskPhone(phone), "error", decrErr)
		}
	}

	if count > int64(s.config.DailyLimit) {
		release()
		return nil, ErrPhoneCodeDailyLimit
	}

	return release, nil
}

func durationUntilNextDay(now time.Time) time.Duration {
	nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return nextDay.Sub(now)
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
