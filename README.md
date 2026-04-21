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

### 需认证接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/me` | 获取当前用户信息 |
| POST | `/api/v1/bindings` | 家长发起绑定学生（仅 `user_type=parent`） |
| GET | `/api/v1/bindings` | 查询我的绑定关系 |

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
