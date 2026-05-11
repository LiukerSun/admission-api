-- ============================================================================
-- 009: Recommendation algorithm — precomputed five-dimension scores.
--
-- Stores the five base scores keyed by (university_id, local_major_code):
--   city_score / school_score / major_score
--   ability_improvement_score      (LLM 评估：该专业对学生综合能力提升)
--   future_competitiveness_score   (LLM 评估：该专业未来竞争力)
--
-- Plus per-dimension reasons so the frontend can show "why this score".
--
-- The recommendation algorithm reads these as the BASE scores and multiplies
-- runtime personalization (preferred cities/majors, family resources, holland,
-- student's single-subject scores). Rows missing → algorithm falls back to its
-- old formula, so this table can be partially populated.
--
-- Refresh strategy: a CLI / admin endpoint iterates rows where
--   evaluated_at IS NULL OR evaluated_at < NOW() - INTERVAL '90 days'
-- and calls an LLM (or operator) to refill them.
-- ============================================================================

CREATE TABLE IF NOT EXISTS recommendation_precomputed_scores (
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

    evaluated_by    VARCHAR(32)  NOT NULL DEFAULT 'algorithm', -- algorithm | llm | manual
    evaluator_model VARCHAR(120),                              -- e.g. claude-opus-4-7
    evaluated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (university_id, local_major_code)
);

-- Lookup index: hit during recommendation queries (joined via uma.local_major_code).
CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_lookup
    ON recommendation_precomputed_scores(university_id, local_major_code);

-- Refresh queue index: "give me the oldest / unevaluated rows first".
CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_evaluated_at
    ON recommendation_precomputed_scores(evaluated_at);

-- Filter index: "rows evaluated by the LLM" / "manual overrides".
CREATE INDEX IF NOT EXISTS idx_recommendation_precomputed_scores_evaluated_by
    ON recommendation_precomputed_scores(evaluated_by);
