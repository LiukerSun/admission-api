package admission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubUniversityStore struct {
	universities []UniversityResponse
	profile      *UniversityProfileResponse
	err          error
}

func (s stubUniversityStore) ListUniversities(ctx context.Context, filter *UniversityFilter) ([]UniversityResponse, error) {
	return s.universities, s.err
}

func (s stubUniversityStore) GetUniversityProfile(ctx context.Context, universityID int64, profileYear *int) (*UniversityProfileResponse, error) {
	return s.profile, s.err
}

func TestUniversityServiceListsUniversities(t *testing.T) {
	service := NewUniversityService(stubUniversityStore{universities: []UniversityResponse{
		{ID: 1, UniversityCode: "1003", Name: "清华大学"},
	}})

	universities, err := service.ListUniversities(context.Background(), &UniversityFilter{Query: "清华"})

	require.NoError(t, err)
	require.Len(t, universities, 1)
	require.Equal(t, "1003", universities[0].UniversityCode)
}

func TestUniversityServiceGetsProfile(t *testing.T) {
	year := 2025
	service := NewUniversityService(stubUniversityStore{profile: &UniversityProfileResponse{
		UniversityID: 1,
		ProfileYear:  2025,
		RegionCode:   "110000",
	}})

	profile, err := service.GetUniversityProfile(context.Background(), 1, &year)

	require.NoError(t, err)
	require.Equal(t, 2025, profile.ProfileYear)
	require.Equal(t, "110000", profile.RegionCode)
}
