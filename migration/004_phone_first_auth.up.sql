-- ============================================================================
-- 002_phone_first_auth: switch primary identity from email to phone.
--
-- After this migration the new register / login flow is phone + SMS code (or
-- phone + password). The email column stays for historical records but becomes
-- optional and is no longer unique-required across all rows.
-- ============================================================================

-- Email becomes optional.
ALTER TABLE users
    ALTER COLUMN email DROP NOT NULL;

-- Replace the table-level UNIQUE on email with a partial unique index so we
-- can have multiple rows where email IS NULL.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_email_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email
    ON users(email)
    WHERE email IS NOT NULL;

-- Dev convenience: give the seeded admin user a default phone so it can log in
-- after this migration. Safe no-op if the row was removed or already has a
-- phone. The dev password for this account is whatever was seeded in
-- 001_init_schema.up.sql.
UPDATE users
SET phone = '13800000000',
    phone_verified_at = NOW(),
    updated_at = NOW()
WHERE email = 'admin@admin.com'
  AND phone IS NULL;
