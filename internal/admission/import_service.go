package admission

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type ImportStore interface {
	DictionaryCodeExists(ctx context.Context, table, code string) (bool, error)
	AdmissionLineExists(ctx context.Context, groupKey *AdmissionGroupKey, localMajorCode string) (bool, error)
}

type ImportService interface {
	ValidateRows(ctx context.Context, rows []AdmissionImportRow) (*ImportValidationResult, error)
}

type importService struct {
	store ImportStore
}

func NewImportService(store ImportStore) ImportService {
	return &importService{store: store}
}

func (s *importService) ValidateRows(ctx context.Context, rows []AdmissionImportRow) (*ImportValidationResult, error) {
	result := &ImportValidationResult{TotalRows: len(rows)}
	for index := range rows {
		row := &rows[index]
		if err := s.validateRow(ctx, row); err != nil {
			result.FailedRows++
			result.Errors = append(result.Errors, ImportRowError{
				RowNumber: row.SourceRowNumber,
				Message:   err.Error(),
			})
			continue
		}
		result.SuccessRows++
	}
	return result, nil
}

func (s *importService) validateRow(ctx context.Context, row *AdmissionImportRow) error {
	if row.AdmissionYear == 0 {
		return fmt.Errorf("admission_year is required")
	}
	dictionaryChecks := []struct {
		table string
		code  string
		field string
	}{
		{"regions", row.RegionCode, "region_code"},
		{"subject_categories", row.SubjectCategoryCode, "subject_category_code"},
		{"subject_requirements", row.SubjectRequirementCode, "subject_requirement_code"},
		{"batches", row.BatchCode, "batch_code"},
	}
	for _, check := range dictionaryChecks {
		if strings.TrimSpace(check.code) == "" {
			return fmt.Errorf("%s is required", check.field)
		}
		ok, err := s.store.DictionaryCodeExists(ctx, check.table, check.code)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s %q is not mapped", check.field, check.code)
		}
	}

	if err := validateOptionalInts(row); err != nil {
		return err
	}

	if strings.TrimSpace(row.LocalMajorCode) == "" {
		return fmt.Errorf("local_major_code is required")
	}
	if strings.TrimSpace(row.LocalMajorName) == "" {
		return fmt.Errorf("local_major_name is required")
	}

	duplicate, err := s.store.AdmissionLineExists(ctx, &AdmissionGroupKey{
		UniversityCode:      row.UniversityCode,
		UniversityName:      row.UniversityName,
		AdmissionYear:       row.AdmissionYear,
		RegionCode:          row.RegionCode,
		SubjectCategoryCode: row.SubjectCategoryCode,
		BatchCode:           row.BatchCode,
		GroupCode:           row.GroupCode,
	}, row.LocalMajorCode)
	if err != nil {
		return err
	}
	if duplicate {
		return fmt.Errorf("duplicate admission line")
	}

	return nil
}

func validateOptionalInts(row *AdmissionImportRow) error {
	fields := []struct {
		name  string
		value string
	}{
		{"plan_count", row.PlanCount},
		{"admitted_count", row.AdmittedCount},
		{"min_score", row.MinScore},
		{"min_rank", row.MinRank},
		{"max_score", row.MaxScore},
		{"max_rank", row.MaxRank},
		{"equivalent_min_score", row.EquivalentMinScore},
		{"tuition", row.Tuition},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if _, err := strconv.Atoi(field.value); err != nil {
			return fmt.Errorf("%s must be a number", field.name)
		}
	}
	return nil
}
