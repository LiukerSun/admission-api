package candidate

import (
	"context"
	"errors"

	"admission-api/internal/platform/web"
	"admission-api/internal/user"

	"github.com/jackc/pgx/v5"
)

// IntentionService defines intention business operations.
type IntentionService interface {
	GetIntentions(ctx context.Context, userID int64, profileID int64) (*IntentionGroupResponse, error)
	SaveIntentions(ctx context.Context, userID int64, profileID int64, intentionType string, req *SaveIntentionsRequest) error
	RemoveIntention(ctx context.Context, userID int64, intentionID int64) error
	ClearIntentions(ctx context.Context, userID int64, profileID int64, intentionType string) error
}

type intentionService struct {
	store        IntentionStore
	profileStore ProfileStore
	bindingStore user.BindingStore
	activityLog  ActivityLogService
}

// NewIntentionService creates a new intention service.
func NewIntentionService(
	store IntentionStore,
	profileStore ProfileStore,
	bindingStore user.BindingStore,
	activityLog ActivityLogService,
) IntentionService {
	return &intentionService{
		store:        store,
		profileStore: profileStore,
		bindingStore: bindingStore,
		activityLog:  activityLog,
	}
}

func (s *intentionService) GetIntentions(ctx context.Context, userID int64, profileID int64) (*IntentionGroupResponse, error) {
	if ok, err := s.canAccessProfile(ctx, userID, profileID); err != nil {
		return nil, err
	} else if !ok {
		return nil, web.NewError(web.ErrCodeForbidden, "无权访问该档案")
	}

	intentions, err := s.store.ListByProfile(ctx, profileID, "")
	if err != nil {
		return nil, err
	}

	resp := &IntentionGroupResponse{}
	for _, i := range intentions {
		switch i.IntentionType {
		case "province":
			resp.Province = append(resp.Province, i)
		case "school":
			resp.School = append(resp.School, i)
		case "major":
			resp.Major = append(resp.Major, i)
		case "school_major":
			resp.SchoolMajor = append(resp.SchoolMajor, i)
		}
	}
	return resp, nil
}

func (s *intentionService) SaveIntentions(ctx context.Context, userID int64, profileID int64, intentionType string, req *SaveIntentionsRequest) error {
	if !isValidIntentionType(intentionType) {
		return web.NewError(web.ErrCodeBadRequest, "无效的意向类型")
	}

	if ok, err := s.isOwner(ctx, userID, profileID); err != nil {
		return err
	} else if !ok {
		return web.NewError(web.ErrCodeForbidden, "仅档案所有者可修改意向")
	}

	inputs := make([]*CreateIntentionInput, len(req.Items))
	for i, item := range req.Items {
		var notes *string
		if item.Notes != "" {
			notes = &item.Notes
		}
		var targetName *string
		if item.TargetName != "" {
			targetName = &item.TargetName
		}
		inputs[i] = &CreateIntentionInput{
			ProfileID:     profileID,
			IntentionType: intentionType,
			TargetID:      item.TargetID,
			TargetName:    targetName,
			Priority:      item.Priority,
			Notes:         notes,
		}
	}

	if err := s.store.ReplaceByType(ctx, profileID, intentionType, inputs); err != nil {
		return err
	}

	if s.activityLog != nil {
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "intention_add",
			Metadata: map[string]any{
				"profile_id":     profileID,
				"intention_type": intentionType,
				"count":          len(req.Items),
			},
		})
	}

	return nil
}

func (s *intentionService) RemoveIntention(ctx context.Context, userID int64, intentionID int64) error {
	intention, err := s.store.GetByID(ctx, intentionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.NewError(web.ErrCodeNotFound, "意向不存在")
		}
		return err
	}

	if ok, err := s.isOwner(ctx, userID, intention.ProfileID); err != nil {
		return err
	} else if !ok {
		return web.NewError(web.ErrCodeForbidden, "仅档案所有者可删除意向")
	}

	if err := s.store.DeleteByID(ctx, intention.ProfileID, intentionID); err != nil {
		return err
	}

	if s.activityLog != nil {
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "intention_remove",
			TargetType:   "intention",
			TargetID:     intentionID,
			Metadata: map[string]any{
				"profile_id":     intention.ProfileID,
				"intention_type": intention.IntentionType,
				"target_id":      intention.TargetID,
			},
		})
	}

	return nil
}

func (s *intentionService) ClearIntentions(ctx context.Context, userID int64, profileID int64, intentionType string) error {
	if !isValidIntentionType(intentionType) {
		return web.NewError(web.ErrCodeBadRequest, "无效的意向类型")
	}

	if ok, err := s.isOwner(ctx, userID, profileID); err != nil {
		return err
	} else if !ok {
		return web.NewError(web.ErrCodeForbidden, "仅档案所有者可清空意向")
	}

	if err := s.store.DeleteByType(ctx, profileID, intentionType); err != nil {
		return err
	}

	if s.activityLog != nil {
		_ = s.activityLog.LogActivity(ctx, CreateActivityInput{
			UserID:       userID,
			ActivityType: "intention_remove",
			Metadata: map[string]any{
				"profile_id":     profileID,
				"intention_type": intentionType,
				"cleared":        true,
			},
		})
	}

	return nil
}

func (s *intentionService) canAccessProfile(ctx context.Context, userID, profileID int64) (bool, error) {
	ownerID, err := s.profileStore.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return false, err
	}
	if userID == ownerID {
		return true, nil
	}
	// Caller might be a student bound to owner-as-parent.
	if b, err := s.bindingStore.GetBindingByStudent(ctx, userID); err == nil {
		if b.ParentID == ownerID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	// Caller might be a parent whose bound student owns the profile.
	if b, err := s.bindingStore.GetBindingByStudent(ctx, ownerID); err == nil {
		if b.ParentID == userID {
			return true, nil
		}
	} else if !errors.Is(err, user.ErrBindingNotFound) {
		return false, err
	}
	return false, nil
}

func (s *intentionService) isOwner(ctx context.Context, userID, profileID int64) (bool, error) {
	ownerID, err := s.profileStore.GetOwnerUserID(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, web.NewError(web.ErrCodeNotFound, "档案不存在")
		}
		return false, err
	}
	return userID == ownerID, nil
}
