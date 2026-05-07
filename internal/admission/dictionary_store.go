package admission

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DictionaryStore interface {
	ListDictionaries(ctx context.Context) (*DictionaryResponse, error)
}

type dictionaryStore struct {
	pool *pgxpool.Pool
}

func NewDictionaryStore(pool *pgxpool.Pool) DictionaryStore {
	return &dictionaryStore{pool: pool}
}

func (s *dictionaryStore) ListDictionaries(ctx context.Context) (*DictionaryResponse, error) {
	regions, err := s.listDictionaryItems(ctx, "regions")
	if err != nil {
		return nil, err
	}
	subjectCategories, err := s.listDictionaryItems(ctx, "subject_categories")
	if err != nil {
		return nil, err
	}
	subjectRequirements, err := s.listDictionaryItems(ctx, "subject_requirements")
	if err != nil {
		return nil, err
	}
	batches, err := s.listDictionaryItems(ctx, "batches")
	if err != nil {
		return nil, err
	}
	educationLevels, err := s.listDictionaryItems(ctx, "education_levels")
	if err != nil {
		return nil, err
	}
	ownershipTypes, err := s.listDictionaryItems(ctx, "school_ownership_types")
	if err != nil {
		return nil, err
	}
	schoolCategories, err := s.listDictionaryItems(ctx, "school_categories")
	if err != nil {
		return nil, err
	}
	return &DictionaryResponse{
		Regions:              regions,
		SubjectCategories:    subjectCategories,
		SubjectRequirements:  subjectRequirements,
		Batches:              batches,
		EducationLevels:      educationLevels,
		SchoolOwnershipTypes: ownershipTypes,
		SchoolCategories:     schoolCategories,
	}, nil
}

func (s *dictionaryStore) listDictionaryItems(ctx context.Context, table string) ([]DictionaryItem, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf("SELECT code, name FROM %s ORDER BY code", table))
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", table, err)
	}
	defer rows.Close()

	items := []DictionaryItem{}
	for rows.Next() {
		var item DictionaryItem
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", table, err)
	}
	return items, nil
}
