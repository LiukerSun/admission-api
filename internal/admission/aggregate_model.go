package admission

type AggregateFilter struct {
	AdmissionYear       *int     `json:"admission_year,omitempty"`
	RegionCode          string   `json:"region_code,omitempty"`
	SubjectCategoryCode string   `json:"subject_category_code,omitempty"`
	UniversityIDs       []int64  `json:"university_ids,omitempty"`
	UniversityCodes     []string `json:"university_codes,omitempty"`
	GroupCodes          []string `json:"group_codes,omitempty"`
	TagCatalogYear      *int     `json:"tag_catalog_year,omitempty"`
	TagQuery            string   `json:"tag_query,omitempty"`
	TagCategoryCode     string   `json:"tag_category_code,omitempty"`
	TagClassCode        string   `json:"tag_class_code,omitempty"`
	TagMajorCode        string   `json:"tag_major_code,omitempty"`
	MinRankFrom         *int     `json:"min_rank_from,omitempty"`
	MinRankTo           *int     `json:"min_rank_to,omitempty"`
	MinScoreFrom        *int     `json:"min_score_from,omitempty"`
	MinScoreTo          *int     `json:"min_score_to,omitempty"`
	Is985               *bool    `json:"is_985,omitempty"`
	Is211               *bool    `json:"is_211,omitempty"`
	IsDoubleFirstClass  *bool    `json:"is_double_first_class,omitempty"`
	Cities              []string `json:"cities,omitempty"`
	ExcludeCities       []string `json:"exclude_cities,omitempty"`
	Provinces           []string `json:"provinces,omitempty"`
	ExcludeProvinces    []string `json:"exclude_provinces,omitempty"`
	SubjectCategories   []string `json:"subject_categories,omitempty"`
	GroupBy             string   `json:"group_by,omitempty"`
	Metrics             []string `json:"metrics,omitempty"`
}

type AggregateItem struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Count         int64    `json:"count"`
	AvgMinScore   *float64 `json:"avg_min_score,omitempty"`
	AvgMinRank    *float64 `json:"avg_min_rank,omitempty"`
	AvgTuition    *float64 `json:"avg_tuition,omitempty"`
	Is985Count    *int64   `json:"is_985_count,omitempty"`
	Is211Count    *int64   `json:"is_211_count,omitempty"`
	IsDoubleCount *int64   `json:"is_double_first_class_count,omitempty"`
}

type AggregateResponse struct {
	GroupBy string          `json:"group_by"`
	Total   int64           `json:"total"`
	Items   []AggregateItem `json:"items"`
}
