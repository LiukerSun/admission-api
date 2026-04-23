package analysis

import "time"

const (
	defaultPage    = 1
	defaultPerPage = 20
	maxPerPage     = 100
)

// ListResponse is the standard payload for analysis list endpoints.
type ListResponse[T any] struct {
	Items   []T            `json:"items"`
	Total   int64          `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
	HasMore bool           `json:"has_more"`
	Facets  map[string]any `json:"facets,omitempty"`
}

type FacetValue struct {
	Value any    `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type IncludeQuery struct {
	Include string `form:"include"`
}

type PageQuery struct {
	Page    int `form:"page"`
	PerPage int `form:"per_page"`
}

type DatasetOverviewQuery struct {
	IncludeTables   bool `form:"include_tables"`
	IncludeCoverage bool `form:"include_coverage"`
	IncludeImports  bool `form:"include_imports"`
}

type DatasetOverviewResponse struct {
	Summary          DatasetSummary             `json:"summary"`
	TableCounts      map[string]int64           `json:"table_counts,omitempty"`
	Coverage         map[string]DatasetCoverage `json:"coverage,omitempty"`
	LatestImports    []ImportLogItem            `json:"latest_imports,omitempty"`
	LatestImportedAt *time.Time                 `json:"latest_imported_at,omitempty"`
}

type DatasetSummary struct {
	ProvinceCount             int64 `json:"province_count"`
	SchoolCount               int64 `json:"school_count"`
	MajorCount                int64 `json:"major_count"`
	SchoolProfileCount        int64 `json:"school_profile_count"`
	MajorProfileCount         int64 `json:"major_profile_count"`
	EnrollmentPlanCount       int64 `json:"enrollment_plan_count"`
	SchoolAdmissionScoreCount int64 `json:"school_admission_score_count"`
	MajorAdmissionScoreCount  int64 `json:"major_admission_score_count"`
	ProvinceBatchLineCount    int64 `json:"province_batch_line_count"`
}

type DatasetCoverage struct {
	YearMin       *int `json:"year_min"`
	YearMax       *int `json:"year_max"`
	YearCount     int  `json:"year_count"`
	ProvinceCount int  `json:"province_count"`
	SchoolCount   int  `json:"school_count,omitempty"`
	MajorCount    int  `json:"major_count,omitempty"`
}

type ImportLogItem struct {
	SourceSystem string    `json:"source_system"`
	SourceTable  string    `json:"source_table"`
	FileName     string    `json:"file_name"`
	RowCount     *int64    `json:"row_count"`
	ImportedAt   time.Time `json:"imported_at"`
	Remark       *string   `json:"remark,omitempty"`
}

type FacetsQuery struct {
	Scope      string `form:"scope"`
	Fields     string `form:"fields"`
	Province   string `form:"province"`
	ProvinceID string `form:"province_id"`
	Year       string `form:"year"`
	SchoolID   string `form:"school_id"`
	MajorName  string `form:"major_name"`
	Batch      string `form:"batch"`
	Section    string `form:"section"`
}

type FacetsResponse struct {
	Scope  string                  `json:"scope"`
	Facets map[string][]FacetValue `json:"facets"`
}

type SchoolListQuery struct {
	PageQuery
	Q                 string   `form:"q"`
	SchoolID          string   `form:"school_id"`
	ProvinceID        string   `form:"province_id"`
	Province          string   `form:"province"`
	CityCode          string   `form:"city_code"`
	City              string   `form:"city"`
	SchoolType        string   `form:"school_type"`
	SchoolLevel       string   `form:"school_level"`
	SchoolNature      string   `form:"school_nature"`
	Department        string   `form:"department"`
	Tags              string   `form:"tags"`
	RankingSource     string   `form:"ranking_source"`
	RankingMin        *int     `form:"ranking_min"`
	RankingMax        *int     `form:"ranking_max"`
	EmploymentRateMin *float64 `form:"employment_rate_min"`
	EmploymentRateMax *float64 `form:"employment_rate_max"`
	CompositeScoreMin *float64 `form:"composite_score_min"`
	CompositeScoreMax *float64 `form:"composite_score_max"`
	Include           string   `form:"include"`
	Facets            string   `form:"facets"`
	Sort              string   `form:"sort"`
	SourceSystem      string   `form:"source_system"`
}

type SchoolDetailQuery struct {
	Include    string `form:"include"`
	Province   string `form:"province"`
	ProvinceID string `form:"province_id"`
	Year       int    `form:"year"`
}

type SchoolCompareQuery struct {
	SchoolIDs     string `form:"school_ids"`
	Province      string `form:"province"`
	ProvinceID    string `form:"province_id"`
	Year          int    `form:"year"`
	RankingSource string `form:"ranking_source"`
	Include       string `form:"include"`
}

type School struct {
	SchoolID     int64               `json:"school_id"`
	SchoolName   string              `json:"school_name"`
	SchoolCode   *string             `json:"school_code"`
	ProvinceID   *int                `json:"province_id"`
	ProvinceName *string             `json:"province_name"`
	CityCode     *int                `json:"city_code"`
	CityName     *string             `json:"city_name"`
	LogoURL      *string             `json:"logo_url"`
	Profile      *SchoolProfile      `json:"profile,omitempty"`
	Tags         []PolicyTag         `json:"tags,omitempty"`
	Rankings     []SchoolRanking     `json:"rankings,omitempty"`
	ScoreSummary *ScoreSummary       `json:"score_summary,omitempty"`
	PlanSummary  *PlanSummary        `json:"plan_summary,omitempty"`
	MajorSummary *SchoolMajorSummary `json:"major_summary,omitempty"`
}

type SchoolProfile struct {
	AliasName        *string  `json:"alias_name,omitempty"`
	FormerName       *string  `json:"former_name,omitempty"`
	FoundedYear      *int     `json:"founded_year,omitempty"`
	Address          *string  `json:"address,omitempty"`
	WebsiteURL       *string  `json:"website_url,omitempty"`
	AdmissionSiteURL *string  `json:"admission_site_url,omitempty"`
	Phone            *string  `json:"phone,omitempty"`
	Email            *string  `json:"email,omitempty"`
	Description      *string  `json:"description,omitempty"`
	LearningIndex    *float64 `json:"learning_index,omitempty"`
	LifeIndex        *float64 `json:"life_index,omitempty"`
	EmploymentIndex  *float64 `json:"employment_index,omitempty"`
	CompositeScore   *float64 `json:"composite_score,omitempty"`
	EmploymentRate   *float64 `json:"employment_rate,omitempty"`
	MaleRatio        *float64 `json:"male_ratio,omitempty"`
	FemaleRatio      *float64 `json:"female_ratio,omitempty"`
	ChinaRate        *float64 `json:"china_rate,omitempty"`
	AbroadRate       *float64 `json:"abroad_rate,omitempty"`
}

type PolicyTag struct {
	TagType       string `json:"tag_type"`
	TagValue      string `json:"tag_value"`
	EffectiveYear int    `json:"effective_year,omitempty"`
	ExpireYear    *int   `json:"expire_year,omitempty"`
}

type SchoolRanking struct {
	RankingSource string `json:"ranking_source"`
	RankingYear   int    `json:"ranking_year"`
	RankValue     int    `json:"rank_value"`
}

type ScoreSummary struct {
	AvailableYears     []int        `json:"available_years"`
	AvailableProvinces []string     `json:"available_provinces"`
	LowestScoreMin     *float64     `json:"lowest_score_min"`
	LowestScoreMax     *float64     `json:"lowest_score_max"`
	LowestRankMin      *int64       `json:"lowest_rank_min"`
	LowestRankMax      *int64       `json:"lowest_rank_max"`
	DataQuality        *DataQuality `json:"data_quality,omitempty"`
}

type PlanSummary struct {
	AvailableYears     []int    `json:"available_years"`
	AvailableProvinces []string `json:"available_provinces"`
	PlanCountTotal     *int64   `json:"plan_count_total"`
}

type SchoolMajorSummary struct {
	SchoolCount int64    `json:"school_count"`
	TopSchools  []School `json:"top_schools,omitempty"`
}

type MajorListQuery struct {
	PageQuery
	Q              string   `form:"q"`
	MajorID        string   `form:"major_id"`
	MajorCode      string   `form:"major_code"`
	MajorSubject   string   `form:"major_subject"`
	MajorCategory  string   `form:"major_category"`
	DegreeName     string   `form:"degree_name"`
	StudyYears     string   `form:"study_years"`
	Tags           string   `form:"tags"`
	SalaryMin      *float64 `form:"salary_min"`
	SalaryMax      *float64 `form:"salary_max"`
	FreshSalaryMin *float64 `form:"fresh_salary_min"`
	FreshSalaryMax *float64 `form:"fresh_salary_max"`
	WorkArea       string   `form:"work_area"`
	WorkIndustry   string   `form:"work_industry"`
	WorkJob        string   `form:"work_job"`
	Include        string   `form:"include"`
	Facets         string   `form:"facets"`
	Sort           string   `form:"sort"`
	SourceSystem   string   `form:"source_system"`
}

type MajorDetailQuery struct {
	Include    string `form:"include"`
	Province   string `form:"province"`
	ProvinceID string `form:"province_id"`
	Year       int    `form:"year"`
}

type Major struct {
	MajorID        int64               `json:"major_id"`
	MajorCode      *string             `json:"major_code"`
	MajorName      string              `json:"major_name"`
	MajorSubject   *string             `json:"major_subject"`
	MajorCategory  *string             `json:"major_category"`
	DegreeName     *string             `json:"degree_name"`
	StudyYearsText *string             `json:"study_years_text"`
	Profile        *MajorProfile       `json:"profile,omitempty"`
	Employment     *MajorEmployment    `json:"employment,omitempty"`
	Tags           []PolicyTag         `json:"tags,omitempty"`
	SchoolSummary  *SchoolMajorSummary `json:"school_summary,omitempty"`
	ScoreSummary   *ScoreSummary       `json:"score_summary,omitempty"`
	PlanSummary    *PlanSummary        `json:"plan_summary,omitempty"`
}

type MajorProfile struct {
	IntroText          *string  `json:"intro_text,omitempty"`
	CourseText         *string  `json:"course_text,omitempty"`
	JobText            *string  `json:"job_text,omitempty"`
	SelectSuggests     *string  `json:"select_suggests,omitempty"`
	AverageSalary      *float64 `json:"average_salary,omitempty"`
	FreshAverageSalary *float64 `json:"fresh_average_salary,omitempty"`
}

type MajorEmployment struct {
	SalaryInfos    any `json:"salary_infos,omitempty"`
	WorkAreas      any `json:"work_areas,omitempty"`
	WorkIndustries any `json:"work_industries,omitempty"`
	WorkJobs       any `json:"work_jobs,omitempty"`
}

type SchoolMajorsQuery struct {
	PageQuery
	Q            string `form:"q"`
	MajorCode    string `form:"major_code"`
	ObservedYear int    `form:"observed_year"`
	DegreeName   string `form:"degree_name"`
	MajorSubject string `form:"major_subject"`
	Province     string `form:"province"`
	ProvinceID   string `form:"province_id"`
	Year         int    `form:"year"`
	Include      string `form:"include"`
	Sort         string `form:"sort"`
}

type SchoolMajorItem struct {
	SchoolMajorID   int64                `json:"school_major_id"`
	SchoolID        int64                `json:"school_id"`
	MajorID         *int64               `json:"major_id"`
	MajorCode       *string              `json:"major_code"`
	MajorName       *string              `json:"major_name"`
	SchoolMajorName string               `json:"school_major_name"`
	StudyYearsText  *string              `json:"study_years_text"`
	ObservedYear    *int                 `json:"observed_year"`
	MajorProfile    *MajorProfile        `json:"major_profile,omitempty"`
	LatestPlan      *EnrollmentPlan      `json:"latest_plan,omitempty"`
	LatestScore     *MajorAdmissionScore `json:"latest_score,omitempty"`
}

type EnrollmentPlanQuery struct {
	PageQuery
	Q              string   `form:"q"`
	ProvinceID     string   `form:"province_id"`
	Province       string   `form:"province"`
	Year           string   `form:"year"`
	YearMin        *int     `form:"year_min"`
	YearMax        *int     `form:"year_max"`
	SchoolID       string   `form:"school_id"`
	SchoolName     string   `form:"school_name"`
	SchoolTags     string   `form:"school_tags"`
	MajorID        string   `form:"major_id"`
	MajorName      string   `form:"major_name"`
	MajorCode      string   `form:"major_code"`
	Batch          string   `form:"batch"`
	Section        string   `form:"section"`
	AdmissionType  string   `form:"admission_type"`
	MajorGroup     string   `form:"major_group"`
	SubjectReq     string   `form:"subject_req"`
	FirstSubject   string   `form:"first_subject"`
	SecondSubjects string   `form:"second_subjects"`
	PlanCountMin   *int     `form:"plan_count_min"`
	PlanCountMax   *int     `form:"plan_count_max"`
	TuitionMin     *float64 `form:"tuition_min"`
	TuitionMax     *float64 `form:"tuition_max"`
	Include        string   `form:"include"`
	Facets         string   `form:"facets"`
	Sort           string   `form:"sort"`
	SourceSystem   string   `form:"source_system"`
}

type EnrollmentPlan struct {
	EnrollmentPlanID   int64       `json:"enrollment_plan_id"`
	ID                 int64       `json:"id"`
	SchoolID           int64       `json:"school_id"`
	SchoolName         string      `json:"school_name"`
	ProvinceID         int         `json:"province_id"`
	ProvinceName       string      `json:"province_name"`
	Province           string      `json:"province"`
	PolicyID           int64       `json:"policy_id"`
	SchoolMajorGroupID *int64      `json:"school_major_group_id"`
	PlanYear           int         `json:"plan_year"`
	Year               int         `json:"year"`
	RawBatchName       *string     `json:"raw_batch_name"`
	RawSectionName     *string     `json:"raw_section_name"`
	RawAdmissionType   *string     `json:"raw_admission_type"`
	RawMajorGroupName  *string     `json:"raw_major_group_name"`
	RawElectiveReq     *string     `json:"raw_elective_req"`
	SchoolMajorID      *int64      `json:"school_major_id"`
	MajorID            *int64      `json:"major_id"`
	SchoolMajorName    *string     `json:"school_major_name"`
	MajorName          string      `json:"major_name"`
	MajorCode          *string     `json:"major_code"`
	PlanCount          *int        `json:"plan_count"`
	TuitionFee         *float64    `json:"tuition_fee"`
	StudyYearsText     *string     `json:"study_years_text"`
	SchoolCode         *string     `json:"school_code"`
	MajorPlanCode      *string     `json:"major_plan_code"`
	SourceSystem       string      `json:"source_system"`
	SourceTable        string      `json:"source_table"`
	Batch              string      `json:"batch"`
	SubjectRequire     string      `json:"subject_require"`
	Tags               []PolicyTag `json:"tags,omitempty"`
}

type EnrollmentPlanResponse struct {
	Items   []EnrollmentPlan `json:"items"`
	Data    []EnrollmentPlan `json:"data,omitempty"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
	HasMore bool             `json:"has_more"`
	Facets  map[string]any   `json:"facets,omitempty"`
}

type BatchLineQuery struct {
	PageQuery
	ProvinceID   string   `form:"province_id"`
	Province     string   `form:"province"`
	Year         string   `form:"year"`
	YearMin      *int     `form:"year_min"`
	YearMax      *int     `form:"year_max"`
	Batch        string   `form:"batch"`
	Category     string   `form:"category"`
	Section      string   `form:"section"`
	ScoreMin     *float64 `form:"score_min"`
	ScoreMax     *float64 `form:"score_max"`
	SourceSystem string   `form:"source_system"`
	Facets       string   `form:"facets"`
	Sort         string   `form:"sort"`
}

type ProvinceBatchLine struct {
	ProvinceBatchLineID int64   `json:"province_batch_line_id"`
	ProvinceID          int     `json:"province_id"`
	ProvinceName        string  `json:"province_name"`
	PolicyID            int64   `json:"policy_id"`
	ScoreYear           int     `json:"score_year"`
	RawBatchName        *string `json:"raw_batch_name"`
	RawCategoryName     *string `json:"raw_category_name"`
	RawSectionName      *string `json:"raw_section_name"`
	ScoreValue          float64 `json:"score_value"`
	RankValue           *int64  `json:"rank_value"`
	SourceSystem        string  `json:"source_system"`
	SourceTable         string  `json:"source_table"`
}

type BatchLineTrendQuery struct {
	ProvinceID   string `form:"province_id"`
	Province     string `form:"province"`
	Batch        string `form:"batch"`
	Category     string `form:"category"`
	Section      string `form:"section"`
	YearMin      *int   `form:"year_min"`
	YearMax      *int   `form:"year_max"`
	SourceSystem string `form:"source_system"`
}

type BatchLineTrendResponse struct {
	ProvinceID   int                   `json:"province_id"`
	ProvinceName string                `json:"province_name"`
	Batch        string                `json:"batch"`
	Category     *string               `json:"category"`
	Section      *string               `json:"section"`
	Series       []BatchLineTrendPoint `json:"series"`
}

type BatchLineTrendPoint struct {
	Year         int     `json:"year"`
	ScoreValue   float64 `json:"score_value"`
	RankValue    *int64  `json:"rank_value"`
	SourceSystem string  `json:"source_system,omitempty"`
}

type ScoreListQuery struct {
	PageQuery
	Q                 string   `form:"q"`
	ProvinceID        string   `form:"province_id"`
	Province          string   `form:"province"`
	Year              string   `form:"year"`
	YearMin           *int     `form:"year_min"`
	YearMax           *int     `form:"year_max"`
	SchoolID          string   `form:"school_id"`
	SchoolName        string   `form:"school_name"`
	SchoolTags        string   `form:"school_tags"`
	MajorID           string   `form:"major_id"`
	MajorName         string   `form:"major_name"`
	MajorCode         string   `form:"major_code"`
	Batch             string   `form:"batch"`
	Section           string   `form:"section"`
	AdmissionType     string   `form:"admission_type"`
	MajorGroup        string   `form:"major_group"`
	SubjectReq        string   `form:"subject_req"`
	ScoreMin          *float64 `form:"score_min"`
	ScoreMax          *float64 `form:"score_max"`
	RankMin           *int     `form:"rank_min"`
	RankMax           *int     `form:"rank_max"`
	LineDeviationMin  *float64 `form:"line_deviation_min"`
	LineDeviationMax  *float64 `form:"line_deviation_max"`
	HasRank           bool     `form:"has_rank"`
	HasAverageScore   bool     `form:"has_average_score"`
	IncludeZeroScores bool     `form:"include_zero_scores"`
	Include           string   `form:"include"`
	Facets            string   `form:"facets"`
	Sort              string   `form:"sort"`
	SourceSystem      string   `form:"source_system"`
}

type SchoolAdmissionScore struct {
	SchoolAdmissionScoreID int64       `json:"school_admission_score_id"`
	SchoolID               int64       `json:"school_id"`
	SchoolName             string      `json:"school_name"`
	ProvinceID             int         `json:"province_id"`
	ProvinceName           string      `json:"province_name"`
	PolicyID               int64       `json:"policy_id"`
	SchoolMajorGroupID     *int64      `json:"school_major_group_id"`
	AdmissionYear          int         `json:"admission_year"`
	RawBatchName           *string     `json:"raw_batch_name"`
	RawSectionName         *string     `json:"raw_section_name"`
	RawAdmissionType       *string     `json:"raw_admission_type"`
	RawMajorGroupName      *string     `json:"raw_major_group_name"`
	RawElectiveReq         *string     `json:"raw_elective_req"`
	HighestScore           *float64    `json:"highest_score"`
	AverageScore           *float64    `json:"average_score"`
	LowestScore            *float64    `json:"lowest_score"`
	LowestRank             *int64      `json:"lowest_rank"`
	ProvinceControlScore   *float64    `json:"province_control_score"`
	LineDeviation          *float64    `json:"line_deviation"`
	SourceSystem           string      `json:"source_system"`
	SourceTable            string      `json:"source_table"`
	Tags                   []PolicyTag `json:"tags,omitempty"`
}

type MajorAdmissionScore struct {
	MajorAdmissionScoreID int64       `json:"major_admission_score_id"`
	SchoolID              int64       `json:"school_id"`
	SchoolName            string      `json:"school_name"`
	MajorID               *int64      `json:"major_id"`
	SchoolMajorID         *int64      `json:"school_major_id"`
	ProvinceID            int         `json:"province_id"`
	ProvinceName          string      `json:"province_name"`
	PolicyID              int64       `json:"policy_id"`
	SchoolMajorGroupID    *int64      `json:"school_major_group_id"`
	AdmissionYear         int         `json:"admission_year"`
	RawBatchName          *string     `json:"raw_batch_name"`
	RawSectionName        *string     `json:"raw_section_name"`
	RawAdmissionType      *string     `json:"raw_admission_type"`
	RawMajorGroupName     *string     `json:"raw_major_group_name"`
	RawElectiveReq        *string     `json:"raw_elective_req"`
	SchoolMajorName       *string     `json:"school_major_name"`
	MajorCode             *string     `json:"major_code"`
	HighestScore          *float64    `json:"highest_score"`
	AverageScore          *float64    `json:"average_score"`
	LowestScore           *float64    `json:"lowest_score"`
	LowestRank            *int64      `json:"lowest_rank"`
	LineDeviation         *float64    `json:"line_deviation"`
	SourceSystem          string      `json:"source_system"`
	SourceTable           string      `json:"source_table"`
	Tags                  []PolicyTag `json:"tags,omitempty"`
}

type ScoreTrendQuery struct {
	Level             string `form:"level"`
	ProvinceID        string `form:"province_id"`
	Province          string `form:"province"`
	SchoolID          int64  `form:"school_id"`
	MajorName         string `form:"major_name"`
	MajorCode         string `form:"major_code"`
	Batch             string `form:"batch"`
	Section           string `form:"section"`
	YearMin           *int   `form:"year_min"`
	YearMax           *int   `form:"year_max"`
	Metric            string `form:"metric"`
	IncludeZeroScores bool   `form:"include_zero_scores"`
}

type ScoreTrendResponse struct {
	Level        string            `json:"level"`
	SchoolID     int64             `json:"school_id"`
	SchoolName   string            `json:"school_name"`
	MajorName    *string           `json:"major_name,omitempty"`
	ProvinceID   int               `json:"province_id"`
	ProvinceName string            `json:"province_name"`
	Series       []ScoreTrendPoint `json:"series"`
	DataQuality  DataQuality       `json:"data_quality"`
}

type ScoreTrendPoint struct {
	Year          int      `json:"year"`
	Batch         *string  `json:"batch,omitempty"`
	Section       *string  `json:"section,omitempty"`
	LowestScore   *float64 `json:"lowest_score"`
	AverageScore  *float64 `json:"average_score"`
	HighestScore  *float64 `json:"highest_score"`
	LowestRank    *int64   `json:"lowest_rank"`
	LineDeviation *float64 `json:"line_deviation"`
}

type DataQuality struct {
	MissingYears  []int  `json:"missing_years,omitempty"`
	HasZeroScores bool   `json:"has_zero_scores"`
	Note          string `json:"note,omitempty"`
	Disclaimer    string `json:"disclaimer,omitempty"`
}

type ScoreMatchQuery struct {
	PageQuery
	ProvinceID      string   `form:"province_id"`
	Province        string   `form:"province"`
	Year            int      `form:"year"`
	Section         string   `form:"section"`
	Score           *float64 `form:"score"`
	Rank            *int     `form:"rank"`
	Target          string   `form:"target"`
	Strategy        string   `form:"strategy"`
	ScoreWindow     *int     `form:"score_window"`
	RankWindowRatio *float64 `form:"rank_window_ratio"`
	SchoolTags      string   `form:"school_tags"`
	ProvinceFilter  string   `form:"province_filter"`
	MajorName       string   `form:"major_name"`
	TuitionMax      *float64 `form:"tuition_max"`
	Include         string   `form:"include"`
	Sort            string   `form:"sort"`
}

type ScoreMatchResponse struct {
	Input       ScoreMatchInput             `json:"input"`
	Buckets     map[string][]ScoreMatchItem `json:"buckets"`
	DataQuality DataQuality                 `json:"data_quality"`
}

type ScoreMatchInput struct {
	ProvinceName string   `json:"province_name"`
	Year         int      `json:"year"`
	Section      string   `json:"section,omitempty"`
	Score        *float64 `json:"score"`
	Rank         *int     `json:"rank"`
	Target       string   `json:"target"`
}

type ScoreMatchItem struct {
	SchoolID        int64    `json:"school_id"`
	SchoolName      string   `json:"school_name"`
	SchoolMajorName *string  `json:"school_major_name,omitempty"`
	LowestScore     *float64 `json:"lowest_score"`
	LowestRank      *int64   `json:"lowest_rank"`
	MatchDistance   float64  `json:"match_distance"`
	RiskLevel       string   `json:"risk_level"`
	Reason          string   `json:"reason"`
}

type EmploymentDataQuery struct {
	MajorName string `form:"major_name"`
	Province  string `form:"province"`
	Year      int    `form:"year"`
	Industry  string `form:"industry"`
	Page      int    `form:"page"`
	PerPage   int    `form:"per_page"`
}

type EmploymentData struct {
	ID                 int     `json:"id"`
	MajorName          string  `json:"major_name"`
	Province           string  `json:"province"`
	Year               int     `json:"year"`
	GraduatesCount     int     `json:"graduates_count"`
	EmploymentRate     float64 `json:"employment_rate"`
	AverageSalary      float64 `json:"average_salary"`
	HighestSalary      float64 `json:"highest_salary"`
	LowestSalary       float64 `json:"lowest_salary"`
	Industry           string  `json:"industry"`
	JobTitle           string  `json:"job_title"`
	FurtherStudyRate   float64 `json:"further_study_rate"`
	MajorCode          string  `json:"major_code"`
	EmploymentProvince string  `json:"employment_province"`
}

type EmploymentDataResponse struct {
	Total   int              `json:"total"`
	Data    []EmploymentData `json:"data"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}
