package reportoutput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
)

var (
	ErrNotFound           = errors.New("report output not found")
	ErrUnavailable        = errors.New("report output unavailable")
	ErrConfigConflict     = errors.New("report output settings version conflict")
	ErrPreviewRateLimited = errors.New("report output preview rate limited")
	ErrIssueDateMissing   = errors.New("report output weekly issue date missing")
	ErrInvalidWeekDate    = errors.New("invalid report output weekly issue date")
	ErrWeekNotFound       = errors.New("report output week not found")
)

type Settings struct {
	Introduction         string          `json:"introduction"`
	IntroductionDocument json.RawMessage `json:"introductionDocument"`
	WordTypeCodes        []string        `json:"wordTypeCodes"`
	ExcelTypeCodes       []string        `json:"excelTypeCodes"`
	Version              int             `json:"version"`
}
type GenerateInput struct {
	SemesterID      string   `json:"semesterId"`
	WeekID          string   `json:"weekId"`
	VersionNo       int      `json:"versionNo"`
	Format          string   `json:"format"`
	TypeCodes       []string `json:"typeCodes"`
	Combined        bool     `json:"combined"`
	SettingsVersion int      `json:"settingsVersion"`
}

// WeekDate is the editable file date for one academic week. Version zero means
// that the week has not yet been configured.
type WeekDate struct {
	SemesterID   string `json:"semesterId"`
	SemesterName string `json:"semesterName"`
	WeekID       string `json:"weekId"`
	WeekNo       int    `json:"weekNo"`
	IssueDate    string `json:"issueDate"`
	Version      int    `json:"version"`
}
type File struct {
	ID                   string          `json:"id"`
	ReportVersionID      string          `json:"reportVersionId"`
	Format               string          `json:"format"`
	TypeCodes            []string        `json:"typeCodes"`
	Combined             bool            `json:"combined"`
	Status               string          `json:"status"`
	FileID               string          `json:"-"`
	FailureCode          string          `json:"failureCode,omitempty"`
	SettingsVersion      int             `json:"settingsVersion,omitempty"`
	LayoutVersion        string          `json:"layoutVersion,omitempty"`
	IssueDate            string          `json:"issueDate,omitempty"`
	IntroductionSnapshot json.RawMessage `json:"-"`
	CreatedAt            time.Time       `json:"createdAt"`
}

type Repository interface {
	GetOutputSettings(context.Context) (Settings, error)
	ReplaceOutputSettings(context.Context, Settings, audit.Metadata) (Settings, error)
	ListWeekDates(context.Context) ([]WeekDate, error)
	GetWeekDate(context.Context, string, string) (WeekDate, error)
	SaveWeekDate(context.Context, WeekDate, audit.Metadata) (WeekDate, error)
	StartReportOutput(context.Context, File, string, audit.Metadata) error
	FinishReportOutput(context.Context, string, string, audit.Metadata) error
	FailReportOutput(context.Context, string, string, audit.Metadata) error
	ListReportOutputs(context.Context, string) ([]File, error)
	GetReportOutput(context.Context, string) (File, error)
	LoadReportOutputRecords(context.Context, string) ([]reporting.Record, error)
	ListOutputTypes(context.Context) ([]OutputType, error)
	RecordOutputPreview(context.Context, []string, audit.Metadata) error
	RecordTransientOutput(context.Context, File, int, audit.Metadata) error
}

type OutputType struct {
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Columns      []string        `json:"columns"`
	Sections     []OutputSection `json:"sections,omitempty"`
	ExcelColumns []string        `json:"excelColumns"`
}

type OutputSection struct {
	Title   string   `json:"title"`
	Columns []string `json:"columns"`
}
type FileStore interface {
	Store(context.Context, storage.StoreInput, audit.Metadata) (storage.File, error)
	ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error)
}
type PermissionReader interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}
type WorkflowReader interface {
	Workflow(context.Context, string, string, string) (reporting.ReportWorkflow, error)
}
type Service struct {
	repository  Repository
	files       FileStore
	permissions PermissionReader
	reports     WorkflowReader
}

func NewService(repository Repository, files FileStore, permissions PermissionReader, reports WorkflowReader) *Service {
	return &Service{repository, files, permissions, reports}
}
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	settings, err := s.repository.GetOutputSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	if len(settings.IntroductionDocument) == 0 {
		settings.IntroductionDocument = legacyIntroduction(settings.Introduction)
	}
	return settings, nil
}
func (s *Service) ReplaceSettings(ctx context.Context, settings Settings, metadata audit.Metadata) (Settings, error) {
	if settings.Version < 1 || len(settings.Introduction) > 20000 {
		return Settings{}, ErrInvalidSelection
	}
	if len(settings.IntroductionDocument) == 0 {
		settings.IntroductionDocument = legacyIntroduction(settings.Introduction)
	}
	document, err := decodeIntroduction(settings.IntroductionDocument)
	if err != nil {
		return Settings{}, err
	}
	settings.Introduction = plainIntroduction(document)
	for _, codes := range [][]string{settings.WordTypeCodes, settings.ExcelTypeCodes} {
		if len(codes) > 0 {
			if _, err := chosenTypes(codes); err != nil {
				return Settings{}, err
			}
		}
	}
	return s.repository.ReplaceOutputSettings(ctx, settings, metadata)
}
func (s *Service) WeekDates(ctx context.Context) ([]WeekDate, error) {
	return s.repository.ListWeekDates(ctx)
}
func (s *Service) SaveWeekDate(ctx context.Context, input WeekDate, metadata audit.Metadata) (WeekDate, error) {
	if _, err := uuid.Parse(input.SemesterID); err != nil {
		return WeekDate{}, ErrInvalidSelection
	}
	if _, err := uuid.Parse(input.WeekID); err != nil {
		return WeekDate{}, ErrInvalidSelection
	}
	if input.Version < 0 {
		return WeekDate{}, ErrInvalidSelection
	}
	date, err := time.Parse("2006-01-02", input.IssueDate)
	if err != nil || date.Format("2006-01-02") != input.IssueDate {
		return WeekDate{}, ErrInvalidWeekDate
	}
	return s.repository.SaveWeekDate(ctx, input, metadata)
}

func (s *Service) EditorSettings(ctx context.Context) (Settings, []OutputType, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return Settings{}, nil, err
	}
	types, err := s.repository.ListOutputTypes(ctx)
	for index := range types {
		types[index].Title, types[index].Columns = outputTypeLayout(types[index].Code)
		types[index].Sections = wordOutputSections(types[index].Code, types[index].Columns)
		types[index].ExcelColumns = excelTypeColumns(types[index].Code)
	}
	return settings, types, err
}

// PreviewExcel renders the selected sheets from fictional data without changing report state.
func (s *Service) PreviewExcel(ctx context.Context, codes []string, metadata audit.Metadata) ([]byte, error) {
	if _, err := chosenTypes(codes); err != nil {
		return nil, err
	}
	content := reporting.ReportVersionContent{Period: reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}}
	data, err := RenderExcel(content, nil, codes)
	if err != nil {
		return nil, err
	}
	if err := ValidateGeneratedFile("EXCEL", data); err != nil {
		return nil, err
	}
	if err := s.repository.RecordOutputPreview(ctx, codes, metadata); err != nil {
		return nil, err
	}
	return data, nil
}

// PreviewWord uses fictional period data and never stores a report output.
func (s *Service) PreviewWord(ctx context.Context, document json.RawMessage, codes []string, semesterID, weekID string, metadata audit.Metadata) ([]byte, error) {
	if _, err := decodeIntroduction(document); err != nil {
		return nil, err
	}
	if _, err := chosenTypes(codes); err != nil {
		return nil, err
	}
	if (semesterID == "") != (weekID == "") {
		return nil, ErrInvalidSelection
	}
	if semesterID != "" {
		if _, err := uuid.Parse(semesterID); err != nil {
			return nil, ErrInvalidSelection
		}
		if _, err := uuid.Parse(weekID); err != nil {
			return nil, ErrInvalidSelection
		}
	}
	period := reporting.PeriodOption{SemesterName: "示例学期", WeekNo: 9}
	issueDate := "2025-06-01"
	if semesterID != "" {
		weekDate, err := s.repository.GetWeekDate(ctx, semesterID, weekID)
		if err != nil {
			return nil, err
		}
		if weekDate.IssueDate == "" {
			return nil, ErrIssueDateMissing
		}
		period.SemesterName, period.WeekNo, issueDate = weekDate.SemesterName, weekDate.WeekNo, weekDate.IssueDate
	}
	content := reporting.ReportVersionContent{Period: period}
	previewDocument, _, err := resolveIntroductionDate(document, issueDate)
	if err != nil {
		return nil, err
	}
	data, err := RenderWordDocument(content, nil, codes, previewDocument, true)
	if err != nil {
		return nil, err
	}
	if err := ValidateGeneratedFile("WORD", data); err != nil {
		return nil, err
	}
	if err := s.repository.RecordOutputPreview(ctx, codes, metadata); err != nil {
		return nil, err
	}
	return data, nil
}
func (s *Service) version(ctx context.Context, actorID, semesterID, weekID string, versionNo int) (reporting.ReportVersion, error) {
	if _, err := uuid.Parse(semesterID); err != nil {
		return reporting.ReportVersion{}, ErrInvalidSelection
	}
	if _, err := uuid.Parse(weekID); err != nil {
		return reporting.ReportVersion{}, ErrInvalidSelection
	}
	if versionNo < 1 {
		return reporting.ReportVersion{}, ErrInvalidSelection
	}
	workflow, err := s.reports.Workflow(ctx, actorID, semesterID, weekID)
	if err != nil {
		return reporting.ReportVersion{}, err
	}
	for _, version := range workflow.Versions {
		if version.VersionNo == versionNo {
			return version, nil
		}
	}
	return reporting.ReportVersion{}, ErrNotFound
}
func (s *Service) Generate(ctx context.Context, actorID string, input GenerateInput, metadata audit.Metadata) (File, error) {
	types, err := chosenTypes(input.TypeCodes)
	if err != nil {
		return File{}, err
	}
	if input.Format != "WORD" && input.Format != "EXCEL" || (!input.Combined && len(types) != 1) || input.SettingsVersion < 1 {
		return File{}, ErrInvalidSelection
	}
	version, err := s.version(ctx, actorID, input.SemesterID, input.WeekID, input.VersionNo)
	if err != nil {
		return File{}, err
	}
	if version.Content.ScopeLimited {
		return File{}, reporting.ErrForbidden
	}
	available := make(map[string]bool)
	for _, row := range version.Content.Rows {
		available[row.InspectionTypeCode] = true
	}
	for _, item := range types {
		if !available[item.code] {
			return File{}, ErrInvalidSelection
		}
	}
	effective, err := s.permissions.Effective(ctx, actorID)
	if err != nil {
		return File{}, err
	}
	for _, row := range version.Content.Rows {
		selected := false
		for _, item := range types {
			if item.code == row.InspectionTypeCode {
				selected = true
				break
			}
		}
		if !selected {
			continue
		}
		scope := permission.Scope{AcademicYearID: version.Content.Period.AcademicYearID, SemesterID: input.SemesterID, WeekID: input.WeekID, InspectionTypeID: row.InspectionTypeID, DepartmentID: row.DepartmentID}
		if !permission.AllowsWholeScope(effective.Permissions, permission.ActionReportGenerate, scope) {
			return File{}, reporting.ErrForbidden
		}
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return File{}, err
	}
	if input.SettingsVersion != settings.Version {
		return File{}, ErrConfigConflict
	}
	introduction := settings.IntroductionDocument
	issueDate := ""
	if input.Format == "WORD" && input.Combined {
		weekDate, dateErr := s.repository.GetWeekDate(ctx, input.SemesterID, input.WeekID)
		if dateErr != nil {
			return File{}, dateErr
		}
		if weekDate.IssueDate == "" {
			return File{}, ErrIssueDateMissing
		}
		issueDate = weekDate.IssueDate
		introduction, _, err = resolveIntroductionDate(introduction, issueDate)
		if err != nil {
			return File{}, err
		}
	}
	output := File{ID: uuid.NewString(), ReportVersionID: version.ID, Format: input.Format, TypeCodes: append([]string(nil), input.TypeCodes...), Combined: input.Combined, Status: "PENDING", CreatedAt: time.Now().UTC(), SettingsVersion: settings.Version, LayoutVersion: "WORD_V2"}
	output.IssueDate = issueDate
	if input.Format == "WORD" && input.Combined {
		output.IntroductionSnapshot = introduction
	}
	if err := s.repository.StartReportOutput(ctx, output, actorID, metadata); err != nil {
		return File{}, err
	}
	fail := func(cause error) (File, error) {
		_ = s.repository.FailReportOutput(ctx, output.ID, "GENERATION_FAILED", metadata)
		return File{}, cause
	}
	records, err := s.repository.LoadReportOutputRecords(ctx, version.ID)
	if err != nil {
		return fail(err)
	}
	for _, item := range types {
		expected, observed := 0, 0
		for _, row := range version.Content.Rows {
			if row.InspectionTypeCode == item.code {
				expected += row.RecordCount
			}
		}
		for _, record := range records {
			if record.TypeCode == item.code {
				observed++
			}
		}
		if expected != observed {
			return fail(ErrInvalidGeneratedFile)
		}
	}
	var data []byte
	if input.Format == "EXCEL" {
		data, err = RenderExcel(version.Content, records, input.TypeCodes)
	} else {
		data, err = RenderWordDocument(version.Content, records, input.TypeCodes, introduction, input.Combined)
	}
	if err != nil {
		return fail(err)
	}
	if err := ValidateGeneratedFile(input.Format, data); err != nil {
		return fail(err)
	}
	mime := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	ext := "xlsx"
	if input.Format == "WORD" {
		mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		ext = "docx"
	}
	stored, err := s.files.Store(ctx, storage.StoreInput{OriginalName: fmt.Sprintf("report-v%d.%s", input.VersionNo, ext), DeclaredContentType: mime, Size: int64(len(data)), RetentionType: storage.RetentionReportDraft, CreatedBy: actorID, Body: bytes.NewReader(data)}, metadata)
	if err != nil {
		return fail(err)
	}
	if err := s.repository.FinishReportOutput(ctx, output.ID, stored.ID, metadata); err != nil {
		return fail(err)
	}
	output.FileID = stored.ID
	output.Status = "AVAILABLE"
	return output, nil
}

// GenerateDownload renders a verified file for this request without creating an output record or stored object.
func (s *Service) GenerateDownload(ctx context.Context, actorID string, input GenerateInput, metadata audit.Metadata) (File, []byte, error) {
	types, err := chosenTypes(input.TypeCodes)
	if err != nil {
		return File{}, nil, err
	}
	if input.Format != "WORD" && input.Format != "EXCEL" || (!input.Combined && len(types) != 1) || input.SettingsVersion < 1 {
		return File{}, nil, ErrInvalidSelection
	}
	version, err := s.version(ctx, actorID, input.SemesterID, input.WeekID, input.VersionNo)
	if err != nil {
		return File{}, nil, err
	}
	if version.Content.ScopeLimited {
		return File{}, nil, reporting.ErrForbidden
	}
	available := make(map[string]bool)
	for _, row := range version.Content.Rows {
		available[row.InspectionTypeCode] = true
	}
	for _, item := range types {
		if !available[item.code] {
			return File{}, nil, ErrInvalidSelection
		}
	}
	effective, err := s.permissions.Effective(ctx, actorID)
	if err != nil {
		return File{}, nil, err
	}
	for _, row := range version.Content.Rows {
		selected := false
		for _, item := range types {
			if item.code == row.InspectionTypeCode {
				selected = true
				break
			}
		}
		if !selected {
			continue
		}
		scope := permission.Scope{AcademicYearID: version.Content.Period.AcademicYearID, SemesterID: input.SemesterID, WeekID: input.WeekID, InspectionTypeID: row.InspectionTypeID, DepartmentID: row.DepartmentID}
		if !permission.AllowsWholeScope(effective.Permissions, permission.ActionReportGenerate, scope) {
			return File{}, nil, reporting.ErrForbidden
		}
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return File{}, nil, err
	}
	if input.SettingsVersion != settings.Version {
		return File{}, nil, ErrConfigConflict
	}
	introduction := settings.IntroductionDocument
	issueDate := ""
	if input.Format == "WORD" && input.Combined {
		weekDate, dateErr := s.repository.GetWeekDate(ctx, input.SemesterID, input.WeekID)
		if dateErr != nil {
			return File{}, nil, dateErr
		}
		if weekDate.IssueDate == "" {
			return File{}, nil, ErrIssueDateMissing
		}
		issueDate = weekDate.IssueDate
		introduction, _, err = resolveIntroductionDate(introduction, issueDate)
		if err != nil {
			return File{}, nil, err
		}
	}
	records, err := s.repository.LoadReportOutputRecords(ctx, version.ID)
	if err != nil {
		return File{}, nil, err
	}
	for _, item := range types {
		expected, observed := 0, 0
		for _, row := range version.Content.Rows {
			if row.InspectionTypeCode == item.code {
				expected += row.RecordCount
			}
		}
		for _, record := range records {
			if record.TypeCode == item.code {
				observed++
			}
		}
		if expected != observed {
			return File{}, nil, ErrInvalidGeneratedFile
		}
	}
	var data []byte
	if input.Format == "EXCEL" {
		data, err = RenderExcel(version.Content, records, input.TypeCodes)
	} else {
		data, err = RenderWordDocument(version.Content, records, input.TypeCodes, introduction, input.Combined)
	}
	if err != nil {
		return File{}, nil, err
	}
	if err := ValidateGeneratedFile(input.Format, data); err != nil {
		return File{}, nil, err
	}
	output := File{ReportVersionID: version.ID, Format: input.Format, TypeCodes: append([]string(nil), input.TypeCodes...), Combined: input.Combined, Status: "AVAILABLE", SettingsVersion: settings.Version, LayoutVersion: "WORD_V2", IssueDate: issueDate}
	if err := s.repository.RecordTransientOutput(ctx, output, len(data), metadata); err != nil {
		return File{}, nil, err
	}
	return output, data, nil
}
func (s *Service) List(ctx context.Context, actorID, semesterID, weekID string, versionNo int) ([]File, error) {
	version, err := s.version(ctx, actorID, semesterID, weekID, versionNo)
	if err != nil {
		return nil, err
	}
	return s.repository.ListReportOutputs(ctx, version.ID)
}
func (s *Service) Download(ctx context.Context, actorID, semesterID, weekID string, versionNo int, outputID string, metadata audit.Metadata) (File, []byte, error) {
	if _, err := uuid.Parse(outputID); err != nil {
		return File{}, nil, ErrInvalidSelection
	}
	version, err := s.version(ctx, actorID, semesterID, weekID, versionNo)
	if err != nil {
		return File{}, nil, err
	}
	output, err := s.repository.GetReportOutput(ctx, outputID)
	if err != nil {
		return File{}, nil, err
	}
	if output.ReportVersionID != version.ID || output.Status != "AVAILABLE" || strings.TrimSpace(output.FileID) == "" {
		return File{}, nil, ErrUnavailable
	}
	_, content, err := s.files.ReadVerified(ctx, output.FileID, metadata)
	if err != nil {
		return File{}, nil, err
	}
	return output, content, nil
}
