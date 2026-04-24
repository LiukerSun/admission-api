package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"admission-api/internal/membership"
)

type Service interface {
	CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*OrderResponse, error)
	ListMyOrders(ctx context.Context, userID int64, page, pageSize int) (*OrderListResponse, error)
	GetMyOrder(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	PayMock(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	ProcessMockCallback(ctx context.Context, req MockCallbackRequest) (*OrderResponse, error)
	Detect(ctx context.Context, userID int64, orderNo string) (*OrderResponse, error)
	ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) (*OrderListResponse, error)
	GetAdminOrder(ctx context.Context, orderNo string) (*AdminOrderDetailResponse, error)
	CloseAdminOrder(ctx context.Context, orderNo string) (*OrderResponse, error)
	RedetectAdmin(ctx context.Context, orderNo string) (*OrderResponse, error)
	RegrantMembership(ctx context.Context, orderNo string) (*OrderResponse, error)
}

type service struct {
	store             Store
	membershipService membership.Service
}

func NewService(store Store, membershipService membership.Service) Service {
	return &service{store: store, membershipService: membershipService}
}

func (s *service) CreateOrder(ctx context.Context, userID int64, req CreateOrderRequest) (*OrderResponse, error) {
	plan, err := s.membershipService.GetPurchasablePlan(ctx, req.PlanCode)
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
	callbacks, err := s.store.ListCallbacks(ctx, tradeNo)
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
	if o.OrderStatus == OrderStatusClosed || o.OrderStatus == OrderStatusFailed || o.OrderStatus == OrderStatusFulfilled {
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
