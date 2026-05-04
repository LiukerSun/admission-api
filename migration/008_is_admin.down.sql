DROP INDEX IF EXISTS idx_users_is_admin;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'premium', 'admin'));

UPDATE users
    SET role = 'admin',
        updated_at = NOW()
    WHERE is_admin = TRUE;

ALTER TABLE users
    DROP COLUMN IF EXISTS is_admin;
