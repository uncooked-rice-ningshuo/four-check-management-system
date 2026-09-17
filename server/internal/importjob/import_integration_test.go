package importjob_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/xuri/excelize/v2"
)

const (
	integrationDepartmentID     = "10000000-0000-0000-0000-000000000002"
	integrationInspectionTypeID = "30000000-0000-0000-0000-000000000001"
)

type integrationEnvironment struct {
	databaseURL string
	endpoint    string
	accessKey   string
	secretKey   string
	useTLS      bool
}

type attendanceIntegrationFixture struct {
	accountID, semesterID, weekID string
	workbook                      []byte
}

func TestAttendanceImportIntegrationAgainstPostgresAndMinIO(t *testing.T) {
	environment := requireIntegrationEnvironment(t)
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, environment.databaseURL)
	database, err := store.Open(ctx, environment.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	objects := newIntegrationMinIO(t, ctx, environment)
	files := storage.NewService(database, objects, 16<<20)
	fixture := seedAttendanceIntegrationFixture(t, ctx, database)
	parser := importjob.NewAttendanceXLSXParser(16 << 20)
	imports := importjob.NewService(database, files, parser, importjob.NewAttendanceAdapter(database))
	metadata := audit.Metadata{ActorID: fixture.accountID, RequestID: "integration-" + uuid.NewString()}

	templateFile := storeIntegrationFile(t, ctx, files, fixture.workbook, storage.RetentionTemplate, fixture.accountID, metadata)
	sourceFile := storeIntegrationFile(t, ctx, files, fixture.workbook, storage.RetentionImportSource, fixture.accountID, metadata)
	t.Cleanup(func() {
		_ = objects.Remove(ctx, templateFile.ObjectKey)
		_ = objects.Remove(ctx, sourceFile.ObjectKey)
	})
	templateID := uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `UPDATE import_templates SET status='RETIRED' WHERE inspection_type_id=$1 AND status='PUBLISHED'`, integrationInspectionTypeID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,published_at,created_by) VALUES($1,$2,'到课率集成测试模板',$3,1,$4,'PUBLISHED',now(),$5)`, templateID, "integration-"+uuid.NewString(), integrationInspectionTypeID, templateFile.ID, fixture.accountID); err != nil {
		t.Fatal(err)
	}

	created, err := imports.CreateBatch(ctx, importjob.CreateBatchInput{
		InspectionTypeID: integrationInspectionTypeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID,
		DepartmentID: integrationDepartmentID, SourceFile: sourceFile, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := imports.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: created.ID, ExpectedVersion: created.Version, AuditMetadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != importjob.BatchStatusPreviewReady || preview.TotalRows != 2 || preview.ValidRows != 2 || preview.InvalidRows != 0 {
		t.Fatalf("preview = %#v", preview)
	}
	objectInfo, err := objects.Stat(ctx, sourceFile.ObjectKey)
	if err != nil || objectInfo.Size != int64(len(fixture.workbook)) || len(sourceFile.SHA256) != 64 {
		t.Fatalf("source object info=%#v sha=%q error=%v", objectInfo, sourceFile.SHA256, err)
	}

	wrongScope := importjob.ConfirmBatchInput{
		BatchID: preview.ID, ExpectedVersion: preview.Version, IdempotencyKey: "wrong-scope-" + uuid.NewString(),
		Scope:         importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: "10000000-0000-0000-0000-000000000003", ActorID: fixture.accountID},
		AuditMetadata: metadata,
	}
	if _, err := imports.ConfirmBatch(ctx, wrongScope); !errors.Is(err, importjob.ErrBatchScopeMismatch) {
		t.Fatalf("wrong-scope confirmation error = %v", err)
	}
	confirmed, err := imports.ConfirmBatch(ctx, importjob.ConfirmBatchInput{
		BatchID: preview.ID, ExpectedVersion: preview.Version, IdempotencyKey: "confirm-" + uuid.NewString(),
		Scope:         importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, ActorID: fixture.accountID},
		AuditMetadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != importjob.BatchStatusConfirmed || confirmed.ImportedRows != 2 {
		t.Fatalf("confirmed = %#v", confirmed)
	}
	assertConfirmedIntegrationState(t, ctx, database, preview.ID)

	tamperedFile := storeIntegrationFile(t, ctx, files, fixture.workbook, storage.RetentionImportSource, fixture.accountID, metadata)
	t.Cleanup(func() { _ = objects.Remove(ctx, tamperedFile.ObjectKey) })
	tamperedBatch, err := imports.CreateBatch(ctx, importjob.CreateBatchInput{
		InspectionTypeID: integrationInspectionTypeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID,
		DepartmentID: integrationDepartmentID, SourceFile: tamperedFile, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	tamperedContent := []byte("not-the-original-workbook")
	if _, err := objects.Put(ctx, tamperedFile.ObjectKey, bytes.NewReader(tamperedContent), int64(len(tamperedContent)), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := imports.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: tamperedBatch.ID, ExpectedVersion: tamperedBatch.Version, AuditMetadata: metadata}); !errors.Is(err, storage.ErrIntegrityMismatch) {
		t.Fatalf("tampered parse error = %v", err)
	}
	failed, err := imports.GetBatch(ctx, tamperedBatch.ID)
	if err != nil || failed.Status != importjob.BatchStatusParseFailed || failed.FailureCode != string(importjob.ErrorSourceFileUnavailable) {
		t.Fatalf("tampered batch=%#v error=%v", failed, err)
	}
}

func requireIntegrationEnvironment(t *testing.T) integrationEnvironment {
	t.Helper()
	values := make(map[string]string)
	for _, key := range []string{"TEST_DATABASE_URL", "TEST_MINIO_ENDPOINT", "TEST_MINIO_ACCESS_KEY", "TEST_MINIO_SECRET_KEY"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			t.Skip("PostgreSQL/MinIO integration environment is not configured")
		}
		values[key] = value
	}
	useTLS, _ := strconv.ParseBool(os.Getenv("TEST_MINIO_USE_TLS"))
	return integrationEnvironment{databaseURL: values["TEST_DATABASE_URL"], endpoint: values["TEST_MINIO_ENDPOINT"], accessKey: values["TEST_MINIO_ACCESS_KEY"], secretKey: values["TEST_MINIO_SECRET_KEY"], useTLS: useTLS}
}

func applyIntegrationMigrations(t *testing.T, ctx context.Context, databaseURL string) {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	paths, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("migration paths=%v error=%v", paths, err)
	}
	for _, path := range paths {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := connection.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(path), execErr)
		}
	}
}

func newIntegrationMinIO(t *testing.T, ctx context.Context, environment integrationEnvironment) *storage.MinIO {
	t.Helper()
	bucket := "import-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := minio.New(environment.endpoint, &minio.Options{Creds: credentials.NewStaticV4(environment.accessKey, environment.secretKey, ""), Secure: environment.useTLS})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.RemoveBucket(ctx, bucket) })
	objects, err := storage.NewMinIO(storage.MinIOConfig{
		Endpoint: environment.endpoint, AccessKey: environment.accessKey, SecretKey: environment.secretKey, Bucket: bucket,
		UseTLS: environment.useTLS, ConnectTimeout: 2 * time.Second, OperationTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return objects
}

func seedAttendanceIntegrationFixture(t *testing.T, ctx context.Context, database *store.Postgres) attendanceIntegrationFixture {
	t.Helper()
	unique := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	workbook, values := integrationWorkbook(t, unique)
	accountID, yearID, semesterID, weekID, facultyID, gradeID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	classAID, classBID, buildingAID, buildingBID, roomAID, roomBID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	courseAID, courseBID := uuid.NewString(), uuid.NewString()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,$2,'集成测试账号','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, []any{accountID, "integration-" + unique}},
		{`INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,$2,'2026-01-01','2026-12-31')`, []any{yearID, "integration-year-" + unique}},
		{`INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,'FIRST',$3,'2026-01-01','2026-12-31')`, []any{semesterID, yearID, "集成测试学期-" + unique}},
		{`INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date) VALUES($1,$2,1,'2026-06-15','2026-06-21')`, []any{weekID, semesterID}},
		{`INSERT INTO faculties(id,name) VALUES($1,$2)`, []any{facultyID, "测试学院-" + unique}},
		{`INSERT INTO grades(id,name) VALUES($1,'2026级')`, []any{gradeID}},
		{`INSERT INTO classes(id,name,grade_id,faculty_id) VALUES($1,$2,$3,$4)`, []any{classAID, values["classA"], gradeID, facultyID}},
		{`INSERT INTO classes(id,name,grade_id,faculty_id) VALUES($1,$2,$3,$4)`, []any{classBID, values["classB"], gradeID, facultyID}},
		{`INSERT INTO teaching_buildings(id,name) VALUES($1,$2)`, []any{buildingAID, "测试楼A-" + unique}},
		{`INSERT INTO teaching_buildings(id,name) VALUES($1,$2)`, []any{buildingBID, "测试楼B-" + unique}},
		{`INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES($1,$2,'101')`, []any{roomAID, buildingAID}},
		{`INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES($1,$2,'202')`, []any{roomBID, buildingBID}},
		{`INSERT INTO courses(id,name) VALUES($1,$2)`, []any{courseAID, values["courseA"]}},
		{`INSERT INTO courses(id,name) VALUES($1,$2)`, []any{courseBID, values["courseB"]}},
		{`INSERT INTO course_classes(course_id,class_id) VALUES($1,$2)`, []any{courseAID, classAID}},
		{`INSERT INTO course_classes(course_id,class_id) VALUES($1,$2)`, []any{courseBID, classBID}},
		{`INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section) VALUES($1,$2,$3,$4,1,1,2)`, []any{uuid.NewString(), courseAID, semesterID, roomAID}},
		{`INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section) VALUES($1,$2,$3,$4,2,3,4)`, []any{uuid.NewString(), courseBID, semesterID, roomBID}},
		{`INSERT INTO students(id,student_no,name,grade_id,faculty_id,class_id) VALUES($1,$2,$3,$4,$5,$6)`, []any{uuid.NewString(), values["studentNoA"], values["studentNameA"], gradeID, facultyID, classAID}},
		{`INSERT INTO students(id,student_no,name,grade_id,faculty_id,class_id) VALUES($1,$2,$3,$4,$5,$6)`, []any{uuid.NewString(), values["studentNoB"], values["studentNameB"], gradeID, facultyID, classBID}},
	}
	for _, statement := range statements {
		if _, err := database.Pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	return attendanceIntegrationFixture{accountID: accountID, semesterID: semesterID, weekID: weekID, workbook: workbook}
}

func integrationWorkbook(t *testing.T, unique string) ([]byte, map[string]string) {
	t.Helper()
	path := filepath.Join("testdata", "attendance-v1-valid.xlsx")
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	values := map[string]string{
		"studentNoA": "S" + unique + "A", "studentNameA": "虚构学生甲-" + unique, "classA": "示例班级A-" + unique,
		"courseA": "数据结构-" + unique, "roomA": "测试楼A-" + unique + " 101",
		"studentNoB": "S" + unique + "B", "studentNameB": "虚构学生乙-" + unique, "classB": "示例班级B-" + unique,
		"courseB": "操作系统-" + unique, "roomB": "测试楼B-" + unique + " 202",
	}
	for cell, key := range map[string]string{"A5": "studentNoA", "B5": "studentNameA", "C5": "classA", "D5": "courseA", "I5": "roomA", "A6": "studentNoB", "B6": "studentNameB", "C6": "classB", "D6": "courseB", "I6": "roomB"} {
		if err := workbook.SetCellValue("到课率导入", cell, values[key]); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), values
}

func storeIntegrationFile(t *testing.T, ctx context.Context, files *storage.Service, content []byte, retention, accountID string, metadata audit.Metadata) storage.File {
	t.Helper()
	file, err := files.Store(ctx, storage.StoreInput{
		OriginalName: "attendance-" + uuid.NewString() + ".xlsx", DeclaredContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Size: int64(len(content)), RetentionType: retention, CreatedBy: accountID, Body: bytes.NewReader(content),
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func assertConfirmedIntegrationState(t *testing.T, ctx context.Context, database *store.Postgres, batchID string) {
	t.Helper()
	var records, details, linkedRows, audits int
	queries := []struct {
		name string
		sql  string
		to   *int
	}{
		{"records", `SELECT count(*) FROM inspection_records WHERE import_batch_id=$1`, &records},
		{"details", `SELECT count(*) FROM attendance_details ad JOIN inspection_records ir ON ir.id=ad.record_id WHERE ir.import_batch_id=$1`, &details},
		{"linked rows", `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL`, &linkedRows},
		{"audits", `SELECT count(*) FROM audit_logs WHERE resource_type='import_batch' AND resource_id=$1 AND action=$2`, &audits},
	}
	for _, query := range queries {
		args := []any{batchID}
		if query.name == "audits" {
			args = append(args, audit.ActionImportBatchConfirmed)
		}
		if err := database.Pool.QueryRow(ctx, query.sql, args...).Scan(query.to); err != nil {
			t.Fatal(err)
		}
	}
	if records != 2 || details != 2 || linkedRows != 2 || audits != 1 {
		t.Fatal(fmt.Sprintf("confirmed counts records=%d details=%d linkedRows=%d audits=%d", records, details, linkedRows, audits))
	}
}
