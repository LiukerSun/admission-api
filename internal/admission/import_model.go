package admission

type AdmissionImportRow struct { //nolint:revive
	SourceRowNumber        int
	AdmissionYear          int
	CatalogYear            int
	UniversityCode         string
	UniversityName         string
	RegionCode             string
	SubjectCategoryCode    string
	BatchCode              string
	GroupCode              string
	SubjectRequirementCode string
	LocalMajorCode         string
	LocalMajorName         string
	PlanCount              string
	AdmittedCount          string
	MinScore               string
	MinRank                string
	MaxScore               string
	MaxRank                string
	EquivalentMinScore     string
	Tuition                string
}

type AdmissionGroupKey struct { //nolint:revive
	UniversityCode      string
	UniversityName      string
	AdmissionYear       int
	RegionCode          string
	SubjectCategoryCode string
	BatchCode           string
	GroupCode           string
}

type ImportValidationResult struct {
	TotalRows   int              `json:"total_rows"`
	SuccessRows int              `json:"success_rows"`
	FailedRows  int              `json:"failed_rows"`
	Errors      []ImportRowError `json:"errors"`
}

type ImportRowError struct {
	RowNumber int    `json:"row_number"`
	Message   string `json:"message"`
}
