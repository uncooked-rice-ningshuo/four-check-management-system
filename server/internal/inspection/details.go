package inspection

import "time"

const (
	AttributionModeDepartment            = "DEPARTMENT"
	AttributionModeCollege               = "COLLEGE"
	AttributionModeAdministrator         = "ADMINISTRATOR"
	InspectionTypeUnattended             = "UNATTENDED_CLASSROOM"
	InspectionTypeMorning                = "MORNING_ATTENDANCE"
	InspectionTypeLateReturn             = "LATE_RETURN"
	InspectionTypeDormitoryHygiene       = "DORMITORY_HYGIENE"
	InspectionTypeClassroomHygiene       = "CLASSROOM_HYGIENE"
	InspectionTypePublicDormitoryHygiene = "PUBLIC_DORMITORY_HYGIENE"
	InspectionTypeEveningSelfStudy       = "EVENING_SELF_STUDY"
	OccurrenceTime                       = "TIME"
	OccurrenceDate                       = "DATE"
	OccurrenceRound                      = "ROUND"
	HygieneExcellent                     = "EXCELLENT"
	HygieneQualified                     = "QUALIFIED"
	HygieneUnqualified                   = "UNQUALIFIED"
)

// InspectionDetails is a closed set of typed payloads; raw maps never reach repositories.
type InspectionDetails interface{ inspectionDetails() }

// CourseAbsenceDetails contains fields shared by the two course absence models.
type CourseAbsenceDetails struct {
	StudentID         string   `json:"studentId"`
	ClassID           string   `json:"classId"`
	CourseID          string   `json:"courseId"`
	ClassroomID       string   `json:"classroomId"`
	Date              string   `json:"date"`
	Time              string   `json:"time,omitempty"`
	Period            string   `json:"period"`
	AbsenceHours      float64  `json:"absenceHours"`
	PenaltyMultiplier *int     `json:"penaltyMultiplier,omitempty"`
	PenaltyHours      *float64 `json:"penaltyHours,omitempty"`
	Remark            string   `json:"remark,omitempty"`
}

// AttendanceDetails and UnattendedClassroomDetails share storage but retain distinct domain types.
type AttendanceDetails struct{ CourseAbsenceDetails }

func (*AttendanceDetails) inspectionDetails() {}

// UnattendedClassroomDetails records one violation per day. The week multiplier is a
// server-owned snapshot: penalty_count is what reports and future discipline statistics read.
type UnattendedClassroomDetails struct {
	CourseAbsenceDetails
	PenaltyCount *int `json:"penaltyCount,omitempty"`
}

func (*UnattendedClassroomDetails) inspectionDetails() {}

// EveningSelfStudyDetails records only students absent during the fixed ninth and tenth periods.
// Penalty fields are server-derived and never accepted from client payloads.
type EveningSelfStudyDetails struct {
	StudentID         string   `json:"studentId"`
	ClassID           string   `json:"classId"`
	ClassroomID       string   `json:"classroomId"`
	Date              string   `json:"date"`
	Time              string   `json:"time,omitempty"`
	Period            string   `json:"period"`
	AbsenceHours      float64  `json:"-"`
	PenaltyMultiplier *int     `json:"penaltyMultiplier,omitempty"`
	PenaltyHours      *float64 `json:"-"`
	Remark            string   `json:"remark,omitempty"`
	PenaltyCount      *int     `json:"penaltyCount,omitempty"`
}

func (*EveningSelfStudyDetails) inspectionDetails() {}

// MorningAttendanceDetails represents one student/day without an exercise/sign-in subtype.
type MorningAttendanceDetails struct {
	StudentID    string `json:"studentId"`
	ClassID      string `json:"classId"`
	Date         string `json:"date"`
	AbsenceCount int    `json:"absenceCount"`
	Remark       string `json:"remark,omitempty"`
}

func (*MorningAttendanceDetails) inspectionDetails() {}

// LateReturnDetails represents one student event; OccurredAt is derived from date/time.
type LateReturnDetails struct {
	StudentID   string `json:"studentId"`
	ClassID     string `json:"classId"`
	DormitoryID string `json:"dormitoryId"`
	Date        string `json:"date"`
	Time        string `json:"time,omitempty"`
	Remark      string `json:"remark,omitempty"`
}

func (*LateReturnDetails) inspectionDetails() {}

// HygieneDetails contains the shared result and responsibility inputs, not authorization decisions.
type HygieneDetails struct {
	Date                  string   `json:"date"`
	Time                  string   `json:"time,omitempty"`
	Round                 string   `json:"round,omitempty"`
	Result                string   `json:"result"`
	ResponsibleClassIDs   []string `json:"responsibleClassIds"`
	ResponsibleStudentIDs []string `json:"responsibleStudentIds,omitempty"`
	ReasonCode            string   `json:"reasonCode,omitempty"`
	Description           string   `json:"description,omitempty"`
	Remark                string   `json:"remark,omitempty"`
}

// DormitoryHygieneDetails identifies the inspected room; responsibility remains a separate snapshot.
type DormitoryHygieneDetails struct {
	HygieneDetails
	DormitoryID string `json:"dormitoryId"`
}

func (*DormitoryHygieneDetails) inspectionDetails() {}

// ClassroomHygieneDetails reads legacy area IDs only to preserve old history.
type ClassroomHygieneDetails struct {
	HygieneDetails
	ClassroomID string `json:"classroomId"`
	AreaID      string `json:"areaId,omitempty"`
}

func (*ClassroomHygieneDetails) inspectionDetails() {}

// PublicDormitoryHygieneDetails reads legacy areas while new records use a responsibility dormitory.
type PublicDormitoryHygieneDetails struct {
	HygieneDetails
	AreaID                  string   `json:"areaId,omitempty"`
	ResponsibleDormitoryIDs []string `json:"responsibleDormitoryIds,omitempty"`
}

func (*PublicDormitoryHygieneDetails) inspectionDetails() {}

// DetailContext must come from server-resolved semester/week/type references.
// This value validates date boundaries; it is not proof of access or reference existence.
type DetailContext struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	WeekStart        string
	WeekEnd          string
}

// ValidatedDetails is a typed, normalized domain value. Repositories must still
// check scoped references and save the complete aggregate with version/audit in a transaction.
type ValidatedDetails struct {
	Details        InspectionDetails
	OccurrenceKind string
	OccurredAt     time.Time
	Date           string
	Round          string
	ViolationCount int
	DuplicateKey   string
}

// DetailIssue contains stable paths/codes and safe messages without rejected input values.
type DetailIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DetailAdapter selects an explicit domain model after catalog-driven validation.
type DetailAdapter struct{ newDetails func() InspectionDetails }

var detailAdapters = map[string]DetailAdapter{
	InspectionTypeAttendance:             {func() InspectionDetails { return &AttendanceDetails{} }},
	InspectionTypeUnattended:             {func() InspectionDetails { return &UnattendedClassroomDetails{} }},
	InspectionTypeMorning:                {func() InspectionDetails { return &MorningAttendanceDetails{} }},
	InspectionTypeLateReturn:             {func() InspectionDetails { return &LateReturnDetails{} }},
	InspectionTypeDormitoryHygiene:       {func() InspectionDetails { return &DormitoryHygieneDetails{} }},
	InspectionTypeClassroomHygiene:       {func() InspectionDetails { return &ClassroomHygieneDetails{} }},
	InspectionTypePublicDormitoryHygiene: {func() InspectionDetails { return &PublicDormitoryHygieneDetails{} }},
	InspectionTypeEveningSelfStudy:       {func() InspectionDetails { return &EveningSelfStudyDetails{} }},
}
