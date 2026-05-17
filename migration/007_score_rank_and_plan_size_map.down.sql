-- 回滚 007：按反向 FK 顺序删表，最后删 user_profiles 列。
ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_elective_subjects_check;

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS elective_subjects;

DROP INDEX IF EXISTS idx_score_rank_staging_import;
DROP TABLE IF EXISTS score_rank_map_staging;

DROP INDEX IF EXISTS idx_score_rank_lookup;
DROP TABLE IF EXISTS score_rank_map;

DROP INDEX IF EXISTS idx_region_plan_size_lookup;
DROP TABLE IF EXISTS region_plan_size_map;
