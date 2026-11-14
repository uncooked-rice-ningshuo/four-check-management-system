package reportoutput

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/storage"
)

type outputRepositoryStub struct {
	settings         Settings
	weekDate         WeekDate
	output           File
	starts           int
	previews         int
	transientOutputs int
	records          []reporting.Record
}

func (r *outputRepositoryStub) ListWeekDates(context.Context) ([]WeekDate, error) {
	return []WeekDate{r.weekDate}, nil
}
func (r *outputRepositoryStub) GetWeekDate(context.Context, string, string) (WeekDate, error) {
	return r.weekDate, nil
}
func (r *outputRepositoryStub) SaveWeekDate(_ context.Context, input WeekDate, _ audit.Metadata) (WeekDate, error) {
	input.Version++
	r.weekDate = input
	return input, nil
}

func TestGenerateRequiresAndFreezesConfiguredIssueDate(t *testing.T) {
	period := reporting.PeriodOption{SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", SemesterName: "示例学期", WeekNo: 8}
	version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeID: "type", InspectionTypeCode: "LATE_RETURN", DepartmentID: "department", DataStatus: "NO_CONFIRMED_DATA"}}}}
	repo := &outputRepositoryStub{settings: Settings{Version: 1, IntroductionDocument: json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"date","textAlign":"right"},"content":[{"type":"text","text":"{reportDate}"}]}]}`)}}
	files := &outputFilesStub{}
	service := NewService(repo, files, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}})
	input := GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}
	if _, err := service.Generate(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, ErrIssueDateMissing) || repo.starts != 0 {
		t.Fatalf("missing issue date persisted output: err=%v starts=%d", err, repo.starts)
	}
	repo.weekDate = WeekDate{IssueDate: "2025-06-01", Version: 1}
	output, err := service.Generate(context.Background(), "actor", input, audit.Metadata{})
	if err != nil || output.Status != "AVAILABLE" || output.IssueDate != repo.weekDate.IssueDate || repo.output.IssueDate != repo.weekDate.IssueDate || !strings.Contains(string(repo.output.IntroductionSnapshot), "2025年6月1日") {
		t.Fatalf("issue date not frozen: output=%+v err=%v", output, err)
	}
	repo.weekDate.IssueDate = "2025-06-08"
	if output.IssueDate != "2025-06-01" || !strings.Contains(string(repo.output.IntroductionSnapshot), "2025年6月1日") {
		t.Fatal("changing a week date modified an existing output snapshot")
	}
}

func TestSaveWeekDateRejectsInvalidInput(t *testing.T) {
	service := NewService(&outputRepositoryStub{}, nil, nil, nil)
	valid := WeekDate{SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", IssueDate: "2026-03-30", Version: 0}
	for _, input := range []WeekDate{
		{SemesterID: "invalid", WeekID: valid.WeekID, IssueDate: valid.IssueDate},
		{SemesterID: valid.SemesterID, WeekID: "invalid", IssueDate: valid.IssueDate},
		{SemesterID: valid.SemesterID, WeekID: valid.WeekID, IssueDate: valid.IssueDate, Version: -1},
	} {
		if _, err := service.SaveWeekDate(context.Background(), input, audit.Metadata{}); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("invalid input accepted: %+v err=%v", input, err)
		}
	}
	invalidDate := valid
	invalidDate.IssueDate = "2026-02-30"
	if _, err := service.SaveWeekDate(context.Background(), invalidDate, audit.Metadata{}); !errors.Is(err, ErrInvalidWeekDate) {
		t.Fatalf("invalid date accepted: %v", err)
	}
}

func (r *outputRepositoryStub) GetOutputSettings(context.Context) (Settings, error) {
	return r.settings, nil
}
func (r *outputRepositoryStub) ReplaceOutputSettings(_ context.Context, s Settings, _ audit.Metadata) (Settings, error) {
	return s, nil
}
func (r *outputRepositoryStub) StartReportOutput(_ context.Context, f File, _ string, _ audit.Metadata) error {
	r.output = f
	r.starts++
	return nil
}
func (r *outputRepositoryStub) FinishReportOutput(_ context.Context, _ string, fileID string, _ audit.Metadata) error {
	r.output.FileID = fileID
	r.output.Status = "AVAILABLE"
	return nil
}
func (r *outputRepositoryStub) FailReportOutput(_ context.Context, _ string, _ string, _ audit.Metadata) error {
	r.output.Status = "FAILED"
	return nil
}
func (r *outputRepositoryStub) ListReportOutputs(context.Context, string) ([]File, error) {
	return []File{r.output}, nil
}
func (r *outputRepositoryStub) GetReportOutput(context.Context, string) (File, error) {
	return r.output, nil
}
func (r *outputRepositoryStub) LoadReportOutputRecords(context.Context, string) ([]reporting.Record, error) {
	return r.records, nil
}
func (r *outputRepositoryStub) ListOutputTypes(context.Context) ([]OutputType, error) {
	return []OutputType{{Code: "LATE_RETURN", Name: "晚归"}}, nil
}
func (r *outputRepositoryStub) RecordOutputPreview(context.Context, []string, audit.Metadata) error {
	r.previews++
	return nil
}
func (r *outputRepositoryStub) RecordTransientOutput(context.Context, File, int, audit.Metadata) error {
	r.transientOutputs++
	return nil
}

func TestGenerateDownloadDoesNotStoreFiles(t *testing.T) {
	period := reporting.PeriodOption{SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", SemesterName: "示例学期", WeekNo: 8}
	version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeID: "type", InspectionTypeCode: "LATE_RETURN", DepartmentID: "department", DataStatus: "NO_CONFIRMED_DATA"}}}}
	repo := &outputRepositoryStub{settings: Settings{Version: 1}, weekDate: WeekDate{IssueDate: "2025-06-01"}}
	files := &outputFilesStub{}
	input := GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}
	workflow := outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}}
	service := NewService(repo, files, outputPermissionsStub{}, workflow)
	if _, _, err := service.GenerateDownload(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, reporting.ErrForbidden) || repo.transientOutputs != 0 {
		t.Fatalf("unauthorized output: %v", err)
	}
	service = NewService(repo, files, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, workflow)
	output, data, err := service.GenerateDownload(context.Background(), "actor", input, audit.Metadata{})
	if err != nil || output.Status != "AVAILABLE" || ValidateGeneratedFile("WORD", data) != nil || repo.starts != 0 || len(files.content) != 0 || repo.transientOutputs != 1 {
		t.Fatalf("transient output=%+v bytes=%d starts=%d stores=%d audits=%d err=%v", output, len(data), repo.starts, len(files.content), repo.transientOutputs, err)
	}
	input.Format = "EXCEL"
	output, data, err = service.GenerateDownload(context.Background(), "actor", input, audit.Metadata{})
	if err != nil || output.Format != "EXCEL" || ValidateGeneratedFile("EXCEL", data) != nil || repo.starts != 0 || len(files.content) != 0 || repo.transientOutputs != 2 {
		t.Fatalf("transient Excel output=%+v bytes=%d starts=%d stores=%d audits=%d err=%v", output, len(data), repo.starts, len(files.content), repo.transientOutputs, err)
	}
	version.Content.Rows[0].RecordCount = 1
	service = NewService(repo, files, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}})
	if _, _, err := service.GenerateDownload(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, ErrInvalidGeneratedFile) || repo.transientOutputs != 2 || repo.starts != 0 {
		t.Fatalf("inconsistent source generated or audited output: err=%v audits=%d starts=%d", err, repo.transientOutputs, repo.starts)
	}
}

func TestGenerateDownloadResolvesConfiguredWeekDateInCover(t *testing.T) {
	period := reporting.PeriodOption{SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", SemesterName: "示例学期", WeekNo: 8}
	version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeID: "type", InspectionTypeCode: "LATE_RETURN", DepartmentID: "department", DataStatus: "NO_CONFIRMED_DATA"}}}}
	repo := &outputRepositoryStub{settings: Settings{Version: 1, IntroductionDocument: json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"date","textAlign":"right"},"content":[{"type":"text","text":"{reportDate}"}]}]}`)}, weekDate: WeekDate{IssueDate: "2026-11-14", Version: 1}}
	service := NewService(repo, &outputFilesStub{}, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}})
	_, data, err := service.GenerateDownload(context.Background(), "actor", GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "2026年11月14日") || strings.Contains(string(body), "{reportDate}") {
			t.Fatalf("configured date missing from generated Word: %s", body)
		}
		return
	}
	t.Fatal("document.xml missing")
}

func TestPreviewAuditsOnlyValidatedOutput(t *testing.T) {
	repository := &outputRepositoryStub{}
	service := NewService(repository, nil, nil, nil)
	if _, err := service.PreviewWord(context.Background(), legacyIntroduction("虚构说明"), []string{"INVALID"}, "", "", audit.Metadata{}); err == nil || repository.previews != 0 {
		t.Fatal("invalid preview must not be audited as a generated file")
	}
	if _, err := service.PreviewWord(context.Background(), legacyIntroduction("虚构说明"), []string{"LATE_RETURN"}, "", "", audit.Metadata{}); err != nil || repository.previews != 1 {
		t.Fatalf("valid preview audit count=%d err=%v", repository.previews, err)
	}
}

func TestPreviewWordUsesConfiguredWeekDate(t *testing.T) {
	repository := &outputRepositoryStub{weekDate: WeekDate{SemesterName: "2026年秋季学期", WeekNo: 10, IssueDate: "2026-11-14"}}
	service := NewService(repository, nil, nil, nil)
	document := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"date","textAlign":"right"},"content":[{"type":"text","text":"{reportDate}"}]}]}`)
	semesterID := "10000000-0000-0000-0000-000000000001"
	weekID := "10000000-0000-0000-0000-000000000002"
	if _, err := service.PreviewWord(context.Background(), document, []string{"LATE_RETURN"}, semesterID, "", audit.Metadata{}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("incomplete period accepted: %v", err)
	}
	if _, err := service.PreviewWord(context.Background(), document, []string{"LATE_RETURN"}, "invalid", weekID, audit.Metadata{}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid period accepted: %v", err)
	}
	data, err := service.PreviewWord(context.Background(), document, []string{"LATE_RETURN"}, semesterID, weekID, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		bodyFile, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(bodyFile)
		bodyFile.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "2026年11月14日") || strings.Contains(string(body), "2025年6月1日") {
			t.Fatalf("preview did not use configured date: %s", body)
		}
		return
	}
	t.Fatal("Word document.xml missing")
}

type outputFilesStub struct {
	content []byte
	reads   int
}

func (f *outputFilesStub) Store(_ context.Context, input storage.StoreInput, _ audit.Metadata) (storage.File, error) {
	f.content = make([]byte, input.Size)
	_, err := io.ReadFull(input.Body, f.content)
	return storage.File{ID: "stored-file"}, err
}
func (f *outputFilesStub) ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error) {
	f.reads++
	return storage.File{ID: "stored-file"}, f.content, nil
}

type outputPermissionsStub struct {
	permissions []permission.EffectivePermission
}

type officeOutputPermissionsStub struct{ outputPermissionsStub }

func (p officeOutputPermissionsStub) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{DepartmentCode: "OFFICE", OrganizationRoleCode: "MEMBER", Permissions: p.permissions}, nil
}

func TestOfficeMemberCannotGenerateFinalizedReportFile(t *testing.T) {
	period := reporting.PeriodOption{SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002"}
	version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeCode: "LATE_RETURN"}}}}
	workflow := reporting.ReportWorkflow{Report: &reporting.ReportState{Status: reporting.ReportStatusFinalized, VersionNo: 1}, Versions: []reporting.ReportVersion{version}}
	repo := &outputRepositoryStub{settings: Settings{Version: 1}}
	service := NewService(repo, &outputFilesStub{}, officeOutputPermissionsStub{outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}}, outputWorkflowStub{workflow})
	input := GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}
	if _, err := service.Generate(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, reporting.ErrForbidden) || repo.starts != 0 {
		t.Fatalf("finalized file generation was not denied before write: err=%v starts=%d", err, repo.starts)
	}
	if _, _, err := service.GenerateDownload(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, reporting.ErrForbidden) || repo.transientOutputs != 0 {
		t.Fatalf("finalized direct download was not denied before audit: err=%v audits=%d", err, repo.transientOutputs)
	}
}

func (p outputPermissionsStub) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.permissions}, nil
}

type outputWorkflowStub struct{ value reporting.ReportWorkflow }

func (w outputWorkflowStub) Workflow(context.Context, string, string, string) (reporting.ReportWorkflow, error) {
	return w.value, nil
}
func TestGenerateAndDownloadUseFrozenVersion(t *testing.T) {
	period := reporting.PeriodOption{AcademicYearID: "year", SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", SemesterName: "示例学期", WeekNo: 8}
	version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeID: "type", InspectionTypeCode: "LATE_RETURN", DepartmentID: "department", DataStatus: "CONFIRMED_RECORDS", RecordCount: 1}}}}
	repo := &outputRepositoryStub{settings: Settings{Introduction: "说明", Version: 1}, weekDate: WeekDate{IssueDate: "2025-06-01", Version: 1}, records: []reporting.Record{{TypeCode: "LATE_RETURN", StudentName: "虚构学生"}}}
	files := &outputFilesStub{}
	input := GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}
	workflow := outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}}
	service := NewService(repo, files, outputPermissionsStub{}, workflow)
	if _, err := service.Generate(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, reporting.ErrForbidden) {
		t.Fatalf("unauthorized generation = %v", err)
	}
	if repo.starts != 0 {
		t.Fatal("unauthorized attempt persisted")
	}
	service = NewService(repo, files, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, workflow)
	stale := input
	stale.SettingsVersion = 2
	if _, err := service.Generate(context.Background(), "actor", stale, audit.Metadata{}); !errors.Is(err, ErrConfigConflict) || repo.starts != 0 {
		t.Fatalf("stale settings generated output: %v, starts=%d", err, repo.starts)
	}
	output, err := service.Generate(context.Background(), "actor", input, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != "AVAILABLE" || output.SettingsVersion != 1 || len(repo.output.IntroductionSnapshot) == 0 || repo.starts != 1 || len(files.content) == 0 {
		t.Fatalf("output=%+v starts=%d bytes=%d", output, repo.starts, len(files.content))
	}
	_, content, err := service.Download(context.Background(), "actor", period.SemesterID, period.WeekID, 1, output.ID, audit.Metadata{})
	if err != nil || len(content) == 0 || files.reads != 1 {
		t.Fatalf("download bytes=%d reads=%d err=%v", len(content), files.reads, err)
	}
	repo.output.Status = "FAILED"
	if _, _, err := service.Download(context.Background(), "actor", period.SemesterID, period.WeekID, 1, output.ID, audit.Metadata{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("failed output download = %v", err)
	}
	repo.records = nil
	if _, err := service.Generate(context.Background(), "actor", input, audit.Metadata{}); !errors.Is(err, ErrInvalidGeneratedFile) || repo.output.Status != "FAILED" {
		t.Fatalf("missing frozen records generated output: err=%v output=%+v", err, repo.output)
	}
}

func TestGenerateFromEmptyFrozenVersion(t *testing.T) {
	period := reporting.PeriodOption{AcademicYearID: "year", SemesterID: "10000000-0000-0000-0000-000000000001", WeekID: "10000000-0000-0000-0000-000000000002", SemesterName: "示例学期", WeekNo: 8}
	for _, source := range []struct {
		name, status string
		receipts     int
	}{
		{name: "confirmed zero", status: "CONFIRMED_ZERO_RECORD_RECEIPT", receipts: 1},
		{name: "not submitted", status: "NO_CONFIRMED_DATA"},
	} {
		version := reporting.ReportVersion{ID: "version", VersionNo: 1, Content: reporting.ReportVersionContent{Period: period, Rows: []reporting.ReportCountRow{{InspectionTypeID: "type", InspectionTypeCode: "LATE_RETURN", DepartmentID: "department", DataStatus: source.status, ZeroReceiptCount: source.receipts}}}}
		for _, format := range []string{"WORD", "EXCEL"} {
			t.Run(source.name+"/"+format, func(t *testing.T) {
				repo := &outputRepositoryStub{settings: Settings{Introduction: "说明", Version: 1}, weekDate: WeekDate{IssueDate: "2025-06-01", Version: 1}}
				files := &outputFilesStub{}
				service := NewService(repo, files, outputPermissionsStub{[]permission.EffectivePermission{{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow}}}, outputWorkflowStub{reporting.ReportWorkflow{Versions: []reporting.ReportVersion{version}}})
				output, err := service.Generate(context.Background(), "actor", GenerateInput{SemesterID: period.SemesterID, WeekID: period.WeekID, VersionNo: 1, Format: format, TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}, audit.Metadata{})
				if err != nil || output.Status != "AVAILABLE" || len(files.content) == 0 {
					t.Fatalf("%s %s output=%+v bytes=%d err=%v", source.name, format, output, len(files.content), err)
				}
			})
		}
	}
}
