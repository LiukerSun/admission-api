package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"admission-api/internal/user"
)

// Store defines the data access interface for admin operations.
type Store interface {
	ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) ([]*user.User, int64, error)
	ListBindings(ctx context.Context, page, pageSize int) ([]*BindingListItem, int64, error)
	GetStats(ctx context.Context) (*StatsResponse, error)
}

type store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new admin store.
func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func (s *store) ListUsers(ctx context.Context, filter ListUsersFilter, page, pageSize int) ([]*user.User, int64, error) {
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
		SELECT id, email, username, phone, phone_verified_at, password_hash, role, user_type, status, created_at, updated_at
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

	var users []*user.User
	for rows.Next() {
		var u user.User
		err := rows.Scan(
			&u.ID, &u.Email, &u.Username, &u.Phone, &u.PhoneVerifiedAt, &u.PasswordHash, &u.Role, &u.UserType, &u.Status, &u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	return users, total, nil
}

func (s *store) ListBindings(ctx context.Context, page, pageSize int) ([]*BindingListItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Count total
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_bindings").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count bindings: %w", err)
	}

	query := `
		SELECT b.id, b.parent_id, p.email, b.student_id, s.email, b.created_at
		FROM user_bindings b
		JOIN users p ON p.id = b.parent_id
		JOIN users s ON s.id = b.student_id
		ORDER BY b.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.pool.Query(ctx, query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	var bindings []*BindingListItem
	for rows.Next() {
		var b BindingListItem
		err := rows.Scan(
			&b.ID, &b.Parent.ID, &b.Parent.Email, &b.Student.ID, &b.Student.Email, &b.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan binding: %w", err)
		}
		bindings = append(bindings, &b)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate bindings: %w", err)
	}

	return bindings, total, nil
}

func (s *store) GetStats(ctx context.Context) (*StatsResponse, error) {
	stats := &StatsResponse{
		UsersByRole: make(map[string]int64),
	}

	// Total users
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&stats.TotalUsers); err != nil {
		return nil, fmt.Errorf("count total users: %w", err)
	}

	// Users by role
	rows, err := s.pool.Query(ctx, "SELECT role, COUNT(*) FROM users GROUP BY role")
	if err != nil {
		return nil, fmt.Errorf("count users by role: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var count int64
		if err := rows.Scan(&role, &count); err != nil {
			return nil, fmt.Errorf("scan role count: %w", err)
		}
		stats.UsersByRole[role] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role counts: %w", err)
	}

	// Total bindings
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_bindings").Scan(&stats.TotalBindings); err != nil {
		return nil, fmt.Errorf("count bindings: %w", err)
	}

	// Active users
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE status = 'active'").Scan(&stats.ActiveUsers); err != nil {
		return nil, fmt.Errorf("count active users: %w", err)
	}

	// Banned users
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE status = 'banned'").Scan(&stats.BannedUsers); err != nil {
		return nil, fmt.Errorf("count banned users: %w", err)
	}

	return stats, nil
}
