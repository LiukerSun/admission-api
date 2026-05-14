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
	"admission-api/internal/admission"
	"admission-api/internal/ai"
	"admission-api/internal/conversation"
	"admission-api/internal/health"
	"admission-api/internal/membership"
	"admission-api/internal/payment"
	"admission-api/internal/platform/alipay"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/sms"
	"admission-api/internal/user"
	"admission-api/internal/volunteerplan"
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
			ParamFormat:     cfg.AliyunSMSTemplateParamFormat,
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

	membershipStore := membership.NewStore(database.Pool())
	membershipService := membership.NewService(membershipStore)
	membershipHandler := membership.NewHandler(membershipService)

	var alipayClient alipay.Client
	if cfg.AlipayAppID != "" &&
		(cfg.AlipayAppPrivateKey != "" || cfg.AlipayAppPrivateKeyPath != "") &&
		cfg.AlipayAppPublicCertPath != "" && cfg.AlipayAlipayPublicCertPath != "" && cfg.AlipayAlipayRootCertPath != "" {
		var err error
		alipayClient, err = alipay.NewClient(&alipay.Config{
			AppID:                cfg.AlipayAppID,
			AppPrivateKey:        cfg.AlipayAppPrivateKey,
			AppPrivateKeyPath:    cfg.AlipayAppPrivateKeyPath,
			AppPublicCertPath:    cfg.AlipayAppPublicCertPath,
			AlipayPublicCertPath: cfg.AlipayAlipayPublicCertPath,
			AlipayRootCertPath:   cfg.AlipayAlipayRootCertPath,
			NotifyURL:            cfg.AlipayNotifyURL,
			ReturnURL:            cfg.AlipayReturnURL,
			IsProduction:         !cfg.AlipaySandbox,
			EncryptKey:           cfg.AlipayEncryptKey,
			DecryptKey:           cfg.AlipayDecryptKey,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize alipay client: %w", err)
		}
	}

	paymentStore := payment.NewStore(database.Pool())
	paymentService := payment.NewService(paymentStore, membershipService, alipayClient, cfg.AlipayNotifyURL, cfg.AlipayReturnURL)
	paymentHandler := payment.NewHandler(paymentService, payment.HandlerOptions{
		AllowAnonymousMockCallback: cfg.Env == "development",
		MockCallbackSecret:         cfg.MockCallbackSecret,
	})

	dictionaryStore := admission.NewDictionaryStore(database.Pool())
	dictionaryService := admission.NewDictionaryService(dictionaryStore)
	dictionaryHandler := admission.NewDictionaryHandler(dictionaryService)
	majorCatalogStore := admission.NewMajorCatalogStore(database.Pool())
	majorCatalogService := admission.NewMajorCatalogService(majorCatalogStore)
	majorCatalogHandler := admission.NewMajorCatalogHandler(majorCatalogService)
	universityStore := admission.NewUniversityStore(database.Pool())
	universityService := admission.NewUniversityService(universityStore)
	universityHandler := admission.NewUniversityHandler(universityService)
	admissionLineStore := admission.NewAdmissionLineStore(database.Pool())
	admissionLineService := admission.NewAdmissionLineService(admissionLineStore)
	admissionLineHandler := admission.NewAdmissionLineHandler(admissionLineService)
	aggregateStore := admission.NewAggregateStore(database.Pool())
	aggregateService := admission.NewAggregateService(aggregateStore)
	aggregateHandler := admission.NewAggregateHandler(aggregateService)
	volunteerPlanService := admission.NewVolunteerPlanService(database.Pool())
	volunteerPlanHandler := admission.NewVolunteerPlanHandler(volunteerPlanService)
	conversationStore := conversation.NewStore(database.Pool())
	conversationService := conversation.NewService(conversationStore)
	conversationHandler := conversation.NewHandler(conversationService)
	volunteerDraftStore := volunteerplan.NewDraftStore(database.Pool())
	volunteerPlanStore := volunteerplan.NewPlanStore(database.Pool())
	volunteerPlanServiceV2 := volunteerplan.NewService(volunteerDraftStore, volunteerPlanStore, conversationService)
	volunteerPlanHandlerV2 := volunteerplan.NewHandler(volunteerPlanServiceV2)

	var llmProxy ai.LLMProxy
	switch cfg.LLMProvider {
	case "anthropic":
		// True Anthropic streaming is not implemented yet; the client
		// satisfies ChatCompletionStream by wrapping a single
		// ChatCompletion call. First-token latency therefore equals
		// total generation time on this backend.
		slog.Warn("anthropic provider falls back to non-streaming completion in this version")
		llmProxy = ai.NewAnthropicClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	default:
		llmProxy = ai.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	}

	// 推荐算法栈：store + metadata + LLM tuner 组装出 v2 service。
	// 在 toolExecutor 之前初始化，因为 AI 工具 generate_volunteer_plan_draft 要调它。
	recommendationStore := admission.NewRecommendationStore(database.Pool())
	recommendationMetadataStore := admission.NewRecommendationMetadataStore(database.Pool())
	recommendationTuner := ai.NewRecommendationTuner(llmProxy)
	recommendationService := admission.NewRecommendationService(recommendationStore, recommendationMetadataStore, recommendationTuner)
	recommendationHandler := admission.NewRecommendationHandler(recommendationService)

	recommendationScoreStore := admission.NewRecommendationScoreStore(database.Pool())
	// AlgorithmicScoreEvaluator 需要 metadata snapshot 来跑 fallback 公式（城市群匹配等）。
	// Load 失败不阻塞启动——只是 fallback 评估器拿到空 metadata，退化成全 1.0。
	startupMD, mdErr := recommendationMetadataStore.Load(context.Background())
	if mdErr != nil {
		slog.Warn("load recommendation metadata at startup failed; algorithmic evaluator will use empty snapshot", "error", mdErr)
		startupMD = &admission.RecommendationMetadata{}
	}
	var scoreEvaluator admission.ScoreEvaluator = admission.NewAlgorithmicScoreEvaluator(startupMD)
	if cfg.LLMAPIKey != "" {
		scoreEvaluator = ai.NewLLMScoreEvaluator(llmProxy, cfg.LLMModel)
	}
	recommendationScoreRefresher := admission.NewRecommendationScoreRefresher(recommendationScoreStore, scoreEvaluator)
	recommendationScoreHandler := admission.NewRecommendationScoreHandler(recommendationScoreRefresher)

	toolExecutor := ai.NewToolExecutor(admissionLineStore, aggregateStore, recommendationService, volunteerDraftStore)
	toolExecutor.SetCardLinkWhitelist(cfg.CardLinkWhitelist)
	agent := ai.NewAgent(llmProxy, toolExecutor)
	aiHandler := ai.NewHandler(agent, conversationService)
	aiSuggestionsHandler := ai.NewSuggestionsHandler(llmProxy, conversationService, redisClient)
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

		api.POST("/payment/callbacks/mock", paymentHandler.MockCallback)
		api.POST("/payment/callbacks/alipay", paymentHandler.AlipayCallback)
		api.GET("/admission/dictionaries", dictionaryHandler.ListDictionaries)
		api.GET("/admission/major-catalog/latest-year", majorCatalogHandler.LatestCatalogYear)
		api.GET("/admission/standard-majors", majorCatalogHandler.ListStandardMajors)
		api.GET("/admission/universities", universityHandler.ListUniversities)
		api.GET("/admission/universities/:id/profile", universityHandler.GetUniversityProfile)
		api.GET("/admission/admission-lines", admissionLineHandler.ListAdmissionLines)
		api.GET("/admission/aggregate", aggregateHandler.Aggregate)

		authorized := api.Group("")
		authorized.Use(middleware.JWTMiddleware(jwtConfig))
		authorized.Use(middleware.AuthStatusMiddleware(redisClient, func(ctx context.Context, userID int64) (string, error) {
			u, err := userStore.GetByID(ctx, userID)
			if err != nil {
				return "", err
			}
			return u.Status, nil
		}))
		authorized.GET("/admission/volunteer-plans", volunteerPlanHandler.ListVolunteerPlans)
		authorized.GET("/admission/volunteer-plans/:id/rich-details", volunteerPlanHandler.GetRichVolunteerPlan)
		authorized.PUT("/admission/volunteer-plans/:id", volunteerPlanHandler.UpdateVolunteerPlan)
		authorized.PUT("/admission/volunteer-plans/groups/:groupId/remark", volunteerPlanHandler.UpdateGroupRemark)
		authorized.GET("/me", userHandler.Me)
		authorized.PUT("/me/password", userHandler.ChangePassword)
		authorized.POST("/me/phone/send-code", userHandler.SendPhoneVerificationCode)
		authorized.POST("/me/phone/verify", userHandler.VerifyPhone)
		authorized.GET("/membership/plans", membershipHandler.ListPlans)
		authorized.GET("/membership", membershipHandler.GetCurrent)
		authorized.POST("/conversations", conversationHandler.CreateConversation)
		authorized.GET("/conversations", conversationHandler.ListConversations)
		authorized.GET("/conversations/:id", conversationHandler.GetConversation)
		authorized.POST("/conversations/:id/messages", conversationHandler.AddMessage)
		authorized.DELETE("/conversations/:id", conversationHandler.DeleteConversation)
		authorized.POST("/conversations/:id/archive", conversationHandler.ArchiveConversation)
		authorized.POST("/conversations/:id/rollback", middleware.RateLimitByUser(redisClient.RDB(), 30, 1*time.Minute), conversationHandler.Rollback)
		authorized.POST("/ai/chat", middleware.RateLimitByUser(redisClient.RDB(), 30, 1*time.Minute), aiHandler.Chat)
		authorized.POST("/conversations/:id/ai-chat", middleware.RateLimitByUser(redisClient.RDB(), 30, 1*time.Minute), aiHandler.ChatWithConversation)
		authorized.POST("/conversations/:id/regenerate", middleware.RateLimitByUser(redisClient.RDB(), 30, 1*time.Minute), aiHandler.Regenerate)
		authorized.GET("/conversations/:id/suggestions", middleware.RateLimitByUser(redisClient.RDB(), 30, 1*time.Minute), aiSuggestionsHandler.Suggestions)
		authorized.GET("/conversations/:id/plan-drafts", volunteerPlanHandlerV2.ListDraftsByConversation)
		authorized.GET("/plan-drafts/:draft_id", volunteerPlanHandlerV2.GetDraft)
		authorized.GET("/volunteer-plans", volunteerPlanHandlerV2.ListPlans)
		authorized.GET("/volunteer-plans/:id", volunteerPlanHandlerV2.GetPlan)
		authorized.POST("/volunteer-plans/adopt", volunteerPlanHandlerV2.Adopt)
		authorized.POST("/payment/orders", paymentHandler.CreateOrder)
		authorized.GET("/payment/orders", paymentHandler.ListMyOrders)
		authorized.GET("/payment/orders/:order_no", paymentHandler.GetMyOrder)
		authorized.POST("/payment/orders/:order_no/pay", paymentHandler.PayMock)
		authorized.POST("/payment/orders/:order_no/pay/alipay", paymentHandler.PayAlipay)
		authorized.POST("/payment/orders/:order_no/detect", paymentHandler.Detect)
		authorized.POST("/payment/orders/:order_no/refund", paymentHandler.RefundOrder)
		authorized.GET("/payment/orders/:order_no/refunds", paymentHandler.ListRefunds)
		authorized.POST("/admission/recommendations",
			middleware.RateLimitByUser(redisClient.RDB(), 6, 1*time.Minute),
			recommendationHandler.Recommend)

		adminRoutes := authorized.Group("/admin")
		adminRoutes.Use(middleware.RequireAdmin())
		adminRoutes.GET("/users/:id", adminHandler.GetUser)
		adminRoutes.GET("/users", adminHandler.ListUsers)
		adminRoutes.PUT("/users/:id", adminHandler.UpdateUser)
		adminRoutes.PUT("/users/:id/role", adminHandler.UpdateRole)
		adminRoutes.PUT("/users/:id/password", adminHandler.ResetPassword)
		adminRoutes.POST("/users/:id/disable", adminHandler.DisableUser)
		adminRoutes.POST("/users/:id/enable", adminHandler.EnableUser)
		adminRoutes.GET("/stats", adminHandler.GetStats)
		adminRoutes.GET("/payment/orders", paymentHandler.AdminListOrders)
		adminRoutes.GET("/payment/orders/:order_no", paymentHandler.AdminGetOrder)
		adminRoutes.POST("/payment/orders/:order_no/close", paymentHandler.AdminCloseOrder)
		adminRoutes.POST("/payment/orders/:order_no/redetect", paymentHandler.AdminRedetect)
		adminRoutes.POST("/payment/orders/:order_no/regrant-membership", paymentHandler.AdminRegrantMembership)

		// 退款审批
		adminRoutes.GET("/payment/refunds/pending", paymentHandler.ListPendingRefunds)
		adminRoutes.POST("/payment/refunds/:refund_no/approve", paymentHandler.ApproveRefund)
		adminRoutes.POST("/payment/refunds/:refund_no/reject", paymentHandler.RejectRefund)

		adminRoutes.POST("/recommendation/scores/refresh", recommendationScoreHandler.Refresh)
	}

	server := &http.Server{
		Addr:        ":" + cfg.Port,
		Handler:     r,
		ReadTimeout: 15 * time.Second,
		// 10 分钟覆盖 /admin/recommendation/scores/refresh 的最坏批量 (20 行 × 25s)。
		// 业务接口都远低于这个数（最重的推荐接口 ~400ms），所以全局放宽不影响正常路径。
		WriteTimeout: 10 * time.Minute,
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
