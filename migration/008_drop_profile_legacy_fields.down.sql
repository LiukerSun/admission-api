-- 回滚 008：仅恢复 schema 形状，CHECK 约束与 006 baseline 保持一致。
-- 注意：数据无法恢复 —— up 是不可逆操作。
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS provincial_rank INTEGER
        CHECK (provincial_rank IS NULL OR (provincial_rank >= 0 AND provincial_rank <= 500000)),
    ADD COLUMN IF NOT EXISTS plan_size INTEGER
        CHECK (plan_size IS NULL OR (plan_size >= 1 AND plan_size <= 96)),
    ADD COLUMN IF NOT EXISTS priority_strategy VARCHAR(16)
        CHECK (priority_strategy IS NULL OR priority_strategy IN ('auto', 'school', 'major')),
    ADD COLUMN IF NOT EXISTS math_score INTEGER
        CHECK (math_score IS NULL OR (math_score >= 0 AND math_score <= 150)),
    ADD COLUMN IF NOT EXISTS physics_score INTEGER
        CHECK (physics_score IS NULL OR (physics_score >= 0 AND physics_score <= 150)),
    ADD COLUMN IF NOT EXISTS chinese_score INTEGER
        CHECK (chinese_score IS NULL OR (chinese_score >= 0 AND chinese_score <= 150)),
    ADD COLUMN IF NOT EXISTS english_score INTEGER
        CHECK (english_score IS NULL OR (english_score >= 0 AND english_score <= 150)),
    ADD COLUMN IF NOT EXISTS preferences JSONB NOT NULL DEFAULT '{}'::jsonb;
