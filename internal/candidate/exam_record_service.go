package candidate

import (
	"context"
	"database/sql"
	"errors"

	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/jackc/pgx/v5"
)

// ExamRecordService defines exam record business operations.
type ExamRecordService interface {
	ListByProfile(ctx context.Context, userID, profileID int64) ([]*ExamRecordResponse, error)
	GetByID(ctx context.Context, userID, recordID int64) (*ExamRecordResponse, error)
	Create(ctx context.Context, userID, profileID int64, req CreateExamRecordRequest) (*ExamRecordResponse, error)
	Update(ctx context.Context, userID, recordID int64, req UpdateExamRecordRequest) (*ExamRecordResponse, error)
	Void(ctx context.Context, userID, recordID int64) error
	ListScoreHistories(ctx context.Context, userID, recordID int64) ([]*ScoreHistoryResponse, error)
}

type examRecordService struct {
	store        ExamRecordStore
	historyStore ScoreHistoryStore
	profileStore ProfileStore
	bindingStore user.BindingStore
	activityLog  ActivityLogService
}

// NewExamRecordService creates a new exam record service.
func NewExamRecordService(
	store ExamRecordStore,
	historyStore ScoreHistoryStore,
	profileStore ProfileStore,
	bindingStore user.BindingStore,
	activityLog ActivityLogService,
) ExamRecordService {
	return &examRecordService{
		store:        store,
		historyStore: historyStore,
		profileStore: profileStore,
		bindingStore: bindingStore,
		activityLog:  activityLog,
	}
}

func (s *examRecordService) ListByProfile(ctx context.Context, userID, profileID int64) ([]*ExamRecordResponse, error) {
	if ok, err := s.canAccessProfile(ctx, userID, profileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "无权访问该档案")
	}

	records, err := s.store.ListByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}

	ownerID, _ := s.profileStore.GetOwnerUserID(ctx, profileID)
	canWrite := userID == ownerID

	out := make([]*ExamRecordResponse, len(records))
	for i, r := range records {
		out[i] = toExamRecordResponse(r, canWrite)
	}
	return out, nil
}

func (s *examRecordService) GetByID(ctx context.Context, userID, recordID int64) (*ExamRecordResponse, error) {
	record, err := s.store.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return nil, err
	}

	if ok, err := s.canAccessProfile(ctx, userID, record.ProfileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "无权访问该档案")
	}

	ownerID, _ := s.profileStore.GetOwnerUserID(ctx, record.ProfileID)
	return toExamRecordResponse(record, userID == ownerID), nil
}

func (s *examRecordService) Create(ctx context.Context, userID, profileID int64, req CreateExamRecordRequest) (*ExamRecordResponse, error) {
	if !isValidExamModel(req.ExamModel) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的考试模式")
	}
	if req.ExamType != "" && !isValidExamType(req.ExamType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的考试类型")
	}
	if req.SectionType != "" && !isValidSectionType(req.SectionType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的科类")
	}
	if req.ArtType != "" && !isValidArtType(req.ArtType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的艺体类型")
	}

	if ok, err := s.isOwner(ctx, userID, profileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "仅档案所有者可录入成绩")
	}

	input := &createExamRecordInput{
		ProfileID:      profileID,
		ExamYear:       req.ExamYear,
		ExamModel:      req.ExamModel,
		ExamType:       req.ExamType,
		TotalScore:     sql.NullFloat64{Valid: req.TotalScore > 0, Float64: req.TotalScore},
		RankValue:      sql.NullInt32{Valid: req.RankValue > 0, Int32: req.RankValue},
		SectionType:    sql.NullString{Valid: req.SectionType != "", String: req.SectionType},
		SelectSubjects: req.SelectSubjects,
		SubjectScores:  req.SubjectScores,
		ArtScore:       sql.NullFloat64{Valid: req.ArtScore > 0, Float64: req.ArtScore},
		CultureScore:   sql.NullFloat64{Valid: req.CultureScore > 0, Float64: req.CultureScore},
		ArtType:        sql.NullString{Valid: req.ArtType != "", String: req.ArtType},
	}

	record, err := s.store.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	if s.activityLog != nil {
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "score_input",
			TargetType:   "exam_record",
			TargetID:     record.ID,
			Metadata: map[string]any{
				"profile_id":  profileID,
				"exam_year":   req.ExamYear,
				"exam_model":  req.ExamModel,
				"total_score": req.TotalScore,
			},
		})
	}

	return toExamRecordResponse(record, true), nil
}

func (s *examRecordService) Update(ctx context.Context, userID, recordID int64, req UpdateExamRecordRequest) (*ExamRecordResponse, error) {
	record, err := s.store.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return nil, err
	}

	if ok, err := s.isOwner(ctx, userID, record.ProfileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "仅档案所有者可修改成绩")
	}

	if req.ExamModel != "" && !isValidExamModel(req.ExamModel) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的考试模式")
	}
	if req.ExamType != "" && !isValidExamType(req.ExamType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的考试类型")
	}
	if req.SectionType != "" && !isValidSectionType(req.SectionType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的科类")
	}
	if req.ArtType != "" && !isValidArtType(req.ArtType) {
		return nil, web.NewError(web.ErrCodeBadRequest, "无效的艺体类型")
	}

	input := &updateExamRecordInput{}
	if req.ExamYear > 0 {
		input.ExamYear = &req.ExamYear
	}
	if req.ExamModel != "" {
		input.ExamModel = &req.ExamModel
	}
	if req.ExamType != "" {
		input.ExamType = &req.ExamType
	}
	if req.SectionType != "" {
		input.SectionType = &req.SectionType
	}
	if req.SelectSubjects != nil {
		input.SelectSubjects = req.SelectSubjects
	}
	if req.ArtType != "" {
		input.ArtType = &req.ArtType
	}
	if req.Verified != nil {
		input.Verified = req.Verified
	}

	// Detect score changes
	scoreChanged := false
	if req.TotalScore != nil {
		input.TotalScore = req.TotalScore
		if !record.TotalScore.Valid || record.TotalScore.Float64 != *req.TotalScore {
			scoreChanged = true
		}
	}
	if req.RankValue != nil {
		input.RankValue = req.RankValue
		if !record.RankValue.Valid || record.RankValue.Int32 != *req.RankValue {
			scoreChanged = true
		}
	}
	if req.SubjectScores != nil {
		input.SubjectScores = req.SubjectScores
		if !float64MapsEqual(record.SubjectScores, req.SubjectScores) {
			scoreChanged = true
		}
	}
	if req.ArtScore != nil {
		input.ArtScore = req.ArtScore
		if !record.ArtScore.Valid || record.ArtScore.Float64 != *req.ArtScore {
			scoreChanged = true
		}
	}
	if req.CultureScore != nil {
		input.CultureScore = req.CultureScore
		if !record.CultureScore.Valid || record.CultureScore.Float64 != *req.CultureScore {
			scoreChanged = true
		}
	}

	updated, err := s.store.Update(ctx, recordID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return nil, err
	}

	// Record history if any score field changed
	if scoreChanged {
		historyInput := &createScoreHistoryInput{
			ExamRecordID:       recordID,
			PrevTotalScore:     record.TotalScore,
			PrevRankValue:      record.RankValue,
			PrevSubjectScores:  record.SubjectScores,
			PrevSelectSubjects: record.SelectSubjects,
			NewTotalScore:      updated.TotalScore.Float64,
			NewRankValue:       updated.RankValue.Int32,
			NewSubjectScores:   updated.SubjectScores,
			NewSelectSubjects:  updated.SelectSubjects,
			ChangeReason:       sql.NullString{Valid: req.ChangeReason != "", String: req.ChangeReason},
			Source:             "manual",
		}
		_, _ = s.historyStore.Create(ctx, historyInput)
	}

	if s.activityLog != nil {
		fields := "basic_info"
		if scoreChanged {
			fields = "scores"
		}
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "score_modify",
			TargetType:   "exam_record",
			TargetID:     recordID,
			Metadata: map[string]any{
				"profile_id": record.ProfileID,
				"fields":     fields,
			},
		})
	}

	return toExamRecordResponse(updated, true), nil
}

func (s *examRecordService) Void(ctx context.Context, userID, recordID int64) error {
	record, err := s.store.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return err
	}

	if ok, err := s.isOwner(ctx, userID, record.ProfileID); err != nil {
		return err
	} else if !ok {
		return web.NewError(web.ErrCodeForbidden, "仅档案所有者可作废成绩")
	}

	if err := s.store.Void(ctx, recordID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return err
	}

	if s.activityLog != nil {
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "score_void",
			TargetType:   "exam_record",
			TargetID:     recordID,
			Metadata: map[string]any{
				"profile_id": record.ProfileID,
			},
		})
	}

	return nil
}

func (s *examRecordService) ListScoreHistories(ctx context.Context, userID, recordID int64) ([]*ScoreHistoryResponse, error) {
	record, err := s.store.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.NewError(web.ErrCodeNotFound, "考试记录不存在")
		}
		return nil, err
	}

	if ok, err := s.canAccessProfile(ctx, userID, record.ProfileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "无权访问该档案")
	}

	histories, err := s.historyStore.ListByExamRecord(ctx, recordID)
	if err != nil {
		return nil, err
	}

	out := make([]*ScoreHistoryResponse, len(histories))
	for i, h := range histories {
		out[i] = toScoreHistoryResponse(h)
	}
	return out, nil
}

// Permission helpers (same pattern as intention_service)

func (s *examRecordService) canAccessProfile(ctx context.Context, userID, profileID int64) (bool, error) {
	ownerID, err := s.profileStore.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return false, err
	}
	if userID == ownerID {
		return true, nil
	}
	if b, err := s.bindingStore.GetBindingByStudent(ctx, userID); err == nil {
		if b.ParentID == ownerID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	if b, err := s.bindingStore.GetBindingByStudent(ctx, ownerID); err == nil {
		if b.ParentID == userID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	return false, nil
}

func (s *examRecordService) isOwner(ctx context.Context, userID, profileID int64) (bool, error) {
	ownerID, err := s.profileStore.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return false, err
	}
	return userID == ownerID, nil
}
