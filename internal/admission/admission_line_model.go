package admission

type AdmissionLineFilter struct { //nolint:revive
	AdmissionYear       *int
	RegionCode          string
	SubjectCategoryCode string
	UniversityIDs       []int64
	UniversityCodes     []string
	GroupCodes          []string
	TagCatalogYear      *int
	TagQuery            string
	TagCategoryCode     string
	TagClassCode        string
	TagMajorCode        string
	MinRankFrom         *int
	MinRankTo           *int
	MinScoreFrom        *int
	MinScoreTo          *int
}

type AdmissionLineResponse struct { //nolint:revive
	ID                          int64    `json:"id"`
	AdmissionGroupID            int64    `json:"admission_group_id"`
	UniversityMajorLineID       int64    `json:"university_major_line_id"`
	UniversityID                int64    `json:"university_id"`
	UniversityCode              string   `json:"university_code"`
	UniversityName              string   `json:"university_name"`
	AdmissionYear               int      `json:"admission_year"`
	RegionCode                  string   `json:"region_code"`
	SubjectCategory             string   `json:"subject_category_code"`
	BatchCode                   string   `json:"batch_code"`
	GroupCode                   string   `json:"group_code"`
	SubjectRequirementCode      string   `json:"subject_requirement_code,omitempty"`
	BatchRemark                 string   `json:"batch_remark,omitempty"`
	GroupMinScore               *int     `json:"group_min_score,omitempty"`
	GroupMinRank                *int     `json:"group_min_rank,omitempty"`
	EquivalentMinScore2024      *int     `json:"equivalent_min_score_2024,omitempty"`
	EquivalentMinScore2023      *int     `json:"equivalent_min_score_2023,omitempty"`
	EquivalentMinScore2022      *int     `json:"equivalent_min_score_2022,omitempty"`
	SubjectChange2024           string   `json:"subject_change_2024,omitempty"`
	LocalMajorCode              string   `json:"local_major_code"`
	LocalMajorName              string   `json:"local_major_name"`
	PlanCount                   *int     `json:"plan_count,omitempty"`
	AdmittedCount               *int     `json:"admitted_count,omitempty"`
	MinScore                    *int     `json:"min_score,omitempty"`
	MinRank                     *int     `json:"min_rank,omitempty"`
	MaxScore                    *int     `json:"max_score,omitempty"`
	MaxRank                     *int     `json:"max_rank,omitempty"`
	EquivalentMinScore          *int     `json:"equivalent_min_score,omitempty"`
	Tuition                     *int     `json:"tuition,omitempty"`
	Duration                    string   `json:"duration,omitempty"`
	AdmissionRemark             string   `json:"admission_remark,omitempty"`
	MajorIntro                  string   `json:"major_intro,omitempty"`
	TrainingGoal                string   `json:"training_goal,omitempty"`
	SubjectStudyRequirement     string   `json:"subject_study_requirement,omitempty"`
	MainCourses                 string   `json:"main_courses,omitempty"`
	PostgraduateDirection       string   `json:"postgraduate_direction,omitempty"`
	EmploymentDirection         string   `json:"employment_direction,omitempty"`
	DisciplineCategory          string   `json:"discipline_category,omitempty"`
	FirstLevelDiscipline        string   `json:"first_level_discipline,omitempty"`
	FourthRoundSubjectEval      string   `json:"fourth_round_subject_eval,omitempty"`
	DoubleFirstClassSubject     string   `json:"double_first_class_subject,omitempty"`
	SoftMajorGrade              string   `json:"soft_major_grade,omitempty"`
	MajorEvaluationScore        *float64 `json:"major_evaluation_score,omitempty"`
	MajorRank                   string   `json:"major_rank,omitempty"`
	IsNationalFeature           *bool    `json:"is_national_feature,omitempty"`
	CorrespondingMasterMajors   string   `json:"corresponding_master_majors,omitempty"`
	CorrespondingDoctoralMajors string   `json:"corresponding_doctoral_majors,omitempty"`
	MasterMajorCount            *int     `json:"master_major_count,omitempty"`
	MasterMajorNames            string   `json:"master_major_names,omitempty"`
	DoctoralMajorCount          *int     `json:"doctoral_major_count,omitempty"`
	DoctoralMajorNames          string   `json:"doctoral_major_names,omitempty"`
}
