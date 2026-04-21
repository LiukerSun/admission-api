// @title           Admission API
// @version         1.0
// @description     志愿报考分析平台后端 API
// @host
// @BasePath        /
// @schemes         http
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description "Bearer {token}"
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "admission-api/docs"
	"admission-api/internal/admin"
	"admission-api/internal/analysis"
	"admission-api/internal/health"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/user"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	migrateFlag := flag.String("migrate", "", "Run migrations: up or down")
	flag.Parse()

	cfg := config.Load()

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer database.Close()

	if *migrateFlag != "" {
		return runMigrations(cfg.DatabaseURL, *migrateFlag)
	}

	redisClient, err := redis.New(cfg.RedisAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}
	defer redisClient.Close()

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  time.Duration(cfg.JWTAccessTTLMinutes) * time.Minute,
		RefreshTTL: time.Duration(cfg.JWTRefreshTTLHours) * time.Hour,
	}

	tokenManager := redis.NewRefreshTokenManager(redisClient, jwtConfig.RefreshTTL)

	userStore := user.NewStore(database.Pool())
	userService := user.NewAuthService(userStore, tokenManager, jwtConfig)
	userHandler := user.NewHandler(userService, jwtConfig)

	bindingStore := user.NewBindingStore(database.Pool())
	bindingService := user.NewBindingService(userStore, bindingStore)
	bindingHandler := user.NewBindingHandler(bindingService)

	// 初始化数据分析模块
	analysisStore := analysis.NewStore()
	analysisService := analysis.NewService(analysisStore)
	analysisHandler := analysis.NewHandler(analysisService)

	healthHandler := health.NewHandler(database)

	// Initialize admin module
	adminStore := admin.NewStore(database.Pool())
	adminService := admin.NewService(adminStore, userStore, tokenManager, redisClient)
	adminHandler := admin.NewHandler(adminService)

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger)
	r.Use(middleware.CORS)
	r.Use(middleware.Platform)

	r.GET("/health", healthHandler.Check)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/v1")
	{
		api.POST("/auth/register", middleware.RateLimitMiddleware(redisClient.RDB(), 20, 1*time.Minute), userHandler.Register)
		api.POST("/auth/login", middleware.RateLimitMiddleware(redisClient.RDB(), 20, 1*time.Minute), userHandler.Login)
		api.POST("/auth/refresh", userHandler.Refresh)

		api.GET("/analysis/enrollment-plans", analysisHandler.GetEnrollmentPlans)

		authorized := api.Group("")
		authorized.Use(middleware.JWTMiddleware(jwtConfig))
		authorized.Use(middleware.AuthStatusMiddleware(redisClient))
		authorized.GET("/me", userHandler.Me)
		authorized.PUT("/me/password", userHandler.ChangePassword)
		authorized.POST("/bindings", bindingHandler.CreateBinding)
		authorized.GET("/bindings", bindingHandler.GetMyBindings)

		adminRoutes := authorized.Group("/admin")
		adminRoutes.Use(middleware.RequireRole("admin"))
		adminRoutes.GET("/users/:id", adminHandler.GetUser)
		adminRoutes.GET("/users", adminHandler.ListUsers)
		adminRoutes.PUT("/users/:id", adminHandler.UpdateUser)
		adminRoutes.PUT("/users/:id/role", adminHandler.UpdateRole)
		adminRoutes.PUT("/users/:id/password", adminHandler.ResetPassword)
		adminRoutes.POST("/users/:id/disable", adminHandler.DisableUser)
		adminRoutes.POST("/users/:id/enable", adminHandler.EnableUser)
		adminRoutes.GET("/bindings", adminHandler.ListBindings)
		adminRoutes.GET("/stats", adminHandler.GetStats)
		adminRoutes.DELETE("/bindings/:id", bindingHandler.DeleteBinding)
	}

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server started", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("server stopped")
	return nil
}

func runMigrations(databaseURL, direction string) error {
	m, err := migrate.New("file://migration", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	switch direction {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to run migrations up: %w", err)
		}
		slog.Info("migrations applied successfully")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to run migrations down: %w", err)
		}
		slog.Info("migrations rolled back successfully")
	default:
		return fmt.Errorf("usage: -migrate up | -migrate down")
	}
	return nil
}
