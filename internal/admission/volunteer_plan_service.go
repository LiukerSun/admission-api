package admission

import (
	"encoding/json"
	"fmt"
	"os"
)

type VolunteerPlanService struct {
	plansFilePath string
}

func NewVolunteerPlanService(plansFilePath string) *VolunteerPlanService {
	return &VolunteerPlanService{
		plansFilePath: plansFilePath,
	}
}

func (s *VolunteerPlanService) GetPlans() (*VolunteerPlansResponse, error) {
	data, err := os.ReadFile(s.plansFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plans file: %w", err)
	}

	var response VolunteerPlansResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plans data: %w", err)
	}

	return &response, nil
}
