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

func TestDormitoryHygieneImportPostgres(t *testing.T) {
	url := os.Getenv("DORMITORY_HYGIENE_IMPORT_TEST_DATABASE_URL")
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
	areaID, buildingID, dormID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO dormitory_areas(id,name) VALUES($1,'isolated area')`, areaID)
	exec(`INSERT INTO dormitory_buildings(id,dormitory_area_id,name) VALUES($1,$2,'isolated building')`, buildingID, areaID)
	exec(`INSERT INTO dormitories(id,dormitory_building_id,room_no) VALUES($1,$2,'101')`, dormID, buildingID)
	var typeID, departmentID string
	if err = db.Pool.QueryRow(ctx, `SELECT i.id,ds.department_id FROM inspection_types i JOIN department_inspection_scopes ds ON ds.inspection_type_id=i.id WHERE i.code='DORMITORY_HYGIENE' AND ds.enabled AND ds.can_enter ORDER BY ds.department_id LIMIT 1`).Scan(&typeID, &departmentID); err != nil {
		t.Fatal(err)
	}
	reasonID := uuid.NewString()
	exec(`INSERT INTO dictionary_types(id,code,name) VALUES($1,'DORMITORY_HYGIENE_REASON','隔离宿舍卫生原因') ON CONFLICT(code) DO NOTHING`, uuid.NewString())
	exec(`INSERT INTO dictionary_items(id,type_id,value,label) SELECT $1,id,'ISOLATED_REASON','隔离原因' FROM dictionary_types WHERE code='DORMITORY_HYGIENE_REASON'`, reasonID)
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
	content, err := importjob.BuildDormitoryHygieneTemplate()
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for i := 0; i < 2; i++ {
		outcome, round, people, reason := "优秀", "首轮", "", ""
		if i == 1 {
			outcome, round, people, reason = "不合格", "复查", strings.Join(students, ";"), "隔离原因"
		}
		row := []any{"isolated area isolated building 101", original.Rows[0].Values["absence_date"], "", round, outcome, strings.Join(classes, ";"), people, reason, "隔离检查", ""}
		cell, _ := excelize.CoordinatesToCellName(1, i+5)
		if err = book.SetSheetRow("宿舍卫生导入", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	filled, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	fileID, templateID := uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','hygiene/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID)
	exec(`INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'DORMITORY_HYGIENE_IMPORT','hygiene',$2,1,$3,'DRAFT',$4)`, templateID, typeID, fileID, fixture.accountID)
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
	t.Run("authorized import grade options", func(t *testing.T) {
		options := inspection.NewContextOptionService(db, permission.NewService(db))
		page, err := options.ContextOptions(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeDormitoryHygiene, Purpose: "entry", Kind: "grades", SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID})
		if err != nil || page.Total != 1 || page.Items[0].ID != "2026级" {
			t.Fatal("grade options", page, err)
		}
		for _, grade := range []string{"2026级", "unavailable-grade"} {
			classes, err := options.ContextOptions(ctx, inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeDormitoryHygiene, Purpose: "entry", Kind: "classes", SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: departmentID, Grade: grade})
			expected := 2
			if grade != "2026级" {
				expected = 0
			}
			if err != nil || classes.Total != expected {
				t.Fatal("grade filtered classes", classes, err)
			}
		}
	})
	t.Run("name ambiguity and responsibility scope", func(t *testing.T) {
		duplicateReason := uuid.NewString()
		exec(`INSERT INTO dictionary_items(id,type_id,value,label) SELECT $1,type_id,'ISOLATED_OTHER',label FROM dictionary_items WHERE id=$2`, duplicateReason, reasonID)
		secondArea, secondBuilding, secondRoom := uuid.NewString(), uuid.NewString(), uuid.NewString()
		exec(`INSERT INTO dormitory_areas(id,name) VALUES($1,'isolated')`, secondArea)
		exec(`INSERT INTO dormitory_buildings(id,dormitory_area_id,name) VALUES($1,$2,'area isolated building')`, secondBuilding, secondArea)
		exec(`INSERT INTO dormitories(id,dormitory_building_id,room_no) VALUES($1,$2,'101')`, secondRoom, secondBuilding)
		objects, err := db.LoadHygieneImportObjects(ctx, importjob.HygieneImportLookup{TypeCode: inspection.InspectionTypeDormitoryHygiene, DormitoryNames: []string{"isolated area isolated building 101"}, ReasonNames: []string{"隔离原因"}})
		if err != nil || len(objects.Dormitories["isolated area isolated building 101"]) != 2 || len(objects.Reasons["隔离原因"]) != 2 {
			t.Fatal("ambiguous lookup collapsed", objects, err)
		}
		parser, err := importjob.NewInspectionXLSXParser(inspection.InspectionTypeDormitoryHygiene, 16<<20)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.Parse("source.xlsx", filled.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		scope := importjob.AttendanceImportScope{TypeCode: inspection.InspectionTypeDormitoryHygiene, SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: typeID, DepartmentID: departmentID, WeekStart: batch.WeekStart, WeekEnd: batch.WeekEnd, CreatedBy: fixture.accountID, ImportBatchID: batch.ID}
		checked, err := importjob.NewDormitoryHygieneAdapter(db).Validate(ctx, scope, parsed.Rows)
		if err != nil || checked.ValidCount != 0 || checked.InvalidCount != 2 {
			t.Fatal("ambiguous object accepted", checked, err)
		}
		exec(`UPDATE dormitories SET enabled=false WHERE id=$1`, secondRoom)
		checked, err = importjob.NewDormitoryHygieneAdapter(db).Validate(ctx, scope, parsed.Rows)
		if err != nil || checked.ValidCount != 1 || checked.InvalidCount != 1 {
			t.Fatal("ambiguous reason accepted", checked, err)
		}
		exec(`UPDATE dictionary_items SET enabled=false WHERE id=$1`, duplicateReason)
		scope.ClassID = rows.Items[0].NormalizedInspection.References.Classes[0].ID
		checked, err = importjob.NewDormitoryHygieneAdapter(db).Validate(ctx, scope, parsed.Rows)
		if err != nil || checked.ValidCount != 0 || checked.InvalidCount != 2 {
			t.Fatal("multi-class escaped restricted batch", checked, err)
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
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records r JOIN dorm_hygiene_details d ON d.record_id=r.id WHERE r.import_batch_id=$1 AND r.occurrence_kind='ROUND' AND r.occurred_at IS NULL AND r.source='EXCEL' AND r.status='DRAFT' AND ((d.result='EXCELLENT' AND r.violation_count=0) OR (d.result='UNQUALIFIED' AND r.violation_count=1))`, batch.ID).Scan(&records); err != nil || records != 2 {
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
