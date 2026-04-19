# Admission API

志愿报考分析平台后端 API。基于 Go + Chi + PostgreSQL + Redis 的模块化单体架构模板，内置用户认证、JWT 双 Token、RBAC、限流、健康检查等基础设施。

---

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.25 | 编译快、并发性能好 |
| 路由 | chi | 标准库兼容、中间件链清晰 |
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
| POST | `/api/v1/auth/register` | 用户注册 |
| POST | `/api/v1/auth/login` | 用户登录 |
| POST | `/api/v1/auth/refresh` | 刷新 Token |

### 需认证接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/me` | 获取当前用户信息 |

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
│   ├── user/                # 用户模块（handler/service/store）
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

   r.Route("/api/v1", func(r chi.Router) {
       r.Use(middleware.JWTMiddleware(jwtConfig))
       r.Get("/schools", schoolHandler.List)
   })
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
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) { ... }
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
│                    Chi Router                            │
│  ┌──────────┬──────────┬──────────┬──────────┐         │
│  │  CORS    │  Logger  │ Recover  │Platform  │         │
│  └──────────┴──────────┴──────────┴──────────┘         │
│  ┌──────────┬──────────┬──────────┬──────────┐         │
│  │RateLimit │  JWT     │  RBAC    │         │         │
│  └──────────┴──────────┴──────────┴──────────┘         │
│                       │                                  │
│         ┌─────────────┼─────────────┐                  │
│         ▼             ▼             ▼                  │
│    ┌─────────┐   ┌─────────┐   ┌─────────┐            │
│    │ /health │   │ /auth/* │   │  /me    │            │
│    └────┬────┘   └────┬────┘   └────┬────┘            │
│         │             │             │                   │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐            │
│    │ Handler │   │ Handler │   │ Handler │            │
│    └────┬────┘   └────┬────┘   └────┬────┘            │
│         │             │             │                   │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐            │
│    │ Service │   │ Service │   │ Service │            │
│    └────┬────┘   └────┬────┘   └────┬────┘            │
│         │             │             │                   │
│    ┌────▼────┐   ┌────▼────┐   ┌────▼────┐            │
│    │  Store  │   │  Store  │   │  Store  │            │
│    └────┬────┘   └────┬────┘   └────┬────┘            │
│         │             │             │                   │
│         └─────────────┼─────────────┘                   │
│                       │                                  │
│              ┌────────┴────────┐                        │
│              ▼                 ▼                        │
│        ┌──────────┐     ┌──────────┐                   │
│        │PostgreSQL│     │  Redis   │                   │
│        └──────────┘     └──────────┘                   │
└─────────────────────────────────────────────────────────┘
```

---

## 许可证

MIT
