package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reportWorkflowPermissionStub struct {
	permissions []permission.EffectivePermission
}

func (p reportWorkflowPermissionStub) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.permissions}, nil
}

func TestWeeklyReportVersionWorkflowPostgres(t *testing.T) {
	databaseURL := os.Getenv("REPORT_WORKFLOW_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REPORT_WORKFLOW_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatal("requires an empty disposable database", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		contents, readErr := os.ReadFile(migration)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := pool.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("migration %s: %v", filepath.Base(migration), execErr)
		}
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	const (
		actorID             = "a0000000-0000-0000-0000-000000000001"
		semesterID          = "a0000000-0000-0000-0000-000000000003"
		weekID              = "a0000000-0000-0000-0000-000000000004"
		recordID            = "a0000000-0000-0000-0000-000000000014"
		eveningID           = "a0000000-0000-0000-0000-000000000015"
		departmentID        = "10000000-0000-0000-0000-000000000002"
		attendanceID        = "30000000-0000-0000-0000-000000000001"
		submissionID        = "b0000000-0000-0000-0000-000000000001"
		eveningSubmissionID = "b0000000-0000-0000-0000-000000000003"
	)
	var eveningTypeID string
	if err := pool.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code='EVENING_SELF_STUDY' AND participates_in_report`).Scan(&eveningTypeID); err != nil {
		t.Fatal("load evening self-study report type:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submissions(
 id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,row_version,created_by,submitted_by,reviewed_by,request_fingerprint,submitted_at,reviewed_at)
VALUES($1,$2,$3,$4,$5,'CONFIRMED',1,2,$6,$6,$6,repeat('f',64),now(),now())`,
		submissionID, semesterID, weekID, departmentID, attendanceID, actorID); err != nil {
		t.Fatal("create confirmed submission:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE inspection_records SET status='CONFIRMED',submission_id=$1,row_version=2 WHERE id=$2`, submissionID, recordID); err != nil {
		t.Fatal("confirm source record:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
SELECT 'c0000000-0000-0000-0000-000000000001',$1,1,$2,ARRAY[$3::uuid],jsonb_build_array(jsonb_build_object(
 'record',to_jsonb(ir),'attendanceDetails',to_jsonb(ad),'eveningSelfStudyDetails',NULL,'morningDetails',NULL,
 'lateReturnDetails',NULL,'dormitoryHygieneDetails',NULL))
FROM inspection_records ir JOIN attendance_details ad ON ad.record_id=ir.id WHERE ir.id=$3`, submissionID, actorID, recordID); err != nil {
		t.Fatal("create confirmed source revision:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter,can_view,can_review)
VALUES($1,$2,true,true,true,true) ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true,can_view=true,can_review=true`, departmentID, eveningTypeID); err != nil {
		t.Fatal("enable evening self-study report cell:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submissions(
 id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,row_version,created_by,submitted_by,reviewed_by,request_fingerprint,submitted_at,reviewed_at)
VALUES($1,$2,$3,$4,$5,'CONFIRMED',1,2,$6,$6,$6,repeat('b',64),now(),now())`, eveningSubmissionID, semesterID, weekID, departmentID, eveningTypeID, actorID); err != nil {
		t.Fatal("create confirmed evening submission:", err)
	}
	eveningTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin evening source fixture:", err)
	}
	if _, err = eveningTx.Exec(ctx, `INSERT INTO inspection_records(
 id,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,student_name_snapshot,class_id,
 class_name_snapshot,grade_snapshot,occurred_at,occurrence_kind,violation_count,remark,source,status,submission_id,dedup_key,row_version,created_by)
SELECT $1,it.id,source.semester_id,source.week_id,source.department_id,source.student_id,source.student_no_snapshot,source.student_name_snapshot,
 source.class_id,source.class_name_snapshot,source.grade_snapshot,'2026-09-02 20:15+08'::timestamptz,'TIME',1,'晚自习缺勤','MANUAL','CONFIRMED',$2,repeat('b',64),2,source.created_by
FROM inspection_records source JOIN inspection_types it ON it.code='EVENING_SELF_STUDY' WHERE source.id=$3`, eveningID, eveningSubmissionID, recordID); err == nil {
		_, err = eveningTx.Exec(ctx, `INSERT INTO evening_self_study_details(record_id,classroom_id,period_text,penalty_multiplier,penalty_count)
VALUES($1,'a0000000-0000-0000-0000-000000000009','9-10',1,1)`, eveningID)
	}
	if err == nil {
		_, err = eveningTx.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,submission_kind,record_ids,records_snapshot)
SELECT 'c0000000-0000-0000-0000-000000000003',$1,1,$2,'RECORDS',ARRAY[$3::uuid],jsonb_build_array(jsonb_build_object(
 'record',to_jsonb(ir),'attendanceDetails',NULL,'eveningSelfStudyDetails',to_jsonb(esd),'morningDetails',NULL,
 'lateReturnDetails',NULL,'dormitoryHygieneDetails',NULL))
FROM inspection_records ir JOIN evening_self_study_details esd ON esd.record_id=ir.id WHERE ir.id=$3`, eveningSubmissionID, actorID, eveningID)
	}
	if err == nil {
		err = eveningTx.Commit(ctx)
	} else {
		_ = eveningTx.Rollback(ctx)
	}
	if err != nil {
		t.Fatal("create confirmed evening source record and revision:", err)
	}
	permissions := []permission.EffectivePermission{
		{Action: permission.ActionReportView, Effect: permission.EffectAllow},
		{Action: permission.ActionReportGenerate, Effect: permission.EffectAllow},
		{Action: permission.ActionReportReview, Effect: permission.EffectAllow},
		{Action: permission.ActionReportReturn, Effect: permission.EffectAllow},
		{Action: permission.ActionReportFinalize, Effect: permission.EffectAllow},
		{Action: permission.ActionReportCorrect, Effect: permission.EffectAllow},
		{Action: permission.ActionInspectionReview, Effect: permission.EffectAllow},
	}
	database := &Postgres{Pool: pool}
	if err := database.Ready(ctx); err != nil {
		t.Fatal("reporting migrations did not satisfy database readiness:", err)
	}
	service := reporting.NewService(database, reportWorkflowPermissionStub{permissions: permissions})
	draft, err := service.GenerateDraft(ctx, actorID, reporting.GenerateDraftInput{SemesterID: semesterID, WeekID: weekID}, audit.Metadata{})
	if err != nil {
		t.Fatal("generate report draft:", err)
	}
	if draft.VersionNo != 1 || draft.RowVersion != 1 || draft.Content.ConfirmedRecordCount != 2 || len(draft.Content.Rows) < 2 {
		t.Fatalf("draft count-only snapshot = %+v", draft)
	}
	var foundAttendance, foundEvening bool
	for _, row := range draft.Content.Rows {
		if row.InspectionTypeID == attendanceID {
			foundAttendance = row.RecordCount == 1 && row.DataStatus == "CONFIRMED_RECORDS"
		} else if row.InspectionTypeID == eveningTypeID {
			foundEvening = row.RecordCount == 1 && row.DataStatus == "CONFIRMED_RECORDS"
		} else if row.RecordCount == 0 && row.DataStatus != "NO_CONFIRMED_DATA" {
			t.Fatalf("empty cell misrepresented as confirmed zero: %+v", row)
		}
	}
	if !foundAttendance || !foundEvening {
		t.Fatalf("confirmed attendance/evening row missing from report snapshot: %+v", draft.Content.Rows)
	}
	history, err := service.Workflow(ctx, actorID, semesterID, weekID)
	if err != nil || history.Report == nil || history.Report.Status != reporting.ReportStatusDraft ||
		history.Report.RowVersion != draft.RowVersion || len(history.Versions) != 1 || history.Versions[0].VersionNo != 1 ||
		history.Versions[0].Content.ConfirmedRecordCount != 2 {
		t.Fatalf("authorized report workflow history = %+v, %v", history, err)
	}
	limitedWorkflowService := reporting.NewService(database, reportWorkflowPermissionStub{permissions: []permission.EffectivePermission{
		{Action: permission.ActionReportView, Effect: permission.EffectAllow, Scope: permission.Scope{InspectionTypeID: attendanceID}},
	}})
	if _, err := limitedWorkflowService.Workflow(ctx, actorID, semesterID, weekID); !errors.Is(err, reporting.ErrForbidden) {
		t.Fatalf("partial-scope viewer read a report version containing out-of-scope cells: %v", err)
	}
	var lockedReport string
	if err := pool.QueryRow(ctx, "SELECT locked_by_report_id::text FROM inspection_records WHERE id=$1", recordID).Scan(&lockedReport); err != nil || lockedReport != draft.ID {
		t.Fatalf("source lock = %q, %v; want report %s", lockedReport, err, draft.ID)
	}
	var refs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM weekly_report_version_records WHERE report_version_id=(SELECT id FROM weekly_report_versions WHERE report_id=$1 AND version_no=1)", draft.ID).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("version source references = %d, %v", refs, err)
	}
	var eveningPeriod, eveningHours, eveningCourse string
	if err := pool.QueryRow(ctx, `SELECT included_data_json->'details'->>'period',included_data_json->'details'->>'penaltyCount',
COALESCE(included_data_json->>'currentCourseName','') FROM weekly_report_version_records
WHERE report_version_id=(SELECT id FROM weekly_report_versions WHERE report_id=$1 AND version_no=1) AND inspection_record_id=$2`, draft.ID, eveningID).Scan(&eveningPeriod, &eveningHours, &eveningCourse); err != nil || eveningPeriod != "9-10" || eveningHours != "1" || eveningCourse != "" {
		t.Fatalf("evening report detail snapshot period=%q hours=%q course=%q err=%v", eveningPeriod, eveningHours, eveningCourse, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE inspection_records SET row_version=row_version+1 WHERE id=$1", recordID); err != nil {
		t.Fatal("simulate source record update:", err)
	}
	if _, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionSubmit,
		reporting.ReportTransitionInput{RowVersion: draft.RowVersion}, audit.Metadata{}); !errors.Is(err, reporting.ErrReportSourceChanged) {
		t.Fatalf("changed source snapshot was submitted: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE inspection_records SET row_version=row_version-1 WHERE id=$1", recordID); err != nil {
		t.Fatal("restore source record fixture:", err)
	}
	const receiptID = "b0000000-0000-0000-0000-000000000002"
	if _, err := pool.Exec(ctx, `INSERT INTO department_submissions(
 id,semester_id,week_id,department_id,inspection_type_id,submission_kind,status,revision_no,row_version,created_by,submitted_by,reviewed_by,request_fingerprint,submitted_at,reviewed_at)
VALUES($1,$2,$3,$4,$5,'ZERO_RECORD','CONFIRMED',1,2,$6,$6,$6,repeat('e',64),now(),now())`, receiptID, semesterID, weekID, departmentID, attendanceID, actorID); err != nil {
		t.Fatal("create confirmed zero-record receipt:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,submission_kind,record_ids,records_snapshot)
VALUES('c0000000-0000-0000-0000-000000000002',$1,1,$2,'ZERO_RECORD','{}'::uuid[],'[]'::jsonb)`, receiptID, actorID); err != nil {
		t.Fatal("create zero-record receipt revision:", err)
	}
	if _, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionSubmit,
		reporting.ReportTransitionInput{RowVersion: draft.RowVersion}, audit.Metadata{}); !errors.Is(err, reporting.ErrReportSourceChanged) {
		t.Fatalf("new zero-record receipt did not stale the report snapshot: %v", err)
	}
	draft, err = service.GenerateDraft(ctx, actorID, reporting.GenerateDraftInput{
		SemesterID: semesterID, WeekID: weekID, ExpectedRowVersion: draft.RowVersion,
	}, audit.Metadata{})
	if err != nil || draft.VersionNo != 2 || draft.RowVersion != 2 {
		t.Fatalf("regenerate after confirmed zero receipt = %+v, %v", draft, err)
	}
	var combinedCell bool
	for _, row := range draft.Content.Rows {
		if row.InspectionTypeID == attendanceID {
			combinedCell = row.RecordCount == 1 && row.ZeroReceiptCount == 1 && row.DataStatus == "CONFIRMED_RECORDS_AND_ZERO_RECEIPT"
		}
	}
	if !combinedCell {
		t.Fatalf("record and zero receipt were not shown as distinct completion evidence: %+v", draft.Content.Rows)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weekly_report_version_zero_receipts
WHERE report_version_id=(SELECT id FROM weekly_report_versions WHERE report_id=$1 AND version_no=2)
 AND submission_id=$2 AND source_revision_no=1`, draft.ID, receiptID).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("zero receipt source references = %d, %v", refs, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM weekly_report_version_zero_receipts
WHERE report_version_id=(SELECT id FROM weekly_report_versions WHERE report_id=$1 AND version_no=2) AND submission_id=$2`, draft.ID, receiptID); err == nil {
		t.Fatal("report zero-receipt history allowed deletion")
	}

	// Append a report-only note correction, then require the source department to confirm a fact change.
	_, err = service.AddReportCorrection(ctx, actorID, draft.ID, reporting.AddReportCorrectionInput{
		RowVersion: draft.RowVersion, RecordID: recordID, Field: "details.remark",
		AfterValue: []byte(`"Audit rollback probe"`), Reason: "验证审计失败时回滚", FactChanged: false,
	}, audit.Metadata{IP: "not-an-ip"})
	if err == nil {
		t.Fatal("report correction succeeded despite an invalid audit IP")
	}
	var versionNo, rowVersion, correctionCount int
	if err := pool.QueryRow(ctx, `SELECT current_version,row_version FROM weekly_reports WHERE id=$1`, draft.ID).Scan(&versionNo, &rowVersion); err != nil || versionNo != draft.VersionNo || rowVersion != draft.RowVersion {
		t.Fatalf("failed audit partially advanced report version: version=%d rowVersion=%d err=%v", versionNo, rowVersion, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM weekly_report_corrections WHERE report_id=$1", draft.ID).Scan(&correctionCount); err != nil || correctionCount != 0 {
		t.Fatalf("failed audit left a correction row: count=%d err=%v", correctionCount, err)
	}
	remarkCorrection, err := service.AddReportCorrection(ctx, actorID, draft.ID, reporting.AddReportCorrectionInput{
		RowVersion: draft.RowVersion, RecordID: recordID, Field: "details.remark",
		AfterValue: []byte(`"Corrected report remark"`), Reason: "修正通报备注错字", FactChanged: false,
	}, audit.Metadata{})
	if err != nil || remarkCorrection.Report.VersionNo != 3 || remarkCorrection.Report.RowVersion != 3 {
		t.Fatalf("append report-only correction = %+v, %v", remarkCorrection, err)
	}
	factCorrection, err := service.AddReportCorrection(ctx, actorID, draft.ID, reporting.AddReportCorrectionInput{
		RowVersion: remarkCorrection.Report.RowVersion, RecordID: recordID, Field: "studentName",
		AfterValue: []byte(`"Confirmed student name"`), Reason: "部门核实后的姓名更正", FactChanged: true,
	}, audit.Metadata{})
	if err != nil || factCorrection.Report.VersionNo != 4 || factCorrection.Report.RowVersion != 4 {
		t.Fatalf("append fact correction = %+v, %v", factCorrection, err)
	}
	if _, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionSubmit,
		reporting.ReportTransitionInput{RowVersion: factCorrection.Report.RowVersion}, audit.Metadata{}); !errors.Is(err, reporting.ErrReportReconfirmationPending) {
		t.Fatalf("pending fact correction did not block report submission: %v", err)
	}
	confirmed, err := service.DecideReportCorrection(ctx, actorID, factCorrection.CorrectionID,
		reporting.DecideReportCorrectionInput{RowVersion: factCorrection.Report.RowVersion, Decision: "CONFIRMED"}, audit.Metadata{})
	if err != nil || confirmed.VersionNo != 5 || confirmed.RowVersion != 5 {
		t.Fatalf("department confirmation of fact correction = %+v, %v", confirmed, err)
	}
	var correctedName, sourceName string
	if err := pool.QueryRow(ctx, `SELECT included_data_json->>'studentName' FROM weekly_report_version_records
WHERE report_version_id=(SELECT id FROM weekly_report_versions WHERE report_id=$1 AND version_no=5) AND inspection_record_id=$2`, draft.ID, recordID).Scan(&correctedName); err != nil || correctedName != "Confirmed student name" {
		t.Fatalf("confirmed report snapshot name = %q, %v", correctedName, err)
	}
	versionRecords, err := service.ReportVersionRecords(ctx, actorID, semesterID, weekID, 5, reporting.ReportVersionRecordsQuery{Page: 1, PageSize: 10})
	if err != nil || versionRecords.Total != 2 || len(versionRecords.Items) != 2 ||
		versionRecords.Items[0].ID != recordID || versionRecords.Items[0].StudentName != "Confirmed student name" {
		t.Fatalf("read corrected immutable version records = %+v, %v", versionRecords, err)
	}
	if err := pool.QueryRow(ctx, `SELECT student_name_snapshot FROM inspection_records WHERE id=$1`, recordID).Scan(&sourceName); err != nil || sourceName == correctedName {
		t.Fatalf("department source revision was mutated by report correction: source=%q report=%q err=%v", sourceName, correctedName, err)
	}

	archivePermissions := append(append([]permission.EffectivePermission(nil), permissions...), permission.EffectivePermission{
		Action: permission.ActionArchiveView, Effect: permission.EffectAllow,
	})
	archiveService := reporting.NewService(database, reportWorkflowPermissionStub{permissions: archivePermissions})
	draftArchive, err := archiveService.Archive(ctx, actorID, reporting.ReportArchiveQuery{Page: 1, PageSize: 10})
	if err != nil || draftArchive.Total != 1 || len(draftArchive.Items) != 1 ||
		draftArchive.Items[0].ID != draft.ID || draftArchive.Items[0].Status != reporting.ReportStatusDraft {
		t.Fatalf("generated draft missing from archive = %+v, %v", draftArchive, err)
	}
	draftArchiveWorkflow, err := archiveService.ArchiveWorkflow(ctx, actorID, semesterID, weekID)
	if err != nil || draftArchiveWorkflow.Report == nil || draftArchiveWorkflow.Report.Status != reporting.ReportStatusDraft {
		t.Fatalf("generated draft archive detail = %+v, %v", draftArchiveWorkflow, err)
	}

	// The creator may review and approve their own report when the action grant allows it.
	pending, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionSubmit,
		reporting.ReportTransitionInput{RowVersion: confirmed.RowVersion}, audit.Metadata{})
	if err != nil || pending.Status != reporting.ReportStatusPending || pending.RowVersion != 6 {
		t.Fatalf("submit report = %+v, %v", pending, err)
	}
	if _, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionApprove,
		reporting.ReportTransitionInput{RowVersion: confirmed.RowVersion}, audit.Metadata{}); !errors.Is(err, reporting.ErrReportVersionConflict) {
		t.Fatalf("stale approval error = %v", err)
	}
	finalizing, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionApprove,
		reporting.ReportTransitionInput{RowVersion: pending.RowVersion}, audit.Metadata{})
	if err != nil || finalizing.Status != reporting.ReportStatusFinalizing || finalizing.RowVersion != 7 {
		t.Fatalf("approve report = %+v, %v", finalizing, err)
	}
	finalized, err := service.TransitionReport(ctx, actorID, draft.ID, reporting.ReportActionFinalized,
		reporting.ReportTransitionInput{RowVersion: finalizing.RowVersion}, audit.Metadata{})
	if err != nil || finalized.Status != reporting.ReportStatusFinalized || finalized.RowVersion != 8 {
		t.Fatalf("finalize structured data without output files = %+v, %v", finalized, err)
	}
	var finalizer string
	if err := pool.QueryRow(ctx, "SELECT finalized_by::text FROM weekly_reports WHERE id=$1", draft.ID).Scan(&finalizer); err != nil || finalizer != actorID {
		t.Fatalf("finalizer metadata = %q, %v", finalizer, err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE resource_type='weeklyReport' AND resource_id=$1", draft.ID).Scan(&auditCount); err != nil || auditCount != 8 {
		t.Fatalf("weekly report audit entries = %d, %v; want two drafts, corrections, department confirmation, submit, approve and finalize", auditCount, err)
	}

	archive, err := archiveService.Archive(ctx, actorID, reporting.ReportArchiveQuery{Page: 1, PageSize: 10, Status: reporting.ReportStatusFinalized})
	if err != nil || archive.Total != 1 || len(archive.Items) != 1 || archive.Items[0].ID != draft.ID ||
		archive.Items[0].Status != reporting.ReportStatusFinalized || archive.Items[0].VersionNo != 5 ||
		archive.Items[0].VersionCount != 5 || archive.Items[0].ConfirmedRecordCount != 2 || archive.Items[0].CellCount != len(draft.Content.Rows) {
		t.Fatalf("finalized report archive page = %+v, %v", archive, err)
	}
	archiveWorkflow, err := archiveService.ArchiveWorkflow(ctx, actorID, semesterID, weekID)
	if err != nil || archiveWorkflow.Report == nil || archiveWorkflow.Report.Status != reporting.ReportStatusFinalized ||
		len(archiveWorkflow.Versions) != 5 || len(archiveWorkflow.Reviews) < 3 {
		t.Fatalf("archive version history = %+v, %v", archiveWorkflow, err)
	}
	partialArchiveService := reporting.NewService(database, reportWorkflowPermissionStub{permissions: []permission.EffectivePermission{{
		Action: permission.ActionArchiveView, Effect: permission.EffectAllow,
		Scope: permission.Scope{InspectionTypeID: attendanceID},
	}}})
	partialArchive, err := partialArchiveService.Archive(ctx, actorID, reporting.ReportArchiveQuery{Page: 1, PageSize: 10})
	if err != nil || partialArchive.Total != 0 || len(partialArchive.Items) != 0 {
		t.Fatalf("archive list exposed a report outside its full authorized scope: %+v, %v", partialArchive, err)
	}
	if _, err := partialArchiveService.ArchiveWorkflow(ctx, actorID, semesterID, weekID); !errors.Is(err, reporting.ErrForbidden) {
		t.Fatalf("partial-scope archive viewer read a report with an out-of-scope cell: %v", err)
	}
	archiveCorrection, err := service.AddReportCorrection(ctx, actorID, draft.ID, reporting.AddReportCorrectionInput{
		RowVersion: finalized.RowVersion, RecordID: recordID, Field: "details.remark",
		AfterValue: []byte(`"Corrected after finalization"`), Reason: "定稿后发现备注需要更正", FactChanged: false,
	}, audit.Metadata{})
	if err != nil || archiveCorrection.Report.Status != reporting.ReportStatusDraft || archiveCorrection.Report.VersionNo != 6 {
		t.Fatalf("create append-only correction from finalized archive = %+v, %v", archiveCorrection, err)
	}
	excluded, err := service.AddReportAdjustment(ctx, actorID, draft.ID, reporting.AddReportAdjustmentInput{
		RowVersion: archiveCorrection.Report.RowVersion, RecordID: recordID, Operation: "EXCLUDE", Reason: "该记录不属于本周通报",
	}, audit.Metadata{})
	if err != nil || excluded.Report.VersionNo != 7 || excluded.Report.Status != reporting.ReportStatusDraft {
		t.Fatalf("exclude record from a new report version = %+v, %v", excluded, err)
	}
	var excludedCount int
	if err := pool.QueryRow(ctx, `SELECT confirmed_record_count FROM (
 SELECT (content_json->>'confirmedRecordCount')::int AS confirmed_record_count FROM weekly_report_versions
 WHERE report_id=$1 AND version_no=7) version`, draft.ID).Scan(&excludedCount); err != nil || excludedCount != 1 {
		t.Fatalf("excluded report snapshot count = %d, %v", excludedCount, err)
	}
	included, err := service.AddReportAdjustment(ctx, actorID, draft.ID, reporting.AddReportAdjustmentInput{
		RowVersion: excluded.Report.RowVersion, RecordID: recordID, Operation: "INCLUDE", Reason: "复核后恢复纳入本周通报",
	}, audit.Metadata{})
	if err != nil || included.Report.VersionNo != 8 {
		t.Fatalf("restore previously excluded confirmed record = %+v, %v", included, err)
	}
	restoredRecords, err := service.ReportVersionRecords(ctx, actorID, semesterID, weekID, 8, reporting.ReportVersionRecordsQuery{
		InspectionTypeID: attendanceID, DepartmentID: departmentID, Page: 1, PageSize: 10,
	})
	if err != nil || restoredRecords.Total != 1 || len(restoredRecords.Items) != 1 || restoredRecords.Items[0].ID != recordID {
		t.Fatalf("restored version cell records = %+v, %v", restoredRecords, err)
	}

	// An explicit obligation without a confirmed record batch or zero receipt blocks generation.
	const nextWeekID = "a0000000-0000-0000-0000-000000000024"
	if _, err := pool.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date)
VALUES($1,$2,2,'2026-09-08','2026-09-14')`, nextWeekID, semesterID); err != nil {
		t.Fatal("create second week:", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weekly_report_obligations(id,semester_id,week_id,department_id,inspection_type_id,created_by,updated_by)
VALUES('d0000000-0000-0000-0000-000000000001',$1,$2,$3,$4,$5,$5)`, semesterID, nextWeekID, departmentID, attendanceID, actorID); err != nil {
		t.Fatal("create required cell:", err)
	}
	if _, err := service.GenerateDraft(ctx, actorID, reporting.GenerateDraftInput{SemesterID: semesterID, WeekID: nextWeekID}, audit.Metadata{}); !errors.Is(err, reporting.ErrReportIncomplete) {
		t.Fatalf("uncompleted required cell generation error = %v", err)
	}
}
