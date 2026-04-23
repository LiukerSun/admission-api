package gaokaoimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const batchSize = 500

type batchExecutor struct {
	importer *Importer
	ctx      context.Context
	batch    *pgx.Batch
	count    int
}

func newBatchExecutor(ctx context.Context, importer *Importer) *batchExecutor {
	return &batchExecutor{
		importer: importer,
		ctx:      ctx,
		batch:    &pgx.Batch{},
	}
}

func (b *batchExecutor) Queue(sql string, args ...any) error {
	b.batch.Queue(sql, args...)
	b.count++
	if b.count >= batchSize {
		return b.Flush()
	}
	return nil
}

func (b *batchExecutor) Flush() error {
	if b.count == 0 {
		return nil
	}
	br := b.importer.db.SendBatch(b.ctx, b.batch)
	defer br.Close()
	for idx := 0; idx < b.count; idx++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	b.batch = &pgx.Batch{}
	b.count = 0
	return nil
}

func sqlFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func sqlInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func sqlInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func parseJSONBFromLooseString(s string) []byte {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return []byte(s)
	}
	return nil
}

func intValueDefault(s string, fallback int) int {
	if v := intValue(s); v != nil {
		return *v
	}
	return fallback
}

func inferExamModel(section, elective string) string {
	section = strings.TrimSpace(section)
	elective = strings.TrimSpace(elective)
	switch {
	case strings.Contains(section, "首选"), strings.Contains(section, "历史"), strings.Contains(section, "物理"):
		return "3+1+2"
	case section == "综合":
		return "3+3"
	case strings.Contains(elective, "首选"), strings.Contains(elective, "再选"), strings.Contains(elective, "必选"):
		return "3+1+2"
	case strings.Contains(elective, "物理"), strings.Contains(elective, "化学"), strings.Contains(elective, "生物"), strings.Contains(elective, "政治"), strings.Contains(elective, "地理"), strings.Contains(elective, "历史"):
		return "3+3"
	default:
		return "traditional"
	}
}

func (i *Importer) importProvinceFromXylProvince(ctx context.Context) error {
	const file = "xyl_public_data_xyl_province.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		provinceID := intValue(row["province_id"])
		if provinceID == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.province (province_id, province_name, initial)
VALUES ($1, $2, $3)
ON CONFLICT (province_id) DO UPDATE
SET province_name = COALESCE(EXCLUDED.province_name, gaokao.province.province_name),
    initial = COALESCE(EXCLUDED.initial, gaokao.province.initial),
    updated_at = now()
`, *provinceID, row["province"], nullIfEmpty(row["initial"]))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_province", file, rows, "")
}

func (i *Importer) importProvinceFromProvinceScores(ctx context.Context) error {
	const file = "xyl_public_data_xyl_province_scores.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		provinceID := intValue(row["province_id"])
		if provinceID == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.province (province_id, province_name)
VALUES ($1, $2)
ON CONFLICT (province_id) DO UPDATE
SET province_name = COALESCE(EXCLUDED.province_name, gaokao.province.province_name),
    updated_at = now()
`, *provinceID, row["province"])
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_province_scores", file, rows, "")
}

func (i *Importer) importCityAndProvinceFromSchools(ctx context.Context) error {
	files := []struct {
		file          string
		source        string
		table         string
		provinceField string
		cityField     string
		cityCodeField string
	}{
		{"xyl_public_data_xyl_school.csv", "xyl", "xyl_school", "province", "city", "city_code"},
		{"rzy_365_zr_school.csv", "rzy", "zr_school", "province_name", "city_name", "city_id"},
	}
	for _, item := range files {
		batch := newBatchExecutor(ctx, i)
		rows, err := i.readCSV(item.file, func(row csvRow) error {
			provinceID := firstInt(row["province_id"])
			if provinceID == nil {
				return nil
			}
			provinceName := firstNonEmpty(row[item.provinceField])
			if provinceName != "" {
				if err := batch.Queue(`
INSERT INTO gaokao.province (province_id, province_name)
VALUES ($1, $2)
ON CONFLICT (province_id) DO UPDATE
SET province_name = COALESCE(EXCLUDED.province_name, gaokao.province.province_name),
    updated_at = now()
`, *provinceID, provinceName); err != nil {
					return err
				}
			}

			cityCode := firstInt(row[item.cityCodeField])
			cityName := firstNonEmpty(row[item.cityField])
			if cityCode == nil || cityName == "" {
				return nil
			}

			return batch.Queue(`
INSERT INTO gaokao.city (city_code, city_name, province_id)
VALUES ($1, $2, $3)
ON CONFLICT (city_code) DO UPDATE
SET city_name = COALESCE(EXCLUDED.city_name, gaokao.city.city_name),
    province_id = COALESCE(EXCLUDED.province_id, gaokao.city.province_id),
    updated_at = now()
`, *cityCode, cityName, *provinceID)
		})
		if err != nil {
			return err
		}
		if err := batch.Flush(); err != nil {
			return err
		}
		if err := i.logImport(ctx, item.source, item.table, item.file, rows, "province/city from school"); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) importSchoolsFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.school (school_id, school_name, school_code, province_id, city_code, logo_url)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''))
ON CONFLICT (school_id) DO UPDATE
SET school_name = COALESCE(EXCLUDED.school_name, gaokao.school.school_name),
    school_code = COALESCE(EXCLUDED.school_code, gaokao.school.school_code),
    province_id = COALESCE(EXCLUDED.province_id, gaokao.school.province_id),
    city_code = COALESCE(EXCLUDED.city_code, gaokao.school.city_code),
    logo_url = COALESCE(EXCLUDED.logo_url, gaokao.school.logo_url),
    updated_at = now()
`, *schoolID, row["school_name"], row["school_id"], sqlInt(firstInt(row["province_id"])), sqlInt(firstInt(row["city_code"])), row["school_logo"])
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school", file, rows, "")
}

func (i *Importer) importSchoolsFromRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.school (school_id, school_name, school_code, province_id, city_code, logo_url)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''))
ON CONFLICT (school_id) DO UPDATE
SET school_name = COALESCE(EXCLUDED.school_name, gaokao.school.school_name),
    school_code = COALESCE(EXCLUDED.school_code, gaokao.school.school_code),
    province_id = COALESCE(EXCLUDED.province_id, gaokao.school.province_id),
    city_code = COALESCE(EXCLUDED.city_code, gaokao.school.city_code),
    logo_url = COALESCE(EXCLUDED.logo_url, gaokao.school.logo_url),
    updated_at = now()
`, *schoolID, row["title"], row["code"], sqlInt(firstInt(row["province_id"])), sqlInt(firstInt(row["city_id"])), row["logo"])
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school", file, rows, "")
}

func (i *Importer) importMajorsFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_major.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		code := strings.TrimSpace(row["major_code"])
		name := strings.TrimSpace(row["major_name"])
		if code == "" && name == "" {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.major (major_code, major_name, major_subject, major_category, degree_name, study_years_text)
VALUES (NULLIF($1, ''), $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''))
ON CONFLICT (major_code) WHERE major_code IS NOT NULL AND major_code <> '' DO UPDATE
SET major_name = COALESCE(EXCLUDED.major_name, gaokao.major.major_name),
    major_subject = COALESCE(EXCLUDED.major_subject, gaokao.major.major_subject),
    major_category = COALESCE(EXCLUDED.major_category, gaokao.major.major_category),
    degree_name = COALESCE(EXCLUDED.degree_name, gaokao.major.degree_name),
    study_years_text = COALESCE(EXCLUDED.study_years_text, gaokao.major.study_years_text),
    updated_at = now()
`, code, firstNonEmpty(name, code), row["major_subject"], row["major_category"], row["degree"], row["time"])
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_major", file, rows, "")
}

func (i *Importer) importMajorsFromRzy(ctx context.Context) error {
	const file = "rzy_365_zr_major.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		code := strings.TrimSpace(row["major_code"])
		name := strings.TrimSpace(row["major_title"])
		if code == "" && name == "" {
			return nil
		}
		if code == "" {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.major (major_code, major_name, major_subject, major_category, degree_name, study_years_text)
VALUES (NULLIF($1, ''), $2, NULL, NULL, NULLIF($3, ''), NULLIF($4, ''))
ON CONFLICT (major_code) WHERE major_code IS NOT NULL AND major_code <> '' DO UPDATE
SET major_name = COALESCE(EXCLUDED.major_name, gaokao.major.major_name),
    degree_name = COALESCE(EXCLUDED.degree_name, gaokao.major.degree_name),
    study_years_text = COALESCE(EXCLUDED.study_years_text, gaokao.major.study_years_text),
    updated_at = now()
`, code, firstNonEmpty(name, code), row["degree"], row["system"])
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_major", file, rows, "")
}

func (i *Importer) loadMajorMap(ctx context.Context) error {
	rows, err := i.db.Query(ctx, `SELECT major_id, major_code FROM gaokao.major WHERE major_code IS NOT NULL AND major_code <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	i.majorCodeToID = map[string]int64{}
	for rows.Next() {
		var id int64
		var code string
		if err := rows.Scan(&id, &code); err != nil {
			return err
		}
		i.majorCodeToID[code] = id
	}
	return rows.Err()
}

func (i *Importer) importSchoolProfilesFromRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["title"]); err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_profile (
    school_id, founded_year, address, postcode, website_url, phone, email, area_square_meter,
    description, labels, campus_scenery, learning_index, life_index, employment_index,
    composite_score, employment_rate, male_ratio, female_ratio, china_rate, abroad_rate,
    hostel_text, canteen_text, extra_payload, source_system
)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,
        NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NULLIF($21,''),NULLIF($22,''),$23,$24)
ON CONFLICT (school_id) DO UPDATE
SET founded_year = COALESCE(EXCLUDED.founded_year, gaokao.school_profile.founded_year),
    address = COALESCE(EXCLUDED.address, gaokao.school_profile.address),
    postcode = COALESCE(EXCLUDED.postcode, gaokao.school_profile.postcode),
    website_url = COALESCE(EXCLUDED.website_url, gaokao.school_profile.website_url),
    phone = COALESCE(EXCLUDED.phone, gaokao.school_profile.phone),
    email = COALESCE(EXCLUDED.email, gaokao.school_profile.email),
    area_square_meter = COALESCE(EXCLUDED.area_square_meter, gaokao.school_profile.area_square_meter),
    description = COALESCE(EXCLUDED.description, gaokao.school_profile.description),
    labels = COALESCE(EXCLUDED.labels, gaokao.school_profile.labels),
    campus_scenery = COALESCE(EXCLUDED.campus_scenery, gaokao.school_profile.campus_scenery),
    learning_index = COALESCE(EXCLUDED.learning_index, gaokao.school_profile.learning_index),
    life_index = COALESCE(EXCLUDED.life_index, gaokao.school_profile.life_index),
    employment_index = COALESCE(EXCLUDED.employment_index, gaokao.school_profile.employment_index),
    composite_score = COALESCE(EXCLUDED.composite_score, gaokao.school_profile.composite_score),
    employment_rate = COALESCE(EXCLUDED.employment_rate, gaokao.school_profile.employment_rate),
    male_ratio = COALESCE(EXCLUDED.male_ratio, gaokao.school_profile.male_ratio),
    female_ratio = COALESCE(EXCLUDED.female_ratio, gaokao.school_profile.female_ratio),
    china_rate = COALESCE(EXCLUDED.china_rate, gaokao.school_profile.china_rate),
    abroad_rate = COALESCE(EXCLUDED.abroad_rate, gaokao.school_profile.abroad_rate),
    hostel_text = COALESCE(EXCLUDED.hostel_text, gaokao.school_profile.hostel_text),
    canteen_text = COALESCE(EXCLUDED.canteen_text, gaokao.school_profile.canteen_text),
    extra_payload = COALESCE(EXCLUDED.extra_payload, gaokao.school_profile.extra_payload),
    source_system = COALESCE(EXCLUDED.source_system, gaokao.school_profile.source_system),
    updated_at = now()
`, *schoolID,
			sqlInt(firstInt(row["time"])),
			row["address"],
			row["postcode"],
			row["web_url"],
			row["telephone"],
			row["email"],
			sqlFloat(floatStringToPtr(row["area_covered"])),
			row["brief"],
			row["label"],
			parseJSONBFromLooseString(row["campus_scenery"]),
			sqlFloat(floatStringToPtr(row["learning_index"])),
			sqlFloat(floatStringToPtr(row["life_index"])),
			sqlFloat(floatStringToPtr(row["emp_index"])),
			sqlFloat(floatStringToPtr(row["com_score"])),
			sqlFloat(floatStringToPtr(row["emp_rate"])),
			sqlFloat(floatStringToPtr(row["male"])),
			sqlFloat(floatStringToPtr(row["sex"])),
			sqlFloat(floatStringToPtr(row["china_rate"])),
			sqlFloat(floatStringToPtr(row["abroad_rate"])),
			row["dormitory"],
			firstNonEmpty(row["canteen"], row["canteen_dormitorytext"]),
			marshalJSON(map[string]string{
				"master_program": row["master_program"],
				"nat_dis":        row["nat_dis"],
				"key_lab":        row["key_lab"],
			}),
			"rzy",
		)
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	if err := i.logImport(ctx, "rzy", "zr_school", file, rows, "school_profile"); err != nil {
		return err
	}
	return i.importSchoolRankingsFromRzy(ctx)
}

func (i *Importer) importSchoolProfilesFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_detail.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["name"]); err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_profile (
    school_id, alias_name, former_name, founded_year, address, website_url, admission_site_url,
    phone, email, area_square_meter, description, learning_index, life_index, extra_payload, source_system
)
VALUES ($1,NULLIF($2,''),NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,NULLIF($11,''),$12,$13,$14,$15)
ON CONFLICT (school_id) DO UPDATE
SET alias_name = COALESCE(EXCLUDED.alias_name, gaokao.school_profile.alias_name),
    former_name = COALESCE(EXCLUDED.former_name, gaokao.school_profile.former_name),
    founded_year = COALESCE(EXCLUDED.founded_year, gaokao.school_profile.founded_year),
    address = COALESCE(EXCLUDED.address, gaokao.school_profile.address),
    website_url = COALESCE(EXCLUDED.website_url, gaokao.school_profile.website_url),
    admission_site_url = COALESCE(EXCLUDED.admission_site_url, gaokao.school_profile.admission_site_url),
    phone = COALESCE(EXCLUDED.phone, gaokao.school_profile.phone),
    email = COALESCE(EXCLUDED.email, gaokao.school_profile.email),
    area_square_meter = COALESCE(EXCLUDED.area_square_meter, gaokao.school_profile.area_square_meter),
    description = COALESCE(EXCLUDED.description, gaokao.school_profile.description),
    learning_index = COALESCE(EXCLUDED.learning_index, gaokao.school_profile.learning_index),
    life_index = COALESCE(EXCLUDED.life_index, gaokao.school_profile.life_index),
    extra_payload = COALESCE(EXCLUDED.extra_payload, gaokao.school_profile.extra_payload),
    source_system = COALESCE(EXCLUDED.source_system, gaokao.school_profile.source_system),
    updated_at = now()
`, *schoolID, row["another_name"], row["former_name"], sqlInt(firstInt(row["create_date"])), row["address"], row["site"], row["admission_site"], row["phone"], row["email"], sqlFloat(floatStringToPtr(row["area"])), row["description"], sqlFloat(floatStringToPtr(row["learn"])), sqlFloat(floatStringToPtr(row["life"])), marshalJSON(map[string]string{
			"work": row["work"],
		}), "xyl")
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	if err := i.logImport(ctx, "xyl", "xyl_school_detail", file, rows, "school_profile"); err != nil {
		return err
	}
	return i.importSchoolRankingsFromXyl(ctx)
}

func (i *Importer) importSchoolRankingsFromRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["title"]); err != nil {
			return err
		}
		for _, item := range []struct {
			source string
			value  string
		}{
			{"ruanke", row["ruanke_rank"]},
			{"xyh", row["xyh_rank"]},
			{"qs", row["qs_rank"]},
			{"us", row["us_rank"]},
			{"tws_china", row["tws_china"]},
			{"wsl", row["wsl_rank"]},
			{"zr", row["zr_rank"]},
		} {
			if v := intValue(item.value); v != nil && *v > 0 {
				if _, err := i.db.Exec(ctx, `
INSERT INTO gaokao.school_ranking (school_id, ranking_source, ranking_year, rank_value, source_system)
VALUES ($1, $2, 0, $3, 'rzy')
ON CONFLICT (school_id, ranking_source, ranking_year) DO UPDATE
SET rank_value = EXCLUDED.rank_value,
    source_system = EXCLUDED.source_system
`, *schoolID, item.source, *v); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school", file, rows, "school_ranking")
}

func (i *Importer) importSchoolRankingsFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_detail.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["name"]); err != nil {
			return err
		}
		for _, item := range []struct {
			source string
			value  string
		}{
			{"ruanke", row["ruanke_rank"]},
			{"qs", row["qs_rank"]},
			{"us", row["us_rank"]},
			{"tws_china", row["tws_rank"]},
			{"xyh", row["xiaoyou_rank"]},
		} {
			if v := intValue(item.value); v != nil && *v > 0 {
				if _, err := i.db.Exec(ctx, `
INSERT INTO gaokao.school_ranking (school_id, ranking_source, ranking_year, rank_value, source_system)
VALUES ($1, $2, 0, $3, 'xyl')
ON CONFLICT (school_id, ranking_source, ranking_year) DO UPDATE
SET rank_value = EXCLUDED.rank_value,
    source_system = EXCLUDED.source_system
`, *schoolID, item.source, *v); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_detail", file, rows, "school_ranking")
}

func (i *Importer) importMajorProfilesFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_major_detail.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		majorID := majorIDFromCodeOrName(row["major_code"], row["major_name"], i.majorCodeToID)
		if majorID == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.major_profile (
    major_id, intro_text, course_text, job_text, select_suggests, average_salary,
    fresh_average_salary, salary_infos, work_areas, work_industries, work_jobs, extra_payload, source_system
)
VALUES ($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,'xyl')
ON CONFLICT (major_id) DO UPDATE
SET intro_text = COALESCE(EXCLUDED.intro_text, gaokao.major_profile.intro_text),
    course_text = COALESCE(EXCLUDED.course_text, gaokao.major_profile.course_text),
    job_text = COALESCE(EXCLUDED.job_text, gaokao.major_profile.job_text),
    select_suggests = COALESCE(EXCLUDED.select_suggests, gaokao.major_profile.select_suggests),
    average_salary = COALESCE(EXCLUDED.average_salary, gaokao.major_profile.average_salary),
    fresh_average_salary = COALESCE(EXCLUDED.fresh_average_salary, gaokao.major_profile.fresh_average_salary),
    salary_infos = COALESCE(EXCLUDED.salary_infos, gaokao.major_profile.salary_infos),
    work_areas = COALESCE(EXCLUDED.work_areas, gaokao.major_profile.work_areas),
    work_industries = COALESCE(EXCLUDED.work_industries, gaokao.major_profile.work_industries),
    work_jobs = COALESCE(EXCLUDED.work_jobs, gaokao.major_profile.work_jobs),
    extra_payload = COALESCE(EXCLUDED.extra_payload, gaokao.major_profile.extra_payload),
    source_system = EXCLUDED.source_system,
    updated_at = now()
`, *majorID, row["intro"], row["course"], row["job"], row["select_suggests"], sqlFloat(floatStringToPtr(row["average_salary"])), sqlFloat(floatStringToPtr(row["fresh_average_salary"])), parseJSONBFromLooseString(row["salary_infos"]), parseJSONBFromLooseString(row["work_areas"]), parseJSONBFromLooseString(row["work_industries"]), parseJSONBFromLooseString(row["work_jobs"]), marshalJSON(map[string]string{
			"post_graduate": row["post_graduate"],
			"boy_ratio":     row["boy_ratio"],
			"girl_ratio":    row["girl_ratio"],
		}))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_major_detail", file, rows, "")
}

func (i *Importer) importSchoolTagsFromXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		effectiveYear := int16(2025)
		if err := insertSchoolTag(ctx, i.db, *schoolID, "school_nature", row["school_nature_name"], effectiveYear, "xyl"); err != nil {
			return err
		}
		if err := insertSchoolTag(ctx, i.db, *schoolID, "school_type", row["type_name"], effectiveYear, "xyl"); err != nil {
			return err
		}
		if err := insertSchoolTag(ctx, i.db, *schoolID, "school_level", row["school_type_name"], effectiveYear, "xyl"); err != nil {
			return err
		}
		if err := insertSchoolTag(ctx, i.db, *schoolID, "department", row["department"], effectiveYear, "xyl"); err != nil {
			return err
		}
		for _, item := range []struct {
			field string
			tag   string
		}{
			{"f985", "985"},
			{"f211", "211"},
			{"dual_class", "dual_class"},
			{"qiangji", "qiangji_plan"},
			{"doublehigh", "double_high"},
		} {
			if isTrueFlag(row[item.field]) {
				if err := insertSchoolTag(ctx, i.db, *schoolID, item.tag, "是", effectiveYear, "xyl"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school", file, rows, "school_policy_tag")
}

func (i *Importer) importSchoolTagsFromRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		effectiveYear := int16(2025)
		_ = insertSchoolTag(ctx, i.db, *schoolID, "school_nature", boolToNature(row["is_public"]), effectiveYear, "rzy")
		_ = insertSchoolTag(ctx, i.db, *schoolID, "school_type", row["label"], effectiveYear, "rzy")
		_ = insertSchoolTag(ctx, i.db, *schoolID, "department", row["subordinate"], effectiveYear, "rzy")
		if strings.TrimSpace(row["dual_class_name"]) != "" {
			_ = insertSchoolTag(ctx, i.db, *schoolID, "dual_class", row["dual_class_name"], effectiveYear, "rzy")
		}
		if isTrueFlag(row["f985"]) {
			_ = insertSchoolTag(ctx, i.db, *schoolID, "985", "是", effectiveYear, "rzy")
		}
		if isTrueFlag(row["f211"]) {
			_ = insertSchoolTag(ctx, i.db, *schoolID, "211", "是", effectiveYear, "rzy")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school", file, rows, "school_policy_tag")
}

func (i *Importer) importMajorTagsFromRzyMajor(ctx context.Context) error {
	const file = "rzy_365_zr_major.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		majorID := majorIDFromCodeOrName(row["major_code"], row["major_title"], i.majorCodeToID)
		if majorID == nil {
			return nil
		}
		effectiveYear := int16(2025)
		if strings.TrimSpace(row["degree"]) != "" {
			_ = insertMajorTag(ctx, i.db, *majorID, "degree", row["degree"], effectiveYear, "rzy")
		}
		if strings.TrimSpace(row["system"]) != "" {
			_ = insertMajorTag(ctx, i.db, *majorID, "study_years", row["system"], effectiveYear, "rzy")
		}
		code := strings.TrimSpace(row["major_code"])
		if suffix := detectMajorCodeSuffix(code); suffix != "" {
			_ = insertMajorTag(ctx, i.db, *majorID, "code_suffix", suffix, effectiveYear, "rzy")
			_ = insertMajorTag(ctx, i.db, *majorID, "controlled_major", "是", effectiveYear, "rzy")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_major", file, rows, "major_policy_tag")
}

func (i *Importer) importMajorTagsFromXGK(ctx context.Context) error {
	if i.skipXGK {
		return nil
	}
	const file = "rzy_365_zr_xgk_major.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		majorID := majorIDFromCodeOrName(row["major_code"], row["spname"], i.majorCodeToID)
		if majorID == nil {
			return nil
		}
		year := int16(intValueDefault(row["year"], 2025))
		if strings.TrimSpace(row["level1_name"]) != "" {
			_ = insertMajorTag(ctx, i.db, *majorID, "major_level", row["level1_name"], year, "rzy")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_xgk_major", file, rows, "major_policy_tag")
}

func (i *Importer) importPoliciesFromProvinceScores(ctx context.Context) error {
	const file = "xyl_public_data_xyl_province_scores.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if provinceID == nil || year == nil {
			return nil
		}
		examModel := normalizeExamModel(row["model"])
		return batch.Queue(`
INSERT INTO gaokao.admission_policy (province_id, policy_year, exam_model, volunteer_mode, batch_settings, has_major_group, has_parallel_vol, scoreline_type)
VALUES ($1, $2, $3, $4, '[]'::jsonb, $5, true, NULL)
ON CONFLICT (province_id, policy_year) DO UPDATE
SET exam_model = COALESCE(EXCLUDED.exam_model, gaokao.admission_policy.exam_model),
    volunteer_mode = COALESCE(EXCLUDED.volunteer_mode, gaokao.admission_policy.volunteer_mode),
    has_major_group = EXCLUDED.has_major_group,
    updated_at = now()
`, *provinceID, *year, examModel, normalizeVolunteerMode(false, examModel), examModel != "traditional")
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_province_scores", file, rows, "admission_policy")
}

func (i *Importer) importPoliciesFromXGKProvince(ctx context.Context) error {
	if i.skipXGK {
		return nil
	}
	const file = "rzy_365_zr_xgk_province.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if provinceID == nil || year == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.admission_policy (province_id, policy_year, exam_model, volunteer_mode, batch_settings, has_major_group, has_parallel_vol, scoreline_type, policy_note)
VALUES ($1, $2, 'other', 'school_major_group', '[]'::jsonb, true, true, NULL, 'from rzy_xgk_province')
ON CONFLICT (province_id, policy_year) DO UPDATE
SET volunteer_mode = COALESCE(gaokao.admission_policy.volunteer_mode, EXCLUDED.volunteer_mode),
    has_major_group = true,
    updated_at = now()
`, *provinceID, *year)
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_xgk_province", file, rows, "admission_policy")
}

func (i *Importer) importSchoolMajorCatalogFromXylSchoolMajor(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_major.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["major_code"], row["major_name"], i.majorCodeToID)
		return batch.Queue(`
INSERT INTO gaokao.school_major_catalog (
    school_id, major_id, major_code, major_name, school_major_name, study_years_text, observed_year, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,NULLIF($6,''),NULL,'xyl','xyl_school_major',NULL,$7)
ON CONFLICT DO NOTHING
`, *schoolID, sqlInt64(majorID), row["major_code"], row["major_name"], firstNonEmpty(row["major_name"], row["major_type"]), row["year"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_major", file, rows, "")
}

func (i *Importer) importSchoolMajorCatalogFromRzySchoolMajor(ctx context.Context) error {
	const file = "rzy_365_zr_school_major.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["code"], row["name"], i.majorCodeToID)
		return batch.Queue(`
INSERT INTO gaokao.school_major_catalog (
    school_id, major_id, major_code, major_name, school_major_name, study_years_text, observed_year, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,NULL,NULLIF($6,'')::smallint,'rzy','zr_school_major',NULL,$7)
ON CONFLICT DO NOTHING
`, *schoolID, sqlInt64(majorID), row["code"], row["name"], row["name"], row["year"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school_major", file, rows, "")
}

func (i *Importer) importSchoolMajorCatalogFromXylSchoolMajorCode(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_major_code.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		if schoolID == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["school_name"]); err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["major_code"], row["major_name"], i.majorCodeToID)
		return batch.Queue(`
INSERT INTO gaokao.school_major_catalog (
    school_id, major_id, major_code, major_name, school_major_name, study_years_text, observed_year, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,NULL,NULLIF($6,'')::smallint,'xyl','xyl_school_major_code',NULL,$7)
ON CONFLICT DO NOTHING
`, *schoolID, sqlInt64(majorID), row["major_code"], row["major_name"], row["major_name"], row["year"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_major_code", file, rows, "")
}

func (i *Importer) importSubjectRequirementsFromAdmission(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_admission_score.csv"
	rows, err := i.readCSV(file, func(row csvRow) error {
		_, err := i.upsertSubjectRequirement(ctx, row["elective_info"])
		return err
	})
	if err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_admission_score", file, rows, "subject_requirement_dim")
}

func (i *Importer) importSubjectRequirementsFromPlan(ctx context.Context) error {
	files := []struct {
		file   string
		source string
		table  string
		field  string
	}{
		{"xyl_public_data_xyl_school_enroll_plan.csv", "xyl", "xyl_school_enroll_plan", "group_content"},
		{"rzy_365_zr_school_enrplan.csv", "rzy", "zr_school_enrplan", "selected_subject_requirements"},
	}
	for _, item := range files {
		rows, err := i.readCSV(item.file, func(row csvRow) error {
			_, err := i.upsertSubjectRequirement(ctx, row[item.field])
			return err
		})
		if err != nil {
			return err
		}
		if err := i.logImport(ctx, item.source, item.table, item.file, rows, "subject_requirement_dim"); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) importSubjectRequirementsFromXGK(ctx context.Context) error {
	if i.skipXGK {
		return nil
	}
	files := []struct {
		file   string
		field  string
		source string
		table  string
	}{
		{"xyl_public_data_xyl_xgk_elective_major_match_detail_11.csv", "type_name", "xyl", "xyl_xgk_elective_major_match_detail_11"},
		{"xyl_public_data_xyl_xgk_elective_major_match_detail_12.csv", "type_name", "xyl", "xyl_xgk_elective_major_match_detail_12"},
		{"rzy_365_zr_xgk_elective_major.csv", "type_name", "rzy", "zr_xgk_elective_major"},
	}
	for _, item := range files {
		rows, err := i.readCSV(item.file, func(row csvRow) error {
			_, err := i.upsertSubjectRequirement(ctx, row[item.field])
			return err
		})
		if err != nil {
			return err
		}
		if err := i.logImport(ctx, item.source, item.table, item.file, rows, "subject_requirement_dim"); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) importSchoolMajorGroupsFromAdmission(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_admission_score.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		groupName := strings.TrimSpace(row["group_name"])
		if groupName == "" {
			return nil
		}
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		subjectReqID, err := i.upsertSubjectRequirement(ctx, row["elective_info"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_major_group (
    school_id, province_id, group_year, group_name, group_code, subject_req_id,
    group_plan_count, lowest_score, lowest_rank, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,NULL,$5,NULL,$6,$7,'xyl','xyl_school_admission_score',$8,$9)
ON CONFLICT (school_id, province_id, group_year, COALESCE(group_code, ''), group_name, COALESCE(source_system, ''), COALESCE(source_table, ''))
DO UPDATE SET
    subject_req_id = COALESCE(EXCLUDED.subject_req_id, gaokao.school_major_group.subject_req_id),
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.school_major_group.lowest_score),
    lowest_rank = COALESCE(EXCLUDED.lowest_rank, gaokao.school_major_group.lowest_rank),
    updated_at = now()
`, *schoolID, *provinceID, *year, groupName, sqlInt64(subjectReqID), sqlFloat(floatStringToPtr(row["low"])), sqlInt(intValue(row["rank"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_admission_score", file, rows, "school_major_group")
}

func (i *Importer) importSchoolMajorGroupsFromMajorScore(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_major_score.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		groupName := strings.TrimSpace(row["major_group"])
		if groupName == "" {
			return nil
		}
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		subjectReqID, err := i.upsertSubjectRequirement(ctx, row["major_group_subject"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_major_group (
    school_id, province_id, group_year, group_name, group_code, subject_req_id,
    group_plan_count, lowest_score, lowest_rank, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,NULL,$5,NULL,$6,$7,'xyl','xyl_school_major_score',$8,$9)
ON CONFLICT (school_id, province_id, group_year, COALESCE(group_code, ''), group_name, COALESCE(source_system, ''), COALESCE(source_table, ''))
DO UPDATE SET
    subject_req_id = COALESCE(EXCLUDED.subject_req_id, gaokao.school_major_group.subject_req_id),
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.school_major_group.lowest_score),
    lowest_rank = COALESCE(EXCLUDED.lowest_rank, gaokao.school_major_group.lowest_rank),
    updated_at = now()
`, *schoolID, *provinceID, *year, groupName, sqlInt64(subjectReqID), sqlFloat(floatStringToPtr(row["min"])), sqlInt(intValue(row["rank"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_major_score", file, rows, "school_major_group")
}

func (i *Importer) importSchoolMajorGroupsFromPlan(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_enroll_plan.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		groupName := strings.TrimSpace(row["major_group"])
		if groupName == "" {
			return nil
		}
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["school_name"]); err != nil {
			return err
		}
		subjectReqID, err := i.upsertSubjectRequirement(ctx, row["group_content"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_major_group (
    school_id, province_id, group_year, group_name, group_code, subject_req_id,
    group_plan_count, lowest_score, lowest_rank, source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,NULL,$5,$6,NULL,NULL,'xyl','xyl_school_enroll_plan',$7,$8)
ON CONFLICT (school_id, province_id, group_year, COALESCE(group_code, ''), group_name, COALESCE(source_system, ''), COALESCE(source_table, ''))
DO UPDATE SET
    subject_req_id = COALESCE(EXCLUDED.subject_req_id, gaokao.school_major_group.subject_req_id),
    group_plan_count = COALESCE(EXCLUDED.group_plan_count, gaokao.school_major_group.group_plan_count),
    updated_at = now()
`, *schoolID, *provinceID, *year, groupName, sqlInt64(subjectReqID), sqlInt(intValue(row["number"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_enroll_plan", file, rows, "school_major_group")
}

func (i *Importer) importSchoolAdmissionFacts(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_admission_score.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		policyID, err := i.policyID(ctx, *provinceID, *year, inferExamModel(row["section"], row["elective_info"]))
		if err != nil {
			return err
		}
		groupID, err := i.findSchoolMajorGroupID(ctx, *schoolID, *provinceID, *year, row["group_name"], "xyl_school_admission_score")
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.school_admission_score_fact (
    school_id, province_id, policy_id, school_major_group_id, admission_year,
    raw_batch_name, raw_section_name, raw_admission_type, raw_major_group_name, raw_elective_req,
    highest_score, average_score, lowest_score, lowest_rank, province_control_score, line_deviation,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULL,NULL,$11,$12,$13,$14,'xyl','xyl_school_admission_score',$15,$16)
ON CONFLICT (school_id, province_id, admission_year, COALESCE(raw_batch_name, ''), COALESCE(raw_section_name, ''), COALESCE(raw_admission_type, ''), COALESCE(raw_major_group_name, ''), source_system, source_table)
DO UPDATE SET
    school_major_group_id = COALESCE(EXCLUDED.school_major_group_id, gaokao.school_admission_score_fact.school_major_group_id),
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.school_admission_score_fact.lowest_score),
    lowest_rank = COALESCE(EXCLUDED.lowest_rank, gaokao.school_admission_score_fact.lowest_rank),
    province_control_score = COALESCE(EXCLUDED.province_control_score, gaokao.school_admission_score_fact.province_control_score),
    line_deviation = COALESCE(EXCLUDED.line_deviation, gaokao.school_admission_score_fact.line_deviation)
`, *schoolID, *provinceID, policyID, sqlInt64(groupID), *year, row["batch"], row["section"], row["type"], row["group_name"], row["elective_info"], sqlFloat(floatStringToPtr(row["low"])), sqlInt(intValue(row["rank"])), sqlFloat(floatStringToPtr(row["pro_control"])), sqlFloat(floatStringToPtr(row["line_deviation"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_admission_score", file, rows, "")
}

func (i *Importer) importMajorAdmissionFactsXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_major_score.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		policyID, err := i.policyID(ctx, *provinceID, *year, inferExamModel(row["section"], row["major_group_subject"]))
		if err != nil {
			return err
		}
		groupID, err := i.findSchoolMajorGroupID(ctx, *schoolID, *provinceID, *year, row["major_group"], "xyl_school_major_score")
		if err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["major_code"], row["major"], i.majorCodeToID)
		schoolMajorID, err := i.findSchoolMajorCatalogID(ctx, *schoolID, row["major_code"], row["major"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.major_admission_score_fact (
    school_id, major_id, school_major_id, province_id, policy_id, school_major_group_id, admission_year,
    raw_batch_name, raw_section_name, raw_admission_type, raw_major_group_name, raw_elective_req,
    school_major_name, major_code, highest_score, average_score, lowest_score, lowest_rank, line_deviation,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULL,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULL,NULL,$14,$15,$16,'xyl','xyl_school_major_score',$17,$18)
ON CONFLICT (school_id, province_id, admission_year, COALESCE(raw_batch_name, ''), COALESCE(raw_section_name, ''), COALESCE(raw_admission_type, ''), COALESCE(raw_major_group_name, ''), COALESCE(school_major_id, 0), source_system, source_table)
DO UPDATE SET
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.major_admission_score_fact.lowest_score),
    lowest_rank = COALESCE(EXCLUDED.lowest_rank, gaokao.major_admission_score_fact.lowest_rank),
    line_deviation = COALESCE(EXCLUDED.line_deviation, gaokao.major_admission_score_fact.line_deviation)
`, *schoolID, sqlInt64(majorID), sqlInt64(schoolMajorID), *provinceID, policyID, sqlInt64(groupID), *year, row["batch_for_one"], row["section"], row["major_group"], row["major_group_subject"], row["major"], row["major_code"], sqlFloat(floatStringToPtr(row["min"])), sqlInt(intValue(row["rank"])), sqlFloat(floatStringToPtr(row["line_deviation"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_major_score", file, rows, "")
}

func (i *Importer) importMajorAdmissionFactsRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school_major_score.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		policyID, err := i.policyID(ctx, *provinceID, *year, inferExamModel(row["section"], row["selected_subject_requirements"]))
		if err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName("", row["title"], i.majorCodeToID)
		schoolMajorID, err := i.findSchoolMajorCatalogID(ctx, *schoolID, "", row["title"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.major_admission_score_fact (
    school_id, major_id, school_major_id, province_id, policy_id, school_major_group_id, admission_year,
    raw_batch_name, raw_section_name, raw_admission_type, raw_major_group_name, raw_elective_req,
    school_major_name, major_code, highest_score, average_score, lowest_score, lowest_rank, line_deviation,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,$5,NULL,$6,NULLIF($7,''),NULLIF($8,''),NULL,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULL,$12,$13,$14,$15,NULL,'rzy','zr_school_major_score',$16,$17)
ON CONFLICT (school_id, province_id, admission_year, COALESCE(raw_batch_name, ''), COALESCE(raw_section_name, ''), COALESCE(raw_admission_type, ''), COALESCE(raw_major_group_name, ''), COALESCE(school_major_id, 0), source_system, source_table)
DO UPDATE SET
    highest_score = COALESCE(EXCLUDED.highest_score, gaokao.major_admission_score_fact.highest_score),
    average_score = COALESCE(EXCLUDED.average_score, gaokao.major_admission_score_fact.average_score),
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.major_admission_score_fact.lowest_score),
    lowest_rank = COALESCE(EXCLUDED.lowest_rank, gaokao.major_admission_score_fact.lowest_rank)
`, *schoolID, sqlInt64(majorID), sqlInt64(schoolMajorID), *provinceID, policyID, *year, row["batch"], row["section"], row["major_group"], row["selected_subject_requirements"], row["title"], sqlFloat(floatStringToPtr(row["highest_score"])), sqlFloat(floatStringToPtr(row["avg"])), sqlFloat(floatStringToPtr(row["lowest_score"])), sqlInt(intValue(row["lowest_order"])), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school_major_score", file, rows, "")
}

func (i *Importer) importEnrollmentPlanFactsXyl(ctx context.Context) error {
	const file = "xyl_public_data_xyl_school_enroll_plan.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, row["school_name"]); err != nil {
			return err
		}
		policyID, err := i.policyID(ctx, *provinceID, *year, inferExamModel(row["type"], row["group_content"]))
		if err != nil {
			return err
		}
		groupID, err := i.findSchoolMajorGroupID(ctx, *schoolID, *provinceID, *year, row["major_group"], "xyl_school_enroll_plan")
		if err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["major_enrplan_code"], row["major_name"], i.majorCodeToID)
		schoolMajorID, err := i.findSchoolMajorCatalogID(ctx, *schoolID, row["major_enrplan_code"], row["major_name"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.enrollment_plan_fact (
    school_id, major_id, school_major_id, province_id, policy_id, school_major_group_id, plan_year,
    raw_batch_name, raw_section_name, raw_admission_type, raw_major_group_name, raw_elective_req,
    school_major_name, major_code, plan_count, tuition_fee, study_years_text, school_code, major_plan_code,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULL,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),$14,$15,NULLIF($16,''),NULLIF($17,''),NULLIF($18,''),'xyl','xyl_school_enroll_plan',$19,$20)
ON CONFLICT (school_id, province_id, plan_year, COALESCE(raw_batch_name, ''), COALESCE(raw_section_name, ''), COALESCE(raw_admission_type, ''), COALESCE(raw_major_group_name, ''), COALESCE(school_major_id, 0), source_system, source_table)
DO UPDATE SET
    plan_count = COALESCE(EXCLUDED.plan_count, gaokao.enrollment_plan_fact.plan_count),
    tuition_fee = COALESCE(EXCLUDED.tuition_fee, gaokao.enrollment_plan_fact.tuition_fee)
`, *schoolID, sqlInt64(majorID), sqlInt64(schoolMajorID), *provinceID, policyID, sqlInt64(groupID), *year, row["batch"], row["type"], row["major_group"], row["group_content"], row["major_name"], row["major_enrplan_code"], sqlInt(intValue(row["number"])), sqlFloat(floatStringToPtr(row["tuition_fee"])), row["major_year"], row["school_code"], row["major_enrplan_code"], row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_school_enroll_plan", file, rows, "")
}

func (i *Importer) importEnrollmentPlanFactsRzy(ctx context.Context) error {
	const file = "rzy_365_zr_school_enrplan.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		schoolID := int64Value(row["school_id"])
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if schoolID == nil || provinceID == nil || year == nil {
			return nil
		}
		if err := i.ensureSchoolExists(ctx, *schoolID, ""); err != nil {
			return err
		}
		policyID, err := i.policyID(ctx, *provinceID, *year, inferExamModel(row["section"], row["selected_subject_requirements"]))
		if err != nil {
			return err
		}
		majorID := majorIDFromCodeOrName(row["major_code"], row["title"], i.majorCodeToID)
		schoolMajorID, err := i.findSchoolMajorCatalogID(ctx, *schoolID, row["major_code"], row["title"])
		if err != nil {
			return err
		}
		return batch.Queue(`
INSERT INTO gaokao.enrollment_plan_fact (
    school_id, major_id, school_major_id, province_id, policy_id, school_major_group_id, plan_year,
    raw_batch_name, raw_section_name, raw_admission_type, raw_major_group_name, raw_elective_req,
    school_major_name, major_code, plan_count, tuition_fee, study_years_text, school_code, major_plan_code,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,$5,NULL,$6,NULLIF($7,''),NULLIF($8,''),NULL,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,NULLIF($15,''),NULLIF($16,''),NULLIF($17,''),'rzy','zr_school_enrplan',$18,$19)
ON CONFLICT (school_id, province_id, plan_year, COALESCE(raw_batch_name, ''), COALESCE(raw_section_name, ''), COALESCE(raw_admission_type, ''), COALESCE(raw_major_group_name, ''), COALESCE(school_major_id, 0), source_system, source_table)
DO UPDATE SET
    plan_count = COALESCE(EXCLUDED.plan_count, gaokao.enrollment_plan_fact.plan_count),
    tuition_fee = COALESCE(EXCLUDED.tuition_fee, gaokao.enrollment_plan_fact.tuition_fee)
`, *schoolID, sqlInt64(majorID), sqlInt64(schoolMajorID), *provinceID, policyID, *year, row["batch"], row["section"], row["major_group"], row["selected_subject_requirements"], row["title"], row["major_code"], sqlInt(intValue(row["pro_enr"])), sqlFloat(floatStringToPtr(row["tuition"])), row["system"], row["school_code"], row["major_code"], row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "rzy", "zr_school_enrplan", file, rows, "")
}

func (i *Importer) importProvinceBatchLineFacts(ctx context.Context) error {
	files := []struct {
		file   string
		source string
		table  string
		fn     func(context.Context, csvRow) error
	}{
		{"xyl_public_data_xyl_province_batch.csv", "xyl", "xyl_province_batch", i.insertProvinceBatchLineXyl},
		{"rzy_365_zr_province_fractional.csv", "rzy", "zr_province_fractional", i.insertProvinceBatchLineRzy},
	}
	for _, item := range files {
		rows, err := i.readCSV(item.file, func(row csvRow) error {
			return item.fn(ctx, row)
		})
		if err != nil {
			return err
		}
		if err := i.logImport(ctx, item.source, item.table, item.file, rows, "province_batch_line_fact"); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) insertProvinceBatchLineXyl(ctx context.Context, row csvRow) error {
	provinceID := intValue(row["province_id"])
	year := intValue(row["year"])
	if provinceID == nil || year == nil {
		return nil
	}
	policyID, err := i.policyID(ctx, *provinceID, *year, "traditional")
	if err != nil {
		return err
	}
	_, err = i.db.Exec(ctx, `
INSERT INTO gaokao.province_batch_line_fact (
    province_id, policy_id, score_year, raw_batch_name, raw_category_name, raw_section_name, score_value, rank_value,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULL,$6,$7,'xyl','xyl_province_batch',$8,$9)
ON CONFLICT (province_id, score_year, COALESCE(raw_batch_name, ''), COALESCE(raw_category_name, ''), COALESCE(raw_section_name, ''), source_system, source_table)
DO UPDATE SET
    score_value = EXCLUDED.score_value,
    rank_value = COALESCE(EXCLUDED.rank_value, gaokao.province_batch_line_fact.rank_value)
`, *provinceID, policyID, *year, row["batch"], row["category"], sqlFloat(floatStringToPtr(row["score"])), sqlInt(intValue(row["rank"])), row["id"], marshalJSON(row))
	return err
}

func (i *Importer) insertProvinceBatchLineRzy(ctx context.Context, row csvRow) error {
	provinceID := intValue(row["province_id"])
	year := intValue(row["year"])
	if provinceID == nil || year == nil {
		return nil
	}
	policyID, err := i.policyID(ctx, *provinceID, *year, "traditional")
	if err != nil {
		return err
	}
	_, err = i.db.Exec(ctx, `
INSERT INTO gaokao.province_batch_line_fact (
    province_id, policy_id, score_year, raw_batch_name, raw_category_name, raw_section_name, score_value, rank_value,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULL,$6,NULL,'rzy','zr_province_fractional',$7,$8)
ON CONFLICT (province_id, score_year, COALESCE(raw_batch_name, ''), COALESCE(raw_category_name, ''), COALESCE(raw_section_name, ''), source_system, source_table)
DO UPDATE SET
    score_value = EXCLUDED.score_value
`, *provinceID, policyID, *year, row["batch"], row["type_name"], sqlFloat(floatStringToPtr(row["fractional"])), row["id"], marshalJSON(row))
	return err
}

func (i *Importer) importProvinceScoreRangeFacts(ctx context.Context) error {
	const file = "xyl_public_data_xyl_province_scores.csv"
	batch := newBatchExecutor(ctx, i)
	rows, err := i.readCSV(file, func(row csvRow) error {
		provinceID := intValue(row["province_id"])
		year := intValue(row["year"])
		if provinceID == nil || year == nil {
			return nil
		}
		return batch.Queue(`
INSERT INTO gaokao.province_score_range_fact (
    province_id, score_year, highest_score, lowest_score, exam_model,
    source_system, source_table, source_pk, raw_payload
)
VALUES ($1,$2,$3,$4,NULLIF($5,''),'xyl','xyl_province_scores',$6,$7)
ON CONFLICT (province_id, score_year, source_system, source_table)
DO UPDATE SET
    highest_score = COALESCE(EXCLUDED.highest_score, gaokao.province_score_range_fact.highest_score),
    lowest_score = COALESCE(EXCLUDED.lowest_score, gaokao.province_score_range_fact.lowest_score),
    exam_model = COALESCE(EXCLUDED.exam_model, gaokao.province_score_range_fact.exam_model)
`, *provinceID, *year, sqlFloat(floatStringToPtr(row["highest_score"])), sqlFloat(floatStringToPtr(row["lowest_score"])), normalizeExamModel(row["model"]), row["id"], marshalJSON(row))
	})
	if err != nil {
		return err
	}
	if err := batch.Flush(); err != nil {
		return err
	}
	return i.logImport(ctx, "xyl", "xyl_province_scores", file, rows, "")
}

func (i *Importer) findSchoolMajorGroupID(ctx context.Context, schoolID int64, provinceID, year int, groupName, sourceTable string) (*int64, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil, nil
	}
	cacheKey := fmt.Sprintf("%d:%d:%d:%s", schoolID, provinceID, year, groupName)
	if id, ok := i.schoolMajorGroupIDCache[cacheKey]; ok {
		return id, nil
	}

	var id int64
	err := i.db.QueryRow(ctx, `
SELECT school_major_group_id
FROM gaokao.school_major_group
WHERE school_id = $1 AND province_id = $2 AND group_year = $3 AND group_name = $4
LIMIT 1
`, schoolID, provinceID, year, groupName).Scan(&id)
	if err == nil {
		idPtr := &id
		i.schoolMajorGroupIDCache[cacheKey] = idPtr
		return idPtr, nil
	}
	if err == pgx.ErrNoRows {
		i.schoolMajorGroupIDCache[cacheKey] = nil
		return nil, nil
	}
	return nil, err
}

func (i *Importer) findSchoolMajorCatalogID(ctx context.Context, schoolID int64, majorCode, schoolMajorName string) (*int64, error) {
	majorCode = strings.TrimSpace(majorCode)
	schoolMajorName = strings.TrimSpace(schoolMajorName)
	cacheKey := fmt.Sprintf("%d:%s:%s", schoolID, majorCode, schoolMajorName)
	if id, ok := i.schoolMajorCatalogIDCache[cacheKey]; ok {
		return id, nil
	}

	var row pgx.Row
	if majorCode != "" {
		row = i.db.QueryRow(ctx, `
SELECT school_major_id
FROM gaokao.school_major_catalog
WHERE school_id = $1 AND major_code = $2
ORDER BY school_major_id
LIMIT 1
`, schoolID, majorCode)
	} else {
		row = i.db.QueryRow(ctx, `
SELECT school_major_id
FROM gaokao.school_major_catalog
WHERE school_id = $1 AND school_major_name = $2
ORDER BY school_major_id
LIMIT 1
`, schoolID, schoolMajorName)
	}
	var id int64
	err := row.Scan(&id)
	if err == nil {
		idPtr := &id
		i.schoolMajorCatalogIDCache[cacheKey] = idPtr
		return idPtr, nil
	}
	if err == pgx.ErrNoRows {
		i.schoolMajorCatalogIDCache[cacheKey] = nil
		return nil, nil
	}
	return nil, err
}

func (i *Importer) ensureSchoolExists(ctx context.Context, schoolID int64, schoolName string) error {
	if schoolID == 0 {
		return nil
	}
	if _, ok := i.schoolExistsCache[schoolID]; ok {
		return nil
	}
	name := strings.TrimSpace(schoolName)
	if name == "" {
		name = fmt.Sprintf("学校_%d", schoolID)
	}
	_, err := i.db.Exec(ctx, `
INSERT INTO gaokao.school (school_id, school_name)
VALUES ($1, $2)
ON CONFLICT (school_id) DO NOTHING
`, schoolID, name)
	if err != nil {
		return err
	}
	i.schoolExistsCache[schoolID] = struct{}{}
	return nil
}

func insertSchoolTag(ctx context.Context, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, schoolID int64, tagType, tagValue string, effectiveYear int16, sourceSystem string) error {
	if strings.TrimSpace(tagValue) == "" {
		return nil
	}
	_, err := db.Exec(ctx, `
INSERT INTO gaokao.school_policy_tag (school_id, tag_type, tag_value, effective_year, source_system)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (school_id, tag_type, effective_year, tag_value) DO NOTHING
`, schoolID, tagType, tagValue, effectiveYear, sourceSystem)
	return err
}

func insertMajorTag(ctx context.Context, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, majorID int64, tagType, tagValue string, effectiveYear int16, sourceSystem string) error {
	if strings.TrimSpace(tagValue) == "" {
		return nil
	}
	_, err := db.Exec(ctx, `
INSERT INTO gaokao.major_policy_tag (major_id, tag_type, tag_value, effective_year, source_system)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (major_id, tag_type, effective_year, tag_value) DO NOTHING
`, majorID, tagType, tagValue, effectiveYear, sourceSystem)
	return err
}

func detectMajorCodeSuffix(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	switch {
	case strings.HasSuffix(code, "TK"):
		return "TK"
	case strings.HasSuffix(code, "K"):
		return "K"
	case strings.HasSuffix(code, "T"):
		return "T"
	default:
		return ""
	}
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func isTrueFlag(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "b'\\x01'"
}

func boolToNature(s string) string {
	if isTrueFlag(s) {
		return "公办"
	}
	if strings.TrimSpace(s) != "" {
		return "民办"
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstInt(values ...string) *int {
	for _, value := range values {
		if parsed := intValue(value); parsed != nil {
			return parsed
		}
	}
	return nil
}
