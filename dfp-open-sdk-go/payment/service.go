package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// MembershipOps abstracts membership operations needed by the payment service.
// The main application adapts its membership.Service to this interface.
type MembershipOps interface {
	GetPurchasablePlan(ctx context.Context, planCode string) (*PlanInfo, error)
	GrantFromPaidOrder(ctx context.Context, userID, paymentOrderID int64, durationDays int, idempotencyKey string) error
	RevokeFromOrder(ctx context.Context, userID, paymentOrderID int64, idempotencyKey string) error
}

// Service defines the OpenBank payment business operations.
type Service interface {
	CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*CreateOrderResponse, error)
	Pay(ctx context.Context, userID int64, orderNo string, clientIP string) (*OpenBankPayResponse, error)
	ProcessCallback(ctx context.Context, keyID, timestamp, nonce, sign, body string) (*OrderResponse, error)
	Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	RefundOrder(ctx context.Context, userID int64, orderNo string, req RefundOrderRequest) (*RefundOrderResponse, error)
	ApproveRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*RefundOrderResponse, error)
	RejectRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*Refund, error)
	ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error)
	QueryRefund(ctx context.Context, orderNo, refundNo string) (*Refund, error)
	ListOrderRefunds(ctx context.Context, userID int64, orderNo string) ([]*Refund, error)
	GetOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	CloseOrder(ctx context.Context, orderNo string) (*OrderResponse, error)
	Redetect(ctx context.Context, orderNo string) (*OrderResponse, error)
}

type service struct {
	store        Store
	client       Client
	membership   MembershipOps
	notifyURL    string
	returnURL    string
}

func NewService(store Store, client Client, membership MembershipOps, notifyURL, returnURL string) Service {
	return &service{
		store:      store,
		client:     client,
		membership: membership,
		notifyURL:  notifyURL,
		returnURL:  returnURL,
	}
}

func (s *service) CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*CreateOrderResponse, error) {
	plan, err := s.membership.GetPurchasablePlan(ctx, req.PlanCode)
	if err != nil {
		return nil, err
	}

	o, _, err := s.store.CreateOrder(ctx, &CreateOrderInput{
		UserID:         userID,
		PlanID:         plan.ID,
		PlanCode:       plan.PlanCode,
		PlanName:       plan.PlanName,
		DurationDays:   plan.DurationDays,
		Amount:         plan.PriceAmount,
		Currency:       plan.Currency,
		IdempotencyKey: req.IdempotencyKey,
		Now:            time.Now(),
	})
	if err != nil {
		return nil, err
	}
	return &CreateOrderResponse{
		OrderNo:           o.OrderNo,
		UserID:            o.UserID,
		PlanCode:          plan.PlanCode,
		Subject:           o.Subject,
		Amount:            o.Amount,
		Currency:          o.Currency,
		OrderStatus:       o.OrderStatus,
		PaymentStatus:     o.PaymentStatus,
		EntitlementStatus: o.EntitlementStatus,
		PaymentChannel:    o.PaymentChannel,
		ExpiresAt:         o.ExpiresAt,
		CreatedAt:         o.CreatedAt,
	}, nil
}

func (s *service) Pay(ctx context.Context, userID int64, orderNo string, clientIP string) (*OpenBankPayResponse, error) {
	if s.client == nil {
		return nil, ErrOpenBankNotConfigured
	}
	o, _, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	if err := ensurePayable(o, time.Now()); err != nil {
		if err == ErrOrderExpired {
			_, _ = s.store.CloseOrder(ctx, orderNo)
		}
		return nil, err
	}
	if o.PaymentChannel != ChannelOpenBank {
		return nil, ErrChannelMismatch
	}

	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelOpenBank, o.Amount)
	if err != nil {
		return nil, fmt.Errorf("create openbank attempt: %w", err)
	}

	now := time.Now()
	totalFee := strconv.Itoa(o.Amount)
	resp, err := s.client.CreateNativeOrder(&NativeOrderRequest{
		Service:     "unified.trade.native",
		Version:     "3.0",
		OutTradeNo:  o.OrderNo,
		Body:        o.Subject,
		TotalFee:    totalFee,
		MchCreateIP: clientIP,
		NotifyURL:   s.notifyURL,
		TimeStart:   now.Format("20060102150405"),
		TimeExpire:  o.ExpiresAt.Format("20060102150405"),
	})
	if err != nil {
		return nil, fmt.Errorf("openbank create native order: %w", err)
	}
	if resp.Status != "0" || resp.ResultCode != "0" {
		return nil, fmt.Errorf("openbank create native order failed: status=%s result_code=%s msg=%s err_code=%s",
			resp.Status, resp.ResultCode, resp.Message, resp.ErrCode)
	}

	requestPayload, _ := json.Marshal(map[string]string{
		"out_trade_no":  o.OrderNo,
		"total_fee":     totalFee,
		"subject":       o.Subject,
		"service":       "unified.trade.native",
		"attempt_no":    strconv.Itoa(attempt.AttemptNo),
	})
	_ = s.store.UpdateAttemptRequestPayload(ctx, attempt.ID, requestPayload)

	return &OpenBankPayResponse{
		OrderNo:    o.OrderNo,
		CodeURL:    resp.CodeURL,
		CodeImgURL: resp.CodeImgURL,
		ExpiresAt:  o.ExpiresAt,
	}, nil
}

func (s *service) ProcessCallback(ctx context.Context, keyID, timestamp, nonce, sign, body string) (*OrderResponse, error) {
	decrypted, err := s.client.VerifyAndDecryptCallback(keyID, timestamp, nonce, sign, body)
	if err != nil {
		slog.Error("openbank callback verification failed", "error", err)
		return nil, ErrOpenBankSignature
	}

	var payload CallbackPayload
	if err := json.Unmarshal([]byte(decrypted), &payload); err != nil {
		return nil, fmt.Errorf("parse openbank callback payload: %w", err)
	}

	if payload.TransactionID == "" {
		return nil, fmt.Errorf("openbank callback: missing transaction_id")
	}

	cbPayload, _ := json.Marshal(payload)

	cb, _, err := s.store.SaveCallback(ctx, payload.TransactionID, payload.OutTradeNo, cbPayload)
	if err != nil {
		if err == ErrCallbackDuplicate {
			o, _, getErr := s.store.GetOrderByNo(ctx, payload.OutTradeNo)
			if getErr != nil {
				return nil, getErr
			}
			resp := ToOrderResponse(o, "")
			return &resp, nil
		}
		return nil, err
	}

	var processErr *string
	o, _, err := s.store.GetOrderByNo(ctx, payload.OutTradeNo)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}

	expectedAmount := strconv.Itoa(o.Amount)
	if payload.TotalFee != expectedAmount {
		msg := fmt.Sprintf("amount mismatch: expected %s, got %s", expectedAmount, payload.TotalFee)
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, fmt.Errorf("openbank callback amount mismatch")
	}

	// Check if payment was successful. PayResult=0 means success for WeChat/Alipay JSPay.
	if payload.PayResult != "0" && payload.ResultCode != "0" {
		msg := fmt.Sprintf("openbank callback: payment not successful, pay_result=%s result_code=%s", payload.PayResult, payload.ResultCode)
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, fmt.Errorf("openbank callback payment failed")
	}

	if o.OrderStatus != OrderStatusPaid && o.OrderStatus != OrderStatusFulfilled {
		if err := ensurePayable(o, time.Now()); err != nil {
			if err == ErrOrderExpired {
				_, _ = s.store.CloseOrder(ctx, payload.OutTradeNo)
			}
			msg := err.Error()
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, err
		}
	}

	// Check for existing attempt (duplicate callback)
	if existingAttempt, tradeErr := s.store.GetAttemptByChannelTrade(ctx, ChannelOpenBank, payload.TransactionID); tradeErr == nil {
		if existingAttempt.PaymentOrderID != o.ID {
			msg := "channel trade belongs to another order"
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, fmt.Errorf("openbank callback trade conflict")
		}
		if o.OrderStatus == OrderStatusPaid || o.EntitlementStatus == EntitlementStatusFailed {
			if err := s.fulfillMembership(ctx, o); err != nil {
				msg := err.Error()
				processErr = &msg
				_ = s.store.MarkOrderEntitlementFailed(ctx, o.ID)
				_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
				return nil, err
			}
		}
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, nil)
		updated, _, getErr := s.store.GetOrderByNo(ctx, payload.OutTradeNo)
		if getErr != nil {
			return nil, getErr
		}
		resp := ToOrderResponse(updated, "")
		return &resp, nil
	} else if tradeErr != ErrOrderNotFound {
		msg := tradeErr.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, tradeErr
	}

	// New transaction — create attempt and mark success
	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelOpenBank, o.Amount)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, payload.TransactionID, time.Now()); err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	paid, err := s.store.MarkOrderPaid(ctx, o.ID, time.Now())
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	if err := s.fulfillMembership(ctx, paid); err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkOrderEntitlementFailed(ctx, paid.ID)
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	_ = s.store.MarkCallbackProcessed(ctx, cb.ID, nil)
	fulfilled, _, err := s.store.GetOrderByNo(ctx, payload.OutTradeNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(fulfilled, "")
	return &resp, nil
}

func (s *service) Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}

	if s.client != nil && (o.OrderStatus == OrderStatusAwaitingPayment || o.PaymentStatus == PaymentStatusUnpaid || o.PaymentStatus == PaymentStatusPaying) {
		rsp, err := s.client.TradeQuery(&TradeQueryRequest{
			Service:    "unified.trade.query",
			Version:    "3.0",
			OutTradeNo: o.OrderNo,
		})
		if err != nil {
			slog.Warn("openbank trade query failed during detect", "order_no", orderNo, "error", err)
		} else {
			switch rsp.TradeState {
			case "NOTPAY":
				// still waiting
			case "CLOSED", "REVOKED":
				_, _ = s.store.CloseOrder(ctx, orderNo)
				o, planCode, err = s.store.GetOrderByNo(ctx, orderNo)
				if err != nil {
					return nil, err
				}
			case "SUCCESS":
				tradeNo := rsp.TransactionID
				if tradeNo == "" {
					tradeNo = rsp.OutTransactionID
				}
				existingAttempt, tradeErr := s.store.GetAttemptByChannelTrade(ctx, ChannelOpenBank, tradeNo)
				if tradeErr != nil && tradeErr != ErrOrderNotFound {
					return nil, tradeErr
				}
				if tradeErr == ErrOrderNotFound {
					attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelOpenBank, o.Amount)
					if err != nil {
						return nil, fmt.Errorf("create attempt on detect: %w", err)
					}
					if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, tradeNo, time.Now()); err != nil {
						return nil, fmt.Errorf("mark attempt success on detect: %w", err)
					}
				} else if existingAttempt.ChannelStatus != AttemptStatusSuccess {
					_, err := s.store.MarkAttemptSuccess(ctx, existingAttempt.ID, tradeNo, time.Now())
					if err != nil {
						return nil, fmt.Errorf("mark attempt success on detect: %w", err)
					}
				}
				paid, err := s.store.MarkOrderPaid(ctx, o.ID, time.Now())
				if err != nil {
					return nil, fmt.Errorf("mark order paid on detect: %w", err)
				}
				o = paid
				if err := s.fulfillMembership(ctx, o); err != nil {
					_ = s.store.MarkOrderEntitlementFailed(ctx, o.ID)
					return nil, err
				}
				o, planCode, err = s.store.GetOrderByNo(ctx, orderNo)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	if o.OrderStatus == OrderStatusPaid || o.EntitlementStatus == EntitlementStatusFailed {
		if err := s.fulfillMembership(ctx, o); err != nil {
			_ = s.store.MarkOrderEntitlementFailed(ctx, o.ID)
			return nil, err
		}
		o, planCode, err = s.store.GetOrderByNo(ctx, orderNo)
		if err != nil {
			return nil, err
		}
	}
	resp := ToOrderResponse(o, planCode)
	return &resp, nil
}

func (s *service) RefundOrder(ctx context.Context, userID int64, orderNo string, req RefundOrderRequest) (*RefundOrderResponse, error) {
	o, _, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	if o.PaymentChannel != ChannelOpenBank {
		return nil, ErrChannelMismatch
	}
	if o.OrderStatus != OrderStatusPaid && o.OrderStatus != OrderStatusFulfilled {
		return nil, ErrOrderNotRefundable
	}

	refundAmount := o.Amount
	if req.Amount != nil && *req.Amount > 0 {
		refundAmount = *req.Amount
	}
	if refundAmount > o.Amount {
		return nil, ErrRefundAmountExceeded
	}

	refundNo, err := generateRefundNo(time.Now())
	if err != nil {
		slog.Error("RefundOrder: generate refund no failed", "order_no", o.OrderNo, "error", err)
		return nil, err
	}

	uid := userID
	refund, err := s.store.CreateRefundRequest(ctx, &CreateRefundInput{
		PaymentOrderID:   o.ID,
		RefundNo:         refundNo,
		OutRequestNo:     refundNo,
		Channel:          ChannelOpenBank,
		RefundAmount:     refundAmount,
		TotalOrderAmount: o.Amount,
		RefundReason:     req.Reason,
		InitiatedBy:      &uid,
	})
	if err != nil {
		return nil, err
	}

	return &RefundOrderResponse{
		Order:        ToOrderResponse(o, ""),
		RefundNo:     refund.RefundNo,
		RefundAmount: refund.RefundAmount,
		Status:       refund.Status,
	}, nil
}

func (s *service) ApproveRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*RefundOrderResponse, error) {
	if s.client == nil {
		return nil, ErrOpenBankNotConfigured
	}

	refund, err := s.store.GetRefundByNo(ctx, refundNo)
	if err != nil {
		return nil, err
	}
	if refund.Status != RefundStatusPendingReview {
		return nil, ErrRefundNotPendingReview
	}

	order, _, err := s.store.GetOrderByID(ctx, refund.PaymentOrderID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var notePtr *string
	if req.ReviewNote != "" {
		notePtr = &req.ReviewNote
	}

	if _, err := s.store.MarkRefundApproved(ctx, refund.ID, reviewerID, notePtr, now); err != nil {
		return nil, err
	}
	if _, err := s.store.MarkRefundProcessing(ctx, refund.ID); err != nil {
		slog.Error("ApproveRefund: mark processing failed", "refund_no", refundNo, "error", err)
		return nil, err
	}

	totalFee := strconv.Itoa(order.Amount)
	refundFee := strconv.Itoa(refund.RefundAmount)
	rsp, err := s.client.Refund(&RefundAPIParams{
		Service:       "unified.trade.refund",
		Version:       "3.0",
		OutTradeNo:    order.OrderNo,
		OutRefundNo:   refund.OutRequestNo,
		TotalFee:      totalFee,
		RefundFee:     refundFee,
		OpUserID:      "admin",
		RefundChannel: "ORIGINAL",
	})
	if err != nil {
		slog.Error("ApproveRefund: openbank refund call failed", "refund_no", refundNo, "error", err)
		_ = s.store.UpdateRefundFailed(ctx, refund.ID)
		return nil, fmt.Errorf("openbank refund failed: %w", err)
	}

	switch rsp.ResultCode {
	case "0":
		updated, err := s.store.UpdateRefundSuccess(ctx, refund.ID, rsp.RefundID, now)
		if err != nil {
			slog.Error("ApproveRefund: update refund success failed", "refund_no", refundNo, "error", err)
			return nil, err
		}
		if updated.RefundAmount == order.Amount {
			s.finalizeFullRefund(ctx, order, refundNo)
		}
		finalOrder, _, _ := s.store.GetOrderByID(ctx, order.ID)
		return &RefundOrderResponse{
			Order:        ToOrderResponse(finalOrder, ""),
			RefundNo:     updated.RefundNo,
			RefundAmount: updated.RefundAmount,
			Status:       updated.Status,
		}, nil
	case "2":
		_ = s.store.UpdateRefundFailed(ctx, refund.ID)
		return nil, fmt.Errorf("openbank refund failed: err_code=%s err_msg=%s", rsp.ErrCode, rsp.ErrMsg)
	default:
		// result_code=1 or unknown — keep processing, admin should query later
		slog.Warn("ApproveRefund: refund status uncertain",
			"refund_no", refundNo, "result_code", rsp.ResultCode)
		refund.Status = RefundStatusProcessing
		orderResp := ToOrderResponse(order, "")
		return &RefundOrderResponse{
			Order:        orderResp,
			RefundNo:     refund.RefundNo,
			RefundAmount: refund.RefundAmount,
			Status:       RefundStatusProcessing,
		}, nil
	}
}

func (s *service) finalizeFullRefund(ctx context.Context, order *Order, refundNo string) {
	if err := s.membership.RevokeFromOrder(ctx, order.UserID, order.ID, "refund-revoke-"+refundNo); err != nil {
		slog.Error("finalizeFullRefund: revoke membership failed",
			"order_no", order.OrderNo, "refund_no", refundNo, "error", err)
	}
	if _, err := s.store.MarkOrderRefunded(ctx, order.ID); err != nil {
		slog.Error("finalizeFullRefund: mark order refunded failed",
			"order_no", order.OrderNo, "refund_no", refundNo, "error", err)
	}
}

func (s *service) RejectRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*Refund, error) {
	if req.ReviewNote == "" {
		return nil, ErrRefundReviewNoteMissing
	}
	refund, err := s.store.GetRefundByNo(ctx, refundNo)
	if err != nil {
		return nil, err
	}
	if refund.Status != RefundStatusPendingReview {
		return nil, ErrRefundNotPendingReview
	}
	return s.store.MarkRefundRejected(ctx, refund.ID, reviewerID, req.ReviewNote, time.Now())
}

func (s *service) ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.ListPendingRefunds(ctx, page, pageSize)
}

func (s *service) QueryRefund(ctx context.Context, orderNo, refundNo string) (*Refund, error) {
	o, _, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	refund, err := s.store.GetRefundByNo(ctx, refundNo)
	if err != nil {
		return nil, err
	}
	if refund.PaymentOrderID != o.ID {
		return nil, ErrRefundNotFound
	}

	if refund.Status != RefundStatusProcessing {
		return refund, nil
	}

	rsp, err := s.client.RefundQuery(&RefundQueryAPIParams{
		Service:     "unified.trade.refundquery",
		Version:     "3.0",
		OutTradeNo:  orderNo,
		OutRefundNo: refund.OutRequestNo,
	})
	if err != nil {
		return nil, fmt.Errorf("openbank refund query failed: %w", err)
	}

	if rsp.ResultCode != "0" || len(rsp.RefundDetails) == 0 {
		return refund, nil
	}

	detail := rsp.RefundDetails[0]
	switch detail.RefundStatus {
	case "SUCCESS":
		now := time.Now()
		refund, err = s.store.UpdateRefundSuccess(ctx, refund.ID, detail.RefundID, now)
		if err != nil {
			return nil, err
		}
		if refund.RefundAmount == o.Amount {
			s.finalizeFullRefund(ctx, o, refundNo)
		}
	case "FAIL":
		_ = s.store.UpdateRefundFailed(ctx, refund.ID)
	}
	return refund, nil
}

func (s *service) ListOrderRefunds(ctx context.Context, userID int64, orderNo string) ([]*Refund, error) {
	o, _, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	return s.store.ListRefunds(ctx, o.ID)
}

func (s *service) GetOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, planCode)
	return &resp, nil
}

func (s *service) CloseOrder(ctx context.Context, orderNo string) (*OrderResponse, error) {
	o, err := s.store.CloseOrder(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, "")
	return &resp, nil
}

func (s *service) Redetect(ctx context.Context, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.OrderStatus == OrderStatusPaid || o.EntitlementStatus == EntitlementStatusFailed {
		if err := s.fulfillMembership(ctx, o); err != nil {
			_ = s.store.MarkOrderEntitlementFailed(ctx, o.ID)
			return nil, err
		}
		o, planCode, err = s.store.GetOrderByNo(ctx, orderNo)
		if err != nil {
			return nil, err
		}
	}
	resp := ToOrderResponse(o, planCode)
	return &resp, nil
}

func (s *service) fulfillMembership(ctx context.Context, o *Order) error {
	if o.EntitlementStatus == EntitlementStatusGranted && o.OrderStatus == OrderStatusFulfilled {
		return nil
	}
	_, planCode, err := s.store.GetOrderByNo(ctx, o.OrderNo)
	if err != nil {
		return err
	}
	plan, err := s.membership.GetPurchasablePlan(ctx, planCode)
	if err != nil {
		return err
	}
	if err := s.membership.GrantFromPaidOrder(ctx, o.UserID, o.ID, plan.DurationDays, fmt.Sprintf("payment-order:%d", o.ID)); err != nil {
		return err
	}
	_, err = s.store.MarkOrderFulfilled(ctx, o.ID)
	return err
}

func ensurePayable(o *Order, now time.Time) error {
	if o.OrderStatus == OrderStatusClosed || o.OrderStatus == OrderStatusFailed || o.OrderStatus == OrderStatusPaid || o.OrderStatus == OrderStatusFulfilled {
		return ErrOrderNotPayable
	}
	if o.ExpiresAt.Before(now) && o.PaymentStatus != PaymentStatusPaid {
		return ErrOrderExpired
	}
	return nil
}

func generateRefundNo(now time.Time) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refund entropy: %w", err)
	}
	return fmt.Sprintf("RF%s%s", now.Format("20060102150405"), strings.ToUpper(hex.EncodeToString(b))), nil
}
