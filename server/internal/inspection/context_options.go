package inspection

import (
	"context"
	"strings"
	"unicode/utf8"
)

// ContextOptionQuery separates list visibility from creation targets.
type ContextOptionQuery struct {
	CollegeOnly  bool   `form:"-"`
	RecordID     string `form:"recordId"`
	TypeCode     string `form:"typeCode"`
	Purpose      string `form:"purpose"`
	Kind         string `form:"kind"`
	SemesterID   string `form:"semesterId"`
	WeekID       string `form:"weekId"`
	DepartmentID string `form:"departmentId"`
	Grade        string `form:"grade"`
	Page         int    `form:"page"`
	PageSize     int    `form:"pageSize"`
}

// ContextOptionPage also resolves the configured inspection type identifier.
type ContextOptionPage struct {
	EntryOptionPage
	InspectionTypeID string `json:"inspectionTypeId"`
}

// ContextOptionRepository filters context choices in the authoritative data store.
type ContextOptionRepository interface {
	ListContextOptions(context.Context, ContextOptionQuery, MutationAccess) (ContextOptionPage, error)
}

// ContextOptionService validates bounded context queries before resolving actor access.
type ContextOptionService struct {
	records    *RecordService
	repository ContextOptionRepository
}

// NewContextOptionService separates business options from administrative catalogs.
func NewContextOptionService(r ContextOptionRepository, p manualPermissions) *ContextOptionService {
	return &ContextOptionService{NewRecordService(nil, p), r}
}
func (s *ContextOptionService) ContextOptions(ctx context.Context, a ManualActor, q ContextOptionQuery) (ContextOptionPage, error) {
	if q.TypeCode != InspectionTypeAttendance && q.TypeCode != InspectionTypeUnattended && q.TypeCode != InspectionTypeLateReturn && q.TypeCode != InspectionTypeMorning && q.TypeCode != InspectionTypeDormitoryHygiene && q.TypeCode != InspectionTypeClassroomHygiene && q.TypeCode != InspectionTypePublicDormitoryHygiene {
		return ContextOptionPage{}, ErrInspectionRequest
	}
	if q.Purpose != "view" && q.Purpose != "entry" {
		return ContextOptionPage{}, ErrInspectionRequest
	}
	for _, id := range []*string{&q.SemesterID, &q.WeekID, &q.DepartmentID, &q.RecordID} {
		if *id != "" {
			v, ok := canonicalDetailID(*id)
			if !ok {
				return ContextOptionPage{}, ErrInspectionRequest
			}
			*id = v
		}
	}
	if q.DepartmentID == CollegeDepartmentOptionID {
		if a.IsMember && q.Purpose == "entry" {
			return ContextOptionPage{}, ErrScopeMismatch
		}
		q.DepartmentID = ""
		q.CollegeOnly = true
	}
	if a.IsMember && q.Purpose == "entry" {
		primary, ok := canonicalDetailID(a.PrimaryDepartmentID)
		if !ok || (q.DepartmentID != "" && q.DepartmentID != primary) {
			return ContextOptionPage{}, ErrScopeMismatch
		}
		q.DepartmentID = primary
	}
	if q.RecordID != "" && q.Purpose != "entry" {
		return ContextOptionPage{}, ErrInspectionRequest
	}
	q.Grade = strings.TrimSpace(q.Grade)
	if utf8.RuneCountInString(q.Grade) > 64 {
		return ContextOptionPage{}, ErrInspectionRequest
	}
	switch q.Kind {
	case "semesters":
	case "weeks":
		if q.SemesterID == "" {
			return ContextOptionPage{}, ErrInspectionRequest
		}
	case "departments", "classes", "grades":
		if q.Purpose == "entry" && (q.SemesterID == "" || q.WeekID == "") {
			return ContextOptionPage{}, ErrInspectionRequest
		}
	default:
		return ContextOptionPage{}, ErrInspectionRequest
	}
	if q.Page < 0 || q.Page > 1000000 || q.PageSize < 0 || q.PageSize > 200 {
		return ContextOptionPage{}, ErrInspectionRequest
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	access, err := s.records.access(ctx, a)
	if err != nil {
		return ContextOptionPage{}, err
	}
	return s.repository.ListContextOptions(ctx, q, access)
}
