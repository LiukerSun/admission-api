package ai

import (
	"encoding/json"

	"admission-api/internal/admission"
)

func decodeAdmissionLineFilter(data []byte, filter *admission.AdmissionLineFilter) error {
	type alias admission.AdmissionLineFilter
	var raw struct {
		alias
		AdmissionYearPascal       *int     `json:"AdmissionYear"`
		RegionCodePascal          string   `json:"RegionCode"`
		SubjectCategoryCodePascal string   `json:"SubjectCategoryCode"`
		UniversityIDsPascal       []int64  `json:"UniversityIDs"`
		UniversityCodesPascal     []string `json:"UniversityCodes"`
		GroupCodesPascal          []string `json:"GroupCodes"`
		TagCatalogYearPascal      *int     `json:"TagCatalogYear"`
		TagQueryPascal            string   `json:"TagQuery"`
		TagCategoryCodePascal     string   `json:"TagCategoryCode"`
		TagClassCodePascal        string   `json:"TagClassCode"`
		TagMajorCodePascal        string   `json:"TagMajorCode"`
		MinRankFromPascal         *int     `json:"MinRankFrom"`
		MinRankToPascal           *int     `json:"MinRankTo"`
		MinScoreFromPascal        *int     `json:"MinScoreFrom"`
		MinScoreToPascal          *int     `json:"MinScoreTo"`
		Is985Pascal               *bool    `json:"Is985"`
		Is211Pascal               *bool    `json:"Is211"`
		IsDoubleFirstClassPascal  *bool    `json:"IsDoubleFirstClass"`
		CitiesPascal              []string `json:"Cities"`
		ExcludeCitiesPascal       []string `json:"ExcludeCities"`
		ProvincesPascal           []string `json:"Provinces"`
		ExcludeProvincesPascal    []string `json:"ExcludeProvinces"`
		SubjectCategoriesPascal   []string `json:"SubjectCategories"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result := admission.AdmissionLineFilter(raw.alias)
	if raw.AdmissionYearPascal != nil {
		result.AdmissionYear = raw.AdmissionYearPascal
	}
	if raw.RegionCodePascal != "" {
		result.RegionCode = raw.RegionCodePascal
	}
	if raw.SubjectCategoryCodePascal != "" {
		result.SubjectCategoryCode = raw.SubjectCategoryCodePascal
	}
	if len(raw.UniversityIDsPascal) > 0 {
		result.UniversityIDs = raw.UniversityIDsPascal
	}
	if len(raw.UniversityCodesPascal) > 0 {
		result.UniversityCodes = raw.UniversityCodesPascal
	}
	if len(raw.GroupCodesPascal) > 0 {
		result.GroupCodes = raw.GroupCodesPascal
	}
	if raw.TagCatalogYearPascal != nil {
		result.TagCatalogYear = raw.TagCatalogYearPascal
	}
	if raw.TagQueryPascal != "" {
		result.TagQuery = raw.TagQueryPascal
	}
	if raw.TagCategoryCodePascal != "" {
		result.TagCategoryCode = raw.TagCategoryCodePascal
	}
	if raw.TagClassCodePascal != "" {
		result.TagClassCode = raw.TagClassCodePascal
	}
	if raw.TagMajorCodePascal != "" {
		result.TagMajorCode = raw.TagMajorCodePascal
	}
	if raw.MinRankFromPascal != nil {
		result.MinRankFrom = raw.MinRankFromPascal
	}
	if raw.MinRankToPascal != nil {
		result.MinRankTo = raw.MinRankToPascal
	}
	if raw.MinScoreFromPascal != nil {
		result.MinScoreFrom = raw.MinScoreFromPascal
	}
	if raw.MinScoreToPascal != nil {
		result.MinScoreTo = raw.MinScoreToPascal
	}
	if raw.Is985Pascal != nil {
		result.Is985 = raw.Is985Pascal
	}
	if raw.Is211Pascal != nil {
		result.Is211 = raw.Is211Pascal
	}
	if raw.IsDoubleFirstClassPascal != nil {
		result.IsDoubleFirstClass = raw.IsDoubleFirstClassPascal
	}
	if len(raw.CitiesPascal) > 0 {
		result.Cities = raw.CitiesPascal
	}
	if len(raw.ExcludeCitiesPascal) > 0 {
		result.ExcludeCities = raw.ExcludeCitiesPascal
	}
	if len(raw.ProvincesPascal) > 0 {
		result.Provinces = raw.ProvincesPascal
	}
	if len(raw.ExcludeProvincesPascal) > 0 {
		result.ExcludeProvinces = raw.ExcludeProvincesPascal
	}
	if len(raw.SubjectCategoriesPascal) > 0 {
		result.SubjectCategories = raw.SubjectCategoriesPascal
	}
	*filter = result
	return nil
}

func decodeAggregateFilter(data []byte, filter *admission.AggregateFilter) error {
	type alias admission.AggregateFilter
	var raw struct {
		alias
		AdmissionYearPascal       *int     `json:"AdmissionYear"`
		RegionCodePascal          string   `json:"RegionCode"`
		SubjectCategoryCodePascal string   `json:"SubjectCategoryCode"`
		UniversityIDsPascal       []int64  `json:"UniversityIDs"`
		UniversityCodesPascal     []string `json:"UniversityCodes"`
		GroupCodesPascal          []string `json:"GroupCodes"`
		TagCatalogYearPascal      *int     `json:"TagCatalogYear"`
		TagQueryPascal            string   `json:"TagQuery"`
		TagCategoryCodePascal     string   `json:"TagCategoryCode"`
		TagClassCodePascal        string   `json:"TagClassCode"`
		TagMajorCodePascal        string   `json:"TagMajorCode"`
		MinRankFromPascal         *int     `json:"MinRankFrom"`
		MinRankToPascal           *int     `json:"MinRankTo"`
		MinScoreFromPascal        *int     `json:"MinScoreFrom"`
		MinScoreToPascal          *int     `json:"MinScoreTo"`
		Is985Pascal               *bool    `json:"Is985"`
		Is211Pascal               *bool    `json:"Is211"`
		IsDoubleFirstClassPascal  *bool    `json:"IsDoubleFirstClass"`
		CitiesPascal              []string `json:"Cities"`
		ExcludeCitiesPascal       []string `json:"ExcludeCities"`
		ProvincesPascal           []string `json:"Provinces"`
		ExcludeProvincesPascal    []string `json:"ExcludeProvinces"`
		SubjectCategoriesPascal   []string `json:"SubjectCategories"`
		GroupByPascal             string   `json:"GroupBy"`
		MetricsPascal             []string `json:"Metrics"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result := admission.AggregateFilter(raw.alias)
	if raw.AdmissionYearPascal != nil {
		result.AdmissionYear = raw.AdmissionYearPascal
	}
	if raw.RegionCodePascal != "" {
		result.RegionCode = raw.RegionCodePascal
	}
	if raw.SubjectCategoryCodePascal != "" {
		result.SubjectCategoryCode = raw.SubjectCategoryCodePascal
	}
	if len(raw.UniversityIDsPascal) > 0 {
		result.UniversityIDs = raw.UniversityIDsPascal
	}
	if len(raw.UniversityCodesPascal) > 0 {
		result.UniversityCodes = raw.UniversityCodesPascal
	}
	if len(raw.GroupCodesPascal) > 0 {
		result.GroupCodes = raw.GroupCodesPascal
	}
	if raw.TagCatalogYearPascal != nil {
		result.TagCatalogYear = raw.TagCatalogYearPascal
	}
	if raw.TagQueryPascal != "" {
		result.TagQuery = raw.TagQueryPascal
	}
	if raw.TagCategoryCodePascal != "" {
		result.TagCategoryCode = raw.TagCategoryCodePascal
	}
	if raw.TagClassCodePascal != "" {
		result.TagClassCode = raw.TagClassCodePascal
	}
	if raw.TagMajorCodePascal != "" {
		result.TagMajorCode = raw.TagMajorCodePascal
	}
	if raw.MinRankFromPascal != nil {
		result.MinRankFrom = raw.MinRankFromPascal
	}
	if raw.MinRankToPascal != nil {
		result.MinRankTo = raw.MinRankToPascal
	}
	if raw.MinScoreFromPascal != nil {
		result.MinScoreFrom = raw.MinScoreFromPascal
	}
	if raw.MinScoreToPascal != nil {
		result.MinScoreTo = raw.MinScoreToPascal
	}
	if raw.Is985Pascal != nil {
		result.Is985 = raw.Is985Pascal
	}
	if raw.Is211Pascal != nil {
		result.Is211 = raw.Is211Pascal
	}
	if raw.IsDoubleFirstClassPascal != nil {
		result.IsDoubleFirstClass = raw.IsDoubleFirstClassPascal
	}
	if len(raw.CitiesPascal) > 0 {
		result.Cities = raw.CitiesPascal
	}
	if len(raw.ExcludeCitiesPascal) > 0 {
		result.ExcludeCities = raw.ExcludeCitiesPascal
	}
	if len(raw.ProvincesPascal) > 0 {
		result.Provinces = raw.ProvincesPascal
	}
	if len(raw.ExcludeProvincesPascal) > 0 {
		result.ExcludeProvinces = raw.ExcludeProvincesPascal
	}
	if len(raw.SubjectCategoriesPascal) > 0 {
		result.SubjectCategories = raw.SubjectCategoriesPascal
	}
	if raw.GroupByPascal != "" {
		result.GroupBy = raw.GroupByPascal
	}
	if len(raw.MetricsPascal) > 0 {
		result.Metrics = raw.MetricsPascal
	}
	*filter = result
	return nil
}
