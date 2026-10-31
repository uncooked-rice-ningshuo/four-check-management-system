package reporting

import (
	"encoding/json"
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

// ReportVersionRecordsQuery pages the immutable, report-facing record data for
// one already-authorized report version.
type ReportVersionRecordsQuery struct {
	DepartmentID     string `form:"departmentId"`
	CollegeLevel     bool   `form:"collegeLevel"`
	InspectionTypeID string `form:"inspectionTypeId"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
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
	RowVersion             int               `json:"-"`
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

// GenerateDraftInput requests an immutable weekly report version from the
// currently confirmed source revisions. ExpectedRowVersion is zero only when
// no logical report exists yet.
type GenerateDraftInput struct {
	SemesterID         string `json:"semesterId"`
	WeekID             string `json:"weekId"`
	ExpectedRowVersion int    `json:"expectedRowVersion"`
	Force              bool   `json:"force"`
	Reason             string `json:"reason,omitempty"`
}

type ReportCountRow struct {
	DepartmentID     string `json:"departmentId"`
	DepartmentName   string `json:"departmentName"`
	CollegeLevel     bool   `json:"collegeLevel"`
	InspectionTypeID string `json:"inspectionTypeId"`
	InspectionType   string `json:"inspectionType"`
	RecordCount      int    `json:"recordCount"`
	ZeroReceiptCount int    `json:"confirmedZeroReceiptCount"`
	DataStatus       string `json:"dataStatus"`
}

type IncompleteReportCell struct {
	DepartmentID     string `json:"departmentId,omitempty"`
	DepartmentName   string `json:"departmentName"`
	CollegeLevel     bool   `json:"collegeLevel"`
	InspectionTypeID string `json:"inspectionTypeId"`
	InspectionType   string `json:"inspectionType"`
}

// ReportVersionContent freezes only approved count and detail-navigation data.
// Formal Word/Excel layout and other derived metrics are deliberately absent.
type ReportVersionContent struct {
	Period                               PeriodOption              `json:"period"`
	ScopeLimited                         bool                      `json:"scopeLimited"`
	StatisticsRuleVersion                string                    `json:"statisticsRuleVersion"`
	ConfirmedRecordCount                 int                       `json:"confirmedRecordCount"`
	Rows                                 []ReportCountRow          `json:"rows"`
	IncompleteCells                      []IncompleteReportCell    `json:"incompleteCells"`
	PendingDepartmentReconfirmationCount int                       `json:"pendingDepartmentReconfirmationCount"`
	Corrections                          []ReportCorrectionSummary `json:"corrections"`
	Adjustments                          []ReportAdjustmentSummary `json:"adjustments"`
	GeneratedAt                          time.Time                 `json:"generatedAt"`
}

type ReportCorrectionSummary struct {
	ID          string `json:"id"`
	RecordID    string `json:"recordId"`
	Field       string `json:"field"`
	FactChanged bool   `json:"factChanged"`
	Status      string `json:"status"`
}

// ReportAdjustmentSummary records an include/exclude choice made for one
// immutable report version. Record carries the authorized frozen display data
// needed to restore a previously excluded record without reading live fields.
type ReportAdjustmentSummary struct {
	ID         string    `json:"id"`
	RecordID   string    `json:"recordId"`
	Operation  string    `json:"operation"`
	Reason     string    `json:"reason"`
	Record     Record    `json:"record"`
	AdjustedBy string    `json:"adjustedBy"`
	AdjustedAt time.Time `json:"adjustedAt"`
}

type AddReportAdjustmentInput struct {
	RowVersion int    `json:"rowVersion"`
	RecordID   string `json:"recordId"`
	Operation  string `json:"operation"`
	Reason     string `json:"reason"`
}

type ReportAdjustmentResult struct {
	AdjustmentID string      `json:"adjustmentId"`
	Report       ReportState `json:"report"`
}

type AddReportCorrectionInput struct {
	RowVersion  int             `json:"rowVersion"`
	RecordID    string          `json:"recordId"`
	Field       string          `json:"field"`
	AfterValue  json.RawMessage `json:"afterValue"`
	Reason      string          `json:"reason"`
	FactChanged bool            `json:"factChanged"`
}

type ReportCorrectionResult struct {
	CorrectionID string      `json:"correctionId"`
	Report       ReportState `json:"report"`
}

type DecideReportCorrectionInput struct {
	RowVersion int    `json:"rowVersion"`
	Decision   string `json:"decision"`
	Comment    string `json:"comment,omitempty"`
}

func ReportCorrectionFieldPath(field string) ([]string, bool) {
	paths := map[string][]string{
		"studentNumber": {"studentNumber"}, "studentName": {"studentName"}, "className": {"className"}, "grade": {"grade"},
		"currentDormitoryName": {"currentDormitoryName"}, "currentCourseName": {"currentCourseName"},
		"currentClassroomName": {"currentClassroomName"}, "currentAreaName": {"currentAreaName"},
		"currentReasonLabel": {"currentReasonLabel"}, "details.date": {"details", "date"},
		"details.time": {"details", "time"}, "details.period": {"details", "period"},
		"details.round": {"details", "round"}, "details.absenceHours": {"details", "absenceHours"},
		"details.penaltyHours": {"details", "penaltyHours"}, "details.absenceCount": {"details", "absenceCount"},
		"details.result": {"details", "result"}, "details.reasonCode": {"details", "reasonCode"},
		"details.description": {"details", "description"}, "details.remark": {"details", "remark"},
	}
	path, ok := paths[field]
	return append([]string(nil), path...), ok
}

type ReportDraft struct {
	ID         string               `json:"id"`
	Status     string               `json:"status"`
	RowVersion int                  `json:"rowVersion"`
	VersionNo  int                  `json:"versionNo"`
	Content    ReportVersionContent `json:"content"`
	CreatedAt  time.Time            `json:"createdAt"`
}

type ReportRecordSnapshot struct {
	RowVersion int    `json:"-"`
	Record     Record `json:"record"`
}

type ReportTransitionInput struct {
	RowVersion int    `json:"rowVersion"`
	Comment    string `json:"comment,omitempty"`
}

type ReportState struct {
	ID         string    `json:"id"`
	SemesterID string    `json:"semesterId"`
	WeekID     string    `json:"weekId"`
	Status     string    `json:"status"`
	VersionNo  int       `json:"versionNo"`
	RowVersion int       `json:"rowVersion"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ReportWorkflow is the authorized history view for one weekly report. Every
// immutable version is returned only when the caller can view all its cells.
type ReportWorkflow struct {
	Report      *ReportState          `json:"report"`
	Versions    []ReportVersion       `json:"versions"`
	Reviews     []ReportReview        `json:"reviews"`
	Corrections []ReportCorrectionLog `json:"corrections"`
}

// ReportArchiveQuery pages reports that have at least one generated draft
// version, regardless of the current workflow status.
type ReportArchiveQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Status   string `form:"status"`
}

type ReportArchiveEntry struct {
	ID                        string       `json:"id"`
	Period                    PeriodOption `json:"period"`
	Status                    string       `json:"status"`
	VersionNo                 int          `json:"versionNo"`
	VersionCount              int          `json:"versionCount"`
	ConfirmedRecordCount      int          `json:"confirmedRecordCount"`
	ConfirmedZeroReceiptCount int          `json:"confirmedZeroReceiptCount"`
	CellCount                 int          `json:"cellCount"`
	ScopeLimited              bool         `json:"scopeLimited"`
	FinalizedAt               *time.Time   `json:"finalizedAt,omitempty"`
	PublishedAt               *time.Time   `json:"publishedAt,omitempty"`
	UpdatedAt                 time.Time    `json:"updatedAt"`
}

type ReportArchivePage struct {
	Items    []ReportArchiveEntry `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
}

type ReportVersion struct {
	ID        string               `json:"id"`
	VersionNo int                  `json:"versionNo"`
	Content   ReportVersionContent `json:"content"`
	CreatedBy string               `json:"createdBy"`
	CreatedAt time.Time            `json:"createdAt"`
}

type ReportReview struct {
	ID        string    `json:"id"`
	VersionNo int       `json:"versionNo"`
	Action    string    `json:"action"`
	Comment   string    `json:"comment"`
	ActorID   string    `json:"actorId"`
	CreatedAt time.Time `json:"createdAt"`
}

type ReportCorrectionLog struct {
	ID                string     `json:"id"`
	VersionNo         int        `json:"versionNo"`
	RecordID          string     `json:"recordId"`
	Field             string     `json:"field"`
	Reason            string     `json:"reason"`
	FactChanged       bool       `json:"factChanged"`
	Reconfirmation    string     `json:"reconfirmationStatus"`
	CorrectedBy       string     `json:"correctedBy"`
	CorrectedAt       time.Time  `json:"correctedAt"`
	Decision          string     `json:"decision,omitempty"`
	DecisionComment   string     `json:"decisionComment,omitempty"`
	DecisionActorID   string     `json:"decisionActorId,omitempty"`
	DecisionCreatedAt *time.Time `json:"decisionCreatedAt,omitempty"`
}
