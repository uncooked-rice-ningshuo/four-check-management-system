package inspection

import (
	"context"
	"strings"
	"unicode/utf8"
)

// EntryOptionQuery selects references within an explicit destination scope.
// RecordID enables draft editing options using the original record's ownership.
type EntryOptionQuery struct {
	SemesterID       string `form:"semesterId"`
	WeekID           string `form:"weekId"`
	InspectionTypeID string `form:"inspectionTypeId"`
	DepartmentID     string `form:"departmentId"`
	RecordID         string `form:"recordId"`
	ClassID          string `form:"classId"`
	StudentID        string `form:"studentId"`
	CourseID         string `form:"courseId"`
	ClassroomID      string `form:"classroomId"`
	AreaID           string `form:"areaId"`
	Keyword          string `form:"keyword"`
	Kind             string `form:"kind"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
}

// EntryOption contains only the identifier and human-readable selection label.
type EntryOption struct {
	ID                          string `json:"id"`
	Label                       string `json:"label"`
	AttendancePenaltyMultiplier *int   `json:"attendancePenaltyMultiplier,omitempty"`
	// Grade is current class metadata, never an authorization input.
	Grade string `json:"grade,omitempty"`
}
type EntryOptionPage struct {
	Items    []EntryOption `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

type EntryOptionRepository interface {
	ListEntryOptions(context.Context, EntryOptionQuery, MutationAccess) (EntryOptionPage, error)
}

// EntryOptionService keeps entry references separate from administrative master-data access.
type EntryOptionService struct {
	records    *RecordService
	repository EntryOptionRepository
}

func NewEntryOptionService(r EntryOptionRepository, p manualPermissions) *EntryOptionService {
	return &EntryOptionService{records: NewRecordService(nil, p), repository: r}
}

func (s *EntryOptionService) EntryOptions(ctx context.Context, actor ManualActor, q EntryOptionQuery) (EntryOptionPage, error) {
	q.Keyword = strings.TrimSpace(q.Keyword)
	if utf8.RuneCountInString(q.Keyword) > 128 || (q.Keyword != "" && q.Kind != "courses") {
		return EntryOptionPage{}, ErrInspectionRequest
	}
	if actor.IsMember {
		departmentID, err := ResolveEntryDepartment(actor, q.DepartmentID)
		if err != nil {
			return EntryOptionPage{}, err
		}
		q.DepartmentID = departmentID
	}
	input := InspectionInput{SemesterID: q.SemesterID, WeekID: q.WeekID, InspectionTypeID: q.InspectionTypeID, DepartmentID: q.DepartmentID}
	if normalizeInspectionInput(&input) != nil {
		return EntryOptionPage{}, ErrInspectionRequest
	}
	q.SemesterID, q.WeekID, q.InspectionTypeID, q.DepartmentID = input.SemesterID, input.WeekID, input.InspectionTypeID, input.DepartmentID
	for _, id := range []*string{&q.ClassID, &q.CourseID, &q.StudentID, &q.ClassroomID, &q.AreaID, &q.RecordID} {
		if *id != "" {
			v, ok := canonicalDetailID(*id)
			if !ok {
				return EntryOptionPage{}, ErrInspectionRequest
			}
			*id = v
		}
	}
	switch q.Kind {
	case "classes":
	case "students":
		if q.ClassID == "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	case "courses", "classrooms":
	case "dormitories":
		if q.ClassID == "" || q.StudentID == "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	case "dormitory-objects", "reason-codes":
		if q.ClassID == "" || q.StudentID != "" || q.CourseID != "" || q.ClassroomID != "" || q.AreaID != "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	case "classroom-objects", "public-areas":
		if q.ClassID == "" || q.StudentID != "" || q.CourseID != "" || q.ClassroomID != "" || q.AreaID != "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	case "classroom-areas":
		if q.ClassID == "" || q.ClassroomID == "" || q.StudentID != "" || q.CourseID != "" || q.AreaID != "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	case "responsible-dormitories":
		if q.ClassID == "" || q.AreaID == "" || q.StudentID != "" || q.CourseID != "" || q.ClassroomID != "" {
			return EntryOptionPage{}, ErrInspectionRequest
		}
	default:
		return EntryOptionPage{}, ErrInspectionRequest
	}
	if q.Page < 0 || q.PageSize < 0 || q.PageSize > 200 || (q.Kind == "courses" && q.PageSize > 50) || q.Page > 1000000 {
		return EntryOptionPage{}, ErrInspectionRequest
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	a, err := s.records.access(ctx, actor)
	if err != nil {
		return EntryOptionPage{}, err
	}
	return s.repository.ListEntryOptions(ctx, q, a)
}
