package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

var (
	ErrInvalidPeriod               = errors.New("invalid reporting period")
	ErrInvalidRecordsQuery         = errors.New("invalid reporting records query")
	ErrForbidden                   = errors.New("report summary scope unavailable")
	ErrInvalidObligation           = errors.New("invalid weekly report obligation")
	ErrObligationPeriodUnavailable = errors.New("weekly report period unavailable")
	ErrObligationScopeUnavailable  = errors.New("weekly report obligation scope unavailable")
	ErrObligationAlreadyExists     = errors.New("weekly report obligation already exists")
	ErrObligationNotFound          = errors.New("weekly report obligation not found")
	ErrObligationVersionConflict   = errors.New("weekly report obligation version conflict")
	ErrInvalidReportRequest        = errors.New("invalid weekly report request")
	ErrInvalidReportArchiveQuery   = errors.New("invalid weekly report archive query")
	ErrReportIncomplete            = errors.New("weekly report has incomplete required cells")
	ErrReportVersionConflict       = errors.New("weekly report version conflict")
	ErrReportNotFound              = errors.New("weekly report not found")
	ErrInvalidReportCorrection     = errors.New("invalid weekly report correction")
	ErrInvalidReportAdjustment     = errors.New("invalid weekly report adjustment")
	ErrReportCorrectionNotFound    = errors.New("weekly report correction not found")
	ErrReportCorrectionDecided     = errors.New("weekly report correction already decided")
	ErrReportReconfirmationPending = errors.New("weekly report has pending department reconfirmation")
	ErrReportSourceChanged         = errors.New("weekly report source data changed")
)

type IncompleteReportError struct{ Cells []IncompleteReportCell }

func (e *IncompleteReportError) Error() string { return ErrReportIncomplete.Error() }
func (e *IncompleteReportError) Unwrap() error { return ErrReportIncomplete }

type Repository interface {
	ListReportPeriods(context.Context, []permission.EffectivePermission) ([]PeriodOption, error)
	ReportSummary(context.Context, string, string, []permission.EffectivePermission) (Summary, error)
	ReportRecords(context.Context, string, RecordsQuery, []permission.EffectivePermission) ([]inspection.InspectionRecord, int, error)
}

type obligationRepository interface {
	ObligationPeriod(context.Context, string, string) (PeriodOption, error)
	ReportObligationOptions(context.Context, string, string) (ReportObligationOptions, error)
	ListReportObligations(context.Context, string, string, []permission.EffectivePermission) ([]ReportObligation, error)
	CreateReportObligation(context.Context, string, CreateReportObligationInput, audit.Metadata) (ReportObligation, error)
	GetReportObligation(context.Context, string) (ReportObligation, error)
	RemoveReportObligation(context.Context, string, int, audit.Metadata) error
}

type permissionService interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}

type ReportCell struct {
	DepartmentID     string
	InspectionTypeID string
}

type CreateReportDraftCommand struct {
	ReportID           string
	VersionID          string
	ActorID            string
	ExpectedRowVersion int
	Content            ReportVersionContent
	Records            []ReportRecordSnapshot
	ForceReason        string
	Cells              []ReportCell
	Metadata           audit.Metadata
}

type reportWorkflowRepository interface {
	GetReportWorkflow(context.Context, string, string, []permission.EffectivePermission) (ReportWorkflow, error)
	CreateReportDraft(context.Context, CreateReportDraftCommand) (ReportDraft, error)
	TransitionReport(context.Context, string, string, string, ReportTransitionInput, []permission.EffectivePermission, audit.Metadata) (ReportState, error)
	AddReportCorrection(context.Context, string, string, AddReportCorrectionInput, []permission.EffectivePermission, audit.Metadata) (ReportCorrectionResult, error)
	AddReportAdjustment(context.Context, string, string, AddReportAdjustmentInput, []permission.EffectivePermission, audit.Metadata) (ReportAdjustmentResult, error)
	DecideReportCorrection(context.Context, string, string, DecideReportCorrectionInput, []permission.EffectivePermission, audit.Metadata) (ReportState, error)
}

type reportArchiveRepository interface {
	ListReportArchive(context.Context, ReportArchiveQuery, []permission.EffectivePermission) (ReportArchivePage, error)
}

type reportArchiveWorkflowRepository interface {
	GetReportWorkflowForAction(context.Context, string, string, []permission.EffectivePermission, string) (ReportWorkflow, error)
}

type reportVersionRecordsRepository interface {
	ListReportVersionRecords(context.Context, string, string, int, ReportVersionRecordsQuery, []permission.EffectivePermission) (RecordPage, error)
}

type Service struct {
	repository  Repository
	permissions permissionService
}

func NewService(repository Repository, permissions permissionService) *Service {
	return &Service{repository: repository, permissions: permissions}
}

func (s *Service) ReportObligationOptions(ctx context.Context, accountID string, query ObligationPeriodQuery) (ReportObligationOptions, error) {
	query, err := normalizeObligationPeriod(query)
	if err != nil {
		return ReportObligationOptions{}, err
	}
	repository, err := s.obligationsRepository()
	if err != nil {
		return ReportObligationOptions{}, err
	}
	period, effective, options, err := s.obligationContext(ctx, repository, accountID, query)
	if err != nil {
		return ReportObligationOptions{}, err
	}
	options.Period = period
	return filterObligationOptions(options, period, effective)
}

func (s *Service) ReportObligations(ctx context.Context, accountID string, query ObligationPeriodQuery) ([]ReportObligation, error) {
	query, err := normalizeObligationPeriod(query)
	if err != nil {
		return nil, err
	}
	repository, err := s.obligationsRepository()
	if err != nil {
		return nil, err
	}
	_, effective, _, err := s.obligationContext(ctx, repository, accountID, query)
	if err != nil {
		return nil, err
	}
	items, err := repository.ListReportObligations(ctx, query.SemesterID, query.WeekID, effective.Permissions)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]ReportObligation, 0)
	}
	return items, nil
}

func (s *Service) CreateReportObligation(ctx context.Context, accountID string, input CreateReportObligationInput, metadata audit.Metadata) (ReportObligation, error) {
	query, err := normalizeObligationPeriod(ObligationPeriodQuery{SemesterID: input.SemesterID, WeekID: input.WeekID})
	if err != nil {
		return ReportObligation{}, err
	}
	input.SemesterID, input.WeekID = query.SemesterID, query.WeekID
	typeID, err := canonicalUUID(input.InspectionTypeID)
	if err != nil {
		return ReportObligation{}, ErrInvalidObligation
	}
	input.InspectionTypeID = typeID
	input.DepartmentID = strings.TrimSpace(input.DepartmentID)
	if input.DepartmentID != "" {
		departmentID, parseErr := canonicalUUID(input.DepartmentID)
		if parseErr != nil || departmentID == uuid.Nil.String() {
			return ReportObligation{}, ErrInvalidObligation
		}
		input.DepartmentID = departmentID
	}
	repository, err := s.obligationsRepository()
	if err != nil {
		return ReportObligation{}, err
	}
	effective, err := s.obligationPermissions(ctx, accountID)
	if err != nil {
		return ReportObligation{}, err
	}
	period, err := repository.ObligationPeriod(ctx, query.SemesterID, query.WeekID)
	if err != nil {
		return ReportObligation{}, err
	}
	if !canManageObligationType(effective, period, input.InspectionTypeID) {
		return ReportObligation{}, ErrForbidden
	}
	metadata.ActorID = accountID
	return repository.CreateReportObligation(ctx, uuid.NewString(), input, metadata)
}

func (s *Service) RemoveReportObligation(ctx context.Context, accountID, obligationID string, version int, metadata audit.Metadata) error {
	canonicalID, err := canonicalUUID(obligationID)
	if err != nil || version < 1 {
		return ErrInvalidObligation
	}
	repository, err := s.obligationsRepository()
	if err != nil {
		return err
	}
	effective, err := s.obligationPermissions(ctx, accountID)
	if err != nil {
		return err
	}
	current, err := repository.GetReportObligation(ctx, canonicalID)
	if err != nil {
		return err
	}
	period := PeriodOption{AcademicYearID: current.AcademicYearID, SemesterID: current.SemesterID, WeekID: current.WeekID}
	if !canManageObligationType(effective, period, current.InspectionTypeID) {
		return ErrForbidden
	}
	metadata.ActorID = accountID
	return repository.RemoveReportObligation(ctx, canonicalID, version, metadata)
}

func (s *Service) obligationsRepository() (obligationRepository, error) {
	repository, ok := s.repository.(obligationRepository)
	if !ok {
		return nil, errors.New("weekly report obligation repository unavailable")
	}
	return repository, nil
}

func normalizeObligationPeriod(query ObligationPeriodQuery) (ObligationPeriodQuery, error) {
	semesterID, semesterErr := canonicalUUID(query.SemesterID)
	weekID, weekErr := canonicalUUID(query.WeekID)
	if semesterErr != nil || weekErr != nil {
		return ObligationPeriodQuery{}, ErrInvalidObligation
	}
	query.SemesterID, query.WeekID = semesterID, weekID
	return query, nil
}

func canonicalUUID(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func (s *Service) obligationContext(ctx context.Context, repository obligationRepository, accountID string, query ObligationPeriodQuery) (PeriodOption, permission.EffectiveResult, ReportObligationOptions, error) {
	effective, err := s.obligationPermissions(ctx, accountID)
	if err != nil {
		return PeriodOption{}, permission.EffectiveResult{}, ReportObligationOptions{}, err
	}
	period, err := repository.ObligationPeriod(ctx, query.SemesterID, query.WeekID)
	if err != nil {
		return PeriodOption{}, permission.EffectiveResult{}, ReportObligationOptions{}, err
	}
	options, err := repository.ReportObligationOptions(ctx, query.SemesterID, query.WeekID)
	if err != nil {
		return PeriodOption{}, permission.EffectiveResult{}, ReportObligationOptions{}, err
	}
	if _, err := filterObligationOptions(options, period, effective); err != nil {
		return PeriodOption{}, permission.EffectiveResult{}, ReportObligationOptions{}, err
	}
	return period, effective, options, nil
}

func (s *Service) obligationPermissions(ctx context.Context, accountID string) (permission.EffectiveResult, error) {
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return permission.EffectiveResult{}, err
	}
	for _, item := range effective.Permissions {
		if item.Action == permission.ActionReportObligationManage && item.Effect == permission.EffectAllow && item.Scope.ClassID == "" && item.Scope.Grade == "" && item.Scope.DepartmentID == "" {
			return effective, nil
		}
	}
	return permission.EffectiveResult{}, ErrForbidden
}

func filterObligationOptions(options ReportObligationOptions, period PeriodOption, effective permission.EffectiveResult) (ReportObligationOptions, error) {
	filtered := ReportObligationOptions{
		Period: period, InspectionTypes: make([]ReportObligationInspectionType, 0),
		DepartmentScopes: make([]ReportObligationDepartmentScope, 0),
	}
	allowedTypes := make(map[string]bool, len(options.InspectionTypes))
	for _, inspectionType := range options.InspectionTypes {
		if canManageObligationType(effective, period, inspectionType.ID) {
			allowedTypes[inspectionType.ID] = true
			filtered.InspectionTypes = append(filtered.InspectionTypes, inspectionType)
		}
	}
	if len(allowedTypes) == 0 {
		return ReportObligationOptions{}, ErrForbidden
	}
	for _, scope := range options.DepartmentScopes {
		if allowedTypes[scope.InspectionTypeID] {
			filtered.DepartmentScopes = append(filtered.DepartmentScopes, scope)
		}
	}
	return filtered, nil
}

func canManageObligationType(effective permission.EffectiveResult, period PeriodOption, inspectionTypeID string) bool {
	return effective.Allowed(permission.ActionReportObligationManage, permission.Scope{
		AcademicYearID: period.AcademicYearID, SemesterID: period.SemesterID,
		WeekID: period.WeekID, InspectionTypeID: inspectionTypeID,
	})
}

func (s *Service) Periods(ctx context.Context, accountID string) ([]PeriodOption, error) {
	permissions, err := s.reportPermissions(ctx, accountID)
	if err != nil {
		return nil, err
	}
	periods, err := s.repository.ListReportPeriods(ctx, permissions)
	if err != nil {
		return nil, err
	}
	if periods == nil {
		periods = make([]PeriodOption, 0)
	}
	return periods, nil
}

func (s *Service) Summary(ctx context.Context, accountID, semesterID, weekID string) (Summary, error) {
	semesterID = strings.TrimSpace(semesterID)
	weekID = strings.TrimSpace(weekID)
	if _, err := uuid.Parse(semesterID); err != nil {
		return Summary{}, ErrInvalidPeriod
	}
	if _, err := uuid.Parse(weekID); err != nil {
		return Summary{}, ErrInvalidPeriod
	}
	permissions, err := s.reportPermissions(ctx, accountID)
	if err != nil {
		return Summary{}, err
	}
	summary, err := s.repository.ReportSummary(ctx, semesterID, weekID, permissions)
	if err != nil {
		return Summary{}, err
	}
	if len(summary.Rows) == 0 {
		return Summary{}, ErrForbidden
	}
	for _, row := range summary.Rows {
		summary.Totals.SubmissionCount += row.SubmissionCount
		summary.Totals.DraftBatchCount += row.DraftBatchCount
		summary.Totals.PendingReviewBatchCount += row.PendingReviewBatchCount
		summary.Totals.ReturnedBatchCount += row.ReturnedBatchCount
		summary.Totals.ConfirmedBatchCount += row.ConfirmedBatchCount
		summary.Totals.ConfirmedRecordCount += row.ConfirmedRecordCount
		summary.Totals.PendingReviewOrReturnedRecordCount += row.PendingReviewOrReturnedRecordCount
		summary.Totals.ZeroRecordPendingReviewBatchCount += row.ZeroRecordPendingReviewBatchCount
		summary.Totals.ZeroRecordReturnedBatchCount += row.ZeroRecordReturnedBatchCount
		summary.Totals.ZeroRecordConfirmedBatchCount += row.ZeroRecordConfirmedBatchCount
	}
	return summary, nil
}

// Workflow returns report versions and audit events only after validating that
// the actor can see the selected period and every cell in the frozen history.
func (s *Service) Workflow(ctx context.Context, accountID, semesterID, weekID string) (ReportWorkflow, error) {
	semesterID, weekID = strings.TrimSpace(semesterID), strings.TrimSpace(weekID)
	semester, err := uuid.Parse(semesterID)
	if err != nil {
		return ReportWorkflow{}, ErrInvalidPeriod
	}
	week, err := uuid.Parse(weekID)
	if err != nil {
		return ReportWorkflow{}, ErrInvalidPeriod
	}
	permissions, err := s.reportPermissions(ctx, accountID)
	if err != nil {
		return ReportWorkflow{}, err
	}
	summary, err := s.repository.ReportSummary(ctx, semester.String(), week.String(), permissions)
	if err != nil {
		return ReportWorkflow{}, err
	}
	if len(summary.Rows) == 0 {
		return ReportWorkflow{}, ErrForbidden
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportWorkflow{}, ErrInvalidReportRequest
	}
	return repository.GetReportWorkflow(ctx, semester.String(), week.String(), permissions)
}

// ReportVersionRecords reads the frozen display projection for one report version.
// Workflow authorization is checked first so even empty and zero-receipt cells
// remain subject to the report's complete scope boundary.
func (s *Service) ReportVersionRecords(ctx context.Context, accountID, semesterID, weekID string,
	versionNo int, query ReportVersionRecordsQuery) (RecordPage, error) {
	if versionNo < 1 {
		return RecordPage{}, ErrInvalidRecordsQuery
	}
	semester, semesterErr := uuid.Parse(strings.TrimSpace(semesterID))
	week, weekErr := uuid.Parse(strings.TrimSpace(weekID))
	if semesterErr != nil || weekErr != nil {
		return RecordPage{}, ErrInvalidPeriod
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.Page > 1_000_000 || query.PageSize < 1 || query.PageSize > 100 {
		return RecordPage{}, ErrInvalidRecordsQuery
	}
	query.DepartmentID = strings.TrimSpace(query.DepartmentID)
	query.InspectionTypeID = strings.TrimSpace(query.InspectionTypeID)
	if query.InspectionTypeID != "" {
		inspectionTypeID, parseErr := uuid.Parse(query.InspectionTypeID)
		if parseErr != nil {
			return RecordPage{}, ErrInvalidRecordsQuery
		}
		query.InspectionTypeID = inspectionTypeID.String()
		if query.CollegeLevel {
			if query.DepartmentID != "" {
				return RecordPage{}, ErrInvalidRecordsQuery
			}
		} else {
			departmentID, parseErr := uuid.Parse(query.DepartmentID)
			if parseErr != nil {
				return RecordPage{}, ErrInvalidRecordsQuery
			}
			query.DepartmentID = departmentID.String()
		}
	} else if query.DepartmentID != "" || query.CollegeLevel {
		return RecordPage{}, ErrInvalidRecordsQuery
	}
	workflow, err := s.Workflow(ctx, accountID, semester.String(), week.String())
	if err != nil {
		return RecordPage{}, err
	}
	if workflow.Report == nil {
		return RecordPage{}, ErrReportNotFound
	}
	versionExists := false
	for _, version := range workflow.Versions {
		if version.VersionNo == versionNo {
			versionExists = true
			break
		}
	}
	if !versionExists {
		return RecordPage{}, ErrReportNotFound
	}
	permissions, err := s.reportPermissions(ctx, accountID)
	if err != nil {
		return RecordPage{}, err
	}
	repository, ok := s.repository.(reportVersionRecordsRepository)
	if !ok {
		return RecordPage{}, ErrInvalidReportRequest
	}
	return repository.ListReportVersionRecords(ctx, semester.String(), week.String(), versionNo, query, permissions)
}

// Archive lists report snapshots after their first generated draft and requires
// the independent archive permission, with row-level scope filtering in the repository.
func (s *Service) Archive(ctx context.Context, accountID string, query ReportArchiveQuery) (ReportArchivePage, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Page < 1 || query.Page > 1_000_000 || query.PageSize < 0 || query.PageSize > 100 {
		return ReportArchivePage{}, ErrInvalidReportArchiveQuery
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Status != "" && query.Status != ReportStatusDraft && query.Status != ReportStatusPending &&
		query.Status != ReportStatusReturned && query.Status != ReportStatusFinalizing &&
		query.Status != ReportStatusFinalized && query.Status != ReportStatusPublished && query.Status != ReportStatusArchived {
		return ReportArchivePage{}, ErrInvalidReportArchiveQuery
	}
	permissions, err := s.archivePermissions(ctx, accountID)
	if err != nil {
		return ReportArchivePage{}, err
	}
	repository, ok := s.repository.(reportArchiveRepository)
	if !ok {
		return ReportArchivePage{}, ErrInvalidReportRequest
	}
	return repository.ListReportArchive(ctx, query, permissions)
}

// ArchiveWorkflow returns an immutable report timeline after a draft version
// exists, only when every frozen unit is within the actor's archive scope.
func (s *Service) ArchiveWorkflow(ctx context.Context, accountID, semesterID, weekID string) (ReportWorkflow, error) {
	semester, err := uuid.Parse(strings.TrimSpace(semesterID))
	if err != nil {
		return ReportWorkflow{}, ErrInvalidPeriod
	}
	week, err := uuid.Parse(strings.TrimSpace(weekID))
	if err != nil {
		return ReportWorkflow{}, ErrInvalidPeriod
	}
	permissions, err := s.archivePermissions(ctx, accountID)
	if err != nil {
		return ReportWorkflow{}, err
	}
	repository, ok := s.repository.(reportArchiveWorkflowRepository)
	if !ok {
		return ReportWorkflow{}, ErrInvalidReportRequest
	}
	workflow, err := repository.GetReportWorkflowForAction(ctx, semester.String(), week.String(), permissions, permission.ActionArchiveView)
	if err != nil {
		return ReportWorkflow{}, err
	}
	if workflow.Report == nil || workflow.Report.VersionNo < 1 {
		return ReportWorkflow{}, ErrReportNotFound
	}
	return workflow, nil
}

func (s *Service) archivePermissions(ctx context.Context, accountID string) ([]permission.EffectivePermission, error) {
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return nil, err
	}
	permissions := make([]permission.EffectivePermission, 0)
	hasUsableGrant := false
	for _, item := range effective.Permissions {
		if item.Action != permission.ActionArchiveView {
			continue
		}
		permissions = append(permissions, item)
		if item.Effect == permission.EffectAllow && item.Scope.ClassID == "" && item.Scope.Grade == "" {
			hasUsableGrant = true
		}
	}
	if !hasUsableGrant {
		return nil, ErrForbidden
	}
	return permissions, nil
}

// Records returns the confirmed source records for a single report cell.
func (s *Service) Records(ctx context.Context, accountID string, query RecordsQuery) (RecordPage, error) {
	for _, value := range []*string{&query.SemesterID, &query.WeekID, &query.InspectionTypeID} {
		id, err := uuid.Parse(strings.TrimSpace(*value))
		if err != nil {
			return RecordPage{}, ErrInvalidRecordsQuery
		}
		*value = id.String()
	}
	query.DepartmentID = strings.TrimSpace(query.DepartmentID)
	if query.CollegeLevel {
		if query.DepartmentID != "" {
			return RecordPage{}, ErrInvalidRecordsQuery
		}
	} else {
		id, err := uuid.Parse(query.DepartmentID)
		if err != nil {
			return RecordPage{}, ErrInvalidRecordsQuery
		}
		query.DepartmentID = id.String()
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.Page > 1_000_000 || query.PageSize < 1 || query.PageSize > 100 {
		return RecordPage{}, ErrInvalidRecordsQuery
	}
	permissions, err := s.reportPermissions(ctx, accountID)
	if err != nil {
		return RecordPage{}, err
	}
	visibleSummary, err := s.repository.ReportSummary(ctx, query.SemesterID, query.WeekID, permissions)
	if err != nil {
		return RecordPage{}, err
	}
	visibleCell := false
	for _, row := range visibleSummary.Rows {
		if row.DepartmentID == query.DepartmentID && row.CollegeLevel == query.CollegeLevel && row.InspectionTypeID == query.InspectionTypeID {
			visibleCell = true
			break
		}
	}
	if !visibleCell {
		return RecordPage{}, ErrForbidden
	}
	records, total, err := s.repository.ReportRecords(ctx, accountID, query, permissions)
	if err != nil {
		return RecordPage{}, err
	}
	page := RecordPage{Items: make([]Record, 0, len(records)), Total: total, Page: query.Page, PageSize: query.PageSize}
	for _, record := range records {
		page.Items = append(page.Items, ReportRecordFromInspection(record))
	}
	return page, nil
}

// ReportRecordFromInspection projects an authorized source record into the
// stable, report-safe snapshot shape.
func ReportRecordFromInspection(record inspection.InspectionRecord) Record {
	return Record{
		ID: record.ID, RowVersion: record.RowVersion, StudentNumber: record.StudentNumber, StudentName: record.StudentName,
		ClassName: record.ClassName, Grade: record.Grade, TypeCode: record.TypeCode,
		Details:                reportRecordDetails(record.Details),
		CurrentDormitoryName:   record.CurrentDormitoryName,
		CurrentCourseName:      record.CurrentCourseName,
		CurrentClassroomName:   record.CurrentClassroomName,
		CurrentAreaName:        record.CurrentAreaName,
		CurrentReasonLabel:     record.CurrentReasonLabel,
		ResponsibleClasses:     reportRecordReferences(record.ResponsibleClasses),
		ResponsibleStudents:    reportRecordReferences(record.ResponsibleStudents),
		ResponsibleDormitories: reportRecordReferences(record.ResponsibleDormitories),
	}
}

// GenerateDraft creates a version from the visible, authorized, and confirmed
// source cells. No ratio, risk score, or unconfirmed submission is included.
func (s *Service) GenerateDraft(ctx context.Context, accountID string, input GenerateDraftInput, metadata audit.Metadata) (ReportDraft, error) {
	semesterID, semesterErr := canonicalUUID(input.SemesterID)
	weekID, weekErr := canonicalUUID(input.WeekID)
	if semesterErr != nil || weekErr != nil || input.ExpectedRowVersion < 0 {
		return ReportDraft{}, ErrInvalidReportRequest
	}
	input.SemesterID, input.WeekID = semesterID, weekID
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Force && input.Reason == "") || (!input.Force && input.Reason != "") {
		return ReportDraft{}, ErrInvalidReportRequest
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportDraft{}, errors.New("weekly report workflow repository unavailable")
	}
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return ReportDraft{}, err
	}
	viewPermissions := actionPermissions(effective.Permissions, permission.ActionReportView)
	if !hasUsableReportGrant(viewPermissions) {
		return ReportDraft{}, ErrForbidden
	}
	summary, err := s.repository.ReportSummary(ctx, semesterID, weekID, viewPermissions)
	if err != nil {
		return ReportDraft{}, err
	}
	if len(summary.Rows) == 0 {
		return ReportDraft{}, ErrForbidden
	}
	generatePermissions := actionPermissions(effective.Permissions, permission.ActionReportGenerate)
	forcePermissions := actionPermissions(effective.Permissions, permission.ActionReportForceGenerate)
	content := ReportVersionContent{
		Period: summary.Period, StatisticsRuleVersion: "COUNT_DETAILS_V1",
		Rows: make([]ReportCountRow, 0), IncompleteCells: make([]IncompleteReportCell, 0), Corrections: make([]ReportCorrectionSummary, 0),
		Adjustments: make([]ReportAdjustmentSummary, 0),
		GeneratedAt: time.Now().UTC(),
	}
	cells := make([]ReportCell, 0, len(summary.Rows))
	for _, row := range summary.Rows {
		scope := permission.Scope{AcademicYearID: summary.Period.AcademicYearID, SemesterID: semesterID,
			WeekID: weekID, InspectionTypeID: row.InspectionTypeID, DepartmentID: row.DepartmentID}
		if !permission.AllowsWholeScope(generatePermissions, permission.ActionReportGenerate, scope) {
			continue
		}
		cells = append(cells, ReportCell{DepartmentID: row.DepartmentID, InspectionTypeID: row.InspectionTypeID})
		content.ConfirmedRecordCount += row.ConfirmedRecordCount
		dataStatus := "NO_CONFIRMED_DATA"
		switch {
		case row.ConfirmedRecordCount > 0 && row.ZeroRecordConfirmedBatchCount > 0:
			dataStatus = "CONFIRMED_RECORDS_AND_ZERO_RECEIPT"
		case row.ConfirmedRecordCount > 0:
			dataStatus = "CONFIRMED_RECORDS"
		case row.ZeroRecordConfirmedBatchCount > 0:
			dataStatus = "CONFIRMED_ZERO_RECORD_RECEIPT"
		}
		content.Rows = append(content.Rows, ReportCountRow{
			DepartmentID: row.DepartmentID, DepartmentName: row.DepartmentName, CollegeLevel: row.CollegeLevel,
			InspectionTypeID: row.InspectionTypeID, InspectionType: row.InspectionTypeName,
			RecordCount: row.ConfirmedRecordCount, ZeroReceiptCount: row.ZeroRecordConfirmedBatchCount, DataStatus: dataStatus,
		})
		if row.IsRequired && row.ConfirmedBatchCount == 0 && row.ZeroRecordConfirmedBatchCount == 0 {
			content.IncompleteCells = append(content.IncompleteCells, IncompleteReportCell{
				DepartmentID: row.DepartmentID, DepartmentName: row.DepartmentName, CollegeLevel: row.CollegeLevel,
				InspectionTypeID: row.InspectionTypeID, InspectionType: row.InspectionTypeName,
			})
		}
	}
	if len(cells) == 0 {
		return ReportDraft{}, ErrForbidden
	}
	if input.Force && len(content.IncompleteCells) == 0 {
		return ReportDraft{}, ErrInvalidReportRequest
	}
	if !input.Force && len(content.IncompleteCells) > 0 {
		return ReportDraft{}, &IncompleteReportError{Cells: content.IncompleteCells}
	}
	if input.Force && len(content.IncompleteCells) > 0 {
		for _, incomplete := range content.IncompleteCells {
			if !permission.AllowsWholeScope(forcePermissions, permission.ActionReportForceGenerate, permission.Scope{
				AcademicYearID: summary.Period.AcademicYearID, SemesterID: semesterID, WeekID: weekID,
				InspectionTypeID: incomplete.InspectionTypeID, DepartmentID: incomplete.DepartmentID,
			}) {
				return ReportDraft{}, ErrForbidden
			}
		}
	}
	content.ScopeLimited = reportScopeIsLimited(viewPermissions) || reportScopeIsLimited(generatePermissions)
	recordSnapshots := make([]ReportRecordSnapshot, 0, content.ConfirmedRecordCount)
	for _, row := range content.Rows {
		query := RecordsQuery{SemesterID: semesterID, WeekID: weekID, DepartmentID: row.DepartmentID,
			CollegeLevel: row.CollegeLevel, InspectionTypeID: row.InspectionTypeID, Page: 1, PageSize: 100}
		items, total, err := s.repository.ReportRecords(ctx, accountID, query, viewPermissions)
		if err != nil {
			return ReportDraft{}, err
		}
		if total != row.RecordCount {
			return ReportDraft{}, ErrReportVersionConflict
		}
		loaded := 0
		for page := 1; loaded < total; page++ {
			for _, item := range items {
				recordSnapshots = append(recordSnapshots, ReportRecordSnapshot{RowVersion: item.RowVersion, Record: ReportRecordFromInspection(item)})
			}
			loaded += len(items)
			if loaded >= total {
				break
			}
			query.Page = page + 1
			items, _, err = s.repository.ReportRecords(ctx, accountID, query, viewPermissions)
			if err != nil {
				return ReportDraft{}, err
			}
		}
	}
	if len(recordSnapshots) != content.ConfirmedRecordCount {
		return ReportDraft{}, ErrReportVersionConflict
	}
	metadata.ActorID = accountID
	return repository.CreateReportDraft(ctx, CreateReportDraftCommand{
		ReportID: uuid.NewString(), VersionID: uuid.NewString(), ActorID: accountID,
		ExpectedRowVersion: input.ExpectedRowVersion, Content: content,
		Records: recordSnapshots, ForceReason: input.Reason, Cells: cells, Metadata: metadata,
	})
}

// TransitionReport applies one explicit lifecycle action with current scoped
// permissions and optimistic concurrency. Approval begins data finalization;
// output files are generated independently and do not gate the data snapshot.
func (s *Service) TransitionReport(ctx context.Context, accountID, reportID, action string, input ReportTransitionInput, metadata audit.Metadata) (ReportState, error) {
	canonicalID, err := canonicalUUID(reportID)
	if err != nil || input.RowVersion < 1 || len([]rune(input.Comment)) > 1000 {
		return ReportState{}, ErrInvalidReportRequest
	}
	permissionAction := ""
	switch action {
	case ReportActionSubmit:
		permissionAction = permission.ActionReportGenerate
	case ReportActionReturn:
		permissionAction = permission.ActionReportReturn
	case ReportActionApprove:
		permissionAction = permission.ActionReportReview
	case ReportActionFinalized:
		permissionAction = permission.ActionReportFinalize
	default:
		return ReportState{}, ErrInvalidReportRequest
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportState{}, errors.New("weekly report workflow repository unavailable")
	}
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return ReportState{}, err
	}
	actionRules := actionPermissions(effective.Permissions, permissionAction)
	if !hasUsableReportGrant(actionRules) {
		return ReportState{}, ErrForbidden
	}
	metadata.ActorID = accountID
	return repository.TransitionReport(ctx, canonicalID, accountID, action, input, actionRules, metadata)
}

func (s *Service) AddReportCorrection(ctx context.Context, accountID, reportID string, input AddReportCorrectionInput, metadata audit.Metadata) (ReportCorrectionResult, error) {
	canonicalID, err := canonicalUUID(reportID)
	if err != nil || input.RowVersion < 1 {
		return ReportCorrectionResult{}, ErrInvalidReportCorrection
	}
	if _, err := canonicalUUID(input.RecordID); err != nil {
		return ReportCorrectionResult{}, ErrInvalidReportCorrection
	}
	input.RecordID = strings.TrimSpace(input.RecordID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.Field = strings.TrimSpace(input.Field)
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 500 || len(input.AfterValue) == 0 || len(input.AfterValue) > 4096 {
		return ReportCorrectionResult{}, ErrInvalidReportCorrection
	}
	if _, err := validateCorrectionValue(input.Field, input.AfterValue, input.FactChanged); err != nil {
		return ReportCorrectionResult{}, ErrInvalidReportCorrection
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportCorrectionResult{}, errors.New("weekly report workflow repository unavailable")
	}
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return ReportCorrectionResult{}, err
	}
	correctionPermissions := actionPermissions(effective.Permissions, permission.ActionReportCorrect)
	if !hasUsableReportGrant(correctionPermissions) {
		return ReportCorrectionResult{}, ErrForbidden
	}
	metadata.ActorID = accountID
	metadata.Reason = input.Reason
	result, err := repository.AddReportCorrection(ctx, canonicalID, accountID, input, correctionPermissions, metadata)
	if err != nil {
		return ReportCorrectionResult{}, err
	}
	return result, nil
}

// AddReportAdjustment appends a new immutable version that includes or
// excludes one already confirmed source record. It never deletes source data.
func (s *Service) AddReportAdjustment(ctx context.Context, accountID, reportID string, input AddReportAdjustmentInput,
	metadata audit.Metadata) (ReportAdjustmentResult, error) {
	canonicalID, err := canonicalUUID(reportID)
	if err != nil || input.RowVersion < 1 {
		return ReportAdjustmentResult{}, ErrInvalidReportAdjustment
	}
	if _, err := canonicalUUID(input.RecordID); err != nil {
		return ReportAdjustmentResult{}, ErrInvalidReportAdjustment
	}
	input.RecordID = strings.TrimSpace(input.RecordID)
	input.Operation = strings.TrimSpace(input.Operation)
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Operation != "INCLUDE" && input.Operation != "EXCLUDE") ||
		len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 500 {
		return ReportAdjustmentResult{}, ErrInvalidReportAdjustment
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportAdjustmentResult{}, errors.New("weekly report workflow repository unavailable")
	}
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return ReportAdjustmentResult{}, err
	}
	correctionPermissions := actionPermissions(effective.Permissions, permission.ActionReportCorrect)
	if !hasUsableReportGrant(correctionPermissions) {
		return ReportAdjustmentResult{}, ErrForbidden
	}
	metadata.ActorID = accountID
	metadata.Reason = input.Reason
	return repository.AddReportAdjustment(ctx, canonicalID, accountID, input, correctionPermissions, metadata)
}

func (s *Service) DecideReportCorrection(ctx context.Context, accountID, correctionID string, input DecideReportCorrectionInput, metadata audit.Metadata) (ReportState, error) {
	canonicalID, err := canonicalUUID(correctionID)
	if err != nil || input.RowVersion < 1 || (input.Decision != "CONFIRMED" && input.Decision != "REJECTED") || len([]rune(input.Comment)) > 500 {
		return ReportState{}, ErrInvalidReportCorrection
	}
	if input.Decision == "REJECTED" && len([]rune(strings.TrimSpace(input.Comment))) < 5 {
		return ReportState{}, ErrInvalidReportCorrection
	}
	repository, ok := s.repository.(reportWorkflowRepository)
	if !ok {
		return ReportState{}, errors.New("weekly report workflow repository unavailable")
	}
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return ReportState{}, err
	}
	reviewPermissions := actionPermissions(effective.Permissions, permission.ActionInspectionReview)
	if !hasUsableReportGrant(reviewPermissions) {
		return ReportState{}, ErrForbidden
	}
	metadata.ActorID = accountID
	metadata.Reason = strings.TrimSpace(input.Comment)
	return repository.DecideReportCorrection(ctx, canonicalID, accountID, input, reviewPermissions, metadata)
}

func validateCorrectionValue(field string, raw json.RawMessage, factChanged bool) ([]string, error) {
	path, ok := ReportCorrectionFieldPath(field)
	if !ok || (field != "details.remark" && !factChanged) {
		return nil, ErrInvalidReportCorrection
	}
	stringLimits := map[string]int{
		"studentNumber": 32, "studentName": 64, "className": 96, "grade": 16,
		"currentDormitoryName": 128, "currentCourseName": 128, "currentClassroomName": 128,
		"currentAreaName": 128, "currentReasonLabel": 128, "details.date": 10, "details.time": 8,
		"details.period": 64, "details.round": 32, "details.result": 16, "details.reasonCode": 64,
		"details.description": 500, "details.remark": 500,
	}
	if limit, isString := stringLimits[field]; isString {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || len([]rune(strings.TrimSpace(value))) > limit {
			return nil, ErrInvalidReportCorrection
		}
		if field != "details.remark" && strings.TrimSpace(value) == "" {
			return nil, ErrInvalidReportCorrection
		}
		if field == "details.date" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return nil, ErrInvalidReportCorrection
			}
		}
		if field == "details.time" {
			if _, err := time.Parse("15:04", value); err != nil {
				if _, secondErr := time.Parse("15:04:05", value); secondErr != nil {
					return nil, ErrInvalidReportCorrection
				}
			}
		}
		if field == "details.result" && value != "EXCELLENT" && value != "QUALIFIED" && value != "UNQUALIFIED" {
			return nil, ErrInvalidReportCorrection
		}
		return path, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return nil, ErrInvalidReportCorrection
	}
	var trailingValue any
	if err := decoder.Decode(&trailingValue); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidReportCorrection
	}
	number, err := value.Float64()
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return nil, ErrInvalidReportCorrection
	}
	if field == "details.absenceCount" {
		integer, parseErr := strconv.Atoi(value.String())
		if parseErr != nil || integer < 1 {
			return nil, ErrInvalidReportCorrection
		}
	}
	return path, nil
}

func actionPermissions(permissions []permission.EffectivePermission, action string) []permission.EffectivePermission {
	result := make([]permission.EffectivePermission, 0)
	for _, item := range permissions {
		if item.Action == action {
			result = append(result, item)
		}
	}
	return result
}

func hasUsableReportGrant(permissions []permission.EffectivePermission) bool {
	for _, item := range permissions {
		if item.Effect == permission.EffectAllow && item.Scope.ClassID == "" && item.Scope.Grade == "" {
			return true
		}
	}
	return false
}

func reportScopeIsLimited(permissions []permission.EffectivePermission) bool {
	for _, item := range permissions {
		if item.Effect == permission.EffectDeny || item.Scope.AcademicYearID != "" || item.Scope.SemesterID != "" ||
			item.Scope.WeekID != "" || item.Scope.InspectionTypeID != "" || item.Scope.DepartmentID != "" ||
			item.Scope.ClassID != "" || item.Scope.Grade != "" {
			return true
		}
	}
	return false
}

func reportRecordDetails(details inspection.InspectionDetails) RecordDetails {
	switch item := details.(type) {
	case *inspection.AttendanceDetails:
		return courseAbsenceRecordDetails(item.CourseAbsenceDetails)
	case *inspection.UnattendedClassroomDetails:
		return courseAbsenceRecordDetails(item.CourseAbsenceDetails)
	case *inspection.EveningSelfStudyDetails:
		return RecordDetails{Date: item.Date, Time: item.Time, Period: item.Period, PenaltyCount: item.PenaltyCount, Remark: item.Remark}
	case *inspection.MorningAttendanceDetails:
		return RecordDetails{Date: item.Date, AbsenceCount: &item.AbsenceCount, Remark: item.Remark}
	case *inspection.LateReturnDetails:
		return RecordDetails{Date: item.Date, Time: item.Time, Remark: item.Remark}
	case *inspection.DormitoryHygieneDetails:
		return hygieneRecordDetails(item.HygieneDetails)
	case *inspection.ClassroomHygieneDetails:
		return hygieneRecordDetails(item.HygieneDetails)
	case *inspection.PublicDormitoryHygieneDetails:
		return hygieneRecordDetails(item.HygieneDetails)
	default:
		return RecordDetails{}
	}
}

func courseAbsenceRecordDetails(details inspection.CourseAbsenceDetails) RecordDetails {
	return RecordDetails{
		Date: details.Date, Time: details.Time, Period: details.Period,
		AbsenceHours: &details.AbsenceHours, PenaltyHours: details.PenaltyHours, Remark: details.Remark,
	}
}

func hygieneRecordDetails(details inspection.HygieneDetails) RecordDetails {
	return RecordDetails{
		Date: details.Date, Time: details.Time, Round: details.Round, Result: details.Result,
		ReasonCode: details.ReasonCode, Description: details.Description, Remark: details.Remark,
	}
}

func reportRecordReferences(options []inspection.EntryOption) []RecordReference {
	if len(options) == 0 {
		return nil
	}
	references := make([]RecordReference, 0, len(options))
	for _, option := range options {
		references = append(references, RecordReference{Label: option.Label, Grade: option.Grade})
	}
	return references
}

func (s *Service) reportPermissions(ctx context.Context, accountID string) ([]permission.EffectivePermission, error) {
	effective, err := s.permissions.Effective(ctx, accountID)
	if err != nil {
		return nil, err
	}
	result := make([]permission.EffectivePermission, 0)
	hasUsableGrant := false
	for _, item := range effective.Permissions {
		if item.Action != permission.ActionReportView {
			continue
		}
		result = append(result, item)
		if item.Effect == permission.EffectAllow && item.Scope.ClassID == "" && item.Scope.Grade == "" {
			hasUsableGrant = true
		}
	}
	if !hasUsableGrant {
		return nil, ErrForbidden
	}
	return result, nil
}
