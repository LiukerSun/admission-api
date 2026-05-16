-- ============================================================================
-- 005_admin_phone_seed: refresh the seeded admin phone to 13888888888.
--
-- 004_phone_first_auth backfilled the admin (admin@admin.com) row with phone
-- 13800000000 so it could log in after email became optional. Product wants
-- 13888888888 instead for the demo / hand-testing flow. The password remains
-- whatever 001_init_schema seeded (bcrypt of admin1234) — we only touch the
-- phone column here.
--
-- The UPDATE is unconditional on phone value so it works on both:
--   * Fresh DBs that ran 004 (phone = 13800000000) → flipped to 13888888888
--   * Already-customized envs where someone hand-set a phone → also flipped
-- If another user happens to already hold 13888888888 the partial unique
-- index idx_users_phone will reject the update; that is a real data-collision
-- signal an operator should resolve, not something to silently swallow.
-- ============================================================================

UPDATE users
SET phone             = '13888888888',
    phone_verified_at = COALESCE(phone_verified_at, NOW()),
    updated_at        = NOW()
WHERE email = 'admin@admin.com';
