package planner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MerchantStore defines merchant data access operations.
type MerchantStore interface {
	CreateMerchant(ctx context.Context, input *CreateMerchantInput) (*PlannerMerchant, error)
	GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error)
	ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) ([]*PlannerMerchant, int64, error)
	UpdateMerchant(ctx context.Context, id int64, input *UpdateMerchantInput) (*PlannerMerchant, error)
	UserExists(ctx context.Context, userID int64) (bool, error)
}

type merchantStore struct {
	pool *pgxpool.Pool
}

// NewMerchantStore creates a new merchant store.
func NewMerchantStore(pool *pgxpool.Pool) MerchantStore {
	return &merchantStore{pool: pool}
}

func scanMerchant(row pgx.Row) (*PlannerMerchant, error) {
	var m PlannerMerchant
	if err := row.Scan(
		&m.ID,
		&m.MerchantName,
		&m.ContactPerson,
		&m.ContactPhone,
		&m.Address,
		&m.Logo,
		&m.Banner,
		&m.Description,
		&m.SortOrder,
		&m.OwnerID,
		&m.ServiceRegions,
		&m.DefaultServicePrice,
		&m.Status,
		&m.CreatedAt,
		&m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *merchantStore) CreateMerchant(ctx context.Context, input *CreateMerchantInput) (*PlannerMerchant, error) {
	m, err := scanMerchant(s.pool.QueryRow(ctx, `
		INSERT INTO planner_merchants (
			merchant_name, contact_person, contact_phone, address, logo, banner, description,
			sort_order, owner_id, service_regions, default_service_price, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, merchant_name, contact_person, contact_phone, address, logo, banner, description,
			sort_order, owner_id, service_regions, default_service_price, status, created_at, updated_at
	`, input.MerchantName, input.ContactPerson, input.ContactPhone, input.Address, input.Logo,
		input.Banner, input.Description, input.SortOrder, input.OwnerID, input.ServiceRegions,
		input.DefaultServicePrice, input.Status))
	if err != nil {
		return nil, fmt.Errorf("create merchant: %w", err)
	}
	return m, nil
}

func (s *merchantStore) GetMerchant(ctx context.Context, id int64) (*PlannerMerchant, error) {
	m, err := scanMerchant(s.pool.QueryRow(ctx, `
		SELECT id, merchant_name, contact_person, contact_phone, address, logo, banner, description,
			sort_order, owner_id, service_regions, default_service_price, status, created_at, updated_at
		FROM planner_merchants
		WHERE id = $1
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get merchant: %w", err)
	}
	return m, nil
}

func (s *merchantStore) ListMerchants(ctx context.Context, status, merchantName, serviceRegion string, page, pageSize int) ([]*PlannerMerchant, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var conditions []string
	var args []interface{}
	argNum := 1

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, status)
		argNum++
	}
	if merchantName != "" {
		conditions = append(conditions, fmt.Sprintf("merchant_name ILIKE $%d", argNum))
		args = append(args, "%"+merchantName+"%")
		argNum++
	}
	if serviceRegion != "" {
		conditions = append(conditions, fmt.Sprintf("service_regions @> $%d", argNum))
		args = append(args, []string{serviceRegion})
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM planner_merchants %s", whereClause)
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count merchants: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, merchant_name, contact_person, contact_phone, address, logo, banner, description,
			sort_order, owner_id, service_regions, default_service_price, status, created_at, updated_at
		FROM planner_merchants
		%s
		ORDER BY sort_order DESC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)
	args = append(args, pageSize, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list merchants: %w", err)
	}
	defer rows.Close()

	merchants := []*PlannerMerchant{}
	for rows.Next() {
		m, err := scanMerchant(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan merchant: %w", err)
		}
		merchants = append(merchants, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate merchants: %w", err)
	}
	return merchants, total, nil
}

func (s *merchantStore) UpdateMerchant(ctx context.Context, id int64, input *UpdateMerchantInput) (*PlannerMerchant, error) {
	var sets []string
	var args []interface{}
	argNum := 1

	if input.MerchantName != nil {
		sets = append(sets, fmt.Sprintf("merchant_name = $%d", argNum))
		args = append(args, *input.MerchantName)
		argNum++
	}
	if input.ContactPerson != nil {
		sets = append(sets, fmt.Sprintf("contact_person = $%d", argNum))
		args = append(args, *input.ContactPerson)
		argNum++
	}
	if input.ContactPhone != nil {
		sets = append(sets, fmt.Sprintf("contact_phone = $%d", argNum))
		args = append(args, *input.ContactPhone)
		argNum++
	}
	if input.Address != nil {
		sets = append(sets, fmt.Sprintf("address = $%d", argNum))
		args = append(args, *input.Address)
		argNum++
	}
	if input.Logo != nil {
		sets = append(sets, fmt.Sprintf("logo = $%d", argNum))
		args = append(args, *input.Logo)
		argNum++
	}
	if input.Banner != nil {
		sets = append(sets, fmt.Sprintf("banner = $%d", argNum))
		args = append(args, *input.Banner)
		argNum++
	}
	if input.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argNum))
		args = append(args, *input.Description)
		argNum++
	}
	if input.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argNum))
		args = append(args, *input.SortOrder)
		argNum++
	}
	if input.OwnerID != nil {
		sets = append(sets, fmt.Sprintf("owner_id = $%d", argNum))
		args = append(args, *input.OwnerID)
		argNum++
	}
	if input.ServiceRegions != nil {
		sets = append(sets, fmt.Sprintf("service_regions = $%d", argNum))
		args = append(args, input.ServiceRegions)
		argNum++
	}
	if input.DefaultServicePrice != nil {
		sets = append(sets, fmt.Sprintf("default_service_price = $%d", argNum))
		args = append(args, *input.DefaultServicePrice)
		argNum++
	}
	if input.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *input.Status)
		argNum++
	}

	if len(sets) == 0 {
		return s.GetMerchant(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf(`
		UPDATE planner_merchants
		SET %s
		WHERE id = $%d
		RETURNING id, merchant_name, contact_person, contact_phone, address, logo, banner, description,
			sort_order, owner_id, service_regions, default_service_price, status, created_at, updated_at
	`, strings.Join(sets, ", "), argNum)

	m, err := scanMerchant(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("update merchant: %w", err)
	}
	return m, nil
}

func (s *merchantStore) UserExists(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}
