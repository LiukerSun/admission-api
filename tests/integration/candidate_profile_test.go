//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"admission-api/internal/candidate"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/redis"
	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	candidateTestIDCardMasterKey = "d14f89b3056df76a4c2c55306cea4ffe8dcb0f8c364ed51ed3864763fb80a644"
	idCardSampleA                = "11010119900307123X"
	idCardSampleB                = "44030219950521456X"
	idCardSampleNotFound         = "440302199911234567"
)

func TestCandidateProfileCRUD(t *testing.T) {
	env := setupCandidateProfileTest(t, "crud")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateUsers(t, env, []string{
		"cand-crud-owner@example.com",
		"cand-crud-other@example.com",
		"cand-crud-admin@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-crud-owner@example.com", "student")
	otherID := createIntegrationUser(t, env.database, "cand-crud-other@example.com", "student")
	// DB user_type is constrained to parent|student post-migration-003; admin is a role.
	// Issue a token claiming user_type=admin against a real DB row to exercise the
	// isCandidateOrParent rejection path on the handler side.
	adminID := createIntegrationUser(t, env.database, "cand-crud-admin@example.com", "student")

	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")
	otherToken := issueAccessToken(t, env.cfg, otherID, "user", "student")
	adminToken := issueAccessToken(t, env.cfg, adminID, "admin", "admin")

	cityID := int32(1101)
	countyID := int32(110101)
	createPayload := map[string]any{
		"real_name":         "张三",
		"candidate_id_card": idCardSampleA,
		"candidate_phone":   "13800138000",
		"province_id":       11,
		"city_id":           cityID,
		"county_id":         countyID,
		"grade":             3,
		"candidate_type":    "regular",
		"gender":            "male",
		"ethnicity":         "汉",
	}
	created := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles", createPayload, ownerToken))
	require.NotZero(t, created.ID)
	require.Equal(t, ownerID, created.UserID)
	require.Equal(t, "张三", created.RealName)
	require.True(t, created.CanWrite)
	require.Equal(t, "138****8000", created.PhoneMasked)
	require.NotEmpty(t, created.IDCardMasked, "id card masked placeholder must be set when id card is provided")
	profileID := created.ID

	enc, hash := fetchProfileSecrets(t, env.database, profileID)
	require.NotEmpty(t, enc, "id card encrypted blob must be persisted")
	require.NotNil(t, hash)
	require.Len(t, *hash, 64, "id card hash should be hex sha256")
	originalHash := *hash

	minimalPayload := map[string]any{
		"real_name":   "李四",
		"province_id": 12,
	}
	minimal := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles", minimalPayload, ownerToken))
	require.Empty(t, minimal.IDCardMasked)
	require.Empty(t, minimal.PhoneMasked)
	require.Equal(t, int16(3), minimal.Grade, "grade should default to 3")
	require.Equal(t, "regular", minimal.CandidateType, "candidate_type should default to regular")
	require.Equal(t, "active", minimal.Status, "status should default to active")

	list := candidateExpectOK[candidate.ProfileListResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles", nil, ownerToken))
	require.Equal(t, 2, list.Total)

	got := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles/"+itoa64(profileID), nil, ownerToken))
	require.Equal(t, profileID, got.ID)
	require.True(t, got.CanWrite)

	updatePayload := map[string]any{
		"real_name":         "张三丰",
		"candidate_id_card": idCardSampleB,
	}
	updated := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPut, "/api/v1/candidate/profiles/"+itoa64(profileID), updatePayload, ownerToken))
	require.Equal(t, "张三丰", updated.RealName)
	_, newHash := fetchProfileSecrets(t, env.database, profileID)
	require.NotNil(t, newHash)
	require.NotEqual(t, originalHash, *newHash, "id card hash must change after id card is updated")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut, "/api/v1/candidate/profiles/"+itoa64(profileID), updatePayload, otherToken).Code,
		"non-owner cannot update")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodDelete, "/api/v1/candidate/profiles/"+itoa64(profileID), nil, otherToken).Code,
		"non-owner cannot delete")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles", nil, adminToken).Code,
		"admin user_type is rejected by isCandidateOrParent")

	require.Equal(t, http.StatusUnauthorized,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles", nil, "").Code,
		"missing JWT must return 401")

	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodDelete, "/api/v1/candidate/profiles/"+itoa64(profileID), nil, ownerToken).Code)

	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles/"+itoa64(profileID), nil, ownerToken).Code,
		"soft-deleted profile must not be readable")

	finalList := candidateExpectOK[candidate.ProfileListResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles", nil, ownerToken))
	require.Equal(t, 1, finalList.Total, "deleted profile should drop out of the list")
}

func TestCandidateProfileLookup(t *testing.T) {
	env := setupCandidateProfileTest(t, "lookup")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateUsers(t, env, []string{
		"cand-lookup-owner@example.com",
		"cand-lookup-finder@example.com",
		"cand-lookup-admin@example.com",
	})

	ownerID := createIntegrationUser(t, env.database, "cand-lookup-owner@example.com", "student")
	finderID := createIntegrationUser(t, env.database, "cand-lookup-finder@example.com", "parent")
	// See note in TestCandidateProfileCRUD: DB user_type is constrained to parent|student;
	// admin user_type is exercised via the JWT claim.
	adminID := createIntegrationUser(t, env.database, "cand-lookup-admin@example.com", "student")

	ownerToken := issueAccessToken(t, env.cfg, ownerID, "user", "student")
	finderToken := issueAccessToken(t, env.cfg, finderID, "user", "parent")
	adminToken := issueAccessToken(t, env.cfg, adminID, "admin", "admin")

	createPayload := map[string]any{
		"real_name":         "王小明",
		"candidate_id_card": idCardSampleA,
		"candidate_phone":   "13912345678",
		"province_id":       11,
	}
	created := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles", createPayload, ownerToken))
	profileID := created.ID

	idCardLookup := candidateExpectOK[candidate.LookupResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/idcard",
			map[string]string{"id_card": idCardSampleA}, finderToken))
	require.Equal(t, profileID, idCardLookup.ProfileID)
	require.Equal(t, ownerID, idCardLookup.OwnerUserID)
	require.Equal(t, "cand-lookup-owner@example.com", idCardLookup.OwnerEmail)
	require.Equal(t, "student", idCardLookup.OwnerUserType)
	require.Equal(t, "王*", idCardLookup.RealNameMasked)
	require.Equal(t, "139****5678", idCardLookup.PhoneMasked)

	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/idcard",
			map[string]string{"id_card": idCardSampleNotFound}, finderToken).Code)

	phoneLookup := candidateExpectOK[candidate.LookupResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/phone",
			map[string]string{"phone": "13912345678"}, finderToken))
	require.Equal(t, profileID, phoneLookup.ProfileID)
	require.Equal(t, "139****5678", phoneLookup.PhoneMasked)

	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/phone",
			map[string]string{"phone": "13800000000"}, finderToken).Code)

	invite := candidateExpectOK[candidate.InviteResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/invite-code", nil, ownerToken))
	require.Len(t, invite.Code, 6)

	codeLookup := candidateExpectOK[candidate.LookupResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/code",
			map[string]string{"code": invite.Code}, finderToken))
	require.Equal(t, profileID, codeLookup.ProfileID)

	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/code",
			map[string]string{"code": invite.Code}, finderToken).Code,
		"invite code must be single-use")

	exists, err := env.redis.RDB().Exists(env.ctx, "candidate:bind:code:"+invite.Code).Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), exists, "consumed invite code must be removed from Redis")

	c1 := candidateExpectOK[candidate.InviteResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/invite-code", nil, ownerToken))
	c2 := candidateExpectOK[candidate.InviteResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/profiles/"+itoa64(profileID)+"/invite-code", nil, ownerToken))
	if c1.Code == c2.Code {
		t.Skip("rare random collision between two generated invite codes; skip overwrite assertion")
	}
	require.Equal(t, http.StatusNotFound,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/code",
			map[string]string{"code": c1.Code}, finderToken).Code,
		"old invite code must be invalidated when a new one is generated")
	freshLookup := candidateExpectOK[candidate.LookupResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/code",
			map[string]string{"code": c2.Code}, finderToken))
	require.Equal(t, profileID, freshLookup.ProfileID)

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles/lookup/idcard",
			map[string]string{"id_card": idCardSampleA}, adminToken).Code,
		"admin user_type cannot lookup")
}

func TestCandidateProfileBindingAccess(t *testing.T) {
	env := setupCandidateProfileTest(t, "bind")
	defer env.database.Close()
	defer env.redis.Close()

	resetCandidateUsers(t, env, []string{
		"cand-bind-student@example.com",
		"cand-bind-parent@example.com",
		"cand-bind-other@example.com",
	})

	studentID := createIntegrationUser(t, env.database, "cand-bind-student@example.com", "student")
	parentID := createIntegrationUser(t, env.database, "cand-bind-parent@example.com", "parent")
	otherID := createIntegrationUser(t, env.database, "cand-bind-other@example.com", "student")

	insertUserBinding(t, env.database, parentID, studentID)

	studentToken := issueAccessToken(t, env.cfg, studentID, "user", "student")
	parentToken := issueAccessToken(t, env.cfg, parentID, "user", "parent")
	otherToken := issueAccessToken(t, env.cfg, otherID, "user", "student")

	studentProfile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "考生甲", "province_id": 11}, studentToken))

	parentProfile := candidateExpectOK[candidate.ProfileResponse](t,
		performCandidateRequest(t, env.router, http.MethodPost, "/api/v1/candidate/profiles",
			map[string]any{"real_name": "家长档案", "province_id": 11}, parentToken))

	parentList := candidateExpectOK[candidate.ProfileListResponse](t,
		performCandidateRequest(t, env.router, http.MethodGet, "/api/v1/candidate/profiles", nil, parentToken))
	require.Equal(t, 2, parentList.Total, "parent should see own + bound student's profile")
	canWrite := map[int64]bool{}
	for _, p := range parentList.Profiles {
		canWrite[p.ID] = p.CanWrite
	}
	require.True(t, canWrite[parentProfile.ID], "parent owns parent profile")
	require.False(t, canWrite[studentProfile.ID], "parent does not own student profile")

	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(parentProfile.ID), nil, studentToken).Code,
		"bound student can read parent's profile")

	require.Equal(t, http.StatusOK,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(studentProfile.ID), nil, parentToken).Code,
		"bound parent can read student's profile")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodGet,
			"/api/v1/candidate/profiles/"+itoa64(parentProfile.ID), nil, otherToken).Code,
		"unbound third party cannot read")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPut,
			"/api/v1/candidate/profiles/"+itoa64(studentProfile.ID),
			map[string]any{"real_name": "改名"}, parentToken).Code,
		"bound parent cannot update student's profile (only owner can write)")

	require.Equal(t, http.StatusForbidden,
		performCandidateRequest(t, env.router, http.MethodPost,
			"/api/v1/candidate/profiles/"+itoa64(studentProfile.ID)+"/invite-code", nil, parentToken).Code,
		"only profile owner can generate invite code")
}

// --- shared test fixture ---

type candidateTestEnv struct {
	ctx      context.Context
	cfg      *config.Config
	database *db.DB
	redis    *redis.Client
	router   *gin.Engine
}

func setupCandidateProfileTest(t *testing.T, scope string) *candidateTestEnv {
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

	// cleanupCandidateTestArtifacts(t, ctx, database, redisClient, scope)

	router := newCandidateProfileRouter(t, database, redisClient, cfg)
	return &candidateTestEnv{
		ctx:      ctx,
		cfg:      cfg,
		database: database,
		redis:    redisClient,
		router:   router,
	}
}

func newCandidateProfileRouter(t *testing.T, database *db.DB, redisClient *redis.Client, cfg *config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtConfig := &middleware.JWTConfig{
		Secret:     cfg.JWTSecret,
		AccessTTL:  0,
		RefreshTTL: 0,
	}

	cipher, err := candidate.NewIDCardCipher(cfg.CandidateIDCardMasterKey)
	require.NoError(t, err)

	userStore := user.NewStore(database.Pool())
	bindingStore := user.NewBindingStore(database.Pool())
	activityStore := candidate.NewActivityLogStore(database.Pool())
	activityService := candidate.NewActivityLogService(activityStore, redisClient.RDB(), true)

	profileStore := candidate.NewProfileStore(database.Pool())
	profileService := candidate.NewProfileService(
		profileStore, bindingStore, userStore, cipher, activityService, redisClient.RDB(), cfg,
	)
	profileHandler := candidate.NewProfileHandler(profileService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Platform)

	api := r.Group("/api/v1")
	authorized := api.Group("")
	authorized.Use(middleware.JWTMiddleware(jwtConfig))

	authorized.GET("/candidate/profiles", profileHandler.GetMyProfiles)
	authorized.POST("/candidate/profiles", profileHandler.CreateProfile)
	authorized.GET("/candidate/profiles/:id", profileHandler.GetProfile)
	authorized.PUT("/candidate/profiles/:id", profileHandler.UpdateProfile)
	authorized.DELETE("/candidate/profiles/:id", profileHandler.DeleteProfile)
	authorized.POST("/candidate/profiles/lookup/idcard", profileHandler.LookupByIDCard)
	authorized.POST("/candidate/profiles/lookup/phone", profileHandler.LookupByPhone)
	authorized.POST("/candidate/profiles/lookup/code", profileHandler.LookupByCode)
	authorized.POST("/candidate/profiles/:id/invite-code", profileHandler.GenerateInviteCode)

	return r
}

func performCandidateRequest(t *testing.T, router http.Handler, method, path string, payload any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(payload))
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("X-Platform", "web")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func candidateExpectOK[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equalf(t, http.StatusOK, rec.Code, "unexpected status; body=%s", rec.Body.String())

	var envelope web.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equalf(t, 0, envelope.Code, "expected success envelope, got %+v", envelope)

	var out T
	dataBytes, err := json.Marshal(envelope.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(dataBytes, &out))
	return out
}

// resetCandidateUsers wipes any leftover rows from prior runs for the given test
// emails: dependent profiles, bindings, and the user rows themselves.
func resetCandidateUsers(t *testing.T, env *candidateTestEnv, emails []string) {
	t.Helper()
	pool := env.database.Pool()

	_, err := pool.Exec(env.ctx, `
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

func fetchProfileSecrets(t *testing.T, database *db.DB, profileID int64) ([]byte, *string) {
	t.Helper()
	var enc []byte
	var hash *string
	err := database.Pool().QueryRow(context.Background(),
		`SELECT candidate_id_card_enc, candidate_id_card_hash FROM candidate_profiles WHERE id = $1`,
		profileID,
	).Scan(&enc, &hash)
	require.NoError(t, err)
	return enc, hash
}

func insertUserBinding(t *testing.T, database *db.DB, parentID, studentID int64) {
	t.Helper()
	_, err := database.Pool().Exec(context.Background(),
		`INSERT INTO user_bindings (parent_id, student_id) VALUES ($1, $2)
		 ON CONFLICT (student_id) DO UPDATE SET parent_id = EXCLUDED.parent_id`,
		parentID, studentID,
	)
	require.NoError(t, err)
}

func cleanupCandidateTestArtifacts(t *testing.T, ctx context.Context, database *db.DB, redisClient *redis.Client, scope string) {
	t.Helper()

	emailLike := fmt.Sprintf("cand-%s-%%@example.com", scope)
	_, err := database.Pool().Exec(ctx, `
		DELETE FROM candidate_profiles
		WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)
	`, emailLike)
	require.NoError(t, err)

	_, err = database.Pool().Exec(ctx, `
		DELETE FROM user_bindings
		WHERE parent_id IN (SELECT id FROM users WHERE email LIKE $1)
		   OR student_id IN (SELECT id FROM users WHERE email LIKE $1)
	`, emailLike)
	require.NoError(t, err)

	_, err = database.Pool().Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, emailLike)
	require.NoError(t, err)

	rdb := redisClient.RDB()
	for _, pattern := range []string{"candidate:bind:code:*", "candidate:bind:profile:*"} {
		keys, err := rdb.Keys(ctx, pattern).Result()
		require.NoError(t, err)
		if len(keys) > 0 {
			require.NoError(t, rdb.Del(ctx, keys...).Err())
		}
	}
}
