package permission

import (
	"context"
	"errors"
)

// ErrLegacyStageTemplate requires the administrator to replace stage-based template ranges before assignment.
var ErrLegacyStageTemplate = errors.New("legacy stage template needs manual review")

// LegacyStageItem identifies a template, assigned snapshot or personal rule requiring review.
type LegacyStageItem struct {
	RoleName       string `json:"roleName,omitempty"`
	MajorName      string `json:"majorName,omitempty"`
	ClassName      string `json:"className,omitempty"`
	Source         string `json:"source"`
	AccountID      string `json:"accountId,omitempty"`
	AccountName    string `json:"accountName,omitempty"`
	DepartmentName string `json:"departmentName,omitempty"`
	Action         string `json:"action"`
	TypeName       string `json:"typeName,omitempty"`
	Stage          int    `json:"stage"`
}

// LegacyStages lists the remaining references for an administrator's manual review.
func (s *Service) LegacyStages(ctx context.Context, page, size int) ([]LegacyStageItem, int, error) {
	repo, ok := s.repository.(interface {
		LegacyStages(context.Context, int, int) ([]LegacyStageItem, int, error)
	})
	if !ok {
		return nil, 0, errors.New("legacy stage inventory unavailable")
	}
	return repo.LegacyStages(ctx, page, size)
}

// Clearing a legacy restriction during manual replacement cannot create an unrestricted grant.
func widensLegacyRange(previous []OperationRange, replacement Rule) bool {
	if !replacement.Enabled || replacement.Mode == "DISABLE" || len(replacement.Ranges) > 0 {
		return false
	}
	for _, scope := range previous {
		if scope.GradeStage != 0 {
			return true
		}
	}
	return false
}
