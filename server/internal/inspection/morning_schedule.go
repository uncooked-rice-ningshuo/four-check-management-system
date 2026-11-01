package inspection

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

var ErrMorningScheduleVersionConflict = errors.New("morning schedule version conflict")

type MorningSchedule struct {
	SemesterID   string    `json:"semesterId"`
	WeekID       string    `json:"weekId"`
	StartDate    string    `json:"startDate"`
	EndDate      string    `json:"endDate"`
	PlannedDates []string  `json:"plannedDates"`
	Version      int       `json:"version"`
	UpdatedBy    string    `json:"updatedBy,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty"`
	IsDefault    bool      `json:"isDefault"`
	CanEdit      bool      `json:"canEdit"`
}

type MorningScheduleRepository interface {
	LoadMorningSchedule(context.Context, string) (MorningSchedule, permission.Scope, error)
	SaveMorningSchedule(context.Context, string, int, []string, string, audit.Metadata) (MorningSchedule, error)
}

type MorningScheduleService struct {
	repository  MorningScheduleRepository
	permissions manualPermissions
}

func NewMorningScheduleService(repository MorningScheduleRepository, permissions manualPermissions) *MorningScheduleService {
	return &MorningScheduleService{repository: repository, permissions: permissions}
}

func (s *MorningScheduleService) Get(ctx context.Context, actor ManualActor, weekID string) (MorningSchedule, error) {
	weekID = strings.TrimSpace(weekID)
	if _, err := uuid.Parse(weekID); err != nil || strings.TrimSpace(actor.ID) == "" {
		return MorningSchedule{}, ErrInspectionRequest
	}
	schedule, scope, err := s.repository.LoadMorningSchedule(ctx, weekID)
	if err != nil {
		return MorningSchedule{}, err
	}
	canView, canEdit, err := s.access(ctx, actor, scope)
	if err != nil {
		return MorningSchedule{}, err
	}
	if !canView {
		return MorningSchedule{}, ErrScopeMismatch
	}
	schedule.CanEdit = canEdit
	return schedule, nil
}

func (s *MorningScheduleService) Save(ctx context.Context, actor ManualActor, weekID string, version int, dates []string, metadata audit.Metadata) (MorningSchedule, error) {
	weekID = strings.TrimSpace(weekID)
	if _, err := uuid.Parse(weekID); err != nil || strings.TrimSpace(actor.ID) == "" || version < 0 {
		return MorningSchedule{}, ErrInspectionRequest
	}
	schedule, scope, err := s.repository.LoadMorningSchedule(ctx, weekID)
	if err != nil {
		return MorningSchedule{}, err
	}
	_, canEdit, err := s.access(ctx, actor, scope)
	if err != nil {
		return MorningSchedule{}, err
	}
	if !canEdit {
		return MorningSchedule{}, ErrScopeMismatch
	}
	normalized, err := normalizeMorningPlannedDates(dates, schedule.StartDate, schedule.EndDate)
	if err != nil {
		return MorningSchedule{}, err
	}
	metadata.ActorID = actor.ID
	result, err := s.repository.SaveMorningSchedule(ctx, weekID, version, normalized, actor.ID, metadata)
	if errors.Is(err, ErrMorningScheduleVersionConflict) {
		return MorningSchedule{}, ErrRecordVersionConflict
	}
	if err != nil {
		return MorningSchedule{}, err
	}
	result.CanEdit = true
	return result, nil
}

func (s *MorningScheduleService) access(ctx context.Context, actor ManualActor, scope permission.Scope) (bool, bool, error) {
	if actor.IsSuperAdmin {
		return true, true, nil
	}
	permissions, err := s.permissions.Effective(ctx, actor.ID)
	if err != nil {
		return false, false, err
	}
	canEdit := permissions.Allowed(permission.ActionMorningScheduleManage, scope)
	canView := canEdit || permissions.Allowed(permission.ActionInspectionView, scope) || permissions.Allowed(permission.ActionInspectionEnter, scope)
	return canView, canEdit, nil
}

func normalizeMorningPlannedDates(dates []string, startDate, endDate string) ([]string, error) {
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || endErr != nil || start.After(end) || len(dates) > 7 {
		return nil, ErrInspectionRequest
	}
	seen := make(map[string]bool, len(dates))
	result := make([]string, 0, len(dates))
	for _, value := range dates {
		date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
		if err != nil || date.Format("2006-01-02") != value || date.Before(start) || date.After(end) || seen[value] {
			return nil, ErrInspectionRequest
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

// DefaultMorningPlannedDates creates the Monday-to-Friday plan inside a calendar week.
func DefaultMorningPlannedDates(startDate, endDate string) []string {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return []string{}
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil || start.After(end) || end.Sub(start) > 14*24*time.Hour {
		return []string{}
	}
	result := []string{}
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if date.Weekday() >= time.Monday && date.Weekday() <= time.Friday {
			result = append(result, date.Format("2006-01-02"))
		}
	}
	return result
}
