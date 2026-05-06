//go:build integration

package integration

import (
	"context"
	"net/http"
	"os"
	"testing"

	"admission-api/internal/candidate"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCandidateExamRecordCRUD(t *testing.T) {
	env := setupCandidateExamRecordTest(t, "crud")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateExamRecordUsers(t, env, []string{
		"cand-exam-owner@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-exam-owner@example.com", "student")
	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")

	// Create a profile first
	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生甲", "province_id": 11}, ownerToken))
	profileID := profile.ID

	// Create exam record
	createPayload := map[string]any{
		"exam_year":   2026,
		"exam_model":  "3+1+2",
		"total_score": 650,
		"rank_value":  5000,
		"section_type": "physics",
		"select_subjects": []string{"physics", "chemistry", "biology"},
		"subject_scores": map[string]float64{"语文": 120, "数学": 130, "外语": 125, "physics": 90, "chemistry": 92, "biology": 93},
	}
	created := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), createPayload, ownerToken))
	require.NotZero(t, created.ID)
	require.Equal(t, int64(profileID), created.ProfileID)
	require.Equal(t, int16(2026), created.ExamYear)
	require.Equal(t, "3+1+2", created.ExamModel)
	require.True(t, created.IsCurrent)
	require.True(t, created.CanWrite)
	recordID := created.ID

	// List by profile
	list := candidateExpectOK[[]*candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, ownerToken))
	require.Len(t, list, 1)
	require.Equal(t, recordID, list[0].ID)

	// Get by ID
	got := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(recordID), nil, ownerToken))
	require.Equal(t, recordID, got.ID)
	require.Equal(t, 650.0, *got.TotalScore)
	require.Equal(t, int32(5000), *got.RankValue)

	// Update basic info
	updatePayload := map[string]any{
		"exam_year":  2027,
		"exam_model": "3+3",
	}
	updated := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/"+itoa64(recordID), updatePayload, ownerToken))
	require.Equal(t, int16(2027), updated.ExamYear)
	require.Equal(t, "3+3", updated.ExamModel)
	require.Equal(t, 650.0, *updated.TotalScore, "total_score should remain unchanged")

	// Update scores (triggers history) via unified update endpoint
	scoresPayload := map[string]any{
		"total_score":    660,
		"rank_value":     4800,
		"subject_scores": map[string]float64{"语文": 125, "数学": 135, "外语": 130, "physics": 90, "chemistry": 92, "biology": 93},
		"change_reason":  "复查后修正",
	}
	scored := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/"+itoa64(recordID), scoresPayload, ownerToken))
	require.Equal(t, 660.0, *scored.TotalScore)
	require.Equal(t, int32(4800), *scored.RankValue)

	// List score histories
	histories := candidateExpectOK[[]*candidate.ScoreHistoryResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(recordID)+"/score-histories", nil, ownerToken))
	require.Len(t, histories, 1)
	require.Equal(t, 650.0, *histories[0].PrevTotalScore)
	require.Equal(t, int32(5000), *histories[0].PrevRankValue)
	require.Equal(t, 660.0, *histories[0].NewTotalScore)
	require.Equal(t, int32(4800), *histories[0].NewRankValue)
	require.Equal(t, "复查后修正", histories[0].ChangeReason)

	// Create second record (should auto set first to not current)
	createPayload2 := map[string]any{
		"exam_year":  2025,
		"exam_model": "wenli",
		"total_score": 640,
		"rank_value":  6000,
	}
	created2 := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), createPayload2, ownerToken))
	require.NotZero(t, created2.ID)
	require.True(t, created2.IsCurrent)

	// Verify first record is no longer current
	list = candidateExpectOK[[]*candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, ownerToken))
	require.Len(t, list, 2)
	currentCount := 0
	for _, r := range list {
		if r.IsCurrent {
			currentCount++
		}
	}
	require.Equal(t, 1, currentCount, "only one record should be current")

	// Void the second record
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/exam-records/"+itoa64(created2.ID), nil, ownerToken).Code)

	// Verify voided record is gone from list
	list = candidateExpectOK[[]*candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, ownerToken))
	require.Len(t, list, 1)
	require.Equal(t, recordID, list[0].ID)

	// Verify voided record returns 404
	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(created2.ID), nil, ownerToken).Code)
}

func TestCandidateExamRecordCreate(t *testing.T) {
	env := setupCandidateExamRecordTest(t, "create")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateExamRecordUsers(t, env, []string{
		"cand-exam-create@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-exam-create@example.com", "student")
	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")

	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生 create", "province_id": 11}, ownerToken))
	profileID := profile.ID

	// Minimal creation (only required fields)
	minimal := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_year": 2026, "exam_model": "3+1+2"}, ownerToken))
	require.NotZero(t, minimal.ID)
	require.Equal(t, int16(2026), minimal.ExamYear)
	require.Equal(t, "3+1+2", minimal.ExamModel)
	require.True(t, minimal.IsCurrent)
	require.True(t, minimal.CanWrite)
	require.Nil(t, minimal.TotalScore)
	require.Nil(t, minimal.RankValue)

	// Full creation with scores and art type
	fullPayload := map[string]any{
		"exam_year":      2025,
		"exam_model":     "wenli",
		"exam_type":      "yikao",
		"total_score":    620,
		"rank_value":     8000,
		"section_type":   "liberal_arts",
		"select_subjects": []string{"history", "politics", "geography"},
		"subject_scores":  map[string]float64{"语文": 110, "数学": 100, "外语": 105, "history": 85, "politics": 80, "geography": 78},
		"art_score":      240,
		"culture_score":  380,
		"art_type":       "meishu",
	}
	full := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), fullPayload, ownerToken))
	require.NotZero(t, full.ID)
	require.True(t, full.IsCurrent, "newly created record should be current")
	require.Equal(t, 620.0, *full.TotalScore)
	require.Equal(t, int32(8000), *full.RankValue)
	require.Equal(t, "yikao", full.ExamType)
	require.Equal(t, "liberal_arts", full.SectionType)
	require.Equal(t, "meishu", full.ArtType)
	require.Equal(t, 240.0, *full.ArtScore)
	require.Equal(t, 380.0, *full.CultureScore)
	require.Len(t, full.SelectSubjects, 3)
	require.Len(t, full.SubjectScores, 6)

	// Verify the first record is no longer current
	list := candidateExpectOK[[]*candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, ownerToken))
	require.Len(t, list, 2)
	currentCount := 0
	for _, r := range list {
		if r.IsCurrent {
			currentCount++
		}
	}
	require.Equal(t, 1, currentCount, "only one record should be current after second creation")

	// Verify only one record per profile can be current at a time
	got := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(minimal.ID), nil, ownerToken))
	require.False(t, got.IsCurrent, "first record should no longer be current")

	// Invalid exam_model should be rejected
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_year": 2024, "exam_model": "invalid_model"}, ownerToken).Code)

	// Missing exam_year should be rejected
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_model": "3+3"}, ownerToken).Code)

	// Out of range total_score should be rejected
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_year": 2024, "exam_model": "3+3", "total_score": 1500}, ownerToken).Code)
}

func TestCandidateExamRecordPermission(t *testing.T) {
	env := setupCandidateExamRecordTest(t, "perm")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateExamRecordUsers(t, env, []string{
		"cand-exam-student@example.com",
		"cand-exam-parent@example.com",
		"cand-exam-other@example.com",
	})

	studentID := createIntegrationUser(t, env.database, "cand-exam-student@example.com", "student")
	parentID := createIntegrationUser(t, env.database, "cand-exam-parent@example.com", "parent")
	otherID := createIntegrationUser(t, env.database, "cand-exam-other@example.com", "student")

	insertUserBinding(t, env.database, parentID, studentID)

	studentToken := issueAccessToken(t, env.cfg, studentID, "user", "student")
	parentToken := issueAccessToken(t, env.cfg, parentID, "user", "parent")
	otherToken := issueAccessToken(t, env.cfg, otherID, "user", "student")

	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生乙", "province_id": 11}, studentToken))
	profileID := profile.ID

	// Student creates exam record
	createPayload := map[string]any{
		"exam_year":  2026,
		"exam_model": "3+1+2",
		"total_score": 650,
		"rank_value":  5000,
	}
	created := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), createPayload, studentToken))
	recordID := created.ID

	// Bound parent can read list
	list := candidateExpectOK[[]*candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, parentToken))
	require.Len(t, list, 1)
	require.False(t, list[0].CanWrite, "parent should not have write permission")

	// Bound parent can read single record
	got := candidateExpectOK[candidate.ExamRecordResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(recordID), nil, parentToken))
	require.False(t, got.CanWrite)

	// Bound parent can read score histories
	histories := candidateExpectOK[[]*candidate.ScoreHistoryResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/"+itoa64(recordID)+"/score-histories", nil, parentToken))
	require.Len(t, histories, 0)

	// Bound parent cannot create
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), createPayload, parentToken).Code,
		"bound parent cannot create exam record")

	// Bound parent cannot update
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/"+itoa64(recordID),
			map[string]any{"exam_year": 2027}, parentToken).Code,
		"bound parent cannot update exam record")

	// Bound parent cannot update scores (unified endpoint)
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/"+itoa64(recordID),
			map[string]any{"total_score": 660, "rank_value": 4800}, parentToken).Code,
		"bound parent cannot update scores")

	// Bound parent cannot void
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/exam-records/"+itoa64(recordID), nil, parentToken).Code,
		"bound parent cannot void exam record")

	// Other user cannot read
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, otherToken).Code,
		"unbound third party cannot read")

	// Other user cannot create
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), createPayload, otherToken).Code)
}

func TestCandidateExamRecordValidation(t *testing.T) {
	env := setupCandidateExamRecordTest(t, "validation")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateExamRecordUsers(t, env, []string{
		"cand-exam-val@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-exam-val@example.com", "student")
	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")

	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生丙", "province_id": 11}, ownerToken))
	profileID := profile.ID

	// Invalid exam model
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_year": 2026, "exam_model": "invalid"}, ownerToken).Code)

	// Missing required fields
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID),
			map[string]any{"exam_model": "3+1+2"}, ownerToken).Code)

	// Invalid profile_id in path
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/abc", nil, ownerToken).Code)

	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/exam-records/by_profile_id/abc",
			map[string]any{"exam_year": 2026, "exam_model": "3+1+2"}, ownerToken).Code)

	// Invalid record id
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/abc", nil, ownerToken).Code)

	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/abc",
			map[string]any{"exam_year": 2027}, ownerToken).Code)

	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/exam-records/abc", nil, ownerToken).Code)

	// Not found record id
	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/99999", nil, ownerToken).Code)

	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/exam-records/99999",
			map[string]any{"exam_year": 2027}, ownerToken).Code)

	// Missing auth
	require.Equal(t, http.StatusUnauthorized,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/exam-records/by_profile_id/"+itoa64(profileID), nil, "").Code)
}

// --- shared test fixture ---

type candidateExamRecordTestEnv struct {
	ctx      context.Context
	cfg      *config.Config
	database *db.DB
	redis    *redis.Client
	router   *gin.Engine
}

func setupCandidateExamRecordTest(t *testing.T, scope string) *candidateExamRecordTestEnv {
	t.Helper()
	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "integration-test-secret")
	}
	if os.Getenv("CANDIDATE_IDCARD_MASTER_KEY") == "" {
		t.Setenv("CANDIDATE_IDCARD_MASTER_KEY", candidateTestIDCardMasterKey)
	}

	ctx := context.Background()
	cfg, err := config.Load()
	require.NoError(t, err)

	database, err := db.New(ctx, cfg.DatabaseURL)
	require.NoError(t, err)

	redisClient, err := redis.New(cfg.RedisAddr)
	require.NoError(t, err)

	router := newCandidateExamRecordRouter(t, database, redisClient, cfg)
	return &candidateExamRecordTestEnv{
		ctx:      ctx,
		cfg:      cfg,
		database: database,
		redis:    redisClient,
		router:   router,
	}
}

func newCandidateExamRecordRouter(t *testing.T, database *db.DB, redisClient *redis.Client, cfg *config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  0,
		RefreshTTL: 0,
	}

	userStore := user.NewStore(database.Pool())
	bindingStore := user.NewBindingStore(database.Pool())
	activityStore := candidate.NewActivityLogStore(database.Pool())
	activityService := candidate.NewActivityLogService(activityStore, redisClient.RDB(), true)

	// Profile module is needed to create profiles before testing exam records
	cipher, err := candidate.NewIDCardCipher(cfg.CandidateIDCardMasterKey)
	require.NoError(t, err)
	profileStore := candidate.NewProfileStore(database.Pool())
	profileService := candidate.NewProfileService(
		profileStore, bindingStore, userStore, cipher, activityService, redisClient.RDB(), cfg,
	)
	profileHandler := candidate.NewProfileHandler(profileService)

	// Exam record module
	examRecordStore := candidate.NewExamRecordStore(database.Pool())
	scoreHistoryStore := candidate.NewScoreHistoryStore(database.Pool())
	examRecordService := candidate.NewExamRecordService(
		examRecordStore, scoreHistoryStore, profileStore, bindingStore, activityService,
	)
	examRecordHandler := candidate.NewExamRecordHandler(examRecordService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))

	// Profile routes
	authorized.POST("/candidate/profiles", profileHandler.CreateProfile)

	// Exam record routes
	authorized.GET("/candidate/exam-records/by_profile_id/:profile_id", examRecordHandler.ListByProfile)
	authorized.POST("/candidate/exam-records/by_profile_id/:profile_id", examRecordHandler.Create)
	authorized.GET("/candidate/exam-records/:id", examRecordHandler.GetByID)
	authorized.PUT("/candidate/exam-records/:id", examRecordHandler.Update)
	authorized.DELETE("/candidate/exam-records/:id", examRecordHandler.Void)
	authorized.GET("/candidate/exam-records/:id/score-histories", examRecordHandler.ListScoreHistories)

	return r
}

func resetCandidateExamRecordUsers(t *testing.T, env *candidateExamRecordTestEnv, emails []string) {
	t.Helper()
	pool := env.database.Pool()

	_ = env.redis.RDB().Del(env.ctx, "activity_log:queue").Err()

	_, err := pool.Exec(env.ctx, `
		DELETE FROM candidate_activity_logs
		WHERE user_id IN (SELECT id FROM users WHERE email = ANY($1))
	`, emails)
	require.NoError(t, err)

	_, err = pool.Exec(env.ctx, `
		DELETE FROM candidate_score_histories
		WHERE exam_record_id IN (
			SELECT id FROM candidate_exam_records
			WHERE profile_id IN (SELECT id FROM candidate_profiles WHERE user_id IN (SELECT id FROM users WHERE email = ANY($1)))
		)
	`, emails)
	require.NoError(t, err)

	_, err = pool.Exec(env.ctx, `
		DELETE FROM candidate_exam_records
		WHERE profile_id IN (SELECT id FROM candidate_profiles WHERE user_id IN (SELECT id FROM users WHERE email = ANY($1)))
	`, emails)
	require.NoError(t, err)

	_, err = pool.Exec(env.ctx, `
		DELETE FROM candidate_profiles
		WHERE user_id IN (SELECT id FROM users WHERE email = ANY($1))
	`, emails)
	require.NoError(t, err)

	_, err = pool.Exec(env.ctx, `
		DELETE FROM user_bindings
		WHERE parent_id IN (SELECT id FROM users WHERE email = ANY($1))
		   OR student_id IN (SELECT id FROM users WHERE email = ANY($1))
	`, emails)
	require.NoError(t, err)

	_, err = pool.Exec(env.ctx, `DELETE FROM users WHERE email = ANY($1)`, emails)
	require.NoError(t, err)
}
