package admission

type DictionaryItem struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type DictionaryResponse struct {
	Regions              []DictionaryItem `json:"regions"`
	SubjectCategories    []DictionaryItem `json:"subject_categories"`
	SubjectRequirements  []DictionaryItem `json:"subject_requirements"`
	Batches              []DictionaryItem `json:"batches"`
	EducationLevels      []DictionaryItem `json:"education_levels"`
	SchoolOwnershipTypes []DictionaryItem `json:"school_ownership_types"`
	SchoolCategories     []DictionaryItem `json:"school_categories"`
}
