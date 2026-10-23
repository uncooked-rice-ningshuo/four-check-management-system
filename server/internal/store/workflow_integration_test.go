package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/dashboard"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workflowPermissions struct {
	rules []permission.EffectivePermission
}

func (p *workflowPermissions) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.rules}, nil
}

func TestSubmitDepartmentBatchPostgres(t *testing.T) {
	url := os.Getenv("WORKFLOW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WORKFLOW_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatal("requires empty disposable database", err)
	}
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(contents)); err != nil {
			t.Fatal(file, err)
		}
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	const account = "a0000000-0000-0000-0000-000000000001"
	const reviewer = "b0000000-0000-0000-0000-000000000001"
	const first = "a0000000-0000-0000-0000-000000000014"
	const second = "a0000000-0000-0000-0000-000000000024"
	const third = "a0000000-0000-0000-0000-000000000034"
	const class = "a0000000-0000-0000-0000-000000000006"
	const department = "10000000-0000-0000-0000-000000000002"
	if _, err = pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
 VALUES($1,'workflow-reviewer','Synthetic reviewer','not-a-login-hash','ACTIVE','SYSTEM','MEMBER',false,false)`, reviewer); err != nil {
		t.Fatal(err)
	}
	var typeID string
	if err = pool.QueryRow(ctx, "SELECT id FROM inspection_types WHERE code='ATTENDANCE_RATE'").Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter)
	 VALUES($1,$2,true,true) ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true`, department, typeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE inspection_records SET class_id=$1,grade_snapshot='2026',
 student_id='a0000000-0000-0000-0000-000000000007',student_no_snapshot='S-FIXTURE',
 student_name_snapshot='Synthetic student',class_name_snapshot='Fixture class' WHERE id=$2`, class, first); err != nil {
		t.Fatal(err)
	}
	for _, record := range []struct {
		id, key, offset string
	}{{second, "b", "1 hour"}, {third, "c", "2 hours"}} {
		_, err = pool.Exec(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,
 student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,
 violation_count,remark,source,dedup_key,created_by)
	SELECT $1,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,
	student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at+$2::interval,
	violation_count,remark,source,repeat($3,64),created_by FROM inspection_records WHERE id=$4`, record.id, record.offset, record.key, first)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id)
		SELECT $1,course_id,absence_hours,period_text,classroom_id FROM attendance_details WHERE record_id=$2`, record.id, first)
		if err != nil {
			t.Fatal(err)
		}
	}
	// These records represent entries created while the week was configured at 1x.
	// Later calendar edits must not rewrite their submitted penalty snapshots.
	if _, err = pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=1 WHERE id=$1`, "a0000000-0000-0000-0000-000000000004"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE attendance_details SET penalty_multiplier=1 WHERE record_id=ANY($1::uuid[])`, []string{first, second, third}); err != nil {
		t.Fatal(err)
	}

	permissions := &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter,
		Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: department, InspectionTypeID: typeID, ClassID: class}}}}
	service := workflow.NewService(&Postgres{Pool: pool}, permissions)
	scope := workflow.Scope{SemesterID: "a0000000-0000-0000-0000-000000000003", WeekID: "a0000000-0000-0000-0000-000000000004", DepartmentID: department, InspectionTypeID: typeID}
	input := workflow.SubmitInput{Scope: scope, Records: []workflow.VersionedRecord{{ID: first, Version: 1}, {ID: second, Version: 1}}, IdempotencyKey: "member-batch-1"}
	actor := workflow.Actor{ID: account}

	input.Records[1].Version = 2
	if _, err := service.Submit(ctx, actor, input, audit.Metadata{}); !errors.Is(err, workflow.ErrVersionConflict) {
		t.Fatalf("stale second record: %v", err)
	}
	assertWorkflowState(t, pool, first, "DRAFT", false, 1)
	assertWorkflowState(t, pool, second, "DRAFT", false, 1)
	input.Records[1].Version = 1
	permissions.rules[0].Scope.ClassID = "a0000000-0000-0000-0000-000000000016"
	if _, err := service.Submit(ctx, actor, input, audit.Metadata{}); !errors.Is(err, workflow.ErrRecordUnavailable) {
		t.Fatalf("outside class scope: %v", err)
	}
	permissions.rules[0].Scope.ClassID = class
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_workflow_audit() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF NEW.action='DEPARTMENT_SUBMITTED' THEN RAISE EXCEPTION 'isolated workflow audit fault'; END IF;
 RETURN NEW; END $$;
 CREATE TRIGGER reject_workflow_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_workflow_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Submit(ctx, actor, input, audit.Metadata{}); err == nil {
		t.Fatal("audit failure committed a submission")
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER reject_workflow_audit ON audit_logs; DROP FUNCTION reject_workflow_audit()`); err != nil {
		t.Fatal(err)
	}
	assertWorkflowState(t, pool, first, "DRAFT", false, 1)
	assertWorkflowState(t, pool, second, "DRAFT", false, 1)
	var failedBatchCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM department_submissions`).Scan(&failedBatchCount); err != nil || failedBatchCount != 0 {
		t.Fatalf("partial submission after audit failure: count=%d err=%v", failedBatchCount, err)
	}
	created, err := service.Submit(ctx, actor, input, audit.Metadata{})
	if err != nil || created.RevisionNo != 1 || created.Status != workflow.PendingReview ||
		len(created.RecordIDs) != 2 || len(created.Records) != 2 || created.WeekNo != 1 || created.DepartmentName == "" {
		t.Fatalf("submit: %+v %v", created, err)
	}
	assertWorkflowState(t, pool, first, "PENDING_REVIEW", true, 2)
	assertWorkflowState(t, pool, second, "PENDING_REVIEW", true, 2)
	listed, err := service.List(ctx, actor, workflow.ListQuery{SemesterID: scope.SemesterID, WeekID: scope.WeekID, Page: 1, PageSize: 20})
	if err != nil || listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != created.ID || len(listed.Items[0].RecordIDs) != 2 {
		t.Fatalf("scoped batch list: %+v %v", listed, err)
	}
	if _, err := service.Detail(ctx, actor, created.ID); err != nil {
		t.Fatalf("scoped batch detail: %v", err)
	}
	permissions.rules = nil
	if _, err := service.Submit(ctx, actor, input, audit.Metadata{}); !errors.Is(err, workflow.ErrRecordUnavailable) {
		t.Fatalf("revoked member replayed submitted batch: %v", err)
	}
	listed, err = service.List(ctx, actor, workflow.ListQuery{})
	if err != nil || listed.Total != 0 {
		t.Fatalf("ungranted batch list: %+v %v", listed, err)
	}
	if _, err := service.Detail(ctx, actor, created.ID); !errors.Is(err, workflow.ErrSubmissionMissing) {
		t.Fatalf("ungranted batch detail: %v", err)
	}
	permissions.rules = []permission.EffectivePermission{{Action: permission.ActionInspectionEnter,
		Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: department, InspectionTypeID: typeID, ClassID: class}}}
	var revisions, audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM department_submission_revisions WHERE submission_id=$1", created.ID).Scan(&revisions); err != nil || revisions != 1 {
		t.Fatalf("revision count %d: %v", revisions, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE department_submission_revisions SET records_snapshot='[]'::jsonb WHERE submission_id=$1`, created.ID); err == nil {
		t.Fatal("historical submission snapshot was mutable")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM department_submission_revisions WHERE submission_id=$1`, created.ID); err == nil {
		t.Fatal("historical submission revision was deletable")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='DEPARTMENT_SUBMITTED'", created.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count %d: %v", audits, err)
	}
	replayed, err := service.Submit(ctx, actor, input, audit.Metadata{})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("idempotent replay: %+v %v", replayed, err)
	}
	input.Records = []workflow.VersionedRecord{{ID: third, Version: 1}}
	if _, err := service.Submit(ctx, actor, input, audit.Metadata{}); !errors.Is(err, workflow.ErrIdempotencyReuse) {
		t.Fatalf("reused key for another record: %v", err)
	}
	input.IdempotencyKey = "member-batch-2"
	other, err := service.Submit(ctx, actor, input, audit.Metadata{})
	if err != nil || other.ID == created.ID {
		t.Fatalf("second batch in same scope: %+v %v", other, err)
	}
	batchCounts, err := (&Postgres{Pool: pool}).Aggregate(ctx, dashboard.AggregateQuery{SemesterID: scope.SemesterID,
		WeekID: scope.WeekID, Scopes: []dashboard.Scope{{DepartmentID: department, InspectionTypeID: typeID}}})
	if err != nil || len(batchCounts) != 1 || batchCounts[0].BatchTotal != 2 || batchCounts[0].BatchPending != 2 || batchCounts[0].Total != 3 {
		t.Fatalf("weekly batch and record aggregation: %+v %v", batchCounts, err)
	}
	if _, err := service.Decide(ctx, actor, created.ID, workflow.Return, workflow.DecisionInput{Version: 1, Reason: "请核对"}, audit.Metadata{}); !errors.Is(err, workflow.ErrSubmissionScope) {
		t.Fatalf("member reviewed without grant: %v", err)
	}
	reviewPermissions := &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionReview,
		Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: department, InspectionTypeID: typeID, ClassID: class}}}}
	reviewService := workflow.NewService(&Postgres{Pool: pool}, reviewPermissions)
	reviewActor := workflow.Actor{ID: reviewer}
	if _, err := reviewService.Decide(ctx, reviewActor, created.ID, workflow.Return, workflow.DecisionInput{Version: 2, Reason: "请核对"}, audit.Metadata{}); !errors.Is(err, workflow.ErrVersionConflict) {
		t.Fatalf("stale decision: %v", err)
	}
	returned, err := reviewService.Decide(ctx, reviewActor, created.ID, workflow.Return, workflow.DecisionInput{Version: 1, Reason: "请核对"}, audit.Metadata{})
	if err != nil || returned.Status != workflow.Returned || returned.RowVersion != 2 || len(returned.Records) != 2 || returned.Records[0].Status != "RETURNED" {
		t.Fatalf("return batch: %+v %v", returned, err)
	}
	assertWorkflowState(t, pool, first, "RETURNED", true, 3)
	assertWorkflowState(t, pool, second, "RETURNED", true, 3)
	if _, err := pool.Exec(ctx, `UPDATE semester_weeks SET attendance_penalty_multiplier=2 WHERE id=$1`, scope.WeekID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO course_classes(course_id,class_id)
 VALUES('a0000000-0000-0000-0000-000000000013',$1) ON CONFLICT DO NOTHING`, class); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section)
 VALUES(gen_random_uuid(),'a0000000-0000-0000-0000-000000000013',$1,
 'a0000000-0000-0000-0000-000000000009',3,1,2)`, scope.SemesterID); err != nil {
		t.Fatal(err)
	}
	attendanceAccess := inspection.MutationAccess{ActorID: account, Permissions: permissions.rules}
	attendanceStore := &Postgres{Pool: pool}
	attendanceRecord, err := attendanceStore.ReadAttendanceForMutation(ctx, second, attendanceAccess)
	if err != nil {
		t.Fatalf("read returned attendance: %v", err)
	}
	attendanceRecord.Remark = "second corrected"
	updatedAttendance, err := attendanceStore.UpdateAttendance(ctx, attendanceRecord, 3, attendanceAccess,
		audit.Mutation{Metadata: audit.Metadata{ActorID: account}, Entry: audit.Entry{Action: audit.ActionInspectionRecordUpdated, ResourceType: "inspectionRecord", ResourceID: second}})
	if err != nil || updatedAttendance.RowVersion != 4 {
		t.Fatalf("correct returned attendance: %+v %v", updatedAttendance, err)
	}
	if updatedAttendance.PenaltyMultiplier == nil || *updatedAttendance.PenaltyMultiplier != 1 || updatedAttendance.PenaltyHours == nil || *updatedAttendance.PenaltyHours != updatedAttendance.AbsenceHours {
		t.Fatalf("legacy correction rewrote penalty snapshot: %+v", updatedAttendance)
	}
	if _, err := reviewService.Decide(ctx, reviewActor, created.ID, workflow.Confirm, workflow.DecisionInput{Version: 2}, audit.Metadata{}); !errors.Is(err, workflow.ErrInvalidTransition) {
		t.Fatalf("confirmed returned batch: %v", err)
	}
	if _, err := reviewService.Resubmit(ctx, reviewActor, created.ID, workflow.ResubmitInput{Version: 2,
		Records: []workflow.VersionedRecord{{ID: first, Version: 3}, {ID: second, Version: 3}}}, audit.Metadata{}); !errors.Is(err, workflow.ErrSubmissionScope) {
		t.Fatalf("reviewer resubmitted without entry grant: %v", err)
	}
	details, _ := json.Marshal(map[string]any{"studentId": "a0000000-0000-0000-0000-000000000007",
		"classId": class, "courseId": "a0000000-0000-0000-0000-000000000013",
		"classroomId": "a0000000-0000-0000-0000-000000000009", "date": "2026-09-02",
		"time": "08:00", "period": "1-2", "absenceHours": 2, "remark": "corrected"})
	inspectionService := inspection.NewRecordService(&Postgres{Pool: pool}, permissions)
	if _, err := pool.Exec(ctx, `UPDATE attendance_details SET penalty_multiplier=NULL WHERE record_id=$1`, first); err != nil {
		t.Fatal(err)
	}
	_, err = inspectionService.Update(ctx, inspection.ManualActor{ID: account}, first, inspection.InspectionEdit{
		Version: 3, Reason: "核对后更正", Record: inspection.InspectionInput{SemesterID: scope.SemesterID,
			WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: department, Details: details},
	}, audit.Metadata{})
	if !errors.Is(err, inspection.ErrAttendanceLegacyRuleUnknown) {
		t.Fatalf("historical unknown penalty rule was silently guessed: %v", err)
	}
	assertWorkflowState(t, pool, first, "RETURNED", true, 3)
	if _, err := pool.Exec(ctx, `UPDATE attendance_details SET penalty_multiplier=1 WHERE record_id=$1`, first); err != nil {
		t.Fatal(err)
	}
	corrected, err := inspectionService.Update(ctx, inspection.ManualActor{ID: account}, first, inspection.InspectionEdit{
		Version: 3, Reason: "核对后更正", Record: inspection.InspectionInput{SemesterID: scope.SemesterID,
			WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: department, Details: details},
	}, audit.Metadata{})
	if err != nil || corrected.Status != "RETURNED" || corrected.RowVersion != 4 {
		t.Fatalf("correct returned record: %+v %v", corrected, err)
	}
	if detail, ok := corrected.Details.(*inspection.AttendanceDetails); !ok || detail.PenaltyMultiplier == nil || *detail.PenaltyMultiplier != 1 || detail.PenaltyHours == nil || *detail.PenaltyHours != detail.AbsenceHours {
		t.Fatalf("generic correction rewrote penalty snapshot: %+v", corrected.Details)
	}
	resubmit := workflow.ResubmitInput{Version: 2, Records: []workflow.VersionedRecord{{ID: first, Version: 4}, {ID: second, Version: 4}}}
	resubmit.Records[1].Version = 2
	if _, err := service.Resubmit(ctx, actor, created.ID, resubmit, audit.Metadata{}); !errors.Is(err, workflow.ErrVersionConflict) {
		t.Fatalf("stale record in resubmit: %v", err)
	}
	assertWorkflowState(t, pool, first, "RETURNED", true, 4)
	resubmit.Records[1].Version = 4
	resubmitted, err := service.Resubmit(ctx, actor, created.ID, resubmit, audit.Metadata{})
	if err != nil || resubmitted.ID != created.ID || resubmitted.RevisionNo != 2 || resubmitted.RowVersion != 3 || len(resubmitted.Records) != 2 {
		t.Fatalf("resubmit: %+v %v", resubmitted, err)
	}
	assertWorkflowState(t, pool, first, "PENDING_REVIEW", true, 5)
	assertWorkflowState(t, pool, second, "PENDING_REVIEW", true, 5)
	var oldRemark, newRemark string
	if err := pool.QueryRow(ctx, `SELECT records_snapshot->0->'record'->>'remark' FROM department_submission_revisions
 WHERE submission_id=$1 AND revision_no=1`, created.ID).Scan(&oldRemark); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT records_snapshot->0->'record'->>'remark' FROM department_submission_revisions
 WHERE submission_id=$1 AND revision_no=2`, created.ID).Scan(&newRemark); err != nil || oldRemark == newRemark || newRemark != "corrected" {
		t.Fatalf("revision snapshots: old=%q new=%q err=%v", oldRemark, newRemark, err)
	}
	if _, err := inspectionService.Update(ctx, inspection.ManualActor{ID: account}, first, inspection.InspectionEdit{
		Version: 5, Reason: "不允许待审核修改", Record: inspection.InspectionInput{SemesterID: scope.SemesterID,
			WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID, DepartmentID: department, Details: details},
	}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotEditable) {
		t.Fatalf("pending record editable: %v", err)
	}
	confirmed, err := reviewService.Decide(ctx, reviewActor, other.ID, workflow.Confirm, workflow.DecisionInput{Version: 1}, audit.Metadata{})
	if err != nil || confirmed.Status != workflow.Confirmed || confirmed.RowVersion != 2 {
		t.Fatalf("confirm batch: %+v %v", confirmed, err)
	}
	assertWorkflowState(t, pool, third, "CONFIRMED", true, 3)
	batchCounts, err = (&Postgres{Pool: pool}).Aggregate(ctx, dashboard.AggregateQuery{SemesterID: scope.SemesterID,
		WeekID: scope.WeekID, Scopes: []dashboard.Scope{{DepartmentID: department, InspectionTypeID: typeID}}})
	if err != nil || len(batchCounts) != 1 || batchCounts[0].BatchTotal != 2 || batchCounts[0].BatchPending != 1 || batchCounts[0].BatchConfirmed != 1 {
		t.Fatalf("weekly batch state after confirmation: %+v %v", batchCounts, err)
	}
	for index, item := range []struct {
		code, detailSQL, snapshotKey string
	}{
		{"UNATTENDED_CLASSROOM", `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id)
 VALUES($1,'a0000000-0000-0000-0000-000000000013',2,'1-2','a0000000-0000-0000-0000-000000000009')`, "attendanceDetails"},
		{"MORNING_ATTENDANCE", `INSERT INTO morning_exercise_details(record_id) VALUES($1)`, "morningDetails"},
		{"DORMITORY_HYGIENE", `INSERT INTO dorm_hygiene_details(record_id,dormitory_id,result)
 VALUES($1,'a0000000-0000-0000-0000-000000000012','QUALIFIED')`, "dormitoryHygieneDetails"},
		{"LATE_RETURN", `INSERT INTO late_return_details(record_id,dormitory_id)
 VALUES($1,'a0000000-0000-0000-0000-000000000012')`, "lateReturnDetails"},
	} {
		recordID := fmt.Sprintf("c0000000-0000-0000-0000-%012d", index+1)
		var otherTypeID string
		if err := pool.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code=$1`, item.code).Scan(&otherTypeID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter)
 VALUES($1,$2,true,true) ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true`, department, otherTypeID); err != nil {
			t.Fatal(err)
		}
		var occurredAt any = "2026-09-02 08:00+08"
		var inspectionDate any
		kind, violations := "TIME", 1
		if item.code == "MORNING_ATTENDANCE" {
			occurredAt, inspectionDate, kind = nil, "2026-09-02", "DATE"
		}
		if item.code == "DORMITORY_HYGIENE" {
			violations = 0
		}
		if _, err := pool.Exec(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,
 occurred_at,occurrence_kind,inspection_date,violation_count,source,dedup_key,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'MANUAL',repeat($10,64),$11)`,
			recordID, otherTypeID, scope.SemesterID, scope.WeekID, department, occurredAt, kind,
			inspectionDate, violations, fmt.Sprint(index+3), account); err != nil {
			t.Fatal(err)
		}
		if item.code == "DORMITORY_HYGIENE" {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, item.detailSQL, recordID); err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO inspection_responsibility_classes(record_id,class_id,class_name_snapshot,grade_snapshot)
 VALUES($1,$2,'Fixture class','2026')`, recordID, class)
			}
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
		} else if _, err := pool.Exec(ctx, item.detailSQL, recordID); err != nil {
			t.Fatal(err)
		}
		otherScope := scope
		otherScope.InspectionTypeID = otherTypeID
		batch, err := service.Submit(ctx, workflow.Actor{ID: account, IsSuperAdmin: true}, workflow.SubmitInput{
			Scope: otherScope, Records: []workflow.VersionedRecord{{ID: recordID, Version: 1}},
			IdempotencyKey: "type-" + item.code}, audit.Metadata{})
		if err != nil || len(batch.Records) != 1 || batch.Records[0].Status != "PENDING_REVIEW" {
			t.Fatalf("submit %s: %+v %v", item.code, batch, err)
		}
		var hasDetail bool
		if err := pool.QueryRow(ctx, `SELECT records_snapshot->0->$2::text IS NOT NULL
 FROM department_submission_revisions WHERE submission_id=$1 AND revision_no=1`, batch.ID, item.snapshotKey).Scan(&hasDetail); err != nil || !hasDetail {
			t.Fatalf("snapshot %s detail: %v %v", item.code, hasDetail, err)
		}
	}
	const concurrentRecord = "d0000000-0000-0000-0000-000000000001"
	if _, err := pool.Exec(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,
 student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,
 violation_count,remark,source,dedup_key,created_by)
 SELECT $1,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,
 student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at+interval '3 hours',
 violation_count,remark,source,repeat('d',64),created_by FROM inspection_records WHERE id=$2`, concurrentRecord, first); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id,penalty_multiplier)
 SELECT $1,course_id,absence_hours,period_text,classroom_id,penalty_multiplier FROM attendance_details WHERE record_id=$2`, concurrentRecord, first); err != nil {
		t.Fatal(err)
	}
	type submissionResult struct {
		batch workflow.Submission
		err   error
	}
	results := make(chan submissionResult, 2)
	start := make(chan struct{})
	var writers sync.WaitGroup
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		writers.Add(1)
		go func(key string) {
			defer writers.Done()
			<-start
			batch, submitErr := service.Submit(ctx, actor, workflow.SubmitInput{Scope: scope,
				Records: []workflow.VersionedRecord{{ID: concurrentRecord, Version: 1}}, IdempotencyKey: key}, audit.Metadata{})
			results <- submissionResult{batch: batch, err: submitErr}
		}(key)
	}
	close(start)
	writers.Wait()
	close(results)
	succeeded, rejected := 0, 0
	var concurrentBatch workflow.Submission
	for result := range results {
		if result.err == nil {
			succeeded++
			concurrentBatch = result.batch
			if result.batch.Status != workflow.PendingReview {
				t.Fatalf("concurrent successful batch: %+v", result.batch)
			}
		} else if errors.Is(result.err, workflow.ErrRecordUnavailable) || errors.Is(result.err, workflow.ErrVersionConflict) {
			rejected++
		} else {
			t.Fatalf("concurrent submit returned unexpected error: %v", result.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent submit outcomes: succeeded=%d rejected=%d", succeeded, rejected)
	}
	assertWorkflowState(t, pool, concurrentRecord, "PENDING_REVIEW", true, 2)
	var concurrentBatches, concurrentAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM department_submissions WHERE id=(SELECT submission_id FROM inspection_records WHERE id=$1)`, concurrentRecord).Scan(&concurrentBatches); err != nil || concurrentBatches != 1 {
		t.Fatalf("concurrent submission count=%d: %v", concurrentBatches, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='DEPARTMENT_SUBMITTED' AND resource_id=(SELECT submission_id::text FROM inspection_records WHERE id=$1)`, concurrentRecord).Scan(&concurrentAudits); err != nil || concurrentAudits != 1 {
		t.Fatalf("concurrent audit count=%d: %v", concurrentAudits, err)
	}
	returnedConcurrent, err := reviewService.Decide(ctx, reviewActor, concurrentBatch.ID, workflow.Return,
		workflow.DecisionInput{Version: 1, Reason: "并发重提核对"}, audit.Metadata{})
	if err != nil || returnedConcurrent.RowVersion != 2 || len(returnedConcurrent.Records) != 1 {
		t.Fatalf("prepare concurrent resubmit: %+v %v", returnedConcurrent, err)
	}
	results = make(chan submissionResult, 2)
	start = make(chan struct{})
	for range 2 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			batch, resubmitErr := service.Resubmit(ctx, actor, concurrentBatch.ID, workflow.ResubmitInput{
				Version: returnedConcurrent.RowVersion,
				Records: []workflow.VersionedRecord{{ID: concurrentRecord, Version: returnedConcurrent.Records[0].RowVersion}},
			}, audit.Metadata{})
			results <- submissionResult{batch: batch, err: resubmitErr}
		}()
	}
	close(start)
	writers.Wait()
	close(results)
	succeeded, rejected = 0, 0
	for result := range results {
		if result.err == nil {
			succeeded++
			if result.batch.ID != concurrentBatch.ID || result.batch.RevisionNo != 2 {
				t.Fatalf("concurrent resubmit changed batch identity/revision: %+v", result.batch)
			}
		} else if errors.Is(result.err, workflow.ErrVersionConflict) {
			rejected++
		} else {
			t.Fatalf("concurrent resubmit returned unexpected error: %v", result.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent resubmit outcomes: succeeded=%d rejected=%d", succeeded, rejected)
	}
	var concurrentRevisions, concurrentResubmitAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM department_submission_revisions WHERE submission_id=$1`, concurrentBatch.ID).Scan(&concurrentRevisions); err != nil || concurrentRevisions != 2 {
		t.Fatalf("concurrent revision count=%d: %v", concurrentRevisions, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='DEPARTMENT_RESUBMITTED' AND resource_id=$1`, concurrentBatch.ID).Scan(&concurrentResubmitAudits); err != nil || concurrentResubmitAudits != 1 {
		t.Fatalf("concurrent resubmit audit count=%d: %v", concurrentResubmitAudits, err)
	}
	t.Run("independent college batch", func(t *testing.T) {
		const schoolRecord = "a0000000-0000-0000-0000-000000000044"
		if _, err := pool.Exec(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,
 student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,
 violation_count,remark,source,dedup_key,created_by)
 SELECT $1,inspection_type_id,semester_id,week_id,NULL,student_id,student_no_snapshot,
 student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at+interval '4 hours',
 violation_count,remark,source,repeat('s',64),created_by FROM inspection_records WHERE id=$2`, schoolRecord, first); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id,penalty_multiplier)
 SELECT $1,course_id,absence_hours,period_text,classroom_id,penalty_multiplier FROM attendance_details WHERE record_id=$2`, schoolRecord, first); err != nil {
			t.Fatal(err)
		}
		schoolScope := scope
		schoolScope.DepartmentID = ""
		schoolInput := workflow.SubmitInput{Scope: schoolScope, Records: []workflow.VersionedRecord{{ID: schoolRecord, Version: 1}}, IdempotencyKey: "college-batch"}
		departmentOnly := workflow.NewService(&Postgres{Pool: pool}, &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: department, InspectionTypeID: typeID, ClassID: class}}}})
		if _, err := departmentOnly.Submit(ctx, actor, schoolInput, audit.Metadata{}); !errors.Is(err, workflow.ErrRecordUnavailable) {
			t.Fatalf("department-scoped submit reached college record: %v", err)
		}
		schoolSubmit := workflow.NewService(&Postgres{Pool: pool}, &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: typeID, ClassID: class}}}})
		batch, err := schoolSubmit.Submit(ctx, actor, schoolInput, audit.Metadata{})
		if err != nil || batch.Scope.DepartmentID != "" || batch.DepartmentName != "校级" || batch.Status != workflow.PendingReview {
			t.Fatalf("college submit: %+v %v", batch, err)
		}
		if _, err := departmentOnly.Detail(ctx, actor, batch.ID); !errors.Is(err, workflow.ErrSubmissionMissing) {
			t.Fatalf("department-scoped actor read college batch: %v", err)
		}
		schoolReview := workflow.NewService(&Postgres{Pool: pool}, &workflowPermissions{rules: []permission.EffectivePermission{{Action: permission.ActionInspectionReview, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: typeID, ClassID: class}}}})
		confirmed, err := schoolReview.Decide(ctx, workflow.Actor{ID: reviewer}, batch.ID, workflow.Confirm, workflow.DecisionInput{Version: 1}, audit.Metadata{})
		if err != nil || confirmed.Status != workflow.Confirmed || confirmed.Scope.DepartmentID != "" {
			t.Fatalf("college review: %+v %v", confirmed, err)
		}
		assertWorkflowState(t, pool, schoolRecord, "CONFIRMED", true, 3)
		counts, err := (&Postgres{Pool: pool}).Aggregate(ctx, dashboard.AggregateQuery{SemesterID: scope.SemesterID,
			WeekID: scope.WeekID, Scopes: []dashboard.Scope{{DepartmentID: "", InspectionTypeID: typeID}, {DepartmentID: department, InspectionTypeID: typeID}}})
		if err != nil || len(counts) != 2 || counts[0].DepartmentID != "" || counts[0].Total != 1 || counts[0].Confirmed != 1 || counts[0].BatchTotal != 1 || counts[0].BatchConfirmed != 1 {
			t.Fatalf("college weekly aggregation: %+v %v", counts, err)
		}
	})
}

func assertWorkflowState(t *testing.T, pool *pgxpool.Pool, recordID, status string, submitted bool, version int) {
	t.Helper()
	var gotStatus string
	var gotSubmitted bool
	var gotVersion int
	if err := pool.QueryRow(context.Background(), "SELECT status,submission_id IS NOT NULL,row_version FROM inspection_records WHERE id=$1", recordID).
		Scan(&gotStatus, &gotSubmitted, &gotVersion); err != nil || gotStatus != status || gotSubmitted != submitted || gotVersion != version {
		t.Fatalf("record %s: %s submitted=%v version=%d err=%v", recordID, gotStatus, gotSubmitted, gotVersion, err)
	}
}
