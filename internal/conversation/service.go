package conversation

import "context"

type Service interface {
	CreateConversation(ctx context.Context, title string, userID *int64) (*Conversation, error)
	GetConversation(ctx context.Context, id int64) (*Conversation, error)
	ListConversations(ctx context.Context, userID *int64) ([]*Conversation, error)
	ArchiveConversation(ctx context.Context, id int64) error
	DeleteConversation(ctx context.Context, id int64) error
	AddMessage(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*Message, error)
	ListMessages(ctx context.Context, conversationID int64) ([]*Message, error)
	Rollback(ctx context.Context, conversationID, messageID int64, inclusive bool) (deleted int, latest *int64, err error)
}

type service struct {
	store Store
}

func NewService(store Store) Service {
	return &service{store: store}
}

func (s *service) CreateConversation(ctx context.Context, title string, userID *int64) (*Conversation, error) {
	return s.store.CreateConversation(ctx, title, userID)
}

func (s *service) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	return s.store.GetConversation(ctx, id)
}

func (s *service) ListConversations(ctx context.Context, userID *int64) ([]*Conversation, error) {
	return s.store.ListConversations(ctx, userID, "active")
}

func (s *service) ArchiveConversation(ctx context.Context, id int64) error {
	return s.store.UpdateConversationStatus(ctx, id, "archived")
}

func (s *service) DeleteConversation(ctx context.Context, id int64) error {
	return s.store.UpdateConversationStatus(ctx, id, "deleted")
}

func (s *service) AddMessage(ctx context.Context, conversationID int64, role, content string, toolCalls, toolResults, widgets []byte) (*Message, error) {
	return s.store.AddMessage(ctx, conversationID, role, content, toolCalls, toolResults, widgets)
}

func (s *service) ListMessages(ctx context.Context, conversationID int64) ([]*Message, error) {
	return s.store.ListMessages(ctx, conversationID)
}

func (s *service) Rollback(ctx context.Context, conversationID, messageID int64, inclusive bool) (deleted int, latest *int64, err error) {
	return s.store.Rollback(ctx, conversationID, messageID, inclusive)
}
