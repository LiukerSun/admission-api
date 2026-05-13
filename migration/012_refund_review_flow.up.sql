-- Refund review flow:
--   payment_refunds.status: 加 pending_review / approved / rejected
--   payment_refunds 加 review 字段
--   payment_orders.order_status 加 refunded
--   payment_orders.entitlement_status 加 revoked
--   user_memberships.status 加 refunded

ALTER TABLE payment_refunds
    DROP CONSTRAINT IF EXISTS payment_refunds_status_check;

ALTER TABLE payment_refunds
    ADD CONSTRAINT payment_refunds_status_check
    CHECK (status IN ('pending_review', 'rejected', 'approved', 'processing', 'success', 'failed'));

ALTER TABLE payment_refunds
    ADD COLUMN IF NOT EXISTS review_note VARCHAR(512),
    ADD COLUMN IF NOT EXISTS reviewed_by BIGINT REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

-- 同一订单只允许存在一条 pending_review 记录，避免重复申请
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_refunds_pending_per_order
    ON payment_refunds(payment_order_id)
    WHERE status = 'pending_review';

CREATE INDEX IF NOT EXISTS idx_payment_refunds_status_created
    ON payment_refunds(status, created_at DESC);

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_order_status_check;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_order_status_check
    CHECK (order_status IN ('created', 'awaiting_payment', 'paid', 'fulfilled', 'closed', 'failed', 'refunded'));

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_entitlement_status_check;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_entitlement_status_check
    CHECK (entitlement_status IN ('pending', 'granted', 'failed', 'revoked'));

ALTER TABLE user_memberships
    DROP CONSTRAINT IF EXISTS user_memberships_status_check;

ALTER TABLE user_memberships
    ADD CONSTRAINT user_memberships_status_check
    CHECK (status IN ('inactive', 'active', 'expired', 'refunded'));

-- membership_grants 加 revoke 动作，便于审计撤销历史
ALTER TABLE membership_grants
    DROP CONSTRAINT IF EXISTS membership_grants_action_check;

ALTER TABLE membership_grants
    ADD CONSTRAINT membership_grants_action_check
    CHECK (action IN ('activate', 'renew', 'extend', 'restore', 'revoke'));
