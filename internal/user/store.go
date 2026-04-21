package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateUserFields holds optional fields for updating a user.
type UpdateUserFields struct {
	Email    *string
	Username *string
	Role     *string
	UserType *string
	Status   *string
}

// Store defines the data access interface for users.
type Store interface {
	Create(ctx context.Context, email, passwordHash, role, userType string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmailAndType(ctx context.Context, email, userType string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	ListUsers(ctx context.Context, filter Filter, page, pageSize int) ([]*User, int64, error)
	UpdateRole(ctx context.Context, id int64, role string) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	UpdateUser(ctx context.Context, id int64, fields UpdateUserFields) error
}

// Filter defines filters for listing users.
type Filter struct {
	Email    string
	Username string
	Role     string
	Status   string
}

// store implements Store using pgx.
type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func (s *store) scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.Role, &u.UserType, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *store) Create(ctx context.Context, email, passwordHash, role, userType string) (*User, error) {
	if role == "" {
		role = "user"
	}

	query := `
		INSERT INTO users (email, password_hash, role, user_type, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, email, username, password_hash, role, user_type, status, created_at, updated_at
	`

	u, err := s.scanUser(s.pool.QueryRow(ctx, query, email, passwordHash, role, userType))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("email already exists")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	return u, nil
}

func (s *store) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, role, user_type, status, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	u, err := s.scanUser(s.pool.QueryRow(ctx, query, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	return u, nil
}

func (s *store) GetByID(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, role, user_type, status, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	u, err := s.scanUser(s.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return u, nil
}

func (s *store) GetByEmailAndType(ctx context.Context, email, userType string) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, role, user_type, status, created_at, updated_at
		FROM users
		WHERE email = $1 AND user_type = $2
	`

	u, err := s.scanUser(s.pool.QueryRow(ctx, query, email, userType))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by email and type: %w", err)
	}

	return u, nil
}

func (s *store) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, email, username, password_hash, role, user_type, status, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	u, err := s.scanUser(s.pool.QueryRow(ctx, query, username))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}

	return u, nil
}

func (s *store) ListUsers(ctx context.Context, filter Filter, page, pageSize int) ([]*User, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	where := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if filter.Email != "" {
		where = append(where, fmt.Sprintf("email ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Email+"%")
		argIdx++
	}
	if filter.Username != "" {
		where = append(where, fmt.Sprintf("username ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Username+"%")
		argIdx++
	}
	if filter.Role != "" {
		where = append(where, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, filter.Role)
		argIdx++
	}
	if filter.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s", whereClause)
	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Query users
	query := fmt.Sprintf(`
		SELECT id, email, username, password_hash, role, user_type, status, created_at, updated_at
		FROM users
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := s.scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	return users, total, nil
}

func (s *store) UpdateRole(ctx context.Context, id int64, role string) error {
	query := `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.pool.Exec(ctx, query, role, id)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *store) UpdateStatus(ctx context.Context, id int64, status string) error {
	query := `UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *store) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.pool.Exec(ctx, query, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *store) UpdateUser(ctx context.Context, id int64, fields UpdateUserFields) error {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if fields.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, *fields.Email)
		argIdx++
	}
	if fields.Username != nil {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", argIdx))
		args = append(args, *fields.Username)
		argIdx++
	}
	if fields.Role != nil {
		setClauses = append(setClauses, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, *fields.Role)
		argIdx++
	}
	if fields.UserType != nil {
		setClauses = append(setClauses, fmt.Sprintf("user_type = $%d", argIdx))
		args = append(args, *fields.UserType)
		argIdx++
	}
	if fields.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *fields.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	result, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("email or username already exists")
		}
		return fmt.Errorf("update user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}
