# PRD: 高考志愿调查问卷简化 & 选科结构升级

**状态**: Draft, 待 review
**日期**: 2026-05-16
**作者**: AI 协作起草

## 1. 目标

把"高考志愿调查问卷"瘦身到**最少必要字段**，把目前要求用户手填、其实可以从历史数据查出来的字段（位次、目标志愿数）改造成系统自动换算/查表。同时让选科结构从"二选一"升级到"二选一 + 四选二"，并把副选科接入 AI 推荐链路。

## 2. 现状（research findings）

### 2.1 现在的问卷必填项

| 字段 | 来源 | 现实意义 |
| --- | --- | --- |
| `region_code` 省份 | 用户选 | 候选池过滤 |
| `subject_category_code` 物理/历史 | 用户选 | 候选池过滤 + AI prompt |
| `total_score` 总分 | 用户填 | **仅 validate 用**，不参与排序 |
| `provincial_rank` 省内位次 | 用户填 | **算法主轴**，冲/稳/保窗口靠它 |
| `plan_size` 目标志愿数 | 用户填（默认 40） | **算法真在用**，拆分 quota + 漏斗阈值 |

### 2.2 关键耦合点

- `recommendation_service.go::computeBucketWindows(req.ProvincialRank)` — 位次算窗口
- `recommendation_service.go::splitTierQuota(planSize)` — plan_size 拆 1:2:1
- AI agent 通过 message 里的 `recommendation_request/snapshot` JSON 块接收上下文，**不**读 `user_profiles` 表
- `subject_category_code` 枚举硬编码 6 处：profile model 常量 / service switch / migration CHECK / recommendation 字面量 / tools.go enum / agent prompt

## 3. 用户层改造

### 3.1 问卷必填项（重写）

| 字段 | 状态 | 说明 |
| --- | --- | --- |
| 所在省份 | 必填 | 不变 |
| 首选科目（物理/历史） | 必填 | 不变 |
| 再选科目（4 选 2） | **必填，新增** | 生物/化学/地理/政治 任选 2 |
| 总分 | 必填 | 不变 |
| ~~省内位次~~ | **移除** | 由 score_rank_map 自动换算 |
| ~~目标志愿数~~ | **移除** | 由 region_plan_size_map 自动查表 |

### 3.2 单科成绩、专业偏好、地域背景

折叠区保持可选，本次不动。

## 4. 数据模型变更

### 4.1 新表 A：`region_plan_size_map`（地区志愿数映射）

```sql
CREATE TABLE region_plan_size_map (
    id BIGSERIAL PRIMARY KEY,
    year INTEGER NOT NULL,
    region_code VARCHAR(10) NOT NULL,
    subject_category_code VARCHAR(20) NOT NULL,  -- physics / history
    plan_size INTEGER NOT NULL CHECK (plan_size BETWEEN 1 AND 96),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (year, region_code, subject_category_code)
);
```

**为什么分 subject_category**：黑龙江新高考物理/历史可能填报上限不同；保留维度，初期可两条写一样的值。

### 4.2 新表 B：`score_rank_map`（一分一段表）

```sql
CREATE TABLE score_rank_map (
    id BIGSERIAL PRIMARY KEY,
    year INTEGER NOT NULL,
    region_code VARCHAR(10) NOT NULL,
    subject_category_code VARCHAR(20) NOT NULL,  -- physics / history
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 750),
    cumulative_rank INTEGER NOT NULL CHECK (cumulative_rank >= 0),  -- 累计位次（≥该分数人数）
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (year, region_code, subject_category_code, score)
);

CREATE INDEX idx_score_rank_lookup
    ON score_rank_map(year, region_code, subject_category_code, score DESC);
```

**注意**：再选科目（生化地政）**不**进入位次表，因为黑龙江新高考位次以"物理/历史 + 总分"为粒度发布。

**换算规则**：查 (year, region_code, subject_category_code, score=user_score)，没精确命中则查 `score <= user_score` 的最大值（向下取整 = 实际位次的保守上界）。

### 4.3 修改 `user_profiles`

```sql
-- 移除字段（也可保留为 deprecated，初期只在前端隐藏）
-- 推荐：保留列、停止写入；下一个迭代再 drop。
-- ALTER TABLE user_profiles DROP COLUMN provincial_rank;
-- ALTER TABLE user_profiles DROP COLUMN plan_size;

-- 新增：再选科目（4 选 2）
ALTER TABLE user_profiles
    ADD COLUMN elective_subjects TEXT[]
    CHECK (
        elective_subjects IS NULL
        OR (
            array_length(elective_subjects, 1) = 2
            AND elective_subjects <@ ARRAY['biology','chemistry','geography','politics']::TEXT[]
        )
    );
```

**字典约定**：`biology, chemistry, geography, politics`，前端 label 映射放 `profileLabels.ts`。

**兼容策略**：旧字段（`provincial_rank`, `plan_size`）保留 1 个迭代周期。Service 层 upsert 时停止接受这两个值；前端表单移除输入。一个迭代后再起 migration drop 列。

## 5. 后端服务层

### 5.1 新增 service：`lookup` 包

```go
// internal/lookup/service.go
type Service struct { db *sqlx.DB }

// 给定 (year, region, subject, score) → cumulative_rank
// 没精确匹配时取 score <= user_score 的最大值
func (s *Service) LookupRank(ctx, year, regionCode, subjectCode string, score int) (rank int, err error)

// 给定 (year, region, subject) → plan_size
// 缺失时回退到 DefaultPlanSize=40
func (s *Service) LookupPlanSize(ctx, year, regionCode, subjectCode string) (size int, err error)
```

### 5.2 profile → snapshot 适配层（**关键改动**）

新增 `userprofile.Service.BuildRecommendationSnapshot(userID)` 方法：

1. 读 `user_profiles`
2. 调 `lookup.LookupRank(currentYear, region, subject, total_score)` → provincial_rank
3. 调 `lookup.LookupPlanSize(currentYear, region, subject)` → plan_size
4. 返回完整的 `RecommendationSnapshot`（兼容现有 agent 接收格式）

**前端**改为调用这个 endpoint 拿 snapshot，而不是自己拼。这样：
- 算法对位次/plan_size 的依赖不动（不改 recommendation_service.go）
- 用户感知层简化
- AI agent prompt 几乎不动

### 5.3 admin CRUD（套 membership 模板）

新增两个 admin handler，照抄 `membership/admin_handler.go` 五件套：

```
POST   /api/v1/admin/lookup/plan-sizes      → AdminCreatePlanSize
GET    /api/v1/admin/lookup/plan-sizes      → AdminListPlanSizes
GET    /api/v1/admin/lookup/plan-sizes/:id  → AdminGetPlanSize
PUT    /api/v1/admin/lookup/plan-sizes/:id  → AdminUpdatePlanSize
DELETE /api/v1/admin/lookup/plan-sizes/:id  → AdminDeletePlanSize

POST   /api/v1/admin/lookup/score-ranks/import  → 批量 CSV 导入
GET    /api/v1/admin/lookup/score-ranks         → 分页 List
DELETE /api/v1/admin/lookup/score-ranks/:id     → 删单条
```

**score_rank_map 走批量导入**：单条 CRUD 没意义（一分一段表一年几百条）。提供 CSV 上传接口，事务内 truncate-and-insert 某个 (year, region, subject) 切片。

## 6. AI 推荐链路改造

### 6.1 副选科参与推荐 —— 三件事

1. **agent.go::defaultSystemPrompt** 加一段："用户的再选科目是 X、Y，请在推荐时优先匹配选科要求包含 X/Y 的专业组"
2. **tools.go::generate_volunteer_plan_draft schema** 加 `elective_subjects: string[]` 参数（required，length=2）
3. **recommendation.go::RecommendationRequest** 加 `ElectiveSubjects []string`，validate 时校验枚举

### 6.2 算法层（recommendation_service.go）

**本次最小改动**：在 `apply_filter` / `search_universities` 候选集筛选时，加一个"选科要求过滤"——如果专业组要求 X+Y 而用户没选满足，过滤掉。

**前提**：需要专业组的选科要求数据已在 DB 里。如果当前 schema 没有这个字段，本期降级为：副选科只进 prompt，给 LLM 自然语言提示，不做硬过滤。下一期补"专业组选科要求"字段。

> 待确认：`schema/current.sql` 里 `subject_categories` 字典表是否包含选科要求字段？需要后续单独 grep 确认。

### 6.3 plan_size 兜底

`RecommendationRequest.PlanSize` 字段保留必填。snapshot 构造层填默认值（lookup → fallback 40）。算法层零改动。

## 7. 前端改造

### 7.1 `profile-survey/sections/RequiredSection.tsx`

- 移除：位次 InputNumber、目标志愿数 InputNumber
- 改造选科：从单 `Radio.Group` 改为两块
  - 首选：物理/历史 `Radio.Group`（绑定 `subject_category_code`）
  - 再选：生化地政 `Checkbox.Group`，校验 exactly 2 个（绑定 `elective_subjects`）
- 总分 InputNumber 保留，加 tooltip："位次将根据您的分数自动换算"

### 7.2 `profileLabels.ts`

加 `ELECTIVE_SUBJECT_OPTIONS`：`[{code:'biology',label:'生物'}, ...]`。

### 7.3 snapshot 构造点（最容易被遗漏的地方）

搜全 `recommendation_request` / `recommendation_snapshot` 在前端的拼装位置（多半在 `admission-ai` 模块），改成调后端 `GET /api/v1/me/profile/snapshot` 拿完整 snapshot，不再前端本地拼。

### 7.4 字段完整度计数

`userProfileStore.ts` 里 `filledCount/totalCount` 改：`provincial_rank, plan_size` 不再计入；`elective_subjects` 计入必填。

## 8. 数据迁移 & 上线策略

### 阶段 1（PR1，最小改动）
- migration 007：新建 `region_plan_size_map`、`score_rank_map`、给 `user_profiles` 加 `elective_subjects` 列
- 后端 lookup service + admin handler 五件套
- profile→snapshot 适配层
- 黑龙江 2024、2025 两年数据手工导入

### 阶段 2（PR2）
- 前端表单重构（移除两字段、加 4 选 2）
- snapshot 构造改走后端

### 阶段 3（PR3）
- agent prompt + tools schema 加 elective_subjects
- 推荐算法选科过滤（如果专业组数据具备）

### 阶段 4（之后某个迭代）
- 确认无旧客户端依赖后，migration drop `provincial_rank` / `plan_size` 列

## 9. 风险 & 待确认

1. **专业组选科要求**：DB 是否已有这个字段？没有就要先补，否则副选科只能进 prompt 软提示。
2. **一分一段表数据来源**：黑龙江教育考试院每年发布的 PDF，是否能在 7 月新一届高考前导入完？时间窗口紧。
3. **score_rank_map 缺失年份**：如果当前年还没发布（高考 6 月，发榜 6 月底），需要 fallback 到上一年——`LookupRank` 要带 year-walk 兜底逻辑。
4. **历史用户数据**：移除位次输入后，老用户已存的 `provincial_rank` 是用还是覆盖？建议：snapshot 适配层**优先用换算结果**，profile 表的旧值作为 audit 留存。
5. **plan_size 默认值**：在 region_plan_size_map 完全缺失某 (year, region) 时，回退到代码常量 40。要不要在 admin 提示"缺失年份请补"？
6. **`subject_category` 字典表是否已存在**：调研发现有 `subject_categories(code, name)`，新表的 FK 是否需要建立？建议建立。

## 10. 验收清单

- [ ] 用户填问卷只需选省份、选科（首选 + 4 选 2）、填总分，三步完成
- [ ] AI 智能填报开新对话能正确读取位次（自动换算）和志愿数（自动查表）
- [ ] 算法行为与改造前完全一致（同一用户同一分数，推荐结果一致）
- [ ] 后台能 CRUD `region_plan_size_map`，能批量导入 `score_rank_map`
- [ ] 切换年份时（如 2026 → 2027），管理员只需在后台导一份新数据，无代码改动
- [ ] AI 推荐时副选科作为 prompt 提示（阶段 3 后变为硬过滤）
