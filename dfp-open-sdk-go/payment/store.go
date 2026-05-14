package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store manages persistence for OpenBank payment records.
type Store interface {
	CreateOrder(ctx context.Context, input *CreateOrderInput) (*Order, bool, error)
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error)
	GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error)
	GetOrderByID(ctx context.Context, orderID int64) (*Order, string, error)
	CloseOrder(ctx context.Context, orderNo string) (*Order, error)
	MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error)
	MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error)
	MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error
	MarkOrderRefunded(ctx context.Context, orderID int64) (*Order, error)

	CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error)
	MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error)
	UpdateAttemptRequestPayload(ctx context.Context, attemptID int64, payload []byte) error
	GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error)
	ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error)

	SaveCallback(ctx context.Context, callbackID, channelTradeNo string, payload []byte) (*Callback, bool, error)
	MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error

	CreateRefundRequest(ctx context.Context, input *CreateRefundInput) (*Refund, error)
	GetRefundByNo(ctx context.Context, refundNo string) (*Refund, error)
	MarkRefundApproved(ctx context.Context, refundID, reviewerID int64, reviewNote *string, now time.Time) (*Refund, error)
	MarkRefundProcessing(ctx context.Context, refundID int64) (*Refund, error)
	MarkRefundRejected(ctx context.Context, refundID, reviewerID int64, reviewNote string, now time.Time) (*Refund, error)
	UpdateRefundSuccess(ctx context.Context, refundID int64, channelRefundNo string, now time.Time) (*Refund, error)
	UpdateRefundFailed(ctx context.Context, refundID int64) error
	ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error)
	ListRefunds(ctx context.Context, orderID int64) ([]*Refund, error)
	GetTotalRefundedAmount(ctx context.Context, orderID int64) (int, error)
}

type CreateOrderInput struct {
	UserID         int64
	PlanID         int64
	PlanCode       string
	PlanName       string
	DurationDays   int
	Amount         int
	Currency       string
	IdempotencyKey *string
	Now            time.Time
}

type CreateRefundInput struct {
	PaymentOrderID   int64
	RefundNo         string
	OutRequestNo     string
	Channel          string
	RefundAmount     int
	TotalOrderAmount int
	RefundReason     string
	InitiatedBy      *int64
}

type postgresStore struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &postgresStore{pool: pool}
}

const (
	orderColumns = `id, order_no, user_id, product_type, product_ref_id, subject, amount, currency,
		order_status, payment_status, entitlement_status, payment_channel, idempotency_key,
		expires_at, paid_at, closed_at, created_at, updated_at`

	attemptColumns = `id, payment_order_id, attempt_no, channel, channel_trade_no, channel_status, amount,
		callback_received_at, success_at, failed_at, created_at, updated_at`

	refundColumns = `id, payment_order_id, refund_no, out_request_no, channel, channel_refund_no,
		refund_amount, total_order_amount, refund_reason, status, initiated_by,
		review_note, reviewed_by, reviewed_at, refunded_at, created_at, updated_at`
)

// Order operations

func (s *postgresStore) CreateOrder(ctx context.Context, input *CreateOrderInput) (*Order, bool, error) {
	if input.Now.IsZero() {
		input.Now = time.Now()
	}

	if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
		existing, err := scanOrder(s.pool.QueryRow(ctx, `
			SELECT `+orderColumns+`
			FROM payment_orders
			WHERE user_id = $1 AND idempotency_key = $2
		`, input.UserID, *input.IdempotencyKey))
		if err == nil {
			if existing.ProductRefID != input.PlanID || existing.Amount != input.Amount {
				return nil, false, ErrIdempotencyConflict
			}
			return existing, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
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
		VALUES ($1, $2, 'membership', $3, $4, $5, $6, 'awaiting_payment', 'unpaid', 'pending', 'openbank', $7, $8)
		RETURNING `+orderColumns+`
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

func (s *postgresStore) GetOrderByNo(ctx context.Context, orderNo string) (*Order, string, error) {
	return s.getOrderWithPlan(ctx, "po.order_no = $1", orderNo)
}

func (s *postgresStore) GetOrderForUser(ctx context.Context, userID int64, orderNo string) (*Order, string, error) {
	o, planCode, err := s.getOrderWithPlan(ctx, "po.order_no = $1", orderNo)
	if err != nil {
		return nil, "", err
	}
	if o.UserID != userID {
		return nil, "", ErrOrderAccessDenied
	}
	return o, planCode, nil
}

func (s *postgresStore) GetOrderByID(ctx context.Context, orderID int64) (*Order, string, error) {
	return s.getOrderWithPlan(ctx, "po.id = $1", orderID)
}

func (s *postgresStore) getOrderWithPlan(ctx context.Context, where string, args ...any) (*Order, string, error) {
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
		&o.ID, &o.OrderNo, &o.UserID, &o.ProductType, &o.ProductRefID, &o.Subject, &o.Amount, &o.Currency,
		&o.OrderStatus, &o.PaymentStatus, &o.EntitlementStatus, &o.PaymentChannel, &o.IdempotencyKey,
		&o.ExpiresAt, &o.PaidAt, &o.ClosedAt, &o.CreatedAt, &o.UpdatedAt, &planCode,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrOrderNotFound
		}
		return nil, "", fmt.Errorf("get payment order with plan: %w", err)
	}
	return &o, planCode, nil
}

func (s *postgresStore) CloseOrder(ctx context.Context, orderNo string) (*Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'closed', closed_at = NOW(), updated_at = NOW()
		WHERE order_no = $1 AND order_status = 'awaiting_payment'
		RETURNING `+orderColumns+`
	`, orderNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return o, nil
}

func (s *postgresStore) MarkOrderPaid(ctx context.Context, orderID int64, now time.Time) (*Order, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'paid', payment_status = 'paid', paid_at = $2, updated_at = NOW()
		WHERE id = $1 AND order_status = 'awaiting_payment'
		RETURNING `+orderColumns+`
	`, orderID, now))
}

func (s *postgresStore) MarkOrderFulfilled(ctx context.Context, orderID int64) (*Order, error) {
	return scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'fulfilled', entitlement_status = 'granted', updated_at = NOW()
		WHERE id = $1
		RETURNING `+orderColumns+`
	`, orderID))
}

func (s *postgresStore) MarkOrderEntitlementFailed(ctx context.Context, orderID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_orders
		SET entitlement_status = 'failed', updated_at = NOW()
		WHERE id = $1
	`, orderID)
	return err
}

func (s *postgresStore) MarkOrderRefunded(ctx context.Context, orderID int64) (*Order, error) {
	return scanOrder(s.pool.QueryRow(ctx, `
		UPDATE payment_orders
		SET order_status = 'refunded', updated_at = NOW()
		WHERE id = $1
		RETURNING `+orderColumns+`
	`, orderID))
}

// Attempt operations

func (s *postgresStore) CreateAttempt(ctx context.Context, orderID int64, channel string, amount int) (*Attempt, error) {
	attemptNo, err := s.nextAttemptNo(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return scanAttempt(s.pool.QueryRow(ctx, `
		INSERT INTO payment_attempts (payment_order_id, attempt_no, channel, amount)
		VALUES ($1, $2, $3, $4)
		RETURNING `+attemptColumns+`
	`, orderID, attemptNo, channel, amount))
}

func (s *postgresStore) nextAttemptNo(ctx context.Context, orderID int64) (int, error) {
	var next int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_no), 0) + 1 FROM payment_attempts WHERE payment_order_id = $1
	`, orderID).Scan(&next)
	return next, err
}

func (s *postgresStore) MarkAttemptSuccess(ctx context.Context, attemptID int64, channelTradeNo string, now time.Time) (*Attempt, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return scanAttempt(s.pool.QueryRow(ctx, `
		UPDATE payment_attempts
		SET channel_trade_no = COALESCE(channel_trade_no, $2),
		    channel_status = 'success',
		    success_at = $3,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING `+attemptColumns+`
	`, attemptID, channelTradeNo, now))
}

func (s *postgresStore) UpdateAttemptRequestPayload(ctx context.Context, attemptID int64, payload []byte) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_attempts
		SET request_payload = $2, updated_at = NOW()
		WHERE id = $1
	`, attemptID, payload)
	return err
}

func (s *postgresStore) GetAttemptByChannelTrade(ctx context.Context, channel, channelTradeNo string) (*Attempt, error) {
	return scanAttempt(s.pool.QueryRow(ctx, `
		SELECT `+attemptColumns+`
		FROM payment_attempts
		WHERE channel = $1 AND channel_trade_no = $2
	`, channel, channelTradeNo))
}

func (s *postgresStore) ListAttempts(ctx context.Context, orderID int64) ([]*Attempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+attemptColumns+`
		FROM payment_attempts
		WHERE payment_order_id = $1
		ORDER BY created_at DESC
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payment attempts: %w", err)
	}
	defer rows.Close()
	var attempts []*Attempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// Callback operations

func (s *postgresStore) SaveCallback(ctx context.Context, callbackID, channelTradeNo string, payload []byte) (*Callback, bool, error) {
	cb := Callback{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO payment_callbacks (channel, callback_id, channel_trade_no, payload)
		VALUES ('openbank', $1, $2, $3)
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
		return nil, false, fmt.Errorf("save openbank payment callback: %w", err)
	}
	return &cb, true, nil
}

func (s *postgresStore) MarkCallbackProcessed(ctx context.Context, callbackID int64, processErr *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_callbacks
		SET processed = TRUE, processed_at = NOW(), process_error = $2
		WHERE id = $1
	`, callbackID, processErr)
	return err
}

// Refund operations

func (s *postgresStore) CreateRefundRequest(ctx context.Context, input *CreateRefundInput) (*Refund, error) {
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

func (s *postgresStore) GetRefundByNo(ctx context.Context, refundNo string) (*Refund, error) {
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

func (s *postgresStore) MarkRefundApproved(ctx context.Context, refundID, reviewerID int64, reviewNote *string, now time.Time) (*Refund, error) {
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

func (s *postgresStore) MarkRefundProcessing(ctx context.Context, refundID int64) (*Refund, error) {
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
		return nil, fmt.Errorf("refund status is %s, expected %s", currentStatus, RefundStatusApproved)
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

func (s *postgresStore) MarkRefundRejected(ctx context.Context, refundID, reviewerID int64, reviewNote string, now time.Time) (*Refund, error) {
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

func (s *postgresStore) UpdateRefundSuccess(ctx context.Context, refundID int64, channelRefundNo string, now time.Time) (*Refund, error) {
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

func (s *postgresStore) UpdateRefundFailed(ctx context.Context, refundID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE payment_refunds
		SET status = 'failed', updated_at = NOW()
		WHERE id = $1
	`, refundID)
	return err
}

func (s *postgresStore) ListPendingRefunds(ctx context.Context, page, pageSize int) ([]*Refund, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM payment_refunds WHERE status = 'pending_review' AND channel = 'openbank'
	`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pending refunds: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := s.pool.Query(ctx, `
		SELECT `+refundColumns+`
		FROM payment_refunds
		WHERE status = 'pending_review' AND channel = 'openbank'
		ORDER BY created_at ASC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list pending refunds: %w", err)
	}
	defer rows.Close()

	var refunds []*Refund
	for rows.Next() {
		r, err := scanRefund(rows)
		if err != nil {
			return nil, 0, err
		}
		refunds = append(refunds, r)
	}
	return refunds, total, rows.Err()
}

func (s *postgresStore) ListRefunds(ctx context.Context, orderID int64) ([]*Refund, error) {
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
	var refunds []*Refund
	for rows.Next() {
		r, err := scanRefund(rows)
		if err != nil {
			return nil, err
		}
		refunds = append(refunds, r)
	}
	return refunds, rows.Err()
}

func (s *postgresStore) GetTotalRefundedAmount(ctx context.Context, orderID int64) (int, error) {
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

// Scanner helpers

func scanOrder(row pgx.Row) (*Order, error) {
	var o Order
	if err := row.Scan(
		&o.ID, &o.OrderNo, &o.UserID, &o.ProductType, &o.ProductRefID, &o.Subject, &o.Amount, &o.Currency,
		&o.OrderStatus, &o.PaymentStatus, &o.EntitlementStatus, &o.PaymentChannel, &o.IdempotencyKey,
		&o.ExpiresAt, &o.PaidAt, &o.ClosedAt, &o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &o, nil
}

func scanAttempt(row pgx.Row) (*Attempt, error) {
	var a Attempt
	if err := row.Scan(
		&a.ID, &a.PaymentOrderID, &a.AttemptNo, &a.Channel, &a.ChannelTradeNo, &a.ChannelStatus,
		&a.Amount, &a.CallbackReceivedAt, &a.SuccessAt, &a.FailedAt, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

func scanRefund(row pgx.Row) (*Refund, error) {
	var r Refund
	if err := row.Scan(
		&r.ID, &r.PaymentOrderID, &r.RefundNo, &r.OutRequestNo, &r.Channel, &r.ChannelRefundNo,
		&r.RefundAmount, &r.TotalOrderAmount, &r.RefundReason, &r.Status, &r.InitiatedBy,
		&r.ReviewNote, &r.ReviewedBy, &r.ReviewedAt, &r.RefundedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

func generateOrderNo(now time.Time) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate order entropy: %w", err)
	}
	return fmt.Sprintf("MO%s%s", now.Format("20060102150405"), strings.ToUpper(hex.EncodeToString(b))), nil
}
