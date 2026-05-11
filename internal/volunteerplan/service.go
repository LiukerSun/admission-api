package volunteerplan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"admission-api/internal/conversation"
)

type Service interface {
	GetDraft(ctx context.Context, userID, draftID int64) (*Draft, error)
	ListDraftsByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error)
	AdoptDraft(ctx context.Context, userID, draftID int64, title string) (*UserVolunteerPlan, error)
	ListPlans(ctx context.Context, userID int64) ([]*UserVolunteerPlan, error)
	GetPlan(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error)
}

type service struct {
	drafts        DraftStore
	plans         PlanStore
	conversations conversation.Service
}

func NewService(drafts DraftStore, plans PlanStore, conversations conversation.Service) Service {
	return &service{drafts: drafts, plans: plans, conversations: conversations}
}

func (s *service) GetDraft(ctx context.Context, userID, draftID int64) (*Draft, error) {
	return s.drafts.GetByID(ctx, userID, draftID)
}

func (s *service) ListDraftsByConversation(ctx context.Context, userID, conversationID int64) ([]*Draft, error) {
	return s.drafts.ListByConversation(ctx, userID, conversationID)
}

func (s *service) ListPlans(ctx context.Context, userID int64) ([]*UserVolunteerPlan, error) {
	return s.plans.ListByUser(ctx, userID)
}

func (s *service) GetPlan(ctx context.Context, userID, planID int64) (*UserVolunteerPlan, error) {
	return s.plans.GetByID(ctx, userID, planID)
}

func (s *service) AdoptDraft(ctx context.Context, userID, draftID int64, title string) (*UserVolunteerPlan, error) {
	draft, err := s.drafts.GetByID(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}
	if draft.Status != DraftStatusReady {
		return nil, ErrDraftNotReady
	}
	if len(draft.PlanJSON) == 0 || string(draft.PlanJSON) == "null" {
		return nil, fmt.Errorf("draft plan_json is empty")
	}

	normalizedTitle := strings.TrimSpace(title)
	if normalizedTitle == "" {
		normalizedTitle = "志愿方案"
	}

	if !json.Valid(draft.PlanJSON) {
		return nil, fmt.Errorf("draft plan_json is invalid json")
	}

	plan, err := s.plans.CreateFromDraft(ctx, userID, draftID, normalizedTitle, draft.PlanJSON)
	if err != nil {
		return nil, err
	}

	_ = s.drafts.MarkAdopted(ctx, userID, draftID)
	_ = s.conversations.ArchiveConversation(ctx, draft.ConversationID)

	return plan, nil
}
