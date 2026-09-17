package dashboard

import (
	"context"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

const (
	StatusNotEntered    = "NOT_ENTERED"
	StatusEntering      = "ENTERING"
	StatusPendingReview = "PENDING_REVIEW"
	StatusReturned      = "RETURNED"
	StatusCompleted     = "COMPLETED"
)

type Actor struct {
	ID           string
	IsSuperAdmin bool
}

type Scope struct {
	DepartmentID     string
	InspectionTypeID string
}

type AggregateQuery struct {
	AccountID  string
	SemesterID string
	WeekID     string
	Scopes     []Scope
}

type DataScope struct {
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
}

type PersonalTodoQuery struct {
	AccountID    string
	SemesterID   string
	WeekID       string
	Unrestricted bool
	Allowed      []DataScope
	Denied       []DataScope
}

type ScopeCounts struct {
	DepartmentID     string
	InspectionTypeID string
	Total            int
	Draft            int
	PendingReview    int
	Returned         int
	Confirmed        int
	Locked           int
}

type AcademicContext struct {
	AcademicYearID   string    `json:"academicYearId,omitempty"`
	AcademicYearName string    `json:"academicYearName,omitempty"`
	SemesterID       string    `json:"semesterId"`
	SemesterName     string    `json:"semesterName,omitempty"`
	WeekID           string    `json:"weekId"`
	WeekNo           int       `json:"weekNo"`
	StartDate        time.Time `json:"startDate"`
	EndDate          time.Time `json:"endDate"`
}

type MyTodos struct {
	Total    int `json:"total"`
	Draft    int `json:"draft"`
	Returned int `json:"returned"`
}

type DepartmentStatus struct {
	DepartmentID       string `json:"departmentId"`
	DepartmentName     string `json:"departmentName"`
	InspectionTypeID   string `json:"inspectionTypeId"`
	InspectionTypeCode string `json:"inspectionTypeCode"`
	InspectionTypeName string `json:"inspectionTypeName"`
	Status             string `json:"status"`
	Total              int    `json:"total"`
	Draft              int    `json:"draft"`
	PendingReview      int    `json:"pendingReview"`
	Returned           int    `json:"returned"`
	Confirmed          int    `json:"confirmed"`
	Locked             int    `json:"locked"`
	CanEnter           bool   `json:"canEnter"`
	CanReview          bool   `json:"canReview"`
}

type Workbench struct {
	Context            *AcademicContext   `json:"context"`
	MyTodos            MyTodos            `json:"myTodos"`
	DepartmentStatuses []DepartmentStatus `json:"departmentStatuses"`
}

type Repository interface {
	Aggregate(context.Context, AggregateQuery) ([]ScopeCounts, error)
	PersonalTodos(context.Context, PersonalTodoQuery) (MyTodos, error)
}

type academicService interface {
	CurrentWeek(context.Context, time.Time) (*academic.Week, error)
	List(context.Context) (academic.Calendar, error)
}

type organizationService interface {
	ListCatalogs(context.Context, bool) (organization.Catalogs, error)
	ListScopes(context.Context) ([]organization.DepartmentInspectionScope, error)
}

type permissionService interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}

type Service struct {
	repository   Repository
	academic     academicService
	organization organizationService
	permission   permissionService
}

func NewService(repository Repository, academic academicService, organization organizationService, permission permissionService) *Service {
	return &Service{repository: repository, academic: academic, organization: organization, permission: permission}
}

func (s *Service) Workbench(ctx context.Context, actor Actor, at time.Time) (Workbench, error) {
	result := Workbench{DepartmentStatuses: make([]DepartmentStatus, 0)}
	week, err := s.academic.CurrentWeek(ctx, at)
	if err != nil || week == nil {
		return result, err
	}
	calendar, err := s.academic.List(ctx)
	if err != nil {
		return Workbench{}, err
	}
	result.Context = currentContext(*week, calendar)
	catalogs, err := s.organization.ListCatalogs(ctx, true)
	if err != nil {
		return Workbench{}, err
	}
	mappings, err := s.organization.ListScopes(ctx)
	if err != nil {
		return Workbench{}, err
	}
	effective := permission.EffectiveResult{}
	if !actor.IsSuperAdmin {
		effective, err = s.permission.Effective(ctx, actor.ID)
		if err != nil {
			return Workbench{}, err
		}
	}
	result.MyTodos, err = s.repository.PersonalTodos(ctx, personalTodoQuery(actor, effective, *result.Context))
	if err != nil {
		return Workbench{}, err
	}

	departments := make(map[string]organization.Department, len(catalogs.Departments))
	for _, value := range catalogs.Departments {
		departments[value.ID] = value
	}
	types := make(map[string]organization.InspectionType, len(catalogs.InspectionTypes))
	for _, value := range catalogs.InspectionTypes {
		types[value.ID] = value
	}

	authorized := make([]Scope, 0)
	capabilities := make(map[Scope]DepartmentStatus)
	for _, mapping := range mappings {
		department, departmentExists := departments[mapping.DepartmentID]
		inspectionType, typeExists := types[mapping.InspectionTypeID]
		if !mapping.Enabled || !mapping.CanView || !departmentExists || !typeExists {
			continue
		}
		requested := permission.Scope{SemesterID: week.SemesterID, WeekID: week.ID, DepartmentID: mapping.DepartmentID, InspectionTypeID: mapping.InspectionTypeID}
		if !actor.IsSuperAdmin && !effective.Allowed(permission.ActionInspectionView, requested) {
			continue
		}
		scope := Scope{DepartmentID: mapping.DepartmentID, InspectionTypeID: mapping.InspectionTypeID}
		authorized = append(authorized, scope)
		capabilities[scope] = DepartmentStatus{
			DepartmentID: mapping.DepartmentID, DepartmentName: department.Name,
			InspectionTypeID: mapping.InspectionTypeID, InspectionTypeCode: inspectionType.Code, InspectionTypeName: inspectionType.Name,
			CanEnter:  actor.IsSuperAdmin || effective.Allowed(permission.ActionInspectionEnter, requested),
			CanReview: actor.IsSuperAdmin || effective.Allowed(permission.ActionInspectionReview, requested),
		}
	}

	counts, err := s.repository.Aggregate(ctx, AggregateQuery{AccountID: actor.ID, SemesterID: week.SemesterID, WeekID: week.ID, Scopes: authorized})
	if err != nil {
		return Workbench{}, err
	}
	countByScope := make(map[Scope]ScopeCounts, len(counts))
	for _, value := range counts {
		countByScope[Scope{DepartmentID: value.DepartmentID, InspectionTypeID: value.InspectionTypeID}] = value
	}
	for _, scope := range authorized {
		value := capabilities[scope]
		count := countByScope[scope]
		value.Status = deriveStatus(count)
		value.Total, value.Draft, value.PendingReview = count.Total, count.Draft, count.PendingReview
		value.Returned, value.Confirmed, value.Locked = count.Returned, count.Confirmed, count.Locked
		result.DepartmentStatuses = append(result.DepartmentStatuses, value)
	}
	return result, nil
}

func personalTodoQuery(actor Actor, effective permission.EffectiveResult, current AcademicContext) PersonalTodoQuery {
	query := PersonalTodoQuery{AccountID: actor.ID, SemesterID: current.SemesterID, WeekID: current.WeekID, Unrestricted: actor.IsSuperAdmin, Allowed: make([]DataScope, 0), Denied: make([]DataScope, 0)}
	for _, value := range effective.Permissions {
		if value.Action != permission.ActionInspectionView || !permissionContextMatches(value.Scope, current) {
			continue
		}
		scope := DataScope{InspectionTypeID: value.Scope.InspectionTypeID, DepartmentID: value.Scope.DepartmentID, ClassID: value.Scope.ClassID, Grade: value.Scope.Grade}
		if value.Effect == permission.EffectAllow {
			query.Allowed = append(query.Allowed, scope)
		} else if value.Effect == permission.EffectDeny {
			query.Denied = append(query.Denied, scope)
		}
	}
	return query
}

func permissionContextMatches(scope permission.Scope, current AcademicContext) bool {
	return (scope.AcademicYearID == "" || scope.AcademicYearID == current.AcademicYearID) &&
		(scope.SemesterID == "" || scope.SemesterID == current.SemesterID) &&
		(scope.WeekID == "" || scope.WeekID == current.WeekID)
}

func currentContext(week academic.Week, calendar academic.Calendar) *AcademicContext {
	result := &AcademicContext{SemesterID: week.SemesterID, WeekID: week.ID, WeekNo: week.WeekNo, StartDate: week.StartDate, EndDate: week.EndDate}
	for _, semester := range calendar.Semesters {
		if semester.ID == week.SemesterID {
			result.SemesterName, result.AcademicYearID = semester.Name, semester.AcademicYearID
			break
		}
	}
	for _, year := range calendar.AcademicYears {
		if year.ID == result.AcademicYearID {
			result.AcademicYearName = year.Name
			break
		}
	}
	return result
}

func deriveStatus(counts ScopeCounts) string {
	if counts.Total == 0 {
		return StatusNotEntered
	}
	if counts.Confirmed+counts.Locked == counts.Total {
		return StatusCompleted
	}
	if counts.Returned > 0 {
		return StatusReturned
	}
	if counts.PendingReview > 0 {
		return StatusPendingReview
	}
	return StatusEntering
}
