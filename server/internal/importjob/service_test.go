package importjob

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/storage"
)

type memoryImportRepository struct {
	batch           Batch
	rows            []ImportRow
	transitions     []string
	replaceErr      error
	confirmErr      error
	confirmCalls    int
	confirmMutation audit.Mutation
	createCalls     int
	createErr       error
	listCalls       int
	listStatus      string
}

func (r *memoryImportRepository) CreateImportBatch(_ context.Context, batch Batch) error {
	r.createCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.batch = batch
	return nil
}

func (r *memoryImportRepository) FindPublishedImportTemplate(context.Context, string) (Template, error) {
	return Template{}, ErrBatchNotFound
}

func (r *memoryImportRepository) ListImportRows(_ context.Context, _ string, _, _ int, validationStatus string) (ImportRowPage, error) {
	r.listCalls++
	r.listStatus = validationStatus
	return ImportRowPage{Items: []ImportRow{}}, nil
}

func (r *memoryImportRepository) FindImportBatch(_ context.Context, id string) (Batch, error) {
	if r.batch.ID != id {
		return Batch{}, ErrBatchNotFound
	}
	return r.batch, nil
}

func (r *memoryImportRepository) UpdateImportBatchState(_ context.Context, id string, expectedVersion int, target string, failure BatchFailure) (Batch, error) {
	if r.batch.ID != id {
		return Batch{}, ErrBatchNotFound
	}
	if r.batch.Version != expectedVersion {
		return Batch{}, ErrBatchVersionConflict
	}
	if err := ValidateBatchTransition(r.batch.Status, target); err != nil {
		return Batch{}, err
	}
	r.batch.Status = target
	r.batch.Version++
	r.batch.FailureCode = failure.Code
	r.batch.FailureMessage = failure.Message
	r.transitions = append(r.transitions, target)
	return r.batch, nil
}

func (r *memoryImportRepository) ReplaceImportPreview(_ context.Context, batch Batch, rows []ImportRow) (Batch, error) {
	if r.replaceErr != nil {
		return Batch{}, r.replaceErr
	}
	if r.batch.Version != batch.Version || r.batch.Status != BatchStatusParsing {
		return Batch{}, ErrBatchVersionConflict
	}
	r.rows = append([]ImportRow(nil), rows...)
	r.batch.Status = BatchStatusPreviewReady
	r.batch.TotalRows = len(rows)
	for _, row := range rows {
		if row.ValidationStatus == RowStatusValid {
			r.batch.ValidRows++
		} else {
			r.batch.InvalidRows++
		}
	}
	r.batch.Version++
	r.transitions = append(r.transitions, BatchStatusPreviewReady)
	return r.batch, nil
}

func (r *memoryImportRepository) ConfirmAttendanceImport(_ context.Context, input ConfirmBatchInput, mutation audit.Mutation) (ConfirmResult, error) {
	r.confirmCalls++
	r.confirmMutation = mutation
	if r.confirmErr != nil {
		return ConfirmResult{}, r.confirmErr
	}
	if r.batch.Version != input.ExpectedVersion {
		return ConfirmResult{}, ErrBatchVersionConflict
	}
	r.batch.Status = BatchStatusConfirmed
	r.batch.ImportedRows = r.batch.ValidRows
	r.batch.ConfirmIdempotencyKey = input.IdempotencyKey
	r.batch.Version++
	return ConfirmResult{BatchID: r.batch.ID, Status: r.batch.Status, ImportedRows: r.batch.ImportedRows, Version: r.batch.Version}, nil
}

type fakeVerifiedFileReader struct {
	file    storage.File
	content []byte
	err     error
	calls   int
}

func (r *fakeVerifiedFileReader) ReadVerified(_ context.Context, _ string, _ audit.Metadata) (storage.File, []byte, error) {
	r.calls++
	return r.file, r.content, r.err
}

type fakeWorkbookParser struct {
	workbook ParsedWorkbook
	err      error
	calls    int
}

func (p *fakeWorkbookParser) Parse(_ string, _ []byte) (ParsedWorkbook, error) {
	p.calls++
	return p.workbook, p.err
}

type fakeAttendanceValidator struct {
	result AttendanceValidationResult
	err    error
	calls  int
}

func (v *fakeAttendanceValidator) Validate(_ context.Context, _ AttendanceImportScope, _ []ParsedRow) (AttendanceValidationResult, error) {
	v.calls++
	return v.result, v.err
}

func previewBatch(status string) Batch {
	location := time.FixedZone("CST", 8*60*60)
	return Batch{
		ID: "batch-1", InspectionTypeID: "inspection-attendance", SemesterID: "semester-1", WeekID: "week-1",
		DepartmentID: "department-study", OriginalFileID: "file-1", TemplateID: "template-1", TemplateVersion: 1,
		Status: status, Version: 1, CreatedBy: "account-1",
		WeekStart: time.Date(2026, 6, 15, 0, 0, 0, 0, location), WeekEnd: time.Date(2026, 6, 21, 0, 0, 0, 0, location), Location: location,
	}
}

func TestListRowsValidatesAndForwardsValidationStatus(t *testing.T) {
	repository := &memoryImportRepository{}
	service := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{})

	if _, err := service.ListRows(context.Background(), "batch-1", 1, 20, RowStatusInvalid); err != nil {
		t.Fatalf("list invalid rows: %v", err)
	}
	if repository.listCalls != 1 || repository.listStatus != RowStatusInvalid {
		t.Fatalf("repository filter calls=%d status=%q", repository.listCalls, repository.listStatus)
	}

	if _, err := service.ListRows(context.Background(), "batch-1", 1, 20, "BROKEN"); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("invalid validation status error=%v", err)
	}
	if repository.listCalls != 1 {
		t.Fatalf("invalid filter reached repository: calls=%d", repository.listCalls)
	}
}

func TestImportBatchStateMachineAllowsOnlyExplicitTransitions(t *testing.T) {
	allowed := [][2]string{
		{BatchStatusUploaded, BatchStatusParsing}, {BatchStatusUploaded, BatchStatusCancelled},
		{BatchStatusParsing, BatchStatusPreviewReady}, {BatchStatusParsing, BatchStatusParseFailed},
		{BatchStatusParseFailed, BatchStatusParsing}, {BatchStatusParseFailed, BatchStatusCancelled},
		{BatchStatusPreviewReady, BatchStatusConfirming}, {BatchStatusPreviewReady, BatchStatusCancelled},
		{BatchStatusConfirming, BatchStatusConfirmed}, {BatchStatusConfirming, BatchStatusConfirmFailed},
		{BatchStatusConfirmFailed, BatchStatusConfirming}, {BatchStatusConfirmFailed, BatchStatusCancelled},
	}
	for _, transition := range allowed {
		if err := ValidateBatchTransition(transition[0], transition[1]); err != nil {
			t.Errorf("transition %s -> %s: %v", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]string{
		{BatchStatusUploaded, BatchStatusConfirmed}, {BatchStatusParsing, BatchStatusCancelled},
		{BatchStatusPreviewReady, BatchStatusParsing}, {BatchStatusConfirmed, BatchStatusCancelled},
		{BatchStatusCancelled, BatchStatusParsing}, {"UNKNOWN", BatchStatusParsing},
	} {
		if !errors.Is(ValidateBatchTransition(transition[0], transition[1]), ErrInvalidBatchTransition) {
			t.Errorf("transition %s -> %s unexpectedly allowed", transition[0], transition[1])
		}
	}
}

func TestCreateBatchRequiresVerifiedAvailableUploadMetadata(t *testing.T) {
	serviceFor := func(repository *memoryImportRepository) *Service {
		return NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{})
	}
	valid := CreateBatchInput{
		InspectionTypeID: "inspection-attendance", SemesterID: "semester-1", WeekID: "week-1", DepartmentID: "department-study",
		SourceFile: storage.File{ID: "file-1", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)},
		TemplateID: "template-1", TemplateVersion: 1, CreatedBy: "account-1",
	}
	repository := &memoryImportRepository{}
	batch, err := serviceFor(repository).CreateBatch(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.Status != BatchStatusUploaded || batch.Version != 1 || batch.OriginalFileID != "file-1" || repository.createCalls != 1 {
		t.Fatalf("batch = %#v, calls = %d", batch, repository.createCalls)
	}

	for _, status := range []string{storage.StatusPending, storage.StatusFailed, storage.StatusMissing} {
		t.Run(status, func(t *testing.T) {
			repository := &memoryImportRepository{}
			input := valid
			input.SourceFile.Status = status
			if _, err := serviceFor(repository).CreateBatch(context.Background(), input); !errors.Is(err, ErrSourceFileNotReady) {
				t.Fatalf("error = %v", err)
			}
			if repository.createCalls != 0 {
				t.Fatal("batch persisted for unavailable upload")
			}
		})
	}
}

func TestCreateBatchRejectsIncompleteContextAndPropagatesPersistenceFailure(t *testing.T) {
	repository := &memoryImportRepository{}
	service := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{})
	if _, err := service.CreateBatch(context.Background(), CreateBatchInput{}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("incomplete error = %v", err)
	}
	repository.createErr = errors.New("database unavailable")
	_, err := service.CreateBatch(context.Background(), CreateBatchInput{
		InspectionTypeID: "type", SemesterID: "semester", WeekID: "week", DepartmentID: "department",
		SourceFile: storage.File{ID: "file", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)},
		TemplateID: "template", TemplateVersion: 1, CreatedBy: "actor",
	})
	if err == nil || repository.createCalls != 1 {
		t.Fatalf("persistence error/calls = %v / %d", err, repository.createCalls)
	}
}

func TestParseBatchStoresPreviewRowsAndExplicitStates(t *testing.T) {
	repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
	reader := &fakeVerifiedFileReader{file: storage.File{ID: "file-1", OriginalName: "attendance.xlsx", Status: storage.StatusAvailable, SHA256: "verified"}, content: []byte("xlsx")}
	parser := &fakeWorkbookParser{workbook: ParsedWorkbook{TemplateCode: "ATTENDANCE_IMPORT", TemplateVersion: 1, Rows: []ParsedRow{{RowNo: 5, Values: map[string]string{"student_no": "S20260001"}}}}}
	validator := &fakeAttendanceValidator{result: AttendanceValidationResult{Rows: []AttendanceValidationRow{{RowNo: 5}}, ValidCount: 1}}
	service := NewService(repository, reader, parser, validator)

	batch, err := service.ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 1, AuditMetadata: audit.Metadata{ActorID: "account-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != BatchStatusPreviewReady || batch.TotalRows != 1 || batch.ValidRows != 1 || batch.Version != 3 {
		t.Fatalf("batch = %#v", batch)
	}
	if len(repository.transitions) != 2 || repository.transitions[0] != BatchStatusParsing || repository.transitions[1] != BatchStatusPreviewReady {
		t.Fatalf("transitions = %#v", repository.transitions)
	}
	if reader.calls != 1 || parser.calls != 1 || validator.calls != 1 || len(repository.rows) != 1 {
		t.Fatalf("calls rows = reader %d parser %d validator %d rows %#v", reader.calls, parser.calls, validator.calls, repository.rows)
	}
}

func TestParseBatchMarksStableFailureWhenFileIsUnavailable(t *testing.T) {
	repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
	reader := &fakeVerifiedFileReader{err: storage.ErrFileUnavailable}
	parser := &fakeWorkbookParser{}
	service := NewService(repository, reader, parser, &fakeAttendanceValidator{})

	_, err := service.ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 1})
	if !errors.Is(err, storage.ErrFileUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if repository.batch.Status != BatchStatusParseFailed || repository.batch.FailureCode != string(ErrorSourceFileUnavailable) || repository.batch.FailureMessage == "" {
		t.Fatalf("failed batch = %#v", repository.batch)
	}
	if parser.calls != 0 {
		t.Fatal("parser called for unavailable file")
	}
}

func TestParseBatchRejectsDuplicateParsingAndOldVersion(t *testing.T) {
	reader := &fakeVerifiedFileReader{}
	parser := &fakeWorkbookParser{}
	validator := &fakeAttendanceValidator{}

	repository := &memoryImportRepository{batch: previewBatch(BatchStatusPreviewReady)}
	_, err := NewService(repository, reader, parser, validator).ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 1})
	if !errors.Is(err, ErrInvalidBatchTransition) {
		t.Fatalf("duplicate parse error = %v", err)
	}

	repository = &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
	_, err = NewService(repository, reader, parser, validator).ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 2})
	if !errors.Is(err, ErrBatchVersionConflict) {
		t.Fatalf("version error = %v", err)
	}
}

func TestCancelBatchAllowsOnlyRecoverableIdleStates(t *testing.T) {
	for _, status := range []string{BatchStatusUploaded, BatchStatusParseFailed, BatchStatusPreviewReady, BatchStatusConfirmFailed} {
		repository := &memoryImportRepository{batch: previewBatch(status)}
		batch, err := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{}).CancelBatch(context.Background(), "batch-1", 1, audit.Metadata{ActorID: "actor-1"})
		if err != nil || batch.Status != BatchStatusCancelled {
			t.Errorf("cancel %s = %#v, %v", status, batch, err)
		}
	}
	for _, status := range []string{BatchStatusParsing, BatchStatusConfirming, BatchStatusConfirmed, BatchStatusCancelled} {
		repository := &memoryImportRepository{batch: previewBatch(status)}
		_, err := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{}).CancelBatch(context.Background(), "batch-1", 1, audit.Metadata{ActorID: "actor-1"})
		if !errors.Is(err, ErrInvalidBatchTransition) {
			t.Errorf("cancel %s error = %v", status, err)
		}
	}
}

func TestParseBatchMarksSafeFailureWhenPreviewTransactionRollsBack(t *testing.T) {
	repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded), replaceErr: errors.New("transaction rolled back")}
	reader := &fakeVerifiedFileReader{file: storage.File{ID: "file-1", OriginalName: "attendance.xlsx", Status: storage.StatusAvailable}, content: []byte("xlsx")}
	parser := &fakeWorkbookParser{workbook: ParsedWorkbook{TemplateVersion: 1}}
	validator := &fakeAttendanceValidator{result: AttendanceValidationResult{}}
	_, err := NewService(repository, reader, parser, validator).ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 1})
	if err == nil {
		t.Fatal("preview transaction failure was ignored")
	}
	if repository.batch.Status != BatchStatusParseFailed || repository.batch.FailureMessage == "transaction rolled back" {
		t.Fatalf("unsafe failure state = %#v", repository.batch)
	}
}

func TestParseBatchRejectsValidatorResultThatDoesNotMatchParsedRows(t *testing.T) {
	repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
	reader := &fakeVerifiedFileReader{file: storage.File{ID: "file-1", OriginalName: "attendance.xlsx", Status: storage.StatusAvailable}, content: []byte("xlsx")}
	parser := &fakeWorkbookParser{workbook: ParsedWorkbook{TemplateVersion: 1, Rows: []ParsedRow{{RowNo: 5, Values: map[string]string{"student_no": "S20260001"}}}}}
	validator := &fakeAttendanceValidator{result: AttendanceValidationResult{Rows: nil}}

	_, err := NewService(repository, reader, parser, validator).ParseBatch(context.Background(), ParseBatchInput{BatchID: "batch-1", ExpectedVersion: 1})
	if !errors.Is(err, ErrValidationResultMismatch) {
		t.Fatalf("error = %v, want ErrValidationResultMismatch", err)
	}
	if repository.batch.Status != BatchStatusParseFailed || len(repository.rows) != 0 {
		t.Fatalf("batch/rows = %#v / %#v", repository.batch, repository.rows)
	}
}

func confirmableBatch() Batch {
	batch := previewBatch(BatchStatusPreviewReady)
	batch.TotalRows, batch.ValidRows, batch.InvalidRows = 2, 2, 0
	return batch
}

func confirmInput() ConfirmBatchInput {
	return ConfirmBatchInput{
		BatchID: "batch-1", ExpectedVersion: 1, IdempotencyKey: "confirm-request-1",
		Scope:         ConfirmScope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "inspection-attendance", DepartmentID: "department-study", ActorID: "account-1"},
		AuditMetadata: audit.Metadata{ActorID: "account-1", RequestID: "request-1"},
	}
}

func TestConfirmBatchAcceptsZeroErrorPreviewAndEmitsSafeAudit(t *testing.T) {
	repository := &memoryImportRepository{batch: confirmableBatch()}
	service := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{})

	result, err := service.ConfirmBatch(context.Background(), confirmInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BatchStatusConfirmed || result.ImportedRows != 2 || result.Version != 2 || repository.confirmCalls != 1 {
		t.Fatalf("result = %#v, calls = %d", result, repository.confirmCalls)
	}
	encoded, err := json.Marshal(repository.confirmMutation)
	if err != nil {
		t.Fatal(err)
	}
	auditText := string(encoded)
	for _, forbidden := range []string{"rawData", "raw_data_json", "studentName", "studentNo", "测试学生甲", "S20260001"} {
		if strings.Contains(auditText, forbidden) {
			t.Fatalf("audit leaks %q: %s", forbidden, auditText)
		}
	}
	for _, required := range []string{"batch-1", "semester-1", "week-1", "department-study", "inspection-attendance"} {
		if !strings.Contains(auditText, required) {
			t.Fatalf("audit misses %q: %s", required, auditText)
		}
	}
}

func TestConfirmBatchRejectsErrorsOldVersionAndScopeMismatch(t *testing.T) {
	tests := []struct {
		name  string
		batch Batch
		input ConfirmBatchInput
		want  error
	}{
		{name: "preview has errors", batch: func() Batch { b := confirmableBatch(); b.InvalidRows = 1; return b }(), input: confirmInput(), want: ErrBatchHasErrors},
		{name: "old version", batch: confirmableBatch(), input: func() ConfirmBatchInput { v := confirmInput(); v.ExpectedVersion = 2; return v }(), want: ErrBatchVersionConflict},
		{name: "department outside scope", batch: confirmableBatch(), input: func() ConfirmBatchInput { v := confirmInput(); v.Scope.DepartmentID = "other-department"; return v }(), want: ErrBatchScopeMismatch},
		{name: "class outside scope", batch: confirmableBatch(), input: func() ConfirmBatchInput { v := confirmInput(); v.Scope.ClassID = "other-class"; return v }(), want: ErrBatchScopeMismatch},
		{name: "actor cannot confirm another upload", batch: confirmableBatch(), input: func() ConfirmBatchInput { v := confirmInput(); v.Scope.ActorID = "other-account"; return v }(), want: ErrBatchScopeMismatch},
		{name: "missing idempotency key", batch: confirmableBatch(), input: func() ConfirmBatchInput { v := confirmInput(); v.IdempotencyKey = " "; return v }(), want: ErrInvalidIdempotencyKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryImportRepository{batch: test.batch}
			_, err := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{}).ConfirmBatch(context.Background(), test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repository.confirmCalls != 0 {
				t.Fatal("repository confirm called after service validation failed")
			}
		})
	}
}

func TestConfirmBatchIdempotencyReturnsSameResultAndRejectsDifferentKey(t *testing.T) {
	confirmed := confirmableBatch()
	confirmed.Status, confirmed.ImportedRows, confirmed.Version = BatchStatusConfirmed, 2, 2
	confirmed.ConfirmIdempotencyKey = "confirm-request-1"
	service := NewService(&memoryImportRepository{batch: confirmed}, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{})

	result, err := service.ConfirmBatch(context.Background(), func() ConfirmBatchInput { v := confirmInput(); v.ExpectedVersion = 1; return v }())
	if err != nil || result.Status != BatchStatusConfirmed || result.ImportedRows != 2 || result.Version != 2 {
		t.Fatalf("idempotent result = %#v, %v", result, err)
	}
	_, err = service.ConfirmBatch(context.Background(), func() ConfirmBatchInput {
		v := confirmInput()
		v.ExpectedVersion = 2
		v.IdempotencyKey = "different-request"
		return v
	}())
	if !errors.Is(err, ErrBatchAlreadyConfirmed) {
		t.Fatalf("different key error = %v", err)
	}
}

func TestConfirmBatchLeavesServiceViewUnchangedWhenTransactionRollsBack(t *testing.T) {
	repository := &memoryImportRepository{batch: confirmableBatch(), confirmErr: errors.New("attendance detail insert failed")}
	_, err := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{}).ConfirmBatch(context.Background(), confirmInput())
	if err == nil {
		t.Fatal("confirmation transaction failure was ignored")
	}
	if repository.batch.Status != BatchStatusPreviewReady || repository.batch.ImportedRows != 0 || repository.batch.ConfirmIdempotencyKey != "" {
		t.Fatalf("batch changed after rollback = %#v", repository.batch)
	}
}

func (r *memoryImportRepository) CancelImportBatch(ctx context.Context, batch Batch, metadata audit.Metadata) (Batch, error) {
	return r.UpdateImportBatchState(ctx, batch.ID, batch.Version, BatchStatusCancelled, BatchFailure{})
}

func TestCancelBatchRequiresAuditActor(t *testing.T) {
	repository := &memoryImportRepository{batch: previewBatch(BatchStatusUploaded)}
	_, err := NewService(repository, &fakeVerifiedFileReader{}, &fakeWorkbookParser{}, &fakeAttendanceValidator{}).CancelBatch(context.Background(), "batch-1", 1, audit.Metadata{})
	if !errors.Is(err, ErrInvalidBatch) || len(repository.transitions) != 0 {
		t.Fatal("missing actor accepted", err)
	}
}
