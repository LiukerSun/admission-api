package volunteerplan

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanStore interface {
	// ListSummariesByUser 返回轻量摘要，不含 plan_json。前端列表用这一份；
	// 真正展开某个方案才走 GetByID 拉详情。
	ListSummariesByUser(ctx context.Context, userID int64) ([]*UserVolunteerPlanSummary, error)
	GetByID(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error)
	CreateFromDraft(ctx context.Context, userID, draftID int64, title string, planJSON []byte) (*UserVolunteerPlan, error)
	GetByDraftID(ctx context.Context, userID, draftID int64) (*UserVolunteerPlan, error)
	// UpdateMeta 部分更新 title / description（PATCH 语义）。
	// 任一参数为 nil 表示不动该字段。
	UpdateMeta(ctx context.Context, userID, planID int64, title, description *string) (*UserVolunteerPlan, error)
	// SoftDelete 把 deleted_at 置 NOW()。再次 SoftDelete 已删行返回 ErrPlanNotFound
	// （行还在，但所有 SELECT 都加了 deleted_at IS NULL 过滤，对调用方看不见）。
	SoftDelete(ctx context.Context, userID, planID int64) error
}

type planStore struct {
	pool *pgxpool.Pool
}

func NewPlanStore(pool *pgxpool.Pool) PlanStore {
	return &planStore{pool: pool}
}

func scanPlan(row pgx.Row) (*UserVolunteerPlan, error) {
	var p UserVolunteerPlan
	if err := row.Scan(
		&p.ID, &p.UserID, &p.Title, &p.Description,
		&p.SourceDraftID, &p.PlanJSON, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListSummariesByUser 用 plan_json->stats 字段直接取计数，避免拉全量 plan_json
// 再在 Go 端 unmarshal。schoolCount/groupCount 缺失（老数据没 stats）时回退 0。
// description 含进来：方案备注本身上限 500 字符，影响轻；列表里能显示真实持久值。
func (s *planStore) ListSummariesByUser(ctx context.Context, userID int64) ([]*UserVolunteerPlanSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			title,
			description,
			COALESCE((plan_json->'stats'->>'schoolCount')::int, 0) AS school_count,
			COALESCE((plan_json->'stats'->>'groupCount')::int, 0)  AS group_count,
			created_at,
			updated_at
		FROM user_volunteer_plans
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list plan summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*UserVolunteerPlanSummary
	for rows.Next() {
		var s UserVolunteerPlanSummary
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Description, &s.SchoolCount, &s.GroupCount, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan plan summary: %w", err)
		}
		summaries = append(summaries, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plan summaries: %w", err)
	}
	return summaries, nil
}

func (s *planStore) GetByID(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, description, source_draft_id, plan_json, created_at, updated_at
		FROM user_volunteer_plans
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, planID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get plan: %w", err)
	}
	return p, nil
}

func (s *planStore) GetByDraftID(ctx context.Context, userID, draftID int64) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		SELECT id, user_id, title, description, source_draft_id, plan_json, created_at, updated_at
		FROM user_volunteer_plans
		WHERE user_id = $1 AND source_draft_id = $2 AND deleted_at IS NULL
	`, userID, draftID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("get plan by draft: %w", err)
	}
	return p, nil
}

// CreateFromDraft 处理三种状态：
//
//	(a) 新 adopt        → INSERT 成功
//	(b) 已 adopt 活跃   → DO NOTHING 冲突 → 走 GetByDraftID 返回现有（幂等）
//	(c) 已 adopt 但软删 → DO NOTHING 冲突 → 走 restore 把 deleted_at 清空 +
//	                     用新 title/plan_json 覆盖（语义：再次 adopt = 恢复）
//
// UNIQUE(user_id, source_draft_id) 让 (b)(c) 都走冲突路径；二者用 deleted_at
// 区分。如果不处理 (c)，软删后用户在同一 draft 重新点 adopt 会拿到 404。
func (s *planStore) CreateFromDraft(ctx context.Context, userID, draftID int64, title string, planJSON []byte) (*UserVolunteerPlan, error) {
	// (a) 尝试新建
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		INSERT INTO user_volunteer_plans (user_id, title, source_draft_id, plan_json)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, source_draft_id) DO NOTHING
		RETURNING id, user_id, title, description, source_draft_id, plan_json, created_at, updated_at
	`, userID, title, draftID, planJSON))
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, fmt.Errorf("insert plan: %w", err)
		}
		return nil, fmt.Errorf("insert plan: %w", err)
	}

	// (c) 已存在的行可能是软删的——尝试恢复并用新 title/plan_json 覆盖。
	// WHERE deleted_at IS NOT NULL 保证只匹配软删行，活跃行不会被覆盖。
	restored, restoreErr := scanPlan(s.pool.QueryRow(ctx, `
		UPDATE user_volunteer_plans
		SET deleted_at = NULL,
		    title      = $3,
		    plan_json  = $4,
		    updated_at = NOW()
		WHERE user_id = $1 AND source_draft_id = $2 AND deleted_at IS NOT NULL
		RETURNING id, user_id, title, description, source_draft_id, plan_json, created_at, updated_at
	`, userID, draftID, title, planJSON))
	if restoreErr == nil {
		return restored, nil
	}
	if !errors.Is(restoreErr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("restore soft-deleted plan: %w", restoreErr)
	}

	// (b) 现有的是活跃行，幂等返回。
	return s.GetByDraftID(ctx, userID, draftID)
}

// UpdateMeta：用 COALESCE 实现 PATCH。传 nil 时该列保持原值；传 *string("") 则
// 会清空成空串。这样前端可以选择性更新单个字段（只改 title 或只改 description）。
func (s *planStore) UpdateMeta(ctx context.Context, userID, planID int64, title, description *string) (*UserVolunteerPlan, error) {
	p, err := scanPlan(s.pool.QueryRow(ctx, `
		UPDATE user_volunteer_plans
		SET
			title       = COALESCE($3, title),
			description = COALESCE($4, description),
			updated_at  = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, title, description, source_draft_id, plan_json, created_at, updated_at
	`, planID, userID, title, description))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("update plan meta: %w", err)
	}
	return p, nil
}

// SoftDelete 标记方案为已删除（deleted_at = NOW()）。幂等：对已删行再调一次
// 仍返回 ErrPlanNotFound（因为 WHERE 过滤掉了），调用方不会得到二次成功。
func (s *planStore) SoftDelete(ctx context.Context, userID, planID int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE user_volunteer_plans
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, planID, userID)
	if err != nil {
		return fmt.Errorf("soft delete plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPlanNotFound
	}
	return nil
}
