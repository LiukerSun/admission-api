package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetDatasetOverview(ctx context.Context, query *DatasetOverviewQuery) (*DatasetOverviewResponse, error)
	GetFacets(ctx context.Context, query *FacetsQuery) (*FacetsResponse, error)
	ListSchools(ctx context.Context, query *SchoolListQuery) (*ListResponse[School], error)
	GetSchool(ctx context.Context, schoolID int64, query *SchoolDetailQuery) (*School, error)
	CompareSchools(ctx context.Context, query *SchoolCompareQuery) ([]School, []int64, error)
	ListMajors(ctx context.Context, query *MajorListQuery) (*ListResponse[Major], error)
	GetMajor(ctx context.Context, majorID int64, query *MajorDetailQuery) (*Major, error)
	ListSchoolMajors(ctx context.Context, schoolID int64, query *SchoolMajorsQuery) (*ListResponse[SchoolMajorItem], error)
	GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error)
	ListProvinceBatchLines(ctx context.Context, query *BatchLineQuery) (*ListResponse[ProvinceBatchLine], error)
	GetProvinceBatchLineTrend(ctx context.Context, query *BatchLineTrendQuery) (*BatchLineTrendResponse, error)
	ListSchoolAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error)
	ListMajorAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error)
	GetAdmissionScoreTrend(ctx context.Context, query *ScoreTrendQuery) (*ScoreTrendResponse, error)
	GetScoreMatch(ctx context.Context, query *ScoreMatchQuery) (*ScoreMatchResponse, error)
	GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) (*EmploymentDataResponse, error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func (s *store) GetDatasetOverview(ctx context.Context, query *DatasetOverviewQuery) (*DatasetOverviewResponse, error) {
	counts := map[string]int64{}
	tables := map[string]string{
		"province_count":               "gaokao.province",
		"school_count":                 "gaokao.school",
		"major_count":                  "gaokao.major",
		"school_profile_count":         "gaokao.school_profile",
		"major_profile_count":          "gaokao.major_profile",
		"enrollment_plan_count":        "gaokao.enrollment_plan_fact",
		"school_admission_score_count": "gaokao.school_admission_score_fact",
		"major_admission_score_count":  "gaokao.major_admission_score_fact",
		"province_batch_line_count":    "gaokao.province_batch_line_fact",
	}
	for key, table := range tables {
		var count int64
		if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}

	resp := &DatasetOverviewResponse{
		Summary: DatasetSummary{
			ProvinceCount:             counts["province_count"],
			SchoolCount:               counts["school_count"],
			MajorCount:                counts["major_count"],
			SchoolProfileCount:        counts["school_profile_count"],
			MajorProfileCount:         counts["major_profile_count"],
			EnrollmentPlanCount:       counts["enrollment_plan_count"],
			SchoolAdmissionScoreCount: counts["school_admission_score_count"],
			MajorAdmissionScoreCount:  counts["major_admission_score_count"],
			ProvinceBatchLineCount:    counts["province_batch_line_count"],
		},
	}
	if query.IncludeTables {
		resp.TableCounts = counts
	}
	if query.IncludeCoverage {
		coverage, err := s.datasetCoverage(ctx)
		if err != nil {
			return nil, err
		}
		resp.Coverage = coverage
	}
	if query.IncludeImports {
		imports, err := s.latestImports(ctx)
		if err != nil {
			return nil, err
		}
		resp.LatestImports = imports
	}
	var latest sql.NullTime
	if err := s.pool.QueryRow(ctx, `SELECT MAX(imported_at) FROM gaokao.import_file_log`).Scan(&latest); err != nil {
		return nil, err
	}
	if latest.Valid {
		resp.LatestImportedAt = &latest.Time
	}
	return resp, nil
}

func (s *store) datasetCoverage(ctx context.Context) (map[string]DatasetCoverage, error) {
	items := []struct {
		key       string
		table     string
		yearCol   string
		schoolCol string
		majorCol  string
	}{
		{"province_batch_line", "gaokao.province_batch_line_fact", "score_year", "", ""},
		{"enrollment_plan", "gaokao.enrollment_plan_fact", "plan_year", "school_id", "school_major_id"},
		{"school_admission_score", "gaokao.school_admission_score_fact", "admission_year", "school_id", ""},
		{"major_admission_score", "gaokao.major_admission_score_fact", "admission_year", "school_id", "school_major_id"},
	}
	result := map[string]DatasetCoverage{}
	for _, item := range items {
		query := fmt.Sprintf(`
SELECT MIN(%[1]s)::int, MAX(%[1]s)::int, COUNT(DISTINCT %[1]s)::int, COUNT(DISTINCT province_id)::int
FROM %[2]s
`, item.yearCol, item.table)
		var yearMin, yearMax sql.NullInt64
		var yearCount, provinceCount int
		if err := s.pool.QueryRow(ctx, query).Scan(&yearMin, &yearMax, &yearCount, &provinceCount); err != nil {
			return nil, err
		}
		coverage := DatasetCoverage{
			YearCount:     yearCount,
			ProvinceCount: provinceCount,
		}
		if yearMin.Valid {
			v := int(yearMin.Int64)
			coverage.YearMin = &v
		}
		if yearMax.Valid {
			v := int(yearMax.Int64)
			coverage.YearMax = &v
		}
		if item.schoolCol != "" {
			var schoolCount int
			schoolQuery := fmt.Sprintf("SELECT COUNT(DISTINCT %s)::int FROM %s", item.schoolCol, item.table)
			if err := s.pool.QueryRow(ctx, schoolQuery).Scan(&schoolCount); err != nil {
				return nil, err
			}
			coverage.SchoolCount = schoolCount
		}
		if item.majorCol != "" {
			var majorCount int
			majorQuery := fmt.Sprintf("SELECT COUNT(DISTINCT %s)::int FROM %s", item.majorCol, item.table)
			if err := s.pool.QueryRow(ctx, majorQuery).Scan(&majorCount); err != nil {
				return nil, err
			}
			coverage.MajorCount = majorCount
		}
		result[item.key] = coverage
	}
	return result, nil
}

func (s *store) latestImports(ctx context.Context) ([]ImportLogItem, error) {
	rows, err := s.pool.Query(ctx, `
SELECT source_system, source_table, file_name, row_count, imported_at, remark
FROM gaokao.import_file_log
ORDER BY imported_at DESC, import_file_id DESC
LIMIT 20
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ImportLogItem
	for rows.Next() {
		var item ImportLogItem
		var rowCount sql.NullInt64
		var remark sql.NullString
		if err := rows.Scan(&item.SourceSystem, &item.SourceTable, &item.FileName, &rowCount, &item.ImportedAt, &remark); err != nil {
			return nil, err
		}
		if rowCount.Valid {
			item.RowCount = &rowCount.Int64
		}
		if remark.Valid {
			item.Remark = &remark.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *store) GetFacets(ctx context.Context, query *FacetsQuery) (*FacetsResponse, error) {
	scope, ok := facetScopes[query.Scope]
	if !ok {
		return nil, badQuery("unsupported scope %q", query.Scope)
	}
	fields, err := splitFacetFields(query.Fields, scope.fields)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		for field := range scope.fields {
			fields = append(fields, field)
		}
	}
	resp := &FacetsResponse{Scope: query.Scope, Facets: map[string][]FacetValue{}}
	for _, field := range fields {
		items, err := s.facetValues(ctx, scope, field, nil)
		if err != nil {
			return nil, err
		}
		resp.Facets[field] = items
	}
	return resp, nil
}

type facetScope struct {
	from   string
	fields map[string]string
}

var facetScopes = map[string]facetScope{
	"schools": {
		from: `gaokao.school s LEFT JOIN gaokao.province p ON p.province_id=s.province_id LEFT JOIN gaokao.city c ON c.city_code=s.city_code LEFT JOIN gaokao.school_profile sp ON sp.school_id=s.school_id`,
		fields: map[string]string{
			"province":       "p.province_name",
			"city":           "c.city_name",
			"ranking_source": "(SELECT sr.ranking_source FROM gaokao.school_ranking sr WHERE sr.school_id=s.school_id LIMIT 1)",
		},
	},
	"majors": {
		from: `gaokao.major m LEFT JOIN gaokao.major_profile mp ON mp.major_id=m.major_id`,
		fields: map[string]string{
			"major_subject":  "m.major_subject",
			"major_category": "m.major_category",
			"degree_name":    "m.degree_name",
			"study_years":    "m.study_years_text",
		},
	},
	"enrollment_plans": {
		from: `gaokao.enrollment_plan_fact e JOIN gaokao.school s ON s.school_id=e.school_id JOIN gaokao.province p ON p.province_id=e.province_id LEFT JOIN gaokao.major m ON m.major_id=e.major_id LEFT JOIN gaokao.school_major_group smg ON smg.school_major_group_id=e.school_major_group_id LEFT JOIN gaokao.subject_requirement_dim srd ON srd.subject_req_id=smg.subject_req_id`,
		fields: map[string]string{
			"province":      "p.province_name",
			"year":          "e.plan_year::text",
			"batch":         "e.raw_batch_name",
			"section":       "e.raw_section_name",
			"source_system": "e.source_system",
		},
	},
	"school_scores": {
		from: `gaokao.school_admission_score_fact f JOIN gaokao.school s ON s.school_id=f.school_id JOIN gaokao.province p ON p.province_id=f.province_id`,
		fields: map[string]string{
			"province":      "p.province_name",
			"year":          "f.admission_year::text",
			"batch":         "f.raw_batch_name",
			"section":       "f.raw_section_name",
			"source_system": "f.source_system",
		},
	},
	"major_scores": {
		from: `gaokao.major_admission_score_fact f JOIN gaokao.school s ON s.school_id=f.school_id JOIN gaokao.province p ON p.province_id=f.province_id`,
		fields: map[string]string{
			"province":      "p.province_name",
			"year":          "f.admission_year::text",
			"batch":         "f.raw_batch_name",
			"section":       "f.raw_section_name",
			"major_name":    "f.school_major_name",
			"source_system": "f.source_system",
		},
	},
	"batch_lines": {
		from: `gaokao.province_batch_line_fact b JOIN gaokao.province p ON p.province_id=b.province_id`,
		fields: map[string]string{
			"province": "p.province_name",
			"year":     "b.score_year::text",
			"batch":    "b.raw_batch_name",
			"category": "b.raw_category_name",
			"section":  "b.raw_section_name",
		},
	},
}

func (s *store) facetValues(ctx context.Context, scope facetScope, field string, builder *sqlBuilder) ([]FacetValue, error) {
	expr := scope.fields[field]
	where := &sqlBuilder{}
	if builder != nil {
		where.where = append(where.where, builder.where...)
		where.args = append(where.args, builder.args...)
	}
	query := fmt.Sprintf(`
SELECT %s AS value, COUNT(*)::bigint
FROM %s
%s
GROUP BY value
ORDER BY COUNT(*) DESC, value
LIMIT 200
`, expr, scope.from, facetWhereClause(where, expr))
	rows, err := s.pool.Query(ctx, query, where.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []FacetValue
	for rows.Next() {
		var value string
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		result = append(result, FacetValue{Value: value, Label: value, Count: count})
	}
	return result, rows.Err()
}

func facetWhereClause(builder *sqlBuilder, expr string) string {
	notEmpty := expr + " IS NOT NULL AND " + expr + " <> ''"
	if builder == nil || len(builder.where) == 0 {
		return "WHERE " + notEmpty
	}
	return builder.WhereClause() + " AND " + notEmpty
}

func (s *store) requestedFacets(ctx context.Context, scopeName, value string, builder *sqlBuilder) (map[string]any, error) {
	scope := facetScopes[scopeName]
	fields, err := splitFacetFields(value, scope.fields)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	result := map[string]any{}
	for _, field := range fields {
		items, err := s.facetValues(ctx, scope, field, builder)
		if err != nil {
			return nil, err
		}
		result[field] = items
	}
	return result, nil
}

func (s *store) ListSchools(ctx context.Context, query *SchoolListQuery) (*ListResponse[School], error) {
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	if err := ensureRangeInt("ranking", query.RankingMin, query.RankingMax); err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("employment_rate", query.EmploymentRateMin, query.EmploymentRateMax); err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("composite_score", query.CompositeScoreMin, query.CompositeScoreMax); err != nil {
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedSchoolIncludes)
	if err != nil {
		return nil, err
	}
	order, err := orderBy(query.Sort, schoolSorts, "s.school_name ASC")
	if err != nil {
		return nil, err
	}
	builder, err := s.schoolFilters(query)
	if err != nil {
		return nil, err
	}
	from := `gaokao.school s LEFT JOIN gaokao.province p ON p.province_id=s.province_id LEFT JOIN gaokao.city c ON c.city_code=s.city_code LEFT JOIN gaokao.school_profile sp ON sp.school_id=s.school_id`
	total, err := s.count(ctx, from, builder)
	if err != nil {
		return nil, err
	}
	sqlText := `
SELECT s.school_id, s.school_name, s.school_code, s.province_id, p.province_name, s.city_code, c.city_name, s.logo_url
FROM ` + from + builder.WhereClause() + ` ORDER BY ` + order
	limit, args := builder.LimitOffset(page, perPage)
	rows, err := s.pool.Query(ctx, sqlText+limit, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []School
	for rows.Next() {
		school, err := scanSchool(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, school)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachSchoolIncludes(ctx, items, includes); err != nil {
		return nil, err
	}
	facets, err := s.requestedFacets(ctx, "schools", query.Facets, builder)
	if err != nil {
		return nil, err
	}
	return &ListResponse[School]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}, nil
}

func (s *store) schoolFilters(query *SchoolListQuery) (*sqlBuilder, error) {
	b := &sqlBuilder{}
	if query.Q != "" {
		b.Add("s.school_name ILIKE ?", "%"+query.Q+"%")
	}
	if ids, err := splitInt64CSV(query.SchoolID, "school_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("s.school_id = ANY(?)", ids)
	}
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("s.province_id = ANY(?)", ids)
	}
	if values, err := splitCSVLimit(query.Province, "province"); err != nil {
		return nil, err
	} else if len(values) > 0 {
		b.Add("p.province_name = ANY(?)", values)
	}
	if ids, err := splitIntCSV(query.CityCode, "city_code"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("s.city_code = ANY(?)", ids)
	}
	if values, err := splitCSVLimit(query.City, "city"); err != nil {
		return nil, err
	} else if len(values) > 0 {
		b.Add("c.city_name = ANY(?)", values)
	}
	addSchoolTagFilter(b, "school_type", query.SchoolType)
	addSchoolTagFilter(b, "school_level", query.SchoolLevel)
	addSchoolTagFilter(b, "school_nature", query.SchoolNature)
	addSchoolTagFilter(b, "department", query.Department)
	for _, tag := range splitCSV(query.Tags) {
		b.Add("EXISTS (SELECT 1 FROM gaokao.school_policy_tag t WHERE t.school_id=s.school_id AND t.expire_year IS NULL AND (t.tag_type=? OR t.tag_value=?))", tag, tag)
	}
	if query.RankingSource != "" || query.RankingMin != nil || query.RankingMax != nil {
		conds := []string{"sr.school_id=s.school_id"}
		args := []any{}
		if query.RankingSource != "" {
			conds = append(conds, "sr.ranking_source=?")
			args = append(args, query.RankingSource)
		}
		if query.RankingMin != nil {
			conds = append(conds, "sr.rank_value>=?")
			args = append(args, *query.RankingMin)
		}
		if query.RankingMax != nil {
			conds = append(conds, "sr.rank_value<=?")
			args = append(args, *query.RankingMax)
		}
		b.Add("EXISTS (SELECT 1 FROM gaokao.school_ranking sr WHERE "+strings.Join(conds, " AND ")+")", args...)
	}
	addFloatRange(b, "sp.employment_rate", query.EmploymentRateMin, query.EmploymentRateMax)
	addFloatRange(b, "sp.composite_score", query.CompositeScoreMin, query.CompositeScoreMax)
	return b, nil
}

func addSchoolTagFilter(b *sqlBuilder, tagType, value string) {
	if value == "" {
		return
	}
	values := splitCSV(value)
	if len(values) == 0 {
		return
	}
	b.Add("EXISTS (SELECT 1 FROM gaokao.school_policy_tag t WHERE t.school_id=s.school_id AND t.expire_year IS NULL AND t.tag_type=? AND t.tag_value = ANY(?))", tagType, values)
}

func (s *store) GetSchool(ctx context.Context, schoolID int64, query *SchoolDetailQuery) (*School, error) {
	row := s.pool.QueryRow(ctx, `
SELECT s.school_id, s.school_name, s.school_code, s.province_id, p.province_name, s.city_code, c.city_name, s.logo_url
FROM gaokao.school s
LEFT JOIN gaokao.province p ON p.province_id=s.province_id
LEFT JOIN gaokao.city c ON c.city_code=s.city_code
WHERE s.school_id=$1
`, schoolID)
	school, err := scanSchool(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedSchoolDetailIncludes)
	if err != nil {
		return nil, err
	}
	items := []School{school}
	if err := s.attachSchoolIncludes(ctx, items, includes); err != nil {
		return nil, err
	}
	if hasInclude(includes, "score_summary") {
		summary, err := s.schoolScoreSummary(ctx, schoolID, query.ProvinceID, query.Province, query.Year)
		if err != nil {
			return nil, err
		}
		items[0].ScoreSummary = summary
	}
	if hasInclude(includes, "plan_summary") {
		summary, err := s.schoolPlanSummary(ctx, schoolID, query.ProvinceID, query.Province, query.Year)
		if err != nil {
			return nil, err
		}
		items[0].PlanSummary = summary
	}
	return &items[0], nil
}

func (s *store) CompareSchools(ctx context.Context, query *SchoolCompareQuery) ([]School, []int64, error) {
	ids, err := splitInt64CSV(query.SchoolIDs, "school_ids")
	if err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 || len(ids) > 10 {
		return nil, nil, badQuery("school_ids must contain 1 to 10 values")
	}
	sq := &SchoolListQuery{SchoolID: query.SchoolIDs, Include: query.Include, RankingSource: query.RankingSource, PageQuery: PageQuery{Page: 1, PerPage: len(ids)}}
	resp, err := s.ListSchools(ctx, sq)
	if err != nil {
		return nil, nil, err
	}
	byID := map[int64]School{}
	for idx := range resp.Items {
		item := resp.Items[idx]
		byID[item.SchoolID] = item
	}
	items := make([]School, 0, len(ids))
	missing := []int64{}
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			items = append(items, item)
		} else {
			missing = append(missing, id)
		}
	}
	return items, missing, nil
}

func (s *store) ListMajors(ctx context.Context, query *MajorListQuery) (*ListResponse[Major], error) {
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("salary", query.SalaryMin, query.SalaryMax); err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("fresh_salary", query.FreshSalaryMin, query.FreshSalaryMax); err != nil {
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedMajorIncludes)
	if err != nil {
		return nil, err
	}
	order, err := orderBy(query.Sort, majorSorts, "m.major_name ASC")
	if err != nil {
		return nil, err
	}
	builder, err := s.majorFilters(query)
	if err != nil {
		return nil, err
	}
	from := `gaokao.major m LEFT JOIN gaokao.major_profile mp ON mp.major_id=m.major_id`
	total, err := s.count(ctx, from, builder)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT m.major_id, m.major_code, m.major_name, m.major_subject, m.major_category, m.degree_name, m.study_years_text
FROM `+from+builder.WhereClause()+` ORDER BY `+order+builderLimit(builder, page, perPage), builderLimitArgs(builder, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Major
	for rows.Next() {
		item, err := scanMajor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachMajorIncludes(ctx, items, includes); err != nil {
		return nil, err
	}
	facets, err := s.requestedFacets(ctx, "majors", query.Facets, builder)
	if err != nil {
		return nil, err
	}
	return &ListResponse[Major]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}, nil
}

func (s *store) majorFilters(query *MajorListQuery) (*sqlBuilder, error) {
	b := &sqlBuilder{}
	if query.Q != "" {
		b.Add("m.major_name ILIKE ?", "%"+query.Q+"%")
	}
	if ids, err := splitInt64CSV(query.MajorID, "major_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("m.major_id = ANY(?)", ids)
	}
	if query.MajorCode != "" {
		b.Add("m.major_code ILIKE ?", query.MajorCode+"%")
	}
	addStringCSVFilter(b, "m.major_subject", query.MajorSubject, "major_subject")
	addStringCSVFilter(b, "m.major_category", query.MajorCategory, "major_category")
	addStringCSVFilter(b, "m.degree_name", query.DegreeName, "degree_name")
	addStringCSVFilter(b, "m.study_years_text", query.StudyYears, "study_years")
	addFloatRange(b, "mp.average_salary", query.SalaryMin, query.SalaryMax)
	addFloatRange(b, "mp.fresh_average_salary", query.FreshSalaryMin, query.FreshSalaryMax)
	if query.WorkArea != "" {
		b.Add("mp.work_areas::text ILIKE ?", "%"+query.WorkArea+"%")
	}
	if query.WorkIndustry != "" {
		b.Add("mp.work_industries::text ILIKE ?", "%"+query.WorkIndustry+"%")
	}
	if query.WorkJob != "" {
		b.Add("mp.work_jobs::text ILIKE ?", "%"+query.WorkJob+"%")
	}
	for _, tag := range splitCSV(query.Tags) {
		b.Add("EXISTS (SELECT 1 FROM gaokao.major_policy_tag t WHERE t.major_id=m.major_id AND t.expire_year IS NULL AND (t.tag_type=? OR t.tag_value=?))", tag, tag)
	}
	return b, nil
}

func (s *store) GetMajor(ctx context.Context, majorID int64, query *MajorDetailQuery) (*Major, error) {
	row := s.pool.QueryRow(ctx, `
SELECT major_id, major_code, major_name, major_subject, major_category, degree_name, study_years_text
FROM gaokao.major
WHERE major_id=$1
`, majorID)
	major, err := scanMajor(row)
	if err != nil {
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedMajorDetailIncludes)
	if err != nil {
		return nil, err
	}
	items := []Major{major}
	if err := s.attachMajorIncludes(ctx, items, includes); err != nil {
		return nil, err
	}
	if hasInclude(includes, "schools") {
		count, err := s.countMajorSchools(ctx, majorID)
		if err != nil {
			return nil, err
		}
		items[0].SchoolSummary = &SchoolMajorSummary{SchoolCount: count}
	}
	if hasInclude(includes, "score_summary") {
		summary, err := s.majorScoreSummary(ctx, majorID, query.ProvinceID, query.Province, query.Year)
		if err != nil {
			return nil, err
		}
		items[0].ScoreSummary = summary
	}
	if hasInclude(includes, "plan_summary") {
		summary, err := s.majorPlanSummary(ctx, majorID, query.ProvinceID, query.Province, query.Year)
		if err != nil {
			return nil, err
		}
		items[0].PlanSummary = summary
	}
	return &items[0], nil
}

func (s *store) ListSchoolMajors(ctx context.Context, schoolID int64, query *SchoolMajorsQuery) (*ListResponse[SchoolMajorItem], error) {
	exists, err := s.exists(ctx, "SELECT EXISTS (SELECT 1 FROM gaokao.school WHERE school_id=$1)", schoolID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, pgx.ErrNoRows
	}
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedSchoolMajorIncludes)
	if err != nil {
		return nil, err
	}
	order, err := orderBy(query.Sort, schoolMajorSorts, "smc.school_major_name ASC")
	if err != nil {
		return nil, err
	}
	b := &sqlBuilder{}
	b.Add("smc.school_id=?", schoolID)
	if query.Q != "" {
		b.Add("(smc.school_major_name ILIKE ? OR smc.major_name ILIKE ?)", "%"+query.Q+"%", "%"+query.Q+"%")
	}
	if query.MajorCode != "" {
		b.Add("smc.major_code ILIKE ?", query.MajorCode+"%")
	}
	if query.ObservedYear > 0 {
		b.Add("smc.observed_year=?", query.ObservedYear)
	}
	if query.DegreeName != "" {
		b.Add("m.degree_name=?", query.DegreeName)
	}
	if query.MajorSubject != "" {
		b.Add("m.major_subject=?", query.MajorSubject)
	}
	from := `gaokao.school_major_catalog smc LEFT JOIN gaokao.major m ON m.major_id=smc.major_id`
	total, err := s.count(ctx, from, b)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT smc.school_major_id, smc.school_id, smc.major_id, smc.major_code, smc.major_name, smc.school_major_name, smc.study_years_text, smc.observed_year
FROM `+from+b.WhereClause()+` ORDER BY `+order+builderLimit(b, page, perPage), builderLimitArgs(b, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SchoolMajorItem
	for rows.Next() {
		item, err := scanSchoolMajor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if hasInclude(includes, "major_profile") {
		for idx := range items {
			if items[idx].MajorID != nil {
				profile, err := s.getMajorProfile(ctx, *items[idx].MajorID)
				if err != nil {
					return nil, err
				}
				items[idx].MajorProfile = profile
			}
		}
	}
	return &ListResponse[SchoolMajorItem]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total}, nil
}

func (s *store) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) (*EnrollmentPlanResponse, error) {
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	if err := ensureRangeInt("year", query.YearMin, query.YearMax); err != nil {
		return nil, err
	}
	if err := ensureRangeInt("plan_count", query.PlanCountMin, query.PlanCountMax); err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("tuition", query.TuitionMin, query.TuitionMax); err != nil {
		return nil, err
	}
	includes, err := splitIncludes(query.Include, allowedEnrollmentIncludes)
	if err != nil {
		return nil, err
	}
	order, err := orderBy(query.Sort, enrollmentSorts, "e.plan_year DESC, s.school_name ASC")
	if err != nil {
		return nil, err
	}
	builder, err := s.enrollmentFilters(query)
	if err != nil {
		return nil, err
	}
	from := `gaokao.enrollment_plan_fact e JOIN gaokao.school s ON s.school_id=e.school_id JOIN gaokao.province p ON p.province_id=e.province_id LEFT JOIN gaokao.major m ON m.major_id=e.major_id LEFT JOIN gaokao.school_major_group smg ON smg.school_major_group_id=e.school_major_group_id LEFT JOIN gaokao.subject_requirement_dim srd ON srd.subject_req_id=smg.subject_req_id`
	total, err := s.count(ctx, from, builder)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, enrollmentSelect+" FROM "+from+builder.WhereClause()+" ORDER BY "+order+builderLimit(builder, page, perPage), builderLimitArgs(builder, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []EnrollmentPlan
	for rows.Next() {
		item, err := scanEnrollmentPlan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if hasInclude(includes, "tags") {
		if err := s.attachPlanTags(ctx, items); err != nil {
			return nil, err
		}
	}
	facets, err := s.requestedFacets(ctx, "enrollment_plans", query.Facets, builder)
	if err != nil {
		return nil, err
	}
	resp := &EnrollmentPlanResponse{Items: items, Data: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}
	return resp, nil
}

const enrollmentSelect = `
SELECT e.enrollment_plan_id, e.school_id, s.school_name, e.province_id, p.province_name, e.policy_id, e.school_major_group_id,
       e.plan_year, e.raw_batch_name, e.raw_section_name, e.raw_admission_type, e.raw_major_group_name, e.raw_elective_req,
       e.school_major_id, e.major_id, e.school_major_name, COALESCE(m.major_name, e.school_major_name), e.major_code,
       e.plan_count, e.tuition_fee::float8, e.study_years_text, e.school_code, e.major_plan_code, e.source_system, e.source_table`

func (s *store) enrollmentFilters(query *EnrollmentPlanQuery) (*sqlBuilder, error) {
	b := &sqlBuilder{}
	if query.Q != "" {
		b.Add("(s.school_name ILIKE ? OR e.school_major_name ILIKE ?)", "%"+query.Q+"%", "%"+query.Q+"%")
	}
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("e.province_id = ANY(?)", ids)
	}
	addStringCSVFilter(b, "p.province_name", query.Province, "province")
	if years, err := splitIntCSV(query.Year, "year"); err != nil {
		return nil, err
	} else if len(years) > 0 {
		b.Add("e.plan_year = ANY(?)", years)
	}
	addIntRange(b, "e.plan_year", query.YearMin, query.YearMax)
	if ids, err := splitInt64CSV(query.SchoolID, "school_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("e.school_id = ANY(?)", ids)
	}
	if query.SchoolName != "" {
		b.Add("s.school_name ILIKE ?", "%"+query.SchoolName+"%")
	}
	if ids, err := splitInt64CSV(query.MajorID, "major_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("e.major_id = ANY(?)", ids)
	}
	if query.MajorName != "" {
		b.Add("(e.school_major_name ILIKE ? OR m.major_name ILIKE ?)", "%"+query.MajorName+"%", "%"+query.MajorName+"%")
	}
	if query.MajorCode != "" {
		b.Add("e.major_code ILIKE ?", query.MajorCode+"%")
	}
	addStringCSVFilter(b, "e.raw_batch_name", query.Batch, "batch")
	addStringCSVFilter(b, "e.raw_section_name", query.Section, "section")
	addStringCSVFilter(b, "e.raw_admission_type", query.AdmissionType, "admission_type")
	if query.MajorGroup != "" {
		b.Add("e.raw_major_group_name ILIKE ?", "%"+query.MajorGroup+"%")
	}
	if query.SubjectReq != "" {
		b.Add("(e.raw_elective_req ILIKE ? OR srd.raw_requirement ILIKE ?)", "%"+query.SubjectReq+"%", "%"+query.SubjectReq+"%")
	}
	if query.FirstSubject != "" {
		b.Add("(srd.first_subject=? OR e.raw_elective_req ILIKE ?)", query.FirstSubject, "%"+query.FirstSubject+"%")
	}
	if query.SecondSubjects != "" {
		b.Add("(srd.second_subjects ILIKE ? OR e.raw_elective_req ILIKE ?)", "%"+query.SecondSubjects+"%", "%"+query.SecondSubjects+"%")
	}
	addIntRange(b, "e.plan_count", query.PlanCountMin, query.PlanCountMax)
	addFloatRange(b, "e.tuition_fee", query.TuitionMin, query.TuitionMax)
	if query.SourceSystem != "" {
		b.Add("e.source_system=?", query.SourceSystem)
	}
	addSchoolTagsFilter(b, "e.school_id", query.SchoolTags)
	return b, nil
}

func (s *store) ListProvinceBatchLines(ctx context.Context, query *BatchLineQuery) (*ListResponse[ProvinceBatchLine], error) {
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	if err := ensureRangeInt("year", query.YearMin, query.YearMax); err != nil {
		return nil, err
	}
	if err := ensureRangeFloat("score", query.ScoreMin, query.ScoreMax); err != nil {
		return nil, err
	}
	order, err := orderBy(query.Sort, batchLineSorts, "b.score_year DESC, p.province_name ASC")
	if err != nil {
		return nil, err
	}
	b, err := batchLineFilters(query)
	if err != nil {
		return nil, err
	}
	from := `gaokao.province_batch_line_fact b JOIN gaokao.province p ON p.province_id=b.province_id`
	total, err := s.count(ctx, from, b)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT b.province_batch_line_id, b.province_id, p.province_name, b.policy_id, b.score_year,
       b.raw_batch_name, b.raw_category_name, b.raw_section_name, b.score_value::float8, b.rank_value, b.source_system, b.source_table
FROM `+from+b.WhereClause()+` ORDER BY `+order+builderLimit(b, page, perPage), builderLimitArgs(b, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProvinceBatchLine
	for rows.Next() {
		item, err := scanBatchLine(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	facets, err := s.requestedFacets(ctx, "batch_lines", query.Facets, b)
	if err != nil {
		return nil, err
	}
	return &ListResponse[ProvinceBatchLine]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}, nil
}

func batchLineFilters(query *BatchLineQuery) (*sqlBuilder, error) {
	b := &sqlBuilder{}
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("b.province_id = ANY(?)", ids)
	}
	addStringCSVFilter(b, "p.province_name", query.Province, "province")
	if years, err := splitIntCSV(query.Year, "year"); err != nil {
		return nil, err
	} else if len(years) > 0 {
		b.Add("b.score_year = ANY(?)", years)
	}
	addIntRange(b, "b.score_year", query.YearMin, query.YearMax)
	addStringCSVFilter(b, "b.raw_batch_name", query.Batch, "batch")
	addStringCSVFilter(b, "b.raw_category_name", query.Category, "category")
	addStringCSVFilter(b, "b.raw_section_name", query.Section, "section")
	addFloatRange(b, "b.score_value", query.ScoreMin, query.ScoreMax)
	if query.SourceSystem != "" {
		b.Add("b.source_system=?", query.SourceSystem)
	}
	return b, nil
}

func (s *store) GetProvinceBatchLineTrend(ctx context.Context, query *BatchLineTrendQuery) (*BatchLineTrendResponse, error) {
	if query.Batch == "" {
		return nil, badQuery("batch is required")
	}
	if query.Province == "" && query.ProvinceID == "" {
		return nil, badQuery("province or province_id is required")
	}
	b := &sqlBuilder{}
	b.Add("b.raw_batch_name=?", query.Batch)
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("b.province_id=ANY(?)", ids)
	}
	addStringCSVFilter(b, "p.province_name", query.Province, "province")
	if query.Category != "" {
		b.Add("b.raw_category_name=?", query.Category)
	}
	if query.Section != "" {
		b.Add("b.raw_section_name=?", query.Section)
	}
	addIntRange(b, "b.score_year", query.YearMin, query.YearMax)
	if query.SourceSystem != "" {
		b.Add("b.source_system=?", query.SourceSystem)
	}
	rows, err := s.pool.Query(ctx, `
SELECT b.province_id, p.province_name, b.raw_category_name, b.raw_section_name, b.score_year, b.score_value::float8, b.rank_value, b.source_system
FROM gaokao.province_batch_line_fact b
JOIN gaokao.province p ON p.province_id=b.province_id
`+b.WhereClause()+` ORDER BY b.score_year ASC`, b.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resp := &BatchLineTrendResponse{Batch: query.Batch}
	for rows.Next() {
		var provinceID int
		var provinceName, sourceSystem string
		var category, section sql.NullString
		var year int
		var score float64
		var rank sql.NullInt64
		if err := rows.Scan(&provinceID, &provinceName, &category, &section, &year, &score, &rank, &sourceSystem); err != nil {
			return nil, err
		}
		resp.ProvinceID = provinceID
		resp.ProvinceName = provinceName
		if category.Valid {
			resp.Category = &category.String
		}
		if section.Valid {
			resp.Section = &section.String
		}
		point := BatchLineTrendPoint{Year: year, ScoreValue: score, SourceSystem: sourceSystem}
		if rank.Valid {
			point.RankValue = &rank.Int64
		}
		resp.Series = append(resp.Series, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *store) ListSchoolAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error) {
	return s.listSchoolScores(ctx, query)
}

func (s *store) ListMajorAdmissionScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error) {
	return s.listMajorScores(ctx, query)
}

func (s *store) listSchoolScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[SchoolAdmissionScore], error) {
	page, perPage, builder, order, err := s.scoreListSetup(query, false)
	if err != nil {
		return nil, err
	}
	from := `gaokao.school_admission_score_fact f JOIN gaokao.school s ON s.school_id=f.school_id JOIN gaokao.province p ON p.province_id=f.province_id`
	total, err := s.count(ctx, from, builder)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT f.school_admission_score_id, f.school_id, s.school_name, f.province_id, p.province_name, f.policy_id, f.school_major_group_id,
       f.admission_year, f.raw_batch_name, f.raw_section_name, f.raw_admission_type, f.raw_major_group_name, f.raw_elective_req,
       f.highest_score::float8, f.average_score::float8, f.lowest_score::float8, f.lowest_rank, f.province_control_score::float8,
       f.line_deviation::float8, f.source_system, f.source_table
FROM `+from+builder.WhereClause()+` ORDER BY `+order+builderLimit(builder, page, perPage), builderLimitArgs(builder, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SchoolAdmissionScore
	for rows.Next() {
		item, err := scanSchoolScore(rows, query.IncludeZeroScores)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	facets, err := s.requestedFacets(ctx, "school_scores", query.Facets, builder)
	if err != nil {
		return nil, err
	}
	return &ListResponse[SchoolAdmissionScore]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}, nil
}

func (s *store) listMajorScores(ctx context.Context, query *ScoreListQuery) (*ListResponse[MajorAdmissionScore], error) {
	page, perPage, builder, order, err := s.scoreListSetup(query, true)
	if err != nil {
		return nil, err
	}
	from := `gaokao.major_admission_score_fact f JOIN gaokao.school s ON s.school_id=f.school_id JOIN gaokao.province p ON p.province_id=f.province_id`
	total, err := s.count(ctx, from, builder)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT f.major_admission_score_id, f.school_id, s.school_name, f.major_id, f.school_major_id, f.province_id, p.province_name,
       f.policy_id, f.school_major_group_id, f.admission_year, f.raw_batch_name, f.raw_section_name, f.raw_admission_type,
       f.raw_major_group_name, f.raw_elective_req, f.school_major_name, f.major_code, f.highest_score::float8,
       f.average_score::float8, f.lowest_score::float8, f.lowest_rank, f.line_deviation::float8, f.source_system, f.source_table
FROM `+from+builder.WhereClause()+` ORDER BY `+order+builderLimit(builder, page, perPage), builderLimitArgs(builder, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []MajorAdmissionScore
	for rows.Next() {
		item, err := scanMajorScore(rows, query.IncludeZeroScores)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	facets, err := s.requestedFacets(ctx, "major_scores", query.Facets, builder)
	if err != nil {
		return nil, err
	}
	return &ListResponse[MajorAdmissionScore]{Items: items, Total: total, Page: page, PerPage: perPage, HasMore: int64(page*perPage) < total, Facets: facets}, nil
}

func (s *store) scoreListSetup(query *ScoreListQuery, major bool) (page, perPage int, builder *sqlBuilder, order string, err error) {
	page, perPage, err = normalizePage(query.Page, query.PerPage)
	if err != nil {
		return 0, 0, nil, "", err
	}
	if err := ensureRangeInt("year", query.YearMin, query.YearMax); err != nil {
		return 0, 0, nil, "", err
	}
	if err := ensureRangeFloat("score", query.ScoreMin, query.ScoreMax); err != nil {
		return 0, 0, nil, "", err
	}
	if query.RankMin != nil && query.RankMax != nil && *query.RankMin > *query.RankMax {
		return 0, 0, nil, "", badQuery("rank_min cannot be greater than rank_max")
	}
	if err := ensureRangeFloat("line_deviation", query.LineDeviationMin, query.LineDeviationMax); err != nil {
		return 0, 0, nil, "", err
	}
	if _, err := splitIncludes(query.Include, allowedScoreIncludes); err != nil {
		return 0, 0, nil, "", err
	}
	order, err = orderBy(query.Sort, scoreSorts, "f.admission_year DESC, f.lowest_score DESC NULLS LAST")
	if err != nil {
		return 0, 0, nil, "", err
	}
	builder, err = s.scoreFilters(query, major)
	if err != nil {
		return 0, 0, nil, "", err
	}
	return page, perPage, builder, order, nil
}

func (s *store) scoreFilters(query *ScoreListQuery, major bool) (*sqlBuilder, error) {
	b := &sqlBuilder{}
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("f.province_id = ANY(?)", ids)
	}
	addStringCSVFilter(b, "p.province_name", query.Province, "province")
	if years, err := splitIntCSV(query.Year, "year"); err != nil {
		return nil, err
	} else if len(years) > 0 {
		b.Add("f.admission_year = ANY(?)", years)
	}
	addIntRange(b, "f.admission_year", query.YearMin, query.YearMax)
	if ids, err := splitInt64CSV(query.SchoolID, "school_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("f.school_id = ANY(?)", ids)
	}
	if query.SchoolName != "" {
		b.Add("s.school_name ILIKE ?", "%"+query.SchoolName+"%")
	}
	if major {
		if query.Q != "" {
			b.Add("(f.school_major_name ILIKE ? OR s.school_name ILIKE ?)", "%"+query.Q+"%", "%"+query.Q+"%")
		}
		if ids, err := splitInt64CSV(query.MajorID, "major_id"); err != nil {
			return nil, err
		} else if len(ids) > 0 {
			b.Add("f.major_id = ANY(?)", ids)
		}
		if query.MajorName != "" {
			b.Add("f.school_major_name ILIKE ?", "%"+query.MajorName+"%")
		}
		if query.MajorCode != "" {
			b.Add("f.major_code ILIKE ?", query.MajorCode+"%")
		}
		if query.HasAverageScore && !query.IncludeZeroScores {
			b.Add("f.average_score IS NOT NULL AND f.average_score <> 0")
		} else if query.HasAverageScore {
			b.Add("f.average_score IS NOT NULL")
		}
	}
	addStringCSVFilter(b, "f.raw_batch_name", query.Batch, "batch")
	addStringCSVFilter(b, "f.raw_section_name", query.Section, "section")
	addStringCSVFilter(b, "f.raw_admission_type", query.AdmissionType, "admission_type")
	if query.MajorGroup != "" {
		b.Add("f.raw_major_group_name ILIKE ?", "%"+query.MajorGroup+"%")
	}
	if query.SubjectReq != "" {
		b.Add("f.raw_elective_req ILIKE ?", "%"+query.SubjectReq+"%")
	}
	addFloatRange(b, "f.lowest_score", query.ScoreMin, query.ScoreMax)
	if query.RankMin != nil {
		b.Add("f.lowest_rank >= ?", *query.RankMin)
	}
	if query.RankMax != nil {
		b.Add("f.lowest_rank <= ?", *query.RankMax)
	}
	addFloatRange(b, "f.line_deviation", query.LineDeviationMin, query.LineDeviationMax)
	if query.HasRank {
		b.Add("f.lowest_rank IS NOT NULL")
	}
	if query.SourceSystem != "" {
		b.Add("f.source_system=?", query.SourceSystem)
	}
	addSchoolTagsFilter(b, "f.school_id", query.SchoolTags)
	return b, nil
}

func (s *store) GetAdmissionScoreTrend(ctx context.Context, query *ScoreTrendQuery) (*ScoreTrendResponse, error) {
	if query.Level != "school" && query.Level != "major" {
		return nil, badQuery("level must be school or major")
	}
	if query.SchoolID == 0 {
		return nil, badQuery("school_id is required")
	}
	if query.Province == "" && query.ProvinceID == "" {
		return nil, badQuery("province or province_id is required")
	}
	b := &sqlBuilder{}
	b.Add("f.school_id=?", query.SchoolID)
	if ids, err := splitIntCSV(query.ProvinceID, "province_id"); err != nil {
		return nil, err
	} else if len(ids) > 0 {
		b.Add("f.province_id=ANY(?)", ids)
	}
	addStringCSVFilter(b, "p.province_name", query.Province, "province")
	if query.Batch != "" {
		b.Add("f.raw_batch_name=?", query.Batch)
	}
	if query.Section != "" {
		b.Add("f.raw_section_name=?", query.Section)
	}
	addIntRange(b, "f.admission_year", query.YearMin, query.YearMax)
	if query.Level == "major" {
		if query.MajorName != "" {
			b.Add("f.school_major_name ILIKE ?", "%"+query.MajorName+"%")
		}
		if query.MajorCode != "" {
			b.Add("f.major_code ILIKE ?", query.MajorCode+"%")
		}
	}
	table := "gaokao.school_admission_score_fact"
	extraSelect := "NULL::text"
	if query.Level == "major" {
		table = "gaokao.major_admission_score_fact"
		extraSelect = "f.school_major_name"
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
SELECT f.school_id, s.school_name, %s, f.province_id, p.province_name, f.admission_year,
       f.raw_batch_name, f.raw_section_name, f.lowest_score::float8, f.average_score::float8,
       f.highest_score::float8, f.lowest_rank, f.line_deviation::float8
FROM %s f
JOIN gaokao.school s ON s.school_id=f.school_id
JOIN gaokao.province p ON p.province_id=f.province_id
%s
ORDER BY f.admission_year ASC, f.raw_batch_name, f.raw_section_name
`, extraSelect, table, b.WhereClause()), b.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resp := &ScoreTrendResponse{Level: query.Level, DataQuality: DataQuality{Note: "部分来源将未知平均分或最高分记录为 0，默认响应已转为 null"}}
	seenYears := map[int]struct{}{}
	for rows.Next() {
		var point ScoreTrendPoint
		var majorName sql.NullString
		var high, avg, low, line sql.NullFloat64
		var rank sql.NullInt64
		if err := rows.Scan(&resp.SchoolID, &resp.SchoolName, &majorName, &resp.ProvinceID, &resp.ProvinceName, &point.Year, &point.Batch, &point.Section, &low, &avg, &high, &rank, &line); err != nil {
			return nil, err
		}
		if majorName.Valid {
			resp.MajorName = &majorName.String
		}
		point.LowestScore = nullFloatPtr(low)
		point.AverageScore = cleanScorePtr(nullFloatPtr(avg), query.IncludeZeroScores)
		point.HighestScore = cleanScorePtr(nullFloatPtr(high), query.IncludeZeroScores)
		point.LowestRank = nullInt64Ptr(rank)
		point.LineDeviation = nullFloatPtr(line)
		if (avg.Valid && avg.Float64 == 0) || (high.Valid && high.Float64 == 0) {
			resp.DataQuality.HasZeroScores = true
		}
		seenYears[point.Year] = struct{}{}
		resp.Series = append(resp.Series, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if query.YearMin != nil && query.YearMax != nil {
		for year := *query.YearMin; year <= *query.YearMax; year++ {
			if _, ok := seenYears[year]; !ok {
				resp.DataQuality.MissingYears = append(resp.DataQuality.MissingYears, year)
			}
		}
	}
	return resp, nil
}

func (s *store) GetScoreMatch(ctx context.Context, query *ScoreMatchQuery) (*ScoreMatchResponse, error) {
	if query.Year == 0 {
		return nil, badQuery("year is required")
	}
	if query.Province == "" && query.ProvinceID == "" {
		return nil, badQuery("province or province_id is required")
	}
	if query.Score == nil && query.Rank == nil {
		return nil, badQuery("score or rank is required")
	}
	target := query.Target
	if target == "" {
		target = "major"
	}
	if target != "school" && target != "major" {
		return nil, badQuery("target must be school or major")
	}
	strategy := query.Strategy
	if strategy == "" {
		strategy = "all"
	}
	if strategy != "rush" && strategy != "stable" && strategy != "safe" && strategy != "all" {
		return nil, badQuery("strategy must be rush, stable, safe, or all")
	}
	provinceName, provinceID, err := s.resolveProvince(ctx, query.ProvinceID, query.Province)
	if err != nil {
		return nil, err
	}
	resp := &ScoreMatchResponse{
		Input:       ScoreMatchInput{ProvinceName: provinceName, Year: query.Year, Section: query.Section, Score: query.Score, Rank: query.Rank, Target: target},
		Buckets:     map[string][]ScoreMatchItem{"rush": make([]ScoreMatchItem, 0), "stable": make([]ScoreMatchItem, 0), "safe": make([]ScoreMatchItem, 0)},
		DataQuality: DataQuality{Disclaimer: "该结果基于历史录取分和位次匹配，不代表录取概率", Note: "historical_admission_score"},
	}
	if strategy == "all" || strategy == "rush" {
		items, err := s.matchBucket(ctx, query, target, provinceID, "rush")
		if err != nil {
			return nil, err
		}
		resp.Buckets["rush"] = items
	}
	if strategy == "all" || strategy == "stable" {
		items, err := s.matchBucket(ctx, query, target, provinceID, "stable")
		if err != nil {
			return nil, err
		}
		resp.Buckets["stable"] = items
	}
	if strategy == "all" || strategy == "safe" {
		items, err := s.matchBucket(ctx, query, target, provinceID, "safe")
		if err != nil {
			return nil, err
		}
		resp.Buckets["safe"] = items
	}
	return resp, nil
}

func (s *store) matchBucket(ctx context.Context, query *ScoreMatchQuery, target string, provinceID int, bucket string) ([]ScoreMatchItem, error) {
	b := &sqlBuilder{}
	b.Add("f.province_id=?", provinceID)
	b.Add("f.admission_year=?", query.Year)
	if query.Section != "" {
		b.Add("f.raw_section_name=?", query.Section)
	}
	if query.MajorName != "" && target == "major" {
		b.Add("f.school_major_name ILIKE ?", "%"+query.MajorName+"%")
	}
	if query.Rank != nil {
		minRank, maxRank := rankWindow(*query.Rank, bucket)
		b.Add("f.lowest_rank BETWEEN ? AND ?", minRank, maxRank)
	} else if query.Score != nil {
		minScore, maxScore := scoreWindow(*query.Score, bucket)
		b.Add("f.lowest_score BETWEEN ? AND ?", minScore, maxScore)
	}
	addSchoolTagsFilter(b, "f.school_id", query.SchoolTags)
	table := "gaokao.school_admission_score_fact"
	majorExpr := "NULL::text"
	if target == "major" {
		table = "gaokao.major_admission_score_fact"
		majorExpr = "f.school_major_name"
	}
	args := b.Args()
	rankPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	scorePlaceholder := fmt.Sprintf("$%d", len(args)+2)
	args = append(args, rankForDistance(query), scoreForDistance(query))
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
SELECT f.school_id, s.school_name, %s, f.lowest_score::float8, f.lowest_rank
FROM %s f
JOIN gaokao.school s ON s.school_id=f.school_id
%s
ORDER BY ABS(COALESCE(f.lowest_rank, 0) - %s) ASC, ABS(COALESCE(f.lowest_score, 0) - %s) ASC
LIMIT 20
`, majorExpr, table, b.WhereClause(), rankPlaceholder, scorePlaceholder), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ScoreMatchItem
	for rows.Next() {
		var item ScoreMatchItem
		var majorName sql.NullString
		var score sql.NullFloat64
		var rank sql.NullInt64
		if err := rows.Scan(&item.SchoolID, &item.SchoolName, &majorName, &score, &rank); err != nil {
			return nil, err
		}
		if majorName.Valid {
			item.SchoolMajorName = &majorName.String
		}
		item.LowestScore = nullFloatPtr(score)
		item.LowestRank = nullInt64Ptr(rank)
		item.RiskLevel = bucket
		item.MatchDistance = matchDistance(query, score, rank)
		item.Reason = matchReason(bucket, query.Rank != nil)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *store) GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) (*EmploymentDataResponse, error) {
	// Compatibility endpoint: expose major profile salary data in the legacy shape.
	page, perPage, err := normalizePage(query.Page, query.PerPage)
	if err != nil {
		return nil, err
	}
	b := &sqlBuilder{}
	if query.MajorName != "" {
		b.Add("m.major_name ILIKE ?", "%"+query.MajorName+"%")
	}
	if query.Industry != "" {
		b.Add("mp.work_industries::text ILIKE ?", "%"+query.Industry+"%")
	}
	from := `gaokao.major m JOIN gaokao.major_profile mp ON mp.major_id=m.major_id`
	total, err := s.count(ctx, from, b)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT m.major_id, m.major_name, m.major_code, mp.average_salary::float8, mp.fresh_average_salary::float8,
       COALESCE(mp.work_industries::text, ''), COALESCE(mp.work_jobs::text, '')
FROM `+from+b.WhereClause()+` ORDER BY m.major_name ASC`+builderLimit(b, page, perPage), builderLimitArgs(b, page, perPage)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []EmploymentData
	for rows.Next() {
		var id int
		var majorName string
		var majorCode sql.NullString
		var avg, fresh sql.NullFloat64
		var industry, job string
		if err := rows.Scan(&id, &majorName, &majorCode, &avg, &fresh, &industry, &job); err != nil {
			return nil, err
		}
		item := EmploymentData{ID: id, MajorName: majorName, AverageSalary: avg.Float64, LowestSalary: fresh.Float64, HighestSalary: avg.Float64, Industry: industry, JobTitle: job}
		if majorCode.Valid {
			item.MajorCode = majorCode.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &EmploymentDataResponse{Total: int(total), Data: items, Page: page, PerPage: perPage}, nil
}

// Helpers

var allowedSchoolIncludes = map[string]struct{}{"profile": {}, "tags": {}, "rankings": {}}
var allowedSchoolDetailIncludes = map[string]struct{}{"profile": {}, "tags": {}, "rankings": {}, "majors": {}, "score_summary": {}, "plan_summary": {}}
var allowedMajorIncludes = map[string]struct{}{"profile": {}, "tags": {}, "employment": {}}
var allowedMajorDetailIncludes = map[string]struct{}{"profile": {}, "tags": {}, "schools": {}, "score_summary": {}, "plan_summary": {}}
var allowedSchoolMajorIncludes = map[string]struct{}{"major_profile": {}, "latest_plan": {}, "latest_score": {}}
var allowedEnrollmentIncludes = map[string]struct{}{"school": {}, "major": {}, "policy": {}, "group": {}, "tags": {}}
var allowedScoreIncludes = map[string]struct{}{"school": {}, "major": {}, "policy": {}, "group": {}, "tags": {}}

var schoolSorts = map[string]string{"school_name": "s.school_name", "province": "p.province_name", "city": "c.city_name", "ranking": "(SELECT MIN(sr.rank_value) FROM gaokao.school_ranking sr WHERE sr.school_id=s.school_id)", "employment_rate": "sp.employment_rate", "composite_score": "sp.composite_score"}
var majorSorts = map[string]string{"major_name": "m.major_name", "major_code": "m.major_code", "average_salary": "mp.average_salary", "fresh_average_salary": "mp.fresh_average_salary"}
var schoolMajorSorts = map[string]string{"school_major_name": "smc.school_major_name", "major_code": "smc.major_code", "observed_year": "smc.observed_year"}
var enrollmentSorts = map[string]string{"year": "e.plan_year", "plan_count": "e.plan_count", "tuition_fee": "e.tuition_fee", "school_name": "s.school_name", "major_name": "e.school_major_name"}
var batchLineSorts = map[string]string{"year": "b.score_year", "province": "p.province_name", "score_value": "b.score_value"}
var scoreSorts = map[string]string{"year": "f.admission_year", "lowest_score": "f.lowest_score", "lowest_rank": "f.lowest_rank", "line_deviation": "f.line_deviation", "school_name": "s.school_name", "major_name": "f.school_major_name"}

func (s *store) count(ctx context.Context, from string, builder *sqlBuilder) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+from+builder.WhereClause(), builder.Args()...).Scan(&total)
	return total, err
}

func (s *store) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, query, args...).Scan(&ok)
	return ok, err
}

func builderLimit(builder *sqlBuilder, page, perPage int) string {
	limit, _ := builder.LimitOffset(page, perPage)
	return limit
}

func builderLimitArgs(builder *sqlBuilder, page, perPage int) []any {
	_, args := builder.LimitOffset(page, perPage)
	return args
}

func addStringCSVFilter(b *sqlBuilder, column, value, name string) {
	values, _ := splitCSVLimit(value, name)
	if len(values) > 0 {
		b.Add(column+" = ANY(?)", values)
	}
}

func addIntRange(b *sqlBuilder, column string, minValue, maxValue *int) {
	if minValue != nil {
		b.Add(column+" >= ?", *minValue)
	}
	if maxValue != nil {
		b.Add(column+" <= ?", *maxValue)
	}
}

func addFloatRange(b *sqlBuilder, column string, minValue, maxValue *float64) {
	if minValue != nil {
		b.Add(column+" >= ?", *minValue)
	}
	if maxValue != nil {
		b.Add(column+" <= ?", *maxValue)
	}
}

func addSchoolTagsFilter(b *sqlBuilder, schoolColumn, value string) {
	for _, tag := range splitCSV(value) {
		b.Add("EXISTS (SELECT 1 FROM gaokao.school_policy_tag t WHERE t.school_id="+schoolColumn+" AND t.expire_year IS NULL AND (t.tag_type=? OR t.tag_value=?))", tag, tag)
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSchool(row rowScanner) (School, error) {
	var item School
	var schoolCode, provinceName, cityName, logoURL sql.NullString
	var provinceID, cityCode sql.NullInt64
	err := row.Scan(&item.SchoolID, &item.SchoolName, &schoolCode, &provinceID, &provinceName, &cityCode, &cityName, &logoURL)
	if err != nil {
		return item, err
	}
	item.SchoolCode = nullStringPtr(schoolCode)
	if provinceID.Valid {
		v := int(provinceID.Int64)
		item.ProvinceID = &v
	}
	item.ProvinceName = nullStringPtr(provinceName)
	if cityCode.Valid {
		v := int(cityCode.Int64)
		item.CityCode = &v
	}
	item.CityName = nullStringPtr(cityName)
	item.LogoURL = nullStringPtr(logoURL)
	return item, nil
}

func scanMajor(row rowScanner) (Major, error) {
	var item Major
	var code, subject, category, degree, years sql.NullString
	err := row.Scan(&item.MajorID, &code, &item.MajorName, &subject, &category, &degree, &years)
	item.MajorCode = nullStringPtr(code)
	item.MajorSubject = nullStringPtr(subject)
	item.MajorCategory = nullStringPtr(category)
	item.DegreeName = nullStringPtr(degree)
	item.StudyYearsText = nullStringPtr(years)
	return item, err
}

func scanSchoolMajor(row rowScanner) (SchoolMajorItem, error) {
	var item SchoolMajorItem
	var majorID, observed sql.NullInt64
	var majorCode, majorName, years sql.NullString
	err := row.Scan(&item.SchoolMajorID, &item.SchoolID, &majorID, &majorCode, &majorName, &item.SchoolMajorName, &years, &observed)
	if majorID.Valid {
		item.MajorID = &majorID.Int64
	}
	item.MajorCode = nullStringPtr(majorCode)
	item.MajorName = nullStringPtr(majorName)
	item.StudyYearsText = nullStringPtr(years)
	if observed.Valid {
		v := int(observed.Int64)
		item.ObservedYear = &v
	}
	return item, err
}

func scanEnrollmentPlan(row rowScanner) (EnrollmentPlan, error) {
	var item EnrollmentPlan
	var groupID, schoolMajorID, majorID sql.NullInt64
	var batch, section, admissionType, groupName, elective, schoolMajorName, majorName, majorCode, years, schoolCode, majorPlanCode sql.NullString
	var planCount sql.NullInt64
	var tuition sql.NullFloat64
	err := row.Scan(&item.EnrollmentPlanID, &item.SchoolID, &item.SchoolName, &item.ProvinceID, &item.ProvinceName, &item.PolicyID, &groupID, &item.PlanYear, &batch, &section, &admissionType, &groupName, &elective, &schoolMajorID, &majorID, &schoolMajorName, &majorName, &majorCode, &planCount, &tuition, &years, &schoolCode, &majorPlanCode, &item.SourceSystem, &item.SourceTable)
	item.ID = item.EnrollmentPlanID
	item.Year = item.PlanYear
	item.Province = item.ProvinceName
	item.SchoolMajorGroupID = nullInt64Ptr(groupID)
	item.RawBatchName = nullStringPtr(batch)
	item.RawSectionName = nullStringPtr(section)
	item.RawAdmissionType = nullStringPtr(admissionType)
	item.RawMajorGroupName = nullStringPtr(groupName)
	item.RawElectiveReq = nullStringPtr(elective)
	item.SchoolMajorID = nullInt64Ptr(schoolMajorID)
	item.MajorID = nullInt64Ptr(majorID)
	item.SchoolMajorName = nullStringPtr(schoolMajorName)
	if majorName.Valid {
		item.MajorName = majorName.String
	}
	item.MajorCode = nullStringPtr(majorCode)
	if planCount.Valid {
		v := int(planCount.Int64)
		item.PlanCount = &v
	}
	item.TuitionFee = nullFloatPtr(tuition)
	item.StudyYearsText = nullStringPtr(years)
	item.SchoolCode = nullStringPtr(schoolCode)
	item.MajorPlanCode = nullStringPtr(majorPlanCode)
	item.Batch = stringPtrValue(item.RawBatchName)
	item.SubjectRequire = stringPtrValue(item.RawElectiveReq)
	return item, err
}

func scanBatchLine(row rowScanner) (ProvinceBatchLine, error) {
	var item ProvinceBatchLine
	var batch, category, section sql.NullString
	var rank sql.NullInt64
	err := row.Scan(&item.ProvinceBatchLineID, &item.ProvinceID, &item.ProvinceName, &item.PolicyID, &item.ScoreYear, &batch, &category, &section, &item.ScoreValue, &rank, &item.SourceSystem, &item.SourceTable)
	item.RawBatchName = nullStringPtr(batch)
	item.RawCategoryName = nullStringPtr(category)
	item.RawSectionName = nullStringPtr(section)
	item.RankValue = nullInt64Ptr(rank)
	return item, err
}

func scanSchoolScore(row rowScanner, includeZero bool) (SchoolAdmissionScore, error) {
	var item SchoolAdmissionScore
	var groupID, rank sql.NullInt64
	var batch, section, admissionType, groupName, elective sql.NullString
	var high, avg, low, control, line sql.NullFloat64
	err := row.Scan(&item.SchoolAdmissionScoreID, &item.SchoolID, &item.SchoolName, &item.ProvinceID, &item.ProvinceName, &item.PolicyID, &groupID, &item.AdmissionYear, &batch, &section, &admissionType, &groupName, &elective, &high, &avg, &low, &rank, &control, &line, &item.SourceSystem, &item.SourceTable)
	item.SchoolMajorGroupID = nullInt64Ptr(groupID)
	item.RawBatchName = nullStringPtr(batch)
	item.RawSectionName = nullStringPtr(section)
	item.RawAdmissionType = nullStringPtr(admissionType)
	item.RawMajorGroupName = nullStringPtr(groupName)
	item.RawElectiveReq = nullStringPtr(elective)
	item.HighestScore = cleanScorePtr(nullFloatPtr(high), includeZero)
	item.AverageScore = cleanScorePtr(nullFloatPtr(avg), includeZero)
	item.LowestScore = cleanScorePtr(nullFloatPtr(low), includeZero)
	item.LowestRank = nullInt64Ptr(rank)
	item.ProvinceControlScore = cleanScorePtr(nullFloatPtr(control), includeZero)
	item.LineDeviation = cleanScorePtr(nullFloatPtr(line), includeZero)
	return item, err
}

func scanMajorScore(row rowScanner, includeZero bool) (MajorAdmissionScore, error) {
	var item MajorAdmissionScore
	var majorID, schoolMajorID, groupID, rank sql.NullInt64
	var batch, section, admissionType, groupName, elective, schoolMajorName, majorCode sql.NullString
	var high, avg, low, line sql.NullFloat64
	err := row.Scan(&item.MajorAdmissionScoreID, &item.SchoolID, &item.SchoolName, &majorID, &schoolMajorID, &item.ProvinceID, &item.ProvinceName, &item.PolicyID, &groupID, &item.AdmissionYear, &batch, &section, &admissionType, &groupName, &elective, &schoolMajorName, &majorCode, &high, &avg, &low, &rank, &line, &item.SourceSystem, &item.SourceTable)
	item.MajorID = nullInt64Ptr(majorID)
	item.SchoolMajorID = nullInt64Ptr(schoolMajorID)
	item.SchoolMajorGroupID = nullInt64Ptr(groupID)
	item.RawBatchName = nullStringPtr(batch)
	item.RawSectionName = nullStringPtr(section)
	item.RawAdmissionType = nullStringPtr(admissionType)
	item.RawMajorGroupName = nullStringPtr(groupName)
	item.RawElectiveReq = nullStringPtr(elective)
	item.SchoolMajorName = nullStringPtr(schoolMajorName)
	item.MajorCode = nullStringPtr(majorCode)
	item.HighestScore = cleanScorePtr(nullFloatPtr(high), includeZero)
	item.AverageScore = cleanScorePtr(nullFloatPtr(avg), includeZero)
	item.LowestScore = cleanScorePtr(nullFloatPtr(low), includeZero)
	item.LowestRank = nullInt64Ptr(rank)
	item.LineDeviation = cleanScorePtr(nullFloatPtr(line), includeZero)
	return item, err
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func jsonAny(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func (s *store) attachSchoolIncludes(ctx context.Context, items []School, includes map[string]bool) error {
	if len(items) == 0 {
		return nil
	}
	if hasInclude(includes, "profile") {
		for idx := range items {
			profile, err := s.getSchoolProfile(ctx, items[idx].SchoolID)
			if err != nil {
				return err
			}
			items[idx].Profile = profile
		}
	}
	if hasInclude(includes, "tags") {
		for idx := range items {
			tags, err := s.schoolTags(ctx, items[idx].SchoolID)
			if err != nil {
				return err
			}
			items[idx].Tags = tags
		}
	}
	if hasInclude(includes, "rankings") {
		for idx := range items {
			rankings, err := s.schoolRankings(ctx, items[idx].SchoolID)
			if err != nil {
				return err
			}
			items[idx].Rankings = rankings
		}
	}
	return nil
}

func (s *store) getSchoolProfile(ctx context.Context, schoolID int64) (*SchoolProfile, error) {
	row := s.pool.QueryRow(ctx, `
SELECT alias_name, former_name, founded_year, address, website_url, admission_site_url, phone, email, description,
       learning_index::float8, life_index::float8, employment_index::float8, composite_score::float8, employment_rate::float8,
       male_ratio::float8, female_ratio::float8, china_rate::float8, abroad_rate::float8
FROM gaokao.school_profile WHERE school_id=$1
`, schoolID)
	var p SchoolProfile
	var alias, former, address, website, admission, phone, email, desc sql.NullString
	var founded sql.NullInt64
	var learning, life, employment, composite, rate, male, female, china, abroad sql.NullFloat64
	if err := row.Scan(&alias, &former, &founded, &address, &website, &admission, &phone, &email, &desc, &learning, &life, &employment, &composite, &rate, &male, &female, &china, &abroad); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.AliasName = nullStringPtr(alias)
	p.FormerName = nullStringPtr(former)
	if founded.Valid {
		v := int(founded.Int64)
		p.FoundedYear = &v
	}
	p.Address = nullStringPtr(address)
	p.WebsiteURL = nullStringPtr(website)
	p.AdmissionSiteURL = nullStringPtr(admission)
	p.Phone = nullStringPtr(phone)
	p.Email = nullStringPtr(email)
	p.Description = nullStringPtr(desc)
	p.LearningIndex = nullFloatPtr(learning)
	p.LifeIndex = nullFloatPtr(life)
	p.EmploymentIndex = nullFloatPtr(employment)
	p.CompositeScore = nullFloatPtr(composite)
	p.EmploymentRate = nullFloatPtr(rate)
	p.MaleRatio = nullFloatPtr(male)
	p.FemaleRatio = nullFloatPtr(female)
	p.ChinaRate = nullFloatPtr(china)
	p.AbroadRate = nullFloatPtr(abroad)
	return &p, nil
}

func (s *store) schoolTags(ctx context.Context, schoolID int64) ([]PolicyTag, error) {
	rows, err := s.pool.Query(ctx, `SELECT tag_type, tag_value, effective_year, expire_year FROM gaokao.school_policy_tag WHERE school_id=$1 AND expire_year IS NULL ORDER BY tag_type, tag_value`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTags(rows)
}

func (s *store) schoolRankings(ctx context.Context, schoolID int64) ([]SchoolRanking, error) {
	rows, err := s.pool.Query(ctx, `SELECT ranking_source, ranking_year, rank_value FROM gaokao.school_ranking WHERE school_id=$1 ORDER BY ranking_source, ranking_year DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rankings []SchoolRanking
	for rows.Next() {
		var r SchoolRanking
		if err := rows.Scan(&r.RankingSource, &r.RankingYear, &r.RankValue); err != nil {
			return nil, err
		}
		rankings = append(rankings, r)
	}
	return rankings, rows.Err()
}

func scanTags(rows pgx.Rows) ([]PolicyTag, error) {
	var tags []PolicyTag
	for rows.Next() {
		var tag PolicyTag
		var expire sql.NullInt64
		if err := rows.Scan(&tag.TagType, &tag.TagValue, &tag.EffectiveYear, &expire); err != nil {
			return nil, err
		}
		if expire.Valid {
			v := int(expire.Int64)
			tag.ExpireYear = &v
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *store) attachMajorIncludes(ctx context.Context, items []Major, includes map[string]bool) error {
	for idx := range items {
		if hasInclude(includes, "profile") || hasInclude(includes, "employment") {
			profile, employment, err := s.getMajorProfileAndEmployment(ctx, items[idx].MajorID)
			if err != nil {
				return err
			}
			if hasInclude(includes, "profile") {
				items[idx].Profile = profile
			}
			if hasInclude(includes, "employment") {
				items[idx].Employment = employment
			}
		}
		if hasInclude(includes, "tags") {
			tags, err := s.majorTags(ctx, items[idx].MajorID)
			if err != nil {
				return err
			}
			items[idx].Tags = tags
		}
	}
	return nil
}

func (s *store) getMajorProfile(ctx context.Context, majorID int64) (*MajorProfile, error) {
	profile, _, err := s.getMajorProfileAndEmployment(ctx, majorID)
	return profile, err
}

func (s *store) getMajorProfileAndEmployment(ctx context.Context, majorID int64) (*MajorProfile, *MajorEmployment, error) {
	row := s.pool.QueryRow(ctx, `
SELECT intro_text, course_text, job_text, select_suggests, average_salary::float8, fresh_average_salary::float8,
       salary_infos, work_areas, work_industries, work_jobs
FROM gaokao.major_profile WHERE major_id=$1
`, majorID)
	var intro, course, job, suggests sql.NullString
	var avg, fresh sql.NullFloat64
	var salaryInfos, workAreas, workIndustries, workJobs []byte
	if err := row.Scan(&intro, &course, &job, &suggests, &avg, &fresh, &salaryInfos, &workAreas, &workIndustries, &workJobs); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return &MajorProfile{
			IntroText:          nullStringPtr(intro),
			CourseText:         nullStringPtr(course),
			JobText:            nullStringPtr(job),
			SelectSuggests:     nullStringPtr(suggests),
			AverageSalary:      nullFloatPtr(avg),
			FreshAverageSalary: nullFloatPtr(fresh),
		}, &MajorEmployment{
			SalaryInfos:    jsonAny(salaryInfos),
			WorkAreas:      jsonAny(workAreas),
			WorkIndustries: jsonAny(workIndustries),
			WorkJobs:       jsonAny(workJobs),
		}, nil
}

func (s *store) majorTags(ctx context.Context, majorID int64) ([]PolicyTag, error) {
	rows, err := s.pool.Query(ctx, `SELECT tag_type, tag_value, effective_year, expire_year FROM gaokao.major_policy_tag WHERE major_id=$1 AND expire_year IS NULL ORDER BY tag_type, tag_value`, majorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTags(rows)
}

func (s *store) countMajorSchools(ctx context.Context, majorID int64) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT school_id) FROM gaokao.school_major_catalog WHERE major_id=$1`, majorID).Scan(&count)
	return count, err
}

func (s *store) schoolScoreSummary(ctx context.Context, schoolID int64, provinceID, province string, year int) (*ScoreSummary, error) {
	b := &sqlBuilder{}
	b.Add("f.school_id=?", schoolID)
	return s.scoreSummary(ctx, "gaokao.school_admission_score_fact", b, provinceID, province, year)
}

func (s *store) majorScoreSummary(ctx context.Context, majorID int64, provinceID, province string, year int) (*ScoreSummary, error) {
	b := &sqlBuilder{}
	b.Add("f.major_id=?", majorID)
	return s.scoreSummary(ctx, "gaokao.major_admission_score_fact", b, provinceID, province, year)
}

func (s *store) scoreSummary(ctx context.Context, table string, b *sqlBuilder, provinceID, province string, year int) (*ScoreSummary, error) {
	if ids, err := splitIntCSV(provinceID, "province_id"); err == nil && len(ids) > 0 {
		b.Add("f.province_id=ANY(?)", ids)
	}
	if province != "" {
		b.Add("p.province_name=?", province)
	}
	if year > 0 {
		b.Add("f.admission_year=?", year)
	}
	row := s.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT ARRAY_AGG(DISTINCT f.admission_year ORDER BY f.admission_year), ARRAY_AGG(DISTINCT p.province_name ORDER BY p.province_name),
       MIN(NULLIF(f.lowest_score, 0))::float8, MAX(NULLIF(f.lowest_score, 0))::float8,
       MIN(f.lowest_rank), MAX(f.lowest_rank)
FROM %s f JOIN gaokao.province p ON p.province_id=f.province_id
%s
`, table, b.WhereClause()), b.Args()...)
	var years []int
	var provinces []string
	var minScore, maxScore sql.NullFloat64
	var minRank, maxRank sql.NullInt64
	if err := row.Scan(&years, &provinces, &minScore, &maxScore, &minRank, &maxRank); err != nil {
		return nil, err
	}
	return &ScoreSummary{AvailableYears: years, AvailableProvinces: provinces, LowestScoreMin: nullFloatPtr(minScore), LowestScoreMax: nullFloatPtr(maxScore), LowestRankMin: nullInt64Ptr(minRank), LowestRankMax: nullInt64Ptr(maxRank)}, nil
}

func (s *store) schoolPlanSummary(ctx context.Context, schoolID int64, provinceID, province string, year int) (*PlanSummary, error) {
	b := &sqlBuilder{}
	b.Add("e.school_id=?", schoolID)
	return s.planSummary(ctx, b, provinceID, province, year)
}

func (s *store) majorPlanSummary(ctx context.Context, majorID int64, provinceID, province string, year int) (*PlanSummary, error) {
	b := &sqlBuilder{}
	b.Add("e.major_id=?", majorID)
	return s.planSummary(ctx, b, provinceID, province, year)
}

func (s *store) planSummary(ctx context.Context, b *sqlBuilder, provinceID, province string, year int) (*PlanSummary, error) {
	if ids, err := splitIntCSV(provinceID, "province_id"); err == nil && len(ids) > 0 {
		b.Add("e.province_id=ANY(?)", ids)
	}
	if province != "" {
		b.Add("p.province_name=?", province)
	}
	if year > 0 {
		b.Add("e.plan_year=?", year)
	}
	row := s.pool.QueryRow(ctx, `
SELECT ARRAY_AGG(DISTINCT e.plan_year ORDER BY e.plan_year), ARRAY_AGG(DISTINCT p.province_name ORDER BY p.province_name), SUM(e.plan_count)::bigint
FROM gaokao.enrollment_plan_fact e JOIN gaokao.province p ON p.province_id=e.province_id
`+b.WhereClause(), b.Args()...)
	var years []int
	var provinces []string
	var total sql.NullInt64
	if err := row.Scan(&years, &provinces, &total); err != nil {
		return nil, err
	}
	return &PlanSummary{AvailableYears: years, AvailableProvinces: provinces, PlanCountTotal: nullInt64Ptr(total)}, nil
}

func (s *store) attachPlanTags(ctx context.Context, items []EnrollmentPlan) error {
	for idx := range items {
		tags, err := s.schoolTags(ctx, items[idx].SchoolID)
		if err != nil {
			return err
		}
		items[idx].Tags = tags
	}
	return nil
}

func (s *store) resolveProvince(ctx context.Context, provinceIDRaw, province string) (name string, id int, err error) {
	if provinceIDRaw != "" {
		ids, err := splitIntCSV(provinceIDRaw, "province_id")
		if err != nil {
			return "", 0, err
		}
		if len(ids) == 0 {
			return "", 0, badQuery("province_id is invalid")
		}
		err = s.pool.QueryRow(ctx, `SELECT province_id, province_name FROM gaokao.province WHERE province_id=$1`, ids[0]).Scan(&id, &name)
		return name, id, err
	}
	err = s.pool.QueryRow(ctx, `SELECT province_id, province_name FROM gaokao.province WHERE province_name=$1`, province).Scan(&id, &name)
	return name, id, err
}

func scoreWindow(score float64, bucket string) (minScore, maxScore float64) {
	switch bucket {
	case "rush":
		return score, score + 15
	case "stable":
		return score - 10, score + 5
	default:
		return score - 30, score - 10
	}
}

func rankWindow(rank int, bucket string) (minRank, maxRank int) {
	r := float64(rank)
	switch bucket {
	case "rush":
		return int(math.Max(1, r*0.7)), rank
	case "stable":
		return int(math.Max(1, r*0.8)), int(r * 1.2)
	default:
		return int(r * 1.2), int(r * 1.6)
	}
}

func rankForDistance(query *ScoreMatchQuery) int {
	if query.Rank == nil {
		return 0
	}
	return *query.Rank
}

func scoreForDistance(query *ScoreMatchQuery) float64 {
	if query.Score == nil {
		return 0
	}
	return *query.Score
}

func matchDistance(query *ScoreMatchQuery, score sql.NullFloat64, rank sql.NullInt64) float64 {
	if query.Rank != nil && rank.Valid {
		return float64(rank.Int64 - int64(*query.Rank))
	}
	if query.Score != nil && score.Valid {
		return score.Float64 - *query.Score
	}
	return 0
}

func matchReason(bucket string, rankBased bool) string {
	metric := "历史最低分"
	if rankBased {
		metric = "历史最低位次"
	}
	switch bucket {
	case "rush":
		return metric + "高于当前水平，仅作冲刺参考"
	case "stable":
		return metric + "接近当前水平，可作为稳妥参考"
	default:
		return metric + "低于当前水平，可作为保底参考"
	}
}
