package admission

import (
	"time"
)

// RichVolunteerPlan matches the frontend's RichVolunteerPlan interface for detailed export
type RichVolunteerPlan struct {
	ID          int64               `json:"id"`
	UserID      int64               `json:"userId"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Groups      []VolunteerPlanGroup `json:"groups"` 
	Stats       VolunteerPlanStats  `json:"stats"`  
    CreatedAt   time.Time           `json:"createdAt"` 
	UserDetails UserDetails         `json:"userDetails"`
	PlanStatistics PlanStatistics    `json:"planStatistics"`
	DetailedGroups []DetailedVolunteerPlanGroup `json:"detailedGroups"`
}

type UserDetails struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type PlanStatistics struct {
	TotalUniversities int               `json:"totalUniversities"`
	TotalGroups       int               `json:"totalGroups"`
	TotalMajors       int               `json:"totalMajors"`
	MajorDistribution map[string]int `json:"majorDistribution"` // e.g., { '计算机科学与技术': 10, '软件工程': 8 }
}

type ScoreTrend struct {
	Year    int `json:"year"`
	MinScore int `json:"minScore"`
	MinRank int `json:"minRank"`
}

type DetailedVolunteerPlanGroup struct {
	ID               int64                `json:"id"`
	PlanID           int64                `json:"planId"`
	OrderNo          int                  `json:"orderNo"`
	UniversityID     *int64               `json:"universityId,omitempty"`
	UniversityCode   string               `json:"universityCode"`
	UniversityName   string               `json:"universityName"`
	GroupID          *int64               `json:"groupId,omitempty"`
	GroupCode        string               `json:"groupCode"`
	GroupName        string               `json:"groupName"`
	IsObeyAdjustment bool                 `json:"isObeyAdjustment"`
	Remark           string               `json:"remark"`
	Majors           []VolunteerPlanMajor `json:"majors"` // Existing majors

	// Additional fields for rich export
	UniversityDetails UniversityDetails `json:"universityDetails"`
	GroupAdmissionDetails GroupAdmissionDetails `json:"groupAdmissionDetails"`
	DetailedMajors []DetailedVolunteerPlanMajor `json:"detailedMajors"`
}

type UniversityDetails struct {
	Is985          bool   `json:"is985"`
	Is211          bool   `json:"is211"`
	SchoolCategory string `json:"schoolCategory"`
	Region         string `json:"region"`
	// Add more university profile details as needed
}

type GroupAdmissionDetails struct {
	MinScore2024 int `json:"minScore2024"`
	MinScore2023 int `json:"minScore2023"`
	MinScore2022 int `json:"minScore2022"`
	// Add more admission group extension details
}

type DetailedVolunteerPlanMajor struct {
	MajorCode          string `json:"majorCode"`
	MajorName          string `json:"majorName"`
	MajorIntro         string `json:"majorIntro"`
	TrainingGoal       string `json:"trainingGoal"`
	EmploymentDirection string `json:"employmentDirection"`
	MinScore           int    `json:"minScore"`
	MinRank            int    `json:"minRank"`
	Tuition            int    `json:"tuition"`
	MajorOrder         int    `json:"majorOrder"` // Added for consistency with frontend
	// Add more university major admission/profile details
}
