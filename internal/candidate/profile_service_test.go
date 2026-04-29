package candidate

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"admission-api/internal/platform/config"
	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockProfileStore struct {
	mock.Mock
}

func (m *mockProfileStore) Create(ctx context.Context, in *CreateProfileInput) (*Profile, error) {
	args := m.Called(ctx, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Profile), args.Error(1)
}
func (m *mockProfileStore) GetByID(ctx context.Context, id int64) (*Profile, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Profile), args.Error(1)
}
func (m *mockProfileStore) ListByOwner(ctx context.Context, ownerUserID int64) ([]*Profile, error) {
	args := m.Called(ctx, ownerUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Profile), args.Error(1)
}
func (m *mockProfileStore) ListByOwnerOrBoundUser(ctx context.Context, userID int64) ([]*Profile, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Profile), args.Error(1)
}
func (m *mockProfileStore) Update(ctx context.Context, id int64, in *UpdateProfileInput) (*Profile, error) {
	args := m.Called(ctx, id, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Profile), args.Error(1)
}
func (m *mockProfileStore) SoftDelete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockProfileStore) GetByIDCardHash(ctx context.Context, hash string) (*Profile, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Profile), args.Error(1)
}
func (m *mockProfileStore) GetByPhone(ctx context.Context, phone string) (*Profile, error) {
	args := m.Called(ctx, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Profile), args.Error(1)
}
func (m *mockProfileStore) GetOwnerUserID(ctx context.Context, profileID int64) (int64, error) {
	args := m.Called(ctx, profileID)
	return args.Get(0).(int64), args.Error(1)
}

type mockBindingStore struct{ mock.Mock }

func (m *mockBindingStore) CreateBinding(ctx context.Context, parentID, studentID int64) (*user.Binding, error) {
	args := m.Called(ctx, parentID, studentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Binding), args.Error(1)
}
func (m *mockBindingStore) GetBindingsByParent(ctx context.Context, parentID int64) ([]*user.Binding, error) {
	args := m.Called(ctx, parentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*user.Binding), args.Error(1)
}
func (m *mockBindingStore) GetBindingByStudent(ctx context.Context, studentID int64) (*user.Binding, error) {
	args := m.Called(ctx, studentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Binding), args.Error(1)
}
func (m *mockBindingStore) DeleteBinding(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

type mockUserStore struct{ mock.Mock }

func (m *mockUserStore) Create(ctx context.Context, email, passwordHash, role, userType string) (*user.User, error) {
	args := m.Called(ctx, email, passwordHash, role, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) GetByID(ctx context.Context, id int64) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) GetByEmailAndType(ctx context.Context, email, userType string) (*user.User, error) {
	args := m.Called(ctx, email, userType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) GetByPhone(ctx context.Context, phone string) (*user.User, error) {
	args := m.Called(ctx, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *mockUserStore) ListUsers(ctx context.Context, filter user.Filter, page, pageSize int) ([]*user.User, int64, error) {
	args := m.Called(ctx, filter, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*user.User), args.Get(1).(int64), args.Error(2)
}
func (m *mockUserStore) UpdateRole(ctx context.Context, id int64, role string) error {
	return m.Called(ctx, id, role).Error(0)
}
func (m *mockUserStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	return m.Called(ctx, id, status).Error(0)
}
func (m *mockUserStore) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	return m.Called(ctx, id, passwordHash).Error(0)
}
func (m *mockUserStore) UpdatePhone(ctx context.Context, id int64, phone string) error {
	return m.Called(ctx, id, phone).Error(0)
}
func (m *mockUserStore) UpdateUser(ctx context.Context, id int64, fields user.UpdateUserFields) error {
	return m.Called(ctx, id, fields).Error(0)
}

// mockActivityLogService is declared in activity_log_handler_test.go and reused here.

// --- Test helpers ---

func newTestService(t *testing.T) (*profileService, *mockProfileStore, *mockBindingStore, *mockUserStore, *mockActivityLogService, *miniredis.Miniredis) {
	t.Helper()
	store := &mockProfileStore{}
	bind := &mockBindingStore{}
	users := &mockUserStore{}
	logSvc := &mockActivityLogService{}
	logSvc.On("LogActivity", mock.Anything, mock.Anything).Return(nil).Maybe()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	cipher, err := NewIDCardCipher(testMasterKey)
	require.NoError(t, err)

	cfg := &config.Config{CandidateBindCodeTTLHours: 24}

	svc := &profileService{
		store:        store,
		bindingStore: bind,
		userStore:    users,
		cipher:       cipher,
		activityLog:  logSvc,
		rdb:          rdb,
		cfg:          cfg,
	}
	return svc, store, bind, users, logSvc, mr
}

func samplePhone(s string) *string { return &s }

// --- Tests ---

func TestCreateProfile_EncryptsIDCardAndComputesHash(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("Create", ctx, mock.MatchedBy(func(in *CreateProfileInput) bool {
		// idcard must be encrypted (not equal plaintext) and hash present
		return len(in.CandidateIDCardEnc) > gcmNonceBytes &&
			in.CandidateIDCardHash != nil &&
			len(*in.CandidateIDCardHash) == 64
	})).Return(&Profile{ID: 1, UserID: 10, RealName: "张三", ProvinceID: 11, Status: "active"}, nil)

	resp, err := svc.CreateProfile(ctx, 10, CreateProfileRequest{
		RealName:        "张三",
		CandidateIDCard: "110105200001011234",
		ProvinceID:      11,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ID)
	assert.True(t, resp.CanWrite)
	store.AssertExpectations(t)
}

func TestGetProfile_OwnerCanRead(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&Profile{ID: 1, UserID: 10, ProvinceID: 11, Status: "active"}, nil)

	resp, err := svc.GetProfile(ctx, 10, 1, "parent")
	require.NoError(t, err)
	assert.True(t, resp.CanWrite)
}

func TestGetProfile_BoundUserCanReadButNotWrite(t *testing.T) {
	svc, store, bind, _, _, _ := newTestService(t)
	ctx := context.Background()

	// Profile owned by user 10 (parent). Caller is user 20 (student bound to parent 10).
	store.On("GetByID", ctx, int64(1)).Return(&Profile{ID: 1, UserID: 10, ProvinceID: 11, Status: "active"}, nil)
	bind.On("GetBindingByStudent", ctx, int64(20)).Return(&user.Binding{ID: 1, ParentID: 10, StudentID: 20}, nil)

	resp, err := svc.GetProfile(ctx, 20, 1, "student")
	require.NoError(t, err)
	assert.False(t, resp.CanWrite)
}

func TestGetProfile_StrangerForbidden(t *testing.T) {
	svc, store, bind, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetByID", ctx, int64(1)).Return(&Profile{ID: 1, UserID: 10, ProvinceID: 11, Status: "active"}, nil)
	bind.On("GetBindingByStudent", ctx, int64(99)).Return(nil, user.ErrBindingNotFound)
	bind.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)

	_, err := svc.GetProfile(ctx, 99, 1, "parent")
	require.Error(t, err)
	appErr, ok := err.(*web.AppError)
	require.True(t, ok)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestGetProfile_AdminBlocked(t *testing.T) {
	svc, _, _, _, _, _ := newTestService(t)
	_, err := svc.GetProfile(context.Background(), 1, 1, "admin")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestUpdateProfile_OnlyOwner(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	_, err := svc.UpdateProfile(ctx, 99, 1, UpdateProfileRequest{})
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestUpdateProfile_OwnerCanUpdate(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	newName := "李四"
	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Update", ctx, int64(1), mock.MatchedBy(func(in *UpdateProfileInput) bool {
		return in.RealName != nil && *in.RealName == "李四" && !in.UpdateIDCardFields
	})).Return(&Profile{ID: 1, UserID: 10, RealName: "李四", ProvinceID: 11, Status: "active"}, nil)

	resp, err := svc.UpdateProfile(ctx, 10, 1, UpdateProfileRequest{RealName: &newName})
	require.NoError(t, err)
	assert.Equal(t, "李四", resp.RealName)
}

func TestUpdateProfile_IDCardChangeReencryptsAndRehashes(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	idcard := "110105200001011234"
	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("Update", ctx, int64(1), mock.MatchedBy(func(in *UpdateProfileInput) bool {
		return in.UpdateIDCardFields &&
			len(in.CandidateIDCardEnc) > gcmNonceBytes &&
			in.CandidateIDCardHash != nil
	})).Return(&Profile{ID: 1, UserID: 10, ProvinceID: 11, Status: "active"}, nil)

	_, err := svc.UpdateProfile(ctx, 10, 1, UpdateProfileRequest{CandidateIDCard: &idcard})
	require.NoError(t, err)
}

func TestDeleteProfile_OnlyOwnerSoftDeletes(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	store.On("SoftDelete", ctx, int64(1)).Return(nil)

	err := svc.DeleteProfile(ctx, 10, 1)
	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestDeleteProfile_NonOwnerForbidden(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)

	err := svc.DeleteProfile(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestLookup_AdminForbidden(t *testing.T) {
	svc, _, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	_, err1 := svc.LookupByIDCard(ctx, "admin", "110105200001011234")
	_, err2 := svc.LookupByPhone(ctx, "admin", "13800138000")
	_, err3 := svc.LookupByCode(ctx, "admin", "123456")
	for _, e := range []error{err1, err2, err3} {
		require.Error(t, e)
		appErr, _ := e.(*web.AppError)
		assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
	}
}

func TestLookupByIDCard_HitReturnsMaskedPayload(t *testing.T) {
	svc, store, _, users, _, _ := newTestService(t)
	ctx := context.Background()

	idcard := "110105200001011234"
	hash := svc.cipher.Hash(idcard)
	phone := "13800138000"
	store.On("GetByIDCardHash", ctx, hash).Return(&Profile{
		ID: 1, UserID: 10, RealName: "张三", CandidatePhone: &phone, ProvinceID: 11, Status: "active",
	}, nil)
	users.On("GetByID", ctx, int64(10)).Return(&user.User{ID: 10, Email: "p@example.com", UserType: "parent"}, nil)

	resp, err := svc.LookupByIDCard(ctx, "student", idcard)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ProfileID)
	assert.Equal(t, "p@example.com", resp.OwnerEmail)
	assert.Equal(t, "parent", resp.OwnerUserType)
	assert.Equal(t, "张*", resp.RealNameMasked)
	assert.Equal(t, "138****8000", resp.PhoneMasked)
}

func TestLookupByIDCard_MissReturnsNotFound(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()
	hash := svc.cipher.Hash("110105200001011234")
	store.On("GetByIDCardHash", ctx, hash).Return(nil, pgx.ErrNoRows)

	_, err := svc.LookupByIDCard(ctx, "student", "110105200001011234")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestGenerateInviteCode_StoresKeysAndOverwritesPrevious(t *testing.T) {
	svc, store, _, _, _, mr := newTestService(t)
	ctx := context.Background()

	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil).Twice()

	resp1, err := svc.GenerateInviteCode(ctx, 10, 1)
	require.NoError(t, err)
	assert.Len(t, resp1.Code, 6)
	got, err := mr.Get(inviteCodeKeyPrefix + resp1.Code)
	require.NoError(t, err)
	assert.Equal(t, "1", got)

	// Second generation must replace the first; old code becomes invalid.
	resp2, err := svc.GenerateInviteCode(ctx, 10, 1)
	require.NoError(t, err)
	assert.NotEqual(t, resp1.Code, resp2.Code)
	_, err = mr.Get(inviteCodeKeyPrefix + resp1.Code)
	assert.Equal(t, miniredis.ErrKeyNotFound, err, "previous code should be wiped")
	got, err = mr.Get(inviteCodeKeyPrefix + resp2.Code)
	require.NoError(t, err)
	assert.Equal(t, "1", got)
}

func TestGenerateInviteCode_NonOwnerForbidden(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("GetOwnerUserID", ctx, int64(1)).Return(int64(10), nil)
	_, err := svc.GenerateInviteCode(ctx, 99, 1)
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestLookupByCode_SingleUseAndDeletesKeys(t *testing.T) {
	svc, store, _, users, _, mr := newTestService(t)
	ctx := context.Background()

	const code = "246810"
	const profileID int64 = 1
	require.NoError(t, mr.Set(inviteCodeKeyPrefix+code, strconv.FormatInt(profileID, 10)))
	require.NoError(t, mr.Set(inviteCodeProfileKey+strconv.FormatInt(profileID, 10), code))

	store.On("GetByID", ctx, profileID).Return(&Profile{ID: profileID, UserID: 10, RealName: "张三", ProvinceID: 11}, nil)
	users.On("GetByID", ctx, int64(10)).Return(&user.User{ID: 10, Email: "p@example.com", UserType: "parent"}, nil)

	resp, err := svc.LookupByCode(ctx, "student", code)
	require.NoError(t, err)
	assert.Equal(t, profileID, resp.ProfileID)

	// First redemption succeeded. Second must miss because key is deleted.
	_, err = mr.Get(inviteCodeKeyPrefix + code)
	assert.Equal(t, miniredis.ErrKeyNotFound, err)
}

func TestLookupByCode_InvalidCode(t *testing.T) {
	svc, _, _, _, _, _ := newTestService(t)
	_, err := svc.LookupByCode(context.Background(), "student", "000000")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeNotFound, appErr.Code)
}

func TestGetMyProfiles_NonStudentParentForbidden(t *testing.T) {
	svc, _, _, _, _, _ := newTestService(t)
	_, err := svc.GetMyProfiles(context.Background(), 1, "admin")
	require.Error(t, err)
	appErr, _ := err.(*web.AppError)
	assert.Equal(t, web.ErrCodeForbidden, appErr.Code)
}

func TestGetMyProfiles_FlagsCanWriteByOwnership(t *testing.T) {
	svc, store, _, _, _, _ := newTestService(t)
	ctx := context.Background()

	store.On("ListByOwnerOrBoundUser", ctx, int64(10)).Return([]*Profile{
		{ID: 1, UserID: 10, ProvinceID: 11, Status: "active"},
		{ID: 2, UserID: 99, ProvinceID: 11, Status: "active"},
	}, nil)

	out, err := svc.GetMyProfiles(ctx, 10, "parent")
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.True(t, out[0].CanWrite)
	assert.False(t, out[1].CanWrite)
}

// Sanity: user.ErrBindingNotFound is what we expect to filter on.
func TestCanRead_BindingNotFoundDoesNotPropagate(t *testing.T) {
	svc, _, bind, _, _, _ := newTestService(t)
	ctx := context.Background()

	bind.On("GetBindingByStudent", ctx, int64(99)).Return(nil, user.ErrBindingNotFound)
	bind.On("GetBindingByStudent", ctx, int64(10)).Return(nil, user.ErrBindingNotFound)

	allowed, err := svc.canRead(ctx, 99, 10)
	require.NoError(t, err)
	assert.False(t, allowed)
}

// Sanity: an unrelated binding error should propagate.
func TestCanRead_UnexpectedErrorPropagates(t *testing.T) {
	svc, _, bind, _, _, _ := newTestService(t)
	ctx := context.Background()
	boom := errors.New("boom")
	bind.On("GetBindingByStudent", ctx, int64(99)).Return(nil, boom)

	_, err := svc.canRead(ctx, 99, 10)
	require.ErrorIs(t, err, boom)
}
