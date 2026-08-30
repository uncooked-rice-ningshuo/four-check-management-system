package importjob

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"four-check-management-system/server/internal/inspection"
)

const (
	ErrorRequiredField       ErrorCode = "REQUIRED_FIELD"
	ErrorReferenceNotFound   ErrorCode = "REFERENCE_NOT_FOUND"
	ErrorReferenceMismatch   ErrorCode = "REFERENCE_MISMATCH"
	ErrorInvalidDate         ErrorCode = "INVALID_DATE"
	ErrorInvalidTime         ErrorCode = "INVALID_TIME"
	ErrorOccurredOutsideWeek ErrorCode = "OCCURRED_OUTSIDE_WEEK"
	ErrorInvalidHours        ErrorCode = "INVALID_HOURS"
	ErrorTextTooLong         ErrorCode = "TEXT_TOO_LONG"
	ErrorDuplicateInBatch    ErrorCode = "DUPLICATE_IN_BATCH"
	ErrorRecordAlreadyExists ErrorCode = "RECORD_ALREADY_EXISTS"
)

// ValidationError is safe to persist and return in an import preview.
type ValidationError struct {
	Code       ErrorCode `json:"code"`
	Field      string    `json:"field"`
	Message    string    `json:"message"`
	Suggestion string    `json:"suggestion"`
}

type AttendanceImportScope struct {
	InspectionTypeID string
	SemesterID       string
	WeekID           string
	DepartmentID     string
	ClassID          string
	Grade            string
	ImportBatchID    string
	CreatedBy        string
	WeekStart        time.Time
	WeekEnd          time.Time
	Location         *time.Location
}

type AttendanceReferenceLookup struct {
	SemesterID       string
	WeekID           string
	InspectionTypeID string
	DepartmentID     string
	ClassID          string
	Grade            string
	StudentNos       []string
	ClassNames       []string
	CourseNames      []string
	ClassroomCodes   []string
}

type AttendanceStudentReference struct {
	ID, StudentNo, Name, ClassID, ClassName, Grade string
}

type AttendanceClassReference struct {
	ID, Name, Grade string
}

type AttendanceCourseReference struct {
	ID, Name, ClassID, ClassroomID string
}

type AttendanceClassroomReference struct {
	ID, Code string
}

type AttendanceReferences struct {
	StudentsByNo          map[string]AttendanceStudentReference
	ClassesByName         map[string]AttendanceClassReference
	CoursesByClassAndName map[string]AttendanceCourseReference
	ClassroomsByCode      map[string]AttendanceClassroomReference
}

type AttendanceReferenceRepository interface {
	LoadAttendanceReferences(context.Context, AttendanceReferenceLookup) (AttendanceReferences, error)
	FindExistingAttendanceKeys(context.Context, AttendanceImportScope, []string) (map[string]struct{}, error)
}

type AttendanceValidationRow struct {
	RowNo  int                         `json:"rowNo"`
	Record inspection.AttendanceRecord `json:"record"`
	Errors []ValidationError           `json:"errors"`
}

type AttendanceValidationResult struct {
	Rows         []AttendanceValidationRow `json:"rows"`
	ValidCount   int                       `json:"validCount"`
	InvalidCount int                       `json:"invalidCount"`
}

type AttendanceAdapter struct {
	repository AttendanceReferenceRepository
}

var chinaStandardTime = time.FixedZone("Asia/Shanghai", 8*60*60)

func NewAttendanceAdapter(repository AttendanceReferenceRepository) *AttendanceAdapter {
	return &AttendanceAdapter{repository: repository}
}

// Validate prefetches all scoped references once, then validates rows without N+1 database calls.
func (a *AttendanceAdapter) Validate(ctx context.Context, scope AttendanceImportScope, rows []ParsedRow) (AttendanceValidationResult, error) {
	lookup := attendanceLookup(scope, rows)
	references, err := a.repository.LoadAttendanceReferences(ctx, lookup)
	if err != nil {
		return AttendanceValidationResult{}, fmt.Errorf("load attendance references: %w", err)
	}
	result := AttendanceValidationResult{Rows: make([]AttendanceValidationRow, 0, len(rows))}
	keys := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		validated := a.validateRow(scope, references, row)
		if len(validated.Errors) == 0 {
			if _, exists := seen[validated.Record.DedupKey]; exists {
				validated.Errors = append(validated.Errors, validationError(ErrorDuplicateInBatch, "student_no", "本批次存在重复到课率记录", "删除重复行或修正日期、课程和节次"))
			} else {
				seen[validated.Record.DedupKey] = struct{}{}
				keys = append(keys, validated.Record.DedupKey)
			}
		}
		result.Rows = append(result.Rows, validated)
	}
	existing, err := a.repository.FindExistingAttendanceKeys(ctx, scope, uniqueStrings(keys))
	if err != nil {
		return AttendanceValidationResult{}, fmt.Errorf("find existing attendance keys: %w", err)
	}
	for index := range result.Rows {
		row := &result.Rows[index]
		if row.Record.DedupKey != "" {
			if _, exists := existing[row.Record.DedupKey]; exists {
				row.Errors = append(row.Errors, validationError(ErrorRecordAlreadyExists, "student_no", "系统中已存在相同到课率记录", "核对已录入数据，不要重复导入"))
			}
		}
		if len(row.Errors) == 0 {
			result.ValidCount++
		} else {
			result.InvalidCount++
		}
	}
	return result, nil
}

func (a *AttendanceAdapter) validateRow(scope AttendanceImportScope, references AttendanceReferences, row ParsedRow) AttendanceValidationRow {
	result := AttendanceValidationRow{RowNo: row.RowNo, Errors: make([]ValidationError, 0)}
	value := func(field string) string { return strings.TrimSpace(row.Values[field]) }
	studentNo, studentName, className := value("student_no"), value("student_name"), value("class_name")
	courseName, classroomCode := value("course_name"), value("classroom_code")
	period, remark := value("period"), value("remark")
	for field, fieldValue := range map[string]string{
		"student_no": studentNo, "student_name": studentName, "class_name": className, "course_name": courseName,
		"absence_date": value("absence_date"), "absence_time": value("absence_time"), "period": period,
		"absence_hours": value("absence_hours"), "classroom_code": classroomCode,
	} {
		if fieldValue == "" {
			result.Errors = append(result.Errors, validationError(ErrorRequiredField, field, "必填字段不能为空", "填写模板要求的值"))
		}
	}
	student, studentOK := references.StudentsByNo[studentNo]
	class, classOK := references.ClassesByName[className]
	var course AttendanceCourseReference
	courseOK := false
	if classOK {
		course, courseOK = references.CoursesByClassAndName[attendanceCourseKey(class.ID, courseName)]
	}
	classroom, classroomOK := references.ClassroomsByCode[classroomCode]
	if studentNo != "" && !studentOK {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "student_no", "学号不存在或不在当前数据范围", "核对学号及当前检查范围"))
	}
	if className != "" && !classOK {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "class_name", "班级不存在或不在当前数据范围", "使用系统中的有效班级名称"))
	}
	if courseName != "" && !courseOK {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "course_name", "课程不存在或不在当前数据范围", "使用系统中的有效课程名称"))
	}
	if classroomCode != "" && !classroomOK {
		result.Errors = append(result.Errors, validationError(ErrorReferenceNotFound, "classroom_code", "教室不存在或不在当前数据范围", "使用系统中的有效教室编码"))
	}
	if studentOK && studentName != student.Name {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "student_name", "姓名与学号不一致", "使用学号对应的姓名"))
	}
	if studentOK && classOK && student.ClassID != class.ID {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "class_name", "学生与班级不一致", "使用学生当前所在班级"))
	}
	if classOK && scope.ClassID != "" && class.ID != scope.ClassID {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "class_name", "班级不在当前授权范围", "使用当前授权班级"))
	}
	if classOK && scope.Grade != "" && class.Grade != scope.Grade {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "class_name", "年级不在当前授权范围", "使用当前授权年级的班级"))
	}
	if courseOK && classOK && course.ClassID != "" && course.ClassID != class.ID {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "course_name", "课程与班级不一致", "使用该班级当前课程"))
	}
	if courseOK && classroomOK && course.ClassroomID != "" && course.ClassroomID != classroom.ID {
		result.Errors = append(result.Errors, validationError(ErrorReferenceMismatch, "classroom_code", "教室与课程安排不一致", "使用课程安排中的教室"))
	}

	location := scope.Location
	if location == nil {
		location = chinaStandardTime
	}
	date, dateErr := time.ParseInLocation("2006-01-02", value("absence_date"), location)
	if value("absence_date") != "" && dateErr != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidDate, "absence_date", "日期格式无效", "使用 YYYY-MM-DD 格式"))
	}
	clock, timeErr := time.ParseInLocation("15:04", value("absence_time"), location)
	if value("absence_time") != "" && timeErr != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidTime, "absence_time", "时间格式无效", "使用 HH:MM 的 24 小时格式"))
	}
	var occurredAt time.Time
	if dateErr == nil && timeErr == nil {
		occurredAt = time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, location)
		if occurredAt.Before(startOfBusinessDay(scope.WeekStart, location)) || occurredAt.After(endOfBusinessDay(scope.WeekEnd, location)) {
			result.Errors = append(result.Errors, validationError(ErrorOccurredOutsideWeek, "absence_date", "旷课日期不在所选周次", "选择该周次范围内的日期"))
		}
	}
	hours, hoursErr := strconv.ParseFloat(value("absence_hours"), 64)
	if value("absence_hours") != "" && (hoursErr != nil || hours <= 0 || hours > 24) {
		result.Errors = append(result.Errors, validationError(ErrorInvalidHours, "absence_hours", "旷课学时格式或范围无效", "填写大于 0 且不超过 24 的数字"))
	}
	if utf8.RuneCountInString(period) > 64 {
		result.Errors = append(result.Errors, validationError(ErrorTextTooLong, "period", "节次文本过长", "将节次控制在 64 个字符内"))
	}
	if utf8.RuneCountInString(remark) > 500 {
		result.Errors = append(result.Errors, validationError(ErrorTextTooLong, "remark", "备注过长", "将备注控制在 500 个字符内"))
	}
	if len(result.Errors) != 0 {
		return result
	}
	record, err := inspection.NewAttendanceRecord(inspection.AttendanceInput{
		InspectionTypeID: scope.InspectionTypeID, InspectionTypeCode: inspection.InspectionTypeAttendance,
		SemesterID: scope.SemesterID, WeekID: scope.WeekID, DepartmentID: scope.DepartmentID,
		StudentID: student.ID, StudentNoSnapshot: student.StudentNo, StudentNameSnapshot: student.Name,
		ClassID: class.ID, ClassNameSnapshot: class.Name, GradeSnapshot: class.Grade,
		CourseID: course.ID, ClassroomID: classroom.ID, OccurredAt: occurredAt, WeekStart: scope.WeekStart, WeekEnd: scope.WeekEnd,
		AbsenceHours: hours, PeriodText: period, Remark: remark, ImportBatchID: scope.ImportBatchID, CreatedBy: scope.CreatedBy,
	})
	if err != nil {
		result.Errors = append(result.Errors, validationError(ErrorInvalidDate, "absence_date", "到课率记录不符合当前周次规则", "核对日期、时间和业务上下文"))
		return result
	}
	result.Record = record
	return result
}

func attendanceLookup(scope AttendanceImportScope, rows []ParsedRow) AttendanceReferenceLookup {
	lookup := AttendanceReferenceLookup{
		SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID, ClassID: scope.ClassID, Grade: scope.Grade,
	}
	for _, row := range rows {
		lookup.StudentNos = append(lookup.StudentNos, strings.TrimSpace(row.Values["student_no"]))
		lookup.ClassNames = append(lookup.ClassNames, strings.TrimSpace(row.Values["class_name"]))
		lookup.CourseNames = append(lookup.CourseNames, strings.TrimSpace(row.Values["course_name"]))
		lookup.ClassroomCodes = append(lookup.ClassroomCodes, strings.TrimSpace(row.Values["classroom_code"]))
	}
	lookup.StudentNos = uniqueStrings(lookup.StudentNos)
	lookup.ClassNames = uniqueStrings(lookup.ClassNames)
	lookup.CourseNames = uniqueStrings(lookup.CourseNames)
	lookup.ClassroomCodes = uniqueStrings(lookup.ClassroomCodes)
	return lookup
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func attendanceCourseKey(classID, courseName string) string {
	return strings.TrimSpace(classID) + "\x1f" + strings.TrimSpace(courseName)
}

func validationError(code ErrorCode, field, message, suggestion string) ValidationError {
	return ValidationError{Code: code, Field: field, Message: message, Suggestion: suggestion}
}

func startOfBusinessDay(value time.Time, location *time.Location) time.Time {
	localized := value.In(location)
	return time.Date(localized.Year(), localized.Month(), localized.Day(), 0, 0, 0, 0, location)
}

func endOfBusinessDay(value time.Time, location *time.Location) time.Time {
	return startOfBusinessDay(value, location).Add(24*time.Hour - time.Nanosecond)
}
