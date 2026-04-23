package gaokaoimport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	DataDir     string
	Truncate    bool
	Only        []string
	SkipXGK     bool
	SampleRows  int
	MaxReadRows int
	Profile     string
}

type Importer struct {
	db                        *pgxpool.Pool
	dataDir                   string
	truncate                  bool
	only                      map[string]struct{}
	skipXGK                   bool
	sampleRows                int
	maxReadRows               int
	profile                   string
	majorCodeToID             map[string]int64
	policyIDCache             map[string]int64
	schoolExistsCache         map[int64]struct{}
	schoolMajorGroupIDCache   map[string]*int64
	schoolMajorCatalogIDCache map[string]*int64
	subjectRequirementIDCache map[string]*int64
}

func New(db *pgxpool.Pool, opts *Options) *Importer {
	only := make(map[string]struct{}, len(opts.Only))
	for _, item := range opts.Only {
		item = strings.TrimSpace(item)
		if item != "" {
			only[item] = struct{}{}
		}
	}
	return &Importer{
		db:                        db,
		dataDir:                   opts.DataDir,
		truncate:                  opts.Truncate,
		only:                      only,
		skipXGK:                   opts.SkipXGK,
		sampleRows:                opts.SampleRows,
		maxReadRows:               opts.MaxReadRows,
		profile:                   strings.TrimSpace(opts.Profile),
		majorCodeToID:             map[string]int64{},
		policyIDCache:             map[string]int64{},
		schoolExistsCache:         map[int64]struct{}{},
		schoolMajorGroupIDCache:   map[string]*int64{},
		schoolMajorCatalogIDCache: map[string]*int64{},
		subjectRequirementIDCache: map[string]*int64{},
	}
}

func (i *Importer) Run(ctx context.Context) error {
	if i.dataDir == "" {
		return errors.New("data dir is required")
	}

	log.Printf("[gaokao-import] start data_dir=%s truncate=%v profile=%s sample_rows=%d max_read_rows=%d skip_xgk=%v only=%v", i.dataDir, i.truncate, i.profile, i.sampleRows, i.maxReadRows, i.skipXGK, sortedOnlyKeys(i.only))

	if i.truncate {
		log.Printf("[gaokao-import] truncating gaokao tables")
		if err := i.truncateTables(ctx); err != nil {
			return err
		}
		log.Printf("[gaokao-import] truncate finished")
	}

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"base", i.importBaseDimensions},
		{"schools", i.importSchools},
		{"majors", i.importMajors},
		{"school_profile", i.importSchoolProfiles},
		{"major_profile", i.importMajorProfiles},
		{"school_tags", i.importSchoolPolicyTags},
		{"major_tags", i.importMajorPolicyTags},
		{"policies", i.importAdmissionPolicies},
		{"school_major_catalog", i.importSchoolMajorCatalog},
		{"subject_requirements", i.importSubjectRequirements},
		{"school_major_groups", i.importSchoolMajorGroups},
		{"facts", i.importFacts},
	}

	for _, step := range steps {
		if !i.shouldRun(step.name) {
			log.Printf("[gaokao-import] skip step=%s", step.name)
			continue
		}
		log.Printf("[gaokao-import] step start=%s", step.name)
		if err := step.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		log.Printf("[gaokao-import] step done=%s", step.name)
	}

	log.Printf("[gaokao-import] all steps completed")
	return nil
}

func (i *Importer) shouldRun(name string) bool {
	if len(i.only) == 0 {
		return true
	}
	_, ok := i.only[name]
	return ok
}

func (i *Importer) truncateTables(ctx context.Context) error {
	sql := `
TRUNCATE TABLE
    gaokao.import_file_log,
    gaokao.province_batch_line_fact,
    gaokao.province_score_range_fact,
    gaokao.enrollment_plan_fact,
    gaokao.major_admission_score_fact,
    gaokao.school_admission_score_fact,
    gaokao.school_major_group,
    gaokao.subject_requirement_dim,
    gaokao.school_major_catalog,
    gaokao.major_profile,
    gaokao.school_ranking,
    gaokao.school_profile,
    gaokao.major_policy_tag,
    gaokao.school_policy_tag,
    gaokao.admission_policy,
    gaokao.major,
    gaokao.school,
    gaokao.city,
    gaokao.province
RESTART IDENTITY CASCADE;
`
	_, err := i.db.Exec(ctx, sql)
	return err
}

func (i *Importer) importBaseDimensions(ctx context.Context) error {
	if err := i.importProvinceFromXylProvince(ctx); err != nil {
		return err
	}
	if err := i.importProvinceFromProvinceScores(ctx); err != nil {
		return err
	}
	if err := i.importCityAndProvinceFromSchools(ctx); err != nil {
		return err
	}
	return nil
}

func (i *Importer) importSchools(ctx context.Context) error {
	if err := i.importSchoolsFromXyl(ctx); err != nil {
		return err
	}
	if err := i.importSchoolsFromRzy(ctx); err != nil {
		return err
	}
	return nil
}

func (i *Importer) importMajors(ctx context.Context) error {
	if err := i.importMajorsFromXyl(ctx); err != nil {
		return err
	}
	if err := i.importMajorsFromRzy(ctx); err != nil {
		return err
	}
	return i.loadMajorMap(ctx)
}

func (i *Importer) importSchoolProfiles(ctx context.Context) error {
	if err := i.importSchoolProfilesFromRzy(ctx); err != nil {
		return err
	}
	if err := i.importSchoolProfilesFromXyl(ctx); err != nil {
		return err
	}
	return nil
}

func (i *Importer) importMajorProfiles(ctx context.Context) error {
	return i.importMajorProfilesFromXyl(ctx)
}

func (i *Importer) importSchoolPolicyTags(ctx context.Context) error {
	if err := i.importSchoolTagsFromXyl(ctx); err != nil {
		return err
	}
	return i.importSchoolTagsFromRzy(ctx)
}

func (i *Importer) importMajorPolicyTags(ctx context.Context) error {
	if err := i.importMajorTagsFromRzyMajor(ctx); err != nil {
		return err
	}
	return i.importMajorTagsFromXGK(ctx)
}

func (i *Importer) importAdmissionPolicies(ctx context.Context) error {
	if err := i.importPoliciesFromProvinceScores(ctx); err != nil {
		return err
	}
	if err := i.importPoliciesFromXGKProvince(ctx); err != nil {
		return err
	}
	return nil
}

func (i *Importer) importSchoolMajorCatalog(ctx context.Context) error {
	if err := i.importSchoolMajorCatalogFromXylSchoolMajor(ctx); err != nil {
		return err
	}
	if err := i.importSchoolMajorCatalogFromRzySchoolMajor(ctx); err != nil {
		return err
	}
	return i.importSchoolMajorCatalogFromXylSchoolMajorCode(ctx)
}

func (i *Importer) importSubjectRequirements(ctx context.Context) error {
	if err := i.importSubjectRequirementsFromAdmission(ctx); err != nil {
		return err
	}
	if err := i.importSubjectRequirementsFromPlan(ctx); err != nil {
		return err
	}
	return i.importSubjectRequirementsFromXGK(ctx)
}

func (i *Importer) importSchoolMajorGroups(ctx context.Context) error {
	if err := i.importSchoolMajorGroupsFromAdmission(ctx); err != nil {
		return err
	}
	if err := i.importSchoolMajorGroupsFromMajorScore(ctx); err != nil {
		return err
	}
	return i.importSchoolMajorGroupsFromPlan(ctx)
}

func (i *Importer) importFacts(ctx context.Context) error {
	if err := i.importSchoolAdmissionFacts(ctx); err != nil {
		return err
	}
	if err := i.importMajorAdmissionFactsXyl(ctx); err != nil {
		return err
	}
	if err := i.importMajorAdmissionFactsRzy(ctx); err != nil {
		return err
	}
	if err := i.importEnrollmentPlanFactsXyl(ctx); err != nil {
		return err
	}
	if err := i.importEnrollmentPlanFactsRzy(ctx); err != nil {
		return err
	}
	if err := i.importProvinceBatchLineFacts(ctx); err != nil {
		return err
	}
	return i.importProvinceScoreRangeFacts(ctx)
}

type csvRow map[string]string

func (i *Importer) readCSV(name string, fn func(csvRow) error) (int, error) {
	path := filepath.Join(i.dataDir, name)
	log.Printf("[gaokao-import] file start=%s", name)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("[gaokao-import] file missing=%s skip", name)
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return 0, err
	}
	for idx := range header {
		header[idx] = strings.TrimPrefix(header[idx], "\ufeff")
		header[idx] = strings.TrimSpace(header[idx])
	}

	count := 0
	readCount := 0
	skippedCount := 0
	for {
		if i.sampleRows > 0 && count >= i.sampleRows {
			log.Printf("[gaokao-import] file sample-limit reached=%s imported=%d", name, count)
			break
		}
		if i.maxReadRows > 0 && readCount >= i.maxReadRows {
			log.Printf("[gaokao-import] file read-limit reached=%s read=%d imported=%d skipped=%d", name, readCount, count, skippedCount)
			break
		}
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return count, err
		}
		row := make(csvRow, len(header))
		for idx, key := range header {
			if idx < len(record) {
				row[key] = strings.TrimSpace(record[idx])
			} else {
				row[key] = ""
			}
		}
		readCount++
		if !i.shouldIncludeRow(name, row) {
			skippedCount++
			if readCount%50000 == 0 {
				log.Printf("[gaokao-import] file progress=%s read=%d imported=%d skipped=%d", name, readCount, count, skippedCount)
			}
			continue
		}
		if err := fn(row); err != nil {
			return count, err
		}
		count++
		if count%10000 == 0 {
			log.Printf("[gaokao-import] file progress=%s read=%d imported=%d skipped=%d", name, readCount, count, skippedCount)
		}
	}

	log.Printf("[gaokao-import] file done=%s read=%d imported=%d skipped=%d", name, readCount, count, skippedCount)
	return count, nil
}

func (i *Importer) logImport(ctx context.Context, sourceSystem, sourceTable, fileName string, rowCount int, remark string) error {
	_, err := i.db.Exec(ctx, `
INSERT INTO gaokao.import_file_log (source_system, source_table, file_name, row_count, remark)
VALUES ($1, $2, $3, $4, $5)
`, sourceSystem, sourceTable, fileName, rowCount, remark)
	return err
}

func intValue(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func int64Value(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func floatStringToPtr(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func normalizeExamModel(model string) string {
	model = strings.TrimSpace(model)
	switch model {
	case "3+1+2", "3+3":
		return model
	case "文理分科":
		return "traditional"
	default:
		return "traditional"
	}
}

func normalizeVolunteerMode(hasMajorGroup bool, examModel string) string {
	if hasMajorGroup {
		return "school_major_group"
	}
	if examModel == "traditional" {
		return "school"
	}
	return "school_plus_major"
}

func sortedOnlyKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (i *Importer) upsertSubjectRequirement(ctx context.Context, raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if id, ok := i.subjectRequirementIDCache[raw]; ok {
		return id, nil
	}
	var id int64
	err := i.db.QueryRow(ctx, `
INSERT INTO gaokao.subject_requirement_dim (raw_requirement, first_subject, second_subjects, is_new_gaokao)
VALUES ($1, NULL, NULL, $2)
ON CONFLICT (raw_requirement)
DO UPDATE SET raw_requirement = EXCLUDED.raw_requirement
RETURNING subject_req_id
`, raw, isNewGaokaoRequirement(raw)).Scan(&id)
	if err != nil {
		return nil, err
	}
	idPtr := &id
	i.subjectRequirementIDCache[raw] = idPtr
	return idPtr, nil
}

func isNewGaokaoRequirement(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.Contains(raw, "首选") || strings.Contains(raw, "再选") || strings.Contains(raw, "必选") || strings.Contains(raw, "不限") || strings.Contains(raw, "物理") || strings.Contains(raw, "历史")
}

func (i *Importer) policyID(ctx context.Context, provinceID, year int, examModel string) (int64, error) {
	cacheKey := fmt.Sprintf("%d:%d", provinceID, year)
	if id, ok := i.policyIDCache[cacheKey]; ok {
		return id, nil
	}

	var id int64
	err := i.db.QueryRow(ctx, `
SELECT policy_id
FROM gaokao.admission_policy
WHERE province_id = $1 AND policy_year = $2
`, provinceID, year).Scan(&id)
	if err == nil {
		i.policyIDCache[cacheKey] = id
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	err = i.db.QueryRow(ctx, `
INSERT INTO gaokao.admission_policy (province_id, policy_year, exam_model, volunteer_mode, batch_settings, has_major_group, has_parallel_vol, scoreline_type)
VALUES ($1, $2, $3, $4, '[]'::jsonb, false, true, NULL)
RETURNING policy_id
`, provinceID, year, examModel, normalizeVolunteerMode(false, examModel)).Scan(&id)
	if err != nil {
		return 0, err
	}
	i.policyIDCache[cacheKey] = id
	return id, nil
}

func majorIDFromCodeOrName(code, name string, m map[string]int64) *int64 {
	code = strings.TrimSpace(code)
	if code != "" {
		if id, ok := m[code]; ok {
			return &id
		}
	}
	return nil
}
