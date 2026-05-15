package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubImportStore struct {
	dictionaries map[string]map[string]bool
	duplicate    bool
}

func (s stubImportStore) DictionaryCodeExists(ctx context.Context, table, code string) (bool, error) {
	if s.dictionaries == nil || s.dictionaries[table] == nil {
		return false, nil
	}
	return s.dictionaries[table][code], nil
}

func (s stubImportStore) AdmissionLineExists(ctx context.Context, groupKey *AdmissionGroupKey, localMajorCode string) (bool, error) {
	return s.duplicate, nil
}

func TestValidateAdmissionImportRowsAcceptsValidRow(t *testing.T) {
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{validImportRow()})

	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessRows)
	require.Zero(t, result.FailedRows)
	require.Empty(t, result.Errors)
}

func TestValidateAdmissionImportRowsRejectsUnknownDictionaryCode(t *testing.T) {
	row := validImportRow()
	row.RegionCode = "999999"
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{row})

	require.NoError(t, err)
	require.Zero(t, result.SuccessRows)
	require.Equal(t, 1, result.FailedRows)
	require.Contains(t, result.Errors[0].Message, "region_code")
}

func TestValidateAdmissionImportRowsRejectsMissingLocalMajorCode(t *testing.T) {
	row := validImportRow()
	row.LocalMajorCode = ""
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{row})

	require.NoError(t, err)
	require.Zero(t, result.SuccessRows)
	require.Equal(t, 1, result.FailedRows)
	require.Contains(t, result.Errors[0].Message, "local_major_code")
}

func TestValidateAdmissionImportRowsRejectsDuplicateAdmissionLine(t *testing.T) {
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
		duplicate:    true,
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{validImportRow()})

	require.NoError(t, err)
	require.Zero(t, result.SuccessRows)
	require.Equal(t, 1, result.FailedRows)
	require.Contains(t, result.Errors[0].Message, "duplicate")
}

func TestValidateAdmissionImportRowsAllowsCatalogYearMismatchBecauseChsiIsOnlyATag(t *testing.T) {
	row := validImportRow()
	row.CatalogYear = 2024
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{row})

	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessRows)
	require.Zero(t, result.FailedRows)
}

func TestValidateAdmissionImportRowsRejectsNonNumericScore(t *testing.T) {
	row := validImportRow()
	row.MinScore = "seven hundred"
	service := NewImportService(stubImportStore{
		dictionaries: validImportDictionaries(),
	})

	result, err := service.ValidateRows(context.Background(), []AdmissionImportRow{row})

	require.NoError(t, err)
	require.Zero(t, result.SuccessRows)
	require.Equal(t, 1, result.FailedRows)
	require.Contains(t, result.Errors[0].Message, "min_score")
}

func validImportRow() AdmissionImportRow {
	return AdmissionImportRow{
		SourceRowNumber:        3,
		AdmissionYear:          2025,
		CatalogYear:            2025,
		UniversityCode:         "1003",
		UniversityName:         "清华大学",
		RegionCode:             "230000",
		SubjectCategoryCode:    "physics",
		BatchCode:              "regular_undergraduate",
		GroupCode:              "008",
		SubjectRequirementCode: "chemistry",
		LocalMajorCode:         "25",
		LocalMajorName:         "计算机类",
		AdmittedCount:          "1",
		MinScore:               "721",
		MinRank:                "45",
	}
}

func validImportDictionaries() map[string]map[string]bool {
	return map[string]map[string]bool{
		"regions":              {"230000": true},
		"subject_categories":   {"physics": true},
		"subject_requirements": {"chemistry": true},
		"batches":              {"regular_undergraduate": true},
	}
}
