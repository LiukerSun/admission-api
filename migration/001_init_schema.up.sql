-- ============================================================================
-- Compact baseline schema for the admission service.
--
-- All previous incremental migrations (001-015) are folded into this single
-- file. Sections are ordered by FK dependencies:
--
--   1. Accounts / membership / payments / refunds
--   2. Dictionaries (regions, subjects, batches, school taxonomies)
--   3. National major catalog
--   4. Universities + profiles
--   5. Admission groups and major-level admission rows
--   6. Conversations, plan drafts, saved volunteer plans
--   7. Recommendation algorithm metadata (tiers, city groups, keywords, scores)
--   8. Seed data: admin user, membership plans, university tier backfill
-- ============================================================================


-- ----------------------------------------------------------------------------
-- 1. Accounts / membership / payments / refunds
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user'
        CHECK (role IN ('user', 'premium')),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'banned')),
    username VARCHAR(50),
    phone VARCHAR(20),
    phone_verified_at TIMESTAMPTZ,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone
    ON users(phone)
    WHERE phone IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_is_admin ON users(is_admin);

CREATE TABLE IF NOT EXISTS membership_plans (
    id BIGSERIAL PRIMARY KEY,
    plan_code VARCHAR(32) NOT NULL UNIQUE,
    plan_name VARCHAR(100) NOT NULL,
    membership_level VARCHAR(20) NOT NULL DEFAULT 'premium'
        CHECK (membership_level IN ('premium')),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    price_amount INTEGER NOT NULL CHECK (price_amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_membership_plans_status
    ON membership_plans(status);

CREATE TABLE IF NOT EXISTS payment_orders (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_type VARCHAR(32) NOT NULL CHECK (product_type IN ('membership')),
    product_ref_id BIGINT NOT NULL REFERENCES membership_plans(id),
    subject VARCHAR(200) NOT NULL,
    amount INTEGER NOT NULL CHECK (amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    order_status VARCHAR(32) NOT NULL DEFAULT 'awaiting_payment'
        CHECK (order_status IN ('created', 'awaiting_payment', 'paid', 'fulfilled', 'closed', 'failed', 'refunded')),
    payment_status VARCHAR(32) NOT NULL DEFAULT 'unpaid'
        CHECK (payment_status IN ('unpaid', 'paying', 'paid', 'failed')),
    entitlement_status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (entitlement_status IN ('pending', 'granted', 'failed', 'revoked')),
    payment_channel VARCHAR(32) NOT NULL DEFAULT 'mock',
    idempotency_key VARCHAR(128),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_orders_user_created
    ON payment_orders(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payment_orders_status
    ON payment_orders(order_status, payment_status, entitlement_status);
CREATE INDEX IF NOT EXISTS idx_payment_orders_product
    ON payment_orders(product_type, product_ref_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_orders_user_idempotency
    ON payment_orders(user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS payment_attempts (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
    channel VARCHAR(32) NOT NULL DEFAULT 'mock',
    channel_trade_no VARCHAR(128),
    channel_status VARCHAR(32) NOT NULL DEFAULT 'created'
        CHECK (channel_status IN ('created', 'pending', 'success', 'failed', 'closed')),
    amount INTEGER NOT NULL CHECK (amount >= 0),
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    callback_received_at TIMESTAMPTZ,
    success_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (payment_order_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_order
    ON payment_attempts(payment_order_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_channel_trade
    ON payment_attempts(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE TABLE IF NOT EXISTS payment_callbacks (
    id BIGSERIAL PRIMARY KEY,
    channel VARCHAR(32) NOT NULL,
    callback_id VARCHAR(128) NOT NULL,
    channel_trade_no VARCHAR(128),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMPTZ,
    process_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_callbacks_channel_callback
    ON payment_callbacks(channel, callback_id);
CREATE INDEX IF NOT EXISTS idx_payment_callbacks_trade
    ON payment_callbacks(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_memberships (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    membership_level VARCHAR(20) NOT NULL DEFAULT 'premium'
        CHECK (membership_level IN ('premium')),
    status VARCHAR(20) NOT NULL DEFAULT 'inactive'
        CHECK (status IN ('inactive', 'active', 'expired', 'refunded')),
    started_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    last_order_id BIGINT REFERENCES payment_orders(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_memberships_active_lookup
    ON user_memberships(user_id, ends_at)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS membership_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    source_type VARCHAR(32) NOT NULL DEFAULT 'payment'
        CHECK (source_type IN ('payment')),
    action VARCHAR(32) NOT NULL
        CHECK (action IN ('activate', 'renew', 'extend', 'restore', 'revoke')),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_membership_grants_user_created
    ON membership_grants(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS payment_refunds (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    refund_no VARCHAR(64) NOT NULL UNIQUE,
    out_request_no VARCHAR(64) NOT NULL UNIQUE,
    channel VARCHAR(32) NOT NULL DEFAULT 'alipay',
    channel_refund_no VARCHAR(128),
    refund_amount INTEGER NOT NULL CHECK (refund_amount > 0),
    total_order_amount INTEGER NOT NULL,
    refund_reason VARCHAR(256),
    status VARCHAR(32) NOT NULL DEFAULT 'pending_review'
        CHECK (status IN ('pending_review', 'rejected', 'approved', 'processing', 'success', 'failed')),
    review_note VARCHAR(512),
    reviewed_by BIGINT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    initiated_by BIGINT REFERENCES users(id),
    refunded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_refunds_order
    ON payment_refunds(payment_order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payment_refunds_status_created
    ON payment_refunds(status, created_at DESC);
-- One pending review per order at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_refunds_pending_per_order
    ON payment_refunds(payment_order_id)
    WHERE status = 'pending_review';


-- ----------------------------------------------------------------------------
-- 2. Dictionaries
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS regions (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subject_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subject_requirements (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    normalized_subjects JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS batches (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS education_levels (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS school_ownership_types (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS school_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


-- ----------------------------------------------------------------------------
-- 3. National major catalog
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS major_categories (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, category_code)
);

CREATE TABLE IF NOT EXISTS major_classes (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    class_code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, class_code),
    FOREIGN KEY (catalog_year, category_code)
        REFERENCES major_categories (catalog_year, category_code)
);

CREATE TABLE IF NOT EXISTS standard_majors (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    major_code VARCHAR(20) NOT NULL,
    name VARCHAR(200) NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    class_code VARCHAR(20) NOT NULL,
    duration VARCHAR(50),
    degree_category VARCHAR(100),
    source_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, major_code),
    FOREIGN KEY (catalog_year, category_code)
        REFERENCES major_categories (catalog_year, category_code),
    FOREIGN KEY (catalog_year, class_code)
        REFERENCES major_classes (catalog_year, class_code)
);

CREATE INDEX IF NOT EXISTS idx_standard_majors_catalog_name
    ON standard_majors(catalog_year, name);

CREATE INDEX IF NOT EXISTS idx_standard_majors_catalog_class
    ON standard_majors(catalog_year, class_code);


-- ----------------------------------------------------------------------------
-- 4. Universities + profiles
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS universities (
    id BIGSERIAL PRIMARY KEY,
    university_code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    normalized_name VARCHAR(200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_code, name)
);

CREATE INDEX IF NOT EXISTS idx_universities_name
    ON universities(name);

CREATE TABLE IF NOT EXISTS university_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    profile_year INTEGER NOT NULL,
    region_code VARCHAR(20) REFERENCES regions(code),
    city VARCHAR(100),
    ownership_type_code VARCHAR(50) REFERENCES school_ownership_types(code),
    school_category_code VARCHAR(50) REFERENCES school_categories(code),
    education_level_code VARCHAR(50) REFERENCES education_levels(code),
    is_985 BOOLEAN,
    is_211 BOOLEAN,
    is_double_first_class BOOLEAN,
    is_national_key BOOLEAN,
    is_provincial_key BOOLEAN,
    has_postgraduate_recommendation BOOLEAN,
    postgraduate_recommendation_rate NUMERIC(5,2),
    soft_rank VARCHAR(100),
    alumni_rank VARCHAR(100),
    difficulty_rank VARCHAR(100),
    doctoral_program_count INTEGER,
    master_program_count INTEGER,
    national_key_subject_count INTEGER,
    affiliation VARCHAR(200),
    school_level_tags TEXT,
    excellence_tags TEXT,
    -- 清北 / 华5 / C9 / 985 / 211 / ... Used by the volunteer recommendation
    -- algorithm; NULL = fall back to is_985 / is_211 / is_double_first_class.
    university_tier VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, profile_year)
);

CREATE INDEX IF NOT EXISTS idx_university_profiles_year
    ON university_profiles(university_id, profile_year DESC);

CREATE INDEX IF NOT EXISTS idx_university_profiles_tier
    ON university_profiles(university_tier);


-- ----------------------------------------------------------------------------
-- 5. Admission groups and major-level admission rows
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS admission_groups (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    admission_year INTEGER NOT NULL,
    region_code VARCHAR(20) NOT NULL REFERENCES regions(code),
    subject_category_code VARCHAR(50) NOT NULL REFERENCES subject_categories(code),
    batch_code VARCHAR(50) NOT NULL REFERENCES batches(code),
    group_code VARCHAR(50) NOT NULL,
    subject_requirement_code VARCHAR(50) REFERENCES subject_requirements(code),
    education_level_code VARCHAR(50) REFERENCES education_levels(code),
    group_major_count INTEGER,
    group_major_names TEXT,
    group_type VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (
        university_id,
        admission_year,
        region_code,
        subject_category_code,
        batch_code,
        group_code
    )
);

CREATE INDEX IF NOT EXISTS idx_admission_groups_lookup
    ON admission_groups(admission_year, region_code, subject_category_code, university_id, group_code);

CREATE INDEX IF NOT EXISTS idx_admission_groups_university_year
    ON admission_groups(university_id, admission_year DESC);

CREATE TABLE IF NOT EXISTS admission_group_extensions (
    id BIGSERIAL PRIMARY KEY,
    admission_group_id BIGINT NOT NULL UNIQUE REFERENCES admission_groups(id) ON DELETE CASCADE,
    batch_remark TEXT,
    group_min_score INTEGER,
    group_min_rank INTEGER,
    equivalent_min_score_2024 INTEGER,
    equivalent_min_score_2023 INTEGER,
    equivalent_min_score_2022 INTEGER,
    subject_change_2024 TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admission_group_extensions_score
    ON admission_group_extensions(group_min_rank, group_min_score);

CREATE TABLE IF NOT EXISTS university_major_admissions (
    id BIGSERIAL PRIMARY KEY,
    admission_group_id BIGINT NOT NULL REFERENCES admission_groups(id) ON DELETE CASCADE,
    local_major_code VARCHAR(50) NOT NULL,
    local_major_name TEXT NOT NULL,
    -- 招生人数；2024 为实际录取数据，2025/2023/2022 为招生计划数。
    admitted_count INTEGER,
    min_score INTEGER,
    min_rank INTEGER,
    max_score INTEGER,
    max_rank INTEGER,
    equivalent_min_score INTEGER,
    tuition INTEGER,
    duration VARCHAR(50),
    admission_remark TEXT,
    major_intro TEXT,
    training_goal TEXT,
    subject_study_requirement TEXT,
    main_courses TEXT,
    postgraduate_direction TEXT,
    employment_direction TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (admission_group_id, local_major_code)
);

CREATE INDEX IF NOT EXISTS idx_major_admissions_group
    ON university_major_admissions(admission_group_id, local_major_code);

CREATE INDEX IF NOT EXISTS idx_major_admissions_local_name
    ON university_major_admissions(local_major_name);

CREATE INDEX IF NOT EXISTS idx_major_admissions_rank
    ON university_major_admissions(min_rank, min_score);

CREATE TABLE IF NOT EXISTS university_major_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_major_admission_id BIGINT NOT NULL UNIQUE REFERENCES university_major_admissions(id) ON DELETE CASCADE,
    discipline_category VARCHAR(100),
    first_level_discipline VARCHAR(200),
    fourth_round_subject_eval VARCHAR(100),
    double_first_class_subject TEXT,
    soft_major_grade VARCHAR(100),
    major_evaluation_score NUMERIC(6,2),
    major_rank VARCHAR(100),
    is_national_feature BOOLEAN,
    corresponding_master_majors TEXT,
    corresponding_doctoral_majors TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_university_major_profiles_discipline
    ON university_major_profiles(discipline_category, first_level_discipline);

CREATE INDEX IF NOT EXISTS idx_university_major_profiles_rank
    ON university_major_profiles(major_rank);

CREATE TABLE IF NOT EXISTS university_postgraduate_profiles (
    id BIGSERIAL PRIMARY KEY,
    university_id BIGINT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    profile_year INTEGER NOT NULL,
    master_major_count INTEGER,
    master_major_names TEXT,
    doctoral_major_count INTEGER,
    doctoral_major_names TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, profile_year)
);

CREATE INDEX IF NOT EXISTS idx_university_postgraduate_profiles_year
    ON university_postgraduate_profiles(university_id, profile_year DESC);

CREATE TABLE IF NOT EXISTS admission_major_tags (
    id BIGSERIAL PRIMARY KEY,
    university_major_admission_id BIGINT NOT NULL REFERENCES university_major_admissions(id) ON DELETE CASCADE,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    category_name VARCHAR(100) NOT NULL,
    class_code VARCHAR(20),
    class_name VARCHAR(100),
    major_code VARCHAR(20),
    major_name VARCHAR(200),
    standard_major_id BIGINT REFERENCES standard_majors(id),
    tag_level VARCHAR(20) NOT NULL
        CHECK (tag_level IN ('category', 'class', 'major')),
    note TEXT,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (
        university_major_admission_id,
        catalog_year,
        category_code,
        class_code,
        major_code
    )
);

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_category
    ON admission_major_tags(catalog_year, category_code);

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_class
    ON admission_major_tags(catalog_year, class_code)
    WHERE class_code IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_major
    ON admission_major_tags(catalog_year, major_code)
    WHERE major_code IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admission_major_tags_standard_major
    ON admission_major_tags(standard_major_id)
    WHERE standard_major_id IS NOT NULL;


-- ----------------------------------------------------------------------------
-- 6. Conversations, plan drafts, saved volunteer plans
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    title VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversations_user_id
    ON conversations(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversations_status
    ON conversations(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS conversation_messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tool_calls JSONB,
    tool_results JSONB,
    -- Array of {id, kind, payload} produced by render_chart / render_card tools.
    widgets JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_messages_conversation_id
    ON conversation_messages(conversation_id, created_at ASC);

CREATE TABLE IF NOT EXISTS conversation_filters (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    filter_type VARCHAR(50) NOT NULL,
    filter_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_filters_conversation_id
    ON conversation_filters(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS conversation_plan_drafts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'generating',
    input_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    plan_json JSONB,
    algorithm_version VARCHAR(64) NOT NULL DEFAULT '',
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_plan_drafts_user_created
    ON conversation_plan_drafts(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_plan_drafts_conversation_created
    ON conversation_plan_drafts(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS user_volunteer_plans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    source_draft_id BIGINT REFERENCES conversation_plan_drafts(id) ON DELETE SET NULL,
    plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, source_draft_id)
);

CREATE INDEX IF NOT EXISTS idx_user_volunteer_plans_user_created
    ON user_volunteer_plans(user_id, created_at DESC);


-- ----------------------------------------------------------------------------
-- 7. Recommendation algorithm metadata
--
-- city_groups + city_group_members:                城市群分组
-- recommendation_family_resource_keywords:         家庭资源 -> 专业关键词指向
-- recommendation_holland_keywords:                 RIASEC -> 学科关键词
-- recommendation_major_ability_rules:              CHSI 大类 -> 单科分数门槛
-- recommendation_precomputed_scores:               五维 base scores (LLM/algo)
-- recommendation_strategy_keywords:                stem/humanities 倾向关键词
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS city_groups (
    code VARCHAR(32) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS city_group_members (
    id BIGSERIAL PRIMARY KEY,
    city_group_code VARCHAR(32) NOT NULL REFERENCES city_groups(code) ON DELETE CASCADE,
    city VARCHAR(100) NOT NULL,
    UNIQUE (city_group_code, city)
);

CREATE INDEX IF NOT EXISTS idx_city_group_members_city
    ON city_group_members(city);

CREATE TABLE IF NOT EXISTS recommendation_family_resource_keywords (
    id BIGSERIAL PRIMARY KEY,
    resource_code VARCHAR(50) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (resource_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_family_resource_keywords_resource
    ON recommendation_family_resource_keywords(resource_code);

CREATE TABLE IF NOT EXISTS recommendation_holland_keywords (
    id BIGSERIAL PRIMARY KEY,
    riasec_code CHAR(1) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (riasec_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_holland_keywords_code
    ON recommendation_holland_keywords(riasec_code);

CREATE TABLE IF NOT EXISTS recommendation_major_ability_rules (
    id BIGSERIAL PRIMARY KEY,
    chsi_category_code VARCHAR(10) NOT NULL,
    subject VARCHAR(20) NOT NULL,           -- 'physics' | 'math' | 'chinese' | 'english'
    exclude_below_score INTEGER NOT NULL,   -- 低于此分数 → 直接淘汰
    warn_below_score INTEGER NOT NULL,      -- 低于此分数 → 仅告警，不淘汰 (>= exclude_below_score)
    note VARCHAR(255),
    UNIQUE (chsi_category_code, subject)
);

CREATE INDEX IF NOT EXISTS idx_major_ability_rules_category
    ON recommendation_major_ability_rules(chsi_category_code);

-- Precomputed five-dimension scores keyed by (university_id, local_major_code).
-- Refresh queue: rows where evaluated_at IS NULL OR evaluated_at < NOW() - INTERVAL '90 days'.
CREATE TABLE IF NOT EXISTS recommendation_precomputed_scores (
    id BIGSERIAL PRIMARY KEY,
    university_id    BIGINT       NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    local_major_code VARCHAR(50)  NOT NULL,

    city_score                   NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    school_score                 NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    major_score                  NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    ability_improvement_score    NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    future_competitiveness_score NUMERIC(5,3) NOT NULL DEFAULT 1.000,

    city_reason                   TEXT NOT NULL DEFAULT '',
    school_reason                 TEXT NOT NULL DEFAULT '',
    major_reason                  TEXT NOT NULL DEFAULT '',
    ability_improvement_reason    TEXT NOT NULL DEFAULT '',
    future_competitiveness_reason TEXT NOT NULL DEFAULT '',

    evaluated_by    VARCHAR(32)  NOT NULL DEFAULT 'algorithm', -- algorithm | llm | manual
    evaluator_model VARCHAR(120),                              -- e.g. claude-opus-4-7
    evaluated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (university_id, local_major_code)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_lookup
    ON recommendation_precomputed_scores(university_id, local_major_code);

CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_evaluated_at
    ON recommendation_precomputed_scores(evaluated_at);

CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_evaluated_by
    ON recommendation_precomputed_scores(evaluated_by);

CREATE TABLE IF NOT EXISTS recommendation_strategy_keywords (
    id BIGSERIAL PRIMARY KEY,
    intent_code VARCHAR(32) NOT NULL,            -- 'stem' | 'humanities'
    keyword     VARCHAR(100) NOT NULL,
    UNIQUE (intent_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_strategy_keywords_intent
    ON recommendation_strategy_keywords(intent_code);


-- ============================================================================
-- 8. Seed data
-- ============================================================================

-- Default admin account (password is bcrypt of project-internal default).
INSERT INTO users (email, password_hash, role, is_admin, status, username)
VALUES (
    'admin@admin.com',
    '$2a$10$mcQZqW6NK2qhnzBAs5xp2OjgTGXbnavl9LPXzsyFS9zCr1gQMfKvC',
    'user',
    TRUE,
    'active',
    'admin'
)
ON CONFLICT (email) DO UPDATE
SET is_admin = TRUE,
    status = 'active',
    updated_at = NOW();

INSERT INTO membership_plans (plan_code, plan_name, membership_level, duration_days, price_amount, currency, status)
VALUES
    ('monthly',   '月度会员', 'premium', 30,  990,  'CNY', 'active'),
    ('quarterly', '季度会员', 'premium', 90,  2690, 'CNY', 'active'),
    ('yearly',    '年度会员', 'premium', 365, 9990, 'CNY', 'active')
ON CONFLICT (plan_code) DO UPDATE
SET plan_name = EXCLUDED.plan_name,
    membership_level = EXCLUDED.membership_level,
    duration_days = EXCLUDED.duration_days,
    price_amount = EXCLUDED.price_amount,
    currency = EXCLUDED.currency,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO regions (code, name)
VALUES
    ('230000', '黑龙江省'),
    ('110000', '北京市')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_categories (code, name)
VALUES
    ('physics', '物理'),
    ('history', '历史'),
    ('science', '理科'),
    ('arts',    '文科')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO subject_requirements (code, name, normalized_subjects)
VALUES
    ('none',              '不限',     '[]'::jsonb),
    ('chemistry',         '化学',     '["化学"]'::jsonb),
    ('physics_chemistry', '物理+化学', '["物理", "化学"]'::jsonb)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    normalized_subjects = EXCLUDED.normalized_subjects,
    updated_at = NOW();

INSERT INTO batches (code, name)
VALUES
    ('regular_undergraduate', '普通本科批'),
    ('early_undergraduate',   '本科提前批'),
    ('specialist',            '专科批')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO education_levels (code, name)
VALUES
    ('undergraduate', '本科'),
    ('specialist',    '专科')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_ownership_types (code, name)
VALUES
    ('public',       '公办'),
    ('private',      '民办'),
    ('sino_foreign', '中外合作办学')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

INSERT INTO school_categories (code, name)
VALUES
    ('comprehensive',        '综合类'),
    ('science_engineering',  '理工类'),
    ('medicine',             '医药类'),
    ('normal',               '师范类'),
    ('finance_economics',    '财经类'),
    ('agriculture_forestry', '农林类'),
    ('language',             '语言类'),
    ('politics_law',         '政法类'),
    ('art',                  '艺术类'),
    ('sports',               '体育类')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    updated_at = NOW();

-- City groups + members
INSERT INTO city_groups (code, name, sort_order) VALUES
    ('jjj', '京津冀城市群',   10),
    ('yrd', '长三角城市群',   20),
    ('gba', '粤港澳大湾区',   30),
    ('cd',  '成渝城市群',     40),
    ('gz',  '关中平原城市群', 50),
    ('ne',  '东北城市群',     60)
ON CONFLICT (code) DO NOTHING;

INSERT INTO city_group_members (city_group_code, city) VALUES
    ('jjj', '北京'),  ('jjj', '天津'),  ('jjj', '雄安'),  ('jjj', '石家庄'), ('jjj', '保定'),
    ('yrd', '上海'),  ('yrd', '南京'),  ('yrd', '杭州'),  ('yrd', '苏州'),  ('yrd', '无锡'),
    ('yrd', '合肥'),  ('yrd', '宁波'),  ('yrd', '常州'),  ('yrd', '南通'),  ('yrd', '嘉兴'),
    ('yrd', '绍兴'),  ('yrd', '镇江'),  ('yrd', '扬州'),  ('yrd', '芜湖'),  ('yrd', '马鞍山'),
    ('gba', '广州'),  ('gba', '深圳'),  ('gba', '珠海'),  ('gba', '香港'),  ('gba', '澳门'),
    ('gba', '佛山'),  ('gba', '东莞'),  ('gba', '中山'),  ('gba', '惠州'),  ('gba', '江门'),
    ('gba', '肇庆'),
    ('cd',  '重庆'),  ('cd',  '成都'),  ('cd',  '绵阳'),  ('cd',  '德阳'),
    ('gz',  '西安'),  ('gz',  '咸阳'),  ('gz',  '宝鸡'),  ('gz',  '渭南'),  ('gz',  '铜川'),
    ('ne',  '哈尔滨'), ('ne', '长春'),  ('ne',  '沈阳'),  ('ne',  '大连'),  ('ne',  '吉林')
ON CONFLICT (city_group_code, city) DO NOTHING;

-- Family-resource → major keyword 指向
INSERT INTO recommendation_family_resource_keywords (resource_code, keyword, weight) VALUES
    ('公检法', '法学',     1.40),
    ('公检法', '法律',     1.30),
    ('金融',   '金融',     1.30),
    ('金融',   '经济',     1.20),
    ('金融',   '财政',     1.10),
    ('医疗',   '临床',     1.40),
    ('医疗',   '药学',     1.20),
    ('医疗',   '医学',     1.20),
    ('教育',   '教育',     1.30),
    ('教育',   '师范',     1.30),
    ('电网',   '电气',     1.40),
    ('电网',   '能源动力', 1.20),
    ('电网',   '能动',     1.20),
    ('商业',   '工商管理', 1.20),
    ('商业',   '会计',     1.30),
    ('商业',   '财务',     1.20),
    ('从医',   '医学',     1.30)
ON CONFLICT (resource_code, keyword) DO NOTHING;

-- 霍兰德 RIASEC → 学科关键词
INSERT INTO recommendation_holland_keywords (riasec_code, keyword, weight) VALUES
    ('R', '工学',     1.20),
    ('R', '农学',     1.10),
    ('I', '理学',     1.20),
    ('I', '医学',     1.10),
    ('A', '艺术',     1.30),
    ('A', '文学',     1.10),
    ('S', '教育',     1.20),
    ('S', '心理',     1.20),
    ('S', '社会工作', 1.10),
    ('E', '管理',     1.20),
    ('E', '金融',     1.20),
    ('E', '经济',     1.10),
    ('C', '会计',     1.20),
    ('C', '财务',     1.10),
    ('C', '档案',     1.10)
ON CONFLICT (riasec_code, keyword) DO NOTHING;

-- CHSI 大类 → 单科分数门槛
INSERT INTO recommendation_major_ability_rules
    (chsi_category_code, subject, exclude_below_score, warn_below_score, note) VALUES
    ('0807', 'physics', 40, 50, '电子信息类大学物理强度高'),
    ('0808', 'physics', 40, 50, '自动化类大学物理强度高'),
    ('0802', 'physics', 40, 50, '机械类大学物理强度高'),
    ('0826', 'physics', 40, 50, '航空航天类大学物理强度高'),
    ('0810', 'physics', 40, 50, '土木类大学物理强度高'),
    ('0811', 'physics', 40, 50, '水利类大学物理强度高'),
    ('0809', 'math',    50, 70, '计算机类大学数学强度高'),
    ('0712', 'math',    50, 70, '统计学类对数学要求高'),
    ('0701', 'math',    50, 70, '数学类专业对数学要求最高'),
    ('0203', 'math',    50, 70, '金融工程对数学要求高'),
    ('0807', 'math',    30, 60, '电子信息类大学数学也常用'),
    ('0808', 'math',    30, 60, '自动化类大学数学也常用')
ON CONFLICT (chsi_category_code, subject) DO NOTHING;

-- Strategy keyword seeds powering admission.decideStrategy.
INSERT INTO recommendation_strategy_keywords (intent_code, keyword) VALUES
    ('stem', '计算机'),
    ('stem', '电子'),
    ('stem', '电气'),
    ('stem', '自动化'),
    ('stem', '机械'),
    ('stem', '通信'),
    ('stem', '软件'),
    ('stem', '人工智能'),
    ('stem', '数学'),
    ('stem', '物理'),
    ('stem', '土木'),
    ('stem', '航空'),
    ('stem', '材料'),

    ('humanities', '法学'),
    ('humanities', '汉语言'),
    ('humanities', '新闻'),
    ('humanities', '金融'),
    ('humanities', '会计'),
    ('humanities', '经济'),
    ('humanities', '管理'),
    ('humanities', '外语'),
    ('humanities', '教育'),
    ('humanities', '心理')
ON CONFLICT (intent_code, keyword) DO NOTHING;

-- Backfill university_tier for known top schools (latest profile_year only).
-- No-op when university_profiles is empty (e.g. fresh DB before data import).
WITH latest AS (
    SELECT DISTINCT ON (up.university_id) up.id, u.name
    FROM university_profiles up
    JOIN universities u ON u.id = up.university_id
    ORDER BY up.university_id, up.profile_year DESC
)
UPDATE university_profiles up
SET university_tier = CASE l.name
    WHEN '清华大学'             THEN 'top_2'
    WHEN '北京大学'             THEN 'top_2'
    WHEN '复旦大学'             THEN 'hua_5'
    WHEN '上海交通大学'         THEN 'hua_5'
    WHEN '浙江大学'             THEN 'hua_5'
    WHEN '中国科学技术大学'     THEN 'hua_5'
    WHEN '南京大学'             THEN 'hua_5'
    WHEN '西安交通大学'         THEN 'c9'
    WHEN '哈尔滨工业大学'       THEN 'c9'
END
FROM latest l
WHERE up.id = l.id
  AND l.name IN (
    '清华大学','北京大学',
    '复旦大学','上海交通大学','浙江大学','中国科学技术大学','南京大学',
    '西安交通大学','哈尔滨工业大学'
  );
