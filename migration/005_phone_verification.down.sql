DROP INDEX IF EXISTS idx_users_phone;

ALTER TABLE users
    DROP COLUMN IF EXISTS phone_verified_at;

ALTER TABLE users
    DROP COLUMN IF EXISTS phone;
