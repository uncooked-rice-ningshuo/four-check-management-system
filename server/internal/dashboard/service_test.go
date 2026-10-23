package dashboard

import (
	"context"
	"testing"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

type academicStub struct {
	week     *academic.Week
	calendar academic.Calendar
}

func (s academicStub) CurrentWeek(context.Context, time.Time) (*academic.Week, error) {
	return s.week, nil
}
func (s academicStub) List(context.Context) (academic.Calendar, error) { return s.calendar, nil }

type organizationStub struct {
	catalogs organization.Catalogs
	scopes   []organization.DepartmentInspectionScope
}

func (s organizationStub) ListCatalogs(context.Context, bool) (organization.Catalogs, error) {
	return s.catalogs, nil
}
func (s organizationStub) ListScopes(context.Context) ([]organization.DepartmentInspectionScope, error) {
	return s.scopes, nil
}

type permissionStub struct{ result permission.EffectiveResult }

func (s permissionStub) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return s.result, nil
}

type repositoryStub struct {
	query         AggregateQuery
	personalQuery PersonalTodoQuery
	counts        []ScopeCounts
	personal      MyTodos
}

func (s *repositoryStub) PersonalTodos(_ context.Context, query PersonalTodoQuery) (MyTodos, error) {
	s.personalQuery = query
	return s.personal, nil
}

func (s *repositoryStub) Aggregate(_ context.Context, query AggregateQuery) ([]ScopeCounts, error) {
	s.query = query
	return s.counts, nil
}

func TestWorkbenchCountsPersonalTodosButShowsDepartmentOverall(t *testing.T) {
	repository := &repositoryStub{personal: MyTodos{Total: 2, Draft: 1, Returned: 1}, counts: []ScopeCounts{{
		DepartmentID: "study", InspectionTypeID: "attendance", Total: 9, Draft: 2,
		PendingReview: 3, Returned: 1, Confirmed: 3,
		BatchTotal: 3, BatchPending: 1, BatchReturned: 1, BatchConfirmed: 1,
	}}}
	service := NewService(repository, academicStub{
		week: &academic.Week{ID: "week-1", SemesterID: "semester-1", WeekNo: 2},
		calendar: academic.Calendar{
			AcademicYears: []academic.AcademicYear{{ID: "year-1", Name: "2026-2027 学年"}},
			Semesters:     []academic.Semester{{ID: "semester-1", AcademicYearID: "year-1", Name: "第一学期"}},
		},
	}, organizationStub{
		catalogs: organization.Catalogs{
			Departments:     []organization.Department{{ID: "study", Name: "学习部", Enabled: true}},
			InspectionTypes: []organization.InspectionType{{ID: "attendance", Code: "ATTENDANCE_RATE", Name: "到课率", Enabled: true}},
		},
		scopes: []organization.DepartmentInspectionScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanView: true, CanEnter: true, Enabled: true}},
	}, permissionStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{
		{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "study", InspectionTypeID: "attendance"}},
		{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "study", InspectionTypeID: "attendance"}},
	}}})

	result, err := service.Workbench(context.Background(), Actor{ID: "member-1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.MyTodos.Total != 2 || result.MyTodos.Draft != 1 || result.MyTodos.Returned != 1 {
		t.Fatalf("personal todos = %#v", result.MyTodos)
	}
	if len(result.DepartmentStatuses) != 1 || result.DepartmentStatuses[0].Total != 9 || result.DepartmentStatuses[0].Status != StatusReturned ||
		result.DepartmentStatuses[0].BatchTotal != 3 || result.DepartmentStatuses[0].BatchReturned != 1 {
		t.Fatalf("department statuses = %#v", result.DepartmentStatuses)
	}
	if repository.query.AccountID != "member-1" || len(repository.query.Scopes) != 1 {
		t.Fatalf("aggregate query = %#v", repository.query)
	}
}

func TestWorkbenchDoesNotExposePartialDepartmentScopeAsOverall(t *testing.T) {
	repository := &repositoryStub{personal: MyTodos{Total: 1, Draft: 1}}
	service := NewService(repository,
		academicStub{week: &academic.Week{ID: "week-1", SemesterID: "semester-1"}},
		organizationStub{
			catalogs: organization.Catalogs{
				Departments:     []organization.Department{{ID: "study", Name: "学习部", Enabled: true}},
				InspectionTypes: []organization.InspectionType{{ID: "attendance", Name: "到课率", Enabled: true}},
			},
			scopes: []organization.DepartmentInspectionScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanView: true, Enabled: true}},
		},
		permissionStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{
			{Action: permission.ActionInspectionView, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "study", InspectionTypeID: "attendance"}},
			{Action: permission.ActionInspectionView, Effect: permission.EffectDeny, Scope: permission.Scope{DepartmentID: "study", InspectionTypeID: "attendance", ClassID: "class-1"}},
		}}},
	)

	result, err := service.Workbench(context.Background(), Actor{ID: "member-1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DepartmentStatuses) != 0 || len(repository.query.Scopes) != 0 {
		t.Fatalf("partial range must not be presented as department overall: %#v / %#v", result, repository.query)
	}
	if result.MyTodos.Total != 1 || len(repository.personalQuery.Allowed) != 1 || len(repository.personalQuery.Denied) != 1 {
		t.Fatalf("partial range personal todos = %#v / %#v", result.MyTodos, repository.personalQuery)
	}
}

func TestWorkbenchSuperAdminSeesAllConfiguredScopes(t *testing.T) {
	repository := &repositoryStub{counts: []ScopeCounts{{DepartmentID: "study", InspectionTypeID: "attendance"}}}
	service := NewService(repository,
		academicStub{week: &academic.Week{ID: "week-1", SemesterID: "semester-1"}},
		organizationStub{
			catalogs: organization.Catalogs{
				Departments:     []organization.Department{{ID: "study", Name: "学习部", Enabled: true}},
				InspectionTypes: []organization.InspectionType{{ID: "attendance", Name: "到课率", Enabled: true}},
			},
			scopes: []organization.DepartmentInspectionScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanView: true, Enabled: true}},
		}, permissionStub{},
	)

	result, err := service.Workbench(context.Background(), Actor{ID: "root", IsSuperAdmin: true}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DepartmentStatuses) != 1 || len(repository.query.Scopes) != 1 {
		t.Fatalf("super admin workbench = %#v / %#v", result, repository.query)
	}
}

func TestDepartmentStatusDerivation(t *testing.T) {
	tests := []struct {
		name   string
		counts ScopeCounts
		want   string
	}{
		{"empty", ScopeCounts{}, StatusNotEntered},
		{"draft", ScopeCounts{Total: 2, Draft: 2}, StatusEntering},
		{"pending", ScopeCounts{Total: 2, PendingReview: 2}, StatusPendingReview},
		{"returned", ScopeCounts{Total: 2, Returned: 1, Draft: 1}, StatusReturned},
		{"completed", ScopeCounts{Total: 2, Confirmed: 1, Locked: 1}, StatusCompleted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deriveStatus(test.counts); got != test.want {
				t.Fatalf("deriveStatus() = %s, want %s", got, test.want)
			}
		})
	}
}
