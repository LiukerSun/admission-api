package candidate

import (
	"database/sql"
	"time"
)

// ExamRecord represents a candidate's exam score record.
type ExamRecord struct {
	ID             int64              `db:"id" json:"id"`
	ProfileID      int64              `db:"profile_id" json:"profile_id"`
	ExamYear       int16              `db:"exam_year" json:"exam_year"`
	ExamModel      string             `db:"exam_model" json:"exam_model"`
	ExamType       string             `db:"exam_type" json:"exam_type"`
	TotalScore     sql.NullFloat64    `db:"total_score" json:"total_score"`
	RankValue      sql.NullInt32      `db:"rank_value" json:"rank_value"`
	SectionType    sql.NullString     `db:"section_type" json:"section_type"`
	SelectSubjects []string           `db:"select_subjects" json:"select_subjects,omitempty"`
	SubjectScores  map[string]float64 `db:"subject_scores" json:"subject_scores,omitempty"`
	ArtScore       sql.NullFloat64    `db:"art_score" json:"art_score"`
	CultureScore   sql.NullFloat64    `db:"culture_score" json:"culture_score"`
	ArtType        sql.NullString     `db:"art_type" json:"art_type"`
	IsCurrent      bool               `db:"is_current" json:"is_current"`
	Verified       bool               `db:"verified" json:"verified"`
	Status         string             `db:"status" json:"status"`
	CreatedAt      time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `db:"updated_at" json:"updated_at"`
}

// ScoreHistory records a score modification snapshot.
type ScoreHistory struct {
	ID                 int64              `db:"id" json:"id"`
	ExamRecordID       int64              `db:"exam_record_id" json:"exam_record_id"`
	PrevTotalScore     sql.NullFloat64    `db:"prev_total_score" json:"prev_total_score,omitempty"`
	PrevRankValue      sql.NullInt32      `db:"prev_rank_value" json:"prev_rank_value,omitempty"`
	PrevSubjectScores  map[string]float64 `db:"prev_subject_scores" json:"prev_subject_scores,omitempty"`
	PrevSelectSubjects []string           `db:"prev_select_subjects" json:"prev_select_subjects,omitempty"`
	NewTotalScore      sql.NullFloat64    `db:"new_total_score" json:"new_total_score,omitempty"`
	NewRankValue       sql.NullInt32      `db:"new_rank_value" json:"new_rank_value,omitempty"`
	NewSubjectScores   map[string]float64 `db:"new_subject_scores" json:"new_subject_scores,omitempty"`
	NewSelectSubjects  []string           `db:"new_select_subjects" json:"new_select_subjects,omitempty"`
	ChangeReason       sql.NullString     `db:"change_reason" json:"change_reason,omitempty"`
	Source             string             `db:"source" json:"source"`
	CreatedAt          time.Time          `db:"created_at" json:"created_at"`
}

// --- Request / Response DTOs ---

// CreateExamRecordRequest is the request to create an exam record.
type CreateExamRecordRequest struct {
	ExamYear       int16              `json:"exam_year" validate:"required,min=2000,max=2100"`
	ExamModel      string             `json:"exam_model" validate:"required"`
	ExamType       string             `json:"exam_type" validate:"omitempty,oneof=tongkao yikao"`
	TotalScore     float64            `json:"total_score" validate:"omitempty,min=0,max=1000"`
	RankValue      int32              `json:"rank_value" validate:"omitempty,min=1"`
	SectionType    string             `json:"section_type" validate:"omitempty"`
	SelectSubjects []string           `json:"select_subjects" validate:"omitempty,dive,max=50"`
	SubjectScores  map[string]float64 `json:"subject_scores" validate:"omitempty"`
	ArtScore       float64            `json:"art_score" validate:"omitempty,min=0,max=1000"`
	CultureScore   float64            `json:"culture_score" validate:"omitempty,min=0,max=1000"`
	ArtType        string             `json:"art_type" validate:"omitempty"`
}

// UpdateExamRecordRequest updates basic and/or score fields.
// Score fields are optional; when any score field changes, a history snapshot is recorded.
type UpdateExamRecordRequest struct {
	ExamYear       int16              `json:"exam_year" validate:"omitempty,min=2000,max=2100"`
	ExamModel      string             `json:"exam_model" validate:"omitempty"`
	ExamType       string             `json:"exam_type" validate:"omitempty,oneof=tongkao yikao"`
	SectionType    string             `json:"section_type" validate:"omitempty"`
	SelectSubjects []string           `json:"select_subjects" validate:"omitempty,dive,max=50"`
	ArtType        string             `json:"art_type" validate:"omitempty"`
	Verified       *bool              `json:"verified" validate:"omitempty"`
	TotalScore     *float64           `json:"total_score" validate:"omitempty,min=0,max=1000"`
	RankValue      *int32             `json:"rank_value" validate:"omitempty,min=1"`
	SubjectScores  map[string]float64 `json:"subject_scores" validate:"omitempty"`
	ArtScore       *float64           `json:"art_score" validate:"omitempty,min=0,max=1000"`
	CultureScore   *float64           `json:"culture_score" validate:"omitempty,min=0,max=1000"`
	ChangeReason   string             `json:"change_reason" validate:"omitempty,max=255"`
}

// ExamRecordResponse is the exam record response DTO.
type ExamRecordResponse struct {
	ID             int64              `json:"id"`
	ProfileID      int64              `json:"profile_id"`
	ExamYear       int16              `json:"exam_year"`
	ExamModel      string             `json:"exam_model"`
	ExamType       string             `json:"exam_type"`
	TotalScore     *float64           `json:"total_score,omitempty"`
	RankValue      *int32             `json:"rank_value,omitempty"`
	SectionType    string             `json:"section_type,omitempty"`
	SelectSubjects []string           `json:"select_subjects,omitempty"`
	SubjectScores  map[string]float64 `json:"subject_scores,omitempty"`
	ArtScore       *float64           `json:"art_score,omitempty"`
	CultureScore   *float64           `json:"culture_score,omitempty"`
	ArtType        string             `json:"art_type,omitempty"`
	IsCurrent      bool               `json:"is_current"`
	Verified       bool               `json:"verified"`
	Status         string             `json:"status"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	CanWrite       bool               `json:"can_write"`
}

// ScoreHistoryResponse is the score history response DTO.
type ScoreHistoryResponse struct {
	ID                 int64              `json:"id"`
	ExamRecordID       int64              `json:"exam_record_id"`
	PrevTotalScore     *float64           `json:"prev_total_score,omitempty"`
	PrevRankValue      *int32             `json:"prev_rank_value,omitempty"`
	PrevSubjectScores  map[string]float64 `json:"prev_subject_scores,omitempty"`
	PrevSelectSubjects []string           `json:"prev_select_subjects,omitempty"`
	NewTotalScore      *float64           `json:"new_total_score,omitempty"`
	NewRankValue       *int32             `json:"new_rank_value,omitempty"`
	NewSubjectScores   map[string]float64 `json:"new_subject_scores,omitempty"`
	NewSelectSubjects  []string           `json:"new_select_subjects,omitempty"`
	ChangeReason       string             `json:"change_reason,omitempty"`
	Source             string             `json:"source"`
	CreatedAt          time.Time          `json:"created_at"`
}

// --- store inputs ---

type createExamRecordInput struct {
	ProfileID      int64
	ExamYear       int16
	ExamModel      string
	ExamType       string
	TotalScore     sql.NullFloat64
	RankValue      sql.NullInt32
	SectionType    sql.NullString
	SelectSubjects []string
	SubjectScores  map[string]float64
	ArtScore       sql.NullFloat64
	CultureScore   sql.NullFloat64
	ArtType        sql.NullString
}

type updateExamRecordInput struct {
	ExamYear       *int16
	ExamModel      *string
	ExamType       *string
	SectionType    *string
	SelectSubjects []string
	ArtType        *string
	Verified       *bool
	TotalScore     *float64
	RankValue      *int32
	SubjectScores  map[string]float64
	ArtScore       *float64
	CultureScore   *float64
}

type createScoreHistoryInput struct {
	ExamRecordID       int64
	PrevTotalScore     sql.NullFloat64
	PrevRankValue      sql.NullInt32
	PrevSubjectScores  map[string]float64
	PrevSelectSubjects []string
	NewTotalScore      sql.NullFloat64
	NewRankValue       sql.NullInt32
	NewSubjectScores   map[string]float64
	NewSelectSubjects  []string
	ChangeReason       sql.NullString
	Source             string
}

// --- helpers ---

func toExamRecordResponse(r *ExamRecord, canWrite bool) *ExamRecordResponse {
	resp := &ExamRecordResponse{
		ID:             r.ID,
		ProfileID:      r.ProfileID,
		ExamYear:       r.ExamYear,
		ExamModel:      r.ExamModel,
		ExamType:       r.ExamType,
		SectionType:    r.SectionType.String,
		SelectSubjects: r.SelectSubjects,
		SubjectScores:  r.SubjectScores,
		ArtType:        r.ArtType.String,
		IsCurrent:      r.IsCurrent,
		Verified:       r.Verified,
		Status:         r.Status,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
		CanWrite:       canWrite,
	}
	if r.TotalScore.Valid {
		resp.TotalScore = &r.TotalScore.Float64
	}
	if r.RankValue.Valid {
		resp.RankValue = &r.RankValue.Int32
	}
	if r.ArtScore.Valid {
		resp.ArtScore = &r.ArtScore.Float64
	}
	if r.CultureScore.Valid {
		resp.CultureScore = &r.CultureScore.Float64
	}
	return resp
}

func toScoreHistoryResponse(h *ScoreHistory) *ScoreHistoryResponse {
	resp := &ScoreHistoryResponse{
		ID:                 h.ID,
		ExamRecordID:       h.ExamRecordID,
		NewSubjectScores:   h.NewSubjectScores,
		NewSelectSubjects:  h.NewSelectSubjects,
		ChangeReason:       h.ChangeReason.String,
		Source:             h.Source,
		CreatedAt:          h.CreatedAt,
		PrevSubjectScores:  h.PrevSubjectScores,
		PrevSelectSubjects: h.PrevSelectSubjects,
	}
	if h.PrevTotalScore.Valid {
		resp.PrevTotalScore = &h.PrevTotalScore.Float64
	}
	if h.PrevRankValue.Valid {
		resp.PrevRankValue = &h.PrevRankValue.Int32
	}
	if h.NewTotalScore.Valid {
		resp.NewTotalScore = &h.NewTotalScore.Float64
	}
	if h.NewRankValue.Valid {
		resp.NewRankValue = &h.NewRankValue.Int32
	}
	return resp
}

func isValidExamModel(m string) bool {
	switch m {
	case "3+1+2", "3+3", "7选3", "wenli":
		return true
	}
	return false
}

func isValidExamType(t string) bool {
	switch t {
	case "tongkao", "yikao":
		return true
	}
	return false
}

func isValidSectionType(s string) bool {
	switch s {
	case "physics", "history", "science", "liberal_arts", "comprehensive":
		return true
	}
	return false
}

func isValidArtType(a string) bool {
	switch a {
	case "meishu", "yinyue", "wudao", "boyin", "tiyu":
		return true
	}
	return false
}

func float64MapsEqual(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
