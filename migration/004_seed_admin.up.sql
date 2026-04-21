-- Seed default admin account
-- Email: admin@admin.com
-- Password: admin1234
-- Change the password immediately after first login.

INSERT INTO users (email, password_hash, role, user_type, status, username)
VALUES (
    'admin@admin.com',
    '$2a$10$mcQZqW6NK2qhnzBAs5xp2OjgTGXbnavl9LPXzsyFS9zCr1gQMfKvC',
    'admin',
    'parent',
    'active',
    'admin'
)
ON CONFLICT (email) DO NOTHING;
