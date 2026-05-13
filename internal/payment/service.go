package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"admission-api/internal/membership"
	"admission-api/internal/platform/alipay"
)

type Service interface {
	CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*OrderResponse, error)
	ListMyOrders(ctx context.Context, userID int64, page, pageSize int) (*OrderListResponse, error)
	GetMyOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	PayMock(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	PayAlipay(ctx context.Context, userID int64, orderNo string) (*AlipayPayResponse, error)
	ProcessMockCallback(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error)
	ProcessAlipayCallback(ctx context.Context, params map[string]string) (*OrderResponse, error)
	Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) (*OrderListResponse, error)
	GetAdminOrder(ctx context.Context, orderNo string) (*AdminOrderDetailResponse, error)
	CloseAdminOrder(ctx context.Context, orderNo string) (*OrderResponse, error)
	RedetectAdmin(ctx context.Context, orderNo string) (*OrderResponse, error)
	RegrantMembership(ctx context.Context, orderNo string) (*OrderResponse, error)
	// RefundOrder 由已认证用户调用，仅创建 pending_review 退款申请，不立即调用支付宝。
	RefundOrder(ctx context.Context, userID int64, orderNo string, req RefundOrderRequest) (*RefundOrderResponse, error)
	// ApproveRefund 由管理员调用，把 pending_review 退款转为执行：调用支付宝退款 API，成功后撤销会员、更新订单状态。
	ApproveRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*RefundOrderResponse, error)
	// RejectRefund 由管理员调用，把 pending_review 退款标记为 rejected，review_note 必填。
	RejectRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*Refund, error)
	// ListPendingRefunds 列出所有等待审核的退款申请，供管理后台使用。
	ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error)
	// QueryRefund 主动查询某笔退款的最新状态（包含向支付宝查询 processing 退款）。
	QueryRefund(ctx context.Context, orderNo, refundNo string) (*Refund, error)
	// ListOrderRefunds 列出某笔订单所有退款记录（用户自查）。
	ListOrderRefunds(ctx context.Context, userID int64, orderNo string) ([]*Refund, error)
}

type service struct {
	store             Store
	membershipService membership.Service
	alipayClient      alipay.Client
	alipayNotifyURL   string
	alipayReturnURL   string
}

func NewService(store Store, membershipService membership.Service, alipayClient alipay.Client, alipayNotifyURL, alipayReturnURL string) Service {
	return &service{
		store:             store,
		membershipService: membershipService,
		alipayClient:      alipayClient,
		alipayNotifyURL:   alipayNotifyURL,
		alipayReturnURL:   alipayReturnURL,
	}
}

func (s *service) CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*OrderResponse, error) {
	plan, err := s.membershipService.GetPurchasablePlan(ctx, req.PlanCode)
	if err != nil {
		return nil, err
	}
	channel := ChannelMock
	if req.Channel != nil && *req.Channel != "" {
		channel = *req.Channel
	}
	o, _, err := s.store.CreateOrder(ctx, &CreateOrderInput{
		UserID:         userID,
		PlanID:         plan.ID,
		PlanCode:       plan.PlanCode,
		PlanName:       plan.PlanName,
		DurationDays:   plan.DurationDays,
		Amount:         plan.PriceAmount,
		Currency:       plan.Currency,
		Channel:        channel,
		IdempotencyKey: req.IdempotencyKey,
		Now:            time.Now(),
	})
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, plan.PlanCode)
	return &resp, nil
}

func (s *service) ListMyOrders(ctx context.Context, userID int64, page, pageSize int) (*OrderListResponse, error) {
	orders, total, err := s.store.ListOrdersForUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]OrderResponse, 0, len(orders))
	for _, o := range orders {
		items = append(items, ToOrderResponse(o, ""))
	}
	return &OrderListResponse{Items: items, Total: total, Page: normalizedPage(page), PageSize: normalizedPageSize(pageSize)}, nil
}

func (s *service) GetMyOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, planCode)
	return &resp, nil
}

func (s *service) PayMock(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	if err := ensurePayable(o, time.Now()); err != nil {
		if errors.Is(err, ErrOrderExpired) {
			_, _ = s.store.CloseOrder(ctx, orderNo)
		}
		return nil, err
	}

	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelMock, o.Amount)
	if err != nil {
		return nil, fmt.Errorf("create payment attempt: %w", err)
	}
	tradeNo := fmt.Sprintf("MOCK%s-%d", o.OrderNo, attempt.AttemptNo)
	if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, tradeNo, time.Now()); err != nil {
		return nil, fmt.Errorf("mark mock attempt success: %w", err)
	}
	paid, err := s.store.MarkOrderPaid(ctx, o.ID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("mark order paid: %w", err)
	}
	if err := s.fulfillMembership(ctx, paid); err != nil {
		_ = s.store.MarkOrderEntitlementFailed(ctx, paid.ID)
		return nil, err
	}
	fulfilled, _, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(fulfilled, planCode)
	return &resp, nil
}

func (s *service) PayAlipay(ctx context.Context, userID int64, orderNo string) (*AlipayPayResponse, error) {
	if s.alipayClient == nil {
		return nil, ErrAlipayNotConfigured
	}
	o, _, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	if err := ensurePayable(o, time.Now()); err != nil {
		if errors.Is(err, ErrOrderExpired) {
			_, _ = s.store.CloseOrder(ctx, orderNo)
		}
		return nil, err
	}
	if o.PaymentChannel != ChannelAlipay {
		return nil, ErrChannelMismatch
	}

	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelAlipay, o.Amount)
	if err != nil {
		return nil, fmt.Errorf("create alipay attempt: %w", err)
	}

	totalAmount := formatAmount(o.Amount)
	req := &alipay.PagePayRequest{
		OutTradeNo:  o.OrderNo,
		Subject:     o.Subject,
		TotalAmount: totalAmount,
	}
	payURL, err := s.alipayClient.BuildPagePayURL(req)
	if err != nil {
		return nil, fmt.Errorf("alipay build page pay url: %w", err)
	}

	requestPayload, _ := json.Marshal(map[string]string{
		"out_trade_no":  req.OutTradeNo,
		"subject":       req.Subject,
		"total_amount":  req.TotalAmount,
		"product_code":  "FAST_INSTANT_TRADE_PAY",
		"attempt_no":    strconv.Itoa(attempt.AttemptNo),
	})
	_ = s.store.UpdateAttemptRequestPayload(ctx, attempt.ID, requestPayload)

	return &AlipayPayResponse{
		OrderNo:   o.OrderNo,
		PayURL:    payURL,
		ExpiresAt: o.ExpiresAt,
	}, nil
}

func (s *service) ProcessAlipayCallback(ctx context.Context, params map[string]string) (*OrderResponse, error) {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	if err := s.alipayClient.VerifySign(ctx, values); err != nil {
		slog.Error("alipay callback signature verification failed", "error", err)
		return nil, ErrAlipaySignature
	}

	tradeStatus := params["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" && tradeStatus != "TRADE_FINISHED" {
		return nil, nil
	}

	outTradeNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	totalAmount := params["total_amount"]
	notifyID := params["notify_id"]

	payload, _ := json.Marshal(params)

	cb, _, err := s.store.SaveAlipayCallback(ctx, notifyID, tradeNo, payload)
	if err != nil {
		if errors.Is(err, ErrCallbackDuplicate) {
			o, _, getErr := s.store.GetOrderByNo(ctx, outTradeNo)
			if getErr != nil {
				return nil, getErr
			}
			resp := ToOrderResponse(o, "")
			return &resp, nil
		}
		return nil, err
	}

	var processErr *string
	o, _, err := s.store.GetOrderByNo(ctx, outTradeNo)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}

	expectedAmount := formatAmount(o.Amount)
	if totalAmount != expectedAmount {
		msg := fmt.Sprintf("amount mismatch: expected %s, got %s", expectedAmount, totalAmount)
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, fmt.Errorf("alipay callback amount mismatch")
	}

	if o.OrderStatus != OrderStatusPaid && o.OrderStatus != OrderStatusFulfilled {
		if err := ensurePayable(o, time.Now()); err != nil {
			if errors.Is(err, ErrOrderExpired) {
				_, _ = s.store.CloseOrder(ctx, outTradeNo)
			}
			msg := err.Error()
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, err
		}
	}

	if existingAttempt, tradeErr := s.store.GetAttemptByChannelTrade(ctx, ChannelAlipay, tradeNo); tradeErr == nil {
		if existingAttempt.PaymentOrderID != o.ID {
			msg := "channel trade belongs to another order"
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, ErrIdempotencyConflict
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
		updated, _, getErr := s.store.GetOrderByNo(ctx, outTradeNo)
		if getErr != nil {
			return nil, getErr
		}
		resp := ToOrderResponse(updated, "")
		return &resp, nil
	} else if !errors.Is(tradeErr, ErrOrderNotFound) {
		msg := tradeErr.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, tradeErr
	}

	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelAlipay, o.Amount)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, tradeNo, time.Now()); err != nil {
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
	fulfilled, _, err := s.store.GetOrderByNo(ctx, outTradeNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(fulfilled, "")
	return &resp, nil
}

func (s *service) ProcessMockCallback(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error) {
	payload, _ := json.Marshal(req)
	cb, _, err := s.store.SaveCallback(ctx, req, payload)
	if err != nil {
		if errors.Is(err, ErrCallbackDuplicate) {
			o, planCode, getErr := s.store.GetOrderByNo(ctx, req.OrderNo)
			if getErr != nil {
				return nil, getErr
			}
			resp := ToOrderResponse(o, planCode)
			return &resp, nil
		}
		return nil, err
	}

	var processErr *string
	o, _, err := s.store.GetOrderByNo(ctx, req.OrderNo)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	if !req.Success {
		msg := "mock callback indicates payment failure"
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, ErrOrderNotPayable
	}
	if o.OrderStatus != OrderStatusPaid && o.OrderStatus != OrderStatusFulfilled {
		if err := ensurePayable(o, time.Now()); err != nil {
			if errors.Is(err, ErrOrderExpired) {
				_, _ = s.store.CloseOrder(ctx, req.OrderNo)
			}
			msg := err.Error()
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, err
		}
	}

	if existingAttempt, tradeErr := s.store.GetAttemptByChannelTrade(ctx, ChannelMock, req.ChannelTradeNo); tradeErr == nil {
		existingOrder, planCode, getErr := s.store.GetOrderByNo(ctx, req.OrderNo)
		if getErr != nil {
			msg := getErr.Error()
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, getErr
		}
		if existingAttempt.PaymentOrderID != existingOrder.ID {
			msg := "channel trade belongs to another order"
			processErr = &msg
			_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
			return nil, ErrIdempotencyConflict
		}
		if existingOrder.OrderStatus == OrderStatusPaid || existingOrder.EntitlementStatus == EntitlementStatusFailed {
			if err := s.fulfillMembership(ctx, existingOrder); err != nil {
				msg := err.Error()
				processErr = &msg
				_ = s.store.MarkOrderEntitlementFailed(ctx, existingOrder.ID)
				_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
				return nil, err
			}
			existingOrder, planCode, getErr = s.store.GetOrderByNo(ctx, req.OrderNo)
			if getErr != nil {
				return nil, getErr
			}
		}
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, nil)
		resp := ToOrderResponse(existingOrder, planCode)
		return &resp, nil
	} else if !errors.Is(tradeErr, ErrOrderNotFound) {
		msg := tradeErr.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, tradeErr
	}

	attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelMock, o.Amount)
	if err != nil {
		msg := err.Error()
		processErr = &msg
		_ = s.store.MarkCallbackProcessed(ctx, cb.ID, processErr)
		return nil, err
	}
	if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, req.ChannelTradeNo, time.Now()); err != nil {
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
	fulfilled, planCode, err := s.store.GetOrderByNo(ctx, req.OrderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(fulfilled, planCode)
	return &resp, nil
}

func (s *service) Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error) {
	o, planCode, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}

	if s.alipayClient != nil && (o.OrderStatus == OrderStatusAwaitingPayment || o.PaymentStatus == PaymentStatusUnpaid || o.PaymentStatus == PaymentStatusPaying) {
		rsp, err := s.alipayClient.TradeQuery(&alipay.TradeQueryRequest{OutTradeNo: o.OrderNo})
		if err != nil {
			slog.Warn("alipay trade query failed during detect", "order_no", orderNo, "error", err)
		} else if rsp.TradeStatus == "WAIT_BUYER_PAY" {
			// still waiting, nothing to do
		} else if rsp.TradeStatus == "TRADE_CLOSED" {
			_, _ = s.store.CloseOrder(ctx, orderNo)
			o, planCode, err = s.store.GetOrderByNo(ctx, orderNo)
			if err != nil {
				return nil, err
			}
		} else if rsp.TradeStatus == "TRADE_SUCCESS" || rsp.TradeStatus == "TRADE_FINISHED" {
			existingAttempt, tradeErr := s.store.GetAttemptByChannelTrade(ctx, ChannelAlipay, rsp.TradeNo)
			if tradeErr != nil && !errors.Is(tradeErr, ErrOrderNotFound) {
				return nil, tradeErr
			}
			if errors.Is(tradeErr, ErrOrderNotFound) {
				attempt, err := s.store.CreateAttempt(ctx, o.ID, ChannelAlipay, o.Amount)
				if err != nil {
					return nil, fmt.Errorf("create attempt on detect: %w", err)
				}
				if _, err := s.store.MarkAttemptSuccess(ctx, attempt.ID, rsp.TradeNo, time.Now()); err != nil {
					return nil, fmt.Errorf("mark attempt success on detect: %w", err)
				}
			} else if existingAttempt.ChannelStatus != AttemptStatusSuccess {
				_, err := s.store.MarkAttemptSuccess(ctx, existingAttempt.ID, rsp.TradeNo, time.Now())
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

func (s *service) ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) (*OrderListResponse, error) {
	orders, total, err := s.store.ListAdminOrders(ctx, filter, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]OrderResponse, 0, len(orders))
	for _, o := range orders {
		items = append(items, ToOrderResponse(o, ""))
	}
	return &OrderListResponse{Items: items, Total: total, Page: normalizedPage(page), PageSize: normalizedPageSize(pageSize)}, nil
}

func (s *service) GetAdminOrder(ctx context.Context, orderNo string) (*AdminOrderDetailResponse, error) {
	o, planCode, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	attempts, err := s.store.ListAttempts(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	var tradeNo *string
	if len(attempts) > 0 {
		tradeNo = attempts[0].ChannelTradeNo
	}
	channel := ChannelMock
	if len(attempts) > 0 {
		channel = attempts[0].Channel
	}
	callbacks, err := s.store.ListCallbacks(ctx, channel, tradeNo)
	if err != nil {
		return nil, err
	}
	return &AdminOrderDetailResponse{
		Order:     ToOrderResponse(o, planCode),
		Attempts:  attempts,
		Callbacks: callbacks,
	}, nil
}

func (s *service) CloseAdminOrder(ctx context.Context, orderNo string) (*OrderResponse, error) {
	o, err := s.store.CloseOrder(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, "")
	return &resp, nil
}

func (s *service) RedetectAdmin(ctx context.Context, orderNo string) (*OrderResponse, error) {
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

func (s *service) RegrantMembership(ctx context.Context, orderNo string) (*OrderResponse, error) {
	o, _, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.PaymentStatus != PaymentStatusPaid {
		return nil, ErrOrderNotPayable
	}
	if err := s.fulfillMembership(ctx, o); err != nil {
		_ = s.store.MarkOrderEntitlementFailed(ctx, o.ID)
		return nil, err
	}
	o, planCode, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	resp := ToOrderResponse(o, planCode)
	return &resp, nil
}

func (s *service) fulfillMembership(ctx context.Context, o *Order) error {
	if o.EntitlementStatus == EntitlementStatusGranted && o.OrderStatus == OrderStatusFulfilled {
		return nil
	}
	planCode := ""
	_, planCode, err := s.store.GetOrderByNo(ctx, o.OrderNo)
	if err != nil {
		return err
	}
	plan, err := s.membershipService.GetPurchasablePlan(ctx, planCode)
	if err != nil {
		return err
	}
	_, _, _, err = s.membershipService.GrantFromPaidOrder(ctx, membership.GrantRequest{
		UserID:         o.UserID,
		PaymentOrderID: o.ID,
		DurationDays:   plan.DurationDays,
		IdempotencyKey: fmt.Sprintf("payment-order:%d", o.ID),
		Now:            time.Now(),
	})
	if err != nil {
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

func normalizedPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizedPageSize(pageSize int) int {
	if pageSize < 1 || pageSize > 100 {
		return 20
	}
	return pageSize
}

// RefundOrder 由用户调用：提交退款申请，进入 pending_review 状态等待管理员审核。
// 不会立即向支付宝发起退款。
func (s *service) RefundOrder(ctx context.Context, userID int64, orderNo string, req RefundOrderRequest) (*RefundOrderResponse, error) {
	o, _, err := s.store.GetOrderForUser(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	if o.PaymentChannel != ChannelAlipay {
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
		Channel:          ChannelAlipay,
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

// ApproveRefund 管理员审批通过：
//  1. 把 pending_review 改为 approved（事务+行锁，幂等校验）
//  2. 调用支付宝退款 API
//  3. 成功后写 success；若是全额退款且会员还由该订单激活，撤销会员并把订单标记为 refunded
//
// 退款审批失败时（支付宝错误），把 refund 回滚到 failed 而非 rejected，
// 让管理员后续可以查问题再重试（重新发起新申请）。
func (s *service) ApproveRefund(ctx context.Context, refundNo string, reviewerID int64, req ReviewRefundRequest) (*RefundOrderResponse, error) {
	if s.alipayClient == nil {
		return nil, ErrAlipayNotConfigured
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

	rsp, err := s.alipayClient.Refund(&alipay.RefundRequest{
		OutTradeNo:   order.OrderNo,
		RefundAmount: formatAmount(refund.RefundAmount),
		RefundReason: refund.RefundReason,
		OutRequestNo: refund.OutRequestNo,
	})
	if err != nil {
		slog.Error("ApproveRefund: alipay refund call failed", "refund_no", refundNo, "error", err)
		_ = s.store.UpdateRefundFailed(ctx, refund.ID)
		return nil, fmt.Errorf("alipay refund failed: %w", err)
	}
	if rsp.FundChange != "Y" {
		slog.Warn("ApproveRefund: alipay refund returned fund_change!=Y",
			"refund_no", refundNo, "fund_change", rsp.FundChange)
		_ = s.store.UpdateRefundFailed(ctx, refund.ID)
		return nil, fmt.Errorf("alipay refund fund_change not Y: %s", rsp.FundChange)
	}

	updated, err := s.store.UpdateRefundSuccess(ctx, refund.ID, rsp.TradeNo, now)
	if err != nil {
		slog.Error("ApproveRefund: update refund success failed", "refund_no", refundNo, "error", err)
		return nil, err
	}

	// 仅当全额退款时联动撤销会员与更新订单状态
	if updated.RefundAmount == order.Amount {
		s.finalizeFullRefund(ctx, order, refundNo)
	}

	finalOrder, _, getErr := s.store.GetOrderByID(ctx, order.ID)
	if getErr != nil {
		finalOrder = order
	}

	return &RefundOrderResponse{
		Order:        ToOrderResponse(finalOrder, ""),
		RefundNo:     updated.RefundNo,
		RefundAmount: updated.RefundAmount,
		Status:       updated.Status,
	}, nil
}

// finalizeFullRefund 在全额退款成功后联动撤销会员、更新订单状态。
// 任何一步失败仅记录日志而不阻塞主流程——资金已经退给用户，必须保证响应能返回。
// 数据不一致由后续人工修复或定时校对脚本修正。
func (s *service) finalizeFullRefund(ctx context.Context, order *Order, refundNo string) {
	if _, _, err := s.membershipService.RevokeFromOrder(ctx, membership.RevokeRequest{
		UserID:         order.UserID,
		PaymentOrderID: order.ID,
		IdempotencyKey: fmt.Sprintf("refund-revoke-%s", refundNo),
	}); err != nil {
		slog.Error("finalizeFullRefund: revoke membership failed",
			"order_no", order.OrderNo, "refund_no", refundNo, "error", err)
	}
	if _, err := s.store.MarkOrderRefunded(ctx, order.ID); err != nil {
		slog.Error("finalizeFullRefund: mark order refunded failed",
			"order_no", order.OrderNo, "refund_no", refundNo, "error", err)
	}
}

// RejectRefund 管理员拒绝退款申请，review_note 必填。
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
	return s.store.ListPendingRefunds(ctx, normalizedPage(page), normalizedPageSize(pageSize))
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

	rsp, err := s.alipayClient.RefundQuery(&alipay.RefundQueryRequest{
		OutTradeNo:   orderNo,
		OutRequestNo: refund.OutRequestNo,
	})
	if err != nil {
		return nil, fmt.Errorf("alipay refund query failed: %w", err)
	}

	if rsp.RefundStatus == "REFUND_SUCCESS" {
		now := time.Now()
		refund, err = s.store.UpdateRefundSuccess(ctx, refund.ID, rsp.TradeNo, now)
		if err != nil {
			return nil, err
		}
	} else if rsp.RefundStatus == "REFUND_FAIL" {
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

func generateRefundNo(now time.Time) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refund entropy: %w", err)
	}
	return fmt.Sprintf("RF%s%s", now.Format("20060102150405"), strings.ToUpper(hex.EncodeToString(b))), nil
}

func formatAmount(cents int) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}
