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
	"time"
)

func TestLateReturnImportPostgres(t *testing.T) {
	url := os.Getenv("LATE_RETURN_IMPORT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LATE_RETURN_IMPORT_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	areaID, buildingID, dormID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := db.Pool.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO dormitory_areas(id,name) VALUES($1,'isolated area')`, areaID)
	exec(`INSERT INTO dormitory_buildings(id,dormitory_area_id,name) VALUES($1,$2,'isolated building')`, buildingID, areaID)
	exec(`INSERT INTO dormitories(id,dormitory_building_id,room_no) VALUES($1,$2,'101')`, dormID, buildingID)
	exec(`UPDATE students SET dormitory_id=$1`, dormID)

	var typeID, departmentID string
	if err = db.Pool.QueryRow(ctx, `SELECT i.id,ds.department_id FROM inspection_types i JOIN department_inspection_scopes ds ON ds.inspection_type_id=i.id WHERE i.code='LATE_RETURN' AND ds.enabled AND ds.can_enter ORDER BY ds.department_id LIMIT 1`).Scan(&typeID, &departmentID); err != nil {
		t.Fatal(err)
	}
	old, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("source.xlsx", fixture.workbook)
	if err != nil {
		t.Fatal(err)
	}
	content, err := importjob.BuildLateReturnTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for i, row := range old.Rows {
		values := []any{row.Values["student_no"], row.Values["student_name"], row.Values["class_name"], row.Values["absence_date"], "23:05:00", "isolated area isolated building 101", "isolated late-return"}
		cell, _ := excelize.CoordinatesToCellName(1, i+5)
		if err = book.SetSheetRow("晚归导入", cell, &values); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	fileID, templateID := uuid.NewString(), uuid.NewString()
	if _, err = db.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','late-return/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'LATE_RETURN_IMPORT','late-return',$2,1,$3,'DRAFT',$4)`, templateID, typeID, fileID, fixture.accountID); err != nil {
		t.Fatal(err)
	}
	source := storage.File{ID: fileID, OriginalName: "source.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	service := importjob.NewInspectionImportService(db, rollbackFileReader{source, filled.Bytes()}, importjob.NewAttendanceAdapter(db), importjob.NewMorningAdapter(db), importjob.NewLateReturnAdapter(db), importjob.NewDormitoryHygieneAdapter(db), importjob.NewClassroomHygieneAdapter(db), 16<<20)
	t.Run("cancel audit", func(t *testing.T) {
		cancelBatch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
		if err != nil {
			t.Fatal(err)
		}
		cancelBatch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: cancelBatch.ID, ExpectedVersion: cancelBatch.Version})
		if err != nil || cancelBatch.ValidRows != 2 {
			t.Fatal("cancel preview", err)
		}
		exec(`CREATE FUNCTION reject_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='IMPORT_CANCELLED' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_cancel_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_cancel_audit()`)
		if _, err := service.CancelBatch(ctx, cancelBatch.ID, cancelBatch.Version, audit.Metadata{ActorID: fixture.accountID}); err == nil {
			t.Fatal("audit failure accepted")
		}
		unchanged, err := service.GetBatch(ctx, cancelBatch.ID)
		if err != nil || unchanged.Status != cancelBatch.Status || unchanged.Version != cancelBatch.Version || !unchanged.CancelledAt.Equal(cancelBatch.CancelledAt) {
			t.Fatal("failed cancellation changed batch", unchanged.Status, unchanged.Version, err)
		}
		var cancellationIsNull bool
		if err := db.Pool.QueryRow(ctx, `SELECT cancelled_at IS NULL FROM import_batches WHERE id=$1`, cancelBatch.ID).Scan(&cancellationIsNull); err != nil || !cancellationIsNull {
			t.Fatal("failed cancellation left timestamp", err)
		}
		exec(`DROP TRIGGER reject_cancel_audit ON audit_logs; DROP FUNCTION reject_cancel_audit()`)
		type cancelOutcome struct {
			batch importjob.Batch
			err   error
		}
		outcomes := make(chan cancelOutcome, 2)
		for i := 0; i < 2; i++ {
			go func() {
				value, err := service.CancelBatch(ctx, cancelBatch.ID, cancelBatch.Version, audit.Metadata{ActorID: fixture.accountID})
				outcomes <- cancelOutcome{value, err}
			}()
		}
		var cancelled importjob.Batch
		succeeded, conflicted := 0, 0
		for i := 0; i < 2; i++ {
			outcome := <-outcomes
			if outcome.err == nil {
				cancelled = outcome.batch
				succeeded++
			} else if errors.Is(outcome.err, importjob.ErrBatchVersionConflict) {
				conflicted++
			} else {
				t.Fatal(outcome.err)
			}
		}
		if succeeded != 1 || conflicted != 1 || cancelled.Status != importjob.BatchStatusCancelled {
			t.Fatal("concurrent cancellation", succeeded, conflicted, cancelled.Status)
		}
		if _, err := service.CancelBatch(ctx, cancelBatch.ID, cancelBatch.Version, audit.Metadata{ActorID: fixture.accountID}); !errors.Is(err, importjob.ErrBatchVersionConflict) {
			t.Fatal("stale cancellation accepted", err)
		}
		preserved, err := service.ListRows(ctx, cancelBatch.ID, 1, 20, "")
		if err != nil || len(preserved.Items) != 2 || cancelled.OriginalFileID != source.ID {
			t.Fatal("cancellation lost provenance", err)
		}
		var count int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='IMPORT_CANCELLED' AND actor_id=$2`, cancelBatch.ID, fixture.accountID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("cancellation audit count=%d err=%v", count, err)
		}
	})
	batch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
	if err != nil || batch.ValidRows != 2 {
		t.Fatal("late-return preview", batch, err)
	}
	rows, err := service.ListRows(ctx, batch.ID, 1, 20, "")
	if err != nil || len(rows.Items) != 2 || rows.Items[0].NormalizedInspection == nil {
		t.Fatal("typed preview was lost", rows, err)
	}
	// Preview must reject disabled grades just as confirmation does.
	t.Run("disabled grade preview", func(t *testing.T) {
		classID := rows.Items[0].NormalizedInspection.References.Classes[0].ID
		exec(`UPDATE grades SET enabled=false WHERE id=(SELECT grade_id FROM classes WHERE id=$1)`, classID)
		defer exec(`UPDATE grades SET enabled=true WHERE id=(SELECT grade_id FROM classes WHERE id=$1)`, classID)
		lookup := importjob.AttendanceReferenceLookup{SemesterID: fixture.semesterID}
		for _, row := range old.Rows {
			lookup.StudentNos = append(lookup.StudentNos, row.Values["student_no"])
			lookup.ClassNames = append(lookup.ClassNames, row.Values["class_name"])
		}
		refs, err := db.LoadAttendanceReferences(ctx, lookup)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs.StudentsByNo) != 0 || len(refs.ClassesByName) != 0 {
			t.Errorf("disabled grade references exposed: students=%d classes=%d", len(refs.StudentsByNo), len(refs.ClassesByName))
		}
		var attendanceTypeID string
		if err := db.Pool.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code=$1`, inspection.InspectionTypeAttendance).Scan(&attendanceTypeID); err != nil {
			t.Fatal(err)
		}
		start, _ := time.Parse("2006-01-02", "2026-06-15")
		end, _ := time.Parse("2006-01-02", "2026-06-21")
		preview, err := importjob.NewAttendanceAdapter(db).Validate(ctx, importjob.AttendanceImportScope{TypeCode: inspection.InspectionTypeAttendance, SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: attendanceTypeID, DepartmentID: departmentID, CreatedBy: fixture.accountID, ImportBatchID: batch.ID, WeekStart: start, WeekEnd: end}, old.Rows)
		if err != nil || preview.ValidCount != 0 || preview.InvalidCount != 2 {
			t.Errorf("disabled grade course preview: valid=%d invalid=%d err=%v", preview.ValidCount, preview.InvalidCount, err)
		}
		disabledBatch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
		if err != nil {
			t.Fatal(err)
		}
		disabledBatch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: disabledBatch.ID, ExpectedVersion: disabledBatch.Version})
		if err != nil || disabledBatch.ValidRows != 0 || disabledBatch.InvalidRows != 2 {
			t.Errorf("disabled grade preview: valid=%d invalid=%d err=%v", disabledBatch.ValidRows, disabledBatch.InvalidRows, err)
		}
	})
	studentID := rows.Items[0].NormalizedInspection.References.Students[0].ID
	input := importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID, DepartmentID: departmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}}
	if _, err = db.Pool.Exec(ctx, `UPDATE students SET enabled=false WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("stale late-return reference accepted", err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE students SET enabled=true WHERE id=$1`, studentID); err != nil {
		t.Fatal(err)
	}
	// A room reassignment between preview and confirmation must reject all rows.
	exec(`UPDATE students SET dormitory_id=NULL WHERE id=$1`, studentID)
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("changed dormitory accepted", err)
	}
	var count int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, batch.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial write", count, err)
	}
	exec(`UPDATE students SET dormitory_id=$2 WHERE id=$1`, studentID, dormID)
	confirmed, err := service.ConfirmBatch(ctx, input)
	if err != nil || confirmed.ImportedRows != 2 {
		t.Fatal("late-return confirmation", confirmed, err)
	}
	retry, err := service.ConfirmBatch(ctx, input)
	if err != nil || retry != confirmed {
		t.Fatal("late-return confirmation not idempotent", retry, err)
	}
	var valid, wrongTable, links int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records r JOIN late_return_details d ON d.record_id=r.id WHERE r.import_batch_id=$1 AND r.source='EXCEL' AND r.status='DRAFT' AND r.occurred_at IS NOT NULL AND r.inspection_date IS NULL AND r.occurrence_kind='TIME' AND r.violation_count=1`, batch.ID).Scan(&valid); err != nil || valid != 2 {
		t.Fatal("wrong late-return granularity", valid, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_details a JOIN inspection_records r ON r.id=a.record_id WHERE r.import_batch_id=$1`, batch.ID).Scan(&wrongTable); err != nil || wrongTable != 0 {
		t.Fatal("course details were inserted", wrongTable, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, batch.ID).Scan(&links); err != nil || links != 2 {
		t.Fatal("missing provenance", links, err)
	}
	rollback := importjob.NewRollbackService(db, permission.NewService(db))
	result, err := rollback.Rollback(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, importjob.RollbackInput{BatchID: batch.ID, Version: confirmed.Version, Reason: "isolated late-return rollback"}, audit.Metadata{})
	if err != nil || result.RolledBackRows != 2 {
		t.Fatal("late-return rollback", result, err)
	}
}
