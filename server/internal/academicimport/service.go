package academicimport

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"github.com/google/uuid"
)

var asciiDigits = regexp.MustCompile(`^[0-9]+$`)

type Service struct {
	repository Repository
	catalog    Catalog
	maxBytes   int64
	now        func() time.Time
}

func NewService(repository Repository, catalog Catalog, maxBytes int64) *Service {
	return &Service{repository: repository, catalog: catalog, maxBytes: maxBytes, now: time.Now}
}

func (s *Service) Preview(ctx context.Context, semesterID, fileID, filename, actorID string, content []byte, metadata audit.Metadata) (Batch, error) {
	semesterID, fileID, actorID = strings.TrimSpace(semesterID), strings.TrimSpace(fileID), strings.TrimSpace(actorID)
	if s.repository == nil || s.catalog == nil || s.maxBytes <= 0 || semesterID == "" || fileID == "" || actorID == "" {
		return Batch{}, ErrInvalidBatch
	}
	parser, err := NewParser(s.maxBytes)
	if err != nil {
		return Batch{}, err
	}
	workbook, err := parser.Parse(filename, content)
	if err != nil {
		return Batch{}, err
	}
	if len(workbook.Rows) == 0 {
		return Batch{}, ErrEmptyWorkbook
	}
	calendar, err := s.catalog.List(ctx)
	if err != nil {
		return Batch{}, err
	}
	semester, yearName, ok := calendarSemester(calendar, semesterID)
	if !ok {
		return Batch{}, ErrSemesterNotFound
	}
	rows, affectsToday := validateRows(workbook.Rows, semester, calendar.Weeks, s.now())
	batch := Batch{
		ID: uuid.NewString(), Status: StatusPreviewReady, Version: 1, TemplateVersion: TemplateVersion,
		SemesterID: semester.ID, SemesterName: semester.Name, AcademicYearName: yearName,
		TotalRows: len(rows), Rows: rows, AffectsToday: affectsToday,
		CreatedAt: s.now().UTC(), SourceFileID: fileID, CreatedBy: actorID,
	}
	for _, row := range rows {
		if row.Status == RowValid {
			batch.ValidRows++
		} else {
			batch.InvalidRows++
		}
	}
	if batch.InvalidRows > 0 {
		batch.Status = StatusPreviewHasErrors
	}
	metadata.ActorID = actorID
	if err := s.repository.CreateAcademicWeekImportBatch(ctx, batch, metadata); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func (s *Service) GetBatch(ctx context.Context, id, actorID string) (Batch, error) {
	id, actorID = strings.TrimSpace(id), strings.TrimSpace(actorID)
	if id == "" || actorID == "" || s.repository == nil {
		return Batch{}, ErrBatchNotFound
	}
	return s.repository.FindAcademicWeekImportBatch(ctx, id, actorID)
}

func (s *Service) Confirm(ctx context.Context, input ConfirmInput) (ConfirmResult, error) {
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.BatchID == "" || input.ActorID == "" || input.ExpectedVersion < 1 || len(input.IdempotencyKey) < 16 || len(input.IdempotencyKey) > 128 {
		return ConfirmResult{}, ErrInvalidIdempotencyKey
	}
	input.Metadata.ActorID = input.ActorID
	return s.repository.ConfirmAcademicWeekImportBatch(ctx, input)
}

func validateRows(parsed []importjob.ParsedRow, semester academic.Semester, existing []academic.Week, now time.Time) ([]PreviewRow, bool) {
	rows := make([]PreviewRow, len(parsed))
	weekNoRows := make(map[int][]int)
	var affectsToday bool
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		shanghai = time.FixedZone("CST", 8*60*60)
	}
	today := dateOnly(now.In(shanghai))

	for index, row := range parsed {
		preview := PreviewRow{RowNo: row.RowNo, Values: cloneValues(row.Values), Status: RowValid, Errors: make([]RowError, 0)}
		week := academic.Week{ID: uuid.NewString(), SemesterID: semester.ID, Enabled: true, AttendancePenaltyMultiplier: intPointer(1)}
		valid := true
		weekNoText := strings.TrimSpace(row.Values["week_no"])
		if !asciiDigits.MatchString(weekNoText) {
			addRowError(&preview, "week_no", "周次必须是 1–60 的 ASCII 十进制整数")
			valid = false
		} else if number, parseErr := strconv.Atoi(weekNoText); parseErr != nil || number < 1 || number > 60 {
			addRowError(&preview, "week_no", "周次范围必须为 1–60")
			valid = false
		} else {
			week.WeekNo = number
			weekNoRows[number] = append(weekNoRows[number], index)
		}
		startDate, startOK := parseDate(row.Values["start_date"])
		endDate, endOK := parseDate(row.Values["end_date"])
		if !startOK {
			addRowError(&preview, "start_date", "开始日期必须使用 YYYY-MM-DD 格式")
			valid = false
		}
		if !endOK {
			addRowError(&preview, "end_date", "结束日期必须使用 YYYY-MM-DD 格式")
			valid = false
		}
		if startOK && endOK {
			week.StartDate, week.EndDate = startDate, endDate
			if startDate.After(endDate) {
				addRowError(&preview, "end_date", "结束日期不得早于开始日期")
				valid = false
			} else if academic.ValidateWeekWithinSemester(academic.Calendar{Semesters: []academic.Semester{semester}}, week) != nil {
				addRowError(&preview, "start_date", "周次日期必须位于所选学期范围内")
				valid = false
			}
			if !today.Before(startDate) && !today.After(endDate) && startDate.Compare(endDate) <= 0 {
				affectsToday = true
			}
		}
		week.Note = strings.TrimSpace(row.Values["note"])
		if utf8.RuneCountInString(week.Note) > 255 {
			addRowError(&preview, "note", "备注最多 255 个 Unicode 字符")
			valid = false
		}
		multiplierText := strings.TrimSpace(row.Values["attendance_penalty_multiplier"])
		if multiplierText != "" {
			if multiplierText != "1" && multiplierText != "2" {
				addRowError(&preview, "attendance_penalty_multiplier", "倍数只能填写 1 或 2")
				valid = false
			} else {
				week.AttendancePenaltyMultiplier = intPointer(int(multiplierText[0] - '0'))
			}
		}
		preview.Week = &week
		rows[index] = preview
		_ = valid
	}

	for _, indexes := range weekNoRows {
		if len(indexes) > 1 {
			for _, index := range indexes {
				addRowError(&rows[index], "week_no", "文件中周次号重复")
			}
		}
	}
	for index := range rows {
		week := rows[index].Week
		if week == nil || week.WeekNo == 0 {
			continue
		}
		for _, existingWeek := range existing {
			if existingWeek.SemesterID == semester.ID && existingWeek.WeekNo == week.WeekNo {
				addRowError(&rows[index], "week_no", "目标学期已存在该周次号，包括已停用周次")
				break
			}
		}
		if week.StartDate.IsZero() || week.EndDate.IsZero() || week.StartDate.After(week.EndDate) {
			continue
		}
		for prior := 0; prior < index; prior++ {
			other := rows[prior].Week
			if other != nil && other.WeekNo > 0 && !other.StartDate.IsZero() && !other.EndDate.IsZero() && dateRangesOverlap(week.StartDate, week.EndDate, other.StartDate, other.EndDate) {
				addRowError(&rows[index], "start_date", "日期区间与文件中的其他周次重叠")
				addRowError(&rows[prior], "start_date", "日期区间与文件中的其他周次重叠")
			}
		}
		for _, existingWeek := range existing {
			if existingWeek.Enabled && dateRangesOverlap(week.StartDate, week.EndDate, existingWeek.StartDate, existingWeek.EndDate) {
				addRowError(&rows[index], "start_date", "日期区间与现有启用周次重叠")
				break
			}
		}
	}
	for index := range rows {
		if len(rows[index].Errors) > 0 {
			rows[index].Status = RowInvalid
		}
	}
	return rows, affectsToday
}

func calendarSemester(calendar academic.Calendar, semesterID string) (academic.Semester, string, bool) {
	years := make(map[string]string, len(calendar.AcademicYears))
	for _, year := range calendar.AcademicYears {
		years[year.ID] = year.Name
	}
	for _, semester := range calendar.Semesters {
		if semester.ID == semesterID {
			return semester, years[semester.AcademicYearID], true
		}
	}
	return academic.Semester{}, "", false
}

func addRowError(row *PreviewRow, field, message string) {
	row.Errors = append(row.Errors, RowError{Field: field, Message: message})
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if len(value) != len("2006-01-02") {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, false
	}
	return parsed, true
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func dateRangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aEnd.Before(bStart) && !bEnd.Before(aStart)
}

func intPointer(value int) *int { return &value }

func cloneValues(values map[string]string) map[string]string {
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
