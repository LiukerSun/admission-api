package admission

import "time"

type VolunteerPlan struct {
	ID          int64                `json:"id"`
	UserID      int64                 `json:"userId"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Groups      []VolunteerPlanGroup `json:"groups"`
	Stats       VolunteerPlanStats   `json:"stats"`
	CreatedAt   time.Time            `json:"createdAt"`
}

type VolunteerPlanGroup struct {
	ID               int64                `json:"id"`
	OrderNo          int                  `json:"orderNo"`
	UniversityID     *int64               `json:"universityId,omitempty"`
	UniversityCode   string               `json:"universityCode"`
	UniversityName   string               `json:"universityName"`
	GroupID          *int64               `json:"groupId,omitempty"`
	GroupCode        string               `json:"groupCode"`
	GroupName        string               `json:"groupName"`
	IsObeyAdjustment bool                 `json:"isObeyAdjustment"`
	Remark           string               `json:"remark"`
	Majors           []VolunteerPlanMajor `json:"majors"`
}

type VolunteerPlanMajor struct {
	ID               int64  `json:"id"`
	MajorOrder       int    `json:"majorOrder"`
	MajorAdmissionID *int64 `json:"majorAdmissionId,omitempty"`
	MajorCode        string `json:"majorCode"`
	MajorName        string `json:"majorName"`
}

type VolunteerPlanStats struct {
	SchoolCount int `json:"schoolCount"`
	GroupCount  int `json:"groupCount"`
	RecordCount int `json:"recordCount"`
}

type VolunteerPlansResponse struct {
	Plans []VolunteerPlan `json:"plans"`
}
