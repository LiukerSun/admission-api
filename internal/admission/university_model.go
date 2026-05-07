package admission

type UniversityFilter struct {
	Query string
}

type UniversityResponse struct {
	ID             int64  `json:"id"`
	UniversityCode string `json:"university_code"`
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name,omitempty"`
}

type UniversityProfileResponse struct {
	ID                             int64    `json:"id"`
	UniversityID                   int64    `json:"university_id"`
	ProfileYear                    int      `json:"profile_year"`
	RegionCode                     string   `json:"region_code,omitempty"`
	City                           string   `json:"city,omitempty"`
	OwnershipTypeCode              string   `json:"ownership_type_code,omitempty"`
	SchoolCategoryCode             string   `json:"school_category_code,omitempty"`
	EducationLevelCode             string   `json:"education_level_code,omitempty"`
	Is985                          *bool    `json:"is_985,omitempty"`
	Is211                          *bool    `json:"is_211,omitempty"`
	IsDoubleFirstClass             *bool    `json:"is_double_first_class,omitempty"`
	IsNationalKey                  *bool    `json:"is_national_key,omitempty"`
	IsProvincialKey                *bool    `json:"is_provincial_key,omitempty"`
	HasPostgraduateRecommendation  *bool    `json:"has_postgraduate_recommendation,omitempty"`
	PostgraduateRecommendationRate *float64 `json:"postgraduate_recommendation_rate,omitempty"`
	SoftRank                       string   `json:"soft_rank,omitempty"`
	AlumniRank                     string   `json:"alumni_rank,omitempty"`
	DifficultyRank                 string   `json:"difficulty_rank,omitempty"`
	DoctoralProgramCount           *int     `json:"doctoral_program_count,omitempty"`
	MasterProgramCount             *int     `json:"master_program_count,omitempty"`
	NationalKeySubjectCount        *int     `json:"national_key_subject_count,omitempty"`
	Affiliation                    string   `json:"affiliation,omitempty"`
	SchoolLevelTags                string   `json:"school_level_tags,omitempty"`
	ExcellenceTags                 string   `json:"excellence_tags,omitempty"`
}
