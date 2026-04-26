package planner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProfileStore defines planner profile data access operations.
type ProfileStore interface {
	CreateUserAndProfile(ctx context.Context, email, passwordHash, role, userType string, input *CreateProfileInput) (*PlannerProfile, error)
	GetProfileByUserID(ctx context.Context, userID int64) (*PlannerProfile, error)
	GetProfile(ctx context.Context, id int64) (*PlannerProfile, error)
	UpdateProfile(ctx context.Context, userID int64, input *UpdateProfileInput) (*PlannerProfile, error)
	ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) ([]*PlannerProfile, int64, error)
	UserExists(ctx context.Context, userID int64) (bool, error)
	EmailExists(ctx context.Context, email string) (bool, error)
}

type profileStore struct {
	pool *pgxpool.Pool
}

// NewProfileStore creates a new profile store.
func NewProfileStore(pool *pgxpool.Pool) ProfileStore {
	return &profileStore{pool: pool}
}

func scanProfile(row pgx.Row) (*PlannerProfile, error) {
	var p PlannerProfile
	if err := row.Scan(
		&p.ID, &p.UserID, &p.RealName, &p.Avatar, &p.Phone, &p.Title,
		&p.Introduction, &p.SpecialtyTags, &p.ServiceRegion, &p.ServicePrice,
		&p.Level, &p.LevelExpireAt, &p.CertificationNo, &p.MerchantID, &p.MerchantName,
		&p.TotalServiceCount, &p.RatingAvg, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *profileStore) CreateUserAndProfile(ctx context.Context, email, passwordHash, role, userType string, input *CreateProfileInput) (*PlannerProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, user_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', NOW(), NOW())
		RETURNING id
	`, email, passwordHash, role, userType).Scan(&userID); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	p, err := scanProfile(tx.QueryRow(ctx, `
		INSERT INTO planner_profiles (
			user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, total_service_count, rating_avg, status, created_at, updated_at
	`, userID, input.RealName, input.Avatar, input.Phone, input.Title, input.Introduction,
		input.SpecialtyTags, input.ServiceRegion, input.ServicePrice, input.Level,
		input.LevelExpireAt, input.CertificationNo, input.MerchantID, input.MerchantName, input.Status))
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return p, nil
}

func (s *profileStore) GetProfileByUserID(ctx context.Context, userID int64) (*PlannerProfile, error) {
	p, err := scanProfile(s.pool.QueryRow(ctx, `
		SELECT id, user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, total_service_count, rating_avg, status, created_at, updated_at
		FROM planner_profiles
		WHERE user_id = $1
	`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get profile by user id: %w", err)
	}
	return p, nil
}

func (s *profileStore) GetProfile(ctx context.Context, id int64) (*PlannerProfile, error) {
	p, err := scanProfile(s.pool.QueryRow(ctx, `
		SELECT id, user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, total_service_count, rating_avg, status, created_at, updated_at
		FROM planner_profiles
		WHERE id = $1
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

func (s *profileStore) UpdateProfile(ctx context.Context, userID int64, input *UpdateProfileInput) (*PlannerProfile, error) {
	var sets []string
	var args []interface{}
	argNum := 1

	if input.RealName != nil {
		sets = append(sets, fmt.Sprintf("real_name = $%d", argNum))
		args = append(args, *input.RealName)
		argNum++
	}
	if input.Avatar != nil {
		sets = append(sets, fmt.Sprintf("avatar = $%d", argNum))
		args = append(args, *input.Avatar)
		argNum++
	}
	if input.Phone != nil {
		sets = append(sets, fmt.Sprintf("phone = $%d", argNum))
		args = append(args, *input.Phone)
		argNum++
	}
	if input.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argNum))
		args = append(args, *input.Title)
		argNum++
	}
	if input.Introduction != nil {
		sets = append(sets, fmt.Sprintf("introduction = $%d", argNum))
		args = append(args, *input.Introduction)
		argNum++
	}
	if input.SpecialtyTags != nil {
		sets = append(sets, fmt.Sprintf("specialty_tags = $%d", argNum))
		args = append(args, input.SpecialtyTags)
		argNum++
	}
	if input.ServiceRegion != nil {
		sets = append(sets, fmt.Sprintf("service_region = $%d", argNum))
		args = append(args, input.ServiceRegion)
		argNum++
	}
	if input.ServicePrice != nil {
		sets = append(sets, fmt.Sprintf("service_price = $%d", argNum))
		args = append(args, *input.ServicePrice)
		argNum++
	}
	if input.Level != nil {
		sets = append(sets, fmt.Sprintf("level = $%d", argNum))
		args = append(args, *input.Level)
		argNum++
	}
	if input.LevelExpireAt != nil {
		sets = append(sets, fmt.Sprintf("level_expire_at = $%d", argNum))
		args = append(args, *input.LevelExpireAt)
		argNum++
	}
	if input.CertificationNo != nil {
		sets = append(sets, fmt.Sprintf("certification_no = $%d", argNum))
		args = append(args, *input.CertificationNo)
		argNum++
	}
	if input.MerchantID != nil {
		sets = append(sets, fmt.Sprintf("merchant_id = $%d", argNum))
		args = append(args, *input.MerchantID)
		argNum++
	}
	if input.MerchantName != nil {
		sets = append(sets, fmt.Sprintf("merchant_name = $%d", argNum))
		args = append(args, *input.MerchantName)
		argNum++
	}
	if input.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *input.Status)
		argNum++
	}

	if len(sets) == 0 {
		return s.GetProfileByUserID(ctx, userID)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, userID)
	query := fmt.Sprintf(`
		UPDATE planner_profiles
		SET %s
		WHERE user_id = $%d
		RETURNING id, user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, total_service_count, rating_avg, status, created_at, updated_at
	`, strings.Join(sets, ", "), argNum)

	p, err := scanProfile(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return p, nil
}

func (s *profileStore) ListProfiles(ctx context.Context, filter ProfileFilter, page, pageSize int, sortField, sortOrder string) ([]*PlannerProfile, int64, error) {
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

	if filter.Level != "" {
		conditions = append(conditions, fmt.Sprintf("level = $%d", argNum))
		args = append(args, filter.Level)
		argNum++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, filter.Status)
		argNum++
	}
	if filter.MerchantID != nil {
		conditions = append(conditions, fmt.Sprintf("merchant_id = $%d", argNum))
		args = append(args, *filter.MerchantID)
		argNum++
	}
	if filter.RealName != "" {
		conditions = append(conditions, fmt.Sprintf("real_name ILIKE $%d", argNum))
		args = append(args, "%"+filter.RealName+"%")
		argNum++
	}
	if filter.Phone != "" {
		conditions = append(conditions, fmt.Sprintf("phone ILIKE $%d", argNum))
		args = append(args, "%"+filter.Phone+"%")
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM planner_profiles %s", whereClause)
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count profiles: %w", err)
	}

	orderField := "created_at"
	if sortField == "rating_avg" || sortField == "total_service_count" {
		orderField = sortField
	}
	orderDir := "DESC"
	if strings.ToLower(sortOrder) == "asc" {
		orderDir = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, real_name, avatar, phone, title, introduction, specialty_tags,
			service_region, service_price, level, level_expire_at, certification_no,
			merchant_id, merchant_name, total_service_count, rating_avg, status, created_at, updated_at
		FROM planner_profiles
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderField, orderDir, argNum, argNum+1)
	args = append(args, pageSize, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list profiles: %w", err)
	}
	defer rows.Close()

	profiles := []*PlannerProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate profiles: %w", err)
	}
	return profiles, total, nil
}

func (s *profileStore) UserExists(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}

func (s *profileStore) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("check email exists: %w", err)
	}
	return exists, nil
}
