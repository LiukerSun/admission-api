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
	"admission-api/internal/membership"
	"admission-api/internal/payment"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/sms"
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

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

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
	smsClient := sms.NewMockClient()
	if cfg.AliyunSMSAccessKeyID != "" && cfg.AliyunSMSAccessKeySecret != "" && cfg.AliyunSMSSignName != "" && cfg.AliyunSMSTemplateCode != "" {
		aliyunClient, err := sms.NewAliyunClient(&sms.AliyunConfig{
			AccessKeyID:     cfg.AliyunSMSAccessKeyID,
			AccessKeySecret: cfg.AliyunSMSAccessKeySecret,
			Endpoint:        cfg.AliyunSMSEndpoint,
			SignName:        cfg.AliyunSMSSignName,
			TemplateCode:    cfg.AliyunSMSTemplateCode,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize aliyun sms client: %w", err)
		}
		smsClient = aliyunClient
	}
	phoneVerificationService := user.NewPhoneVerificationService(userStore, redisClient, smsClient, user.PhoneVerificationConfig{
		CodeTTL:      time.Duration(cfg.SMSCodeTTLMinutes) * time.Minute,
		SendCooldown: time.Duration(cfg.SMSSendCooldownSeconds) * time.Second,
		DailyLimit:   cfg.SMSDailyLimit,
		MaxAttempts:  cfg.SMSMaxVerifyAttempts,
	})
	userHandler := user.NewHandler(userService, phoneVerificationService, jwtConfig)

	bindingStore := user.NewBindingStore(database.Pool())
	bindingService := user.NewBindingService(userStore, bindingStore)
	bindingHandler := user.NewBindingHandler(bindingService)

	membershipStore := membership.NewStore(database.Pool())
	membershipService := membership.NewService(membershipStore)
	membershipHandler := membership.NewHandler(membershipService)

	paymentStore := payment.NewStore(database.Pool())
	paymentService := payment.NewService(paymentStore, membershipService)
	paymentHandler := payment.NewHandler(paymentService, payment.HandlerOptions{
		AllowAnonymousMockCallback: cfg.Env == "development",
		MockCallbackSecret:         cfg.MockCallbackSecret,
	})

	// 初始化数据分析模块
	analysisStore := analysis.NewStore(database.Pool())
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

		api.GET("/analysis/dataset-overview", analysisHandler.GetDatasetOverview)
		api.GET("/analysis/facets", analysisHandler.GetFacets)
		api.GET("/analysis/schools", analysisHandler.ListSchools)
		api.GET("/analysis/schools/compare", analysisHandler.CompareSchools)
		api.GET("/analysis/schools/:school_id", analysisHandler.GetSchool)
		api.GET("/analysis/schools/:school_id/majors", analysisHandler.ListSchoolMajors)
		api.GET("/analysis/majors", analysisHandler.ListMajors)
		api.GET("/analysis/majors/:major_id", analysisHandler.GetMajor)
		api.GET("/analysis/enrollment-plans", analysisHandler.GetEnrollmentPlans)
		api.GET("/analysis/province-batch-lines", analysisHandler.ListProvinceBatchLines)
		api.GET("/analysis/province-batch-line-trends", analysisHandler.GetProvinceBatchLineTrend)
		api.GET("/analysis/admission-scores/schools", analysisHandler.ListSchoolAdmissionScores)
		api.GET("/analysis/admission-scores/majors", analysisHandler.ListMajorAdmissionScores)
		api.GET("/analysis/admission-score-trends", analysisHandler.GetAdmissionScoreTrend)
		api.GET("/analysis/score-match", analysisHandler.GetScoreMatch)
		api.GET("/analysis/employment-data", analysisHandler.GetEmploymentData)

		api.POST("/payment/callbacks/mock", paymentHandler.MockCallback)

		authorized := api.Group("")
		authorized.Use(middleware.JWTMiddleware(jwtConfig))
		authorized.Use(middleware.AuthStatusMiddleware(redisClient, func(ctx context.Context, userID int64) (string, error) {
			u, err := userStore.GetByID(ctx, userID)
			if err != nil {
				return "", err
			}
			return u.Status, nil
		}))
		authorized.GET("/me", userHandler.Me)
		authorized.PUT("/me/password", userHandler.ChangePassword)
		authorized.POST("/me/phone/send-code", userHandler.SendPhoneVerificationCode)
		authorized.POST("/me/phone/verify", userHandler.VerifyPhone)
		authorized.POST("/bindings", bindingHandler.CreateBinding)
		authorized.GET("/bindings", bindingHandler.GetMyBindings)
		authorized.GET("/membership/plans", membershipHandler.ListPlans)
		authorized.GET("/membership", membershipHandler.GetCurrent)
		authorized.POST("/payment/orders", paymentHandler.CreateOrder)
		authorized.GET("/payment/orders", paymentHandler.ListMyOrders)
		authorized.GET("/payment/orders/:order_no", paymentHandler.GetMyOrder)
		authorized.POST("/payment/orders/:order_no/pay", paymentHandler.PayMock)
		authorized.POST("/payment/orders/:order_no/detect", paymentHandler.Detect)

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
		adminRoutes.GET("/payment/orders", paymentHandler.AdminListOrders)
		adminRoutes.GET("/payment/orders/:order_no", paymentHandler.AdminGetOrder)
		adminRoutes.POST("/payment/orders/:order_no/close", paymentHandler.AdminCloseOrder)
		adminRoutes.POST("/payment/orders/:order_no/redetect", paymentHandler.AdminRedetect)
		adminRoutes.POST("/payment/orders/:order_no/regrant-membership", paymentHandler.AdminRegrantMembership)
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
