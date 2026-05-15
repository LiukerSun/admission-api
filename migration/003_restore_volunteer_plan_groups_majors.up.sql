-- Restore PR #25 "志愿方案导出" schema that the baseline accidentally dropped.
--
-- internal/admission/volunteer_plan_service.go reads:
--   user_volunteer_plans.name, user_volunteer_plans.description
--   user_volunteer_groups  (plan_id -> plan, ordered group choices)
--   user_volunteer_majors  (group_id -> the 6 majors per group)
--
-- The baseline migration kept only the JSONB form (title / plan_json) used by
-- internal/volunteerplan/, which broke GET /api/v1/admission/volunteer-plans
-- at runtime. This migration adds the missing columns and tables back without
-- touching anything the JSONB code uses, so both modules coexist on the same
-- user_volunteer_plans row.

ALTER TABLE user_volunteer_plans
    ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS user_volunteer_groups (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES user_volunteer_plans(id) ON DELETE CASCADE,
    order_no INTEGER NOT NULL,
    university_id BIGINT REFERENCES universities(id) ON DELETE SET NULL,
    university_code TEXT NOT NULL,
    university_name TEXT NOT NULL,
    group_id BIGINT REFERENCES admission_groups(id) ON DELETE SET NULL,
    group_code TEXT NOT NULL,
    group_name TEXT NOT NULL DEFAULT '',
    is_obey_adjustment BOOLEAN DEFAULT TRUE,
    remark TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_volunteer_majors (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES user_volunteer_groups(id) ON DELETE CASCADE,
    major_admission_id BIGINT REFERENCES university_major_admissions(id) ON DELETE SET NULL,
    major_order INTEGER NOT NULL,
    major_code TEXT NOT NULL DEFAULT '',
    major_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (group_id, major_order)
);

CREATE INDEX IF NOT EXISTS idx_user_volunteer_groups_plan_id
    ON user_volunteer_groups(plan_id);
CREATE INDEX IF NOT EXISTS idx_user_volunteer_majors_group_id
    ON user_volunteer_majors(group_id);
