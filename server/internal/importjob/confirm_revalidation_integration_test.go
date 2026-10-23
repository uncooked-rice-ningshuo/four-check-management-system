package importjob_test

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
	"time"
)

func TestImportConfirmationRevalidatesPostgres(t *testing.T) {
	url := os.Getenv("CONFIRM_REVALIDATION_DATABASE_URL")
	if url == "" {
		t.Skip("CONFIRM_REVALIDATION_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	fileID, templateID := uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','revalidation/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID)
	exec(`INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'ATTENDANCE_IMPORT','revalidation',$2,1,$3,'DRAFT',$4)`, templateID, integrationInspectionTypeID, fileID, fixture.accountID)
	source := storage.File{ID: fileID, OriginalName: "source.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	imports := importjob.NewService(db, rollbackFileReader{source, fixture.workbook}, importjob.NewAttendanceXLSXParser(16<<20), importjob.NewAttendanceAdapter(db))
	batch, err := imports.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: integrationInspectionTypeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: integrationDepartmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = imports.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ValidRows != 2 {
		t.Fatalf("valid rows %d", batch.ValidRows)
	}
	input := importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}}
	rows, err := db.ListImportRows(ctx, batch.ID, 1, 20, "")
	if err != nil || len(rows.Items) != 2 {
		t.Fatal("preview rows", err)
	}
	first := rows.Items[0].NormalizedRecord
	for _, test := range []struct {
		name, change, restore string
		args                  []any
	}{
		{"student disabled", `UPDATE students SET enabled=false WHERE id=$1`, `UPDATE students SET enabled=true WHERE id=$1`, []any{first.StudentID}},
		{"class disabled", `UPDATE classes SET enabled=false WHERE id=$1`, `UPDATE classes SET enabled=true WHERE id=$1`, []any{first.ClassID}},
		{"course disabled", `UPDATE courses SET enabled=false WHERE id=$1`, `UPDATE courses SET enabled=true WHERE id=$1`, []any{first.CourseID}},
		{"classroom disabled", `UPDATE classrooms SET enabled=false WHERE id=$1`, `UPDATE classrooms SET enabled=true WHERE id=$1`, []any{first.ClassroomID}},
		{"grade disabled", `UPDATE grades SET enabled=false WHERE id=(SELECT grade_id FROM classes WHERE id=$1)`, `UPDATE grades SET enabled=true WHERE id=(SELECT grade_id FROM classes WHERE id=$1)`, []any{first.ClassID}},
		{"building disabled", `UPDATE teaching_buildings SET enabled=false WHERE id=(SELECT teaching_building_id FROM classrooms WHERE id=$1)`, `UPDATE teaching_buildings SET enabled=true WHERE id=(SELECT teaching_building_id FROM classrooms WHERE id=$1)`, []any{first.ClassroomID}},
		{"semester disabled", `UPDATE semesters SET enabled=false WHERE id=$1`, `UPDATE semesters SET enabled=true WHERE id=$1`, []any{fixture.semesterID}},
		{"department mapping removed", `UPDATE department_inspection_scopes SET can_enter=false WHERE department_id=$1 AND inspection_type_id=$2`, `UPDATE department_inspection_scopes SET can_enter=true WHERE department_id=$1 AND inspection_type_id=$2`, []any{integrationDepartmentID, integrationInspectionTypeID}},
		{"week dates moved", `UPDATE semester_weeks SET start_date=start_date+7,end_date=end_date+7 WHERE id=$1`, `UPDATE semester_weeks SET start_date=start_date-7,end_date=end_date-7 WHERE id=$1`, []any{fixture.weekID}},
		{"course class removed", `DELETE FROM course_classes WHERE course_id=$1 AND class_id=$2`, `INSERT INTO course_classes(course_id,class_id) VALUES($1,$2)`, []any{first.CourseID, first.ClassID}},
		{"week disabled", `UPDATE semester_weeks SET enabled=false WHERE id=$1`, `UPDATE semester_weeks SET enabled=true WHERE id=$1`, []any{fixture.weekID}},
		{"student moved", `UPDATE students SET class_id=$2 WHERE id=$1`, `UPDATE students SET class_id=$3 WHERE id=$1 AND $2::uuid IS NOT NULL`, []any{first.StudentID, rows.Items[1].NormalizedRecord.ClassID, first.ClassID}},
	} {
		changeArgs := test.args
		if test.name == "student moved" {
			changeArgs = test.args[:2]
		}
		exec(test.change, changeArgs...)
		_, err = imports.ConfirmBatch(ctx, input)
		if !errors.Is(err, importjob.ErrPreviewStale) {
			t.Fatalf("%s: expected stale preview conflict, got %v", test.name, err)
		}
		var status string
		var version, records, links, audits int
		if err = db.Pool.QueryRow(ctx, `SELECT status,version,(SELECT count(*) FROM inspection_records WHERE import_batch_id=b.id),(SELECT count(*) FROM import_rows WHERE batch_id=b.id AND record_id IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE resource_id=b.id::text AND action='IMPORT_BATCH_CONFIRMED') FROM import_batches b WHERE id=$1`, batch.ID).Scan(&status, &version, &records, &links, &audits); err != nil || status != "PREVIEW_READY" || version != batch.Version || records != 0 || links != 0 || audits != 0 {
			t.Fatalf("%s left partial changes: %s %d %d %d %d %v", test.name, status, version, records, links, audits, err)
		}
		exec(test.restore, test.args...)
	}

	// A manual record inserted after preview must produce a domain conflict,
	// not a database failure, and roll back the whole imported batch.
	duplicateRow := rows.Items[1].NormalizedRecord
	occurred := duplicateRow.OccurredAt.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	details, err := json.Marshal(inspection.CourseAbsenceDetails{StudentID: duplicateRow.StudentID, ClassID: duplicateRow.ClassID, CourseID: duplicateRow.CourseID, ClassroomID: duplicateRow.ClassroomID, Date: occurred.Format("2006-01-02"), Time: occurred.Format("15:04:05"), Period: duplicateRow.PeriodText, AbsenceHours: duplicateRow.AbsenceHours})
	if err != nil {
		t.Fatal(err)
	}
	recordsService := inspection.NewRecordService(db, permission.NewService(db))
	actor := inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}
	manual, err := recordsService.Create(ctx, actor, inspection.InspectionInput{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, Details: details}, audit.Metadata{})
	if err != nil {
		t.Fatal("prepare concurrent manual record", err)
	}
	if _, err = imports.ConfirmBatch(ctx, input); !errors.Is(err, inspection.ErrAttendanceDuplicate) {
		t.Fatalf("duplicate after preview should be an explicit conflict, got %v", err)
	}
	var imported int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, batch.ID).Scan(&imported); err != nil || imported != 0 {
		t.Fatal("partial duplicate import", imported, err)
	}
	if err = recordsService.Delete(ctx, actor, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: manual.ID, Version: manual.RowVersion}}, Reason: "isolated duplicate recovery"}, audit.Metadata{}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := imports.ConfirmBatch(ctx, input)
	if err != nil || confirmed.ImportedRows != 2 {
		t.Fatal("restored references did not permit retry", confirmed, err)
	}
}
