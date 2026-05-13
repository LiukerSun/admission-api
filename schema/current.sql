-- Current application schema after removing the gaokao domain.
-- This file is a readable snapshot, not a golang-migrate migration.

CREATE TABLE users (
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

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_role ON users(role);
CREATE UNIQUE INDEX idx_users_phone
    ON users(phone)
    WHERE phone IS NOT NULL;
CREATE INDEX idx_users_is_admin ON users(is_admin);

CREATE TABLE membership_plans (
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

CREATE INDEX idx_membership_plans_status
    ON membership_plans(status);

CREATE TABLE payment_orders (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_type VARCHAR(32) NOT NULL CHECK (product_type IN ('membership')),
    product_ref_id BIGINT NOT NULL REFERENCES membership_plans(id),
    subject VARCHAR(200) NOT NULL,
    amount INTEGER NOT NULL CHECK (amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    order_status VARCHAR(32) NOT NULL DEFAULT 'awaiting_payment'
        CHECK (order_status IN ('created', 'awaiting_payment', 'paid', 'fulfilled', 'closed', 'failed')),
    payment_status VARCHAR(32) NOT NULL DEFAULT 'unpaid'
        CHECK (payment_status IN ('unpaid', 'paying', 'paid', 'failed')),
    entitlement_status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (entitlement_status IN ('pending', 'granted', 'failed')),
    payment_channel VARCHAR(32) NOT NULL DEFAULT 'mock',
    idempotency_key VARCHAR(128),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_orders_user_created
    ON payment_orders(user_id, created_at DESC);
CREATE INDEX idx_payment_orders_status
    ON payment_orders(order_status, payment_status, entitlement_status);
CREATE INDEX idx_payment_orders_product
    ON payment_orders(product_type, product_ref_id);
CREATE UNIQUE INDEX uq_payment_orders_user_idempotency
    ON payment_orders(user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE payment_attempts (
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

CREATE INDEX idx_payment_attempts_order
    ON payment_attempts(payment_order_id, created_at DESC);
CREATE UNIQUE INDEX uq_payment_attempts_channel_trade
    ON payment_attempts(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE TABLE payment_callbacks (
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

CREATE UNIQUE INDEX uq_payment_callbacks_channel_callback
    ON payment_callbacks(channel, callback_id);
CREATE INDEX idx_payment_callbacks_trade
    ON payment_callbacks(channel, channel_trade_no)
    WHERE channel_trade_no IS NOT NULL;

CREATE TABLE user_memberships (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    membership_level VARCHAR(20) NOT NULL DEFAULT 'premium'
        CHECK (membership_level IN ('premium')),
    status VARCHAR(20) NOT NULL DEFAULT 'inactive'
        CHECK (status IN ('inactive', 'active', 'expired')),
    started_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    last_order_id BIGINT REFERENCES payment_orders(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_memberships_active_lookup
    ON user_memberships(user_id, ends_at)
    WHERE status = 'active';

CREATE TABLE membership_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE CASCADE,
    source_type VARCHAR(32) NOT NULL DEFAULT 'payment'
        CHECK (source_type IN ('payment')),
    action VARCHAR(32) NOT NULL
        CHECK (action IN ('activate', 'renew', 'extend', 'restore')),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_membership_grants_user_created
    ON membership_grants(user_id, created_at DESC);

CREATE TABLE regions (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE subject_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE subject_requirements (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    normalized_subjects JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE batches (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE education_levels (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE school_ownership_types (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE school_categories (
    code VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE major_categories (
    id BIGSERIAL PRIMARY KEY,
    catalog_year INTEGER NOT NULL,
    category_code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_year, category_code)
);

CREATE TABLE major_classes (
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

CREATE TABLE standard_majors (
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

CREATE INDEX idx_standard_majors_catalog_name
    ON standard_majors(catalog_year, name);

CREATE INDEX idx_standard_majors_catalog_class
    ON standard_majors(catalog_year, class_code);

CREATE TABLE universities (
    id BIGSERIAL PRIMARY KEY,
    university_code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    normalized_name VARCHAR(200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_code, name)
);

CREATE INDEX idx_universities_name
    ON universities(name);

CREATE TABLE university_profiles (
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
    university_tier VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, profile_year)
);

CREATE INDEX idx_university_profiles_year
    ON university_profiles(university_id, profile_year DESC);

CREATE INDEX idx_university_profiles_tier
    ON university_profiles(university_tier);

CREATE TABLE admission_groups (
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

CREATE INDEX idx_admission_groups_lookup
    ON admission_groups(admission_year, region_code, subject_category_code, university_id, group_code);

CREATE INDEX idx_admission_groups_university_year
    ON admission_groups(university_id, admission_year DESC);

CREATE TABLE admission_group_extensions (
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

CREATE INDEX idx_admission_group_extensions_score
    ON admission_group_extensions(group_min_rank, group_min_score);

CREATE TABLE university_major_admissions (
    id BIGSERIAL PRIMARY KEY,
    admission_group_id BIGINT NOT NULL REFERENCES admission_groups(id) ON DELETE CASCADE,
    local_major_code VARCHAR(50) NOT NULL,
    local_major_name TEXT NOT NULL,
    plan_count INTEGER,
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

CREATE INDEX idx_major_admissions_group
    ON university_major_admissions(admission_group_id, local_major_code);

CREATE INDEX idx_major_admissions_local_name
    ON university_major_admissions(local_major_name);

CREATE INDEX idx_major_admissions_rank
    ON university_major_admissions(min_rank, min_score);

CREATE TABLE university_major_profiles (
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

CREATE INDEX idx_university_major_profiles_discipline
    ON university_major_profiles(discipline_category, first_level_discipline);

CREATE INDEX idx_university_major_profiles_rank
    ON university_major_profiles(major_rank);

CREATE TABLE university_postgraduate_profiles (
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

CREATE INDEX idx_university_postgraduate_profiles_year
    ON university_postgraduate_profiles(university_id, profile_year DESC);

CREATE TABLE admission_major_tags (
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

CREATE INDEX idx_admission_major_tags_category
    ON admission_major_tags(catalog_year, category_code);

CREATE INDEX idx_admission_major_tags_class
    ON admission_major_tags(catalog_year, class_code)
    WHERE class_code IS NOT NULL;

CREATE INDEX idx_admission_major_tags_major
    ON admission_major_tags(catalog_year, major_code)
    WHERE major_code IS NOT NULL;

CREATE INDEX idx_admission_major_tags_standard_major
    ON admission_major_tags(standard_major_id)
    WHERE standard_major_id IS NOT NULL;

-- Recommendation algorithm metadata (added in migration 008)

CREATE TABLE city_groups (
    code VARCHAR(32) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE city_group_members (
    id BIGSERIAL PRIMARY KEY,
    city_group_code VARCHAR(32) NOT NULL REFERENCES city_groups(code) ON DELETE CASCADE,
    city VARCHAR(100) NOT NULL,
    UNIQUE (city_group_code, city)
);

CREATE INDEX idx_city_group_members_city
    ON city_group_members(city);

CREATE TABLE recommendation_family_resource_keywords (
    id BIGSERIAL PRIMARY KEY,
    resource_code VARCHAR(50) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (resource_code, keyword)
);

CREATE INDEX idx_family_resource_keywords_resource
    ON recommendation_family_resource_keywords(resource_code);

CREATE TABLE recommendation_holland_keywords (
    id BIGSERIAL PRIMARY KEY,
    riasec_code CHAR(1) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (riasec_code, keyword)
);

CREATE INDEX idx_holland_keywords_code
    ON recommendation_holland_keywords(riasec_code);

CREATE TABLE recommendation_major_ability_rules (
    id BIGSERIAL PRIMARY KEY,
    chsi_category_code VARCHAR(10) NOT NULL,
    subject VARCHAR(20) NOT NULL,
    exclude_below_score INTEGER NOT NULL,
    warn_below_score INTEGER NOT NULL,
    note VARCHAR(255),
    UNIQUE (chsi_category_code, subject)
);

CREATE INDEX idx_major_ability_rules_category
    ON recommendation_major_ability_rules(chsi_category_code);

CREATE TABLE recommendation_precomputed_scores (
    id BIGSERIAL PRIMARY KEY,
    university_id    BIGINT       NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    local_major_code VARCHAR(50)  NOT NULL,
    city_score                   NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    school_score                 NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    major_score                  NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    ability_improvement_score    NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    future_competitiveness_score NUMERIC(5,3) NOT NULL DEFAULT 1.000,
    city_reason                  TEXT NOT NULL DEFAULT '',
    school_reason                TEXT NOT NULL DEFAULT '',
    major_reason                 TEXT NOT NULL DEFAULT '',
    ability_improvement_reason   TEXT NOT NULL DEFAULT '',
    future_competitiveness_reason TEXT NOT NULL DEFAULT '',
    evaluated_by    VARCHAR(32)  NOT NULL DEFAULT 'algorithm',
    evaluator_model VARCHAR(120),
    evaluated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (university_id, local_major_code)
);

CREATE INDEX idx_recommendation_precomputed_scores_lookup
    ON recommendation_precomputed_scores(university_id, local_major_code);

CREATE INDEX idx_recommendation_precomputed_scores_evaluated_at
    ON recommendation_precomputed_scores(evaluated_at);

CREATE INDEX idx_recommendation_precomputed_scores_evaluated_by
    ON recommendation_precomputed_scores(evaluated_by);

CREATE TABLE conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_conversations_user_id
    ON conversations(user_id, updated_at DESC);

CREATE INDEX idx_conversations_status
    ON conversations(status, updated_at DESC);

CREATE TABLE conversation_messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tool_calls JSONB,
    tool_results JSONB,
    widgets JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_conversation_messages_conversation_id
    ON conversation_messages(conversation_id, created_at ASC);

CREATE TABLE conversation_filters (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    filter_type VARCHAR(50) NOT NULL,
    filter_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_conversation_filters_conversation_id
    ON conversation_filters(conversation_id, created_at DESC);

CREATE TABLE conversation_plan_drafts (
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

CREATE INDEX idx_conversation_plan_drafts_user_created
    ON conversation_plan_drafts(user_id, created_at DESC);

CREATE INDEX idx_conversation_plan_drafts_conversation_created
    ON conversation_plan_drafts(conversation_id, created_at DESC);

CREATE TABLE user_volunteer_plans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL DEFAULT '',
    source_draft_id BIGINT REFERENCES conversation_plan_drafts(id) ON DELETE SET NULL,
    plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, source_draft_id)
);

CREATE INDEX idx_user_volunteer_plans_user_created
    ON user_volunteer_plans(user_id, created_at DESC);
