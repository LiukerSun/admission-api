package planner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"admission-api/internal/platform/web"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// ProfileService defines planner profile business operations.
type ProfileService interface {
	CreateProfile(ctx context.Context, req CreateProfileRequest) (*PlannerProfileResponse, error)
	GetMyProfile(ctx context.Context, userID int64) (*PlannerProfileResponse, error)
	UpdateMyProfile(ctx context.Context, userID int64, req UpdateMyProfileRequest) (*PlannerProfileResponse, error)
	GetProfile(ctx context.Context, id int64) (*PlannerProfileResponse, error)
	ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) (*ProfileListResponse, error)
}

type profileService struct {
	profileStore  ProfileStore
	merchantStore MerchantStore
}

// NewProfileService creates a new profile service.
func NewProfileService(profileStore ProfileStore, merchantStore MerchantStore) ProfileService {
	return &profileService{
		profileStore:  profileStore,
		merchantStore: merchantStore,
	}
}

func (s *profileService) CreateProfile(ctx context.Context, req CreateProfileRequest) (*PlannerProfileResponse, error) {
	exists, err := s.profileStore.EmailExists(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, web.NewError(web.ErrCodeConflict, "邮箱已存在")
	}

	var merchant *PlannerMerchant
	if req.MerchantID != nil && *req.MerchantID > 0 {
		m, err := s.merchantStore.GetMerchant(ctx, *req.MerchantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, web.NewError(web.ErrCodeBadRequest, "机构不存在")
			}
			return nil, err
		}
		merchant = m
	}

	serviceRegion := req.ServiceRegion
	if merchant != nil {
		if len(serviceRegion) == 0 {
			serviceRegion = merchant.ServiceRegions
		} else {
			if err := validateServiceRegions(serviceRegion, merchant.ServiceRegions); err != nil {
				return nil, err
			}
		}
	}

	if req.LevelExpireAt != nil && req.LevelExpireAt.Before(time.Now()) {
		return nil, web.NewError(web.ErrCodeBadRequest, "等级过期时间必须在将来")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	level := req.Level
	if level == "" {
		level = "junior"
	}
	status := req.Status
	if status == "" {
		status = "active"
	}

	var merchantName *string
	if merchant != nil {
		merchantName = &merchant.MerchantName
	}

	input := &CreateProfileInput{
		RealName:        req.RealName,
		Avatar:          strPtr(req.Avatar),
		Phone:           strPtr(req.Phone),
		Title:           strPtr(req.Title),
		Introduction:    strPtr(req.Introduction),
		SpecialtyTags:   req.SpecialtyTags,
		ServiceRegion:   serviceRegion,
		ServicePrice:    req.ServicePrice,
		Level:           level,
		LevelExpireAt:   req.LevelExpireAt,
		CertificationNo: strPtr(req.CertificationNo),
		MerchantID:      req.MerchantID,
		MerchantName:    merchantName,
		Status:          status,
	}

	p, err := s.profileStore.CreateUserAndProfile(ctx, req.Email, string(hash), "planner", "student", input)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, web.NewError(web.ErrCodeConflict, "邮箱已存在")
		}
		return nil, err
	}

	return toProfileResponse(p), nil
}

func (s *profileService) GetMyProfile(ctx context.Context, userID int64) (*PlannerProfileResponse, error) {
	p, err := s.profileStore.GetProfileByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}
	return toProfileResponse(p), nil
}

func (s *profileService) UpdateMyProfile(ctx context.Context, userID int64, req UpdateMyProfileRequest) (*PlannerProfileResponse, error) {
	existing, err := s.profileStore.GetProfileByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}

	if req.LevelExpireAt != nil && req.LevelExpireAt.Before(time.Now()) {
		return nil, web.NewError(web.ErrCodeBadRequest, "等级过期时间必须在将来")
	}

	if req.MerchantID != nil && *req.MerchantID > 0 {
		_, err := s.merchantStore.GetMerchant(ctx, *req.MerchantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, web.NewError(web.ErrCodeBadRequest, "机构不存在")
			}
			return nil, err
		}
	}

	var merchant *PlannerMerchant
	effectiveMerchantID := existing.MerchantID
	if req.MerchantID != nil {
		if *req.MerchantID > 0 {
			effectiveMerchantID = req.MerchantID
		} else {
			effectiveMerchantID = nil
		}
	}

	if effectiveMerchantID != nil {
		m, err := s.merchantStore.GetMerchant(ctx, *effectiveMerchantID)
		if err != nil {
			return nil, err
		}
		merchant = m
	}

	serviceRegion := req.ServiceRegion
	if serviceRegion != nil && merchant != nil {
		if len(serviceRegion) == 0 {
			serviceRegion = merchant.ServiceRegions
		} else {
			if err := validateServiceRegions(serviceRegion, merchant.ServiceRegions); err != nil {
				return nil, err
			}
		}
	}

	var merchantName *string
	if merchant != nil {
		merchantName = &merchant.MerchantName
	}

	input := &UpdateProfileInput{
		RealName:        req.RealName,
		Avatar:          req.Avatar,
		Phone:           req.Phone,
		Title:           req.Title,
		Introduction:    req.Introduction,
		SpecialtyTags:   req.SpecialtyTags,
		ServiceRegion:   serviceRegion,
		ServicePrice:    req.ServicePrice,
		Level:           req.Level,
		LevelExpireAt:   req.LevelExpireAt,
		CertificationNo: req.CertificationNo,
		MerchantID:      req.MerchantID,
		MerchantName:    merchantName,
		Status:          req.Status,
	}

	p, err := s.profileStore.UpdateProfile(ctx, userID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}

	return toProfileResponse(p), nil
}

func (s *profileService) GetProfile(ctx context.Context, id int64) (*PlannerProfileResponse, error) {
	p, err := s.profileStore.GetProfile(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return nil, err
	}
	return toProfileResponse(p), nil
}

func (s *profileService) ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) (*ProfileListResponse, error) {
	profiles, total, err := s.profileStore.ListProfiles(ctx, filter, page, pageSize, sortField, sortOrder)
	if err != nil {
		return nil, err
	}

	resp := make([]*PlannerProfileResponse, len(profiles))
	for i, p := range profiles {
		resp[i] = toProfileResponse(p)
	}

	return &ProfileListResponse{
		Profiles: resp,
		Total:    total,
	}, nil
}

func toProfileResponse(p *PlannerProfile) *PlannerProfileResponse {
	return &PlannerProfileResponse{
		ID:                p.ID,
		UserID:            p.UserID,
		RealName:          p.RealName,
		Avatar:            p.Avatar,
		Phone:             p.Phone,
		Title:             p.Title,
		Introduction:      p.Introduction,
		SpecialtyTags:     p.SpecialtyTags,
		ServiceRegion:     p.ServiceRegion,
		ServicePrice:      p.ServicePrice,
		Level:             p.Level,
		LevelExpireAt:     p.LevelExpireAt,
		CertificationNo:   p.CertificationNo,
		MerchantID:        p.MerchantID,
		MerchantName:      p.MerchantName,
		TotalServiceCount: p.TotalServiceCount,
		RatingAvg:         p.RatingAvg,
		Status:            p.Status,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func validateServiceRegions(regions, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}
	for _, r := range regions {
		if _, ok := allowedSet[r]; !ok {
			return web.NewError(web.ErrCodeBadRequest, fmt.Sprintf("服务区域 %s 不在机构服务范围内", r))
		}
	}
	return nil
}
