-- Remove indexes
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_status;
DROP INDEX IF EXISTS idx_users_role;

-- Remove new columns
ALTER TABLE users DROP COLUMN IF EXISTS status;
ALTER TABLE users DROP COLUMN IF EXISTS username;

-- Revert role constraint to original
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));

-- Revert user_type constraint to original (with 'admin')
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_user_type_check;

ALTER TABLE users
    ADD CONSTRAINT users_user_type_check CHECK (user_type IN ('parent', 'student', 'admin'));
