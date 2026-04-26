package candidate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityLogStore defines activity log data access operations.
type ActivityLogStore interface {
	Create(ctx context.Context, input *CreateActivityInput) (*ActivityLog, error)
	BatchCreate(ctx context.Context, inputs []*CreateActivityInput) error
	List(ctx context.Context, filter ActivityFilter, page, pageSize int) ([]*ActivityLog, int64, error)
	GetStats(ctx context.Context, targetType string, targetID int64) (int64, error)
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

type activityLogStore struct {
	pool *pgxpool.Pool
}

// NewActivityLogStore creates a new activity log store.
func NewActivityLogStore(pool *pgxpool.Pool) ActivityLogStore {
	return &activityLogStore{pool: pool}
}

func scanActivityLog(row pgx.Row) (*ActivityLog, error) {
	var l ActivityLog
	var rawMeta []byte
	if err := row.Scan(
		&l.ID, &l.UserID, &l.ActivityType, &l.TargetType, &l.TargetID,
		&rawMeta, &l.IPAddress, &l.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(rawMeta) > 0 {
		_ = json.Unmarshal(rawMeta, &l.Metadata)
	}
	return &l, nil
}

func (s *activityLogStore) Create(ctx context.Context, input *CreateActivityInput) (*ActivityLog, error) {
	meta, _ := json.Marshal(input.Metadata)
	var targetType *string
	if input.TargetType != "" {
		targetType = &input.TargetType
	}
	var targetID *int64
	if input.TargetID > 0 {
		targetID = &input.TargetID
	}
	var ipAddr *string
	if input.IPAddress != "" {
		ipAddr = &input.IPAddress
	}

	l, err := scanActivityLog(s.pool.QueryRow(ctx, `
		INSERT INTO candidate_activity_logs (user_id, activity_type, target_type, target_id, metadata, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, user_id, activity_type, target_type, target_id, metadata, ip_address, created_at
	`, input.UserID, input.ActivityType, targetType, targetID, meta, ipAddr))
	if err != nil {
		return nil, fmt.Errorf("create activity log: %w", err)
	}
	return l, nil
}

func (s *activityLogStore) BatchCreate(ctx context.Context, inputs []*CreateActivityInput) error {
	if len(inputs) == 0 {
		return nil
	}

	userIDs := make([]int64, len(inputs))
	activityTypes := make([]string, len(inputs))
	targetTypes := make([]*string, len(inputs))
	targetIDs := make([]*int64, len(inputs))
	metadatas := make([]string, len(inputs))
	ipAddrs := make([]*string, len(inputs))

	for i, input := range inputs {
		userIDs[i] = input.UserID
		activityTypes[i] = input.ActivityType
		if input.TargetType != "" {
			t := input.TargetType
			targetTypes[i] = &t
		}
		if input.TargetID > 0 {
			t := input.TargetID
			targetIDs[i] = &t
		}
		metaBytes, _ := json.Marshal(input.Metadata)
		metadatas[i] = string(metaBytes)
		if input.IPAddress != "" {
			ip := input.IPAddress
			ipAddrs[i] = &ip
		}
	}

	_, err := s.pool.CopyFrom(ctx, pgx.Identifier{"candidate_activity_logs"},
		[]string{"user_id", "activity_type", "target_type", "target_id", "metadata", "ip_address", "created_at"},
		pgx.CopyFromSlice(len(inputs), func(i int) ([]any, error) {
			return []any{
				userIDs[i], activityTypes[i], targetTypes[i], targetIDs[i],
				metadatas[i], ipAddrs[i], time.Now(),
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("batch create activity logs: %w", err)
	}
	return nil
}

func (s *activityLogStore) List(ctx context.Context, filter ActivityFilter, page, pageSize int) ([]*ActivityLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var conditions []string
	var args []any
	argNum := 1

	if filter.UserID > 0 {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argNum))
		args = append(args, filter.UserID)
		argNum++
	}
	if filter.ActivityType != "" {
		conditions = append(conditions, fmt.Sprintf("activity_type = $%d", argNum))
		args = append(args, filter.ActivityType)
		argNum++
	}
	if filter.TargetType != "" {
		conditions = append(conditions, fmt.Sprintf("target_type = $%d", argNum))
		args = append(args, filter.TargetType)
		argNum++
	}
	if filter.StartTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, *filter.StartTime)
		argNum++
	}
	if filter.EndTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argNum))
		args = append(args, *filter.EndTime)
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM candidate_activity_logs %s", whereClause)
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity logs: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, activity_type, target_type, target_id, metadata, ip_address, created_at
		FROM candidate_activity_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)
	args = append(args, pageSize, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list activity logs: %w", err)
	}
	defer rows.Close()

	logs := []*ActivityLog{}
	for rows.Next() {
		l, err := scanActivityLog(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan activity log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate activity logs: %w", err)
	}
	return logs, total, nil
}

func (s *activityLogStore) GetStats(ctx context.Context, targetType string, targetID int64) (int64, error) {
	var count int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM candidate_activity_logs WHERE target_type = $1 AND target_id = $2
	`, targetType, targetID).Scan(&count); err != nil {
		return 0, fmt.Errorf("get activity stats: %w", err)
	}
	return count, nil
}

func (s *activityLogStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result, err := s.pool.Exec(ctx, `
		DELETE FROM candidate_activity_logs WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return 0, fmt.Errorf("delete activity logs by ids: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *activityLogStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM candidate_activity_logs WHERE created_at < $1
	`, before)
	if err != nil {
		return 0, fmt.Errorf("delete activity logs before: %w", err)
	}
	return result.RowsAffected(), nil
}
