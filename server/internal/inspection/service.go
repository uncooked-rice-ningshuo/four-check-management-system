package inspection

import (
	"context"
	"strings"
)

type Repository interface {
	ListAttendance(context.Context, AttendanceListQuery) (AttendancePage, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// ListAttendance intersects requested filters with an already-compiled scope before reaching persistence.
func (s *Service) ListAttendance(ctx context.Context, filter AttendanceListFilter, scope AttendanceScope) (AttendancePage, error) {
	if strings.TrimSpace(filter.SemesterID) == "" || strings.TrimSpace(filter.WeekID) == "" || strings.TrimSpace(filter.InspectionTypeID) == "" {
		return AttendancePage{}, ErrInvalidAttendanceQuery
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 20
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return AttendancePage{}, ErrInvalidAttendanceQuery
	}
	if filter.Sort == "" {
		filter.Sort = SortOccurredAtDescending
	}
	if !validAttendanceSort(filter.Sort) {
		return AttendancePage{}, ErrInvalidAttendanceQuery
	}

	semesterID, ok := intersectScopeValue(filter.SemesterID, scope.SemesterID)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}
	weekID, ok := intersectScopeValue(filter.WeekID, scope.WeekID)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}
	inspectionTypeID, ok := intersectScopeValue(filter.InspectionTypeID, scope.InspectionTypeID)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}
	departmentID, ok := intersectScopeValue(filter.DepartmentID, scope.DepartmentID)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}
	classID, ok := intersectScopeValue(filter.ClassID, scope.ClassID)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}
	grade, ok := intersectScopeValue(filter.Grade, scope.Grade)
	if !ok {
		return AttendancePage{}, ErrScopeMismatch
	}

	page, err := s.repository.ListAttendance(ctx, AttendanceListQuery{
		SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID,
		DepartmentID: departmentID, ClassID: classID, Grade: grade,
		Keyword: strings.TrimSpace(filter.Keyword), Sort: filter.Sort, Page: filter.Page, PageSize: filter.PageSize,
	})
	if err != nil {
		return AttendancePage{}, err
	}
	if page.Items == nil {
		page.Items = make([]AttendanceRecord, 0)
	}
	return page, nil
}

func intersectScopeValue(requested, allowed string) (string, bool) {
	requested = strings.TrimSpace(requested)
	allowed = strings.TrimSpace(allowed)
	if requested != "" && allowed != "" && requested != allowed {
		return "", false
	}
	if allowed != "" {
		return allowed, true
	}
	return requested, true
}

func validAttendanceSort(sort string) bool {
	switch sort {
	case SortOccurredAtDescending, SortOccurredAtAscending, SortStudentNoAscending, SortStudentNoDescending:
		return true
	default:
		return false
	}
}
