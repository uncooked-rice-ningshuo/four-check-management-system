package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	candidates, err := database.ListFileReconciliationCandidates(ctx, now.Add(-30*time.Minute), 10)
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
	faculty, err := masterService.Create(ctx, "", masterdata.Record{Kind: masterdata.KindFaculty, Code: "T" + uuid.NewString()[:8], Name: "测试院系" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Exec(ctx, `DELETE FROM master_data_changes WHERE entity_id=$1`, faculty.ID)
	defer database.Pool.Exec(ctx, `DELETE FROM faculties WHERE id=$1`, faculty.ID)
	page, err := masterService.List(ctx, masterdata.Query{Kind: masterdata.KindFaculty, Keyword: faculty.Code, Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("master data page=%+v err=%v", page, err)
	}
	faculty.Name = "测试院系（已修改）"
	if _, err := masterService.Update(ctx, "", faculty); err != nil {
		t.Fatal(err)
	}
	changes, err := masterService.ListChanges(ctx, masterdata.KindFaculty, faculty.ID)
	if err != nil || len(changes) != 2 {
		t.Fatalf("changes=%+v err=%v", changes, err)
	}
	unique := uuid.NewString()[:8]
	createdRecords := []masterdata.Record{}
	createRecord := func(record masterdata.Record) masterdata.Record {
		t.Helper()
		created, createErr := masterService.Create(ctx, "", record)
		if createErr != nil {
			t.Fatal(createErr)
		}
		createdRecords = append(createdRecords, created)
		return created
	}
	major := createRecord(masterdata.Record{Kind: masterdata.KindMajor, Code: "M" + unique, Name: "测试专业" + unique, FacultyID: faculty.ID})
	staff := createRecord(masterdata.Record{Kind: masterdata.KindStaff, Code: "E" + unique, Name: "测试教师", FacultyID: faculty.ID})
	class := createRecord(masterdata.Record{Kind: masterdata.KindClass, Code: "C" + unique, Name: "测试班级" + unique, Grade: "2026", FacultyID: faculty.ID, MajorID: major.ID, HeadTeacherID: staff.ID, EnrolledSize: 30})
	dormitory := createRecord(masterdata.Record{Kind: masterdata.KindDormitory, Code: "D" + unique, Name: "D" + unique, CampusArea: "东区", BuildingNo: "1", RoomNo: unique})
	classroom := createRecord(masterdata.Record{Kind: masterdata.KindClassroom, Code: "R" + unique, Name: "R" + unique, Building: "教学楼", RoomNo: unique, Floor: 2})
	createRecord(masterdata.Record{Kind: masterdata.KindCourse, Code: "K" + unique, Name: "测试课程", TeacherID: staff.ID, ClassID: class.ID, ClassroomID: classroom.ID, ScheduleText: "周一 1-2 节"})
	createRecord(masterdata.Record{Kind: masterdata.KindStudent, Code: "S" + unique, Name: "测试学生", Gender: "MALE", Grade: "2026", FacultyID: faculty.ID, MajorID: major.ID, ClassID: class.ID, DormitoryID: dormitory.ID})
	defer func() {
		for index := len(createdRecords) - 1; index >= 0; index-- {
			record := createdRecords[index]
			_, _ = database.Pool.Exec(ctx, `DELETE FROM `+record.Kind+` WHERE id=$1`, record.ID)
			_, _ = database.Pool.Exec(ctx, `DELETE FROM master_data_changes WHERE entity_id=$1`, record.ID)
		}
	}()
	for _, record := range createdRecords {
		listed, listErr := masterService.List(ctx, masterdata.Query{Kind: record.Kind, Keyword: record.Code, Page: 1, PageSize: 20})
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
