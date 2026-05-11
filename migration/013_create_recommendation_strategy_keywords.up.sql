-- ============================================================================
-- 010: Recommendation strategy keyword seeds.
--
-- Powers admission.decideStrategy: when the user does not explicitly choose
-- `school` or `major`, the algorithm peeks at PreferredMajors and decides
-- whether STEM intent (→ major-first) or humanities intent (→ school-first)
-- dominates. Until now those keyword lists were hard-coded in Go; this table
-- lets ops adjust them without a release.
--
-- intent_code: 'stem' | 'humanities'
-- keyword:     substring matched against the user's PreferredMajors entries
-- ============================================================================

CREATE TABLE IF NOT EXISTS recommendation_strategy_keywords (
    id BIGSERIAL PRIMARY KEY,
    intent_code VARCHAR(32) NOT NULL,            -- 'stem' | 'humanities'
    keyword     VARCHAR(100) NOT NULL,
    UNIQUE (intent_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_strategy_keywords_intent
    ON recommendation_strategy_keywords(intent_code);

-- Seed: mirrors the previously-hardcoded lists in recommendation_service.go.
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
