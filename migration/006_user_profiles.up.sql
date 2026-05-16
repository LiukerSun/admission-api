-- ============================================================================
-- 006_user_profiles: persistent user questionnaire / basic-info profile.
--
-- Stores the data the AI agent needs as its "recommendation_request" pre-fill
-- so the user does not have to re-enter region / subject / score / rank / etc.
-- in every new chat. One row per user (user_id is PK + FK).
--
-- Design notes:
--   * region_code stays generic (no DEFAULT '230000') — the data model must
--     accommodate future provinces. The AI system prompt (agent.go) currently
--     restricts to 黑龙江=230000 as a business rule, not a schema rule.
--   * Required scalars (subject_category_code, total_score, provincial_rank,
--     plan_size, priority_strategy) are typed columns so the service layer
--     can validate them individually.
--   * Optional array / free-text preferences live in a single JSONB column to
--     keep the schema flexible as new optional fields are added to agent.go.
--   * completed_at is set by the service layer when all 4 required scalars
--     are present; the partial index lets dashboards query "completed" users
--     cheaply.
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,

    -- Required scalars (NULL allowed at DB layer; service enforces business rules)
    region_code VARCHAR(10),
    subject_category_code VARCHAR(20)
        CHECK (subject_category_code IS NULL OR subject_category_code IN ('physics', 'history')),
    total_score INTEGER
        CHECK (total_score IS NULL OR (total_score >= 0 AND total_score <= 750)),
    provincial_rank INTEGER
        CHECK (provincial_rank IS NULL OR (provincial_rank >= 0 AND provincial_rank <= 500000)),
    plan_size INTEGER
        CHECK (plan_size IS NULL OR (plan_size >= 1 AND plan_size <= 96)),
    priority_strategy VARCHAR(16)
        CHECK (priority_strategy IS NULL OR priority_strategy IN ('auto', 'school', 'major')),

    -- Single-subject scores (optional, typed)
    math_score INTEGER
        CHECK (math_score IS NULL OR (math_score >= 0 AND math_score <= 150)),
    physics_score INTEGER
        CHECK (physics_score IS NULL OR (physics_score >= 0 AND physics_score <= 150)),
    chinese_score INTEGER
        CHECK (chinese_score IS NULL OR (chinese_score >= 0 AND chinese_score <= 150)),
    english_score INTEGER
        CHECK (english_score IS NULL OR (english_score >= 0 AND english_score <= 150)),

    -- Flexible bag for the remaining 14 optional preference fields. Shape:
    --   {
    --     "required_majors":      string[],   // hard whitelist
    --     "preferred_majors":     string[],   // soft sort
    --     "excluded_majors":      string[],   // hard exclude
    --     "excluded_keywords":    string[],
    --     "preferred_cities":     string[],
    --     "excluded_cities":      string[],
    --     "preferred_provinces":  string[],
    --     "excluded_provinces":   string[],
    --     "holland_code":         string,     // RIASEC subset, e.g. "RIA"
    --     "family_resources":     string,     // free text ≤500 chars
    --     "family_economy":       string,     // free text ≤500 chars
    --     "career_plans":         string,     // free text ≤500 chars
    --     "budget_tuition_max":   number      // CNY / year
    --   }
    preferences JSONB NOT NULL DEFAULT '{}'::jsonb,

    completed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial index: lets analytics queries scan only completed profiles without
-- bloating the index with rows that are still being filled.
CREATE INDEX IF NOT EXISTS idx_user_profiles_completed
    ON user_profiles(completed_at)
    WHERE completed_at IS NOT NULL;
