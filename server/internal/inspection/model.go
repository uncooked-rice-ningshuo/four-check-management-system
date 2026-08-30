package inspection

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	InspectionTypeAttendance = "ATTENDANCE"

	RecordStatusDraft = "DRAFT"
	RecordSourceExcel = "EXCEL"

	SortOccurredAtDescending = "occurredAtDesc"
	SortOccurredAtAscending  = "occurredAtAsc"
	SortStudentNoAscending   = "studentNoAsc"
	SortStudentNoDescending  = "studentNoDesc"
)

var (
	ErrInvalidAttendanceRecord = errors.New("invalid attendance record")
	ErrOccurredOutsideWeek     = errors.New("attendance occurrence is outside week")
	ErrInspectionTypeMismatch  = errors.New("inspection type is not attendance")
	ErrInvalidAttendanceQuery  = errors.New("invalid attendance query")
	ErrScopeMismatch           = errors.New("attendance query is outside compiled scope")
)

// AttendanceInput contains validated reference snapshots and week boundaries used to create an attendance draft.
type AttendanceInput struct {
	InspectionTypeID    string
	InspectionTypeCode  string
	SemesterID          string
	WeekID              string
	DepartmentID        string
	StudentID           string
	StudentNoSnapshot   string
	StudentNameSnapshot string
	ClassID             string
	ClassNameSnapshot   string
	GradeSnapshot       string
	CourseID            string
	ClassroomID         string
	OccurredAt          time.Time
	WeekStart           time.Time
	WeekEnd             time.Time
	AbsenceHours        float64
	PeriodText          string
	Remark              string
	ImportBatchID       string
	CreatedBy           string
}

// AttendanceRecord combines the common inspection record with attendance-specific fields.
type AttendanceRecord struct {
	ID                  string    `json:"id"`
	InspectionTypeID    string    `json:"inspectionTypeId"`
	SemesterID          string    `json:"semesterId"`
	WeekID              string    `json:"weekId"`
	DepartmentID        string    `json:"departmentId"`
	StudentID           string    `json:"studentId"`
	StudentNoSnapshot   string    `json:"studentNo"`
	StudentNameSnapshot string    `json:"studentName"`
	ClassID             string    `json:"classId"`
	ClassNameSnapshot   string    `json:"className"`
	GradeSnapshot       string    `json:"grade"`
	CourseID            string    `json:"courseId"`
	CourseName          string    `json:"courseName"`
	ClassroomID         string    `json:"classroomId,omitempty"`
	ClassroomCode       string    `json:"classroomCode,omitempty"`
	OccurredAt          time.Time `json:"occurredAt"`
	AbsenceHours        float64   `json:"absenceHours"`
	PeriodText          string    `json:"period"`
	Remark              string    `json:"remark"`
	Source              string    `json:"source"`
	Status              string    `json:"status"`
	ImportBatchID       string    `json:"importBatchId,omitempty"`
	DedupKey            string    `json:"-"`
	RowVersion          int       `json:"rowVersion"`
	CreatedBy           string    `json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// NewAttendanceRecord enforces the invariants shared by manual and imported attendance drafts.
func NewAttendanceRecord(input AttendanceInput) (AttendanceRecord, error) {
	if strings.TrimSpace(input.InspectionTypeCode) != InspectionTypeAttendance {
		return AttendanceRecord{}, ErrInspectionTypeMismatch
	}
	if missingAttendanceContext(input) || input.OccurredAt.IsZero() || input.WeekStart.IsZero() || input.WeekEnd.IsZero() ||
		math.IsNaN(input.AbsenceHours) || math.IsInf(input.AbsenceHours, 0) || input.AbsenceHours <= 0 ||
		strings.TrimSpace(input.PeriodText) == "" {
		return AttendanceRecord{}, ErrInvalidAttendanceRecord
	}
	weekStart, weekEnd, occurredDate := attendanceBusinessDates(input)
	if weekStart.After(weekEnd) {
		return AttendanceRecord{}, ErrInvalidAttendanceRecord
	}
	if occurredDate.Before(weekStart) || occurredDate.After(weekEnd) {
		return AttendanceRecord{}, ErrOccurredOutsideWeek
	}

	input.StudentNoSnapshot = strings.TrimSpace(input.StudentNoSnapshot)
	input.StudentNameSnapshot = strings.TrimSpace(input.StudentNameSnapshot)
	input.ClassNameSnapshot = strings.TrimSpace(input.ClassNameSnapshot)
	input.GradeSnapshot = strings.TrimSpace(input.GradeSnapshot)
	input.PeriodText = strings.TrimSpace(input.PeriodText)
	input.Remark = strings.TrimSpace(input.Remark)
	return AttendanceRecord{
		ID: uuid.NewString(), InspectionTypeID: input.InspectionTypeID, SemesterID: input.SemesterID, WeekID: input.WeekID,
		DepartmentID: input.DepartmentID, StudentID: input.StudentID, StudentNoSnapshot: input.StudentNoSnapshot,
		StudentNameSnapshot: input.StudentNameSnapshot, ClassID: input.ClassID, ClassNameSnapshot: input.ClassNameSnapshot,
		GradeSnapshot: input.GradeSnapshot, CourseID: input.CourseID, ClassroomID: input.ClassroomID,
		OccurredAt: input.OccurredAt, AbsenceHours: input.AbsenceHours, PeriodText: input.PeriodText, Remark: input.Remark,
		Source: RecordSourceExcel, Status: RecordStatusDraft, ImportBatchID: input.ImportBatchID,
		DedupKey: attendanceDedupKey(input), RowVersion: 1, CreatedBy: input.CreatedBy,
	}, nil
}

func attendanceBusinessDates(input AttendanceInput) (time.Time, time.Time, time.Time) {
	location := input.OccurredAt.Location()
	dateAtLocation := func(value time.Time) time.Time {
		localized := value.In(location)
		return time.Date(localized.Year(), localized.Month(), localized.Day(), 0, 0, 0, 0, location)
	}
	return dateAtLocation(input.WeekStart), dateAtLocation(input.WeekEnd), dateAtLocation(input.OccurredAt)
}

func missingAttendanceContext(input AttendanceInput) bool {
	values := []string{
		input.InspectionTypeID, input.SemesterID, input.WeekID, input.DepartmentID, input.StudentID,
		input.StudentNoSnapshot, input.StudentNameSnapshot, input.ClassID, input.ClassNameSnapshot,
		input.GradeSnapshot, input.CourseID, input.CreatedBy,
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func attendanceDedupKey(input AttendanceInput) string {
	canonical := strings.Join([]string{
		input.SemesterID, input.WeekID, input.InspectionTypeID, input.StudentID, input.CourseID,
		input.OccurredAt.UTC().Format(time.RFC3339Nano), strings.TrimSpace(input.PeriodText),
	}, "\x1f")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

type AttendanceListFilter struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
	Keyword          string
	Sort             string
	Page             int
	PageSize         int
}

// AttendanceScope is the already-compiled effective data scope for one list request.
// Empty values mean the corresponding dimension is unrestricted.
type AttendanceScope struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
}

type AttendanceListQuery struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
	Keyword          string
	Sort             string
	Page             int
	PageSize         int
}

type AttendancePage struct {
	Items    []AttendanceRecord `json:"items"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}
