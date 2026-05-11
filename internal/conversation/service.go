package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Service exposes user-facing conversation operations with ownership checks.
type Service interface {
	Create(ctx context.Context, userID int64, title, modelName string) (*Conversation, error)
	GetForUser(ctx context.Context, userID, conversationID int64) (*Conversation, error)
	ListForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Conversation, int64, error)
	DeleteForUser(ctx context.Context, userID, conversationID int64) error
	RenameForUser(ctx context.Context, userID, conversationID int64, title string) error

	// Direct accessors used by the ai package — callers must perform ownership
	// checks themselves via GetForUser before invoking these.
	Store() Store
}

type service struct {
	store Store
}

// NewService creates a conversation service backed by the given store.
func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) Store() Store {
	return s.store
}

func (s *service) Create(ctx context.Context, userID int64, title, modelName string) (*Conversation, error) {
	if userID <= 0 {
		return nil, ErrInvalidArgument
	}
	title = strings.TrimSpace(title)
	if len(title) > 200 {
		title = title[:200]
	}
	return s.store.CreateConversation(ctx, userID, title, modelName)
}

func (s *service) GetForUser(ctx context.Context, userID, conversationID int64) (*Conversation, error) {
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.UserID != userID {
		return nil, ErrNotFound
	}
	return conv, nil
}

func (s *service) ListForUser(ctx context.Context, userID int64, page, pageSize int) ([]*Conversation, int64, error) {
	return s.store.ListConversationsByUser(ctx, userID, page, pageSize)
}

func (s *service) DeleteForUser(ctx context.Context, userID, conversationID int64) error {
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.UserID != userID {
		return ErrNotFound
	}
	return s.store.DeleteConversation(ctx, conversationID)
}

func (s *service) RenameForUser(ctx context.Context, userID, conversationID int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return ErrInvalidArgument
	}
	if len(title) > 200 {
		title = title[:200]
	}
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.UserID != userID {
		return ErrNotFound
	}
	return s.store.UpdateConversationTitle(ctx, conversationID, title)
}

// IsNotFound is a small helper so callers don't need to import errors.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// EmptyJSONArray returns a JSON `[]` raw message, useful when callers want a
// non-nil value without manually marshalling.
func EmptyJSONArray() json.RawMessage {
	return json.RawMessage("[]")
}
