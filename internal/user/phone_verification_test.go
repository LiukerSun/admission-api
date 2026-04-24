package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type memoryVerificationRedis struct {
	values   map[string]string
	ttls     map[string]time.Duration
	exists   map[string]bool
	counters map[string]int64
}

func newMemoryVerificationRedis() *memoryVerificationRedis {
	return &memoryVerificationRedis{
		values:   make(map[string]string),
		ttls:     make(map[string]time.Duration),
		exists:   make(map[string]bool),
		counters: make(map[string]int64),
	}
}

func (m *memoryVerificationRedis) Get(_ context.Context, key string) (string, error) {
	value, ok := m.values[key]
	if !ok {
		return "", errors.New("redis: nil")
	}
	return value, nil
}

func (m *memoryVerificationRedis) TTL(_ context.Context, key string) (time.Duration, error) {
	ttl, ok := m.ttls[key]
	if !ok {
		return 0, nil
	}
	return ttl, nil
}

func (m *memoryVerificationRedis) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	m.values[key] = value.(string)
	m.exists[key] = true
	m.ttls[key] = ttl
	return nil
}

func (m *memoryVerificationRedis) Del(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(m.values, key)
		delete(m.exists, key)
		delete(m.ttls, key)
		delete(m.counters, key)
	}
	return nil
}

func (m *memoryVerificationRedis) Exists(_ context.Context, keys ...string) (int64, error) {
	var count int64
	for _, key := range keys {
		if m.exists[key] {
			count++
		}
	}
	return count, nil
}

func (m *memoryVerificationRedis) Incr(_ context.Context, key string) (int64, error) {
	m.counters[key]++
	m.exists[key] = true
	return m.counters[key], nil
}

func (m *memoryVerificationRedis) Decr(_ context.Context, key string) (int64, error) {
	m.counters[key]--
	if m.counters[key] <= 0 {
		delete(m.counters, key)
		delete(m.exists, key)
		delete(m.ttls, key)
		return 0, nil
	}
	return m.counters[key], nil
}

func (m *memoryVerificationRedis) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.ttls[key] = ttl
	m.exists[key] = true
	return nil
}

type stubSMSClient struct {
	mock.Mock
}

func (m *stubSMSClient) SendVerificationCode(ctx context.Context, phone, code string) error {
	args := m.Called(ctx, phone, code)
	return args.Error(0)
}

func newTestPhoneVerificationService(store Store, redisClient verificationRedis, smsClient *stubSMSClient) *phoneVerificationService {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	return &phoneVerificationService{
		store: store,
		redis: redisClient,
		sms:   smsClient,
		config: PhoneVerificationConfig{
			CodeTTL:      5 * time.Minute,
			SendCooldown: time.Minute,
			DailyLimit:   2,
			MaxAttempts:  3,
			Now:          func() time.Time { return now },
		},
		now: func() time.Time { return now },
	}
}

func TestPhoneVerificationService_SendCode_InvalidPhone(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	err := svc.SendPhoneVerificationCode(context.Background(), 1, "123")

	assert.ErrorIs(t, err, ErrPhoneInvalid)
	smsClient.AssertNotCalled(t, "SendVerificationCode", mock.Anything, mock.Anything, mock.Anything)
}

func TestPhoneVerificationService_SendCode_Cooldown(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	redisClient.exists[verificationCooldownKey(phone)] = true
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound).Once()

	err := svc.SendPhoneVerificationCode(context.Background(), 1, phone)

	assert.ErrorIs(t, err, ErrPhoneCodeTooFrequent)
	smsClient.AssertNotCalled(t, "SendVerificationCode", mock.Anything, mock.Anything, mock.Anything)
	store.AssertExpectations(t)
}

func TestPhoneVerificationService_SendCode_DailyLimitExceeded(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	dailyKey := verificationDailyLimitKey(phone, svc.now())
	redisClient.counters[dailyKey] = 2
	redisClient.exists[dailyKey] = true
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound).Once()

	err := svc.SendPhoneVerificationCode(context.Background(), 1, phone)

	assert.ErrorIs(t, err, ErrPhoneCodeDailyLimit)
	assert.Equal(t, int64(2), redisClient.counters[dailyKey])
	smsClient.AssertNotCalled(t, "SendVerificationCode", mock.Anything, mock.Anything, mock.Anything)
	store.AssertExpectations(t)
}

func TestPhoneVerificationService_SendCode_Success(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	rawPhone := " +86 138-0013-8000 "
	normalizedPhone := "13800138000"
	store.On("GetByPhone", mock.Anything, normalizedPhone).Return(nil, ErrUserNotFound).Once()
	smsClient.On("SendVerificationCode", mock.Anything, normalizedPhone, mock.MatchedBy(func(code string) bool {
		return len(code) == verificationCodeLength
	})).Return(nil).Once()

	err := svc.SendPhoneVerificationCode(context.Background(), 1, rawPhone)

	require.NoError(t, err)
	code, getErr := redisClient.Get(context.Background(), verificationCodeKey(normalizedPhone))
	require.NoError(t, getErr)
	assert.Len(t, code, verificationCodeLength)
	assert.Equal(t, time.Minute, redisClient.ttls[verificationCooldownKey(normalizedPhone)])
	assert.Equal(t, int64(1), redisClient.counters[verificationDailyLimitKey(normalizedPhone, svc.now())])
	store.AssertExpectations(t)
	smsClient.AssertExpectations(t)
}

func TestPhoneVerificationService_SendCode_SMSFailureRollsBackState(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound).Once()
	smsClient.On("SendVerificationCode", mock.Anything, phone, mock.AnythingOfType("string")).
		Return(errors.New("provider down")).Once()

	err := svc.SendPhoneVerificationCode(context.Background(), 1, phone)

	assert.EqualError(t, err, "send verification code: provider down")
	_, codeErr := redisClient.Get(context.Background(), verificationCodeKey(phone))
	assert.Error(t, codeErr)
	exists, existsErr := redisClient.Exists(context.Background(), verificationCooldownKey(phone))
	require.NoError(t, existsErr)
	assert.Zero(t, exists)
	assert.Zero(t, redisClient.counters[verificationDailyLimitKey(phone, svc.now())])
	store.AssertExpectations(t)
	smsClient.AssertExpectations(t)
}

func TestPhoneVerificationService_VerifyCode_Success(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	redisClient.values[verificationCodeKey(phone)] = "123456"
	redisClient.exists[verificationCodeKey(phone)] = true
	store.On("GetByPhone", mock.Anything, phone).Return(&User{ID: 1, Phone: &phone}, nil).Once()
	store.On("UpdatePhone", mock.Anything, int64(1), phone).Return(nil).Once()

	err := svc.VerifyPhoneCode(context.Background(), 1, phone, "123456")

	require.NoError(t, err)
	_, codeErr := redisClient.Get(context.Background(), verificationCodeKey(phone))
	assert.Error(t, codeErr)
	store.AssertExpectations(t)
	smsClient.AssertExpectations(t)
}

func TestPhoneVerificationService_VerifyCode_InvalidCodeIncrementsAttempts(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	redisClient.values[verificationCodeKey(phone)] = "123456"
	redisClient.exists[verificationCodeKey(phone)] = true
	redisClient.ttls[verificationCodeKey(phone)] = 5 * time.Minute
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound).Once()

	err := svc.VerifyPhoneCode(context.Background(), 1, phone, "000000")

	assert.ErrorIs(t, err, ErrVerificationCodeInvalid)
	assert.Equal(t, int64(1), redisClient.counters[verificationAttemptsKey(phone)])
	assert.Equal(t, 5*time.Minute, redisClient.ttls[verificationAttemptsKey(phone)])
	store.AssertExpectations(t)
}

func TestPhoneVerificationService_VerifyCode_AttemptsExceeded(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	redisClient.values[verificationCodeKey(phone)] = "123456"
	redisClient.exists[verificationCodeKey(phone)] = true
	redisClient.counters[verificationAttemptsKey(phone)] = 2
	redisClient.exists[verificationAttemptsKey(phone)] = true
	store.On("GetByPhone", mock.Anything, phone).Return(nil, ErrUserNotFound).Once()

	err := svc.VerifyPhoneCode(context.Background(), 1, phone, "000000")

	assert.ErrorIs(t, err, ErrVerificationCodeExceeded)
	_, codeErr := redisClient.Get(context.Background(), verificationCodeKey(phone))
	assert.Error(t, codeErr)
	store.AssertExpectations(t)
}

func TestPhoneVerificationService_VerifyCode_PhoneAlreadyExists(t *testing.T) {
	store := new(mockStore)
	redisClient := newMemoryVerificationRedis()
	smsClient := new(stubSMSClient)
	svc := newTestPhoneVerificationService(store, redisClient, smsClient)

	phone := "13800138000"
	store.On("GetByPhone", mock.Anything, phone).Return(&User{ID: 2}, nil).Once()

	err := svc.VerifyPhoneCode(context.Background(), 1, phone, "123456")

	assert.ErrorIs(t, err, ErrPhoneAlreadyExists)
	store.AssertExpectations(t)
}
