CREATE TABLE IF NOT EXISTS membership_plans (
    id BIGSERIAL PRIMARY KEY,
    plan_code VARCHAR(32) NOT NULL UNIQUE,
    plan_name VARCHAR(100) NOT NULL,
    membership_level VARCHAR(20) NOT NULL DEFAULT 'premium'
        CHECK (membership_level IN ('premium')),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    price_amount INTEGER NOT NULL CHECK (price_amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payment_orders (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_type VARCHAR(32) NOT NULL CHECK (product_type IN ('membership')),
    product_ref_id BIGINT NOT NULL REFERENCES membership_plans(id),
    subject VARCHAR(200) NOT NULL,
    amount INTEGER NOT NULL CHECK (amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    order_status VARCHAR(32) NOT NULL DEFAULT 'awaiting_payment'
        CHECK (order_status IN ('created', 'awaiting_payment', 'paid', 'fulfilled', 'closed', 'failed')),
    payment_status VARCHAR(32) NOT NULL DEFAULT 'unpaid'
        CHECK (payment_status IN ('unpaid', 'paying', 'paid', 'failed')),
    entitlement_status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (entitlement_status IN ('pending', 'granted', 'failed')),
    payment_channel VARCHAR(32) NOT NULL DEFAULT 'mock',
    idempotency_key VARCHAR(128),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payment_attempts (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
    channel VARCHAR(32) NOT NULL DEFAULT 'mock',
    channel_trade_no VARCHAR(128),
    channel_status VARCHAR(32) NOT NULL DEFAULT 'created'
        CHECK (channel_status IN ('created', 'pending', 'success', 'failed', 'closed')),
    amount INTEGER NOT NULL CHECK (amount >= 0),
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    callback_received_at TIMESTAMPTZ,
    success_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (payment_order_id, attempt_no)
);

CREATE TABLE IF NOT EXISTS payment_callbacks (
    id BIGSERIAL PRIMARY KEY,
    channel VARCHAR(32) NOT NULL,
    callback_id VARCHAR(128) NOT NULL,
    channel_trade_no VARCHAR(128),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMPTZ,
    process_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_memberships (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    membership_level VARCHAR(20) NOT NULL DEFAULT 'premium'
        CHECK (membership_level IN ('premium')),
    status VARCHAR(20) NOT NULL DEFAULT 'inactive'
        CHECK (status IN ('inactive', 'active', 'expired')),
    started_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    last_order_id BIGINT REFERENCES payment_orders(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS membership_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    source_type VARCHAR(32) NOT NULL DEFAULT 'payment'
        CHECK (source_type IN ('payment')),
    action VARCHAR(32) NOT NULL CHECK (action IN ('activate', 'renew', 'extend', 'restore')),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_membership_plans_status
    ON membership_plans(status);

CREATE INDEX IF NOT EXISTS idx_payment_orders_user_created
    ON payment_orders(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_payment_orders_status
    ON payment_orders(order_status, payment_status, entitlement_status);

CREATE INDEX IF NOT EXISTS idx_payment_orders_product
    ON payment_orders(product_type, product_ref_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_orders_user_idempotency
    ON payment_orders(user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_payment_attempts_order
    ON payment_attempts(payment_order_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_channel_trade
    ON payment_attempts(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_callbacks_channel_callback
    ON payment_callbacks(channel, callback_id);

CREATE INDEX IF NOT EXISTS idx_payment_callbacks_trade
    ON payment_callbacks(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_memberships_active_lookup
    ON user_memberships(user_id, ends_at)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_membership_grants_user_created
    ON membership_grants(user_id, created_at DESC);

INSERT INTO membership_plans (plan_code, plan_name, membership_level, duration_days, price_amount, currency, status)
VALUES
    ('monthly', '月度会员', 'premium', 30, 990, 'CNY', 'active'),
    ('quarterly', '季度会员', 'premium', 90, 2690, 'CNY', 'active'),
    ('yearly', '年度会员', 'premium', 365, 9990, 'CNY', 'active')
ON CONFLICT (plan_code) DO UPDATE
SET plan_name = EXCLUDED.plan_name,
    membership_level = EXCLUDED.membership_level,
    duration_days = EXCLUDED.duration_days,
    price_amount = EXCLUDED.price_amount,
    currency = EXCLUDED.currency,
    status = EXCLUDED.status,
    updated_at = NOW();
