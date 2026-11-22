package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
)

type DisciplineReview struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Version  int    `json:"version"`
}

func (s *Service) ReviewDisciplineCase(ctx context.Context, id, actor string, input DisciplineReview, meta audit.Metadata) error {
	effective, err := s.permissions.Effective(ctx, actor)
	if err != nil {
		return err
	}
	if effective.SystemRole != permission.SystemRoleSuperAdmin && !effective.HasAnyAllowed(permission.ActionDisciplineConfirm) {
		return ErrForbidden
	}
	repo, ok := s.repository.(interface {
		ReviewDisciplineCase(context.Context, string, DisciplineReview, permission.EffectiveResult, audit.Metadata) error
	})
	if !ok {
		return errors.New("discipline case repository unavailable")
	}
	meta.ActorID = actor
	return repo.ReviewDisciplineCase(ctx, id, input, effective, meta)
}

type ReviewQueueFilter struct {
	Status   string `json:"status"`
	Student  string `json:"student"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

func (s *Service) DisciplineReviewQueue(ctx context.Context, actor string, f ReviewQueueFilter) (json.RawMessage, error) {
	if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 || (f.Status != "" && f.Status != "PENDING" && f.Status != "DEFERRED") {
		return nil, ErrInvalidReportRequest
	}
	effective, err := s.permissions.Effective(ctx, actor)
	if err != nil {
		return nil, err
	}
	if effective.SystemRole != permission.SystemRoleSuperAdmin && !effective.HasAnyAllowed(permission.ActionDisciplineConfirm) && !effective.HasAnyAllowed("DISCIPLINE_CASE_VIEW") {
		return nil, ErrForbidden
	}
	repo, ok := s.repository.(interface {
		DisciplineReviewQueue(context.Context, ReviewQueueFilter, permission.EffectiveResult) (json.RawMessage, error)
	})
	if !ok {
		return nil, errors.New("discipline queue unavailable")
	}
	return repo.DisciplineReviewQueue(ctx, f, effective)
}
