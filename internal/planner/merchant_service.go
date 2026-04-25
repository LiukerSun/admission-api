package planner

import (
	"context"
	"errors"

	"admission-api/internal/platform/web"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MerchantService defines merchant business operations.
type MerchantService interface {
	CreateMerchant(ctx context.Context, req CreateMerchantRequest) (*PlannerMerchant, error)
	GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error)
	ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) (*MerchantListResponse, error)
	UpdateMerchant(ctx context.Context, id int64, req UpdateMerchantRequest) (*PlannerMerchant, error)
}

type merchantService struct {
	store MerchantStore
}

// NewMerchantService creates a new merchant service.
func NewMerchantService(store MerchantStore) MerchantService {
	return &merchantService{store: store}
}

func (s *merchantService) CreateMerchant(ctx context.Context, req CreateMerchantRequest) (*PlannerMerchant, error) {
	if req.OwnerID != nil && *req.OwnerID > 0 {
		exists, err := s.store.UserExists(ctx, *req.OwnerID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, web.NewError(web.ErrCodeBadRequest, "负责人用户不存在")
		}
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	input := &CreateMerchantInput{
		MerchantName:        req.MerchantName,
		ContactPerson:       strPtr(req.ContactPerson),
		ContactPhone:        strPtr(req.ContactPhone),
		Address:             strPtr(req.Address),
		Logo:                strPtr(req.Logo),
		Banner:              strPtr(req.Banner),
		Description:         strPtr(req.Description),
		SortOrder:           req.SortOrder,
		OwnerID:             req.OwnerID,
		ServiceRegions:      req.ServiceRegions,
		DefaultServicePrice: req.DefaultServicePrice,
		Status:              status,
	}

	m, err := s.store.CreateMerchant(ctx, input)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, web.NewError(web.ErrCodeConflict, "机构名称已存在")
		}
		return nil, err
	}
	return m, nil
}

func (s *merchantService) GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error) {
	m, err := s.store.GetMerchant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "机构不存在")
		}
		return nil, err
	}
	return m, nil
}

func (s *merchantService) ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) (*MerchantListResponse, error) {
	merchants, total, err := s.store.ListMerchants(ctx, status, merchantName, serviceRegion, page, pageSize)
	if err != nil {
		return nil, err
	}
	return &MerchantListResponse{
		Merchants: merchants,
		Total:     total,
	}, nil
}

func (s *merchantService) UpdateMerchant(ctx context.Context, id int64, req UpdateMerchantRequest) (*PlannerMerchant, error) {
	_, err := s.store.GetMerchant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "机构不存在")
		}
		return nil, err
	}

	if req.OwnerID != nil && *req.OwnerID > 0 {
		exists, err := s.store.UserExists(ctx, *req.OwnerID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, web.NewError(web.ErrCodeBadRequest, "负责人用户不存在")
		}
	}

	input := &UpdateMerchantInput{
		MerchantName:        req.MerchantName,
		ContactPerson:       req.ContactPerson,
		ContactPhone:        req.ContactPhone,
		Address:             req.Address,
		Logo:                req.Logo,
		Banner:              req.Banner,
		Description:         req.Description,
		SortOrder:           req.SortOrder,
		OwnerID:             req.OwnerID,
		ServiceRegions:      req.ServiceRegions,
		DefaultServicePrice: req.DefaultServicePrice,
		Status:              req.Status,
	}

	m, err := s.store.UpdateMerchant(ctx, id, input)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, web.NewError(web.ErrCodeConflict, "机构名称已存在")
		}
		return nil, err
	}
	return m, nil
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
