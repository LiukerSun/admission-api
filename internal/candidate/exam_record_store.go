package candidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExamRecordStore defines exam record data access operations.
type ExamRecordStore interface {
	ListByProfile(ctx context.Context, profileID int64) ([]*ExamRecord, error)
	GetByID(ctx context.Context, id int64) (*ExamRecord, error)
	Create(ctx context.Context, input *createExamRecordInput) (*ExamRecord, error)
	Update(ctx context.Context, id int64, input *updateExamRecordInput) (*ExamRecord, error)
	SetOtherRecordsNotCurrent(ctx context.Context, profileID int64, excludeID int64) error
	Void(ctx context.Context, id int64) error
}

// ScoreHistoryStore defines score history data access operations.
type ScoreHistoryStore interface {
	ListByExamRecord(ctx context.Context, examRecordID int64) ([]*ScoreHistory, error)
	Create(ctx context.Context, input *createScoreHistoryInput) (*ScoreHistory, error)
}

type examRecordStore struct {
	pool *pgxpool.Pool
}

type scoreHistoryStore struct {
	pool *pgxpool.Pool
}

// NewExamRecordStore creates a new exam record store.
func NewExamRecordStore(pool *pgxpool.Pool) ExamRecordStore {
	return &examRecordStore{pool: pool}
}

// NewScoreHistoryStore creates a new score history store.
func NewScoreHistoryStore(pool *pgxpool.Pool) ScoreHistoryStore {
	return &scoreHistoryStore{pool: pool}
}

func scanExamRecord(row pgx.Row) (*ExamRecord, error) {
	var r ExamRecord
	var rawSelectSubjects, rawSubjectScores []byte
	if err := row.Scan(
		&r.ID, &r.ProfileID, &r.ExamYear, &r.ExamModel, &r.ExamType,
		&r.TotalScore, &r.RankValue, &r.SectionType,
		&rawSelectSubjects, &rawSubjectScores,
		&r.ArtScore, &r.CultureScore, &r.ArtType,
		&r.IsCurrent, &r.Verified, &r.Status,
		&r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(rawSelectSubjects) > 0 {
		_ = json.Unmarshal(rawSelectSubjects, &r.SelectSubjects)
	}
	if len(rawSubjectScores) > 0 {
		_ = json.Unmarshal(rawSubjectScores, &r.SubjectScores)
	}
	return &r, nil
}

func (s *examRecordStore) ListByProfile(ctx context.Context, profileID int64) ([]*ExamRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, profile_id, exam_year, exam_model, exam_type,
		       total_score, rank_value, section_type,
		       select_subjects, subject_scores,
		       art_score, culture_score, art_type,
		       is_current, verified, status,
		       created_at, updated_at
		FROM candidate_exam_records
		WHERE profile_id = $1 AND status = 'active'
		ORDER BY is_current DESC, exam_year DESC, created_at DESC
	`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list exam records: %w", err)
	}
	defer rows.Close()

	out := []*ExamRecord{}
	for rows.Next() {
		r, err := scanExamRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan exam record: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exam records: %w", err)
	}
	return out, nil
}

func (s *examRecordStore) GetByID(ctx context.Context, id int64) (*ExamRecord, error) {
	r, err := scanExamRecord(s.pool.QueryRow(ctx, `
		SELECT id, profile_id, exam_year, exam_model, exam_type,
		       total_score, rank_value, section_type,
		       select_subjects, subject_scores,
		       art_score, culture_score, art_type,
		       is_current, verified, status,
		       created_at, updated_at
		FROM candidate_exam_records
		WHERE id = $1 AND status = 'active'
	`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get exam record: %w", err)
	}
	return r, nil
}

func (s *examRecordStore) Create(ctx context.Context, input *createExamRecordInput) (*ExamRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create exam record tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE candidate_exam_records
		SET is_current = false, updated_at = NOW()
		WHERE profile_id = $1 AND is_current = true
	`, input.ProfileID); err != nil {
		return nil, fmt.Errorf("set other records not current: %w", err)
	}

	selectSubjects, _ := json.Marshal(input.SelectSubjects)
	subjectScores, _ := json.Marshal(input.SubjectScores)

	r, err := scanExamRecord(tx.QueryRow(ctx, `
		INSERT INTO candidate_exam_records (
			profile_id, exam_year, exam_model, exam_type,
			total_score, rank_value, section_type,
			select_subjects, subject_scores,
			art_score, culture_score, art_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, profile_id, exam_year, exam_model, exam_type,
		       total_score, rank_value, section_type,
		       select_subjects, subject_scores,
		       art_score, culture_score, art_type,
		       is_current, verified, status,
		       created_at, updated_at
	`, input.ProfileID, input.ExamYear, input.ExamModel, input.ExamType,
		input.TotalScore, input.RankValue, input.SectionType,
		selectSubjects, subjectScores,
		input.ArtScore, input.CultureScore, input.ArtType))
	if err != nil {
		return nil, fmt.Errorf("create exam record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create exam record tx: %w", err)
	}
	return r, nil
}

func (s *examRecordStore) Update(ctx context.Context, id int64, input *updateExamRecordInput) (*ExamRecord, error) {
	selectSubjects, _ := json.Marshal(input.SelectSubjects)
	subjectScores, _ := json.Marshal(input.SubjectScores)

	var args []any
	var setClauses []string
	argNum := 1

	appendSet := func(field string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argNum))
		args = append(args, val)
		argNum++
	}

	if input.ExamYear != nil {
		appendSet("exam_year", *input.ExamYear)
	}
	if input.ExamModel != nil {
		appendSet("exam_model", *input.ExamModel)
	}
	if input.ExamType != nil {
		appendSet("exam_type", *input.ExamType)
	}
	if input.SectionType != nil {
		appendSet("section_type", *input.SectionType)
	}
	if input.SelectSubjects != nil {
		appendSet("select_subjects", selectSubjects)
	}
	if input.ArtType != nil {
		appendSet("art_type", *input.ArtType)
	}
	if input.Verified != nil {
		appendSet("verified", *input.Verified)
	}
	if input.TotalScore != nil {
		appendSet("total_score", *input.TotalScore)
	}
	if input.RankValue != nil {
		appendSet("rank_value", *input.RankValue)
	}
	if input.SubjectScores != nil {
		appendSet("subject_scores", subjectScores)
	}
	if input.ArtScore != nil {
		appendSet("art_score", *input.ArtScore)
	}
	if input.CultureScore != nil {
		appendSet("culture_score", *input.CultureScore)
	}

	if len(setClauses) == 0 {
		return s.GetByID(ctx, id)
	}

	appendSet("updated_at", "NOW()")
	args = append(args, id)

	setStr := ""
	for i, c := range setClauses {
		if i > 0 {
			setStr += ", "
		}
		setStr += c
	}

	query := fmt.Sprintf(`
		UPDATE candidate_exam_records
		SET %s
		WHERE id = $%d AND status = 'active'
		RETURNING id, profile_id, exam_year, exam_model, exam_type,
		       total_score, rank_value, section_type,
		       select_subjects, subject_scores,
		       art_score, culture_score, art_type,
		       is_current, verified, status,
		       created_at, updated_at
	`, setStr, argNum)

	r, err := scanExamRecord(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("update exam record: %w", err)
	}
	return r, nil
}

func (s *examRecordStore) SetOtherRecordsNotCurrent(ctx context.Context, profileID int64, excludeID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE candidate_exam_records
		SET is_current = false, updated_at = NOW()
		WHERE profile_id = $1 AND id != $2 AND is_current = true
	`, profileID, excludeID)
	if err != nil {
		return fmt.Errorf("set other records not current: %w", err)
	}
	return nil
}

func (s *examRecordStore) Void(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE candidate_exam_records
		SET status = 'void', is_current = false, updated_at = NOW()
		WHERE id = $1 AND status = 'active'
	`, id)
	if err != nil {
		return fmt.Errorf("void exam record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ScoreHistoryStore methods

func scanScoreHistory(row pgx.Row) (*ScoreHistory, error) {
	var h ScoreHistory
	var rawPrevSubjects, rawPrevScores, rawNewSubjects, rawNewScores []byte
	if err := row.Scan(
		&h.ID, &h.ExamRecordID,
		&h.PrevTotalScore, &h.PrevRankValue, &rawPrevScores, &rawPrevSubjects,
		&h.NewTotalScore, &h.NewRankValue, &rawNewScores, &rawNewSubjects,
		&h.ChangeReason, &h.Source, &h.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(rawPrevSubjects) > 0 {
		_ = json.Unmarshal(rawPrevSubjects, &h.PrevSelectSubjects)
	}
	if len(rawPrevScores) > 0 {
		_ = json.Unmarshal(rawPrevScores, &h.PrevSubjectScores)
	}
	if len(rawNewSubjects) > 0 {
		_ = json.Unmarshal(rawNewSubjects, &h.NewSelectSubjects)
	}
	if len(rawNewScores) > 0 {
		_ = json.Unmarshal(rawNewScores, &h.NewSubjectScores)
	}
	return &h, nil
}

func (s *scoreHistoryStore) ListByExamRecord(ctx context.Context, examRecordID int64) ([]*ScoreHistory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, exam_record_id,
		       prev_total_score, prev_rank_value, prev_subject_scores, prev_select_subjects,
		       new_total_score, new_rank_value, new_subject_scores, new_select_subjects,
		       change_reason, source, created_at
		FROM candidate_score_histories
		WHERE exam_record_id = $1
		ORDER BY created_at DESC
	`, examRecordID)
	if err != nil {
		return nil, fmt.Errorf("list score histories: %w", err)
	}
	defer rows.Close()

	out := []*ScoreHistory{}
	for rows.Next() {
		h, err := scanScoreHistory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan score history: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate score histories: %w", err)
	}
	return out, nil
}

func (s *scoreHistoryStore) Create(ctx context.Context, input *createScoreHistoryInput) (*ScoreHistory, error) {
	prevSubjects, _ := json.Marshal(input.PrevSelectSubjects)
	prevScores, _ := json.Marshal(input.PrevSubjectScores)
	newSubjects, _ := json.Marshal(input.NewSelectSubjects)
	newScores, _ := json.Marshal(input.NewSubjectScores)

	h, err := scanScoreHistory(s.pool.QueryRow(ctx, `
		INSERT INTO candidate_score_histories (
			exam_record_id,
			prev_total_score, prev_rank_value, prev_subject_scores, prev_select_subjects,
			new_total_score, new_rank_value, new_subject_scores, new_select_subjects,
			change_reason, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, exam_record_id,
		       prev_total_score, prev_rank_value, prev_subject_scores, prev_select_subjects,
		       new_total_score, new_rank_value, new_subject_scores, new_select_subjects,
		       change_reason, source, created_at
	`, input.ExamRecordID,
		input.PrevTotalScore, input.PrevRankValue, prevScores, prevSubjects,
		input.NewTotalScore, input.NewRankValue, newScores, newSubjects,
		input.ChangeReason, input.Source))
	if err != nil {
		return nil, fmt.Errorf("create score history: %w", err)
	}
	return h, nil
}
