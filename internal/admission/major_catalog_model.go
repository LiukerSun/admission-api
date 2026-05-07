package admission

type StandardMajorFilter struct {
	CatalogYear *int
	Query       string
}

type StandardMajorResponse struct {
	ID             int64  `json:"id"`
	CatalogYear    int    `json:"catalog_year"`
	MajorCode      string `json:"major_code"`
	Name           string `json:"name"`
	CategoryCode   string `json:"category_code"`
	CategoryName   string `json:"category_name"`
	ClassCode      string `json:"class_code"`
	ClassName      string `json:"class_name"`
	Duration       string `json:"duration,omitempty"`
	DegreeCategory string `json:"degree_category,omitempty"`
	SourceURL      string `json:"source_url,omitempty"`
}

type LatestCatalogYearResponse struct {
	CatalogYear int `json:"catalog_year"`
}
