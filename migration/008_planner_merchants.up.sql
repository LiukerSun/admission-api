CREATE TABLE IF NOT EXISTS planner_merchants (
    id                    BIGSERIAL PRIMARY KEY,
    merchant_name         VARCHAR(128) NOT NULL,
    contact_person        VARCHAR(64),
    contact_phone         VARCHAR(20),
    address               VARCHAR(255),
    logo                  VARCHAR(500),
    banner                VARCHAR(500),
    description           TEXT,
    sort_order            INTEGER NOT NULL DEFAULT 0,
    owner_id              BIGINT,
    service_regions       JSONB,
    default_service_price NUMERIC(10, 2),
    status                VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(merchant_name)
);

CREATE INDEX idx_planner_merchants_status ON planner_merchants(status);
