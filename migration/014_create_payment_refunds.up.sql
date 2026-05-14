CREATE TABLE IF NOT EXISTS payment_refunds (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    refund_no VARCHAR(64) NOT NULL UNIQUE,
    out_request_no VARCHAR(64) NOT NULL UNIQUE,
    channel VARCHAR(32) NOT NULL DEFAULT 'alipay',
    channel_refund_no VARCHAR(128),
    refund_amount INTEGER NOT NULL CHECK (refund_amount > 0),
    total_order_amount INTEGER NOT NULL,
    refund_reason VARCHAR(256),
    status VARCHAR(32) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'success', 'failed')),
    initiated_by BIGINT REFERENCES users(id),
    refunded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_refunds_order
    ON payment_refunds(payment_order_id, created_at DESC);
