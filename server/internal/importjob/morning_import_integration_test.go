package importjob_test

import (
	"bytes"
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"os"
	"strings"
	"testing"
)

func TestMorningImportPostgres(t *testing.T) {
	url := os.Getenv("MORNING_IMPORT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MORNING_IMPORT_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	var typeID, departmentID string
	if err = db.Pool.QueryRow(ctx, `SELECT i.id,ds.department_id FROM inspection_types i JOIN department_inspection_scopes ds ON ds.inspection_type_id=i.id WHERE i.code='MORNING_ATTENDANCE' AND ds.enabled AND ds.can_enter ORDER BY ds.department_id LIMIT 1`).Scan(&typeID, &departmentID); err != nil {
		t.Fatal(err)
	}
	old, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("source.xlsx", fixture.workbook)
	if err != nil {
		t.Fatal(err)
	}
	content, err := importjob.BuildMorningTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for i, row := range old.Rows {
		values := []any{row.Values["student_no"], row.Values["student_name"], row.Values["class_name"], row.Values["absence_date"], "isolated morning"}
		cell, _ := excelize.CoordinatesToCellName(1, i+5)
		if err = book.SetSheetRow("早锻炼早签导入", cell, &values); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	fileID, templateID := uuid.NewString(), uuid.NewString()
	if _, err = db.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','morning/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'MORNING_ATTENDANCE_IMPORT','morning',$2,1,$3,'DRAFT',$4)`, templateID, typeID, fileID, fixture.accountID); err != nil {
		t.Fatal(err)
	}
	source := storage.File{ID: fileID, OriginalName: "source.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	service := importjob.NewInspectionImportService(db, rollbackFileReader{source, filled.Bytes()}, importjob.NewAttendanceAdapter(db), importjob.NewMorningAdapter(db), importjob.NewLateReturnAdapter(db), importjob.NewDormitoryHygieneAdapter(db), importjob.NewClassroomHygieneAdapter(db), 16<<20)
	batch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
	if err != nil || batch.ValidRows != 2 {
		t.Fatal("morning preview", batch, err)
	}
	rows, err := service.ListRows(ctx, batch.ID, 1, 20, "")
	if err != nil || len(rows.Items) != 2 || rows.Items[0].NormalizedInspection == nil {
		t.Fatal("typed preview was lost", rows, err)
	}
	studentID := rows.Items[0].NormalizedInspection.References.Students[0].ID
	input := importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID, DepartmentID: departmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}}
	if _, err = db.Pool.Exec(ctx, `UPDATE students SET enabled=false WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("stale morning reference accepted", err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE students SET enabled=true WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.ConfirmBatch(ctx, input)
	if err != nil || confirmed.ImportedRows != 2 {
		t.Fatal("morning confirmation", confirmed, err)
	}
	retry, err := service.ConfirmBatch(ctx, input)
	if err != nil || retry != confirmed {
		t.Fatal("morning confirmation not idempotent", retry, err)
	}
	var valid, wrongTable, links int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records r JOIN morning_exercise_details d ON d.record_id=r.id WHERE r.import_batch_id=$1 AND r.source='EXCEL' AND r.status='DRAFT' AND r.occurred_at IS NULL AND r.inspection_date IS NOT NULL AND r.occurrence_kind='DATE' AND d.absence_count=1`, batch.ID).Scan(&valid); err != nil || valid != 2 {
		t.Fatal("wrong morning granularity", valid, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_details a JOIN inspection_records r ON r.id=a.record_id WHERE r.import_batch_id=$1`, batch.ID).Scan(&wrongTable); err != nil || wrongTable != 0 {
		t.Fatal("course details were inserted", wrongTable, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, batch.ID).Scan(&links); err != nil || links != 2 {
		t.Fatal("missing provenance", links, err)
	}
	rollback := importjob.NewRollbackService(db, permission.NewService(db))
	result, err := rollback.Rollback(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, importjob.RollbackInput{BatchID: batch.ID, Version: confirmed.Version, Reason: "isolated morning rollback"}, audit.Metadata{})
	if err != nil || result.RolledBackRows != 2 {
		t.Fatal("morning rollback", result, err)
	}
	collegeBatch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID,
		DepartmentID: inspection.CollegeDepartmentOptionID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil || collegeBatch.DepartmentID != "" {
		t.Fatal("create college morning batch", collegeBatch, err)
	}
	collegeBatch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: collegeBatch.ID, ExpectedVersion: collegeBatch.Version})
	if err != nil || collegeBatch.ValidRows != 2 {
		t.Fatal("college morning preview", collegeBatch, err)
	}
	collegeConfirmed, err := service.ConfirmBatch(ctx, importjob.ConfirmBatchInput{BatchID: collegeBatch.ID, ExpectedVersion: collegeBatch.Version,
		IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID,
			InspectionTypeID: typeID, DepartmentID: "", ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}})
	if err != nil || collegeConfirmed.ImportedRows != 2 {
		t.Fatal("college morning confirmation", collegeConfirmed, err)
	}
	var collegeRecords int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1 AND department_id IS NULL`, collegeBatch.ID).Scan(&collegeRecords); err != nil || collegeRecords != 2 {
		t.Fatal("college morning ownership", collegeRecords, err)
	}
}
