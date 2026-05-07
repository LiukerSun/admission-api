package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubAdmissionLineStore struct {
	lines []AdmissionLineResponse
	err   error
}

func (s stubAdmissionLineStore) ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error) {
	return s.lines, s.err
}

func TestAdmissionLineServiceListsAdmissionLines(t *testing.T) {
	service := NewAdmissionLineService(stubAdmissionLineStore{lines: []AdmissionLineResponse{
		{
			UniversityCode: "1003",
			UniversityName: "清华大学",
			GroupCode:      "008",
			LocalMajorCode: "25",
			LocalMajorName: "计算机类",
		},
	}})

	lines, err := service.ListAdmissionLines(context.Background(), &AdmissionLineFilter{RegionCode: "230000"})

	require.NoError(t, err)
	require.Len(t, lines, 1)
	require.Equal(t, "25", lines[0].LocalMajorCode)
}
