CREATE TABLE IF NOT EXISTS planner_profiles (
    id                      BIGSERIAL PRIMARY KEY,
    user_id                 BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    real_name               VARCHAR(64) NOT NULL,
    avatar                  VARCHAR(500),
    phone                   VARCHAR(20),
    title                   VARCHAR(64),
    introduction            TEXT,
    specialty_tags          JSONB,
    service_region          JSONB,
    service_price           NUMERIC(10, 2),
    level                   VARCHAR(16) NOT NULL DEFAULT 'junior',
    level_expire_at         DATE,
    certification_no        VARCHAR(64),
    merchant_id             BIGINT REFERENCES planner_merchants(id),
    merchant_name           VARCHAR(128),
    total_service_count     INTEGER NOT NULL DEFAULT 0,
    rating_avg              NUMERIC(2, 1) DEFAULT 5.0,
    status                  VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_planner_profiles_user     ON planner_profiles(user_id);
CREATE INDEX idx_planner_profiles_level    ON planner_profiles(level);
CREATE INDEX idx_planner_profiles_status   ON planner_profiles(status);
CREATE INDEX idx_planner_profiles_merchant ON planner_profiles(merchant_id);
