package reporting

import (
	"context"
	"errors"
	"strings"

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
)

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
		page.Items = append(page.Items, Record{
			ID: record.ID, StudentNumber: record.StudentNumber, StudentName: record.StudentName,
			ClassName: record.ClassName, Grade: record.Grade, TypeCode: record.TypeCode,
			Details:              reportRecordDetails(record.Details),
			CurrentDormitoryName: record.CurrentDormitoryName, CurrentCourseName: record.CurrentCourseName,
			CurrentClassroomName: record.CurrentClassroomName, CurrentAreaName: record.CurrentAreaName,
			CurrentReasonLabel:     record.CurrentReasonLabel,
			ResponsibleClasses:     reportRecordReferences(record.ResponsibleClasses),
			ResponsibleStudents:    reportRecordReferences(record.ResponsibleStudents),
			ResponsibleDormitories: reportRecordReferences(record.ResponsibleDormitories),
		})
	}
	return page, nil
}

func reportRecordDetails(details inspection.InspectionDetails) RecordDetails {
	switch item := details.(type) {
	case *inspection.AttendanceDetails:
		return courseAbsenceRecordDetails(item.CourseAbsenceDetails)
	case *inspection.UnattendedClassroomDetails:
		return courseAbsenceRecordDetails(item.CourseAbsenceDetails)
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
