package reporting

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
)

const (
	reportTestActor      = "a0000000-0000-0000-0000-000000000001"
	reportTestSemester   = "a0000000-0000-0000-0000-000000000003"
	reportTestWeek       = "a0000000-0000-0000-0000-000000000004"
	reportTestDepartment = "10000000-0000-0000-0000-000000000002"
	reportTestType       = "30000000-0000-0000-0000-000000000001"
)

type reportWorkflowRepositoryStub struct {
	*reportingRepositoryStub
	draftCommand     CreateReportDraftCommand
	draft            ReportDraft
	draftErr         error
	transitionRules  []permission.EffectivePermission
	transitionAction string
	transition       ReportState
	transitionErr    error
	correctionResult ReportCorrectionResult
	correctionErr    error
	adjustmentResult ReportAdjustmentResult
	adjustmentErr    error
	adjustmentInput  AddReportAdjustmentInput
	adjustmentRules  []permission.EffectivePermission
	decisionState    ReportState
	decisionErr      error
	workflow         ReportWorkflow
	workflowErr      error
}

func (r *reportWorkflowRepositoryStub) GetReportWorkflow(context.Context, string, string, []permission.EffectivePermission) (ReportWorkflow, error) {
	return r.workflow, r.workflowErr
}

func (r *reportWorkflowRepositoryStub) AddReportCorrection(context.Context, string, string, AddReportCorrectionInput,
	[]permission.EffectivePermission, audit.Metadata) (ReportCorrectionResult, error) {
	return r.correctionResult, r.correctionErr
}

func (r *reportWorkflowRepositoryStub) AddReportAdjustment(_ context.Context, _, _ string, input AddReportAdjustmentInput,
	permissions []permission.EffectivePermission, _ audit.Metadata) (ReportAdjustmentResult, error) {
	r.adjustmentInput, r.adjustmentRules = input, permissions
	return r.adjustmentResult, r.adjustmentErr
}

func (r *reportWorkflowRepositoryStub) DecideReportCorrection(context.Context, string, string, DecideReportCorrectionInput,
	[]permission.EffectivePermission, audit.Metadata) (ReportState, error) {
	return r.decisionState, r.decisionErr
}

func (r *reportWorkflowRepositoryStub) CreateReportDraft(_ context.Context, command CreateReportDraftCommand) (ReportDraft, error) {
	r.draftCommand = command
	return r.draft, r.draftErr
}

func (r *reportWorkflowRepositoryStub) TransitionReport(_ context.Context, _ string, _ string, action string,
	_ ReportTransitionInput, permissions []permission.EffectivePermission, _ audit.Metadata) (ReportState, error) {
	r.transitionAction, r.transitionRules = action, permissions
	return r.transition, r.transitionErr
}

func TestGenerateDraftUsesOnlyConfirmedCountsAndRejectsIncompleteCells(t *testing.T) {
	period := PeriodOption{AcademicYearID: "a0000000-0000-0000-0000-000000000002", SemesterID: reportTestSemester, WeekID: reportTestWeek}
	grant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	generate := permission.EffectivePermission{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}
	summary := Summary{Period: period, Rows: []SummaryRow{{
		DepartmentID: reportTestDepartment, DepartmentName: "学习部", InspectionTypeID: reportTestType,
		InspectionTypeName: "到课率", IsRequired: true, ConfirmedRecordCount: 0, ZeroRecordConfirmedBatchCount: 0,
	}}}
	repository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{summary: summary}}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant, generate}}})
	_, err := service.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester, WeekID: reportTestWeek}, audit.Metadata{})
	var incomplete *IncompleteReportError
	if !errors.As(err, &incomplete) || len(incomplete.Cells) != 1 || incomplete.Cells[0].DepartmentName != "学习部" {
		t.Fatalf("incomplete generation error = %#v", err)
	}
	if repository.draftCommand.ActorID != "" {
		t.Fatal("incomplete cell reached repository")
	}

	repository.summary.Rows[0].ConfirmedBatchCount = 0
	repository.summary.Rows[0].ZeroRecordConfirmedBatchCount = 1
	_, err = service.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester, WeekID: reportTestWeek}, audit.Metadata{})
	if err != nil {
		t.Fatalf("confirmed zero receipt should complete a cell: %v", err)
	}
	if len(repository.draftCommand.Content.Rows) != 1 || repository.draftCommand.Content.ConfirmedRecordCount != 0 ||
		repository.draftCommand.Content.Rows[0].ZeroReceiptCount != 1 ||
		repository.draftCommand.Content.Rows[0].DataStatus != "CONFIRMED_ZERO_RECORD_RECEIPT" ||
		len(repository.draftCommand.Content.IncompleteCells) != 0 {
		t.Fatalf("generated count-only content = %+v", repository.draftCommand.Content)
	}
}

func TestGenerateDraftForceRequiresReasonAndScopedForcePermission(t *testing.T) {
	period := PeriodOption{AcademicYearID: "a0000000-0000-0000-0000-000000000002", SemesterID: reportTestSemester, WeekID: reportTestWeek}
	view := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	generate := permission.EffectivePermission{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}
	force := permission.EffectivePermission{Action: permission.ActionReportForceGenerate, Effect: permission.EffectAllow}
	rows := []SummaryRow{{DepartmentID: reportTestDepartment, DepartmentName: "学习部", InspectionTypeID: reportTestType,
		InspectionTypeName: "到课率", IsRequired: true}}
	repository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{summary: Summary{Period: period, Rows: rows}}}
	permissions := reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{view, generate}}}
	service := NewService(repository, permissions)
	if _, err := service.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester,
		WeekID: reportTestWeek, Force: true}, audit.Metadata{}); !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("force without reason error = %v", err)
	}
	if _, err := service.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester,
		WeekID: reportTestWeek, Force: true, Reason: "紧急出稿"}, audit.Metadata{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("force without dedicated permission error = %v", err)
	}
	permissions.result.Permissions = append(permissions.result.Permissions, force)
	service = NewService(repository, permissions)
	if _, err := service.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester,
		WeekID: reportTestWeek, Force: true, Reason: "紧急出稿"}, audit.Metadata{}); err != nil {
		t.Fatalf("authorized force generation: %v", err)
	}
	if repository.draftCommand.ForceReason != "紧急出稿" || len(repository.draftCommand.Content.IncompleteCells) != 1 {
		t.Fatalf("force generation metadata = %+v", repository.draftCommand)
	}
	completeSummary := Summary{Period: period, Rows: []SummaryRow{{
		DepartmentID: reportTestDepartment, DepartmentName: "学习部", InspectionTypeID: reportTestType,
		InspectionTypeName: "到课率", IsRequired: true, ConfirmedBatchCount: 1, ZeroRecordConfirmedBatchCount: 1,
	}}}
	completeRepository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{summary: completeSummary}}
	completeService := NewService(completeRepository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{view, generate, force}}})
	if _, err := completeService.GenerateDraft(context.Background(), reportTestActor, GenerateDraftInput{SemesterID: reportTestSemester,
		WeekID: reportTestWeek, Force: true, Reason: "紧急出稿"}, audit.Metadata{}); !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("force request without an incomplete cell = %v", err)
	}
}

func TestValidateReportCorrectionValues(t *testing.T) {
	tests := []struct {
		name        string
		field       string
		value       string
		factChanged bool
		wantErr     bool
	}{
		{name: "report note", field: "details.remark", value: `"已核对说明"`},
		{name: "confirmed factual text", field: "studentName", value: `"王同学"`, factChanged: true},
		{name: "confirmed numeric detail", field: "details.absenceHours", value: `2`, factChanged: true},
		{name: "confirmed integer detail", field: "details.absenceCount", value: `1`, factChanged: true},
		{name: "reject changed name without reconfirmation", field: "studentName", value: `"王同学"`, wantErr: true},
		{name: "reject wrong field type", field: "studentName", value: `2`, factChanged: true, wantErr: true},
		{name: "reject zero event hours", field: "details.absenceHours", value: `0`, factChanged: true, wantErr: true},
		{name: "reject fractional absence count", field: "details.absenceCount", value: `1.5`, factChanged: true, wantErr: true},
		{name: "reject multiple JSON values", field: "details.absenceHours", value: `2 3`, factChanged: true, wantErr: true},
		{name: "reject unknown field", field: "internalId", value: `"x"`, factChanged: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateCorrectionValue(test.field, []byte(test.value), test.factChanged)
			if test.wantErr && !errors.Is(err, ErrInvalidReportCorrection) {
				t.Fatalf("validation error = %v; want invalid correction", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("valid correction rejected: %v", err)
			}
		})
	}
}

func TestAddReportAdjustmentRequiresReasonAndCorrectionPermission(t *testing.T) {
	correction := permission.EffectivePermission{Action: permission.ActionReportCorrect, Effect: permission.EffectAllow}
	result := ReportAdjustmentResult{AdjustmentID: "adjustment", Report: ReportState{ID: "report", Status: ReportStatusDraft, RowVersion: 3}}
	repository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{}, adjustmentResult: result}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{correction}}})
	input := AddReportAdjustmentInput{RowVersion: 2, RecordID: "a0000000-0000-0000-0000-000000000014", Operation: "EXCLUDE", Reason: "本版排除重复记录"}
	actual, err := service.AddReportAdjustment(context.Background(), reportTestActor, "a0000000-0000-0000-0000-000000000009", input, audit.Metadata{})
	if err != nil || actual.AdjustmentID != result.AdjustmentID || repository.adjustmentInput != input || len(repository.adjustmentRules) != 1 || repository.adjustmentRules[0].Action != permission.ActionReportCorrect {
		t.Fatalf("authorized report adjustment = %+v, input=%+v rules=%+v err=%v", actual, repository.adjustmentInput, repository.adjustmentRules, err)
	}
	input.Operation = "DELETE"
	if _, err := service.AddReportAdjustment(context.Background(), reportTestActor, "a0000000-0000-0000-0000-000000000009", input, audit.Metadata{}); !errors.Is(err, ErrInvalidReportAdjustment) {
		t.Fatalf("invalid adjustment operation error = %v", err)
	}
}

func TestTransitionAllowsCreatorToReviewWhenActionPermissionAllows(t *testing.T) {
	view := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	review := permission.EffectivePermission{Action: permission.ActionReportReview, Effect: permission.EffectAllow}
	repository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{},
		transition: ReportState{ID: "a0000000-0000-0000-0000-000000000009", Status: ReportStatusFinalizing}}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{view, review}}})
	result, err := service.TransitionReport(context.Background(), reportTestActor, repository.transition.ID,
		ReportActionApprove, ReportTransitionInput{RowVersion: 1}, audit.Metadata{})
	if err != nil || result.Status != ReportStatusFinalizing || repository.transitionAction != ReportActionApprove || len(repository.transitionRules) != 1 {
		t.Fatalf("creator approval result/action/rules = %+v/%s/%+v, %v", result, repository.transitionAction, repository.transitionRules, err)
	}
}
