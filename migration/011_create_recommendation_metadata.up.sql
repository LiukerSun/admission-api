-- ============================================================================
-- 008: Recommendation algorithm metadata.
--
-- Adds the lookup data the volunteer recommendation algorithm relies on,
-- replacing previously hard-coded Go constants:
--   * university_profiles.university_tier        (清北 / 华5 / C9 / 985 / 211 / ...)
--   * city_groups + city_group_members           (京津冀 / 长三角 / 粤港澳 / 成渝 / 关中 / 东北)
--   * recommendation_family_resource_keywords    家庭资源 -> 专业关键词指向
--   * recommendation_holland_keywords            RIASEC -> 学科关键词
--   * recommendation_major_ability_rules         CHSI 大类 -> 单科分数门槛
--
-- Seeds the lookup tables and back-fills tier values for known top universities.
-- Member rows for university_profiles already exist; rows for unknown schools
-- are left NULL so the service falls back to is_985 / is_211 / is_double_first_class.
-- ============================================================================

ALTER TABLE university_profiles
    ADD COLUMN IF NOT EXISTS university_tier VARCHAR(32);

CREATE INDEX IF NOT EXISTS idx_university_profiles_tier
    ON university_profiles(university_tier);

CREATE TABLE IF NOT EXISTS city_groups (
    code VARCHAR(32) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS city_group_members (
    id BIGSERIAL PRIMARY KEY,
    city_group_code VARCHAR(32) NOT NULL REFERENCES city_groups(code) ON DELETE CASCADE,
    city VARCHAR(100) NOT NULL,
    UNIQUE (city_group_code, city)
);

CREATE INDEX IF NOT EXISTS idx_city_group_members_city
    ON city_group_members(city);

CREATE TABLE IF NOT EXISTS recommendation_family_resource_keywords (
    id BIGSERIAL PRIMARY KEY,
    resource_code VARCHAR(50) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (resource_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_family_resource_keywords_resource
    ON recommendation_family_resource_keywords(resource_code);

CREATE TABLE IF NOT EXISTS recommendation_holland_keywords (
    id BIGSERIAL PRIMARY KEY,
    riasec_code CHAR(1) NOT NULL,
    keyword VARCHAR(100) NOT NULL,
    weight NUMERIC(4,2) NOT NULL DEFAULT 1.00,
    UNIQUE (riasec_code, keyword)
);

CREATE INDEX IF NOT EXISTS idx_holland_keywords_code
    ON recommendation_holland_keywords(riasec_code);

CREATE TABLE IF NOT EXISTS recommendation_major_ability_rules (
    id BIGSERIAL PRIMARY KEY,
    chsi_category_code VARCHAR(10) NOT NULL,
    subject VARCHAR(20) NOT NULL,           -- 'physics' | 'math' | 'chinese' | 'english'
    exclude_below_score INTEGER NOT NULL,   -- 低于此分数 → 直接淘汰
    warn_below_score INTEGER NOT NULL,      -- 低于此分数 → 仅告警，不淘汰 (must be >= exclude_below_score)
    note VARCHAR(255),
    UNIQUE (chsi_category_code, subject)
);

CREATE INDEX IF NOT EXISTS idx_major_ability_rules_category
    ON recommendation_major_ability_rules(chsi_category_code);

-- ----------------------------------------------------------------------------
-- Seed: city groups + members
-- ----------------------------------------------------------------------------
INSERT INTO city_groups (code, name, sort_order) VALUES
    ('jjj', '京津冀城市群', 10),
    ('yrd', '长三角城市群', 20),
    ('gba', '粤港澳大湾区', 30),
    ('cd',  '成渝城市群',   40),
    ('gz',  '关中平原城市群', 50),
    ('ne',  '东北城市群',   60)
ON CONFLICT (code) DO NOTHING;

INSERT INTO city_group_members (city_group_code, city) VALUES
    ('jjj', '北京'),  ('jjj', '天津'),  ('jjj', '雄安'),  ('jjj', '石家庄'), ('jjj', '保定'),
    ('yrd', '上海'),  ('yrd', '南京'),  ('yrd', '杭州'),  ('yrd', '苏州'),  ('yrd', '无锡'),
    ('yrd', '合肥'),  ('yrd', '宁波'),  ('yrd', '常州'),  ('yrd', '南通'),  ('yrd', '嘉兴'),
    ('yrd', '绍兴'),  ('yrd', '镇江'),  ('yrd', '扬州'),  ('yrd', '芜湖'),  ('yrd', '马鞍山'),
    ('gba', '广州'),  ('gba', '深圳'),  ('gba', '珠海'),  ('gba', '香港'),  ('gba', '澳门'),
    ('gba', '佛山'),  ('gba', '东莞'),  ('gba', '中山'),  ('gba', '惠州'),  ('gba', '江门'),
    ('gba', '肇庆'),
    ('cd',  '重庆'),  ('cd',  '成都'),  ('cd',  '绵阳'),  ('cd',  '德阳'),
    ('gz',  '西安'),  ('gz',  '咸阳'),  ('gz',  '宝鸡'),  ('gz',  '渭南'),  ('gz',  '铜川'),
    ('ne',  '哈尔滨'), ('ne',  '长春'),  ('ne',  '沈阳'),  ('ne',  '大连'),  ('ne',  '吉林')
ON CONFLICT (city_group_code, city) DO NOTHING;

-- ----------------------------------------------------------------------------
-- Seed: family-resource → major keyword指向
-- ----------------------------------------------------------------------------
INSERT INTO recommendation_family_resource_keywords (resource_code, keyword, weight) VALUES
    ('公检法', '法学',     1.40),
    ('公检法', '法律',     1.30),
    ('金融',   '金融',     1.30),
    ('金融',   '经济',     1.20),
    ('金融',   '财政',     1.10),
    ('医疗',   '临床',     1.40),
    ('医疗',   '药学',     1.20),
    ('医疗',   '医学',     1.20),
    ('教育',   '教育',     1.30),
    ('教育',   '师范',     1.30),
    ('电网',   '电气',     1.40),
    ('电网',   '能源动力', 1.20),
    ('电网',   '能动',     1.20),
    ('商业',   '工商管理', 1.20),
    ('商业',   '会计',     1.30),
    ('商业',   '财务',     1.20),
    ('从医',   '医学',     1.30)
ON CONFLICT (resource_code, keyword) DO NOTHING;

-- ----------------------------------------------------------------------------
-- Seed: 霍兰德 RIASEC → 学科关键词
-- ----------------------------------------------------------------------------
INSERT INTO recommendation_holland_keywords (riasec_code, keyword, weight) VALUES
    ('R', '工学',   1.20),
    ('R', '农学',   1.10),
    ('I', '理学',   1.20),
    ('I', '医学',   1.10),
    ('A', '艺术',   1.30),
    ('A', '文学',   1.10),
    ('S', '教育',   1.20),
    ('S', '心理',   1.20),
    ('S', '社会工作', 1.10),
    ('E', '管理',   1.20),
    ('E', '金融',   1.20),
    ('E', '经济',   1.10),
    ('C', '会计',   1.20),
    ('C', '财务',   1.10),
    ('C', '档案',   1.10)
ON CONFLICT (riasec_code, keyword) DO NOTHING;

-- ----------------------------------------------------------------------------
-- Seed: CHSI 大类 → 单科分数门槛
--   subject = 'physics' / 'math'
--   physics < 40 → exclude (电子/电气/机械/航空/土木)
--   math    < 50 → exclude (计算机/统计/数学/金融)
--   warn_below_score 用于产生 "学得吃力" 的 warning（默认 = exclude + 10）
-- ----------------------------------------------------------------------------
INSERT INTO recommendation_major_ability_rules
    (chsi_category_code, subject, exclude_below_score, warn_below_score, note) VALUES
    ('0807', 'physics', 40, 50, '电子信息类大学物理强度高'),
    ('0808', 'physics', 40, 50, '自动化类大学物理强度高'),
    ('0802', 'physics', 40, 50, '机械类大学物理强度高'),
    ('0826', 'physics', 40, 50, '航空航天类大学物理强度高'),
    ('0810', 'physics', 40, 50, '土木类大学物理强度高'),
    ('0811', 'physics', 40, 50, '水利类大学物理强度高'),
    ('0809', 'math', 50, 70, '计算机类大学数学强度高'),
    ('0712', 'math', 50, 70, '统计学类对数学要求高'),
    ('0701', 'math', 50, 70, '数学类专业对数学要求最高'),
    ('0203', 'math', 50, 70, '金融工程对数学要求高'),
    ('0807', 'math', 30, 60, '电子信息类大学数学也常用'),
    ('0808', 'math', 30, 60, '自动化类大学数学也常用')
ON CONFLICT (chsi_category_code, subject) DO NOTHING;

-- ----------------------------------------------------------------------------
-- Backfill: university_tier (latest profile_year only)
--   Match by university name. Unknown schools stay NULL → service falls back
--   to is_985 / is_211 / is_double_first_class flags.
-- ----------------------------------------------------------------------------
WITH latest AS (
    SELECT DISTINCT ON (up.university_id) up.id, u.name
    FROM university_profiles up
    JOIN universities u ON u.id = up.university_id
    ORDER BY up.university_id, up.profile_year DESC
)
UPDATE university_profiles up
SET university_tier = CASE l.name
    WHEN '清华大学' THEN 'top_2'
    WHEN '北京大学' THEN 'top_2'
    WHEN '复旦大学' THEN 'hua_5'
    WHEN '上海交通大学' THEN 'hua_5'
    WHEN '浙江大学' THEN 'hua_5'
    WHEN '中国科学技术大学' THEN 'hua_5'
    WHEN '南京大学' THEN 'hua_5'
    WHEN '西安交通大学' THEN 'c9'
    WHEN '哈尔滨工业大学' THEN 'c9'
END
FROM latest l
WHERE up.id = l.id
  AND l.name IN (
    '清华大学','北京大学',
    '复旦大学','上海交通大学','浙江大学','中国科学技术大学','南京大学',
    '西安交通大学','哈尔滨工业大学'
  );
