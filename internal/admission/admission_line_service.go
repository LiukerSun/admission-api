package admission

import "context"

type AdmissionLineService interface { //nolint:revive
	ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error)
}

type admissionLineService struct {
	store AdmissionLineStore
}

func NewAdmissionLineService(store AdmissionLineStore) AdmissionLineService {
	return &admissionLineService{store: store}
}

func (s *admissionLineService) ListAdmissionLines(ctx context.Context, filter *AdmissionLineFilter) ([]AdmissionLineResponse, error) {
	return s.store.ListAdmissionLines(ctx, filter)
}
