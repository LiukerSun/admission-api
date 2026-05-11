package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// fakeStore is a minimal in-memory implementation of Store used to exercise
// the service-layer ownership rules. Only the methods used by Service tests
// are implemented; others panic so we notice if Service starts using them.
type fakeStore struct {
	conversations map[int64]*Conversation
	nextID        int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{conversations: map[int64]*Conversation{}, nextID: 1}
}

func (f *fakeStore) CreateConversation(ctx context.Context, userID int64, title, model string) (*Conversation, error) {
	c := &Conversation{
		ID:        f.nextID,
		UserID:    userID,
		Title:     title,
		ModelName: model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.nextID++
	f.conversations[c.ID] = c
	return c, nil
}
func (f *fakeStore) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	if c, ok := f.conversations[id]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}
func (f *fakeStore) ListConversationsByUser(ctx context.Context, userID int64, page, pageSize int) ([]*Conversation, int64, error) {
	out := []*Conversation{}
	for _, c := range f.conversations {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, int64(len(out)), nil
}
func (f *fakeStore) DeleteConversation(ctx context.Context, id int64) error {
	delete(f.conversations, id)
	return nil
}
func (f *fakeStore) UpdateConversationTitle(ctx context.Context, id int64, title string) error {
	if c, ok := f.conversations[id]; ok {
		c.Title = title
	}
	return nil
}
func (f *fakeStore) TouchConversation(ctx context.Context, id int64) error { return nil }
func (f *fakeStore) CreateMessage(context.Context, *CreateMessageInput) (*Message, error) {
	panic("not used in service tests")
}
func (f *fakeStore) GetMessage(context.Context, int64) (*Message, error) {
	panic("not used")
}
func (f *fakeStore) ListMessages(context.Context, int64) ([]*Message, error) { panic("not used") }
func (f *fakeStore) ListRecentMessages(context.Context, int64, int) ([]*Message, error) {
	panic("not used")
}
func (f *fakeStore) GetLastMessage(context.Context, int64) (*Message, error) { panic("not used") }
func (f *fakeStore) UpdateMessageWidgets(context.Context, int64, json.RawMessage) error {
	panic("not used")
}
func (f *fakeStore) DeleteMessagesFrom(context.Context, int64, int64, bool) (int64, int64, error) {
	panic("not used")
}

func TestServiceGetForUser_CrossUserReturns404(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, 1, "mine", "")
	_, err := svc.GetForUser(ctx, 2, owned.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user access should collapse to ErrNotFound, got %v", err)
	}
}

func TestServiceDeleteForUser_OwnershipEnforced(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	ctx := context.Background()

	owned, _ := svc.Create(ctx, 1, "mine", "")
	if err := svc.DeleteForUser(ctx, 2, owned.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-owner delete, got %v", err)
	}
	if _, ok := store.conversations[owned.ID]; !ok {
		t.Fatalf("conversation should still exist after rejected delete")
	}
	if err := svc.DeleteForUser(ctx, 1, owned.ID); err != nil {
		t.Fatalf("owner delete failed: %v", err)
	}
	if _, ok := store.conversations[owned.ID]; ok {
		t.Fatalf("conversation should be gone")
	}
}

func TestServiceRenameForUser_RejectsEmpty(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store)
	ctx := context.Background()
	owned, _ := svc.Create(ctx, 1, "mine", "")
	if err := svc.RenameForUser(ctx, 1, owned.ID, "   "); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for blank title, got %v", err)
	}
}
