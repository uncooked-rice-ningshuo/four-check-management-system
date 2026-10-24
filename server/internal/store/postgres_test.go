package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPhaseOneMigrationDefinesRequiredTables(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "003_phase_one_completion.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, table := range []string{"user_permission_rules", "faculties", "majors", "staff", "classes", "dormitories", "classrooms", "courses", "students", "dictionary_types", "dictionary_items", "master_data_changes"} {
		if !strings.Contains(text, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("migration does not define %s", table)
		}
	}
}

func TestFacultyCodeExpansionMigrationSupportsLongBusinessCodes(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "006_expand_faculty_code.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "ALTER COLUMN code TYPE varchar(64)") {
		t.Fatalf("faculty code migration does not expand code to varchar(64): %s", text)
	}
}

func TestGradesMigrationDefinesMasterDataAndExistingGradeConstraints(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "007_add_grades_master_data.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS grades", "SELECT grade FROM classes", "SELECT grade FROM students", "classes_grade_fkey", "students_grade_fkey"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("grades migration does not contain %q", fragment)
		}
	}
}

func TestDormitoryHierarchyMigrationBackfillsExistingRooms(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "008_add_dormitory_hierarchy.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS dormitory_areas", "CREATE TABLE IF NOT EXISTS dormitory_buildings", "SELECT DISTINCT campus_area", "SELECT DISTINCT campus_area, building_no", "dormitory_building_id"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("dormitory hierarchy migration does not contain %q", fragment)
		}
	}
}

func TestServerManagedMasterDataMigrationRemovesArtificialCodesAndStructuresCourses(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "009_server_managed_master_data.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if strings.Contains(strings.ToUpper(text), "TRUNCATE TABLE") {
		t.Fatal("server-managed master-data migration must preserve existing business and change-history rows")
	}
	for _, fragment := range []string{
		"DROP COLUMN code", "grade_id uuid", "CREATE TABLE course_classes", "CREATE TABLE course_schedules",
		"weekday BETWEEN 1 AND 7", "start_section BETWEEN 1 AND 10", "end_section BETWEEN start_section AND 10",
		"UPDATE classes", "UPDATE students", "INSERT INTO course_classes", "CREATE TABLE course_schedule_migration_backlog",
		"INSERT INTO course_schedule_migration_backlog", "DROP COLUMN schedule_text", "DROP COLUMN class_id", "DROP COLUMN classroom_id",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("server-managed master-data migration does not contain %q", fragment)
		}
	}
}

func TestMasterDataIntegrityMigrationProtectsCourseIdentityAndSchedules(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "010_master_data_integrity.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{"courses_name_key UNIQUE(name)", "CREATE EXTENSION IF NOT EXISTS btree_gist", "course_schedules_no_overlap", "int4range(start_section, end_section, '[]') WITH &&"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("master-data integrity migration does not contain %q", fragment)
		}
	}
}

func TestTeachingBuildingMigrationNormalizesClassroomHierarchy(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "011_add_teaching_buildings.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE teaching_buildings", "SELECT DISTINCT", "teaching_building_id",
		"DROP COLUMN building", "DROP COLUMN floor", "classrooms_building_room_key",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("teaching-building migration does not contain %q", fragment)
		}
	}
}

func TestComposeMigrationRunnerRecordsAndSkipsAppliedFiles(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "migrate.sh")
	contents, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{"schema_migrations", "existing_schema", "001_init.sql", "002_admin_foundation.sql", "009_server_managed_master_data.sql", "already applied", "BEGIN;", "COMMIT;"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("migration runner does not contain %q", fragment)
		}
	}
	composePath := filepath.Join("..", "..", "..", "compose.yaml")
	compose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), `["sh", "/migrate.sh"]`) {
		t.Error("compose does not use the versioned migration runner")
	}
}

func TestMasterDataFilterUsesKindSpecificConstraints(t *testing.T) {
	filter, args := masterDataFilter(masterdata.Query{
		Kind: masterdata.KindStudent, Code: "S01", Name: "张", FacultyID: "faculty-1", MajorID: "major-1", GradeID: "grade-1", EnrollmentStatus: "ENROLLED",
	}, "student_no", "name")
	for _, fragment := range []string{"student_no ILIKE '%'||$2||'%'", "name ILIKE '%'||$3||'%'", "faculty_id=$5", "major_id=$6", "grade_id=$7", "enrollment_status=$8"} {
		if !strings.Contains(filter, fragment) {
			t.Fatalf("filter %q does not contain %q; args=%v", filter, fragment, args)
		}
	}
	filter, args = masterDataFilter(masterdata.Query{Kind: masterdata.KindClassroom, TeachingBuildingID: "building-1"}, "room_no", "room_no")
	if !strings.Contains(filter, "teaching_building_id=$5") || len(args) != 5 {
		t.Fatalf("classroom filter=%q args=%v", filter, args)
	}
	filter, args = masterDataFilter(masterdata.Query{Kind: masterdata.KindCourse, ClassID: "legacy-class", ClassroomID: "legacy-room"}, "", "name")
	if strings.Contains(filter, "course_classes") || strings.Contains(filter, "course_schedules") || len(args) != 4 {
		t.Fatalf("course list must ignore legacy relationships; filter=%q args=%v", filter, args)
	}
}

func TestCourseMasterDataQueryReadsOnlyTheCourseCatalog(t *testing.T) {
	query, _, _, _, ok := serverManagedMasterRecordSelect(masterdata.KindCourse)
	if !ok {
		t.Fatal("course master-data query is unavailable")
	}
	for _, legacy := range []string{"teacher_id", "course_classes", "course_schedules"} {
		if strings.Contains(query, legacy) {
			t.Errorf("course master-data query still reads legacy field %q: %s", legacy, query)
		}
	}
	if !strings.Contains(query, "FROM courses") {
		t.Fatalf("course query does not read the course catalog: %s", query)
	}
}

func TestMasterDataErrorMapsKnownUniqueConstraints(t *testing.T) {
	tests := []struct {
		constraint string
		want       error
	}{
		{constraint: "students_student_no_key", want: masterdata.ErrDuplicateCode},
		{constraint: "faculties_name_key", want: masterdata.ErrDuplicateName},
		{constraint: "public_dormitory_hygiene_area_dormitory_building_id_floor_u_key", want: masterdata.ErrDuplicateName},
	}
	for _, test := range tests {
		t.Run(test.constraint, func(t *testing.T) {
			err := masterDataError(&pgconn.PgError{Code: "23505", ConstraintName: test.constraint})
			if !errors.Is(err, test.want) {
				t.Fatalf("masterDataError() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPhaseOneMigrationSeedsSystemDictionaries(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "003_phase_one_completion.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, value := range []string{"'GENDER'", "'ENROLLMENT_STATUS'", "'MALE'", "'FEMALE'", "'UNKNOWN'", "'ENROLLED'", "'SUSPENDED'", "'GRADUATED'", "'LEFT'"} {
		if !strings.Contains(text, value) {
			t.Errorf("migration does not seed %s", value)
		}
	}
}

func TestStorageAuditMigrationDefinesRequiredSchema(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "004_storage_audit.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS files",
		"CREATE TABLE IF NOT EXISTS audit_logs",
		"files_object_key_key",
		"files_status_check",
		"files_retention_type_check",
		"idx_audit_logs_resource",
		"idx_audit_logs_actor",
		"prevent_audit_log_mutation",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("storage/audit migration does not define %s", fragment)
		}
	}
}

func TestPhaseTwoAttendanceImportMigrationDefinesRequiredSchema(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "005_phase_two_attendance_import.sql")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS import_templates",
		"CREATE TABLE IF NOT EXISTS import_batches",
		"CREATE TABLE IF NOT EXISTS import_rows",
		"CREATE TABLE IF NOT EXISTS inspection_records",
		"CREATE TABLE IF NOT EXISTS attendance_details",
		"import_templates_status_check",
		"import_batches_status_check",
		"import_rows_validation_status_check",
		"inspection_records_source_check",
		"inspection_records_status_check",
		"REFERENCES files(id)",
		"REFERENCES inspection_types(id)",
		"REFERENCES semesters(id)",
		"REFERENCES semester_weeks(id)",
		"REFERENCES departments(id)",
		"REFERENCES students(id)",
		"REFERENCES classes(id)",
		"REFERENCES courses(id)",
		"UNIQUE(batch_id, row_no)",
		"uq_import_templates_published",
		"WHERE status = 'PUBLISHED'",
		"uq_inspection_records_active_dedup",
		"WHERE deleted_at IS NULL",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("phase two migration does not define %s", fragment)
		}
	}
}

func TestBuildAttendanceListQueriesPushesEveryScopeDimensionIntoSQL(t *testing.T) {
	countSQL, countArgs, listSQL, listArgs, err := buildAttendanceListQueries(inspection.AttendanceListQuery{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "attendance",
		DepartmentID: "department-study", DepartmentIDs: []string{"department-study", "department-office"}, ClassID: "class-1", Grade: "2023", Keyword: "张晨", Status: inspection.RecordStatusDraft, Source: inspection.RecordSourceExcel,
		Sort: inspection.SortStudentNoAscending, Page: 2, PageSize: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"ir.semester_id = $1::uuid",
		"ir.week_id = $2::uuid",
		"ir.inspection_type_id = $3::uuid",
		"cardinality($10::text[]) = 0 OR ir.department_id::text = ANY($10::text[])",
		"($5::text = '' OR ir.class_id = $5::uuid)",
		"($6::text = '' OR ir.grade_snapshot = $6)",
		"($8::text = '' OR ir.status = $8)",
		"($9::text = '' OR ir.source = $9)",
		"ir.deleted_at IS NULL",
	} {
		if !strings.Contains(countSQL, fragment) || !strings.Contains(listSQL, fragment) {
			t.Errorf("attendance queries do not contain %q\ncount: %s\nlist: %s", fragment, countSQL, listSQL)
		}
	}
	if !strings.Contains(listSQL, "ORDER BY ir.student_no_snapshot ASC, ir.id ASC") {
		t.Fatalf("list query does not use whitelisted sort: %s", listSQL)
	}
	if strings.Contains(listSQL, "semester-1") || strings.Contains(listSQL, "张晨") {
		t.Fatalf("list query interpolates user input: %s", listSQL)
	}
	if len(countArgs) != 10 || len(listArgs) != 12 || listArgs[10] != 25 || listArgs[11] != 25 {
		t.Fatalf("count/list args = %#v / %#v", countArgs, listArgs)
	}
}

func TestBuildAttendanceListQueriesRejectsUnknownSort(t *testing.T) {
	_, _, _, _, err := buildAttendanceListQueries(inspection.AttendanceListQuery{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "attendance",
		Sort: "occurred_at DESC; DROP TABLE students", Page: 1, PageSize: 20,
	})
	if !errors.Is(err, inspection.ErrInvalidAttendanceQuery) {
		t.Fatalf("error = %v, want ErrInvalidAttendanceQuery", err)
	}
}

func TestBuildAttendanceListQueriesUsesEmptyArrayForUnrestrictedDepartments(t *testing.T) {
	_, countArgs, _, _, err := buildAttendanceListQueries(inspection.AttendanceListQuery{
		SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "attendance",
		Sort: inspection.SortOccurredAtDescending, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	departments, ok := countArgs[9].([]string)
	if !ok || departments == nil || len(departments) != 0 {
		t.Fatalf("department scope argument = %#v, want a non-nil empty array", countArgs[9])
	}
}

func TestConfirmAttendanceImportCommitsIdempotentlyAgainstPostgres(t *testing.T) {
	database := openTestPostgres(t)
	fixture := seedAttendanceConfirmation(t, database, false)
	mutation := confirmationMutation(fixture.input)

	result, err := database.ConfirmAttendanceImport(context.Background(), fixture.input, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != importjob.BatchStatusConfirmed || result.ImportedRows != 1 || result.Version != 2 {
		t.Fatalf("result = %#v", result)
	}
	retry, err := database.ConfirmAttendanceImport(context.Background(), fixture.input, mutation)
	if err != nil || retry != result {
		t.Fatalf("idempotent retry = %#v, %v", retry, err)
	}
	different := fixture.input
	different.ExpectedVersion = result.Version
	different.IdempotencyKey = "different-" + uuid.NewString()
	if _, err := database.ConfirmAttendanceImport(context.Background(), different, mutation); !errors.Is(err, importjob.ErrBatchAlreadyConfirmed) {
		t.Fatalf("different-key error = %v", err)
	}
	var records, details, linkedRows, audits int
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, fixture.input.BatchID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM attendance_details ad JOIN inspection_records ir ON ir.id=ad.record_id WHERE ir.import_batch_id=$1`, fixture.input.BatchID).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, fixture.input.BatchID).Scan(&linkedRows); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE resource_type='import_batch' AND resource_id=$1 AND action=$2`, fixture.input.BatchID, audit.ActionImportBatchConfirmed).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if records != 1 || details != 1 || linkedRows != 1 || audits != 1 {
		t.Fatalf("committed counts records=%d details=%d linked=%d audits=%d", records, details, linkedRows, audits)
	}
}

func TestConfirmAttendanceImportRollsBackAllWritesWhenDetailInsertFails(t *testing.T) {
	database := openTestPostgres(t)
	fixture := seedAttendanceConfirmation(t, database, true)
	if _, err := database.ConfirmAttendanceImport(context.Background(), fixture.input, confirmationMutation(fixture.input)); err == nil {
		t.Fatal("confirmation unexpectedly succeeded with missing classroom")
	}
	var records, linkedRows, audits int
	var status string
	var version, importedRows int
	if err := database.Pool.QueryRow(context.Background(), `SELECT status,version,imported_rows FROM import_batches WHERE id=$1`, fixture.input.BatchID).Scan(&status, &version, &importedRows); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, fixture.input.BatchID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, fixture.input.BatchID).Scan(&linkedRows); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE resource_type='import_batch' AND resource_id=$1`, fixture.input.BatchID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if status != importjob.BatchStatusPreviewReady || version != 1 || importedRows != 0 || records != 0 || linkedRows != 0 || audits != 0 {
		t.Fatalf("rollback state status=%s version=%d imported=%d records=%d linked=%d audits=%d", status, version, importedRows, records, linkedRows, audits)
	}
}

func TestConfirmationAuditEntryContainsOnlySafeBatchSummary(t *testing.T) {
	input := importjob.ConfirmBatchInput{
		BatchID: "batch-1",
		Scope:   importjob.ConfirmScope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-1", DepartmentID: "department-1", ActorID: "actor-1"},
	}
	entry := confirmationAuditEntry(input, 3, 12, 7)
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{"batch-1", "semester-1", "week-1", "type-1", "department-1", "templateVersion", "importedRows"} {
		if !strings.Contains(text, required) {
			t.Errorf("audit entry misses %q: %s", required, text)
		}
	}
	for _, forbidden := range []string{"raw_data_json", "normalized_data_json", "studentNo", "studentName", "actor-1"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("audit entry leaks %q: %s", forbidden, text)
		}
	}
}

func TestConfirmationRecordMustRemainInsideClassAndGradeScope(t *testing.T) {
	input := importjob.ConfirmBatchInput{BatchID: "batch-1", Scope: importjob.ConfirmScope{SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-1", DepartmentID: "department-1", ClassID: "class-1", Grade: "2026", ActorID: "actor-1"}}
	record := inspection.AttendanceRecord{ID: "record-1", SemesterID: "semester-1", WeekID: "week-1", InspectionTypeID: "type-1", DepartmentID: "department-1", ClassID: "class-2", GradeSnapshot: "2026", ImportBatchID: "batch-1", CreatedBy: "actor-1", Source: inspection.RecordSourceExcel, Status: inspection.RecordStatusDraft, DedupKey: "dedup", RowVersion: 1}
	if confirmationRecordMatches(record, input) {
		t.Fatal("record from another class matched narrowed confirmation scope")
	}
	record.ClassID, record.GradeSnapshot = "class-1", "2025"
	if confirmationRecordMatches(record, input) {
		t.Fatal("record from another grade matched narrowed confirmation scope")
	}
}

type attendanceConfirmationFixture struct {
	input importjob.ConfirmBatchInput
}

func openTestPostgres(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	database, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return database
}

// 测试共用一次性数据库，不能用时钟取模分配有限的年级名称。
func unusedTestGradeName(t *testing.T, database *Postgres) string {
	t.Helper()
	var name string
	if err := database.Pool.QueryRow(context.Background(), `SELECT year::text || '级' FROM generate_series(1000,9999) AS year WHERE NOT EXISTS (SELECT 1 FROM grades WHERE name=year::text || '级') ORDER BY year LIMIT 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func seedAttendanceConfirmation(t *testing.T, database *Postgres, invalidClassroom bool) attendanceConfirmationFixture {
	t.Helper()
	ctx := context.Background()
	accountID, yearID, semesterID, weekID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	facultyID, gradeID, classID, studentID, courseID, teachingBuildingID, classroomID, scheduleID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	originalFileID, templateFileID, templateID, batchID, rowID, recordID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	departmentID := "10000000-0000-0000-0000-000000000002"
	inspectionTypeID := "30000000-0000-0000-0000-000000000001"
	unique := uuid.NewString()[:8]
	gradeName := unusedTestGradeName(t, database)
	templateVersion := int(time.Now().UnixNano()%1_000_000_000) + 1
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,$2,'导入确认测试','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, []any{accountID, "confirm-" + unique}},
		{`INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,$2,'2026-01-01','2026-12-31')`, []any{yearID, "year-" + unique}},
		{`INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,$3,'测试学期','2026-01-01','2026-12-31')`, []any{semesterID, yearID, "semester-" + unique}},
		{`INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date) VALUES($1,$2,1,'2026-06-15','2026-06-21')`, []any{weekID, semesterID}},
		{`INSERT INTO faculties(id,name) VALUES($1,$2)`, []any{facultyID, "学院-" + unique}},
		{`INSERT INTO grades(id,name) VALUES($1,$2)`, []any{gradeID, gradeName}},
		{`INSERT INTO classes(id,name,grade_id,faculty_id) VALUES($1,$2,$3,$4)`, []any{classID, "班级-" + unique, gradeID, facultyID}},
		{`INSERT INTO teaching_buildings(id,name) VALUES($1,$2)`, []any{teachingBuildingID, "测试楼-" + unique}},
		{`INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES($1,$2,$3)`, []any{classroomID, teachingBuildingID, unique}},
		{`INSERT INTO courses(id,name) VALUES($1,$2)`, []any{courseID, "数据结构-" + unique}},
		{`INSERT INTO course_classes(course_id,class_id) VALUES($1,$2)`, []any{courseID, classID}},
		{`INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section) VALUES($1,$2,$3,$4,3,1,2)`, []any{scheduleID, courseID, semesterID, classroomID}},
		{`INSERT INTO students(id,student_no,name,grade_id,faculty_id,class_id) VALUES($1,$2,'虚构学生',$3,$4,$5)`, []any{studentID, "student-" + unique, gradeID, facultyID, classID}},
		{`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'test',$2,'source.xlsx',1,$3,'AVAILABLE','IMPORT_SOURCE',$4)`, []any{originalFileID, "source/" + unique, strings.Repeat("a", 64), accountID}},
		{`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'test',$2,'template.xlsx',1,$3,'AVAILABLE','TEMPLATE',$4)`, []any{templateFileID, "template/" + unique, strings.Repeat("b", 64), accountID}},
		{`INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,$2,'到课率模板',$3,$4,$5,'DRAFT',$6)`, []any{templateID, "template-" + unique, inspectionTypeID, templateVersion, templateFileID, accountID}},
		{`INSERT INTO import_batches(id,inspection_type_id,semester_id,week_id,department_id,original_file_id,template_id,template_version,status,total_rows,valid_rows,invalid_rows,version,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PREVIEW_READY',1,1,0,1,$9)`, []any{batchID, inspectionTypeID, semesterID, weekID, departmentID, originalFileID, templateID, templateVersion, accountID}},
	}
	for _, statement := range statements {
		if _, err := database.Pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	recordClassroomID := classroomID
	if invalidClassroom {
		recordClassroomID = uuid.NewString()
	}
	record := inspection.AttendanceRecord{
		ID: recordID, InspectionTypeID: inspectionTypeID, SemesterID: semesterID, WeekID: weekID, DepartmentID: departmentID,
		StudentID: studentID, StudentNoSnapshot: "student-" + unique, StudentNameSnapshot: "虚构学生", ClassID: classID,
		ClassNameSnapshot: "班级-" + unique, GradeSnapshot: gradeName, CourseID: courseID, ClassroomID: recordClassroomID,
		OccurredAt: time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC), AbsenceHours: 2, PeriodText: "第1-2节", Remark: "测试数据",
		Source: inspection.RecordSourceExcel, Status: inspection.RecordStatusDraft, ImportBatchID: batchID,
		DedupKey: strings.Repeat("c", 64), RowVersion: 1, CreatedBy: accountID,
	}
	normalized, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO import_rows(id,batch_id,row_no,raw_data_json,normalized_data_json,validation_status,errors_json) VALUES($1,$2,5,'{}',$3,'VALID','[]')`, rowID, batchID, normalized); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Pool.Exec(ctx, `DELETE FROM inspection_records WHERE import_batch_id=$1`, batchID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM import_rows WHERE batch_id=$1`, batchID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM import_batches WHERE id=$1`, batchID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM import_templates WHERE id=$1`, templateID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM files WHERE id IN ($1,$2)`, originalFileID, templateFileID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM course_schedules WHERE course_id=$1`, courseID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM course_classes WHERE course_id=$1`, courseID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM courses WHERE id=$1`, courseID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM students WHERE id=$1`, studentID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM classes WHERE id=$1`, classID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM classrooms WHERE id=$1`, classroomID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM teaching_buildings WHERE id=$1`, teachingBuildingID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM grades WHERE id=$1`, gradeID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM faculties WHERE id=$1`, facultyID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM semester_weeks WHERE id=$1`, weekID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM semesters WHERE id=$1`, semesterID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM academic_years WHERE id=$1`, yearID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, accountID)
	})
	return attendanceConfirmationFixture{input: importjob.ConfirmBatchInput{
		BatchID: batchID, ExpectedVersion: 1, IdempotencyKey: "confirm-" + unique,
		Scope:         importjob.ConfirmScope{SemesterID: semesterID, WeekID: weekID, InspectionTypeID: inspectionTypeID, DepartmentID: departmentID, ActorID: accountID},
		AuditMetadata: audit.Metadata{ActorID: accountID, RequestID: "request-" + unique},
	}}
}

func confirmationMutation(input importjob.ConfirmBatchInput) audit.Mutation {
	return audit.Mutation{Metadata: input.AuditMetadata, Entry: audit.Entry{
		Action: audit.ActionImportBatchConfirmed, ResourceType: "import_batch", ResourceID: input.BatchID,
		Before: map[string]any{"status": importjob.BatchStatusPreviewReady},
		After:  map[string]any{"status": importjob.BatchStatusConfirmed, "importedRows": 1, "templateVersion": 1, "semesterId": input.Scope.SemesterID, "weekId": input.Scope.WeekID, "departmentId": input.Scope.DepartmentID, "inspectionTypeId": input.Scope.InspectionTypeID},
	}}
}

func TestBusinessMutationAndAuditCommitOrRollbackTogether(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	accountID := uuid.NewString()
	loginName := "audit-test-" + uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(
		id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password
	) VALUES($1,$2,'审计事务测试','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, accountID, loginName); err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, accountID)

	committedAuditID := uuid.NewString()
	err = database.withAudit(ctx, audit.Metadata{RequestID: "request-commit"}, audit.Entry{
		ID: committedAuditID, Action: audit.ActionAccountUpdated, ResourceType: "account", ResourceID: accountID,
		Before: map[string]any{"name": "审计事务测试"}, After: map[string]any{"name": "已提交"},
	}, func(tx pgx.Tx) error {
		_, updateErr := tx.Exec(ctx, `UPDATE accounts SET name='已提交' WHERE id=$1`, accountID)
		return updateErr
	})
	if err != nil {
		t.Fatal(err)
	}

	var name string
	if err := database.Pool.QueryRow(ctx, `SELECT name FROM accounts WHERE id=$1`, accountID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "已提交" {
		t.Fatalf("committed account name = %q", name)
	}
	var committedCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE id=$1`, committedAuditID).Scan(&committedCount); err != nil {
		t.Fatal(err)
	}
	if committedCount != 1 {
		t.Fatalf("committed audit count = %d", committedCount)
	}

	rolledBackAuditID := uuid.NewString()
	err = database.withAudit(ctx, audit.Metadata{RequestID: "request-rollback", IP: "invalid-ip"}, audit.Entry{
		ID: rolledBackAuditID, Action: audit.ActionAccountUpdated, ResourceType: "account", ResourceID: accountID,
		Before: map[string]any{"name": "已提交"}, After: map[string]any{"name": "不应提交"},
	}, func(tx pgx.Tx) error {
		_, updateErr := tx.Exec(ctx, `UPDATE accounts SET name='不应提交' WHERE id=$1`, accountID)
		return updateErr
	})
	if err == nil {
		t.Fatal("withAudit() error = nil, want invalid IP failure")
	}
	if err := database.Pool.QueryRow(ctx, `SELECT name FROM accounts WHERE id=$1`, accountID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "已提交" {
		t.Fatalf("rolled-back account name = %q", name)
	}
	var rolledBackCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE id=$1`, rolledBackAuditID).Scan(&rolledBackCount); err != nil {
		t.Fatal(err)
	}
	if rolledBackCount != 0 {
		t.Fatalf("rolled-back audit count = %d", rolledBackCount)
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE audit_logs SET reason='篡改' WHERE id=$1`, committedAuditID); err == nil {
		t.Fatal("updating an audit log unexpectedly succeeded")
	}
	if _, err := database.Pool.Exec(ctx, `DELETE FROM audit_logs WHERE id=$1`, committedAuditID); err == nil {
		t.Fatal("deleting an audit log unexpectedly succeeded")
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE id=$1`, committedAuditID).Scan(&committedCount); err != nil {
		t.Fatal(err)
	}
	if committedCount != 1 {
		t.Fatalf("append-only audit count = %d", committedCount)
	}
}

func TestFileMetadataTransitionsAndReconciliationAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	file := storage.File{
		ID: uuid.NewString(), StorageProvider: "MINIO", Bucket: "integration-files",
		ObjectKey: "system/integration/" + uuid.NewString(), OriginalName: "integration.txt",
		DeclaredContentType: "text/plain", SizeBytes: 3, Status: storage.StatusPending,
		RetentionType: storage.RetentionTemporary, CreatedAt: now, UpdatedAt: now,
	}
	defer database.Pool.Exec(ctx, `DELETE FROM files WHERE id=$1`, file.ID)
	if err := database.CreateFile(ctx, file); err != nil {
		t.Fatal(err)
	}
	stored, err := database.FindFile(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ObjectKey != file.ObjectKey || stored.Status != storage.StatusPending {
		t.Fatalf("stored file = %#v", stored)
	}

	stored.Status = storage.StatusAvailable
	stored.DetectedContentType = "text/plain; charset=utf-8"
	stored.SHA256 = strings.Repeat("a", 64)
	mutation := audit.Mutation{Entry: audit.Entry{
		Action: audit.ActionFileUploadSucceeded, ResourceType: "file", ResourceID: file.ID,
		Before: map[string]any{"status": storage.StatusPending}, After: map[string]any{"status": storage.StatusAvailable},
	}}
	if err := database.MarkFileAvailable(ctx, stored, mutation); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := database.Pool.QueryRow(ctx, `SELECT status FROM files WHERE id=$1`, file.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != storage.StatusAvailable {
		t.Fatalf("status = %q", status)
	}
	var auditCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_type='file' AND resource_id=$1 AND action=$2`, file.ID, audit.ActionFileUploadSucceeded).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("upload audit count = %d", auditCount)
	}

	if _, err := database.Pool.Exec(ctx, `UPDATE files SET status=$2,updated_at=$3 WHERE id=$1`, file.ID, storage.StatusFailed, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	candidates, err := database.ListFileReconciliationCandidates(ctx, file.Bucket, now.Add(-30*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range candidates {
		if candidate.ID == file.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale file %s not returned in %#v", file.ID, candidates)
	}
}

func TestCreateSystemAccountWithoutStudentNumber(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	account := auth.Account{
		ID: uuid.NewString(), LoginName: "system-test-" + uuid.NewString(), Name: "系统测试账号",
		PasswordHash: "not-used", Status: auth.StatusActive, AccountKind: auth.AccountKindSystem, SystemRole: auth.SystemRoleSuperAdmin,
		IsSuperAdmin: true, MustChangePassword: true, CreatedAt: time.Now().UTC(),
	}
	defer database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, account.ID)
	if err := database.Create(ctx, account, testAccountMutation(account)); err != nil {
		t.Fatal(err)
	}
}

func TestReviewRegistrationAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var departmentID, organizationRoleID string
	if err := database.Pool.QueryRow(ctx, `SELECT id FROM departments WHERE enabled ORDER BY code LIMIT 1`).Scan(&departmentID); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT id FROM organization_roles WHERE enabled ORDER BY sort_order,code LIMIT 1`).Scan(&organizationRoleID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	unique := uuid.NewString()[:8]
	reviewer := auth.Account{
		ID: uuid.NewString(), LoginName: "reviewer-test-" + uuid.NewString(), Name: "审核测试管理员",
		PasswordHash: "not-used", Status: auth.StatusActive, AccountKind: auth.AccountKindSystem,
		SystemRole: auth.SystemRoleSuperAdmin, IsSuperAdmin: true, CreatedAt: now,
	}
	request := auth.Account{
		ID: uuid.NewString(), LoginName: "registration-" + unique, StudentNo: "9" + unique, Name: "审核测试用户",
		PasswordHash: "not-used", Status: auth.StatusPending, AccountKind: auth.AccountKindPerson, SystemRole: auth.SystemRoleMember,
		RequestedDepartmentID: departmentID, RequestedOrgRoleID: organizationRoleID, CreatedAt: now,
	}
	defer func() {
		_, _ = database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, request.ID)
		_, _ = database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, reviewer.ID)
	}()
	if err := database.Create(ctx, reviewer, testAccountMutation(reviewer)); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(ctx, request, testAccountMutation(request)); err != nil {
		t.Fatal(err)
	}

	approved, err := database.ReviewRegistration(ctx, request.ID, reviewer.ID, auth.ReviewApprove, "", now, audit.Mutation{
		Metadata: audit.Metadata{ActorID: reviewer.ID},
		Entry: audit.Entry{Action: audit.ActionRegistrationReviewed, ResourceType: "account", ResourceID: request.ID,
			Before: map[string]any{"status": auth.StatusPending}, After: map[string]any{"status": auth.StatusActive}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != auth.StatusActive || approved.PrimaryDepartmentID != departmentID {
		t.Fatalf("approved account=%+v", approved)
	}
	var roleCount int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM user_org_roles WHERE account_id=$1 AND department_id=$2 AND organization_role_id=$3`, request.ID, departmentID, organizationRoleID).Scan(&roleCount); err != nil {
		t.Fatal(err)
	}
	if roleCount != 1 {
		t.Fatalf("organization role count=%d", roleCount)
	}
}

func testAccountMutation(account auth.Account) audit.Mutation {
	return audit.Mutation{Entry: audit.Entry{
		Action: audit.ActionAccountCreated, ResourceType: "account", ResourceID: account.ID,
		After: map[string]any{"loginName": account.LoginName, "status": account.Status},
	}}
}

func TestPhaseOneRepositoriesAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	masterService := masterdata.NewService(database)
	faculty, err := masterService.Create(ctx, "", masterdata.Record{Kind: masterdata.KindFaculty, Name: "测试院系" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM master_data_changes WHERE entity_id=$1`, faculty.ID)
	defer database.Pool.Exec(ctx, `DELETE FROM faculties WHERE id=$1`, faculty.ID)
	page, err := masterService.List(ctx, masterdata.Query{Kind: masterdata.KindFaculty, Name: faculty.Name, Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("master data page=%+v err=%v", page, err)
	}
	faculty.Name += "（已修改）"
	if _, err := masterService.Update(ctx, "", faculty); err != nil {
		t.Fatal(err)
	}
	changes, err := masterService.ListChanges(ctx, masterdata.KindFaculty, faculty.ID)
	if err != nil || len(changes) != 2 {
		t.Fatalf("changes=%+v err=%v", changes, err)
	}
	unique := uuid.NewString()[:8]
	createdRecords := []masterdata.Record{}
	defer func() {
		tables := map[string]string{masterdata.KindFaculty: "faculties", masterdata.KindGrade: "grades", masterdata.KindMajor: "majors", masterdata.KindStaff: "staff", masterdata.KindClass: "classes", masterdata.KindDormitoryArea: "dormitory_areas", masterdata.KindDormitoryBuilding: "dormitory_buildings", masterdata.KindDormitory: "dormitories", masterdata.KindTeachingBuilding: "teaching_buildings", masterdata.KindClassroom: "classrooms", masterdata.KindCourse: "courses", masterdata.KindStudent: "students"}
		for index := len(createdRecords) - 1; index >= 0; index-- {
			record := createdRecords[index]
			_, _ = database.Pool.Exec(ctx, `DELETE FROM `+tables[record.Kind]+` WHERE id=$1`, record.ID)
			_, _ = database.Pool.Exec(ctx, `DELETE FROM master_data_changes WHERE entity_id=$1`, record.ID)
		}
	}()
	createRecord := func(record masterdata.Record) masterdata.Record {
		t.Helper()
		created, createErr := masterService.Create(ctx, "", record)
		if createErr != nil {
			t.Fatal(createErr)
		}
		createdRecords = append(createdRecords, created)
		return created
	}
	grade := createRecord(masterdata.Record{Kind: masterdata.KindGrade, Name: unusedTestGradeName(t, database)})
	major := createRecord(masterdata.Record{Kind: masterdata.KindMajor, Name: "测试专业" + unique, FacultyID: faculty.ID})
	staff := createRecord(masterdata.Record{Kind: masterdata.KindStaff, Code: "E" + unique, Name: "测试教师" + unique, FacultyID: faculty.ID})
	class := createRecord(masterdata.Record{Kind: masterdata.KindClass, Name: "测试班级" + unique, GradeID: grade.ID, FacultyID: faculty.ID, MajorID: major.ID, HeadTeacherID: staff.ID, EnrolledSize: 30})
	dormitoryArea := createRecord(masterdata.Record{Kind: masterdata.KindDormitoryArea, Name: "测试宿舍区" + unique})
	dormitoryBuilding := createRecord(masterdata.Record{Kind: masterdata.KindDormitoryBuilding, Name: "测试宿舍楼" + unique, DormitoryAreaID: dormitoryArea.ID})
	dormitory := createRecord(masterdata.Record{Kind: masterdata.KindDormitory, DormitoryBuildingID: dormitoryBuilding.ID, RoomNo: unique})
	teachingBuilding := createRecord(masterdata.Record{Kind: masterdata.KindTeachingBuilding, Name: "测试教学楼" + unique})
	classroom := createRecord(masterdata.Record{Kind: masterdata.KindClassroom, TeachingBuildingID: teachingBuilding.ID, RoomNo: unique})
	course := createRecord(masterdata.Record{Kind: masterdata.KindCourse, Name: "测试课程" + unique, ClassID: class.ID, ClassroomID: classroom.ID})
	if course.Name != "测试课程"+unique || course.ClassID != "" || course.ClassroomID != "" {
		t.Fatalf("persisted course should only expose its name: %+v", course)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO courses(id,name) VALUES($1,$2)`, uuid.NewString(), course.Name); err == nil {
		t.Fatal("database accepted a duplicate canonical course name")
	}
	createRecord(masterdata.Record{Kind: masterdata.KindStudent, Code: "S" + unique, Name: "测试学生" + unique, Gender: "MALE", GradeID: grade.ID, FacultyID: faculty.ID, MajorID: major.ID, ClassID: class.ID, DormitoryID: dormitory.ID})
	for _, record := range createdRecords {
		listed, listErr := masterService.List(ctx, masterdata.Query{Kind: record.Kind, Name: record.Name, Page: 1, PageSize: 20})
		if listErr != nil || listed.Total != 1 || len(listed.Items) != 1 {
			t.Fatalf("kind=%s page=%+v err=%v", record.Kind, listed, listErr)
		}
	}

	dictionaryService := dictionary.NewService(database)
	dictionaryType, err := dictionaryService.CreateType(ctx, dictionary.Type{Code: "T" + uuid.NewString()[:8], Name: "测试字典" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM dictionary_types WHERE id=$1`, dictionaryType.ID)
	item, err := dictionaryService.CreateItem(ctx, dictionary.Item{TypeID: dictionaryType.ID, Value: "ONE", Label: "一", SortOrder: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM dictionary_items WHERE id=$1`, item.ID)
	catalog, err := dictionaryService.List(ctx, dictionaryType.ID)
	if err != nil || len(catalog.Items) != 1 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}

	adminID := uuid.NewString()
	loginName := "integration-admin-" + uuid.NewString()[:8]
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(
		id, login_name, student_no, name, password_hash, status, account_kind,
		system_role, is_super_admin, must_change_password, created_at
	) VALUES($1,$2,NULL,'集成测试管理员','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false,NOW())`, adminID, loginName); err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM accounts WHERE id=$1`, adminID)
	permissionService := permission.NewService(database)
	rule, err := permissionService.CreateRule(ctx, adminID, permission.Rule{AccountID: adminID, Action: permission.ActionArchiveView, Effect: permission.EffectAllow}, audit.Metadata{ActorID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM user_permission_rules WHERE id=$1`, rule.ID)
	rules, err := permissionService.ListRules(ctx, adminID)
	if err != nil || len(rules) == 0 {
		t.Fatalf("rules=%+v err=%v", rules, err)
	}
}
