package importjob_test

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestAttendanceMutationsPostgresIntegration(t *testing.T) {
	url := os.Getenv("MUTATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MUTATION_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	parsed, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("fixture.xlsx", fixture.workbook)
	if err != nil {
		t.Fatal(err)
	}
	row := parsed.Rows[0].Values
	var classID string
	if err = db.Pool.QueryRow(ctx, `SELECT id FROM classes WHERE name=$1`, row["class_name"]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	input := inspection.ManualAttendanceInput{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, ClassID: classID, Grade: "2026级", ClassName: row["class_name"], StudentNo: row["student_no"], StudentName: row["student_name"], CourseName: row["course_name"], Classroom: row["classroom_code"], Date: row["absence_date"], Time: row["absence_time"], Period: row["period"], AbsenceHours: 2}
	permissions := permission.NewService(db)
	manual := inspection.NewManualService(db, importjob.NewAttendanceAdapter(db), academic.NewService(db), organization.NewService(db), permissions)
	service := inspection.NewMutationService(db, manual, permissions)
	actor := inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}
	first, err := manual.Create(ctx, actor, input, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := input
	secondInput.Period = "9-10"
	second, err := manual.Create(ctx, actor, secondInput, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	input.Remark = "修正备注"
	updated, err := service.Update(ctx, actor, first.ID, inspection.AttendanceEditInput{Version: 1, Reason: "核对原始记录", Record: input}, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID || updated.CreatedBy != first.CreatedBy || updated.Source != first.Source || updated.RowVersion != 2 || updated.Remark != input.Remark {
		t.Fatal("identity or version changed incorrectly")
	}
	if _, err = service.Update(ctx, actor, first.ID, inspection.AttendanceEditInput{Version: 1, Reason: "旧版本", Record: input}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordVersionConflict) {
		t.Fatalf("stale version=%v", err)
	}
	if _, err = service.Update(ctx, actor, first.ID, inspection.AttendanceEditInput{Version: 2, Reason: "重复", Record: secondInput}, audit.Metadata{}); !errors.Is(err, inspection.ErrAttendanceDuplicate) {
		t.Fatalf("duplicate=%v", err)
	}
	// Failure in the audit INSERT must roll back the business UPDATE.
	input.Remark = "不应保存"
	if _, err = service.Update(ctx, actor, first.ID, inspection.AttendanceEditInput{Version: 2, Reason: "故障注入", Record: input}, audit.Metadata{IP: "invalid-ip"}); err == nil {
		t.Fatal("expected audit failure")
	}
	fresh, err := service.Detail(ctx, actor, first.ID)
	if err != nil || fresh.RowVersion != 2 || fresh.Remark != "修正备注" {
		t.Fatalf("audit rollback record=%+v err=%v", fresh, err)
	}
	// Actual SQL ownership and independent management scopes.
	own := inspection.MutationAccess{ActorID: fixture.accountID, Permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: classID}}}}
	if _, err = db.ReadAttendanceForMutation(ctx, first.ID, own); err != nil {
		t.Fatal(err)
	}
	other := own
	other.ActorID = uuid.NewString()
	if _, err = db.ReadAttendanceForMutation(ctx, first.ID, other); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatalf("entry can read others=%v", err)
	}
	other.Permissions = []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: integrationDepartmentID}}}
	if _, err = db.ReadAttendanceForMutation(ctx, first.ID, other); err != nil {
		t.Fatal(err)
	}
	other.Permissions = append(other.Permissions, permission.EffectivePermission{Action: permission.ActionInspectionManage, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: classID}})
	if _, err = db.ReadAttendanceForMutation(ctx, first.ID, other); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatalf("deny=%v", err)
	}

	managerID := uuid.NewString()
	if _, err = db.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,$2,'管理测试','unused','ACTIVE','SYSTEM','MEMBER',false,false)`, managerID, "manager-"+managerID); err != nil {
		t.Fatal(err)
	}
	managedPermissions := mutationPermissions{permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: integrationDepartmentID}}}}
	managedManual := inspection.NewManualService(db, importjob.NewAttendanceAdapter(db), academic.NewService(db), organization.NewService(db), managedPermissions)
	managedService := inspection.NewMutationService(db, managedManual, managedPermissions)
	manager := inspection.ManualActor{ID: managerID}
	managed, err := managedService.Update(ctx, manager, second.ID, inspection.AttendanceEditInput{Version: second.RowVersion, Reason: "独立管理授权", Record: secondInput}, audit.Metadata{})
	if err != nil || managed.CreatedBy != fixture.accountID {
		t.Fatalf("manage others without entry=%v", err)
	}
	second = managed
	outside := secondInput
	outside.DepartmentID = uuid.NewString()
	if _, err = managedService.Update(ctx, manager, second.ID, inspection.AttendanceEditInput{Version: second.RowVersion, Reason: "目标范围越权", Record: outside}, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
		t.Fatalf("destination scope=%v", err)
	}
	for _, status := range []string{"PENDING_REVIEW", "RETURNED", "CONFIRMED", "LOCKED"} {
		if _, err = db.Pool.Exec(ctx, `UPDATE inspection_records SET status=$2 WHERE id=$1`, second.ID, status); err != nil {
			t.Fatal(err)
		}
		err = service.Delete(ctx, actor, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: first.ID, Version: 2}, {ID: second.ID, Version: second.RowVersion}}, Reason: "测试批量拒绝"}, audit.Metadata{})
		if !errors.Is(err, inspection.ErrRecordNotEditable) {
			t.Fatalf("status %s=%v", status, err)
		}
		if _, err = service.Detail(ctx, actor, first.ID); err != nil {
			t.Fatal("batch partially deleted")
		}
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE inspection_records SET status='DRAFT' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	batch := inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: first.ID, Version: 2}, {ID: second.ID, Version: second.RowVersion}}, Reason: "清理重复草稿"}
	broken := batch
	broken.Items = append([]inspection.VersionedRecord(nil), batch.Items...)
	broken.Items[1].Version = 99
	if err = service.Delete(ctx, actor, broken, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordVersionConflict) {
		t.Fatalf("batch version=%v", err)
	}
	broken.Items[1] = inspection.VersionedRecord{ID: uuid.NewString(), Version: 1}
	if err = service.Delete(ctx, actor, broken, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatalf("batch missing=%v", err)
	}
	if err = service.Delete(ctx, actor, batch, audit.Metadata{IP: "invalid-ip"}); err == nil {
		t.Fatal("expected batch audit failure")
	}
	if _, err = service.Detail(ctx, actor, first.ID); err != nil {
		t.Fatal("audit failure partially deleted")
	}
	if err = service.Delete(ctx, actor, batch, audit.Metadata{}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Detail(ctx, actor, first.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatalf("soft deleted detail=%v", err)
	}
	var count int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_details WHERE record_id=ANY($1::uuid[])`, []string{first.ID, second.ID}).Scan(&count); err != nil || count != 2 {
		t.Fatalf("details=%d err=%v", count, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=ANY($1::text[]) AND action='INSPECTION_RECORD_DELETED'`, []string{first.ID, second.ID}).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audits=%d err=%v", count, err)
	}
}

type mutationPermissions struct {
	permissions []permission.EffectivePermission
}

func (p mutationPermissions) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.permissions}, nil
}
