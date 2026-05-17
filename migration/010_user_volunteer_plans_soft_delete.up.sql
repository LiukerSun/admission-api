-- ============================================================================
-- 010_user_volunteer_plans_soft_delete
--
-- 给 user_volunteer_plans 加软删除支持。DELETE 不真删行，只 set deleted_at=NOW()。
-- 所有 SELECT 加 WHERE deleted_at IS NULL 过滤。
--
-- 设计要点：
--   * 用 partial index 让"活跃方案"查询保持 O(log n)，不被已删数据拖慢。
--   * 不破坏现有 UNIQUE(user_id, source_draft_id) ：如果用户重复 adopt 同一个
--     draft，软删后的旧方案仍占据这个 unique slot，必须先硬清理或换语义。
--     当前 adopt 流是幂等的（ON CONFLICT DO NOTHING + GetByDraftID 回查），
--     软删一个方案后该 user+draft 不会再产生新方案——这是可接受的行为，
--     用户想"恢复"就是把 deleted_at 置 NULL。
-- ============================================================================

ALTER TABLE user_volunteer_plans
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 活跃方案的 user 维度索引：替代原 idx_user_volunteer_plans_user_created。
-- partial index 显式带 deleted_at IS NULL，确保所有 ListByUser 查询走它。
CREATE INDEX IF NOT EXISTS idx_user_volunteer_plans_alive
    ON user_volunteer_plans(user_id, created_at DESC)
    WHERE deleted_at IS NULL;
