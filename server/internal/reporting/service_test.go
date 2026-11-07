package reporting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
)

type reportingRepositoryStub struct {
	periods       []PeriodOption
	periodsErr    error
	summary       Summary
	summaryErr    error
	periodCalls   int
	queryCalls    int
	permissions   []permission.EffectivePermission
	semesterID    string
	weekID        string
	recordActor   string
	recordQuery   RecordsQuery
	records       []inspection.InspectionRecord
	recordTotal   int
	filterOptions ReportVersionRecordFilterOptions
}

func (r *reportingRepositoryStub) ReportRecordFilterOptions(_ context.Context, _ RecordsQuery, _ []permission.EffectivePermission) (ReportVersionRecordFilterOptions, error) {
	if r.filterOptions.StudentNumbers == nil {
		return ReportVersionRecordFilterOptions{StudentNumbers: []string{}, StudentNames: []string{}, ClassNames: []string{}, Grades: []string{}}, nil
	}
	return r.filterOptions, nil
}

func (r *reportingRepositoryStub) ListReportPeriods(_ context.Context, permissions []permission.EffectivePermission) ([]PeriodOption, error) {
	r.periodCalls++
	r.permissions = permissions
	return r.periods, r.periodsErr
}

func (r *reportingRepositoryStub) ReportSummary(_ context.Context, semesterID, weekID string, permissions []permission.EffectivePermission) (Summary, error) {
	r.queryCalls++
	r.permissions = permissions
	r.semesterID = semesterID
	r.weekID = weekID
	return r.summary, r.summaryErr
}

func (r *reportingRepositoryStub) ReportRecords(_ context.Context, accountID string, query RecordsQuery, permissions []permission.EffectivePermission) ([]inspection.InspectionRecord, int, error) {
	r.recordActor, r.recordQuery, r.permissions = accountID, query, permissions
	return r.records, r.recordTotal, nil
}

func (r *reportingRepositoryStub) ReportIndicatorInputs(context.Context, string, string) (ReportMetricInputs, error) {
	return ReportMetricInputs{WeekMultiplier: 1, MorningPlannedDates: []string{}, Classes: []ReportClassSizeSnapshot{}}, nil
}

type reportingPermissionsStub struct {
	result permission.EffectiveResult
	err    error
}

func (p reportingPermissionsStub) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return p.result, p.err
}

func TestSummaryRequiresValidPeriodAndDepartmentWideGrant(t *testing.T) {
	repository := &reportingRepositoryStub{}
	permissions := reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{{
		Action: permission.ActionReportView,
		Effect: permission.EffectAllow,
		Scope:  permission.Scope{ClassID: "class-1"},
	}}}}
	service := NewService(repository, permissions)

	if _, err := service.Summary(context.Background(), "actor", "bad-semester", "bad-week"); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("invalid period error = %v", err)
	}
	if _, err := service.Summary(context.Background(), "actor", "a0000000-0000-0000-0000-000000000003", "a0000000-0000-0000-0000-000000000004"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("class-scoped grant error = %v", err)
	}
	if repository.queryCalls != 0 {
		t.Fatalf("repository called %d times without a department-wide grant", repository.queryCalls)
	}
}

func TestSummaryAggregatesVisibleRowsAndPassesEffectiveScopes(t *testing.T) {
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: "dept-1"}}
	repository := &reportingRepositoryStub{summary: Summary{Rows: []SummaryRow{
		{SubmissionCount: 2, DraftBatchCount: 1, ConfirmedBatchCount: 1, ConfirmedRecordCount: 3},
		{SubmissionCount: 1, PendingReviewBatchCount: 1, PendingReviewOrReturnedRecordCount: 2},
	}}}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})

	result, err := service.Summary(context.Background(), "actor", "a0000000-0000-0000-0000-000000000003", "a0000000-0000-0000-0000-000000000004")
	if err != nil {
		t.Fatal(err)
	}
	if repository.queryCalls != 1 || len(repository.permissions) != 1 || repository.permissions[0].Scope.DepartmentID != "dept-1" {
		t.Fatalf("repository calls/permissions = %d/%+v", repository.queryCalls, repository.permissions)
	}
	if result.Totals.SubmissionCount != 3 || result.Totals.DraftBatchCount != 1 || result.Totals.PendingReviewBatchCount != 1 ||
		result.Totals.ConfirmedBatchCount != 1 || result.Totals.ConfirmedRecordCount != 3 || result.Totals.PendingReviewOrReturnedRecordCount != 2 {
		t.Fatalf("totals = %+v", result.Totals)
	}
}

func TestSummaryTrimsValidatedPeriodIDsBeforeRepositoryQuery(t *testing.T) {
	const semesterID = "a0000000-0000-0000-0000-000000000003"
	const weekID = "a0000000-0000-0000-0000-000000000004"
	repository := &reportingRepositoryStub{summary: Summary{Rows: []SummaryRow{{DepartmentID: "dept-1"}}}}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})

	if _, err := service.Summary(context.Background(), "actor", " \t"+semesterID+"\n", "\n"+weekID+" "); err != nil {
		t.Fatal(err)
	}
	if repository.semesterID != semesterID || repository.weekID != weekID {
		t.Fatalf("repository period = %q/%q, want trimmed %q/%q", repository.semesterID, repository.weekID, semesterID, weekID)
	}
}

func TestPeriodsReturnsVisiblePeriods(t *testing.T) {
	period := PeriodOption{AcademicYearID: "year", SemesterID: "semester", WeekID: "week", WeekNo: 1}
	repository := &reportingRepositoryStub{periods: []PeriodOption{period}}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})

	result, err := service.Periods(context.Background(), "actor")
	if err != nil || len(result) != 1 || result[0].WeekID != "week" {
		t.Fatalf("periods = %+v, %v", result, err)
	}

	emptyService := NewService(&reportingRepositoryStub{}, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})
	empty, err := emptyService.Periods(context.Background(), "actor")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("authorized empty periods = %#v, %v; want a non-nil empty list", empty, err)
	}

	noGrantService := NewService(&reportingRepositoryStub{}, reportingPermissionsStub{})
	if _, err := noGrantService.Periods(context.Background(), "actor"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("missing report grant error = %v", err)
	}
}

func TestRecordsPassesAuthenticatedActorToRepositoryForReadHint(t *testing.T) {
	const (
		actorID          = "a0000000-0000-0000-0000-000000000001"
		semesterID       = "a0000000-0000-0000-0000-000000000003"
		weekID           = "a0000000-0000-0000-0000-000000000004"
		departmentID     = "10000000-0000-0000-0000-000000000002"
		inspectionTypeID = "30000000-0000-0000-0000-000000000001"
	)
	repository := &reportingRepositoryStub{
		summary: Summary{Rows: []SummaryRow{{DepartmentID: departmentID, InspectionTypeID: inspectionTypeID}}},
		records: []inspection.InspectionRecord{{ID: "record-1"}}, recordTotal: 1,
		filterOptions: ReportVersionRecordFilterOptions{StudentNumbers: []string{"20260001"}, StudentNames: []string{"李同学"}, ClassNames: []string{"计算机一班"}, Grades: []string{"2026级"}},
	}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})
	query := RecordsQuery{SemesterID: semesterID, WeekID: weekID, DepartmentID: departmentID,
		InspectionTypeID: inspectionTypeID, Page: 1, PageSize: 20}

	result, err := service.Records(context.Background(), actorID, query)
	if err != nil {
		t.Fatal(err)
	}
	if repository.recordActor != actorID || repository.recordQuery != query || len(repository.permissions) != 1 ||
		result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != "record-1" {
		t.Fatalf("records query actor/query/permissions/result = %q/%+v/%+v/%+v", repository.recordActor, repository.recordQuery, repository.permissions, result)
	}
	if result.FilterOptions == nil || len(result.FilterOptions.StudentNumbers) != 1 || result.FilterOptions.StudentNumbers[0] != "20260001" {
		t.Fatalf("report source filter options = %+v; want full roster choices", result.FilterOptions)
	}
}

func TestRecordsTrimsAndBoundsFilters(t *testing.T) {
	const (
		semesterID       = "a0000000-0000-0000-0000-000000000003"
		weekID           = "a0000000-0000-0000-0000-000000000004"
		inspectionTypeID = "30000000-0000-0000-0000-000000000001"
	)
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	repository := &reportingRepositoryStub{summary: Summary{Rows: []SummaryRow{{InspectionTypeID: inspectionTypeID}}}}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})

	_, err := service.Records(context.Background(), "actor", RecordsQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		StudentNumber: " 20260001 ", StudentName: " 李同学 ", ClassName: " 计算机一班 ", Grade: " 2026级 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.recordQuery.StudentNumber != "20260001" || repository.recordQuery.StudentName != "李同学" ||
		repository.recordQuery.ClassName != "计算机一班" || repository.recordQuery.Grade != "2026级" {
		t.Fatalf("repository filters = student number %q, name %q, class %q, grade %q; want trimmed values",
			repository.recordQuery.StudentNumber, repository.recordQuery.StudentName, repository.recordQuery.ClassName, repository.recordQuery.Grade)
	}

	if _, err := service.Records(context.Background(), "actor", RecordsQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		StudentNumber: strings.Repeat("a", 33),
	}); !errors.Is(err, ErrInvalidRecordsQuery) {
		t.Fatalf("oversized student number error = %v, want %v", err, ErrInvalidRecordsQuery)
	}
	if _, err := service.Records(context.Background(), "actor", RecordsQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		StudentName: strings.Repeat("a", 65),
	}); !errors.Is(err, ErrInvalidRecordsQuery) {
		t.Fatalf("oversized student name error = %v, want %v", err, ErrInvalidRecordsQuery)
	}
	if _, err := service.Records(context.Background(), "actor", RecordsQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		ClassName: strings.Repeat("a", 97),
	}); !errors.Is(err, ErrInvalidRecordsQuery) {
		t.Fatalf("oversized class name error = %v, want %v", err, ErrInvalidRecordsQuery)
	}
	if _, err := service.Records(context.Background(), "actor", RecordsQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		Grade: strings.Repeat("a", 41),
	}); !errors.Is(err, ErrInvalidRecordsQuery) {
		t.Fatalf("oversized grade error = %v, want %v", err, ErrInvalidRecordsQuery)
	}
}

func TestRecordsCanReadOneTypeAcrossVisibleSources(t *testing.T) {
	const semesterID = "a0000000-0000-0000-0000-000000000003"
	const weekID = "a0000000-0000-0000-0000-000000000004"
	const typeID = "30000000-0000-0000-0000-000000000001"
	repository := &reportingRepositoryStub{summary: Summary{Rows: []SummaryRow{
		{DepartmentID: "10000000-0000-0000-0000-000000000002", DepartmentName: "学习部", InspectionTypeID: typeID},
		{DepartmentID: "", DepartmentName: "管理员", CollegeLevel: true, InspectionTypeID: typeID},
	}}, records: []inspection.InspectionRecord{{ID: "record-1", DepartmentID: "10000000-0000-0000-0000-000000000002", InspectionTypeID: typeID},
		{ID: "record-2", InspectionTypeID: typeID}}, recordTotal: 2}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})
	result, err := service.Records(context.Background(), "actor", RecordsQuery{SemesterID: semesterID, WeekID: weekID, InspectionTypeID: typeID})
	if err != nil {
		t.Fatal(err)
	}
	if repository.recordQuery.DepartmentID != "" || repository.recordQuery.CollegeLevel || result.Total != 2 ||
		result.Items[0].SourceDepartmentName != "学习部" || result.Items[1].SourceDepartmentName != "管理员" {
		t.Fatalf("cross-source records = %+v; query = %+v", result, repository.recordQuery)
	}
}

func TestRepositoryFailureIsPropagated(t *testing.T) {
	want := errors.New("repository unavailable")
	repository := &reportingRepositoryStub{summaryErr: want}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})

	_, err := service.Summary(context.Background(), "actor", "a0000000-0000-0000-0000-000000000003", "a0000000-0000-0000-0000-000000000004")
	if !errors.Is(err, want) {
		t.Fatalf("summary error = %v, want %v", err, want)
	}
}
