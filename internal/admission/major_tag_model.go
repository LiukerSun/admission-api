package admission

type AdmissionMajorTagResponse struct { //nolint:revive
	ID                         int64  `json:"id"`
	UniversityMajorAdmissionID int64  `json:"university_major_admission_id"`
	CatalogYear                int    `json:"catalog_year"`
	CategoryCode               string `json:"category_code"`
	CategoryName               string `json:"category_name"`
	ClassCode                  string `json:"class_code,omitempty"`
	ClassName                  string `json:"class_name,omitempty"`
	MajorCode                  string `json:"major_code,omitempty"`
	MajorName                  string `json:"major_name,omitempty"`
	StandardMajorID            *int64 `json:"standard_major_id,omitempty"`
	TagLevel                   string `json:"tag_level"`
	Note                       string `json:"note,omitempty"`
}
