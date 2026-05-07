package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubDictionaryStore struct {
	resp *DictionaryResponse
	err  error
}

func (s stubDictionaryStore) ListDictionaries(ctx context.Context) (*DictionaryResponse, error) {
	return s.resp, s.err
}

func TestDictionaryServiceReturnsAllDictionaryGroups(t *testing.T) {
	service := NewDictionaryService(stubDictionaryStore{resp: &DictionaryResponse{
		Regions:              []DictionaryItem{{Code: "230000", Name: "黑龙江省"}},
		SubjectCategories:    []DictionaryItem{{Code: "physics", Name: "物理"}},
		SubjectRequirements:  []DictionaryItem{{Code: "chemistry", Name: "化学"}},
		Batches:              []DictionaryItem{{Code: "regular_undergraduate", Name: "普通本科批"}},
		EducationLevels:      []DictionaryItem{{Code: "undergraduate", Name: "本科"}},
		SchoolOwnershipTypes: []DictionaryItem{{Code: "public", Name: "公办"}},
		SchoolCategories:     []DictionaryItem{{Code: "comprehensive", Name: "综合类"}},
	}})

	resp, err := service.ListDictionaries(context.Background())

	require.NoError(t, err)
	require.Len(t, resp.Regions, 1)
	require.Len(t, resp.SubjectCategories, 1)
	require.Len(t, resp.SubjectRequirements, 1)
	require.Len(t, resp.Batches, 1)
	require.Len(t, resp.EducationLevels, 1)
	require.Len(t, resp.SchoolOwnershipTypes, 1)
	require.Len(t, resp.SchoolCategories, 1)
}
