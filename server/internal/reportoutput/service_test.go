package reportoutput

import (
	"context"
	"errors"
	"io"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/storage"
)

type outputRepositoryStub struct {
	settings Settings
	output   File
	starts   int
	previews int
	records  []reporting.Record
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

func TestPreviewAuditsOnlyValidatedOutput(t *testing.T) {
	repository := &outputRepositoryStub{}
	service := NewService(repository, nil, nil, nil)
	if _, err := service.PreviewWord(context.Background(), legacyIntroduction("虚构说明"), []string{"INVALID"}, audit.Metadata{}); err == nil || repository.previews != 0 {
		t.Fatal("invalid preview must not be audited as a generated file")
	}
	if _, err := service.PreviewWord(context.Background(), legacyIntroduction("虚构说明"), []string{"LATE_RETURN"}, audit.Metadata{}); err != nil || repository.previews != 1 {
		t.Fatalf("valid preview audit count=%d err=%v", repository.previews, err)
	}
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
	repo := &outputRepositoryStub{settings: Settings{Introduction: "说明", Version: 1}, records: []reporting.Record{{TypeCode: "LATE_RETURN", StudentName: "虚构学生"}}}
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
				repo := &outputRepositoryStub{settings: Settings{Introduction: "说明", Version: 1}}
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
