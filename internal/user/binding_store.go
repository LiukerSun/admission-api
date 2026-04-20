package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BindingStore defines the data access interface for parent-student bindings.
type BindingStore interface {
	CreateBinding(ctx context.Context, parentID, studentID int64) (*Binding, error)
	GetBindingsByParent(ctx context.Context, parentID int64) ([]*Binding, error)
	GetBindingByStudent(ctx context.Context, studentID int64) (*Binding, error)
	DeleteBinding(ctx context.Context, id int64) error
	BindingExistsForStudent(ctx context.Context, studentID int64) (bool, error)
}

type bindingStore struct {
	pool *pgxpool.Pool
}

func NewBindingStore(pool *pgxpool.Pool) BindingStore {
	return &bindingStore{pool: pool}
}

func (s *bindingStore) CreateBinding(ctx context.Context, parentID, studentID int64) (*Binding, error) {
	query := `
		INSERT INTO user_bindings (parent_id, student_id)
		VALUES ($1, $2)
		RETURNING id, parent_id, student_id, created_at
	`

	var b Binding
	err := s.pool.QueryRow(ctx, query, parentID, studentID).Scan(
		&b.ID, &b.ParentID, &b.StudentID, &b.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create binding: %w", err)
	}

	return &b, nil
}

func (s *bindingStore) GetBindingsByParent(ctx context.Context, parentID int64) ([]*Binding, error) {
	query := `
		SELECT id, parent_id, student_id, created_at
		FROM user_bindings
		WHERE parent_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, parentID)
	if err != nil {
		return nil, fmt.Errorf("get bindings by parent: %w", err)
	}
	defer rows.Close()

	var bindings []*Binding
	for rows.Next() {
		var b Binding
		if err := rows.Scan(&b.ID, &b.ParentID, &b.StudentID, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		bindings = append(bindings, &b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bindings: %w", err)
	}

	return bindings, nil
}

func (s *bindingStore) GetBindingByStudent(ctx context.Context, studentID int64) (*Binding, error) {
	query := `
		SELECT id, parent_id, student_id, created_at
		FROM user_bindings
		WHERE student_id = $1
	`

	var b Binding
	err := s.pool.QueryRow(ctx, query, studentID).Scan(
		&b.ID, &b.ParentID, &b.StudentID, &b.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("binding not found")
		}
		return nil, fmt.Errorf("get binding by student: %w", err)
	}

	return &b, nil
}

func (s *bindingStore) DeleteBinding(ctx context.Context, id int64) error {
	query := `DELETE FROM user_bindings WHERE id = $1`

	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete binding: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("binding not found")
	}

	return nil
}

func (s *bindingStore) BindingExistsForStudent(ctx context.Context, studentID int64) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM user_bindings WHERE student_id = $1)`

	var exists bool
	if err := s.pool.QueryRow(ctx, query, studentID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check binding exists: %w", err)
	}

	return exists, nil
}
