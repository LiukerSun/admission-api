-- 志愿方案主表
CREATE TABLE IF NOT EXISTS user_volunteer_plans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_volunteer_plans IS '用户填报的志愿方案主表';
COMMENT ON COLUMN user_volunteer_plans.id IS '方案唯一标识';
COMMENT ON COLUMN user_volunteer_plans.user_id IS '所属用户ID';
COMMENT ON COLUMN user_volunteer_plans.name IS '方案名称';
COMMENT ON COLUMN user_volunteer_plans.description IS '方案描述/备注';

-- 志愿方案中的院校专业组（志愿项）
CREATE TABLE IF NOT EXISTS user_volunteer_groups (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES user_volunteer_plans(id) ON DELETE CASCADE,
    order_no INTEGER NOT NULL, -- 志愿顺序
    university_id BIGINT REFERENCES universities(id) ON DELETE SET NULL, -- 关联基础院校表
    university_code TEXT NOT NULL,
    university_name TEXT NOT NULL,
    group_id BIGINT REFERENCES admission_groups(id) ON DELETE SET NULL, -- 关联基础专业组表
    group_code TEXT NOT NULL,
    group_name TEXT NOT NULL DEFAULT '',
    is_obey_adjustment BOOLEAN DEFAULT TRUE, -- 是否服从调剂
    remark TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_volunteer_groups IS '方案中的院校专业组志愿项';
COMMENT ON COLUMN user_volunteer_groups.id IS '专业组志愿项唯一标识';
COMMENT ON COLUMN user_volunteer_groups.plan_id IS '所属方案ID';
COMMENT ON COLUMN user_volunteer_groups.order_no IS '专业组志愿顺序 (1, 2, 3...)';
COMMENT ON COLUMN user_volunteer_groups.university_id IS '关联的院校基础数据ID';
COMMENT ON COLUMN user_volunteer_groups.university_code IS '院校代号';
COMMENT ON COLUMN user_volunteer_groups.university_name IS '院校名称';
COMMENT ON COLUMN user_volunteer_groups.group_id IS '关联的专业组基础数据ID';
COMMENT ON COLUMN user_volunteer_groups.group_code IS '专业组代号';
COMMENT ON COLUMN user_volunteer_groups.group_name IS '专业组名称';
COMMENT ON COLUMN user_volunteer_groups.is_obey_adjustment IS '是否服从专业调剂';
COMMENT ON COLUMN user_volunteer_groups.remark IS '针对该专业组志愿项的备注';

-- 志愿项下的专业志愿
CREATE TABLE IF NOT EXISTS user_volunteer_majors (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES user_volunteer_groups(id) ON DELETE CASCADE,
    major_admission_id BIGINT REFERENCES university_major_admissions(id) ON DELETE SET NULL, -- 关联基础专业录取表
    major_order INTEGER NOT NULL, -- 专业志愿顺序 (1-6)
    major_code TEXT NOT NULL DEFAULT '',
    major_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(group_id, major_order)
);

COMMENT ON TABLE user_volunteer_majors IS '专业志愿项';
COMMENT ON COLUMN user_volunteer_majors.id IS '专业志愿项唯一标识';
COMMENT ON COLUMN user_volunteer_majors.group_id IS '所属院校专业组ID';
COMMENT ON COLUMN user_volunteer_majors.major_order IS '专业志愿顺序 (1-6)';
COMMENT ON COLUMN user_volunteer_majors.major_admission_id IS '关联的专业录取基础数据ID';
COMMENT ON COLUMN user_volunteer_majors.major_code IS '专业代码';
COMMENT ON COLUMN user_volunteer_majors.major_name IS '专业名称';

CREATE INDEX IF NOT EXISTS idx_user_volunteer_plans_user_id ON user_volunteer_plans(user_id);
CREATE INDEX IF NOT EXISTS idx_user_volunteer_groups_plan_id ON user_volunteer_groups(plan_id);
CREATE INDEX IF NOT EXISTS idx_user_volunteer_majors_group_id ON user_volunteer_majors(group_id);
