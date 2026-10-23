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

func TestClassroomHygieneImportPostgres(t *testing.T) {
	url := os.Getenv("CLASSROOM_HYGIENE_IMPORT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	areaID, buildingID, classroomID, otherRoomID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO teaching_buildings(id,name) VALUES($1,'隔离教室楼')`, buildingID)
	exec(`INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES($1,$2,'101'),($3,$2,'102')`, classroomID, buildingID, otherRoomID)
	exec(`INSERT INTO classroom_hygiene_areas(id,classroom_id,name) VALUES($1,$2,'前排'),($3,$4,'前排')`, areaID, classroomID, uuid.NewString(), otherRoomID)
	var typeID, departmentID string
	if err = db.Pool.QueryRow(ctx, `SELECT i.id,ds.department_id FROM inspection_types i JOIN department_inspection_scopes ds ON ds.inspection_type_id=i.id WHERE i.code='CLASSROOM_HYGIENE' AND ds.enabled AND ds.can_enter ORDER BY ds.department_id LIMIT 1`).Scan(&typeID, &departmentID); err != nil {
		t.Fatal(err)
	}
	reasonID := uuid.NewString()
	exec(`INSERT INTO dictionary_types(id,code,name) VALUES($1,'CLASSROOM_HYGIENE_REASON','隔离教室卫生原因') ON CONFLICT(code) DO NOTHING`, uuid.NewString())
	exec(`INSERT INTO dictionary_items(id,type_id,value,label) SELECT $1,id,'ISOLATED_REASON','隔离原因' FROM dictionary_types WHERE code='CLASSROOM_HYGIENE_REASON'`, reasonID)
	original, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("source.xlsx", fixture.workbook)
	if err != nil {
		t.Fatal(err)
	}
	classes := []string{}
	students := []string{}
	for _, row := range original.Rows {
		classes = append(classes, row.Values["class_name"])
		students = append(students, row.Values["student_no"])
	}
	content, err := importjob.BuildClassroomHygieneTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for i := 0; i < 2; i++ {
		outcome, round, eventTime, people, reason := "优秀", "首轮", "", "", ""
		if i == 1 {
			outcome, round, eventTime, people, reason = "不合格", "", "09:05:00", strings.Join(students, ";"), "隔离原因"
		}
		row := []any{"隔离教室楼 101", "前排", original.Rows[0].Values["absence_date"], eventTime, round, outcome, strings.Join(classes, ";"), people, reason, "隔离检查", ""}
		cell, _ := excelize.CoordinatesToCellName(1, i+5)
		if err = book.SetSheetRow("教室卫生导入", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	fileID, templateID := uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','hygiene/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID)
	exec(`INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'CLASSROOM_HYGIENE_IMPORT','hygiene',$2,1,$3,'DRAFT',$4)`, templateID, typeID, fileID, fixture.accountID)
	source := storage.File{ID: fileID, OriginalName: "source.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	service := importjob.NewInspectionImportService(db, rollbackFileReader{source, filled.Bytes()}, importjob.NewAttendanceAdapter(db), importjob.NewMorningAdapter(db), importjob.NewLateReturnAdapter(db), importjob.NewDormitoryHygieneAdapter(db), importjob.NewClassroomHygieneAdapter(db), 16<<20)
	batch, err := service.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: typeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = service.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
	if err != nil || batch.ValidRows != 2 {
		failedRows, _ := service.ListRows(ctx, batch.ID, 1, 20, "")
		for _, r := range failedRows.Items {
			t.Log("preview errors", r.RowNo, r.Errors)
		}
		t.Fatal("object preview", batch.ValidRows, err)
	}
	rows, err := service.ListRows(ctx, batch.ID, 1, 20, "")
	if err != nil || len(rows.Items) != 2 || rows.Items[0].NormalizedInspection == nil {
		t.Fatal(rows, err)
	}
	if len(rows.Items[0].NormalizedInspection.References.Classes) != 2 || len(rows.Items[0].NormalizedInspection.References.Students) != 0 || len(rows.Items[1].NormalizedInspection.References.Students) != 2 {
		t.Fatal("multi-responsibility preview lost")
	}
	t.Run("area parent and responsibility scope", func(t *testing.T) {
		parser, err := importjob.NewInspectionXLSXParser(inspection.InspectionTypeClassroomHygiene, 16<<20)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.Parse("source.xlsx", filled.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		scope := importjob.AttendanceImportScope{TypeCode: inspection.InspectionTypeClassroomHygiene, SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID, DepartmentID: departmentID, WeekStart: batch.WeekStart, WeekEnd: batch.WeekEnd, CreatedBy: fixture.accountID, ImportBatchID: batch.ID}
		objects, err := db.LoadHygieneImportObjects(ctx, importjob.HygieneImportLookup{TypeCode: scope.TypeCode, ClassroomNames: []string{"隔离教室楼 101", "隔离教室楼 102"}, AreaNames: []string{"前排"}})
		if err != nil || len(objects.Areas["前排"]) != 2 {
			t.Fatal("area identity collapsed", objects, err)
		}
		exec(`UPDATE classroom_hygiene_areas SET enabled=false WHERE id=$1`, areaID)
		checked, err := importjob.NewClassroomHygieneAdapter(db).Validate(ctx, scope, parsed.Rows)
		if err != nil || checked.InvalidCount != 2 {
			t.Fatal("other room area accepted", checked, err)
		}
		exec(`UPDATE classroom_hygiene_areas SET enabled=true WHERE id=$1`, areaID)
		scope.ClassID = rows.Items[0].NormalizedInspection.References.Classes[0].ID
		checked, err = importjob.NewClassroomHygieneAdapter(db).Validate(ctx, scope, parsed.Rows)
		if err != nil || checked.InvalidCount != 2 {
			t.Fatal("responsibility scope escaped", checked, err)
		}
	})
	input := importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID, DepartmentID: departmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}}
	assertEmpty := func() {
		t.Helper()
		var count int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, batch.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial write", count, err)
		}
	}
	exec(`UPDATE classroom_hygiene_areas SET enabled=false WHERE id=$1`, areaID)
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("disabled area accepted", err)
	}
	assertEmpty()
	exec(`UPDATE classroom_hygiene_areas SET enabled=true,name='另一区域',classroom_id=$2 WHERE id=$1`, areaID, otherRoomID)
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("reparented area accepted", err)
	}
	assertEmpty()
	exec(`UPDATE classroom_hygiene_areas SET name='前排',classroom_id=$2 WHERE id=$1`, areaID, classroomID)
	exec(`UPDATE dictionary_items SET enabled=false WHERE id=$1`, reasonID)
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("disabled reason accepted", err)
	}
	assertEmpty()
	exec(`UPDATE dictionary_items SET enabled=true WHERE id=$1`, reasonID)
	student := rows.Items[1].NormalizedInspection.References.Students[0]
	otherClass := rows.Items[1].NormalizedInspection.References.Classes[0].ID
	for _, class := range rows.Items[1].NormalizedInspection.References.Classes {
		if class.ID != student.ClassID {
			otherClass = class.ID
		}
	}
	exec(`UPDATE students SET class_id=$2 WHERE id=$1`, student.ID, otherClass)
	if _, err = service.ConfirmBatch(ctx, input); !errors.Is(err, importjob.ErrPreviewStale) {
		t.Fatal("student reassignment accepted", err)
	}
	assertEmpty()
	exec(`UPDATE students SET class_id=$2 WHERE id=$1`, student.ID, student.ClassID)
	confirmed, err := service.ConfirmBatch(ctx, input)
	if err != nil || confirmed.ImportedRows != 2 {
		t.Fatal("object confirm", confirmed, err)
	}
	var records, responsibilities, people int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records r JOIN classroom_hygiene_details d ON d.record_id=r.id WHERE r.import_batch_id=$1 AND ((r.occurrence_kind='ROUND' AND r.occurred_at IS NULL) OR (r.occurrence_kind='TIME' AND (r.occurred_at AT TIME ZONE 'Asia/Shanghai')::time='09:05:00'::time)) AND d.classroom_id=$2 AND d.area_id=$3 AND r.source='EXCEL' AND r.status='DRAFT' AND ((d.result='EXCELLENT' AND r.violation_count=0) OR (d.result='UNQUALIFIED' AND r.violation_count=1))`, batch.ID, classroomID, areaID).Scan(&records); err != nil || records != 2 {
		t.Fatal("object semantics", records, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_responsibility_classes c JOIN inspection_records r ON r.id=c.record_id WHERE r.import_batch_id=$1`, batch.ID).Scan(&responsibilities); err != nil || responsibilities != 4 {
		t.Fatal("class links", responsibilities, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_responsibility_students s JOIN inspection_records r ON r.id=s.record_id WHERE r.import_batch_id=$1`, batch.ID).Scan(&people); err != nil || people != 2 {
		t.Fatal("student links", people, err)
	}
	rollback := importjob.NewRollbackService(db, permission.NewService(db))
	rolled, err := rollback.Rollback(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, importjob.RollbackInput{BatchID: batch.ID, Version: confirmed.Version, Reason: "隔离回滚"}, audit.Metadata{})
	if err != nil || rolled.RolledBackRows != 2 {
		t.Fatal("object rollback", rolled, err)
	}
}
