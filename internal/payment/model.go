package payment

import (
	"errors"
	"time"
)

const (
	ProductTypeMembership = "membership"

	ChannelMock   = "mock"
	ChannelAlipay = "alipay"

	OrderStatusAwaitingPayment = "awaiting_payment"
	OrderStatusPaid            = "paid"
	OrderStatusFulfilled       = "fulfilled"
	OrderStatusClosed          = "closed"
	OrderStatusFailed          = "failed"

	PaymentStatusUnpaid = "unpaid"
	PaymentStatusPaying = "paying"
	PaymentStatusPaid   = "paid"
	PaymentStatusFailed = "failed"

	EntitlementStatusPending = "pending"
	EntitlementStatusGranted = "granted"
	EntitlementStatusFailed  = "failed"

	AttemptStatusCreated = "created"
	AttemptStatusPending = "pending"
	AttemptStatusSuccess = "success"
	AttemptStatusFailed  = "failed"
	AttemptStatusClosed  = "closed"

	RefundStatusProcessing = "processing"
	RefundStatusSuccess    = "success"
	RefundStatusFailed     = "failed"
)

type Order struct {
	ID                int64      `json:"id"`
	OrderNo           string     `json:"order_no"`
	UserID            int64      `json:"user_id"`
	ProductType       string     `json:"product_type"`
	ProductRefID      int64      `json:"product_ref_id"`
	Subject           string     `json:"subject"`
	Amount            int        `json:"amount"`
	Currency          string     `json:"currency"`
	OrderStatus       string     `json:"order_status"`
	PaymentStatus     string     `json:"payment_status"`
	EntitlementStatus string     `json:"entitlement_status"`
	PaymentChannel    string     `json:"payment_channel"`
	IdempotencyKey    *string    `json:"idempotency_key,omitempty"`
	ExpiresAt         time.Time  `json:"expires_at"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Attempt struct {
	ID                 int64      `json:"id"`
	PaymentOrderID     int64      `json:"payment_order_id"`
	AttemptNo          int        `json:"attempt_no"`
	Channel            string     `json:"channel"`
	ChannelTradeNo     *string    `json:"channel_trade_no,omitempty"`
	ChannelStatus      string     `json:"channel_status"`
	Amount             int        `json:"amount"`
	CallbackReceivedAt *time.Time `json:"callback_received_at,omitempty"`
	SuccessAt          *time.Time `json:"success_at,omitempty"`
	FailedAt           *time.Time `json:"failed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Callback struct {
	ID             int64      `json:"id"`
	Channel        string     `json:"channel"`
	CallbackID     string     `json:"callback_id"`
	ChannelTradeNo *string    `json:"channel_trade_no,omitempty"`
	Processed      bool       `json:"processed"`
	ProcessedAt    *time.Time `json:"processed_at,omitempty"`
	ProcessError   *string    `json:"process_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type CreateOrderRequest struct {
	PlanCode       string  `json:"plan_code" validate:"required,oneof=monthly quarterly yearly" example:"monthly"`
	Channel        *string `json:"channel,omitempty" validate:"omitempty,oneof=mock alipay" example:"alipay"`
	IdempotencyKey *string `json:"idempotency_key,omitempty" validate:"omitempty,max=128" example:"checkout-123"`
}

type OrderResponse struct {
	OrderNo           string     `json:"order_no" example:"MO202604231200000001"`
	UserID            int64      `json:"user_id" example:"1"`
	PlanCode          string     `json:"plan_code,omitempty" example:"monthly"`
	Subject           string     `json:"subject" example:"月度会员"`
	Amount            int        `json:"amount" example:"990"`
	Currency          string     `json:"currency" example:"CNY"`
	OrderStatus       string     `json:"order_status" example:"awaiting_payment"`
	PaymentStatus     string     `json:"payment_status" example:"unpaid"`
	EntitlementStatus string     `json:"entitlement_status" example:"pending"`
	PaymentChannel    string     `json:"payment_channel" example:"mock"`
	ExpiresAt         time.Time  `json:"expires_at" example:"2026-04-23T10:15:00Z"`
	PaidAt            *time.Time `json:"paid_at,omitempty" example:"2026-04-23T10:01:00Z"`
	ClosedAt          *time.Time `json:"closed_at,omitempty" example:"2026-04-23T10:15:00Z"`
	CreatedAt         time.Time  `json:"created_at" example:"2026-04-23T10:00:00Z"`
}

type OrderListResponse struct {
	Items    []OrderResponse `json:"items"`
	Total    int64           `json:"total" example:"1"`
	Page     int             `json:"page" example:"1"`
	PageSize int             `json:"page_size" example:"20"`
}

type AdminOrderDetailResponse struct {
	Order     OrderResponse `json:"order"`
	Attempts  []*Attempt    `json:"attempts"`
	Callbacks []*Callback   `json:"callbacks"`
}

type MockCallbackRequest struct {
	CallbackID     string `json:"callback_id" validate:"required,max=128" example:"mock-callback-1"`
	OrderNo        string `json:"order_no" validate:"required" example:"MO202604231200000001"`
	ChannelTradeNo string `json:"channel_trade_no" validate:"required,max=128" example:"MOCK202604231200000001"`
	Success        bool   `json:"success" example:"true"`
}

type AdminOrderFilter struct {
	OrderNo     string
	UserID      int64
	PlanCode    string
	Channel     string
	OrderStatus string
}

type CreateOrderInput struct {
	UserID         int64
	PlanID         int64
	PlanCode       string
	PlanName       string
	DurationDays   int
	Amount         int
	Currency       string
	Channel        string
	IdempotencyKey *string
	Now            time.Time
}

type AlipayPayResponse struct {
	OrderNo   string    `json:"order_no" example:"MO202605101200000001"`
	PayURL    string    `json:"pay_url" example:"https://openapi-sandbox.dl.alipaydev.com/gateway.do?..."`
	ExpiresAt time.Time `json:"expires_at"`
}

type Refund struct {
	ID               int64      `json:"id"`
	PaymentOrderID   int64      `json:"payment_order_id"`
	RefundNo         string     `json:"refund_no"`
	OutRequestNo     string     `json:"out_request_no"`
	Channel          string     `json:"channel"`
	ChannelRefundNo  *string    `json:"channel_refund_no,omitempty"`
	RefundAmount     int        `json:"refund_amount"`
	TotalOrderAmount int        `json:"total_order_amount"`
	RefundReason     string     `json:"refund_reason"`
	Status           string     `json:"status"`
	InitiatedBy      int64      `json:"initiated_by"`
	RefundedAt       *time.Time `json:"refunded_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateRefundInput struct {
	PaymentOrderID   int64
	RefundNo         string
	OutRequestNo     string
	Channel          string
	RefundAmount     int
	TotalOrderAmount int
	RefundReason     string
	InitiatedBy      int64
}

type RefundOrderRequest struct {
	Amount *int   `json:"amount,omitempty" validate:"omitempty,min=1" example:"990"`
	Reason string `json:"reason,omitempty" validate:"omitempty,max=256" example:"用户取消订单"`
}

type RefundOrderResponse struct {
	Order        OrderResponse `json:"order"`
	RefundNo     string        `json:"refund_no" example:"RF202605121200000001"`
	RefundAmount int           `json:"refund_amount" example:"990"`
	Status       string        `json:"status" example:"success"`
}

var (
	ErrChannelMismatch      = errors.New("payment order channel does not match")
	ErrAlipayNotConfigured  = errors.New("alipay is not configured")
	ErrAlipaySignature      = errors.New("alipay callback signature verification failed")
	ErrOrderNotRefundable   = errors.New("payment order is not refundable")
	ErrRefundAmountExceeded = errors.New("refund amount exceeds remaining order amount")
	ErrRefundNotFound       = errors.New("refund not found")
)

func ToOrderResponse(o *Order, planCode string) OrderResponse {
	return OrderResponse{
		OrderNo:           o.OrderNo,
		UserID:            o.UserID,
		PlanCode:          planCode,
		Subject:           o.Subject,
		Amount:            o.Amount,
		Currency:          o.Currency,
		OrderStatus:       o.OrderStatus,
		PaymentStatus:     o.PaymentStatus,
		EntitlementStatus: o.EntitlementStatus,
		PaymentChannel:    o.PaymentChannel,
		ExpiresAt:         o.ExpiresAt,
		PaidAt:            o.PaidAt,
		ClosedAt:          o.ClosedAt,
		CreatedAt:         o.CreatedAt,
	}
}
