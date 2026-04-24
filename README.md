# Admission API

志愿报考分析平台后端 API。基于 Go + Gin + PostgreSQL + Redis 的模块化单体架构模板，内置用户认证、JWT 双 Token、RBAC、限流、健康检查等基础设施。

---

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.25 | 编译快、并发性能好 |
| 路由 | gin | 高性能、中间件丰富、生态成熟 |
| 数据库 | PostgreSQL 15 + pgx | 高性能连接池 |
| 缓存 | Redis 7 | Refresh Token 存储、限流计数 |
| 迁移 | golang-migrate | 数据库版本管理 |
| 认证 | JWT | Access Token + Refresh Token 双 Token |
| API 文档 | Swagger | 自动生成，访问 `/swagger/index.html` |
| 容器 | Docker + Docker Compose | 一键启动开发/生产环境 |

---

## 快速启动

### 前置条件

- Go 1.25+
- Docker & Docker Compose
- Make

### 1. 初始化数据库

```bash
make db
```

此命令会：
- 从 `.env.example` 创建 `.env`（如果不存在）
- 启动 PostgreSQL 和 Redis 容器
- 自动运行数据库迁移

### 2. 开发模式启动

```bash
make dev
```

此命令会：
- 启动 PostgreSQL、Redis 容器
- 自动生成 Swagger 文档（如果缺失）
- 运行数据库迁移
- 在宿主机启动应用（带热重载，直接 `go run`）

应用启动后访问：
- API: http://localhost:8080
- Swagger 文档: http://localhost:8080/swagger/index.html
- 健康检查: http://localhost:8080/health

### 3. 生产模式启动

```bash
make run
```

此命令会使用 `docker-compose.prod.yml` 构建并启动完整生产环境（包含应用容器）。

---

## Makefile 命令

```bash
make db      # 初始化数据库容器并运行迁移
make dev     # 开发模式：启动基础设施 + 宿主机运行应用
make run     # 生产模式：Docker Compose 全量构建并启动
make down    # 停止所有容器（开发 + 生产）
make logs    # 查看生产环境应用日志
make build   # 构建 Docker 镜像
make gaokao-import DATA_DIR=/path/to/csv-dir
make gaokao-import-reset DATA_DIR=/path/to/csv-dir
make gaokao-import-sample DATA_DIR=/path/to/csv-dir SAMPLE_ROWS=1000
make gaokao-import-dev DATA_DIR=/path/to/csv-dir
```

---

## 提交规范

本项目使用 [Conventional Commits](https://www.conventionalcommits.org/)。git hooks 会在首次运行 `make db` 或 `make dev` 时自动配置，无需手动操作。

提交信息格式：`<type>(<scope>): <description>`

| Type | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修复 bug |
| `docs` | 仅文档更新 |
| `style` | 代码格式（不影响逻辑） |
| `refactor` | 重构（非 bug 修复、非新功能） |
| `perf` | 性能优化 |
| `test` | 添加/修改测试 |
| `build` | 构建系统或依赖变更 |
| `ci` | CI 配置变更 |
| `chore` | 其他不影响源码和测试的变更 |
| `revert` | 回退之前的提交 |

示例：
```
feat(auth): add JWT refresh token rotation
fix(api): resolve nil pointer in login handler
docs(readme): update deployment instructions
```

> 不规范的提交会被本地 hook 拦截，PR 中的不规范提交也会导致 CI 失败。

---

## 配置

所有配置通过 `.env` 文件管理。首次运行 `make db` 或 `make dev` 时会自动从 `.env.example` 创建。

### 配置项说明

```env
PORT=8080
JWT_SECRET=your-super-secret-jwt-key-change-in-production
JWT_ACCESS_TTL_MINUTES=15
JWT_REFRESH_TTL_HOURS=168
ENV=development

# 数据库和 Redis 连接字符串由以下组件变量自动生成
# 修改端口时只需改下方变量即可

# Database
POSTGRES_USER=app
POSTGRES_PASSWORD=app
POSTGRES_DB=admission
POSTGRES_PORT=5432

# Redis
REDIS_PORT=6379

# Alibaba Cloud SMS
ALIYUN_SMS_ACCESS_KEY_ID=
ALIYUN_SMS_ACCESS_KEY_SECRET=
ALIYUN_SMS_ENDPOINT=dysmsapi.aliyuncs.com
ALIYUN_SMS_SIGN_NAME=
ALIYUN_SMS_TEMPLATE_CODE=

# SMS verification
SMS_CODE_TTL_MINUTES=5
SMS_SEND_COOLDOWN_SECONDS=60
SMS_DAILY_LIMIT=10
SMS_MAX_VERIFY_ATTEMPTS=5
```

> 注意：`DATABASE_URL` 和 `REDIS_ADDR` 由应用根据 `POSTGRES_*` 和 `REDIS_PORT` 自动构建，无需手动配置。

---

## API 接口

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| GET | `/swagger/*` | Swagger API 文档 |
| POST | `/api/v1/auth/register` | 用户注册（需选择 `user_type`: `parent` / `student`）|
| POST | `/api/v1/auth/login` | 用户登录 |
| POST | `/api/v1/auth/refresh` | 刷新 Token |

### 高考分析接口

高考分析接口当前为公开读接口，数据来自 PostgreSQL 的 `gaokao` schema。列表接口统一支持 `page`、`per_page` 分页，默认 `page=1`、`per_page=20`，`per_page` 最大为 `100`。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/analysis/dataset-overview` | 数据集概览，返回学校、专业、招生计划、录取分、省控线等数据量和覆盖范围 |
| GET | `/api/v1/analysis/facets` | 获取筛选项元数据，支持 `schools`、`majors`、`enrollment_plans`、`school_scores`、`major_scores`、`batch_lines` 等 scope |
| GET | `/api/v1/analysis/schools` | 院校列表，支持省份、城市、985/211/双一流等标签、排名、就业率、综合评分等筛选 |
| GET | `/api/v1/analysis/schools/compare` | 院校对比，最多支持 `10` 个 `school_ids` |
| GET | `/api/v1/analysis/schools/:school_id` | 院校详情，支持 `include=profile,tags,rankings,score_summary,plan_summary` |
| GET | `/api/v1/analysis/schools/:school_id/majors` | 查询学校开设专业，支持专业关键词、专业代码、年份等筛选 |
| GET | `/api/v1/analysis/majors` | 专业列表，支持专业名、专业代码、门类、专业类、学位、学制、薪资、就业方向等筛选 |
| GET | `/api/v1/analysis/majors/:major_id` | 专业详情，支持 `include=profile,tags,schools,score_summary,plan_summary` |
| GET | `/api/v1/analysis/enrollment-plans` | 招生计划查询，已从 mock 数据切换为 `gaokao.enrollment_plan_fact` 真实数据 |
| GET | `/api/v1/analysis/province-batch-lines` | 省控线/批次线查询，支持省份、年份、批次、类别、科类、分数范围筛选 |
| GET | `/api/v1/analysis/province-batch-line-trends` | 省控线趋势，按省份、批次、类别/科类返回多年序列 |
| GET | `/api/v1/analysis/admission-scores/schools` | 院校录取分查询，支持省份、年份、学校、批次、科类、分数、位次、线差等筛选 |
| GET | `/api/v1/analysis/admission-scores/majors` | 专业录取分查询，优先按 `school_major_name` 匹配专业，兼容 `major_id` 缺失的数据 |
| GET | `/api/v1/analysis/admission-score-trends` | 院校/专业录取分趋势，支持 `level=school` 或 `level=major` |
| GET | `/api/v1/analysis/score-match` | 基于历史分数/位次的冲稳保参考匹配，不代表录取概率 |
| GET | `/api/v1/analysis/employment-data` | 兼容旧就业数据路径，当前基于专业画像薪资和就业方向数据返回 |

常用筛选约定：

- 多选参数使用逗号分隔，例如 `province=北京,山东`、`year=2023,2024`、`school_tags=985,211`
- 范围参数使用 `_min` / `_max`，例如 `score_min=600&score_max=680`、`rank_max=10000`
- 排序参数使用 `sort`，前缀 `-` 表示倒序，例如 `sort=-lowest_score`
- 关联展开使用 `include`，例如 `include=profile,tags,rankings`
- 分面筛选项使用 `/api/v1/analysis/facets`，例如 `scope=enrollment_plans&fields=province,year,batch,section`
- 录取分接口默认把来源中代表未知值的 `0` 平均分/最高分转为 `null`，调试时可传 `include_zero_scores=true`

示例：

```bash
# 数据概览
curl 'http://localhost:8080/api/v1/analysis/dataset-overview?include_coverage=true&include_imports=true'

# 招生计划：山东 2023 综合类，专业名包含“计算机”
curl 'http://localhost:8080/api/v1/analysis/enrollment-plans?province=山东&year=2023&section=综合&major_name=计算机&per_page=20'

# 院校筛选：北京 985 院校，展开画像、标签、排名
curl 'http://localhost:8080/api/v1/analysis/schools?province=北京&tags=985&include=profile,tags,rankings'

# 专业录取分：河南 2024 法学
curl 'http://localhost:8080/api/v1/analysis/admission-scores/majors?province=河南&year=2024&major_name=法学'

# 历史分数/位次匹配：仅供冲稳保参考
curl 'http://localhost:8080/api/v1/analysis/score-match?province=山东&year=2024&score=660&rank=5000&target=major&strategy=all'
```

### 需认证接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/me` | 获取当前用户信息 |
| PUT | `/api/v1/me/password` | 当前用户修改自己的密码 |
| POST | `/api/v1/me/phone/send-code` | 当前用户向手机号发送短信验证码 |
| POST | `/api/v1/me/phone/verify` | 当前用户校验短信验证码并绑定手机号 |
| POST | `/api/v1/bindings` | 家长发起绑定学生（仅 `user_type=parent`） |
| GET | `/api/v1/bindings` | 查询我的绑定关系 |
| GET | `/api/v1/membership/plans` | 获取可购买会员套餐（月卡/季卡/年卡） |
| GET | `/api/v1/membership` | 获取当前用户会员状态与有效期 |
| POST | `/api/v1/payment/orders` | 创建会员支付订单，支持 `idempotency_key` 幂等 |
| GET | `/api/v1/payment/orders` | 查询我的支付订单列表 |
| GET | `/api/v1/payment/orders/:order_no` | 查询我的支付订单详情 |
| POST | `/api/v1/payment/orders/:order_no/pay` | 使用 mock 渠道完成支付并发放会员 |
| POST | `/api/v1/payment/orders/:order_no/detect` | 主动检测订单支付/权益状态 |

### 会员支付接口

会员支付系统当前仅售卖一个会员等级：`premium`。第一阶段提供三个套餐：`monthly`（30 天）、`quarterly`（90 天）、`yearly`（365 天），金额单位为分，币种为 `CNY`。支付渠道当前仅支持 `mock`，但订单、支付尝试、回调审计、会员权益发放已按后续真实支付渠道扩展设计。

典型 mock 支付流程：

```bash
# 1. 查询套餐
curl -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/v1/membership/plans'

# 2. 创建订单
curl -X POST 'http://localhost:8080/api/v1/payment/orders' \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"plan_code":"monthly","idempotency_key":"checkout-001"}'

# 3. mock 支付并发放会员
curl -X POST -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/v1/payment/orders/MO20260423120000ABCD1234/pay'

# 4. 查看会员状态
curl -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/v1/membership'
```

支付回调审计接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/payment/callbacks/mock` | mock 支付回调，先落库再幂等处理 |

### 管理员接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/users` | 分页获取用户列表，支持按 `email`、`username`、`role`、`status` 过滤 |
| GET | `/api/v1/admin/users/:id` | 获取单个用户详情，用于前端编辑表单回填 |
| PUT | `/api/v1/admin/users/:id` | 修改指定用户的 `email`、`username`、`role`、`user_type`、`status` |
| PUT | `/api/v1/admin/users/:id/role` | 单独修改指定用户角色 |
| PUT | `/api/v1/admin/users/:id/password` | 管理员重置指定用户密码，重置后用户需重新登录 |
| POST | `/api/v1/admin/users/:id/disable` | 禁用指定用户 |
| POST | `/api/v1/admin/users/:id/enable` | 启用指定用户 |
| GET | `/api/v1/admin/bindings` | 分页获取所有家长-学生绑定关系 |
| GET | `/api/v1/admin/stats` | 获取系统统计数据，包括用户总数、角色分布、绑定总数等 |
| DELETE | `/api/v1/admin/bindings/:id` | 解除家长-学生绑定（仅 `role=admin`） |
| GET | `/api/v1/admin/payment/orders` | 管理员查询支付订单，支持订单号、用户、套餐、渠道、状态过滤 |
| GET | `/api/v1/admin/payment/orders/:order_no` | 管理员查看订单详情、支付尝试和回调记录 |
| POST | `/api/v1/admin/payment/orders/:order_no/close` | 管理员关闭未支付订单 |
| POST | `/api/v1/admin/payment/orders/:order_no/redetect` | 管理员重新检测订单支付/权益状态 |
| POST | `/api/v1/admin/payment/orders/:order_no/regrant-membership` | 管理员补发已支付但未履约订单的会员权益 |

手机号验证能力当前行为：
- 仅支持中国大陆 11 位手机号
- 验证码默认有效期 `5` 分钟
- 同一手机号默认 `60` 秒内不可重复发送
- 同一手机号默认每天最多发送 `10` 次验证码
- 同一验证码默认最多校验 `5` 次
- 未配置阿里云短信凭证时，开发环境自动回退到本地 mock SMS 客户端
- 当前短信验证码能力已完成开发，后续仅需在真实阿里云短信签名、模板和账号环境下做联调验证

### 请求头

- `Authorization: Bearer <access_token>` — 认证接口必填
- `X-Platform: web` — 平台标识（web / app / wxmini），由 Platform 中间件自动注入

---

## 开发指南

### 项目结构

```
.
├── cmd/api/                  # 应用入口
│   └── main.go              # 路由注册、服务组装
├── internal/
│   ├── health/              # 健康检查模块
│   ├── user/                # 用户模块（含家长-学生绑定）
│   ├── platform/
│   │   ├── config/          # 配置加载
│   │   ├── db/              # 数据库连接池 + 事务封装
│   │   ├── redis/           # Redis 客户端 + Token 管理
│   │   ├── middleware/      # 中间件（JWT/CORS/限流/Recover等）
│   │   └── web/             # 统一响应格式、错误码
│   └── config/              # 配置结构（可合并到 platform/config）
├── migration/               # 数据库迁移文件
├── docs/                    # Swagger 自动生成文档
├── tests/integration/       # 集成测试
├── docker-compose.yml       # 开发环境
├── docker-compose.prod.yml  # 生产环境
├── Dockerfile               # 多阶段构建
├── Makefile                 # 常用命令
└── .env.example             # 配置模板
```

### 添加新模块

以添加 `school` 模块为例：

1. **创建目录结构**
   ```
   internal/school/
   ├── handler.go
   ├── service.go
   ├── store.go
   └── model.go
   ```

2. **在 `cmd/api/main.go` 注册路由**
   ```go
   schoolStore := school.NewStore(database.Pool())
   schoolService := school.NewService(schoolStore)
   schoolHandler := school.NewHandler(schoolService)

   api := r.Group("/api/v1")
   api.Use(middleware.JWTMiddleware(jwtConfig))
   api.GET("/schools", schoolHandler.List)
   ```

3. **编写数据库迁移**
   ```bash
   # 创建迁移文件
   migrate create -ext sql -dir migration -seq create_schools_table
   ```

4. **运行迁移**
   ```bash
   go run ./cmd/api -migrate up
   ```

### 数据库迁移

```bash
# 向上迁移
go run ./cmd/api -migrate up

# 向下回滚
go run ./cmd/api -migrate down
```

迁移文件位于 `migration/` 目录，使用 [golang-migrate](https://github.com/golang-migrate/migrate) 管理。

## 高考数据初始化与导入

项目已经内置了高校分析数据库 schema 迁移和 CSV 导入器，适合把 `/Users/evan/project/db` 这类 dump 目录反复导入到新环境。

### 1. 初始化数据库

```bash
make db
```

这一步会：

- 启动 PostgreSQL 和 Redis
- 执行所有 migration
- 自动创建 `gaokao` schema 以及高校分析相关表

### 2. 导入高考数据

```bash
make gaokao-import DATA_DIR=/Users/evan/project/db
```

这一步会：

- 扫描你提供目录下的 CSV 文件
- 幂等导入 `gaokao` schema
- 重复执行时尽量使用 `ON CONFLICT` 做更新或跳过

### 3. 清空后重导

```bash
make gaokao-import-reset DATA_DIR=/Users/evan/project/db
```

这一步会先清空 `gaokao` schema 下的业务表，再重新导入。适合：

- 修正导入逻辑后整体重跑
- 在新环境完整恢复数据
- 切换到一批更新后的 CSV dump

### 3.1 样本导入

如果数据量太大，想先验证建库、接口和分析逻辑，可以只导入每个 CSV 的前 N 行：

```bash
make gaokao-import-sample DATA_DIR=/Users/evan/project/db SAMPLE_ROWS=1000
```

或者：

```bash
make gaokao-import-reset DATA_DIR=/Users/evan/project/db SAMPLE_ROWS=1000
```

说明：

- `SAMPLE_ROWS=1000` 表示每个 CSV 只导入前 `1000` 行
- 这是固定前 N 行，不是随机抽样，因此结果可重复
- 很适合本地联调、接口开发、前端对接和排查导入逻辑

### 3.2 开发画像导入

如果你想在本地快速验证建库、导入器、接口和前端联调，可以直接使用开发画像：

```bash
make gaokao-import-dev DATA_DIR=/Users/evan/project/db
```

当前 `dev` 画像默认目标是控制在几分钟内完成，不导入全量数据：

- 每个 CSV 默认最多导入 `1000` 条有效样本
- 每个 CSV 默认最多读取 `5000` 行，避免为了筛选条件扫完整个几十万行大文件
- 默认跳过 `xgk/elective` 相关重表
- 大而重的事实表仍会按固定条件筛选
  - 年份：`2021`、`2023`、`2024`
  - 省份：`11`、`12`、`21`、`35`、`37`、`41`

如果你需要稍微扩大样本，可以覆盖默认值：

```bash
make gaokao-import-dev DATA_DIR=/Users/evan/project/db SAMPLE_ROWS=3000 MAX_READ_ROWS=20000
```

这样做的好处是：

- 导入时间稳定可控
- 不会误扫全量数据
- 样本足够用于本地接口开发、页面联调和排查导入逻辑

### 4. 直接运行导入器

如果你想单独调试导入器，也可以直接运行：

```bash
go run ./cmd/importer -data-dir /Users/evan/project/db
go run ./cmd/importer -data-dir /Users/evan/project/db -truncate
go run ./cmd/importer -data-dir /Users/evan/project/db -truncate -sample-rows 1000
go run ./cmd/importer -data-dir /Users/evan/project/db -truncate -profile dev -skip-xgk -sample-rows 1000 -max-read-rows 5000
```

可选参数：

- `-only base,schools,majors`：只跑指定阶段
- `-skip-xgk`：跳过 `xgk/elective` 相关导入
- `-sample-rows 1000`：每个 CSV 只导入前 `1000` 行
- `-max-read-rows 5000`：每个 CSV 最多读取 `5000` 行，避免筛选样本时扫完整个大文件
- `-profile dev`：使用开发导入画像，按固定年份和省份筛选大表

### 导入器当前覆盖范围

当前导入器已覆盖这些核心数据：

- 省份、城市、学校、专业主数据
- 学校档案、学校排名、专业详情
- 学校/专业政策标签
- 招生政策基础记录
- 学校专业目录
- 选科要求维度、院校专业组
- 学校录取线、专业录取线、招生计划
- 省控线、分数范围

后续如果你增加新的 CSV 或想增强推荐分析字段，可以继续在这个导入器上扩展，而不用改手工导库流程。

### 测试

```bash
# 运行单元测试
go test ./internal/...

# 运行集成测试（需要数据库）
go test ./tests/...

# 生成覆盖率报告
go test -cover ./internal/...
```

### Swagger 文档

使用 [swaggo](https://github.com/swaggo/swag) 自动生成：

```bash
# 生成/更新文档
go run github.com/swaggo/swag/cmd/swag@v1.8.12 init -g cmd/api/main.go

# 开发模式下会自动生成（make dev）
```

在 handler 方法上添加注释即可自动生成文档，示例：

```go
// Register godoc
// @Summary      用户注册
// @Description  创建新用户账号
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  RegisterRequest  true  "注册信息"
// @Success      200  {object}  web.Response{data=user.Response}
// @Router       /api/v1/auth/register [post]
func (h *Handler) Register(c *gin.Context) { ... }
```

---

## 部署指南

### Docker 部署

```bash
# 构建镜像
make build

# 或使用 docker build 直接构建
docker build -t admission-api .

# 生产环境启动（包含数据库、Redis、应用）
make run
```

### 环境变量

生产环境通过 `.env` 文件注入，关键变量：

```env
JWT_SECRET=<生产环境强密码>
ENV=production
```

> 生产环境务必修改 `JWT_SECRET`，并使用强密码。

### CI/CD

项目已配置 GitHub Actions 流水线（`.github/workflows/pipeline.yml`）：

| Job | 说明 |
|-----|------|
| lint | golangci-lint 代码检查 |
| unit-test | 单元测试 |
| integration-test | 集成测试（启动数据库容器） |
| build | 构建并推送 Docker 镜像到 ghcr.io |
| deploy | 部署到服务器（需配置 SSH secrets） |

### 手动部署到服务器

```bash
# 1. 服务器上克隆代码
git clone <repo-url>
cd admission-api

# 2. 创建并编辑 .env
cp .env.example .env
vim .env

# 3. 启动
make run
```

---

## 系统架构

```
┌─────────────────────────────────────────────────────────┐
│                        Client                            │
│  (Web / App / 小程序)                                     │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP
┌────────────────────▼────────────────────────────────────┐
│                    Gin Router                            │
│  ┌──────────┬──────────┬──────────┬──────────┐         │
│  │  CORS    │  Logger  │ Recover  │Platform  │         │
│  └──────────┴──────────┴──────────┴──────────┘         │
│  ┌──────────┬──────────┬──────────┬──────────┐         │
│  │RateLimit │  JWT     │  RBAC    │         │         │
│  └──────────┴──────────┴──────────┴──────────┘         │
│                       │                                  │
│         ┌─────────────┼─────────────┐                  │
│         ▼             ▼             ▼             ▼    │
│    ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌────────┐│
│    │ /health │   │ /auth/* │   │  /me    │   │/bindings│
│    └────┬────┘   └────┬────┘   └────┬────┘   └────┬───┘│
│         │             │             │             │       │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐   ┌────▼────┐ │
│    │ Handler │   │ Handler │   │ Handler │   │ Binding │ │
│    │  health │   │  auth   │   │  user   │   │ Handler │ │
│    └────┬────┘   └────┬────┘   └────┬────┘   └────┬────┘ │
│         │             │             │             │       │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐   ┌────▼────┐ │
│    │ Service │   │ Service │   │ Service │   │ Binding │ │
│    └────┬────┘   └────┬────┘   └────┬────┘   │ Service │ │
│         │             │             │        └────┬────┘ │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐   ┌────▼────┐ │
│    │  Store  │   │  Store  │   │  Store  │   │ Binding │ │
│    └────┬────┘   └────┬────┘   └────┬────┘   │  Store  │ │
│         │             │             │        └────┬────┘ │
│         └─────────────┴─────────────┴─────────────┘       │
│                           │                               │
│              ┌────────────┴────────────┐                 │
│              ▼                         ▼                 │
│        ┌──────────┐           ┌──────────┐               │
│        │PostgreSQL│           │  Redis   │               │
│        └──────────┘           └──────────┘               │
└─────────────────────────────────────────────────────────┘
```

---

## 许可证

MIT
