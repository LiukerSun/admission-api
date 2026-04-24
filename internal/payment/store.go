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
	CreateOrder(ctx context.Context, input CreateOrderInput) (*Order, bool, error)
	GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error)
	ListOrdersForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Order, int64, error)
	ListAdminOrders(ctx context.Context, filter AdminOrderFilter, page, pageSize int) ([]*Order, int64, error)
	CloseOrder(ctx context.Context, orderNo string) (*Order, error)
	CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error)
	MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error)
	MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error)
	MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error)
	MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error
	SaveCallback(ctx context.Context, req MockCallbackRequest, payload []byte) (*Callback, bool, error)
	MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error
	GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error)
	ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error)
	ListCallbacks(ctx context.Context, channelTradeNo *string) ([]*Callback, error)
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

func (s *store) CreateOrder(ctx context.Context, input CreateOrderInput) (*Order, bool, error) {
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

	o, err := scanOrder(s.pool.QueryRow(ctx, `
		INSERT INTO payment_orders (
			order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key, expires_at
		)
		VALUES ($1, $2, 'membership', $3, $4, $5, $6, 'awaiting_payment', 'unpaid', 'pending', 'mock', $7, $8)
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderNo, input.UserID, input.PlanID, subject, input.Amount, input.Currency, input.IdempotencyKey, expiresAt))
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
		WHERE order_no = $1 AND order_status IN ('awaiting_payment', 'created')
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("close payment order: %w", err)
	}
	return o, nil
}

func (s *store) CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error) {
	return scanAttempt(s.pool.QueryRow(ctx, `
		INSERT INTO payment_attempts (payment_order_id, attempt_no, channel, channel_status, amount)
		SELECT $1, COALESCE(MAX(attempt_no), 0) + 1, $2, 'pending', $3
		FROM payment_attempts
		WHERE payment_order_id = $1
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
	return scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = CASE WHEN order_status = 'fulfilled' THEN order_status ELSE 'paid' END,
			payment_status = 'paid',
			paid_at = COALESCE(paid_at, $2),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderID, now))
}

func (s *store) MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error) {
	return scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'fulfilled',
			payment_status = 'paid',
			entitlement_status = 'granted',
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
			order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
			expires_at, paid_at, closed_at, created_at, updated_at
	`, orderID))
}

func (s *store) MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_orders
		SET entitlement_status = 'failed', updated_at = NOW()
		WHERE id = $1
	`, orderID)
	return err
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

func (s *store) ListCallbacks(ctx context.Context, channelTradeNo *string) ([]*Callback, error) {
	if channelTradeNo == nil || *channelTradeNo == "" {
		return []*Callback{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, channel, callback_id, channel_trade_no, processed, processed_at, process_error, created_at
		FROM payment_callbacks
		WHERE channel = 'mock' AND channel_trade_no = $1
		ORDER BY created_at DESC
	`, *channelTradeNo)
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
