# 推荐算法 v2 设计文档

> 状态：设计已对齐，未实现
> 日期：2026-05-13
> 旧版本（v1）位于 `internal/admission/recommendation_*.go`，将与 v2 并存

## 1. 目标

让用户通过 AI 对话筛选出**专业标签 + 地区标签 + 院校档次 + 专业评估**，
然后算法用学生分数、所在地、上述标签筛选出推荐表。
位次预测核心：往年数据加权平均 + 标准差作为"上下浮动"边界。

## 2. 输入参数

```
POST /api/v1/admission/recommend
```

**主路径：纯结构化参数**（AI 工具直接传参调用）。

```jsonc
{
  "total_score": 550,                       // 学生分数（最新一年等效坐标）  [required]
  "region_code": "230000",                  // 考生省份（如黑龙江）          [required]
  "subject_category_code": "physics",       // 物理类 / 历史类                [required]

  // 专业标签（OR / IN）
  "class_codes": ["0809", "0807"],          // major_classes.class_code 列表 [required, ≥1]

  // 地区硬过滤（任意命中即可；三者都空 = 不限地区）
  "region_filter": {
    "city_groups": ["yrd", "gba"],          // city_groups.code
    "provinces":   ["110000"],              // region_code
    "cities":      []                       // 城市名直接列举
  },

  // 院校档次硬过滤（多选 OR；空数组 = 不限）
  "school_filter": ["985", "211"],          // 可选: 985 / 211 / 双一流 / 普通本科

  // 专业评估硬过滤（阈值；空字符串或省略 = 不限）
  "major_eval_min": "A-",                   // A+ / A / A- / B+ / B / B- / C+ / C / C-

  // tier 边界（可选，默认值见下）
  "rush_offset":  20,                       // 默认 +20
  "match_offset": -15,                      // 默认 -15
  "safe_offset":  -30,                      // 默认 -30

  // 输出
  "plan_size": 40,                          // 默认 40，无上限
  "tier_split": [1, 2, 1]                   // 冲:稳:保 默认 1:2:1
}
```

**便利路径：自然语言**（可选；用于 chat UI / 模糊场景）。
仅传 `ai_query` 一个字段，后端先调 LLM 把它解析为上面的所有结构化字段，再走算法。

```jsonc
{
  "ai_query": "我550分黑龙江想学计算机只考虑985"
}
```

两条路径互斥：若 `ai_query` 非空则忽略其它字段；否则按主路径执行。
**AI 工具调用一律走主路径**——纯参数、无 side effect、可重试、易于测试。

## 3. 算法流程

### 3.1 数据准备

- 取数据库中**最近三年**有 `min_rank` 的 admission_year（如 [2024, 2023, 2022]）。
- 等效分基准年 = 最新一年。`equivalent_min_score` 已按此校准。
- 学生分数 `S` 按基准年坐标解释（默认 raw 分数 = 基准年等效分）。

### 3.2 最低域公式

对每个 `(university_id, local_major_code)`：

| 命中年组合 | 加权公式 | σ |
|---|---|---|
| 三年全 (Y0, Y-1, Y-2) | `0.5·Y0 + 0.3·Y-1 + 0.2·Y-2` | `STDEV.S(Y0, Y-1, Y-2)` |
| 仅 Y0, Y-1 | `0.7·Y0 + 0.3·Y-1` | `STDEV.S(Y0, Y-1)` |
| 仅 Y0, Y-2 | `0.8·Y0 + 0.2·Y-2` | `STDEV.S(Y0, Y-2)` |
| 仅 Y-1, Y-2 | `0.6·Y-1 + 0.4·Y-2` | `STDEV.S(Y-1, Y-2)` |
| 仅 1 年 | 直接用那一年的值 | 0 |

`最低域 = weighted + σ`，单位是等效分。

> 最高域（`weighted - σ`，用 max_score）暂不计算 — 算法只用最低域。
> `equivalent_max_score` 列也无需补。

### 3.3 候选筛选（全部硬过滤）

1. `admission_groups.admission_year = 最新一年`（带招生计划的那一年）。
2. `admission_groups.region_code = req.region_code`，`subject_category_code = req.subject_category_code`。
3. 通过 `admission_major_tags` JOIN：`tag_level='major' OR 'class' 且 class_code IN req.class_codes`。
4. 地区命中（任意一条满足即可）：
   - `university_profiles.city IN region_filter.cities`，或
   - `university_profiles.city` 属于 `region_filter.city_groups` 任一城市群成员，或
   - `university_profiles.region_code IN region_filter.provinces`。
5. 院校档次命中（任一档次匹配即可）：
   - `"985" → up.is_985 = true`
   - `"211" → up.is_211 = true`
   - `"双一流" → up.is_double_first_class = true`
   - `"普通本科" → 上述三者都为 false`
6. `university_major_profiles.fourth_round_subject_eval >= req.major_eval_min`（按官方等级序）。
7. `最低域 ∈ [S + safe_offset, S + rush_offset]`，即 `[S-30, S+20]`（默认）。

### 3.4 tier 贴标

```
冲: 最低域 > S
稳: S + match_offset ≤ 最低域 ≤ S        // match_offset = -15
保: S + safe_offset  ≤ 最低域 < S + match_offset
```

### 3.5 排序

每个 tier 内按 `|最低域 - S|` **升序**——越贴近 S 越靠前。

### 3.6 输出

```
plan_size = 40, tier_split = [1, 2, 1]
  → 冲 10 / 稳 20 / 保 10
plan_size = 100, tier_split = [1, 2, 1]
  → 冲 25 / 稳 50 / 保 25
```

- 同校不限。
- plan_size 默认 40，传参不限上限。
- 如果某 tier 实际命中数 < quota，按现有数量返回（不强行借位）。

## 4. AI 衔接

### 4.1 自然语言便利路径

`ai_query` 非空时：

1. 后端调 LLM，把自然语言提纯为 `class_codes / region_filter / school_filter / major_eval_min / total_score / region_code`。
2. 把缺失字段用默认值补齐；如果关键字段（`total_score`、`class_codes`）无法提取，返回 400 + 提示。
3. 拿提纯后的参数走算法。

响应同时返回：
- `parsed_params`：算法实际使用的结构化参数（供前端展示 chips）。
- `items`：推荐列表（含 tier、最低域、学校/专业元数据）。

### 4.2 AI Tool 调用契约（主路径）

接口被设计为**纯函数式工具**：相同输入永远返回相同输出（同一数据库快照下），
无 session、无 cookie、无副作用，可被任意 LLM 客户端（Claude function-calling、
OpenAI tools、MCP server、自家智能体）作为工具调用。

**OpenAI / Anthropic tool 定义示例**：

```jsonc
{
  "name": "recommend_admissions",
  "description": "根据学生分数、专业类标签、地区/院校/专业评估硬过滤，返回冲/稳/保 三档推荐表。返回的每条都含学校、专业、估算最低录取分（最低域）和 tier 标签。",
  "input_schema": {
    "type": "object",
    "required": ["total_score", "region_code", "subject_category_code", "class_codes"],
    "properties": {
      "total_score":            { "type": "integer", "minimum": 0, "maximum": 750, "description": "高考总分（最新一年等效坐标）" },
      "region_code":            { "type": "string",  "description": "考生省份代码，如 230000" },
      "subject_category_code":  { "type": "string",  "enum": ["physics", "history"] },
      "class_codes":            { "type": "array",   "items": { "type": "string", "pattern": "^[0-9]{4}$" }, "minItems": 1, "description": "二级专业类代码列表，OR 关系" },
      "region_filter": {
        "type": "object",
        "properties": {
          "city_groups": { "type": "array", "items": { "type": "string" } },
          "provinces":   { "type": "array", "items": { "type": "string" } },
          "cities":      { "type": "array", "items": { "type": "string" } }
        }
      },
      "school_filter":    { "type": "array",   "items": { "type": "string", "enum": ["985", "211", "双一流", "普通本科"] } },
      "major_eval_min":   { "type": "string",  "enum": ["A+", "A", "A-", "B+", "B", "B-", "C+", "C", "C-", ""] },
      "rush_offset":      { "type": "integer", "default": 20 },
      "match_offset":     { "type": "integer", "default": -15 },
      "safe_offset":      { "type": "integer", "default": -30 },
      "plan_size":        { "type": "integer", "minimum": 1, "default": 40 },
      "tier_split":       { "type": "array",   "items": { "type": "integer" }, "minItems": 3, "maxItems": 3, "default": [1, 2, 1] }
    }
  }
}
```

**调用约束**：

- 方法：`POST` （语义上 GET-safe；选 POST 仅因 body 较大）。
- 鉴权：可选 Bearer token（若启用），与现有 API 一致；AI 工具客户端用 service token 即可。
- 幂等：是。
- 超时：< 2s（候选池预筛后 SQL 一次跑完）。
- 错误码：`400` 参数非法、`200` + `items: []` 表示"参数合法但 0 命中"。

**MCP server 暴露建议**（如果未来接入 MCP）：

- Tool 名 `recommend_admissions`
- Resource：`admission://catalog/major_classes`（93 个二级专业类列表）、`admission://catalog/city_groups`（城市群代码表）
  ——AI 工具先查 resource 再决定 `class_codes` / `city_groups`，避免幻觉编造代码。

### 4.3 给 AI 用的辅助查询接口

为了让 AI 不靠记忆而是靠查询来填参数，建议同时暴露：

| 接口 | 用途 |
|---|---|
| `GET /api/v1/catalog/major_classes` | 列出 93 个二级专业类（class_code + name） |
| `GET /api/v1/catalog/city_groups`   | 列出城市群（code + name + 成员城市） |
| `GET /api/v1/catalog/regions`       | 列出省份 region_code + name |

这三个接口都是只读、可被 AI 工具串行调用——AI 先查询，再用结果填到 `recommend_admissions` 的 `class_codes` / `region_filter` 里。

## 5. 与 v1 的关系

- v1 (`recommendation_service.go`) 不动。
- v2 走新路径 `/api/v1/admission/recommend/v2`（最终命名待定）。
- 共享 `university_major_admissions`、`admission_major_tags`、`major_classes`、`university_profiles`、`university_major_profiles`。
- v1 用到的 family_resources / holland / ability_rules / precomputed_scores 在 v2 里**全部不用**。

## 6. 待办（不在 v2 算法范围内）

- 给每个标准专业打**选科 tag**（如"物理"、"化学"），算法里加 hook：当 request 带 `excluded_subjects`，过滤掉打了对应 tag 的专业。
- ETL：每年新数据导入时重算所有历史年的 `equivalent_min_score`，使基准年滚动到最新一年。

## 7. 字段映射速查

| 算法概念 | 数据库字段 |
|---|---|
| 学生分 S | `request.total_score` |
| 候选最低域 | 由 `university_major_admissions.equivalent_min_score`（按 admission_year 取出 3 年）按公式计算 |
| 专业类标签 | `admission_major_tags.class_code`（catalog_year=2026 时 OR JOIN `major_classes`） |
| 城市群 | `city_group_members` JOIN `university_profiles.city` |
| 985 / 211 / 双一流 | `university_profiles.is_985 / is_211 / is_double_first_class` |
| 专业评估等级 | `university_major_profiles.fourth_round_subject_eval` |
| 学校 city / region_code | `university_profiles.city / region_code` |

## 8. 边界场景

- **候选 0 命中**：返回空 `items`，`parsed_params` 仍照常返回；前端提示"未匹配到候选，请放宽筛选条件"。
- **缺历史数据的招生组**：单年也算，σ=0；如果近三年都没数据则丢弃。
- **`major_eval_min` 不限**：跳过该过滤；同理 `school_filter=[]` 跳过、`class_codes=[]` 跳过。
- **`region_filter` 三者都空**：地区不限。
