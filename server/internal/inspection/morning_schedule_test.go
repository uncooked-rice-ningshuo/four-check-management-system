package inspection

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
)

type morningScheduleRepositoryFixture struct {
	schedule     MorningSchedule
	scope        permission.Scope
	savedVersion int
	savedDates   []string
	savedBy      string
	saveErr      error
}

func (r *morningScheduleRepositoryFixture) LoadMorningSchedule(context.Context, string) (MorningSchedule, permission.Scope, error) {
	return r.schedule, r.scope, nil
}

func (r *morningScheduleRepositoryFixture) SaveMorningSchedule(_ context.Context, _ string, expectedVersion int, dates []string, actorID string, _ audit.Metadata) (MorningSchedule, error) {
	r.savedVersion, r.savedDates, r.savedBy = expectedVersion, append([]string(nil), dates...), actorID
	if r.saveErr != nil {
		return MorningSchedule{}, r.saveErr
	}
	r.schedule.Version = expectedVersion + 1
	r.schedule.PlannedDates = append([]string(nil), dates...)
	return r.schedule, nil
}

type morningSchedulePermissionsFixture struct{ result permission.EffectiveResult }

func (p morningSchedulePermissionsFixture) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return p.result, nil
}

func TestDefaultMorningPlanUsesWeekdaysOnly(t *testing.T) {
	want := []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-07"}
	if got := DefaultMorningPlannedDates("2026-09-01", "2026-09-07"); !reflect.DeepEqual(got, want) {
		t.Fatalf("default plan = %v, want %v", got, want)
	}
}

func TestMorningScheduleSaveRequiresScopedPermissionAndNormalizesDates(t *testing.T) {
	weekID := "60000000-0000-4000-8000-000000000003"
	repository := &morningScheduleRepositoryFixture{
		schedule: MorningSchedule{WeekID: weekID, StartDate: "2026-09-01", EndDate: "2026-09-07", Version: 0, IsDefault: true},
		scope:    permission.Scope{WeekID: weekID, DepartmentID: "60000000-0000-4000-8000-000000000004", InspectionTypeID: "60000000-0000-4000-8000-000000000005"},
	}
	actor := ManualActor{ID: "60000000-0000-4000-8000-000000000006"}
	service := NewMorningScheduleService(repository, morningSchedulePermissionsFixture{})
	if _, err := service.Save(context.Background(), actor, weekID, 0, []string{"2026-09-03"}, audit.Metadata{}); !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("save without schedule permission = %v", err)
	}
	permissions := morningSchedulePermissionsFixture{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{{
		Action: permission.ActionMorningScheduleManage, Effect: permission.EffectAllow, Scope: permission.Scope{},
	}}}}
	service = NewMorningScheduleService(repository, permissions)
	if _, err := service.Save(context.Background(), actor, weekID, 0, []string{"2026-09-08"}, audit.Metadata{}); !errors.Is(err, ErrInspectionRequest) {
		t.Fatalf("date outside week = %v", err)
	}
	if repository.savedBy != "" {
		t.Fatal("invalid plan reached persistence")
	}
	saved, err := service.Save(context.Background(), actor, weekID, 0, []string{"2026-09-04", "2026-09-02"}, audit.Metadata{})
	want := []string{"2026-09-02", "2026-09-04"}
	if err != nil || !reflect.DeepEqual(saved.PlannedDates, want) || !reflect.DeepEqual(repository.savedDates, want) || repository.savedVersion != 0 || repository.savedBy != actor.ID {
		t.Fatalf("saved schedule=%+v repository=%+v err=%v", saved, repository, err)
	}
}

func TestMorningScheduleManagerCanReadAndEditPlanWithoutInspectionAccess(t *testing.T) {
	weekID := "60000000-0000-4000-8000-000000000003"
	repository := &morningScheduleRepositoryFixture{
		schedule: MorningSchedule{WeekID: weekID, StartDate: "2026-09-01", EndDate: "2026-09-07", Version: 0, IsDefault: true},
		scope:    permission.Scope{WeekID: weekID},
	}
	permissions := morningSchedulePermissionsFixture{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{{
		Action: permission.ActionMorningScheduleManage, Effect: permission.EffectAllow, Scope: permission.Scope{},
	}}}}
	service := NewMorningScheduleService(repository, permissions)
	schedule, err := service.Get(context.Background(), ManualActor{ID: "60000000-0000-4000-8000-000000000006"}, weekID)
	if err != nil || !schedule.CanEdit || !schedule.IsDefault {
		t.Fatalf("schedule manager read without inspection permission: schedule=%+v err=%v", schedule, err)
	}
}

func TestMorningScheduleSaveMapsConcurrentVersionConflict(t *testing.T) {
	weekID := "60000000-0000-4000-8000-000000000003"
	repository := &morningScheduleRepositoryFixture{
		schedule: MorningSchedule{WeekID: weekID, StartDate: "2026-09-01", EndDate: "2026-09-07", Version: 2},
		scope:    permission.Scope{WeekID: weekID},
		saveErr:  ErrMorningScheduleVersionConflict,
	}
	permissions := morningSchedulePermissionsFixture{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{{
		Action: permission.ActionMorningScheduleManage, Effect: permission.EffectAllow, Scope: permission.Scope{},
	}}}}
	service := NewMorningScheduleService(repository, permissions)
	_, err := service.Save(context.Background(), ManualActor{ID: "60000000-0000-4000-8000-000000000006"}, weekID, 2, []string{"2026-09-02"}, audit.Metadata{})
	if !errors.Is(err, ErrRecordVersionConflict) {
		t.Fatalf("version conflict = %v", err)
	}
}
