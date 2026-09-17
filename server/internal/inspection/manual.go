package inspection

import (
	"context"
	"errors"
	"strings"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

// ErrAttendanceDuplicate also represents a concurrent duplicate rejected by the database.
var ErrAttendanceDuplicate = errors.New("attendance record already exists")

// ManualAttendanceInput carries business fields, never trusted snapshots or status.
type ManualAttendanceInput struct {
	ExcludeRecordID  string  `json:"-"`
	SemesterID       string  `json:"semesterId"`
	WeekID           string  `json:"weekId"`
	InspectionTypeID string  `json:"inspectionTypeId"`
	DepartmentID     string  `json:"departmentId"`
	ClassID          string  `json:"classId"`
	Grade            string  `json:"grade"`
	ClassName        string  `json:"className"`
	StudentNo        string  `json:"studentNo"`
	StudentName      string  `json:"studentName"`
	CourseName       string  `json:"courseName"`
	Classroom        string  `json:"classroom"`
	Date             string  `json:"date"`
	Time             string  `json:"time"`
	Period           string  `json:"period"`
	AbsenceHours     float64 `json:"absenceHours"`
	Remark           string  `json:"remark"`
}

// ManualActor is supplied by authentication, never decoded from the request body.
type ManualActor struct {
	ID           string
	IsSuperAdmin bool
}

// ManualValidationError exposes safe field validation feedback.
type ManualValidationError struct{ Message string }

func (e *ManualValidationError) Error() string { return e.Message }

type manualRepository interface {
	SaveManualAttendance(context.Context, AttendanceRecord, audit.Mutation) (AttendanceRecord, error)
}
type manualValidator interface {
	ValidateManualAttendance(context.Context, ManualAttendanceInput, academic.Week, string) (AttendanceRecord, error)
}
type manualCalendar interface {
	List(context.Context) (academic.Calendar, error)
}
type manualOrganization interface {
	ListCatalogs(context.Context, bool) (organization.Catalogs, error)
	ListScopes(context.Context) ([]organization.DepartmentInspectionScope, error)
}
type manualPermissions interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}

// ManualService orchestrates reference validation, authorization and audited creation.
type ManualService struct {
	repository   manualRepository
	validator    manualValidator
	calendar     manualCalendar
	organization manualOrganization
	permissions  manualPermissions
}

// NewManualService reuses reference validation while keeping creation in the inspection domain.
func NewManualService(r manualRepository, v manualValidator, c manualCalendar, o manualOrganization, p manualPermissions) *ManualService {
	return &ManualService{r, v, c, o, p}
}

// Create authorizes the requested and resolved scope before persisting an audited manual draft.
func (s *ManualService) Create(ctx context.Context, actor ManualActor, input ManualAttendanceInput, metadata audit.Metadata) (AttendanceRecord, error) {
	input.ExcludeRecordID = ""
	record, err := s.prepare(ctx, actor, input, "")
	if err != nil {
		return AttendanceRecord{}, err
	}
	record.Source, record.Status, record.CreatedBy, record.ImportBatchID = RecordSourceManual, RecordStatusDraft, actor.ID, ""
	record.RowVersion = 1
	metadata.ActorID = actor.ID
	mutation := audit.Mutation{Metadata: metadata, Entry: audit.Entry{Action: audit.ActionInspectionRecordCreated, ResourceType: "inspectionRecord", ResourceID: record.ID, After: map[string]any{"source": record.Source, "status": record.Status, "semesterId": input.SemesterID, "weekId": input.WeekID, "departmentId": input.DepartmentID}}}
	return s.repository.SaveManualAttendance(ctx, record, mutation)
}

func (s *ManualService) prepare(ctx context.Context, actor ManualActor, input ManualAttendanceInput, ownerID string) (AttendanceRecord, error) {
	for _, value := range []string{actor.ID, input.SemesterID, input.WeekID, input.InspectionTypeID, input.DepartmentID, input.ClassID, input.Grade} {
		if strings.TrimSpace(value) == "" {
			return AttendanceRecord{}, ErrInvalidAttendanceRecord
		}
	}
	calendar, err := s.calendar.List(ctx)
	if err != nil {
		return AttendanceRecord{}, err
	}
	var week academic.Week
	var yearID string
	for _, w := range calendar.Weeks {
		if w.ID == input.WeekID && w.SemesterID == input.SemesterID && w.Enabled {
			week = w
		}
	}
	for _, semester := range calendar.Semesters {
		if semester.ID == input.SemesterID && semester.Enabled {
			yearID = semester.AcademicYearID
		}
	}
	yearEnabled := false
	for _, year := range calendar.AcademicYears {
		if year.ID == yearID && year.Enabled {
			yearEnabled = true
		}
	}
	if week.ID == "" || !yearEnabled {
		return AttendanceRecord{}, ErrInvalidAttendanceRecord
	}
	scope := permission.Scope{AcademicYearID: yearID, SemesterID: input.SemesterID, WeekID: input.WeekID, InspectionTypeID: input.InspectionTypeID, DepartmentID: input.DepartmentID, ClassID: input.ClassID, Grade: input.Grade}
	effective := permission.EffectiveResult{}
	if !actor.IsSuperAdmin {
		effective, err = s.permissions.Effective(ctx, actor.ID)
		if err != nil {
			return AttendanceRecord{}, err
		}
		if !canPrepareAttendance(actor, effective, scope, ownerID) {
			return AttendanceRecord{}, ErrScopeMismatch
		}
	}
	catalogs, err := s.organization.ListCatalogs(ctx, true)
	if err != nil {
		return AttendanceRecord{}, err
	}
	validType, validDepartment := false, false
	for _, v := range catalogs.InspectionTypes {
		if v.ID == input.InspectionTypeID && v.Code == InspectionTypeAttendance && v.Enabled {
			validType = true
		}
	}
	for _, v := range catalogs.Departments {
		if v.ID == input.DepartmentID && v.Enabled {
			validDepartment = true
		}
	}
	mappings, err := s.organization.ListScopes(ctx)
	if err != nil {
		return AttendanceRecord{}, err
	}
	mapped := false
	for _, v := range mappings {
		if v.DepartmentID == input.DepartmentID && v.InspectionTypeID == input.InspectionTypeID && v.Enabled && v.CanEnter {
			mapped = true
		}
	}
	if !validType || !validDepartment || !mapped {
		return AttendanceRecord{}, ErrScopeMismatch
	}
	record, err := s.validator.ValidateManualAttendance(ctx, input, week, actor.ID)
	if err != nil {
		return AttendanceRecord{}, err
	}
	// Recheck authoritative student/class metadata rather than trusting requested grade.
	scope.ClassID, scope.Grade = record.ClassID, record.GradeSnapshot
	if !actor.IsSuperAdmin && !canPrepareAttendance(actor, effective, scope, ownerID) {
		return AttendanceRecord{}, ErrScopeMismatch
	}
	return record, nil
}

func canPrepareAttendance(actor ManualActor, effective permission.EffectiveResult, scope permission.Scope, ownerID string) bool {
	if actor.IsSuperAdmin {
		return true
	}
	if ownerID == "" {
		return effective.Allowed(permission.ActionInspectionEnter, scope)
	}
	return (actor.ID == ownerID && effective.Allowed(permission.ActionInspectionEnter, scope)) || effective.Allowed(permission.ActionInspectionManage, scope)
}
