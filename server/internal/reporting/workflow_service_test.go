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
	correctionInput  AddReportCorrectionInput
	correctionRules  []permission.EffectivePermission
	adjustmentResult ReportAdjustmentResult
	adjustmentErr    error
	adjustmentInput  AddReportAdjustmentInput
	adjustmentRules  []permission.EffectivePermission
	workflow         ReportWorkflow
	workflowErr      error
}

func (r *reportWorkflowRepositoryStub) GetReportWorkflow(context.Context, string, string, []permission.EffectivePermission) (ReportWorkflow, error) {
	return r.workflow, r.workflowErr
}

func (r *reportWorkflowRepositoryStub) AddReportCorrection(_ context.Context, _, _ string, input AddReportCorrectionInput,
	permissions []permission.EffectivePermission, _ audit.Metadata) (ReportCorrectionResult, error) {
	r.correctionInput, r.correctionRules = input, permissions
	return r.correctionResult, r.correctionErr
}

func (r *reportWorkflowRepositoryStub) AddReportAdjustment(_ context.Context, _, _ string, input AddReportAdjustmentInput,
	permissions []permission.EffectivePermission, _ audit.Metadata) (ReportAdjustmentResult, error) {
	r.adjustmentInput, r.adjustmentRules = input, permissions
	return r.adjustmentResult, r.adjustmentErr
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
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{name: "report note", field: "details.remark", value: `"已核对说明"`},
		{name: "factual text applies without department confirmation", field: "studentName", value: `"王同学"`},
		{name: "numeric detail", field: "details.absenceHours", value: `2`},
		{name: "integer absence detail", field: "details.absenceCount", value: `1`},
		{name: "penalty count", field: "details.penaltyHours", value: `2`},
		{name: "reject wrong field type", field: "studentName", value: `2`, wantErr: true},
		{name: "reject zero event hours", field: "details.absenceHours", value: `0`, wantErr: true},
		{name: "reject fractional absence count", field: "details.absenceCount", value: `1.5`, wantErr: true},
		{name: "reject multiple JSON values", field: "details.absenceHours", value: `2 3`, wantErr: true},
		{name: "reject unknown field", field: "internalId", value: `"x"`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateCorrectionValue(test.field, []byte(test.value))
			if test.wantErr && !errors.Is(err, ErrInvalidReportCorrection) {
				t.Fatalf("validation error = %v; want invalid correction", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("valid correction rejected: %v", err)
			}
		})
	}
}

func TestReportCorrectionFieldsFollowInspectionType(t *testing.T) {
	tests := []struct {
		name, typeCode, field string
		want                  bool
	}{
		{name: "attendance count", typeCode: "ATTENDANCE_RATE", field: "details.penaltyHours", want: true},
		{name: "attendance course", typeCode: "ATTENDANCE_RATE", field: "currentCourseName", want: true},
		{name: "hygiene result", typeCode: "DORMITORY_HYGIENE", field: "details.result", want: true},
		{name: "late return dormitory", typeCode: "LATE_RETURN", field: "currentDormitoryName", want: true},
		{name: "attendance cannot edit hygiene result", typeCode: "ATTENDANCE_RATE", field: "details.result"},
		{name: "hygiene cannot edit student", typeCode: "DORMITORY_HYGIENE", field: "studentName"},
		{name: "unknown type is denied", typeCode: "UNKNOWN", field: "studentName"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ReportCorrectionFieldAllowed(test.typeCode, test.field); got != test.want {
				t.Fatalf("field allowed = %t; want %t", got, test.want)
			}
		})
	}
}

func TestReportCorrectionValuesFollowInspectionType(t *testing.T) {
	if !ReportCorrectionValueAllowed("ATTENDANCE_RATE", "details.penaltyHours", []byte(`2`)) {
		t.Fatal("integer attendance penalty count should be allowed")
	}
	if ReportCorrectionValueAllowed("ATTENDANCE_RATE", "details.penaltyHours", []byte(`1.5`)) {
		t.Fatal("fractional attendance penalty count should be rejected")
	}
	if !ReportCorrectionValueAllowed("UNATTENDED_CLASSROOM", "details.penaltyHours", []byte(`1.5`)) {
		t.Fatal("fractional unattended-classroom penalty hours should remain valid")
	}
}

func TestAddReportCorrectionValidatesOneAtomicMultiFieldEdit(t *testing.T) {
	reportID := "a0000000-0000-0000-0000-000000000009"
	correct := permission.EffectivePermission{Action: permission.ActionReportCorrect, Effect: permission.EffectAllow}
	repository := &reportWorkflowRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{},
		correctionResult: ReportCorrectionResult{CorrectionID: "correction-1", Report: ReportState{ID: reportID, VersionNo: 2}}}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{correct}}})
	input := AddReportCorrectionInput{RowVersion: 1, RecordID: "a0000000-0000-0000-0000-000000000014", Reason: "依据现场证据核对",
		Changes: []ReportCorrectionChange{
			{Field: "studentName", AfterValue: []byte(`"王同学"`)},
			{Field: "details.date", AfterValue: []byte(`"2026-03-16"`)},
		}}
	actual, err := service.AddReportCorrection(context.Background(), reportTestActor, reportID, input, audit.Metadata{})
	if err != nil || actual.CorrectionID != "correction-1" || len(repository.correctionInput.Changes) != 2 ||
		len(repository.correctionRules) != 1 || repository.correctionRules[0].Action != permission.ActionReportCorrect {
		t.Fatalf("authorized multi-field correction = %+v, input=%+v rules=%+v err=%v", actual, repository.correctionInput, repository.correctionRules, err)
	}

	input.Changes = append(input.Changes, input.Changes[0])
	if _, err := service.AddReportCorrection(context.Background(), reportTestActor, reportID, input, audit.Metadata{}); !errors.Is(err, ErrInvalidReportCorrection) {
		t.Fatalf("duplicate changed field error = %v", err)
	}
	input.Changes = []ReportCorrectionChange{{Field: "internalId", AfterValue: []byte(`"hidden"`)}}
	if _, err := service.AddReportCorrection(context.Background(), reportTestActor, reportID, input, audit.Metadata{}); !errors.Is(err, ErrInvalidReportCorrection) {
		t.Fatalf("invalid value for type-agnostic field validation error = %v", err)
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
