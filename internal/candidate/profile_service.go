package candidate

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"admission-api/internal/platform/config"
	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// ProfileService defines candidate profile business operations.
type ProfileService interface {
	GetMyProfiles(ctx context.Context, userID int64, userType string) ([]*ProfileResponse, error)
	GetProfile(ctx context.Context, userID, profileID int64, userType string) (*ProfileResponse, error)
	CreateProfile(ctx context.Context, userID int64, req CreateProfileRequest) (*ProfileResponse, error)
	UpdateProfile(ctx context.Context, userID, profileID int64, req UpdateProfileRequest) (*ProfileResponse, error)
	DeleteProfile(ctx context.Context, userID, profileID int64) error

	LookupByIDCard(ctx context.Context, callerUserType, idCard string) (*LookupResponse, error)
	LookupByPhone(ctx context.Context, callerUserType, phone string) (*LookupResponse, error)
	LookupByCode(ctx context.Context, callerUserType, code string) (*LookupResponse, error)

	GenerateInviteCode(ctx context.Context, userID, profileID int64) (*InviteResponse, error)
}

const (
	inviteCodeKeyPrefix   = "candidate:bind:code:"
	inviteCodeProfileKey  = "candidate:bind:profile:"
	activityProfileTarget = "profile"
)

type profileService struct {
	store        ProfileStore
	bindingStore user.BindingStore
	userStore    user.Store
	cipher       *IDCardCipher
	activityLog  ActivityLogService
	rdb          *redis.Client
	cfg          *config.Config
}

// NewProfileService constructs a profile service with all required dependencies.
func NewProfileService(
	store ProfileStore,
	bindingStore user.BindingStore,
	userStore user.Store,
	cipher *IDCardCipher,
	activityLog ActivityLogService,
	rdb *redis.Client,
	cfg *config.Config,
) ProfileService {
	return &profileService{
		store:        store,
		bindingStore: bindingStore,
		userStore:    userStore,
		cipher:       cipher,
		activityLog:  activityLog,
		rdb:          rdb,
		cfg:          cfg,
	}
}

// --- CRUD ---

func (s *profileService) GetMyProfiles(ctx context.Context, userID int64, userType string) ([]*ProfileResponse, error) {
	if !isCandidateOrParent(userType) {
		return nil, web.NewError(web.ErrCodeForbidden, "仅考生或家长可访问考生档案")
	}
	profiles, err := s.store.ListByOwnerOrBoundUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*ProfileResponse, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, toProfileResp(p, p.UserID == userID))
	}
	return out, nil
}

func (s *profileService) GetProfile(ctx context.Context, userID, profileID int64, userType string) (*ProfileResponse, error) {
	if !isCandidateOrParent(userType) {
		return nil, web.NewError(web.ErrCodeForbidden, "仅考生或家长可访问考生档案")
	}
	p, err := s.store.GetByID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}
	allowed, err := s.canRead(ctx, userID, p.UserID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, web.NewError(web.ErrCodeForbidden, "无权访问该档案")
	}
	return toProfileResp(p, p.UserID == userID), nil
}

func (s *profileService) CreateProfile(ctx context.Context, userID int64, req CreateProfileRequest) (*ProfileResponse, error) {
	input := &CreateProfileInput{
		UserID:               userID,
		RealName:             req.RealName,
		ProvinceID:           req.ProvinceID,
		CityID:               req.CityID,
		CountyID:             req.CountyID,
		GraduationSchoolName: emptyToNil(req.GraduationSchoolName),
		Grade:                defaultInt16(req.Grade, 3),
		CandidateType:        defaultStr(req.CandidateType, "regular"),
		Gender:               emptyToNil(req.Gender),
		Ethnicity:            emptyToNil(req.Ethnicity),
		ColorVision:          emptyToNil(req.ColorVision),
		Status:               defaultStr(req.Status, "active"),
		CandidatePhone:       emptyToNil(req.CandidatePhone),
	}

	if req.CandidateIDCard != "" {
		blob, err := s.cipher.Encrypt(req.CandidateIDCard)
		if err != nil {
			return nil, fmt.Errorf("encrypt id card: %w", err)
		}
		hash := s.cipher.Hash(req.CandidateIDCard)
		input.CandidateIDCardEnc = blob
		input.CandidateIDCardHash = &hash
	}

	p, err := s.store.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	s.logActivity(ctx, userID, "info_complete", p.ID, map[string]any{"action": "create"})

	return toProfileResp(p, true), nil
}

func (s *profileService) UpdateProfile(ctx context.Context, userID, profileID int64, req UpdateProfileRequest) (*ProfileResponse, error) {
	ownerID, err := s.store.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}
	if ownerID != userID {
		return nil, web.NewError(web.ErrCodeForbidden, "仅档案所有者可修改")
	}

	input := &UpdateProfileInput{
		RealName:             req.RealName,
		CandidatePhone:       req.CandidatePhone,
		ProvinceID:           req.ProvinceID,
		CityID:               req.CityID,
		CountyID:             req.CountyID,
		GraduationSchoolName: req.GraduationSchoolName,
		Grade:                req.Grade,
		CandidateType:        req.CandidateType,
		Gender:               req.Gender,
		Ethnicity:            req.Ethnicity,
		ColorVision:          req.ColorVision,
		Status:               req.Status,
	}

	if req.CandidateIDCard != nil {
		blob, err := s.cipher.Encrypt(*req.CandidateIDCard)
		if err != nil {
			return nil, fmt.Errorf("encrypt id card: %w", err)
		}
		hash := s.cipher.Hash(*req.CandidateIDCard)
		input.CandidateIDCardEnc = blob
		input.CandidateIDCardHash = &hash
		input.UpdateIDCardFields = true
	}

	p, err := s.store.Update(ctx, profileID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}

	s.logActivity(ctx, userID, "profile_modify", p.ID, map[string]any{"action": "update"})

	return toProfileResp(p, true), nil
}

func (s *profileService) DeleteProfile(ctx context.Context, userID, profileID int64) error {
	ownerID, err := s.store.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return err
	}
	if ownerID != userID {
		return web.NewError(web.ErrCodeForbidden, "仅档案所有者可删除")
	}
	if err := s.store.SoftDelete(ctx, profileID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return err
	}

	s.logActivity(ctx, userID, "profile_delete", profileID, nil)
	// also clear any pending invite code so it can't outlive the deletion
	_ = s.clearInviteCodeByProfile(ctx, profileID)
	return nil
}

// --- Lookups ---

func (s *profileService) LookupByIDCard(ctx context.Context, callerUserType, idCard string) (*LookupResponse, error) {
	if !isCandidateOrParent(callerUserType) {
		return nil, web.NewError(web.ErrCodeForbidden, "仅考生或家长可执行查询")
	}
	hash := s.cipher.Hash(idCard)
	p, err := s.store.GetByIDCardHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "未找到对应档案")
		}
		return nil, err
	}
	return s.buildLookupResponse(ctx, p, "idcard")
}

func (s *profileService) LookupByPhone(ctx context.Context, callerUserType, phone string) (*LookupResponse, error) {
	if !isCandidateOrParent(callerUserType) {
		return nil, web.NewError(web.ErrCodeForbidden, "仅考生或家长可执行查询")
	}
	p, err := s.store.GetByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "未找到对应档案")
		}
		return nil, err
	}
	return s.buildLookupResponse(ctx, p, "phone")
}

func (s *profileService) LookupByCode(ctx context.Context, callerUserType, code string) (*LookupResponse, error) {
	if !isCandidateOrParent(callerUserType) {
		return nil, web.NewError(web.ErrCodeForbidden, "仅考生或家长可执行查询")
	}
	idStr, err := s.rdb.Get(ctx, inviteCodeKeyPrefix+code).Result()
	if err == redis.Nil {
		return nil, web.NewError(web.ErrCodeNotFound, "绑定码无效或已过期")
	}
	if err != nil {
		return nil, fmt.Errorf("get invite code: %w", err)
	}
	profileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse invite code value: %w", err)
	}
	p, err := s.store.GetByID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Stale mapping (profile was deleted); clean it up.
			_ = s.clearInviteCode(ctx, code, profileID)
			return nil, web.NewError(web.ErrCodeNotFound, "绑定码对应档案已不存在")
		}
		return nil, err
	}
	resp, err := s.buildLookupResponse(ctx, p, "code")
	if err != nil {
		return nil, err
	}
	// Single-use: invalidate after successful redemption.
	_ = s.clearInviteCode(ctx, code, p.ID)
	return resp, nil
}

func (s *profileService) buildLookupResponse(ctx context.Context, p *Profile, kind string) (*LookupResponse, error) {
	owner, err := s.userStore.GetByID(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案所有者不存在")
		}
		return nil, fmt.Errorf("get owner: %w", err)
	}
	resp := &LookupResponse{
		ProfileID:      p.ID,
		OwnerUserID:    owner.ID,
		OwnerEmail:     owner.Email,
		OwnerUserType:  owner.UserType,
		RealNameMasked: maskName(p.RealName),
	}
	if p.CandidatePhone != nil {
		resp.PhoneMasked = MaskPhone(*p.CandidatePhone)
	}

	// Best-effort audit log for the lookup itself; uses owner's profile as the target.
	s.logActivity(ctx, owner.ID, "profile_lookup", p.ID, map[string]any{"kind": kind})

	return resp, nil
}

// --- Invite code ---

func (s *profileService) GenerateInviteCode(ctx context.Context, userID, profileID int64) (*InviteResponse, error) {
	ownerID, err := s.store.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}
	if ownerID != userID {
		return nil, web.NewError(web.ErrCodeForbidden, "仅档案所有者可生成绑定码")
	}

	// Invalidate any existing code for this profile so the old one can no longer be redeemed.
	if err := s.clearInviteCodeByProfile(ctx, profileID); err != nil {
		return nil, err
	}

	code, err := generateInviteCode()
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(s.cfg.CandidateBindCodeTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, inviteCodeKeyPrefix+code, profileID, ttl)
	pipe.Set(ctx, inviteCodeProfileKey+strconv.FormatInt(profileID, 10), code, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("set invite code: %w", err)
	}

	s.logActivity(ctx, userID, "invite_code_generated", profileID, nil)

	return &InviteResponse{
		Code:      code,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

func (s *profileService) clearInviteCode(ctx context.Context, code string, profileID int64) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, inviteCodeKeyPrefix+code)
	pipe.Del(ctx, inviteCodeProfileKey+strconv.FormatInt(profileID, 10))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *profileService) clearInviteCodeByProfile(ctx context.Context, profileID int64) error {
	profileKey := inviteCodeProfileKey + strconv.FormatInt(profileID, 10)
	prevCode, err := s.rdb.Get(ctx, profileKey).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read existing invite code: %w", err)
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, inviteCodeKeyPrefix+prevCode)
	pipe.Del(ctx, profileKey)
	_, err = pipe.Exec(ctx)
	return err
}

// --- helpers ---

func (s *profileService) canRead(ctx context.Context, callerID, ownerID int64) (bool, error) {
	if callerID == ownerID {
		return true, nil
	}
	// Caller might be a student bound to owner-as-parent.
	if b, err := s.bindingStore.GetBindingByStudent(ctx, callerID); err == nil {
		if b.ParentID == ownerID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	// Caller might be a parent whose bound student owns the profile.
	if b, err := s.bindingStore.GetBindingByStudent(ctx, ownerID); err == nil {
		if b.ParentID == callerID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	return false, nil
}

func (s *profileService) logActivity(ctx context.Context, userID int64, activityType string, profileID int64, metadata map[string]any) {
	if s.activityLog == nil {
		return
	}
	_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
		UserID:       userID,
		ActivityType: activityType,
		TargetType:   activityProfileTarget,
		TargetID:     profileID,
		Metadata:     metadata,
	})
}

func isCandidateOrParent(userType string) bool {
	return userType == "student" || userType == "parent"
}

func toProfileResp(p *Profile, canWrite bool) *ProfileResponse {
	resp := &ProfileResponse{
		ID:                   p.ID,
		UserID:               p.UserID,
		RealName:             p.RealName,
		ProvinceID:           p.ProvinceID,
		CityID:               p.CityID,
		CountyID:             p.CountyID,
		GraduationSchoolName: p.GraduationSchoolName,
		Grade:                p.Grade,
		CandidateType:        p.CandidateType,
		Gender:               p.Gender,
		Ethnicity:            p.Ethnicity,
		ColorVision:          p.ColorVision,
		Status:               p.Status,
		CanWrite:             canWrite,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
	if p.CandidatePhone != nil {
		resp.PhoneMasked = MaskPhone(*p.CandidatePhone)
	}
	if len(p.CandidateIDCardEnc) > 0 {
		// We don't decrypt for the response — we only need to know an ID-card is set.
		// Provide a fully-masked placeholder of the canonical 18-digit length.
		resp.IDCardMasked = MaskIDCard("000000000000000000")
	}
	return resp
}

func maskName(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	if len(r) == 1 {
		return string(r)
	}
	return string(r[0]) + "*"
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func defaultInt16(v, fallback int16) int16 {
	if v == 0 {
		return fallback
	}
	return v
}

// generateInviteCode returns a 6-digit numeric code in [100000, 999999].
func generateInviteCode() (string, error) {
	max := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	return strconv.FormatInt(n.Int64()+100000, 10), nil
}
