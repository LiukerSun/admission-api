package admission

type VolunteerPlan struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Columns     []string                 `json:"columns"`
	Rows        []map[string]interface{} `json:"rows"`
	Stats       VolunteerPlanStats       `json:"stats"`
}

type VolunteerPlanStats struct {
	SchoolCount int `json:"schoolCount"`
	GroupCount  int `json:"groupCount"`
	RecordCount int `json:"recordCount"`
}

type VolunteerPlansResponse struct {
	Plans []VolunteerPlan `json:"plans"`
}
