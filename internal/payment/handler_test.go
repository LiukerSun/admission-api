package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPaymentService struct {
	processMockCallbackFn func(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error)
}

func (s stubPaymentService) CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) ListMyOrders(ctx context.Context, userID int64, page, pageSize int) (*OrderListResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) GetMyOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) PayMock(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) ProcessMockCallback(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error) {
	return s.processMockCallbackFn(ctx, req)
}

func (s stubPaymentService) Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) (*OrderListResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) GetAdminOrder(ctx context.Context, orderNo string) (*AdminOrderDetailResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) CloseAdminOrder(ctx context.Context, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) RedetectAdmin(ctx context.Context, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func (s stubPaymentService) RegrantMembership(ctx context.Context, orderNo string) (*OrderResponse, error) {
	panic("unexpected call")
}

func TestMockCallbackRejectsUnauthorizedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	handler := NewHandler(stubPaymentService{
		processMockCallbackFn: func(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error) {
			called = true
			return &OrderResponse{}, nil
		},
	}, HandlerOptions{MockCallbackSecret: "expected-secret"})

	r := gin.New()
	r.POST("/api/v1/payment/callbacks/mock", handler.MockCallback)

	for _, tc := range []struct {
		name   string
		header string
	}{
		{name: "missing header"},
		{name: "wrong header", header: "wrong-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/callbacks/mock", strings.NewReader(`{"callback_id":"cb1","order_no":"MO1","channel_trade_no":"trade1","success":true}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.header != "" {
				req.Header.Set(MockCallbackSecretHeader, tc.header)
			}

			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), "unauthorized mock callback")
			assert.False(t, called)
		})
	}
}

func TestMockCallbackAcceptsSignedInternalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	handler := NewHandler(stubPaymentService{
		processMockCallbackFn: func(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error) {
			called = true
			return &OrderResponse{
				OrderNo:           req.OrderNo,
				OrderStatus:       OrderStatusPaid,
				PaymentStatus:     PaymentStatusPaid,
				EntitlementStatus: EntitlementStatusPending,
				CreatedAt:         time.Now(),
				ExpiresAt:         time.Now().Add(time.Minute),
			}, nil
		},
	}, HandlerOptions{MockCallbackSecret: "expected-secret"})

	r := gin.New()
	r.POST("/api/v1/payment/callbacks/mock", handler.MockCallback)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/callbacks/mock", strings.NewReader(`{"callback_id":"cb1","order_no":"MO1","channel_trade_no":"trade1","success":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(MockCallbackSecretHeader, "expected-secret")

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
	assert.Contains(t, w.Body.String(), `"order_no":"MO1"`)
}

func TestMockCallbackAllowsAnonymousAccessInDevelopment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	called := false
	handler := NewHandler(stubPaymentService{
		processMockCallbackFn: func(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error) {
			called = true
			return &OrderResponse{
				OrderNo:           req.OrderNo,
				OrderStatus:       OrderStatusPaid,
				PaymentStatus:     PaymentStatusPaid,
				EntitlementStatus: EntitlementStatusPending,
				CreatedAt:         time.Now(),
				ExpiresAt:         time.Now().Add(time.Minute),
			}, nil
		},
	}, HandlerOptions{AllowAnonymousMockCallback: true})

	r := gin.New()
	r.POST("/api/v1/payment/callbacks/mock", handler.MockCallback)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/callbacks/mock", strings.NewReader(`{"callback_id":"cb1","order_no":"MO1","channel_trade_no":"trade1","success":true}`))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}
