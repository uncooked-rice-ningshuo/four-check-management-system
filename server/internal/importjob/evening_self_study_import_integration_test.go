package importjob_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func TestEveningSelfStudyImportPostgres(t *testing.T) {
	url := os.Getenv("EVENING_IMPORT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("EVENING_IMPORT_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	var typeID string
	if err = db.Pool.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code=$1`, inspection.InspectionTypeEveningSelfStudy).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter,can_view,can_review)
VALUES($1,$2,true,true,true,true) ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true,can_view=true,can_review=true`, integrationDepartmentID, typeID); err != nil {
		t.Fatal(err)
	}
	workbook, err := importjob.BuildEveningSelfStudyTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(workbook))
	if err != nil {
		t.Fatal(err)
	}
	var classroom string
	if err := db.Pool.QueryRow(ctx, `SELECT b.name || ' ' || r.room_no FROM classrooms r
JOIN teaching_buildings b ON b.id=r.teaching_building_id WHERE r.enabled AND b.enabled ORDER BY r.id LIMIT 1`).Scan(&classroom); err != nil {
		book.Close()
		t.Fatal(err)
	}
	rows, err := db.Pool.Query(ctx, `SELECT s.student_no,s.name,c.name
FROM students s JOIN classes c ON c.id=s.class_id
WHERE s.enabled AND c.enabled ORDER BY s.student_no LIMIT 2`)
	if err != nil {
		book.Close()
		t.Fatal(err)
	}
	rowCount := 0
	for rowNo := 5; rows.Next(); rowNo++ {
		var values []any = make([]any, 0, 7)
		var studentNo, studentName, className string
		if err := rows.Scan(&studentNo, &studentName, &className); err != nil {
			rows.Close()
			book.Close()
			t.Fatal(err)
		}
		values = append(values, studentNo, studentName, className, classroom, "2026-06-17", "20:15", "isolated evening record")
		cell, _ := excelize.CoordinatesToCellName(1, rowNo)
		if err := book.SetSheetRow("晚自习导入", cell, &values); err != nil {
			rows.Close()
			book.Close()
			t.Fatal(err)
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		book.Close()
		t.Fatal(err)
	}
	rows.Close()
	if rowCount != 2 {
		book.Close()
		t.Fatalf("expected two isolated students, got %d", rowCount)
	}
	var sourceFileID, templateFileID, templateID string = uuid.NewString(), uuid.NewString(), uuid.NewString()
	filled, err := book.WriteToBuffer()
	book.Close()
	if err != nil {
		t.Fatal(err)
	}
	content := filled.Bytes()
	source := storage.File{ID: sourceFileID, OriginalName: "evening-self-study.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	for _, file := range []struct {
		id, key, retention string
	}{
		{sourceFileID, "evening/source", storage.RetentionImportSource},
		{templateFileID, "evening/template", storage.RetentionTemplate},
	} {
		if _, err = db.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by)
VALUES($1,'isolated',$2,'evening-self-study.xlsx',$3,$4,'AVAILABLE',$5,$6)`, file.id, file.key, len(content), strings.Repeat("a", 64), file.retention, fixture.accountID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,published_at,created_by)
VALUES($1,'EVENING_SELF_STUDY_IMPORT','晚自习',$2,1,$3,'PUBLISHED',now(),$4)`, templateID, typeID, templateFileID, fixture.accountID); err != nil {
		t.Fatal(err)
	}
	imports := importjob.NewInspectionImportService(db, rollbackFileReader{source, content}, importjob.NewAttendanceAdapter(db),
		importjob.NewMorningAdapter(db), importjob.NewLateReturnAdapter(db), importjob.NewDormitoryHygieneAdapter(db),
		importjob.NewClassroomHygieneAdapter(db), 16<<20, importjob.NewEveningSelfStudyAdapter(db))
	createBatch := func() importjob.Batch {
		t.Helper()
		batch, err := imports.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID,
			WeekID: fixture.weekID, DepartmentID: integrationDepartmentID, SourceFile: source, TemplateID: templateID,
			TemplateVersion: 1, CreatedBy: fixture.accountID})
		if err != nil {
			t.Fatal(err)
		}
		batch, err = imports.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
		if err != nil {
			t.Fatal(err)
		}
		return batch
	}
	batch := createBatch()
	if batch.TotalRows != 2 || batch.ValidRows != 2 || batch.InvalidRows != 0 {
		t.Fatalf("evening preview = %+v", batch)
	}
	preview, err := imports.ListRows(ctx, batch.ID, 1, 20, "")
	if err != nil || preview.Total != 2 || preview.Items[0].NormalizedInspection == nil {
		t.Fatalf("evening typed preview missing: %+v, %v", preview, err)
	}
	var input map[string]any
	if err := json.Unmarshal(preview.Items[0].NormalizedInspection.Input.Details, &input); err != nil {
		t.Fatal(err)
	}
	for _, serverOwned := range []string{"period", "absenceHours", "penaltyMultiplier", "penaltyHours", "courseId"} {
		if _, found := input[serverOwned]; found {
			t.Fatalf("preview input contains server-owned/unsupported field %q: %#v", serverOwned, input)
		}
	}
	if len(preview.Items[0].NormalizedInspection.References.Classes) != 1 || len(preview.Items[0].NormalizedInspection.References.Students) != 1 {
		t.Fatalf("evening preview references missing: %+v", preview.Items[0].NormalizedInspection.References)
	}
	studentID := preview.Items[0].NormalizedInspection.References.Students[0].ID
	confirm := importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(),
		Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID,
			DepartmentID: integrationDepartmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}}
	if _, err := db.Pool.Exec(ctx, `UPDATE students SET enabled=false WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.ConfirmBatch(ctx, confirm); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatalf("stale evening student reference accepted: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE students SET enabled=true WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := imports.ConfirmBatch(ctx, confirm)
	if err != nil || confirmed.ImportedRows != 2 {
		t.Fatalf("evening confirmation = %+v, %v", confirmed, err)
	}
	if retry, err := imports.ConfirmBatch(ctx, confirm); err != nil || retry != confirmed {
		t.Fatalf("evening confirmation was not idempotent: %+v, %v", retry, err)
	}
	var confirmedRecords, correctDetails, courseRows, attendanceRows int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1 AND source='EXCEL' AND status='DRAFT' AND occurrence_kind='TIME' AND violation_count=1`, batch.ID).Scan(&confirmedRecords); err != nil || confirmedRecords != 2 {
		t.Fatalf("evening records are not course-free absence events: count=%d err=%v", confirmedRecords, err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM evening_self_study_details d JOIN inspection_records r ON r.id=d.record_id
WHERE r.import_batch_id=$1 AND d.period_text='9-10' AND d.absence_hours=2 AND d.penalty_multiplier=1 AND d.penalty_hours=2`, batch.ID).Scan(&correctDetails); err != nil || correctDetails != 2 {
		t.Fatalf("evening server-derived values missing: count=%d err=%v", correctDetails, err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_details d JOIN inspection_records r ON r.id=d.record_id WHERE r.import_batch_id=$1`, batch.ID).Scan(&attendanceRows); err != nil || attendanceRows != 0 {
		t.Fatalf("evening import wrote course-attendance detail rows: %d, %v", attendanceRows, err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, batch.ID).Scan(&courseRows); err != nil || courseRows != 2 {
		t.Fatalf("import provenance links missing: %d, %v", courseRows, err)
	}
	duplicate := createBatch()
	duplicateRows, err := imports.ListRows(ctx, duplicate.ID, 1, 20, importjob.RowStatusInvalid)
	if err != nil || duplicate.ValidRows != 0 || duplicate.InvalidRows != 2 || duplicateRows.Total != 2 || len(duplicateRows.Items[0].Errors) == 0 || duplicateRows.Items[0].Errors[0].Code != importjob.ErrorRecordAlreadyExists {
		t.Fatalf("existing evening absences were not rejected: batch=%+v rows=%+v err=%v", duplicate, duplicateRows, err)
	}
	rollback := importjob.NewRollbackService(db, permission.NewService(db))
	result, err := rollback.Rollback(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, importjob.RollbackInput{BatchID: batch.ID, Version: confirmed.Version, Reason: "isolated evening rollback"}, audit.Metadata{})
	if err != nil || result.RolledBackRows != 2 {
		t.Fatalf("evening import rollback = %+v, %v", result, err)
	}
	var activeRecords int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1 AND deleted_at IS NULL`, batch.ID).Scan(&activeRecords); err != nil || activeRecords != 0 {
		t.Fatalf("rolled-back evening records remain active: count=%d err=%v", activeRecords, err)
	}
}
