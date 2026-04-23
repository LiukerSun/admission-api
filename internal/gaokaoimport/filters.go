package gaokaoimport

import "strings"

var devProfileFullImportFiles = map[string]struct{}{
	"xyl_public_data_xyl_province.csv":        {},
	"xyl_public_data_xyl_province_scores.csv": {},
	"xyl_public_data_xyl_school.csv":          {},
	"xyl_public_data_xyl_school_detail.csv":   {},
	"rzy_365_zr_school.csv":                   {},
	"xyl_public_data_xyl_major.csv":           {},
	"xyl_public_data_xyl_major_detail.csv":    {},
	"rzy_365_zr_major.csv":                    {},
	"xyl_public_data_xyl_school_major.csv":    {},
	"rzy_365_zr_school_major.csv":             {},
	"xyl_public_data_xyl_xgk_elective.csv":    {},
	"rzy_365_zr_xgk_elective_base.csv":        {},
}

var devProfileYears = map[string]struct{}{
	"2021": {},
	"2023": {},
	"2024": {},
}

var devProfileProvinceIDs = map[string]struct{}{
	"11": {},
	"12": {},
	"21": {},
	"35": {},
	"37": {},
	"41": {},
}

func (i *Importer) shouldIncludeRow(file string, row csvRow) bool {
	switch i.profile {
	case "", "full":
		return true
	case "dev":
		return i.shouldIncludeRowDev(file, row)
	default:
		return true
	}
}

func (i *Importer) shouldIncludeRowDev(file string, row csvRow) bool {
	if _, ok := devProfileFullImportFiles[file]; ok {
		return true
	}

	switch file {
	case "xyl_public_data_xyl_school_admission_score.csv",
		"xyl_public_data_xyl_school_major_score.csv",
		"xyl_public_data_xyl_school_enroll_plan.csv",
		"rzy_365_zr_school_major_score.csv",
		"rzy_365_zr_school_enrplan.csv",
		"xyl_public_data_xyl_school_major_code.csv",
		"xyl_public_data_xyl_province_batch.csv",
		"xyl_public_data_xyl_xgk_elective_school_match.csv",
		"xyl_public_data_xyl_xgk_elective_major_match.csv",
		"xyl_public_data_xyl_xgk_elective_major_match_detail_11.csv",
		"xyl_public_data_xyl_xgk_elective_major_match_detail_12.csv",
		"rzy_365_zr_xgk_elective.csv",
		"rzy_365_zr_xgk_elective_school_match.csv",
		"rzy_365_zr_xgk_elective_major.csv",
		"rzy_365_zr_xgk_elective_match_major_special.csv",
		"rzy_365_zr_xgk_major.csv",
		"rzy_365_zr_xgk_school.csv",
		"rzy_365_zr_xgk_province.csv":
		return matchesDevYear(row) && matchesDevProvince(row)
	default:
		return true
	}
}

func matchesDevYear(row csvRow) bool {
	year := strings.TrimSpace(row["year"])
	if year == "" {
		return true
	}
	_, ok := devProfileYears[year]
	return ok
}

func matchesDevProvince(row csvRow) bool {
	provinceID := strings.TrimSpace(firstNonEmpty(row["province_id"], row["stu_province_id"]))
	if provinceID == "" {
		return true
	}
	_, ok := devProfileProvinceIDs[provinceID]
	return ok
}
