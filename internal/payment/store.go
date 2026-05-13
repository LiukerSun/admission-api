package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOrderNotFound       = errors.New("payment order not found")
	ErrOrderAccessDenied   = errors.New("payment order access denied")
	ErrOrderNotPayable     = errors.New("payment order is not payable")
	ErrOrderExpired        = errors.New("payment order expired")
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
	ErrCallbackDuplicate   = errors.New("payment callback already processed")
)

type Store interface {
	CreateOrder(ctx context.Context, input *CreateOrderInput) (*Order, bool, error)
	GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error)
	GetOrderByID(ctx context.Context, orderID int64) (*Order, string, error)
	ListOrdersForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Order, int64, error)
	ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) ([]*Order, int64, error)
	CloseOrder(ctx context.Context, orderNo string) (*Order, error)
	CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error)
	MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error)
	MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error)
	MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error)
	MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error
	MarkOrderRefunded(ctx context.Context, orderID int64) (*Order, error)
	SaveCallback(ctx context.Context, req MockCallbackRequest, payload []byte) (*Callback, bool, error)
	SaveAlipayCallback(ctx context.Context, callbackID string, channelTradeNo string, payload []byte) (*Callback, bool, error)
	MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error
	GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error)
	ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error)
	ListCallbacks(ctx context.Context, channel string, channelTradeNo *string) ([]*Callback, error)
	UpdateAttemptRequestPayload(ctx context.Context, attemptID int64, payload []byte) error
	CreateRefund(ctx context.Context, input *CreateRefundInput) (*Refund, error)
	UpdateRefundSuccess(ctx context.Context, refundID int64, channelRefundNo string, now time.Time) (*Refund, error)
	UpdateRefundFailed(ctx context.Context, refundID int64) error
	GetRefundByOutRequestNo(ctx context.Context, outRequestNo string) (*Refund, error)
	GetRefundByNo(ctx context.Context, refundNo string) (*Refund, error)
	ListRefunds(ctx context.Context, orderID int64) ([]*Refund, error)
	GetTotalRefundedAmount(ctx context.Context, orderID int64) (int, error)
	// 退款审核流程
	CreateRefundRequest(ctx context.Context, input *CreateRefundInput) (*Refund, error)
	GetRefundForReview(ctx context.Context, refundNo string) (*Refund, *Order, error)
	MarkRefundApproved(ctx context.Context, refundID, reviewerID int64, reviewNote *string, now time.Time) (*Refund, error)
	MarkRefundProcessing(ctx context.Context, refundID int64) (*Refund, error)
	MarkRefundRejected(ctx context.Context, refundID, reviewerID int64, reviewNote string, now time.Time) (*Refund, error)
	ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func scanOrder(row pgx.Row) (*Order, error) {
	var o Order
	if err := row.Scan(
		&o.ID,
		&o.OrderNo,
		&o.UserID,
		&o.ProductType,
		&o.ProductRefID,
		&o.Subject,
		&o.Amount,
		&o.Currency,
		&o.OrderStatus,
		&o.PaymentStatus,
		&o.EntitlementStatus,
		&o.PaymentChannel,
		&o.IdempotencyKey,
		&o.ExpiresAt,
		&o.PaidAt,
		&o.ClosedAt,
		&o.CreatedAt,
		&o.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &o, nil
}

func scanAttempt(row pgx.Row) (*Attempt, error) {
	var a Attempt
	if err := row.Scan(
		&a.ID,
		&a.PaymentOrderID,
		&a.AttemptNo,
		&a.Channel,
		&a.ChannelTradeNo,
		&a.ChannelStatus,
		&a.Amount,
		&a.CallbackReceivedAt,
		&a.SuccessAt,
		&a.FailedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *store) CreateOrder(ctx context.Context, input *CreateOrderInput) (*Order, bool, error) {
	if input.Now.IsZero() {
		input.Now = time.Now()
	}

	if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
		existing, err := scanOrder(s.pool.QueryRow(ctx, orderSelectSQL()+`
			WHERE user_id = $1 AND idempotency_key = $2
		`, input.UserID, *input.IdempotencyKey))
		if err == nil {
			if existing.ProductRefID != input.PlanID || existing.Amount != input.Amount {
				return nil, false, ErrIdempotencyConflict
			}
			return existing, false, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("lookup idempotent order: %w", err)
		}
	}

	orderNo, err := generateOrderNo(input.Now)
	if err != nil {
		return nil, false, err
	}
	subject := input.PlanName
	expiresAt := input.Now.Add(15 * time.Minute)
	channel := input.Channel
	if channel == "" {
		channel = "mock"
	}

	o, err := scanOrder(s.pool.QueryRow(ctx, `
		INSERT INTO payment_orders (
			order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key, expires_at
		)
		VALUES ($1, $2, 'membership', $3, $4, $5, $6, 'awaiting_payment', 'unpaid', 'pending', $7, $8, $9)
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderNo, input.UserID, input.PlanID, subject, input.Amount, input.Currency, channel, input.IdempotencyKey, expiresAt))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return s.CreateOrder(ctx, input)
		}
		return nil, false, fmt.Errorf("create payment order: %w", err)
	}
	return o, true, nil
}

func (s *store) GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error) {
	o, planCode, err := s.getOrderWithPlan(ctx, "po.order_no = $1", orderNo)
	if err != nil {
		return nil, "", err
	}
	if o.UserID != userID {
		return nil, "", ErrOrderAccessDenied
	}
	return o, planCode, nil
}

func (s *store) GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error) {
	return s.getOrderWithPlan(ctx, "po.order_no = $1", orderNo)
}

func (s *store) getOrderWithPlan(ctx context.Context, where string, args ...any) (*Order, string, error) {
	var planCode string
	row := s.pool.QueryRow(ctx, `
		SELECT po.id, po.order_no, po.user_id, po.product_type, po.product_ref_id, po.subject, po.amount, po.currency,
			po.order_status, po.payment_status, po.entitlement_status, po.payment_channel, po.idempotency_key,
			po.expires_at, po.paid_at, po.closed_at, po.created_at, po.updated_at, mp.plan_code
		FROM payment_orders po
		JOIN membership_plans mp ON mp.id = po.product_ref_id
		WHERE `+where, args...)
	var o Order
	if err := row.Scan(
		&o.ID,
		&o.OrderNo,
		&o.UserID,
		&o.ProductType,
		&o.ProductRefID,
		&o.Subject,
		&o.Amount,
		&o.Currency,
		&o.OrderStatus,
		&o.PaymentStatus,
		&o.EntitlementStatus,
		&o.PaymentChannel,
		&o.IdempotencyKey,
		&o.ExpiresAt,
		&o.PaidAt,
		&o.ClosedAt,
		&o.CreatedAt,
		&o.UpdatedAt,
		&planCode,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrOrderNotFound
		}
		return nil, "", fmt.Errorf("get payment order: %w", err)
	}
	return &o, planCode, nil
}

func (s *store) ListOrdersForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM payment_orders WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count user payment orders: %w", err)
	}

	rows, err := s.pool.Query(ctx, orderSelectSQL()+`
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list user payment orders: %w", err)
	}
	defer rows.Close()
	orders, err := scanOrders(rows)
	if err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (s *store) ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) ([]*Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	where := []string{"1=1"}
	args := []any{}
	argIdx := 1
	if filter.OrderNo != "" {
		where = append(where, fmt.Sprintf("po.order_no ILIKE $%d", argIdx))
		args = append(args, "%"+filter.OrderNo+"%")
		argIdx++
	}
	if filter.UserID > 0 {
		where = append(where, fmt.Sprintf("po.user_id = $%d", argIdx))
		args = append(args, filter.UserID)
		argIdx++
	}
	if filter.PlanCode != "" {
		where = append(where, fmt.Sprintf("mp.plan_code = $%d", argIdx))
		args = append(args, filter.PlanCode)
		argIdx++
	}
	if filter.Channel != "" {
		where = append(where, fmt.Sprintf("po.payment_channel = $%d", argIdx))
		args = append(args, filter.Channel)
		argIdx++
	}
	if filter.OrderStatus != "" {
		where = append(where, fmt.Sprintf("po.order_status = $%d", argIdx))
		args = append(args, filter.OrderStatus)
		argIdx++
	}
	whereClause := strings.Join(where, " AND ")

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM payment_orders po
		JOIN membership_plans mp ON mp.id = po.product_ref_id
		WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin payment orders: %w", err)
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx, `
		SELECT po.id, po.order_no, po.user_id, po.product_type, po.product_ref_id, po.subject, po.amount, po.currency,
			po.order_status, po.payment_status, po.entitlement_status, po.payment_channel, po.idempotency_key,
			po.expires_at, po.paid_at, po.closed_at, po.created_at, po.updated_at
		FROM payment_orders po
		JOIN membership_plans mp ON mp.id = po.product_ref_id
		WHERE `+whereClause+fmt.Sprintf(`
		ORDER BY po.created_at DESC
		LIMIT $%d OFFSET $%d
	`, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin payment orders: %w", err)
	}
	defer rows.Close()
	orders, err := scanOrders(rows)
	if err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (s *store) CloseOrder(ctx context.Context, orderNo string) (*Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'closed', payment_status = 'failed', closed_at = COALESCE(closed_at, NOW()), updated_at = NOW()
		WHERE order_no = $1
		  AND order_status = 'awaiting_payment'
		  AND payment_status = 'unpaid'
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			exists, existsErr := s.orderExistsByNo(ctx, orderNo)
			if existsErr != nil {
				return nil, existsErr
			}
			if exists {
				return nil, ErrOrderNotPayable
			}
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("close payment order: %w", err)
	}
	return o, nil
}

func (s *store) CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error) {
	return scanAttempt(s.pool.QueryRow(ctx, `
		WITH locked_order AS (
			SELECT id
			FROM payment_orders
			WHERE id = $1
			FOR UPDATE
		), next_attempt AS (
			SELECT COALESCE(MAX(attempt_no), 0) + 1 AS attempt_no
			FROM payment_attempts
			WHERE payment_order_id = $1
		)
		INSERT INTO payment_attempts (payment_order_id, attempt_no, channel, channel_status, amount)
		SELECT $1, next_attempt.attempt_no, $2, 'pending', $3
		FROM locked_order, next_attempt
		RETURNING id, payment_order_id, attempt_no, channel, channel_trade_no, channel_status, amount,
			callback_received_at, success_at, failed_at, created_at, updated_at
	`, orderID, channel, amount))
}

func (s *store) MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return scanAttempt(s.pool.QueryRow(ctx, `
		UPDATE payment_attempts
		SET channel_trade_no = COALESCE(channel_trade_no, $2),
			channel_status = 'success',
			success_at = COALESCE(success_at, $3),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, payment_order_id, attempt_no, channel, channel_trade_no, channel_status, amount,
			callback_received_at, success_at, failed_at, created_at, updated_at
	`, attemptID, channelTradeNo, now))
}

func (s *store) MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error) {
	if now.IsZero() {
		now = time.Now()
	}
	o, err := scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = CASE WHEN order_status = 'fulfilled' THEN order_status ELSE 'paid' END,
			payment_status = 'paid',
			paid_at = COALESCE(paid_at, $2),
			updated_at = NOW()
		WHERE id = $1
		  AND order_status IN ('awaiting_payment', 'paid', 'fulfilled')
		  AND payment_status IN ('unpaid', 'paid')
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderID, now))
	if err == nil {
		return o, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		exists, existsErr := s.orderExistsByID(ctx, orderID)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrOrderNotPayable
		}
		return nil, ErrOrderNotFound
	}
	return nil, err
}

func (s *store) MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'fulfilled',
			payment_status = 'paid',
			entitlement_status = 'granted',
			updated_at = NOW()
		WHERE id = $1
		  AND payment_status = 'paid'
		  AND order_status IN ('paid', 'fulfilled')
		  AND entitlement_status IN ('pending', 'failed', 'granted')
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderID))
	if err == nil {
		return o, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		exists, existsErr := s.orderExistsByID(ctx, orderID)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrOrderNotPayable
		}
		return nil, ErrOrderNotFound
	}
	return nil, err
}

func (s *store) MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_orders
		SET entitlement_status = 'failed', updated_at = NOW()
		WHERE id = $1
	`, orderID)
	return err
}

// MarkOrderRefunded 把订单标记为 refunded + entitlement_status=revoked。
// 仅在订单当前处于 paid/fulfilled 时生效。
func (s *store) MarkOrderRefunded(ctx context.Context, orderID int64) (*Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'refunded',
			entitlement_status = 'revoked',
			updated_at = NOW()
		WHERE id = $1
		  AND order_status IN ('paid', 'fulfilled')
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderID))
	if err == nil {
		return o, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		exists, existsErr := s.orderExistsByID(ctx, orderID)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, ErrOrderNotRefundable
		}
		return nil, ErrOrderNotFound
	}
	return nil, err
}

// GetOrderByID 按主键读取订单。
func (s *store) GetOrderByID(ctx context.Context, orderID int64) (*Order, string, error) {
	return s.getOrderWithPlan(ctx, "po.id = $1", orderID)
}

func (s *store) SaveCallback(ctx context.Context, req MockCallbackRequest, payload []byte) (*Callback, bool, error) {
	if len(payload) == 0 {
		payload, _ = json.Marshal(req)
	}
	cb := Callback{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO payment_callbacks (channel, callback_id, channel_trade_no, payload)
		VALUES ('mock', $1, $2, $3)
		ON CONFLICT (channel, callback_id) DO NOTHING
		RETURNING id, channel, callback_id, channel_trade_no, processed, processed_at, process_error, created_at
	`, req.CallbackID, req.ChannelTradeNo, payload).Scan(
		&cb.ID,
		&cb.Channel,
		&cb.CallbackID,
		&cb.ChannelTradeNo,
		&cb.Processed,
		&cb.ProcessedAt,
		&cb.ProcessError,
		&cb.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrCallbackDuplicate
		}
		return nil, false, fmt.Errorf("save payment callback: %w", err)
	}
	return &cb, true, nil
}

func (s *store) MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_callbacks
		SET processed = $2,
			processed_at = NOW(),
			process_error = $3
		WHERE id = $1
	`, callbackID, processErr == nil, processErr)
	return err
}

func (s *store) GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error) {
	a, err := scanAttempt(s.pool.QueryRow(ctx, `
		SELECT id, payment_order_id, attempt_no, channel, channel_trade_no, channel_status, amount,
			callback_received_at, success_at, failed_at, created_at, updated_at
		FROM payment_attempts
		WHERE channel = $1 AND channel_trade_no = $2
	`, channel, channelTradeNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get attempt by channel trade: %w", err)
	}
	return a, nil
}

func (s *store) ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, payment_order_id, attempt_no, channel, channel_trade_no, channel_status, amount,
			callback_received_at, success_at, failed_at, created_at, updated_at
		FROM payment_attempts
		WHERE payment_order_id = $1
		ORDER BY attempt_no DESC
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payment attempts: %w", err)
	}
	defer rows.Close()
	attempts := []*Attempt{}
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (s *store) ListCallbacks(ctx context.Context, channel string, channelTradeNo *string) ([]*Callback, error) {
	if channelTradeNo == nil || *channelTradeNo == "" {
		return []*Callback{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, channel, callback_id, channel_trade_no, processed, processed_at, process_error, created_at
		FROM payment_callbacks
		WHERE channel = $1 AND channel_trade_no = $2
		ORDER BY created_at DESC
	`, channel, *channelTradeNo)
	if err != nil {
		return nil, fmt.Errorf("list payment callbacks: %w", err)
	}
	defer rows.Close()
	callbacks := []*Callback{}
	for rows.Next() {
		var cb Callback
		if err := rows.Scan(&cb.ID, &cb.Channel, &cb.CallbackID, &cb.ChannelTradeNo, &cb.Processed, &cb.ProcessedAt, &cb.ProcessError, &cb.CreatedAt); err != nil {
			return nil, err
		}
		callbacks = append(callbacks, &cb)
	}
	return callbacks, rows.Err()
}

func orderSelectSQL() string {
	return `
		SELECT id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
		FROM payment_orders
	`
}

func scanOrders(rows pgx.Rows) ([]*Order, error) {
	orders := []*Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan payment order: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payment orders: %w", err)
	}
	return orders, nil
}

func generateOrderNo(now time.Time) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate order entropy: %w", err)
	}
	return fmt.Sprintf("MO%s%s", now.Format("20060102150405"), strings.ToUpper(hex.EncodeToString(b))), nil
}

func (s *store) orderExistsByID(ctx context.Context, orderID int64) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_orders WHERE id = $1)`, orderID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check payment order existence: %w", err)
	}
	return exists, nil
}

func (s *store) orderExistsByNo(ctx context.Context, orderNo string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_orders WHERE order_no = $1)`, orderNo).Scan(&exists); err != nil {
		return false, fmt.Errorf("check payment order existence: %w", err)
	}
	return exists, nil
}

func (s *store) SaveAlipayCallback(ctx context.Context, callbackID string, channelTradeNo string, payload []byte) (*Callback, bool, error) {
	cb := Callback{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO payment_callbacks (channel, callback_id, channel_trade_no, payload)
		VALUES ('alipay', $1, $2, $3)
		ON CONFLICT (channel, callback_id) DO NOTHING
		RETURNING id, channel, callback_id, channel_trade_no, processed, processed_at, process_error, created_at
	`, callbackID, channelTradeNo, payload).Scan(
		&cb.ID, &cb.Channel, &cb.CallbackID, &cb.ChannelTradeNo,
		&cb.Processed, &cb.ProcessedAt, &cb.ProcessError, &cb.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrCallbackDuplicate
		}
		return nil, false, fmt.Errorf("save alipay payment callback: %w", err)
	}
	return &cb, true, nil
}

func (s *store) UpdateAttemptRequestPayload(ctx context.Context, attemptID int64, payload []byte) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_attempts
		SET request_payload = $2, updated_at = NOW()
		WHERE id = $1
	`, attemptID, payload)
	return err
}

func (s *store) CreateRefund(ctx context.Context, input *CreateRefundInput) (*Refund, error) {
	refund, err := scanRefund(s.pool.QueryRow(ctx, `
		INSERT INTO payment_refunds (
			payment_order_id, refund_no, out_request_no, channel, refund_amount, total_order_amount, refund_reason, status, initiated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'processing', $8)
		RETURNING `+refundColumns+`
	`, input.PaymentOrderID, input.RefundNo, input.OutRequestNo, input.Channel, input.RefundAmount, input.TotalOrderAmount, input.RefundReason, input.InitiatedBy))
	if err != nil {
		return nil, fmt.Errorf("create payment refund: %w", err)
	}
	return refund, nil
}

func (s *store) UpdateRefundSuccess(ctx context.Context, refundID int64, channelRefundNo string, now time.Time) (*Refund, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return scanRefund(s.pool.QueryRow(ctx, `
		UPDATE payment_refunds
		SET channel_refund_no = COALESCE(channel_refund_no, $2),
			status = 'success',
			refunded_at = COALESCE(refunded_at, $3),
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+refundColumns+`
	`, refundID, channelRefundNo, now))
}

func (s *store) UpdateRefundFailed(ctx context.Context, refundID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_refunds
		SET status = 'failed', updated_at = NOW()
		WHERE id = $1
	`, refundID)
	return err
}

func (s *store) GetRefundByOutRequestNo(ctx context.Context, outRequestNo string) (*Refund, error) {
	refund, err := scanRefund(s.pool.QueryRow(ctx, `
		SELECT `+refundColumns+`
		FROM payment_refunds
		WHERE out_request_no = $1
	`, outRequestNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("get refund by out_request_no: %w", err)
	}
	return refund, nil
}

func (s *store) GetRefundByNo(ctx context.Context, refundNo string) (*Refund, error) {
	refund, err := scanRefund(s.pool.QueryRow(ctx, `
		SELECT `+refundColumns+`
		FROM payment_refunds
		WHERE refund_no = $1
	`, refundNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("get refund by no: %w", err)
	}
	return refund, nil
}

func (s *store) ListRefunds(ctx context.Context, orderID int64) ([]*Refund, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+refundColumns+`
		FROM payment_refunds
		WHERE payment_order_id = $1
		ORDER BY created_at DESC
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payment refunds: %w", err)
	}
	defer rows.Close()
	refunds := []*Refund{}
	for rows.Next() {
		r, err := scanRefund(rows)
		if err != nil {
			return nil, err
		}
		refunds = append(refunds, r)
	}
	return refunds, rows.Err()
}

// GetTotalRefundedAmount 计算「已锁定」的退款金额：success/processing/approved/pending_review 都算占用，
// 只有 rejected / failed 不计入。这样可以阻止用户连续提交多笔合计超额的退款申请。
func (s *store) GetTotalRefundedAmount(ctx context.Context, orderID int64) (int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(refund_amount), 0)
		FROM payment_refunds
		WHERE payment_order_id = $1
		  AND status IN ('pending_review', 'approved', 'processing', 'success')
	`, orderID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("get total refunded amount: %w", err)
	}
	return total, nil
}

// CreateRefundRequest 写入一条 status='pending_review' 的退款申请。
// 通过 payment_orders + payment_refunds 双重锁保证并发安全：
//   1. SELECT FOR UPDATE 锁住订单行，阻止并发申请同时通过余额检查
//   2. uq_payment_refunds_pending_per_order 索引保证同一订单只能有一条 pending_review
func (s *store) CreateRefundRequest(ctx context.Context, input *CreateRefundInput) (*Refund, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund request tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orderAmount int
	err = tx.QueryRow(ctx, `
		SELECT amount FROM payment_orders WHERE id = $1 FOR UPDATE
	`, input.PaymentOrderID).Scan(&orderAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("lock order for refund request: %w", err)
	}

	var locked int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(refund_amount), 0)
		FROM payment_refunds
		WHERE payment_order_id = $1
		  AND status IN ('pending_review', 'approved', 'processing', 'success')
	`, input.PaymentOrderID).Scan(&locked); err != nil {
		return nil, fmt.Errorf("lookup locked refund amount: %w", err)
	}
	if locked+input.RefundAmount > orderAmount {
		return nil, ErrRefundAmountExceeded
	}

	refund, err := scanRefund(tx.QueryRow(ctx, `
		INSERT INTO payment_refunds (
			payment_order_id, refund_no, out_request_no, channel, refund_amount, total_order_amount, refund_reason, status, initiated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending_review', $8)
		RETURNING `+refundColumns+`
	`, input.PaymentOrderID, input.RefundNo, input.OutRequestNo, input.Channel, input.RefundAmount, input.TotalOrderAmount, input.RefundReason, input.InitiatedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrRefundPendingExists
		}
		return nil, fmt.Errorf("create refund request: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit refund request tx: %w", err)
	}
	return refund, nil
}

// GetRefundForReview 用单条事务读取 refund 和它的 order，行锁住二者避免并发审批。
// 调用方有责任在拿到结果后立即调用 MarkRefundApproved / MarkRefundRejected / MarkRefundProcessing
// 以便事务尽快释放——但因为这里需要在事务外调用支付宝，最稳的方案是 caller 在自己的事务里
// 重新 SELECT FOR UPDATE。这个 helper 是只读检查用。
func (s *store) GetRefundForReview(ctx context.Context, refundNo string) (*Refund, *Order, error) {
	refund, err := s.GetRefundByNo(ctx, refundNo)
	if err != nil {
		return nil, nil, err
	}
	order, _, err := s.GetOrderByID(ctx, refund.PaymentOrderID)
	if err != nil {
		return nil, nil, err
	}
	return refund, order, nil
}

// MarkRefundApproved 把 pending_review → approved。同一事务内 SELECT FOR UPDATE 锁住 refund 行
// 防止并发审批；返回时 caller 再调用支付宝退款。
func (s *store) MarkRefundApproved(ctx context.Context, refundID, reviewerID int64, reviewNote *string, now time.Time) (*Refund, error) {
	if now.IsZero() {
		now = time.Now()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin approve refund tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM payment_refunds WHERE id = $1 FOR UPDATE`, refundID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("lock refund row: %w", err)
	}
	if currentStatus != RefundStatusPendingReview {
		return nil, ErrRefundNotPendingReview
	}

	refund, err := scanRefund(tx.QueryRow(ctx, `
		UPDATE payment_refunds
		SET status = 'approved',
			review_note = $2,
			reviewed_by = $3,
			reviewed_at = $4,
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+refundColumns+`
	`, refundID, reviewNote, reviewerID, now))
	if err != nil {
		return nil, fmt.Errorf("mark refund approved: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit approve refund tx: %w", err)
	}
	return refund, nil
}

// MarkRefundProcessing 把 approved → processing，标记已开始向支付宝发起退款。
func (s *store) MarkRefundProcessing(ctx context.Context, refundID int64) (*Refund, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund processing tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM payment_refunds WHERE id = $1 FOR UPDATE`, refundID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("lock refund row: %w", err)
	}
	if currentStatus != RefundStatusApproved {
		return nil, fmt.Errorf("refund is not approved (status=%s)", currentStatus)
	}

	refund, err := scanRefund(tx.QueryRow(ctx, `
		UPDATE payment_refunds
		SET status = 'processing', updated_at = NOW()
		WHERE id = $1
		RETURNING `+refundColumns+`
	`, refundID))
	if err != nil {
		return nil, fmt.Errorf("mark refund processing: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit refund processing tx: %w", err)
	}
	return refund, nil
}

// MarkRefundRejected 把 pending_review → rejected，review_note 必填。
func (s *store) MarkRefundRejected(ctx context.Context, refundID, reviewerID int64, reviewNote string, now time.Time) (*Refund, error) {
	if now.IsZero() {
		now = time.Now()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reject refund tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM payment_refunds WHERE id = $1 FOR UPDATE`, refundID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		return nil, fmt.Errorf("lock refund row: %w", err)
	}
	if currentStatus != RefundStatusPendingReview {
		return nil, ErrRefundNotPendingReview
	}

	refund, err := scanRefund(tx.QueryRow(ctx, `
		UPDATE payment_refunds
		SET status = 'rejected',
			review_note = $2,
			reviewed_by = $3,
			reviewed_at = $4,
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+refundColumns+`
	`, refundID, reviewNote, reviewerID, now))
	if err != nil {
		return nil, fmt.Errorf("mark refund rejected: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reject refund tx: %w", err)
	}
	return refund, nil
}

func (s *store) ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM payment_refunds WHERE status = 'pending_review'
	`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pending refunds: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+refundColumns+`
		FROM payment_refunds
		WHERE status = 'pending_review'
		ORDER BY created_at ASC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list pending refunds: %w", err)
	}
	defer rows.Close()

	refunds := []*Refund{}
	for rows.Next() {
		r, err := scanRefund(rows)
		if err != nil {
			return nil, 0, err
		}
		refunds = append(refunds, r)
	}
	return refunds, total, rows.Err()
}

// refundColumns 是所有读取 payment_refunds 时统一使用的列顺序。
const refundColumns = `id, payment_order_id, refund_no, out_request_no, channel, channel_refund_no, refund_amount, total_order_amount, refund_reason, status, initiated_by, review_note, reviewed_by, reviewed_at, refunded_at, created_at, updated_at`

func scanRefund(row pgx.Row) (*Refund, error) {
	var r Refund
	if err := row.Scan(
		&r.ID,
		&r.PaymentOrderID,
		&r.RefundNo,
		&r.OutRequestNo,
		&r.Channel,
		&r.ChannelRefundNo,
		&r.RefundAmount,
		&r.TotalOrderAmount,
		&r.RefundReason,
		&r.Status,
		&r.InitiatedBy,
		&r.ReviewNote,
		&r.ReviewedBy,
		&r.ReviewedAt,
		&r.RefundedAt,
		&r.CreatedAt,
		&r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}
