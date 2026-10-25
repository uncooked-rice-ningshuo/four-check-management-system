package reporting

import (
	"time"
)

// PeriodOption is a selectable semester-week pair visible to the current actor.
type PeriodOption struct {
	AcademicYearID   string    `json:"academicYearId"`
	AcademicYearName string    `json:"academicYearName"`
	SemesterID       string    `json:"semesterId"`
	SemesterName     string    `json:"semesterName"`
	WeekID           string    `json:"weekId"`
	WeekNo           int       `json:"weekNo"`
	StartDate        time.Time `json:"startDate"`
	EndDate          time.Time `json:"endDate"`
}

// Summary contains only submission state and immutable confirmed-revision counts.
// It deliberately does not calculate any of the still-unconfirmed report metrics.
type Summary struct {
	Period PeriodOption  `json:"period"`
	Rows   []SummaryRow  `json:"rows"`
	Totals SummaryTotals `json:"totals"`
}

type SummaryRow struct {
	DepartmentID                       string `json:"departmentId"`
	DepartmentName                     string `json:"departmentName"`
	CollegeLevel                       bool   `json:"collegeLevel"`
	InspectionTypeID                   string `json:"inspectionTypeId"`
	InspectionTypeName                 string `json:"inspectionTypeName"`
	IsRequired                         bool   `json:"isRequired"`
	SubmissionCount                    int    `json:"submissionCount"`
	DraftBatchCount                    int    `json:"draftBatchCount"`
	PendingReviewBatchCount            int    `json:"pendingReviewBatchCount"`
	ReturnedBatchCount                 int    `json:"returnedBatchCount"`
	ConfirmedBatchCount                int    `json:"confirmedBatchCount"`
	ConfirmedRecordCount               int    `json:"confirmedRecordCount"`
	PendingReviewOrReturnedRecordCount int    `json:"pendingReviewOrReturnedRecordCount"`
	ZeroRecordPendingReviewBatchCount  int    `json:"zeroRecordPendingReviewBatchCount"`
	ZeroRecordReturnedBatchCount       int    `json:"zeroRecordReturnedBatchCount"`
	ZeroRecordConfirmedBatchCount      int    `json:"zeroRecordConfirmedBatchCount"`
}

type SummaryTotals struct {
	SubmissionCount                    int `json:"submissionCount"`
	DraftBatchCount                    int `json:"draftBatchCount"`
	PendingReviewBatchCount            int `json:"pendingReviewBatchCount"`
	ReturnedBatchCount                 int `json:"returnedBatchCount"`
	ConfirmedBatchCount                int `json:"confirmedBatchCount"`
	ConfirmedRecordCount               int `json:"confirmedRecordCount"`
	PendingReviewOrReturnedRecordCount int `json:"pendingReviewOrReturnedRecordCount"`
	ZeroRecordPendingReviewBatchCount  int `json:"zeroRecordPendingReviewBatchCount"`
	ZeroRecordReturnedBatchCount       int `json:"zeroRecordReturnedBatchCount"`
	ZeroRecordConfirmedBatchCount      int `json:"zeroRecordConfirmedBatchCount"`
}

// RecordsQuery selects one authorized department/type cell from a weekly report.
type RecordsQuery struct {
	SemesterID       string `form:"semesterId"`
	WeekID           string `form:"weekId"`
	DepartmentID     string `form:"departmentId"`
	CollegeLevel     bool   `form:"collegeLevel"`
	InspectionTypeID string `form:"inspectionTypeId"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
}

// RecordPage contains the confirmed source records visible in a report cell.
type RecordPage struct {
	Items    []Record `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

// ObligationPeriodQuery always requires an explicit semester and week; report
// administration must never fall back to an inferred current period.
type ObligationPeriodQuery struct {
	SemesterID string `form:"semesterId" json:"semesterId"`
	WeekID     string `form:"weekId" json:"weekId"`
}

// ReportObligation is one configured required department/type cell. An empty
// departmentId with collegeLevel=true identifies the independent college cell.
type ReportObligation struct {
	ID                 string     `json:"id"`
	SemesterID         string     `json:"semesterId"`
	WeekID             string     `json:"weekId"`
	AcademicYearID     string     `json:"academicYearId,omitempty"`
	DepartmentID       string     `json:"departmentId"`
	DepartmentName     string     `json:"departmentName"`
	CollegeLevel       bool       `json:"collegeLevel"`
	InspectionTypeID   string     `json:"inspectionTypeId"`
	InspectionTypeName string     `json:"inspectionTypeName"`
	Eligible           bool       `json:"eligible"`
	Version            int        `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	RemovedAt          *time.Time `json:"removedAt,omitempty"`
}

type ReportObligationInspectionType struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
}

// ReportObligationDepartmentScope describes a currently eligible department
// and type pair. It is an option only and never creates an obligation itself.
type ReportObligationDepartmentScope struct {
	DepartmentID       string `json:"departmentId"`
	DepartmentName     string `json:"departmentName"`
	InspectionTypeID   string `json:"inspectionTypeId"`
	InspectionTypeName string `json:"inspectionTypeName"`
}

type ReportObligationOptions struct {
	Period           PeriodOption                      `json:"period"`
	InspectionTypes  []ReportObligationInspectionType  `json:"inspectionTypes"`
	DepartmentScopes []ReportObligationDepartmentScope `json:"departmentScopes"`
}

type CreateReportObligationInput struct {
	SemesterID       string `json:"semesterId"`
	WeekID           string `json:"weekId"`
	DepartmentID     string `json:"departmentId"`
	InspectionTypeID string `json:"inspectionTypeId"`
}

// Record is a display-safe projection of one confirmed inspection record.
// It exposes typed business details and reference labels without database provenance fields.
type Record struct {
	ID                     string            `json:"id"`
	StudentNumber          string            `json:"studentNumber"`
	StudentName            string            `json:"studentName"`
	ClassName              string            `json:"className"`
	Grade                  string            `json:"grade"`
	TypeCode               string            `json:"typeCode"`
	Details                RecordDetails     `json:"details"`
	CurrentDormitoryName   string            `json:"currentDormitoryName"`
	CurrentCourseName      string            `json:"currentCourseName"`
	CurrentClassroomName   string            `json:"currentClassroomName"`
	CurrentAreaName        string            `json:"currentAreaName,omitempty"`
	CurrentReasonLabel     string            `json:"currentReasonLabel,omitempty"`
	ResponsibleClasses     []RecordReference `json:"responsibleClasses,omitempty"`
	ResponsibleStudents    []RecordReference `json:"responsibleStudents,omitempty"`
	ResponsibleDormitories []RecordReference `json:"responsibleDormitories,omitempty"`
}

// RecordDetails contains only report-facing values; foreign keys and mutation metadata stay server-side.
type RecordDetails struct {
	Date         string   `json:"date,omitempty"`
	Time         string   `json:"time,omitempty"`
	Period       string   `json:"period,omitempty"`
	Round        string   `json:"round,omitempty"`
	AbsenceHours *float64 `json:"absenceHours,omitempty"`
	PenaltyHours *float64 `json:"penaltyHours,omitempty"`
	AbsenceCount *int     `json:"absenceCount,omitempty"`
	Result       string   `json:"result,omitempty"`
	ReasonCode   string   `json:"reasonCode,omitempty"`
	Description  string   `json:"description,omitempty"`
	Remark       string   `json:"remark,omitempty"`
}

// RecordReference is a human-readable responsibility label without internal IDs.
type RecordReference struct {
	Label string `json:"label"`
	Grade string `json:"grade,omitempty"`
}
