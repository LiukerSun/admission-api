package admission

import "context"

type DictionaryService interface {
	ListDictionaries(ctx context.Context) (*DictionaryResponse, error)
}

type dictionaryService struct {
	store DictionaryStore
}

func NewDictionaryService(store DictionaryStore) DictionaryService {
	return &dictionaryService{store: store}
}

func (s *dictionaryService) ListDictionaries(ctx context.Context) (*DictionaryResponse, error) {
	return s.store.ListDictionaries(ctx)
}
