package admission

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MajorCatalogStore interface {
	LatestCatalogYear(ctx context.Context) (int, error)
	ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error)
}

type majorCatalogStore struct {
	pool *pgxpool.Pool
}

func NewMajorCatalogStore(pool *pgxpool.Pool) MajorCatalogStore {
	return &majorCatalogStore{pool: pool}
}

func (s *majorCatalogStore) LatestCatalogYear(ctx context.Context) (int, error) {
	var year int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(catalog_year), 0) FROM standard_majors`).Scan(&year); err != nil {
		return 0, fmt.Errorf("get latest major catalog year: %w", err)
	}
	return year, nil
}

func (s *majorCatalogStore) ListStandardMajors(ctx context.Context, filter StandardMajorFilter) ([]StandardMajorResponse, error) {
	yearExpr := `(SELECT COALESCE(MAX(catalog_year), 0) FROM standard_majors)`
	args := []any{}
	if filter.CatalogYear != nil {
		args = append(args, *filter.CatalogYear)
		yearExpr = "$1"
	}

	query := fmt.Sprintf(`
		SELECT sm.id, sm.catalog_year, sm.major_code, sm.name,
		       sm.category_code, mc.name AS category_name,
		       sm.class_code, mcl.name AS class_name,
		       COALESCE(sm.duration, ''), COALESCE(sm.degree_category, ''), COALESCE(sm.source_url, '')
		FROM standard_majors sm
		JOIN major_categories mc
		  ON mc.catalog_year = sm.catalog_year AND mc.category_code = sm.category_code
		JOIN major_classes mcl
		  ON mcl.catalog_year = sm.catalog_year AND mcl.class_code = sm.class_code
		WHERE sm.catalog_year = %s
	`, yearExpr)

	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		query += fmt.Sprintf(" AND (sm.major_code ILIKE $%d OR sm.name ILIKE $%d)", len(args), len(args))
	}
	query += " ORDER BY sm.major_code"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list standard majors: %w", err)
	}
	defer rows.Close()

	majors := []StandardMajorResponse{}
	for rows.Next() {
		var major StandardMajorResponse
		if err := rows.Scan(
			&major.ID,
			&major.CatalogYear,
			&major.MajorCode,
			&major.Name,
			&major.CategoryCode,
			&major.CategoryName,
			&major.ClassCode,
			&major.ClassName,
			&major.Duration,
			&major.DegreeCategory,
			&major.SourceURL,
		); err != nil {
			return nil, fmt.Errorf("scan standard major: %w", err)
		}
		majors = append(majors, major)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate standard majors: %w", err)
	}
	return majors, nil
}
