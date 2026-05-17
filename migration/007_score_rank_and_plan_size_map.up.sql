-- ============================================================================
-- 007_score_rank_and_plan_size_map
--
-- 把"位次"和"目标志愿数"从用户必填字段改为系统查表/换算的支撑结构：
--
--   1. region_plan_size_map      —— (年, 省, 物理/历史) → 一套志愿可填的最大条数
--   2. score_rank_map            —— (年, 省, 物理/历史, 分数) → 累计位次
--   3. score_rank_map_staging    —— CSV 导入暂存区，校验通过后再 swap 进正表
--   4. user_profiles.elective_subjects —— 再选科目（4 选 2: biology / chemistry
--                                         / geography / politics）
--
-- 设计要点见 PRD-Profile-Survey-Simplification.md §4。
--
-- score_rank_map 不按再选科目分桶（黑龙江一分一段表只按 物理类 / 历史类 总分发布）。
-- region_plan_size_map 保留 subject_category 维度，初期物理/历史可写同值；将来
-- 政策分化时不需要再 migration。
-- ============================================================================

-- ---------------------------------------------------------------------------
-- region_plan_size_map：每年每省每类的志愿条数上限
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS region_plan_size_map (
    id BIGSERIAL PRIMARY KEY,
    year INTEGER NOT NULL CHECK (year BETWEEN 2020 AND 2100),
    region_code VARCHAR(20) NOT NULL REFERENCES regions(code),
    subject_category_code VARCHAR(50) NOT NULL REFERENCES subject_categories(code),
    plan_size INTEGER NOT NULL CHECK (plan_size BETWEEN 1 AND 96),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (year, region_code, subject_category_code)
);

CREATE INDEX IF NOT EXISTS idx_region_plan_size_lookup
    ON region_plan_size_map(year, region_code, subject_category_code);

-- ---------------------------------------------------------------------------
-- score_rank_map：一分一段表正表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS score_rank_map (
    id BIGSERIAL PRIMARY KEY,
    year INTEGER NOT NULL CHECK (year BETWEEN 2020 AND 2100),
    region_code VARCHAR(20) NOT NULL REFERENCES regions(code),
    subject_category_code VARCHAR(50) NOT NULL REFERENCES subject_categories(code),
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 750),
    cumulative_rank INTEGER NOT NULL CHECK (cumulative_rank >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (year, region_code, subject_category_code, score)
);

-- LookupRank 走 score DESC 范围查询，需要这个 index。
CREATE INDEX IF NOT EXISTS idx_score_rank_lookup
    ON score_rank_map(year, region_code, subject_category_code, score DESC);

-- ---------------------------------------------------------------------------
-- score_rank_map_staging：CSV 导入暂存区
--
-- admin 上传新 CSV 时先全量写入 staging，校验（行数、累计单调、score 范围）通过
-- 后，由 admin handler 在事务里把 (year, region, subject) 切片从 staging 移到
-- score_rank_map，旧切片标记为 superseded 保留一版用于回滚。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS score_rank_map_staging (
    id BIGSERIAL PRIMARY KEY,
    import_id UUID NOT NULL,  -- 一次上传一个 import_id，方便整批校验和回滚
    year INTEGER NOT NULL CHECK (year BETWEEN 2020 AND 2100),
    region_code VARCHAR(20) NOT NULL,
    subject_category_code VARCHAR(50) NOT NULL,
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 750),
    cumulative_rank INTEGER NOT NULL CHECK (cumulative_rank >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_score_rank_staging_import
    ON score_rank_map_staging(import_id);

-- ---------------------------------------------------------------------------
-- user_profiles.elective_subjects：再选科目（4 选 2）
--
-- 字典：biology / chemistry / geography / politics。
-- CHECK 约束：
--   1. 允许 NULL（旧用户兼容）
--   2. 长度恰好为 2
--   3. 元素必须在枚举内
--
-- 注：归一化排序（保证 [biology,chemistry] 和 [chemistry,biology] 视为同值）由
-- service 层在 upsert 前完成；Postgres CHECK 不允许 subquery，不在这里做。
-- ---------------------------------------------------------------------------
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS elective_subjects TEXT[];

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_elective_subjects_check
    CHECK (
        elective_subjects IS NULL
        OR (
            array_length(elective_subjects, 1) = 2
            AND elective_subjects <@ ARRAY['biology','chemistry','geography','politics']::TEXT[]
        )
    );
