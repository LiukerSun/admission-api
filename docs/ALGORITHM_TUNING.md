# 志愿推荐算法 — 数据驱动调优文档

> 撰写时间：2026-05-12
> 数据库快照：admission（PG 17，schema v10）— 来自 `sourcedata.xlsx` 黑龙江 2025 招生数据 + LLM 评估 ~150 条

本文回答一个问题：**当前算法给出的志愿表为什么不够准？**
答案不在算法里，在数据里。下面把数据库的实际填充情况、算法对每个字段的依赖、以及缺什么数据怎么补，全部列清楚。

> 表格说明：表内单元格保持纯文本（无 markdown 加粗、无 emoji）以便直接粘贴到飞书表格。状态用单独一列表达：完整 / 部分 / 缺失。

---

## 1. 数据现状速览

| 维度 | 数值 | 状态 |
|---|---|---|
| 已导入招生记录 uma | 69650 条 黑龙江物理 31703 + 历史 13819 + 跨年 | 完整 |
| 院校总数 | 1545 所 | 完整 |
| 院校 profile | 1545 行 每校一行 | 完整 |
| 招生组 admission_group | 18724 个 | 完整 |
| 招生组扩展 跨年等效分 | 18724 行 | 完整 |
| 专业 profile 学科水平 评估 | 24128 行 | 完整 |
| 院校研究生 profile | 1545 行 仅 622 行有 master 数据 约 40% | 部分 |
| CHSI 标签 admission_major_tags | 0 行 | 缺失 |
| 标准专业目录 standard_majors | 0 行 | 缺失 |
| LLM 五维评分 precomputed_scores | 约 150 行 持续刷入中 | 缺失 |
| metadata 词表 城市群 家庭 霍兰德 能力 策略 | 全部已 seed | 完整 |

---

## 2. 字段填充率详表

### 2.1 university_profiles 1545 行

| 字段 | 填充数 | 占比 | 状态 | 用途 | 备注 |
|---|---|---|---|---|---|
| city | 1540 | 99.7% | 完整 | 城市偏好 / 城市群匹配 |  |
| region_code | 1540 | 99.7% | 完整 | 省份排除 |  |
| university_tier | 9 | 0.6% | 缺失 | 学校档次 top_2 hua_5 c9 985_other 211_double | 仅清北华5+西交哈工 9 所 |
| is_985 | 52 | 3.4% | 缺失 | tier fallback |  |
| is_211 | 133 | 8.6% | 缺失 | tier fallback |  |
| is_double_first_class | 158 | 10.2% | 缺失 | tier fallback |  |
| is_national_key | 164 | 10.6% | 缺失 | tier fallback |  |
| soft_rank | 1540 | 99.7% | 完整 | school personalization |  |
| postgraduate_recommendation_rate | 364 | 23.6% | 部分 | future_base fallback / 考研 career_plan | 不到 1/4 |
| has_postgraduate_recommendation | 373 | 24.1% | 部分 | warning 生成 |  |
| alumni_rank | 1540 | 99.7% | 未启用 | 数据有 代码没读 |  |
| difficulty_rank | 1035 | 67.0% | 未启用 | 数据有 代码没读 |  |

### 2.2 university_major_admissions 69650 行 核心事实表

| 字段 | 填充数 | 占比 | 状态 | 用途 |
|---|---|---|---|---|
| plan_count | 50592 | 72.6% | 部分 | safe 桶过滤 + 概率不确定性 未启用 |
| min_score | 45522 | 65.4% | 部分 | 历史最低分展示 |
| min_rank | 45522 | 65.4% | 部分 | 核心 定位 rank 窗口 概率估算 |
| equivalent_min_score | 45136 | 64.8% | 部分 | 等效分 已展示 |
| tuition | 24128 | 34.6% | 部分 | 预算硬过滤 |
| duration | 24128 | 34.6% | 未启用 | - |
| major_intro | 18074 | 26.0% | 部分 | LLM 评估 prompt 输入 |
| employment_direction | 22632 | 32.5% | 部分 | 同上 |
| training_goal | 23264 | 33.4% | 未启用 | - |
| postgraduate_direction | 17356 | 24.9% | 未启用 | - |
| admission_remark | 7120 | 10.2% | 未启用 | - |

**关键数据点**：35% 的 uma 行没有 `min_rank`，多半是 2025 当年招生计划行（仅有 plan_count，没有录取分）。算法的 `pickBucket` 会跳过这些行——意味着推荐池只有 65% 的招生计划被纳入排序。

### 2.3 admission_group_extensions 18724 行

| 字段 | 填充数 | 占比 | 状态 | 用途 |
|---|---|---|---|---|
| equivalent_min_score_2024 | 14871 | 79.4% | 完整 | 跨年校准 |
| equivalent_min_score_2023 | 12812 | 68.4% | 部分 | 同上 |
| equivalent_min_score_2022 | 11319 | 60.5% | 部分 | 同上 |
| group_min_rank | 5120 | 27.3% | 未启用 | - |
| batch_remark | 18724 | 100% | 未启用 | - |

**关键**：跨年等效分覆盖率不错（60-80%），但算法代码完全没读这些字段。算法只用 `uma.min_rank` 单年。

### 2.4 university_major_profiles 24128 行

| 字段 | 填充数 | 占比 | 状态 | 用途 |
|---|---|---|---|---|
| discipline_category | 23945 | 99.2% | 完整 | 学科门类 关键字匹配 / future_base 兜底 |
| first_level_discipline | 23945 | 99.2% | 完整 | 同上 |
| fourth_round_subject_eval | 4681 | 19.4% | 缺失 | 教育部第四轮学科评估等级 A+ A B+ ... |
| double_first_class_subject | 577 | 2.4% | 缺失 | 双一流学科 |
| soft_major_grade | 9899 | 41.0% | 部分 | 软科专业评级 A+ A ... |
| major_evaluation_score | 9836 | 40.8% | 部分 | 软科评分 |
| major_rank | 9836 | 40.8% | 部分 | 排名展示 |
| is_national_feature | 2018 | 8.4% | 缺失 | major_base fallback 系数 |

### 2.5 admission_major_tags 空表

| 项 | 内容 |
|---|---|
| 总行数 | 0 |
| 状态 | 缺失 |
| 用途 | CHSI 标准专业目录映射 category_code 0703 化学 / class_code / major_code |
| 算法依赖 | 单科能力门槛 张雪峰生化环材避雷 家庭资源 霍兰德关键字匹配 部分 |

### 2.6 metadata 词表

| 表 | 行数 | 状态 |
|---|---|---|
| city_groups | 6 | 完整 |
| city_group_members | 45 | 完整 |
| recommendation_family_resource_keywords | 17 | 完整 |
| recommendation_holland_keywords | 15 | 完整 |
| recommendation_major_ability_rules | 12 | 完整 |
| recommendation_strategy_keywords | 23 | 完整 |
| standard_majors | 0 | 缺失 |
| major_categories | 0 | 缺失 |
| major_classes | 0 | 缺失 |

---

## 3. 算法对每个字段的依赖路径

下面这张表把 `internal/admission/recommendation_service.go` 里每个评分/过滤步骤展开，标出实际在跑哪些字段、哪些走 fallback 因为数据缺：

| 步骤 | 类型 | 依赖字段 | 状态 | 实际情况 |
|---|---|---|---|---|
| strategy school/major | 决策 | req.PreferredMajors × md.StrategyKeywords | 完整 | 词表有 23 行 |
| 位次窗口 | 决策 | req.ProvincialRank × 比例常数 | 完整 | - |
| rank 窗口过滤 | 过滤 | uma.min_rank | 部分 | 35% uma 因缺 min_rank 被丢 |
| subject_category / subject_requirement | 过滤 | ag.subject_category_code / ag.subject_requirement_code | 完整 | - |
| 预算过滤 | 过滤 | uma.tuition | 部分 | 仅 34.6% 有学费 高预算用户基本不被过滤 低预算用户可能丢一堆没填学费的项 |
| 排除省 / 市 | 过滤 | up.region_code / up.city | 完整 | 99.7% |
| 单科能力门槛 物理 less than 40 排除电子 | 过滤 | c.TagCategoryCodes - admission_major_tags.category_code | 缺失 | 0 行 完全失效 |
| 张雪峰避雷 生化环材 | 过滤 | 同上 | 缺失 | 完全失效 |
| 排除关键字 用户填的化学 | 过滤 | LocalMajorName + DisciplineCategory + FirstLevelDiscipline + TagNames | 部分 | 前 3 字段约 99% TagNames 0% 粒度较粗的关键字仍能命中专业名 跨级聚合关键字匹配不到 |
| 偏好专业 | 加分 | 同上 | 部分 | 同上 |
| 家庭资源词表 | 加分 | 同上 | 部分 | 同上 |
| 霍兰德词表 | 加分 | DisciplineCategory + LocalMajorName | 完整 | 99% |
| career_plan 考研 | 加分 | up.postgraduate_recommendation_rate | 缺失 | 仅 23.6% 76% 院校算法判不出是否适合考研 |
| 学校档次 tierForCandidate | 评分 | up.university_tier - fallback is_985 / is_211 / is_double_first_class | 缺失 | tier 0.6% fallback 也只到 10% 院校 90% 院校直接落到 regular = 1.0 |
| 学校 score schoolScoreForCandidate | 评分 | tier 映射 | 缺失 | 同上 |
| 5 维评分 base | 评分 | recommendation_precomputed_scores LLM | 缺失 | 仅 150 / 17163 对 小于 1% |
| fallback city base | 评分 | md.CityToGroupCode | 完整 | - |
| fallback school base | 评分 | tierForCandidate | 缺失 | 90% 院校落到 1.0 |
| fallback major base | 评分 | IsNationalFeature / SoftMajorGrade / MajorEvaluationScore | 部分 | 国家特色 8% 软科评级 41% 剩下 60% 行无差异 |
| fallback ability_improvement base | 评分 | 始终 1.0 PrecomputedScoreRow 缺字段 公式无信号 | 缺失 | 无法区分 |
| fallback future base | 评分 | PostgraduateRecommendationRate + DisciplineCategory weight | 部分 | 24% 院校有保研率 76% 仅靠学科门类粗加权 工学 1.1 医学 1.05 文学 0.95 农学 0.9 |
| 概率估算 | 排序 | uma.min_rank 单年 | 部分 | 数据本身波动 plus minus 500-2000 单年估算不稳定 |
| safe 桶过滤 | 过滤 | uma.plan_count 大于等于 5 | 完整 | 但 27% 行无 plan_count 被默认放过 |
| LLM tuner | 调优 | DeepSeek API | 未启用 | 默认关闭 前端要勾 enable_llm_tuning |

**净结论**：5 维评分里 3 个维度（major / ability / future）的 base 实际上对绝大多数院校都是 1.0；加上 90% 院校的 school 维度也是 1.0；composite 排序基本退化为"清北 → 华5 → C9 → 985_other → 其他全等" + 弱 city/personalization 调整。

---

## 4. 数据缺口的影响（按严重度排序）

### P0 决定性影响

**P0.1 — CHSI 标签全空**
- 影响：单科能力门槛、张雪峰避雷、家庭/霍兰德关键字匹配（部分）全部失效
- 现状：`admission_major_tags` 0 行，`standard_majors` 0 行
- 补法：从教育部 CHSI 系统拉国标专业目录（年度公开数据，约 700 标准专业 × 5 年版本）→ 灌入 `standard_majors` + 通过专业名 fuzzy match 建立 `admission_major_tags` 关联
- 工作量：拉数据 1 天，匹配脚本 1 天，整体 ≤3 天
- 预期效果：让"物理 30 分排除电子信息类"这种核心硬过滤真正生效

**P0.2 — `university_tier` 仅 9 行**
- 影响：90% 院校的学校档次维度退化为 `regular = 1.0`，包括所有 211 / 985_other / 普通本科
- 现状：仅清北 + 华5 + 西交 + 哈工 9 所
- 补法：写一段 SQL UPDATE backfill：
  - `is_985=true AND name IN (...)` 分配 top_2 / hua_5 / c9 / 985_other
  - `is_211=true OR is_double_first_class=true` 统一标 211_double
  - 公开 985/211 名单是固定的，~150 所手工列出即可
- 工作量：< 半天
- 预期效果：让 school 维度从 9 个特例 → 150 所院校都有合理 tier 分

**P0.3 — `recommendation_precomputed_scores` 几乎全空**
- 影响：5 维评分有 3 维（major / ability / future）实际是 1.0
- 现状：进行中，目标 500 行（覆盖 ~20 所头部 985）
- 补法：继续跑 LLM 评估。目标分阶段：
  - 阶段 A：覆盖所有 985（~39 所 × ~30 majors ≈ 1170 对，~7 小时）
  - 阶段 B：扩展到所有 211（~115 所 × ~30 ≈ 3450 对，再 ~21 小时）
  - 阶段 C：剩余 1400 院校 × 主流 majors，可选
- 工作量：纯运行时间，约 30 小时全量；可分批 / 分天跑
- 预期效果：决定性的——让 5 维评分真正参与排序

### P1 显著影响

**P1.1 — 跨年位次数据没用上**
- 影响：算法看单年 `min_rank` 做概率估算，但年际波动 ±500-2000 名是常态
- 现状：`admission_group_extensions.equivalent_min_score_2022/2023/2024` 覆盖 60-79%，但算法不读
- 补法：service 里取最近 3 年均值/中位数代替单年 min_rank。需要：
  1. `FetchCandidates` 多 join `equivalent_min_score_2022/2023/2024`
  2. 在内存里算 median / mean
  3. `estimateProbability` 改用 multi-year
- 工作量：1 天
- 预期效果：保档桶的"概率 85%"会真的接近 85%，不再是单年偶发

**P1.2 — `plan_count` 没参与概率**
- 影响：招 1 个 vs 招 30 个的不确定性差一个量级，但概率函数一视同仁
- 现状：仅 `ensureSafeQuality` 用 plan_count ≥ 5 做过滤
- 补法：`estimateProbability` 加 plan_count 调整：
  - plan_count ≤ 2：probability × 0.8（波动大）
  - plan_count ≥ 30：probability × 1.05（稳）
- 工作量：< 1 小时（代码改动小，回归测试要补）
- 预期效果：让"招生人数大、本省优势"的保底校真的稳

**P1.3 — `postgraduate_recommendation_rate` 仅 24%**
- 影响：考研倾向的考生匹配不准，future_base fallback 在 76% 院校无信号
- 现状：1545 院校只有 364 有保研率
- 补法：补全保研数据。来源：教育部年度公示文件（每年 5 月公布次年保研人数）
- 工作量：手工录入或爬一次，1 天
- 预期效果：提升 future_base 区分度

**P1.4 — 软科评级 / 第四轮学科评估覆盖低**
- 影响：major_base fallback 在 ~60% 专业无信号
- 现状：soft_major_grade 41%、fourth_round_subject_eval 19%
- 补法：拉全网软科 2024 评级数据 + 教育部第四轮学科评估全集 → 灌入 `university_major_profiles`
- 工作量：1-2 天

### P2 长尾

**P2.1 — 体检 / 性别 / 语种字段未启用**
- 影响：色弱学生应该排除医学/化学，男生应排除性别限制专业（如学前教育部分院校）
- 现状：`req.Gender / req.Language / req.Health` 字段在 model 里但 filter 没读
- 补法：service 加 hardFilter 步骤；DB 需要 `university_major_admissions.gender_limit / health_limit / language_limit` 字段（目前没有，要看 `admission_remark` 解析或新建字段）

**P2.2 — `family_economy` 未启用**
- 影响：经济紧张时应过滤 `tuition > X` 的中外合作办学
- 补法：service 把 `family_economy=紧张` 翻译成 `budget_tuition_max=5000` 类似默认值

**P2.3 — `university_postgraduate_profiles` master / doctoral 仅 25-40%**
- 影响：研究生导向的推荐不准
- 补法：补全研招数据

---

## 5. 算法层（非数据）改进 — 跟数据无关但同样关键

| 编号 | 改动 | 工作量 | 收益 |
|---|---|---|---|
| A1 | composite 改加权求和 取代连乘 | 半天 | 评分稳定性 可解释性大幅提升 |
| A2 | 偏好硬过滤分层 命中优先排序 硬不要直接砍 | 1 天 | 想学计算机不再被材料专业混进列表 |
| A3 | 张雪峰八问交互式表单 前端 + 后端决策树 | 2-3 天 | 关键意向问题 学医 学农 带物理 转成硬过滤 |
| A4 | 卡档切换学校优先 逻辑一 | 1 天 | 高分边缘考生 清北 30% 弱专业 vs 华5 30% 强专业 能正确切换策略 |
| A5 | 输出限定 8 个二级专业目录 | 半天 | 不出现 40 个志愿但 30 个是同方向 的问题 |
| A6 | enable_llm_tuning 默认开 或前端 prominent CTA | 1 小时内 | step 7 的 LLM 复核能真正发挥 |

---

## 6. 推荐路线图

按"对结果改观 / 工作量"性价比：

### Sprint 1（1 周内，得到肉眼可见的质变）
| 序号 | 任务 | 工作量 |
|---|---|---|
| 1 | P0.2 university_tier backfill | 半天 |
| 2 | P0.3 LLM 评估推到 985 全量 | 运行 7 小时 |
| 3 | A1 composite 改加权求和 | 半天 |
| 4 | A2 偏好硬过滤分层 | 1 天 |

### Sprint 2（2 周内，覆盖深层缺陷）
| 序号 | 任务 | 工作量 |
|---|---|---|
| 5 | P0.1 CHSI 标签导入 | 3 天 |
| 6 | P1.1 跨年位次取均值 | 1 天 |
| 7 | P1.2 plan_count 进概率 | 半天 |
| 8 | A4 卡档切换 | 1 天 |

### Sprint 3（长期）
| 序号 | 任务 | 工作量 |
|---|---|---|
| 9 | P1.3 / P1.4 补全保研 软科 学科评估 | 1-2 天 |
| 10 | P0.3 LLM 评估推到 211 全量 | 运行 21 小时 |
| 11 | A3 张雪峰八问表单 | 2-3 天 |
| 12 | P2 体检 性别 语种 经济硬过滤 | 1-2 天 |

---

## 7. 数据来源对照表

| 缺失数据 | 推荐来源 | 获取方式 | 更新频率 |
|---|---|---|---|
| CHSI 标准专业目录 | 教育部 chsi.com.cn | 公开网页表格 可定期爬 | 年度 |
| 985 / 211 名单 | 教育部官网 | 静态名单 手工录入 | 五年级别变动 |
| 软科专业评级 | 软科官网 shanghairanking.cn | 网页爬取 注意 robots | 年度 |
| 第四轮学科评估 | 教育部学位与研究生教育发展中心 | 已公开 PDF / 表格 | 五年级别 |
| 保研率 | 各校年度数据公开 / 阳光高考平台 | 爬取或第三方 API | 年度 |
| 体检限制 / 性别限制 | 各校招生章程 | NLP 解析 admission_remark 字段 | 年度 |
| 跨年录取数据 | 各省考试院 + 阳光高考 | 已部分导入 补齐其他省 | 年度 |
| LLM 评估 base | DeepSeek API | 已配置 按需触发 | 静态 专业属性变化慢 |

---

## 8. 监控建议

落地任何改动前先建几个观察指标，否则改完不知道好不好：

| 指标 | 计算方式 | 用途 |
|---|---|---|
| 召回率代理 | 相同 req 下 推荐表里 user 历史填报偏好命中的院校占比 | 衡量推荐贴合度 |
| 多样性 | 40 条推荐里 distinct city discipline 组合数 | 防同质化 |
| 概率校准 | 标注的录取概率 85% 实际录取率 | 长期 需要后续随访数据 |
| 冲 稳 保比例 | 各 tier 实际填满率 | underflow 越多说明窗口越紧 |
| 5 维评分稀疏度 | evaluated_by = algorithm 或 NULL 占比 | 跟踪 LLM 评估进度 |

---

## 附 脚本入口

| 任务 | 命令 |
|---|---|
| 触发 LLM 评估批量刷新 | bash /tmp/refresh_scores.sh target |
| 查看评估覆盖 | docker exec admission-db psql -U app -d admission -c "SELECT count(*) FROM recommendation_precomputed_scores WHERE evaluated_by='llm'" |
| 触发推荐做对比测试 | curl -X POST http://127.0.0.1:8080/api/v1/admission/recommendations -H "Authorization: Bearer TOKEN" -d @payload.json |
