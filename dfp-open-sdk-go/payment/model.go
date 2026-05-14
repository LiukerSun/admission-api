package payment

import (
	"errors"
	"time"
)

const (
	ChannelOpenBank = "openbank"

	OrderStatusAwaitingPayment = "awaiting_payment"
	OrderStatusPaid            = "paid"
	OrderStatusFulfilled       = "fulfilled"
	OrderStatusClosed          = "closed"
	OrderStatusFailed          = "failed"
	OrderStatusRefunded        = "refunded"

	PaymentStatusUnpaid = "unpaid"
	PaymentStatusPaying = "paying"
	PaymentStatusPaid   = "paid"
	PaymentStatusFailed = "failed"

	EntitlementStatusPending = "pending"
	EntitlementStatusGranted = "granted"
	EntitlementStatusFailed  = "failed"
	EntitlementStatusRevoked = "revoked"

	AttemptStatusCreated = "created"
	AttemptStatusPending = "pending"
	AttemptStatusSuccess = "success"
	AttemptStatusFailed  = "failed"

	RefundStatusPendingReview = "pending_review"
	RefundStatusRejected      = "rejected"
	RefundStatusApproved      = "approved"
	RefundStatusProcessing    = "processing"
	RefundStatusSuccess       = "success"
	RefundStatusFailed        = "failed"
)

type (
	Order struct {
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

	Attempt struct {
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

	Callback struct {
		ID             int64      `json:"id"`
		Channel        string     `json:"channel"`
		CallbackID     string     `json:"callback_id"`
		ChannelTradeNo *string    `json:"channel_trade_no,omitempty"`
		Processed      bool       `json:"processed"`
		ProcessedAt    *time.Time `json:"processed_at,omitempty"`
		ProcessError   *string    `json:"process_error,omitempty"`
		CreatedAt      time.Time  `json:"created_at"`
	}

	Refund struct {
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
		InitiatedBy      *int64     `json:"initiated_by,omitempty"`
		ReviewNote       *string    `json:"review_note,omitempty"`
		ReviewedBy       *int64     `json:"reviewed_by,omitempty"`
		ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
		RefundedAt       *time.Time `json:"refunded_at,omitempty"`
		CreatedAt        time.Time  `json:"created_at"`
		UpdatedAt        time.Time  `json:"updated_at"`
	}

	PlanInfo struct {
		ID           int64
		PlanCode     string
		PlanName     string
		DurationDays int
		PriceAmount  int
		Currency     string
	}
)

type (
	CreateOrderRequest struct {
		PlanCode       string  `json:"plan_code" validate:"required,oneof=monthly quarterly yearly"`
		IdempotencyKey *string `json:"idempotency_key,omitempty" validate:"omitempty,max=128"`
	}

	CreateOrderResponse struct {
		OrderNo           string     `json:"order_no"`
		UserID            int64      `json:"user_id"`
		PlanCode          string     `json:"plan_code"`
		Subject           string     `json:"subject"`
		Amount            int        `json:"amount"`
		Currency          string     `json:"currency"`
		OrderStatus       string     `json:"order_status"`
		PaymentStatus     string     `json:"payment_status"`
		EntitlementStatus string     `json:"entitlement_status"`
		PaymentChannel    string     `json:"payment_channel"`
		ExpiresAt         time.Time  `json:"expires_at"`
		PaidAt            *time.Time `json:"paid_at,omitempty"`
		CreatedAt         time.Time  `json:"created_at"`
	}

	OpenBankPayResponse struct {
		OrderNo    string    `json:"order_no"`
		CodeURL    string    `json:"code_url"`
		CodeImgURL string    `json:"code_img_url"`
		ExpiresAt  time.Time `json:"expires_at"`
	}

	RefundOrderRequest struct {
		Amount *int   `json:"amount,omitempty" validate:"omitempty,min=1"`
		Reason string `json:"reason" validate:"required,min=2,max=256"`
	}

	RefundOrderResponse struct {
		Order        OrderResponse `json:"order"`
		RefundNo     string        `json:"refund_no"`
		RefundAmount int           `json:"refund_amount"`
		Status       string        `json:"status"`
	}

	ReviewRefundRequest struct {
		ReviewNote string `json:"review_note,omitempty" validate:"max=512"`
	}

	OrderResponse struct {
		OrderNo           string     `json:"order_no"`
		UserID            int64      `json:"user_id"`
		PlanCode          string     `json:"plan_code,omitempty"`
		Subject           string     `json:"subject"`
		Amount            int        `json:"amount"`
		Currency          string     `json:"currency"`
		OrderStatus       string     `json:"order_status"`
		PaymentStatus     string     `json:"payment_status"`
		EntitlementStatus string     `json:"entitlement_status"`
		PaymentChannel    string     `json:"payment_channel"`
		ExpiresAt         time.Time  `json:"expires_at"`
		PaidAt            *time.Time `json:"paid_at,omitempty"`
		ClosedAt          *time.Time `json:"closed_at,omitempty"`
		CreatedAt         time.Time  `json:"created_at"`
	}

	AdminOrderDetailResponse struct {
		Order     OrderResponse `json:"order"`
		Attempts  []*Attempt    `json:"attempts"`
		Callbacks []*Callback   `json:"callbacks"`
	}
)

// API request/response types for OpenBank operations.

type NativeOrderRequest struct {
	Service          string // unified.trade.native
	Version          string
	MchID            string
	OutTradeNo       string
	Body             string
	TotalFee         string // in fen string
	MchCreateIP      string
	NotifyURL        string
	Attach           string
	TimeStart        string // yyyyMMddHHmmss
	TimeExpire       string // yyyyMMddHHmmss
	OpUserID         string
	DeviceLocation   string
	LimitCreditPay   string
	GoodsDetail      string // JSON
	TerminalInfoJSON string // JSON of terminal_info fields (form-style keys)
}

type NativeOrderResponse struct {
	Status        string `json:"status"`
	ResultCode    string `json:"result_code"`
	CodeURL       string `json:"code_url"`
	CodeImgURL    string `json:"code_img_url"`
	Message       string `json:"message"`
	ErrCode       string `json:"err_code"`
	ErrMsg        string `json:"err_msg"`
	TransactionID string `json:"transaction_id"`
	OutTradeNo    string `json:"out_trade_no"`
	MchID         string `json:"mch_id"`
	NonceStr      string `json:"nonce_str"`
}

type TradeQueryRequest struct {
	Service       string // unified.trade.query
	Version       string
	MchID          string
	OutTradeNo    string
	TransactionID string
}

type TradeQueryResponse struct {
	Status           string `json:"status"`
	ResultCode       string `json:"result_code"`
	TradeState       string `json:"trade_state"`
	TradeType        string `json:"trade_type"`
	TransactionID    string `json:"transaction_id"`
	OutTradeNo       string `json:"out_trade_no"`
	OutTransactionID string `json:"out_transaction_id"`
	TotalFee         string `json:"total_fee"`
	Message          string `json:"message"`
	ErrCode          string `json:"err_code"`
	ErrMsg           string `json:"err_msg"`
	MchID            string `json:"mch_id"`
	Attach           string `json:"attach"`
	TimeEnd          string `json:"time_end"`
}

type RefundAPIParams struct {
	Service       string // unified.trade.refund
	Version       string
	MchID          string
	OutTradeNo    string
	TransactionID string
	OutRefundNo   string
	TotalFee      string // in fen
	RefundFee     string // in fen
	OpUserID      string
	RefundChannel string
}

type RefundAPIResponse struct {
	Status             string `json:"status"`
	ResultCode         string `json:"result_code"`
	RefundID           string `json:"refund_id"`
	OutRefundNo        string `json:"out_refund_no"`
	OutTradeNo         string `json:"out_trade_no"`
	TransactionID      string `json:"transaction_id"`
	OutTransactionID   string `json:"out_transaction_id"`
	RefundChannel      string `json:"refund_channel"`
	RefundFee          string `json:"refund_fee"`
	CouponRefundFee    string `json:"coupon_refund_fee"`
	TradeType          string `json:"trade_type"`
	Message            string `json:"message"`
	ErrCode            string `json:"err_code"`
	ErrMsg             string `json:"err_msg"`
	MchID              string `json:"mch_id"`
}

type RefundQueryAPIParams struct {
	Service       string // unified.trade.refundquery
	Version       string
	MchID          string
	OutTradeNo    string
	TransactionID string
	OutRefundNo   string
	RefundID      string
}

type RefundQueryAPIResponse struct {
	Status        string            `json:"status"`
	ResultCode    string            `json:"result_code"`
	TransactionID string            `json:"transaction_id"`
	OutTradeNo    string            `json:"out_trade_no"`
	RefundCount   string            `json:"refund_count"`
	TradeType     string            `json:"trade_type"`
	RefundDetails []RefundDetailAPI `json:"refundDetails"`
	Message       string            `json:"message"`
	ErrCode       string            `json:"err_code"`
	ErrMsg        string            `json:"err_msg"`
}

type RefundDetailAPI struct {
	OutRefundNo   string `json:"out_refund_no"`
	RefundID      string `json:"refund_id"`
	OutRefundID   string `json:"out_refund_id"`
	RefundChannel string `json:"refund_channel"`
	RefundFee     string `json:"refund_fee"`
	RefundStatus  string `json:"refund_status"`
	RefundTime    string `json:"refund_time"`
}

type CloseOrderAPIParams struct {
	Service    string // unified.trade.close
	Version    string
	MchID       string
	OutTradeNo string
}

type CloseOrderAPIResponse struct {
	Status     string `json:"status"`
	ResultCode string `json:"result_code"`
	Message    string `json:"message"`
	ErrCode    string `json:"err_code"`
	ErrMsg     string `json:"err_msg"`
}

// CallbackPayload is the decrypted notification from OpenBank.
type CallbackPayload struct {
	Version           string `json:"version"`
	MchID             string `json:"mch_id"`
	NonceStr          string `json:"nonce_str"`
	ResultCode        string `json:"result_code"`
	Status            string `json:"status"`
	TransactionID     string `json:"transaction_id"`
	OutTradeNo        string `json:"out_trade_no"`
	OutTransactionID  string `json:"out_transaction_id"`
	TradeType         string `json:"trade_type"`
	TotalFee          string `json:"total_fee"`
	PayResult         string `json:"pay_result"`
	Attach            string `json:"attach"`
	TimeEnd           string `json:"time_end"`
	OpenID            string `json:"openid"`
	ErrCode           string `json:"err_code"`
	ErrMsg            string `json:"err_msg"`
}

var (
	ErrOrderNotFound          = errors.New("payment order not found")
	ErrOrderAccessDenied      = errors.New("payment order access denied")
	ErrOrderNotPayable        = errors.New("payment order is not payable")
	ErrOrderExpired           = errors.New("payment order expired")
	ErrIdempotencyConflict    = errors.New("idempotency key conflict")
	ErrCallbackDuplicate      = errors.New("payment callback already processed")
	ErrOpenBankNotConfigured  = errors.New("openbank is not configured")
	ErrOpenBankSignature      = errors.New("openbank callback signature verification failed")
	ErrChannelMismatch        = errors.New("payment order channel does not match")
	ErrOrderNotRefundable     = errors.New("payment order is not refundable")
	ErrRefundAmountExceeded   = errors.New("refund amount exceeds remaining order amount")
	ErrRefundNotFound         = errors.New("refund not found")
	ErrRefundPendingExists    = errors.New("a pending refund request already exists for this order")
	ErrRefundNotPendingReview = errors.New("refund is not in pending_review state")
	ErrRefundReviewNoteMissing = errors.New("review_note is required when rejecting a refund")
	ErrPlanNotFound            = errors.New("membership plan not found")
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
