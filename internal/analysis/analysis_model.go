package analysis

// TrendFilter defines query parameters for the university trend endpoint.
type TrendFilter struct {
	UniversityID   int64
	GroupCode      string
	LocalMajorCode string
	Years          int
}

// TrendResponse represents multi-year admission data for a university or major.
type TrendResponse struct {
	UniversityID   int64       `json:"university_id"`
	UniversityName string      `json:"university_name"`
	GroupCode      string      `json:"group_code,omitempty"`
	LocalMajorCode string      `json:"local_major_code,omitempty"`
	LocalMajorName string      `json:"local_major_name,omitempty"`
	Years          []TrendYear `json:"years"`
}

// TrendYear holds admission metrics for a single year.
type TrendYear struct {
	Year             int               `json:"year"`
	PlanCount        *int              `json:"plan_count,omitempty"`
	AdmittedCount    *int              `json:"admitted_count,omitempty"`
	MinScore         *int              `json:"min_score,omitempty"`
	MinRank          *int              `json:"min_rank,omitempty"`
	GroupMinScore    *int              `json:"group_min_score,omitempty"`
	GroupMinRank     *int              `json:"group_min_rank,omitempty"`
	EquivalentScores []EquivalentScore `json:"equivalent_scores,omitempty"`
}

// EquivalentScore maps a reference year to its equivalent score.
type EquivalentScore struct {
	ReferenceYear int  `json:"reference_year"`
	Score         *int `json:"score"`
}

// GroupComparisonFilter defines query parameters for the group comparison endpoint.
type GroupComparisonFilter struct {
	UniversityID        int64
	AdmissionYear       *int
	RegionCode          string
	SubjectCategoryCode string
}

// GroupComparisonResponse represents all admission groups for a university in a given year.
type GroupComparisonResponse struct {
	UniversityID   int64                 `json:"university_id"`
	UniversityName string                `json:"university_name"`
	AdmissionYear  int                   `json:"admission_year"`
	Groups         []GroupComparisonItem `json:"groups"`
}

// GroupComparisonItem holds aggregated metrics for a single admission group.
type GroupComparisonItem struct {
	GroupCode              string `json:"group_code"`
	GroupMajorNames        string `json:"group_major_names,omitempty"`
	SubjectRequirementName string `json:"subject_requirement_name,omitempty"`
	BatchName              string `json:"batch_name,omitempty"`
	PlanCount              int    `json:"plan_count"`
	AdmittedCount          *int   `json:"admitted_count,omitempty"`
	GroupMinScore          *int   `json:"group_min_score,omitempty"`
	GroupMinRank           *int   `json:"group_min_rank,omitempty"`
	MajorCount             int    `json:"major_count"`
}

// MajorDistributionFilter defines query parameters for the major distribution endpoint.
type MajorDistributionFilter struct {
	UniversityID        int64
	GroupCode           string
	AdmissionYear       *int
	RegionCode          string
	SubjectCategoryCode string
}

// MajorDistributionResponse represents major-level metrics within a single admission group.
type MajorDistributionResponse struct {
	UniversityID  int64                   `json:"university_id"`
	AdmissionYear int                     `json:"admission_year"`
	GroupCode     string                  `json:"group_code"`
	Majors        []MajorDistributionItem `json:"majors"`
}

// MajorDistributionItem holds per-major metrics for chart rendering.
type MajorDistributionItem struct {
	LocalMajorCode string `json:"local_major_code"`
	LocalMajorName string `json:"local_major_name"`
	PlanCount      *int   `json:"plan_count,omitempty"`
	AdmittedCount  *int   `json:"admitted_count,omitempty"`
	MinScore       *int   `json:"min_score,omitempty"`
	MinRank        *int   `json:"min_rank,omitempty"`
	Tuition        *int   `json:"tuition,omitempty"`
}

// MajorComparisonFilter defines query parameters for the cross-university major comparison endpoint.
type MajorComparisonFilter struct {
	LocalMajorName      string
	AdmissionYear       *int
	RegionCode          string
	SubjectCategoryCode string
	Limit               int
}

// MajorComparisonResponse represents the same major across multiple universities.
type MajorComparisonResponse struct {
	LocalMajorName string                `json:"local_major_name"`
	AdmissionYear  int                   `json:"admission_year"`
	Items          []MajorComparisonItem `json:"items"`
}

// MajorComparisonItem holds per-university metrics for a specific major.
type MajorComparisonItem struct {
	UniversityID       int64  `json:"university_id"`
	UniversityName     string `json:"university_name"`
	GroupCode          string `json:"group_code"`
	LocalMajorCode     string `json:"local_major_code"`
	PlanCount          *int   `json:"plan_count,omitempty"`
	AdmittedCount      *int   `json:"admitted_count,omitempty"`
	MinScore           *int   `json:"min_score,omitempty"`
	MinRank            *int   `json:"min_rank,omitempty"`
	EquivalentMinScore *int   `json:"equivalent_min_score,omitempty"`
}
