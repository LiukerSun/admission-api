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

func TestCandidateIntentionCRUD(t *testing.T) {
	env := setupCandidateIntentionTest(t, "crud")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateIntentionUsers(t, env, []string{
		"cand-intention-owner@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-intention-owner@example.com", "student")
	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")

	// Create a profile first
	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生甲", "province_id": 11}, ownerToken))
	profileID := profile.ID

	// Save school intentions
	savePayload := map[string]any{
		"items": []map[string]any{
			{"target_id": "100", "target_name": "清华大学", "priority": 0},
			{"target_id": "101", "target_name": "北京大学", "priority": 1},
			{"target_id": "102", "target_name": "复旦大学", "priority": 2},
		},
	}
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", savePayload, ownerToken).Code)

	// Save province intentions
	provincePayload := map[string]any{
		"items": []map[string]any{
			{"target_id": "11", "target_name": "北京", "priority": 0},
			{"target_id": "31", "target_name": "上海", "priority": 1},
		},
	}
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/province", provincePayload, ownerToken).Code)

	// Get intentions and verify grouping
	group := candidateExpectOK[candidate.IntentionGroupResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, ownerToken))
	require.Len(t, group.School, 3)
	require.Len(t, group.Province, 2)
	require.Len(t, group.Major, 0)
	require.Equal(t, "清华大学", *group.School[0].TargetName)
	require.Equal(t, "北京大学", *group.School[1].TargetName)
	require.Equal(t, 0, group.School[0].Priority)
	require.Equal(t, 1, group.School[1].Priority)

	// Overwrite school intentions (full replace)
	newSchoolPayload := map[string]any{
		"items": []map[string]any{
			{"target_id": "200", "target_name": "浙江大学", "priority": 0},
		},
	}
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", newSchoolPayload, ownerToken).Code)

	group = candidateExpectOK[candidate.IntentionGroupResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, ownerToken))
	require.Len(t, group.School, 1)
	require.Equal(t, "浙江大学", *group.School[0].TargetName)
	require.Len(t, group.Province, 2, "province intentions should remain untouched")

	// Remove single intention
	intentionID := group.Province[0].ID
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/intentions/"+itoa64(intentionID), nil, ownerToken).Code)

	group = candidateExpectOK[candidate.IntentionGroupResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, ownerToken))
	require.Len(t, group.Province, 1)

	// Clear remaining school intentions
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", nil, ownerToken).Code)

	group = candidateExpectOK[candidate.IntentionGroupResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, ownerToken))
	require.Len(t, group.School, 0)
	require.Len(t, group.Province, 1)
}

func TestCandidateIntentionPermission(t *testing.T) {
	env := setupCandidateIntentionTest(t, "perm")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateIntentionUsers(t, env, []string{
		"cand-intention-student@example.com",
		"cand-intention-parent@example.com",
		"cand-intention-other@example.com",
	})

	studentID := createIntegrationUser(t, env.database, "cand-intention-student@example.com", "student")
	parentID := createIntegrationUser(t, env.database, "cand-intention-parent@example.com", "parent")
	otherID := createIntegrationUser(t, env.database, "cand-intention-other@example.com", "student")

	insertUserBinding(t, env.database, parentID, studentID)

	studentToken := issueAccessToken(t, env.cfg, studentID, "user", "student")
	parentToken := issueAccessToken(t, env.cfg, parentID, "user", "parent")
	otherToken := issueAccessToken(t, env.cfg, otherID, "user", "student")

	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生乙", "province_id": 11}, studentToken))
	profileID := profile.ID

	// Student saves intentions
	savePayload := map[string]any{
		"items": []map[string]any{
			{"target_id": "100", "target_name": "清华大学", "priority": 0},
		},
	}
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", savePayload, studentToken).Code)

	// Bound parent can read
	group := candidateExpectOK[candidate.IntentionGroupResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, parentToken))
	require.Len(t, group.School, 1)

	// Bound parent cannot write
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", savePayload, parentToken).Code,
		"bound parent cannot save intentions")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", nil, parentToken).Code,
		"bound parent cannot clear intentions")

	// Other user cannot read
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, otherToken).Code,
		"unbound third party cannot read intentions")

	// Other user cannot write
	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school", savePayload, otherToken).Code)
}

func TestCandidateIntentionValidation(t *testing.T) {
	env := setupCandidateIntentionTest(t, "validation")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateIntentionUsers(t, env, []string{
		"cand-intention-val@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-intention-val@example.com", "student")
	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")

	profile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生丙", "province_id": 11}, ownerToken))
	profileID := profile.ID

	// Invalid profile_id
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/abc/intentions", nil, ownerToken).Code)

	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/abc/intentions/school",
			map[string]any{"items": []map[string]any{{"target_id": "1", "target_name": "T"}}}, ownerToken).Code)

	// Invalid intention type
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/invalid",
			map[string]any{"items": []map[string]any{{"target_id": "1", "target_name": "T"}}}, ownerToken).Code)

	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/invalid", nil, ownerToken).Code)

	// Invalid intention id for remove
	require.Equal(t, http.StatusBadRequest,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/intentions/abc", nil, ownerToken).Code)

	// Not found intention id for remove
	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodDelete,
			"/api/v1/candidate/intentions/99999", nil, ownerToken).Code)

	// Empty items is allowed (clears the type)
	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions/school",
			map[string]any{"items": []map[string]any{}}, ownerToken).Code)

	// Missing auth
	require.Equal(t, http.StatusUnauthorized,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/intentions", nil, "").Code)
}

// --- shared test fixture ---

type candidateIntentionTestEnv struct {
	ctx      context.Context
	cfg      *config.Config
	database *db.DB
	redis    *redis.Client
	router   *gin.Engine
}

func setupCandidateIntentionTest(t *testing.T, scope string) *candidateIntentionTestEnv {
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

	router := newCandidateIntentionRouter(t, database, redisClient, cfg)
	return &candidateIntentionTestEnv{
		ctx:      ctx,
		cfg:      cfg,
		database: database,
		redis:    redisClient,
		router:   router,
	}
}

func newCandidateIntentionRouter(t *testing.T, database *db.DB, redisClient *redis.Client, cfg *config.Config) *gin.Engine {
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

	// Profile module is needed to create profiles before testing intentions
	cipher, err := candidate.NewIDCardCipher(cfg.CandidateIDCardMasterKey)
	require.NoError(t, err)
	profileStore := candidate.NewProfileStore(database.Pool())
	profileService := candidate.NewProfileService(
		profileStore, bindingStore, userStore, cipher, activityService, redisClient.RDB(), cfg,
	)
	profileHandler := candidate.NewProfileHandler(profileService)

	intentionStore := candidate.NewIntentionStore(database.Pool())
	intentionService := candidate.NewIntentionService(
		intentionStore, profileStore, bindingStore, activityService,
	)
	intentionHandler := candidate.NewIntentionHandler(intentionService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))

	// Profile routes
	authorized.POST("/candidate/profiles", profileHandler.CreateProfile)

	// Intention routes
	authorized.GET("/candidate/profiles/:profile_id/intentions", intentionHandler.GetIntentions)
	authorized.PUT("/candidate/profiles/:profile_id/intentions/:type", intentionHandler.SaveIntentions)
	authorized.DELETE("/candidate/profiles/:profile_id/intentions/:type", intentionHandler.ClearIntentions)
	authorized.DELETE("/candidate/intentions/:id", intentionHandler.RemoveIntention)

	return r
}

func resetCandidateIntentionUsers(t *testing.T, env *candidateIntentionTestEnv, emails []string) {
	t.Helper()
	pool := env.database.Pool()

	_, err := pool.Exec(env.ctx, `
		DELETE FROM candidate_intentions
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
