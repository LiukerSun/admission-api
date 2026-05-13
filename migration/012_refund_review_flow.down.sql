DROP INDEX IF EXISTS uq_payment_refunds_pending_per_order;
DROP INDEX IF EXISTS idx_payment_refunds_status_created;

ALTER TABLE payment_refunds
    DROP COLUMN IF EXISTS review_note,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at;

ALTER TABLE payment_refunds
    DROP CONSTRAINT IF EXISTS payment_refunds_status_check;

ALTER TABLE payment_refunds
    ADD CONSTRAINT payment_refunds_status_check
    CHECK (status IN ('processing', 'success', 'failed'));

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_order_status_check;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_order_status_check
    CHECK (order_status IN ('created', 'awaiting_payment', 'paid', 'fulfilled', 'closed', 'failed'));

ALTER TABLE payment_orders
    DROP CONSTRAINT IF EXISTS payment_orders_entitlement_status_check;

ALTER TABLE payment_orders
    ADD CONSTRAINT payment_orders_entitlement_status_check
    CHECK (entitlement_status IN ('pending', 'granted', 'failed'));

ALTER TABLE user_memberships
    DROP CONSTRAINT IF EXISTS user_memberships_status_check;

ALTER TABLE user_memberships
    ADD CONSTRAINT user_memberships_status_check
    CHECK (status IN ('inactive', 'active', 'expired'));

ALTER TABLE membership_grants
    DROP CONSTRAINT IF EXISTS membership_grants_action_check;

ALTER TABLE membership_grants
    ADD CONSTRAINT membership_grants_action_check
    CHECK (action IN ('activate', 'renew', 'extend', 'restore'));
