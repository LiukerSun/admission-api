-- Roll back to the 004 default admin phone (13800000000). phone_verified_at
-- is left as-is — we cannot tell whether it was set by 004 or by an actual
-- verification flow, and clearing it would log the admin out unnecessarily.

UPDATE users
SET phone      = '13800000000',
    updated_at = NOW()
WHERE email = 'admin@admin.com';
