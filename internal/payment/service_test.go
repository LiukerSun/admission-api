package payment

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"admission-api/internal/membership"
)

type mockPaymentStore struct {
	mock.Mock
}

func (m *mockPaymentStore) CreateOrder(ctx context.Context, input *CreateOrderInput) (*Order, bool, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*Order), args.Bool(1), args.Error(2)
}

func (m *mockPaymentStore) GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error) {
	args := m.Called(ctx, userID, orderNo)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(*Order), args.String(1), args.Error(2)
}

func (m *mockPaymentStore) GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error) {
	args := m.Called(ctx, orderNo)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(*Order), args.String(1), args.Error(2)
}

func (m *mockPaymentStore) ListOrdersForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Order, int64, error) {
	args := m.Called(ctx, userID, page, pageSize)
	return args.Get(0).([]*Order), args.Get(1).(int64), args.Error(2)
}

func (m *mockPaymentStore) ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) ([]*Order, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	return args.Get(0).([]*Order), args.Get(1).(int64), args.Error(2)
}

func (m *mockPaymentStore) CloseOrder(ctx context.Context, orderNo string) (*Order, error) {
	args := m.Called(ctx, orderNo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Order), args.Error(1)
}

func (m *mockPaymentStore) CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error) {
	args := m.Called(ctx, orderID, channel, amount)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Attempt), args.Error(1)
}

func (m *mockPaymentStore) MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error) {
	args := m.Called(ctx, attemptID, channelTradeNo, now)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Attempt), args.Error(1)
}

func (m *mockPaymentStore) MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error) {
	args := m.Called(ctx, orderID, now)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Order), args.Error(1)
}

func (m *mockPaymentStore) MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Order), args.Error(1)
}

func (m *mockPaymentStore) MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *mockPaymentStore) SaveCallback(ctx context.Context, req MockCallbackRequest, payload []byte) (*Callback, bool, error) {
	args := m.Called(ctx, req, payload)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*Callback), args.Bool(1), args.Error(2)
}

func (m *mockPaymentStore) MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error {
	args := m.Called(ctx, callbackID, processErr)
	return args.Error(0)
}

func (m *mockPaymentStore) GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error) {
	args := m.Called(ctx, channel, channelTradeNo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Attempt), args.Error(1)
}

func (m *mockPaymentStore) ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error) {
	args := m.Called(ctx, orderID)
	return args.Get(0).([]*Attempt), args.Error(1)
}

func (m *mockPaymentStore) ListCallbacks(ctx context.Context, channelTradeNo *string) ([]*Callback, error) {
	args := m.Called(ctx, channelTradeNo)
	return args.Get(0).([]*Callback), args.Error(1)
}

type mockMembershipSvc struct {
	mock.Mock
}

func (m *mockMembershipSvc) ListPlans(ctx context.Context) ([]membership.PlanResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).([]membership.PlanResponse), args.Error(1)
}

func (m *mockMembershipSvc) GetPurchasablePlan(ctx context.Context, planCode string) (*membership.Plan, error) {
	args := m.Called(ctx, planCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*membership.Plan), args.Error(1)
}

func (m *mockMembershipSvc) GetCurrent(ctx context.Context, userID int64) (*membership.CurrentMembershipResponse, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(*membership.CurrentMembershipResponse), args.Error(1)
}

func (m *mockMembershipSvc) HasActiveMembership(ctx context.Context, userID int64) (bool, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

func (m *mockMembershipSvc) GrantFromPaidOrder(ctx context.Context, req membership.GrantRequest) (*membership.UserMembership, *membership.Grant, bool, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*membership.UserMembership), args.Get(1).(*membership.Grant), args.Bool(2), args.Error(3)
}

func TestCreateOrderUsesPlanSnapshot(t *testing.T) {
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member)
	key := "idem-1"
	plan := &membership.Plan{ID: 9, PlanCode: "monthly", PlanName: "月度会员", DurationDays: 30, PriceAmount: 990, Currency: "CNY"}
	order := &Order{ID: 1, OrderNo: "MO1", UserID: 7, ProductRefID: 9, Subject: "月度会员", Amount: 990, Currency: "CNY", OrderStatus: OrderStatusAwaitingPayment, PaymentStatus: PaymentStatusUnpaid, EntitlementStatus: EntitlementStatusPending, PaymentChannel: ChannelMock, ExpiresAt: time.Now().Add(time.Minute)}

	member.On("GetPurchasablePlan", mock.Anything, "monthly").Return(plan, nil)
	store.On("CreateOrder", mock.Anything, mock.MatchedBy(func(input *CreateOrderInput) bool {
		return input.UserID == 7 && input.PlanID == 9 && input.Amount == 990 && input.IdempotencyKey != nil && *input.IdempotencyKey == key
	})).Return(order, true, nil)

	resp, err := svc.CreateOrder(context.Background(), 7, CreateOrderRequest{PlanCode: "monthly", IdempotencyKey: &key})

	require.NoError(t, err)
	assert.Equal(t, "MO1", resp.OrderNo)
	assert.Equal(t, "monthly", resp.PlanCode)
}

func TestPayMockRejectsExpiredOrderAndCloses(t *testing.T) {
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member)
	expired := &Order{ID: 1, OrderNo: "MO1", UserID: 7, Amount: 990, OrderStatus: OrderStatusAwaitingPayment, PaymentStatus: PaymentStatusUnpaid, ExpiresAt: time.Now().Add(-time.Minute)}

	store.On("GetOrderForUser", mock.Anything, int64(7), "MO1").Return(expired, "monthly", nil)
	store.On("CloseOrder", mock.Anything, "MO1").Return(&Order{OrderNo: "MO1", OrderStatus: OrderStatusClosed}, nil)

	resp, err := svc.PayMock(context.Background(), 7, "MO1")

	require.ErrorIs(t, err, ErrOrderExpired)
	assert.Nil(t, resp)
	store.AssertCalled(t, "CloseOrder", mock.Anything, "MO1")
}

func TestPayMockSuccessfulFlowGrantsMembershipAndFulfillsOrder(t *testing.T) {
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member)
	order := &Order{ID: 1, OrderNo: "MO1", UserID: 7, Amount: 990, OrderStatus: OrderStatusAwaitingPayment, PaymentStatus: PaymentStatusUnpaid, EntitlementStatus: EntitlementStatusPending, ExpiresAt: time.Now().Add(time.Minute)}
	paid := *order
	paid.OrderStatus = OrderStatusPaid
	paid.PaymentStatus = PaymentStatusPaid
	fulfilled := paid
	fulfilled.OrderStatus = OrderStatusFulfilled
	fulfilled.EntitlementStatus = EntitlementStatusGranted
	plan := &membership.Plan{ID: 9, PlanCode: "monthly", DurationDays: 30, PriceAmount: 990, Currency: "CNY"}

	store.On("GetOrderForUser", mock.Anything, int64(7), "MO1").Return(order, "monthly", nil)
	store.On("CreateAttempt", mock.Anything, int64(1), ChannelMock, 990).Return(&Attempt{ID: 3, PaymentOrderID: 1, AttemptNo: 1}, nil)
	store.On("MarkAttemptSuccess", mock.Anything, int64(3), "MOCKMO1-1", mock.AnythingOfType("time.Time")).Return(&Attempt{ID: 3, PaymentOrderID: 1, AttemptNo: 1, ChannelStatus: AttemptStatusSuccess}, nil)
	store.On("MarkOrderPaid", mock.Anything, int64(1), mock.AnythingOfType("time.Time")).Return(&paid, nil)
	store.On("GetOrderByNo", mock.Anything, "MO1").Return(&paid, "monthly", nil).Once()
	member.On("GetPurchasablePlan", mock.Anything, "monthly").Return(plan, nil)
	member.On("GrantFromPaidOrder", mock.Anything, mock.MatchedBy(func(req membership.GrantRequest) bool {
		return req.UserID == 7 && req.PaymentOrderID == 1 && req.DurationDays == 30 && req.IdempotencyKey == "payment-order:1"
	})).Return(&membership.UserMembership{UserID: 7, Status: membership.MembershipStatusActive}, &membership.Grant{PaymentOrderID: 1}, true, nil)
	store.On("MarkOrderFulfilled", mock.Anything, int64(1)).Return(&fulfilled, nil)
	store.On("GetOrderByNo", mock.Anything, "MO1").Return(&fulfilled, "monthly", nil).Once()

	resp, err := svc.PayMock(context.Background(), 7, "MO1")

	require.NoError(t, err)
	assert.Equal(t, OrderStatusFulfilled, resp.OrderStatus)
	assert.Equal(t, EntitlementStatusGranted, resp.EntitlementStatus)
}

func TestDuplicateCallbackReturnsExistingOrderWithoutGrant(t *testing.T) {
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member)
	order := &Order{ID: 1, OrderNo: "MO1", UserID: 7, Amount: 990, OrderStatus: OrderStatusFulfilled, PaymentStatus: PaymentStatusPaid, EntitlementStatus: EntitlementStatusGranted, ExpiresAt: time.Now().Add(time.Minute)}
	req := MockCallbackRequest{CallbackID: "cb1", OrderNo: "MO1", ChannelTradeNo: "trade1", Success: true}

	store.On("SaveCallback", mock.Anything, req, mock.Anything).Return(nil, false, ErrCallbackDuplicate)
	store.On("GetOrderByNo", mock.Anything, "MO1").Return(order, "monthly", nil)

	resp, err := svc.ProcessMockCallback(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, OrderStatusFulfilled, resp.OrderStatus)
	member.AssertNotCalled(t, "GrantFromPaidOrder", mock.Anything, mock.Anything)
}

func TestRegrantMembershipRepairsPaidOrder(t *testing.T) {
	store := new(mockPaymentStore)
	member := new(mockMembershipSvc)
	svc := NewService(store, member)
	paid := &Order{ID: 1, OrderNo: "MO1", UserID: 7, Amount: 990, OrderStatus: OrderStatusPaid, PaymentStatus: PaymentStatusPaid, EntitlementStatus: EntitlementStatusFailed, ExpiresAt: time.Now().Add(time.Minute)}
	fulfilled := *paid
	fulfilled.OrderStatus = OrderStatusFulfilled
	fulfilled.EntitlementStatus = EntitlementStatusGranted
	plan := &membership.Plan{ID: 9, PlanCode: "monthly", DurationDays: 30, PriceAmount: 990, Currency: "CNY"}

	store.On("GetOrderByNo", mock.Anything, "MO1").Return(paid, "monthly", nil).Once()
	store.On("GetOrderByNo", mock.Anything, "MO1").Return(paid, "monthly", nil).Once()
	member.On("GetPurchasablePlan", mock.Anything, "monthly").Return(plan, nil)
	member.On("GrantFromPaidOrder", mock.Anything, mock.AnythingOfType("membership.GrantRequest")).Return(&membership.UserMembership{UserID: 7, Status: membership.MembershipStatusActive}, &membership.Grant{PaymentOrderID: 1}, true, nil)
	store.On("MarkOrderFulfilled", mock.Anything, int64(1)).Return(&fulfilled, nil)
	store.On("GetOrderByNo", mock.Anything, "MO1").Return(&fulfilled, "monthly", nil).Once()

	resp, err := svc.RegrantMembership(context.Background(), "MO1")

	require.NoError(t, err)
	assert.Equal(t, OrderStatusFulfilled, resp.OrderStatus)
	assert.Equal(t, EntitlementStatusGranted, resp.EntitlementStatus)
}

func TestEnsurePayableRejectsFulfilledOrder(t *testing.T) {
	err := ensurePayable(&Order{OrderStatus: OrderStatusFulfilled, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	require.ErrorIs(t, err, ErrOrderNotPayable)
}

func TestToOrderResponseIncludesUserID(t *testing.T) {
	order := &Order{
		ID:                1,
		OrderNo:           "MO1",
		UserID:            42,
		Subject:           "月度会员",
		Amount:            990,
		Currency:          "CNY",
		OrderStatus:       OrderStatusAwaitingPayment,
		PaymentStatus:     PaymentStatusUnpaid,
		EntitlementStatus: EntitlementStatusPending,
		PaymentChannel:    ChannelMock,
		ExpiresAt:         time.Now().Add(time.Hour),
		CreatedAt:         time.Now(),
	}
	resp := ToOrderResponse(order, "monthly")
	assert.Equal(t, int64(42), resp.UserID)
	assert.Equal(t, "MO1", resp.OrderNo)
	assert.Equal(t, "monthly", resp.PlanCode)
}
