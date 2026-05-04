ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE users
    SET is_admin = TRUE,
        role = 'user',
        updated_at = NOW()
    WHERE role = 'admin';

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'premium'));

CREATE INDEX IF NOT EXISTS idx_users_is_admin ON users(is_admin);
