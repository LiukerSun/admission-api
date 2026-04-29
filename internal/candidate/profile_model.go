package candidate

import (
	"errors"
	"time"
)

// Profile is the persisted candidate profile row.
//
// Sensitive ID-card fields (encrypted blob + HMAC hash) never appear in JSON
// responses; the service layer is responsible for translating to ProfileResponse
// with masked values.
type Profile struct {
	ID                   int64      `db:"id"`
	UserID               int64      `db:"user_id"`
	RealName             string     `db:"real_name"`
	CandidateIDCardEnc   []byte     `db:"candidate_id_card_enc"`
	CandidateIDCardHash  *string    `db:"candidate_id_card_hash"`
	CandidatePhone       *string    `db:"candidate_phone"`
	ProvinceID           int        `db:"province_id"`
	CityID               *int32     `db:"city_id"`
	CountyID             *int32     `db:"county_id"`
	GraduationSchoolName *string    `db:"graduation_school_name"`
	Grade                int16      `db:"grade"`
	CandidateType        string     `db:"candidate_type"`
	Gender               *string    `db:"gender"`
	Ethnicity            *string    `db:"ethnicity"`
	ColorVision          *string    `db:"color_vision"`
	Status               string     `db:"status"`
	IsDeleted            bool       `db:"is_deleted"`
	DeletedAt            *time.Time `db:"deleted_at"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

// ProfileResponse is the masked, JSON-friendly view returned to clients.
type ProfileResponse struct {
	ID                   int64     `json:"id"`
	UserID               int64     `json:"user_id"`
	RealName             string    `json:"real_name"`
	IDCardMasked         string    `json:"id_card_masked,omitempty"`
	PhoneMasked          string    `json:"phone_masked,omitempty"`
	ProvinceID           int       `json:"province_id"`
	CityID               *int32    `json:"city_id,omitempty"`
	CountyID             *int32    `json:"county_id,omitempty"`
	GraduationSchoolName *string   `json:"graduation_school_name,omitempty"`
	Grade                int16     `json:"grade"`
	CandidateType        string    `json:"candidate_type"`
	Gender               *string   `json:"gender,omitempty"`
	Ethnicity            *string   `json:"ethnicity,omitempty"`
	ColorVision          *string   `json:"color_vision,omitempty"`
	Status               string    `json:"status"`
	CanWrite             bool      `json:"can_write"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ProfileListResponse wraps a list of profiles for response.
type ProfileListResponse struct {
	Profiles []*ProfileResponse `json:"profiles"`
	Total    int                `json:"total"`
}

// CreateProfileRequest is the client-supplied payload for creating a profile.
type CreateProfileRequest struct {
	RealName             string `json:"real_name" validate:"required,max=50"`
	CandidateIDCard      string `json:"candidate_id_card,omitempty" validate:"omitempty,len=18"`
	CandidatePhone       string `json:"candidate_phone,omitempty" validate:"omitempty,len=11"`
	ProvinceID           int    `json:"province_id" validate:"required,min=1"`
	CityID               *int32 `json:"city_id,omitempty"`
	CountyID             *int32 `json:"county_id,omitempty"`
	GraduationSchoolName string `json:"graduation_school_name,omitempty" validate:"max=128"`
	Grade                int16  `json:"grade,omitempty" validate:"omitempty,min=1,max=4"`
	CandidateType        string `json:"candidate_type,omitempty" validate:"omitempty,oneof=regular yikao tiyao"`
	Gender               string `json:"gender,omitempty" validate:"omitempty,oneof=male female"`
	Ethnicity            string `json:"ethnicity,omitempty" validate:"max=32"`
	ColorVision          string `json:"color_vision,omitempty" validate:"omitempty,oneof=normal red_green total weak"`
	Status               string `json:"status,omitempty" validate:"omitempty,oneof=active inactive archived"`
}

// UpdateProfileRequest carries optional partial-update fields.
// Pointer fields distinguish "not provided" from "set to empty".
type UpdateProfileRequest struct {
	RealName             *string `json:"real_name,omitempty" validate:"omitempty,max=50"`
	CandidateIDCard      *string `json:"candidate_id_card,omitempty" validate:"omitempty,len=18"`
	CandidatePhone       *string `json:"candidate_phone,omitempty" validate:"omitempty,len=11"`
	ProvinceID           *int    `json:"province_id,omitempty" validate:"omitempty,min=1"`
	CityID               *int32  `json:"city_id,omitempty"`
	CountyID             *int32  `json:"county_id,omitempty"`
	GraduationSchoolName *string `json:"graduation_school_name,omitempty" validate:"omitempty,max=128"`
	Grade                *int16  `json:"grade,omitempty" validate:"omitempty,min=1,max=4"`
	CandidateType        *string `json:"candidate_type,omitempty" validate:"omitempty,oneof=regular yikao tiyao"`
	Gender               *string `json:"gender,omitempty" validate:"omitempty,oneof=male female"`
	Ethnicity            *string `json:"ethnicity,omitempty" validate:"omitempty,max=32"`
	ColorVision          *string `json:"color_vision,omitempty" validate:"omitempty,oneof=normal red_green total weak"`
	Status               *string `json:"status,omitempty" validate:"omitempty,oneof=active inactive archived"`
}

// LookupRequest variants for the three lookup endpoints.
type LookupByIDCardRequest struct {
	IDCard string `json:"id_card" validate:"required,len=18"`
}

type LookupByPhoneRequest struct {
	Phone string `json:"phone" validate:"required,len=11"`
}

type LookupByCodeRequest struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

// LookupResponse is returned by the three lookup endpoints. It exposes the
// minimal information needed for the client to subsequently call /api/v1/bindings.
type LookupResponse struct {
	ProfileID      int64  `json:"profile_id"`
	OwnerUserID    int64  `json:"owner_user_id"`
	OwnerEmail     string `json:"owner_email"`
	OwnerUserType  string `json:"owner_user_type"`
	RealNameMasked string `json:"real_name_masked"`
	PhoneMasked    string `json:"phone_masked,omitempty"`
}

// InviteResponse is returned when the owner generates a binding code.
type InviteResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateProfileInput is the store-layer write input. Hash and encrypted
// blob are pre-computed by the service layer.
type CreateProfileInput struct {
	UserID               int64
	RealName             string
	CandidateIDCardEnc   []byte
	CandidateIDCardHash  *string
	CandidatePhone       *string
	ProvinceID           int
	CityID               *int32
	CountyID             *int32
	GraduationSchoolName *string
	Grade                int16
	CandidateType        string
	Gender               *string
	Ethnicity            *string
	ColorVision          *string
	Status               string
}

// UpdateProfileInput is the store-layer dynamic-update input.
//
// A nil pointer means "do not touch the column". To set a nullable column to
// SQL NULL, the service layer should pass a non-nil pointer to a sentinel zero
// value handled at the store level. For this M1 we don't expose that path.
type UpdateProfileInput struct {
	RealName             *string
	CandidateIDCardEnc   []byte // when len > 0, written; sentinel: pass empty + UpdateIDCardHash to clear
	CandidateIDCardHash  *string
	UpdateIDCardFields   bool // true when the caller intends to update both enc and hash columns together
	CandidatePhone       *string
	ProvinceID           *int
	CityID               *int32
	CountyID             *int32
	GraduationSchoolName *string
	Grade                *int16
	CandidateType        *string
	Gender               *string
	Ethnicity            *string
	ColorVision          *string
	Status               *string
}

// Sentinel errors raised by the service layer.
var (
	ErrProfileNotFound        = errors.New("candidate profile not found")
	ErrProfileForbidden       = errors.New("forbidden: not owner or bound user")
	ErrProfileLookupForbidden = errors.New("forbidden: only student/parent can lookup profiles")
	ErrInviteCodeNotFound     = errors.New("invite code not found or expired")
)
