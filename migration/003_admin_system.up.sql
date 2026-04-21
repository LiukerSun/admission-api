-- Fix user_type constraint: remove 'admin', keep only 'parent' and 'student'
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_user_type_check;

ALTER TABLE users
    ADD CONSTRAINT users_user_type_check CHECK (user_type IN ('parent', 'student'));

-- Update any existing 'admin' user_type to 'student'
UPDATE users
    SET user_type = 'student'
    WHERE user_type = 'admin';

-- Fix role constraint: add 'premium' between 'user' and 'admin'
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'premium', 'admin'));

-- Add status field for user status management
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'banned'));

-- Add username field (optional)
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS username VARCHAR(50);

-- Create indexes for admin queries
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
