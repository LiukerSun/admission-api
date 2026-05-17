-- ============================================================================
-- 008_drop_profile_legacy_fields
--
-- 删除 user_profiles 上的 8 个非核心字段。问卷瘦身（PR2）之后，前端只收集 4 项
-- 核心信息（region/subject/electives/total_score）；其余偏好交给 AI 在对话中
-- 现问。这些字段在 DB 上永远保持 NULL —— 留着只会误导后来者。
--
-- 删除的字段：
--   provincial_rank      —— lookup 服务从 total_score 实时换算
--   plan_size            —— lookup 服务从 region_plan_size_map 查表
--   priority_strategy    —— LLM 在对话中现问，每次对话默认 auto
--   math/physics/chinese/english_score —— 同上，LLM 现问
--   preferences (JSONB)  —— 同上，LLM 通过 tool 直接传给推荐算法
--
-- 不可逆：down.sql 仅恢复 schema，已有数据丢失。production 跑前确认 DB 中
-- 这些列没有有价值的数据。
-- ============================================================================

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS provincial_rank,
    DROP COLUMN IF EXISTS plan_size,
    DROP COLUMN IF EXISTS priority_strategy,
    DROP COLUMN IF EXISTS math_score,
    DROP COLUMN IF EXISTS physics_score,
    DROP COLUMN IF EXISTS chinese_score,
    DROP COLUMN IF EXISTS english_score,
    DROP COLUMN IF EXISTS preferences;
