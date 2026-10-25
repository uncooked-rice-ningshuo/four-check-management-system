package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/workflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SubmitDepartmentBatch creates one member batch and its first immutable
// revision. All source records, submission state and audit commit together.
func (p *Postgres) SubmitDepartmentBatch(ctx context.Context, command workflow.SubmitCommand) (workflow.Submission, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return workflow.Submission{}, err
	}
	defer tx.Rollback(ctx)

	var typeCode, academicYearID string
	if command.Scope.DepartmentID == "" {
		err = tx.QueryRow(ctx, `SELECT it.code,y.id FROM semester_weeks w
 JOIN semesters sem ON sem.id=w.semester_id
 JOIN academic_years y ON y.id=sem.academic_year_id
 JOIN inspection_types it ON it.id=$3
 WHERE sem.id=$1 AND w.id=$2 AND sem.enabled AND w.enabled AND y.enabled AND it.enabled
		FOR SHARE OF w,sem,y,it`, command.Scope.SemesterID, command.Scope.WeekID, command.Scope.InspectionTypeID).Scan(&typeCode, &academicYearID)
	} else {
		err = tx.QueryRow(ctx, `SELECT it.code,y.id FROM semester_weeks w
 JOIN semesters sem ON sem.id=w.semester_id
 JOIN academic_years y ON y.id=sem.academic_year_id
 JOIN departments d ON d.id=$3
 JOIN inspection_types it ON it.id=$4
 JOIN department_inspection_scopes ds ON ds.department_id=d.id AND ds.inspection_type_id=it.id
 WHERE sem.id=$1 AND w.id=$2 AND sem.enabled AND w.enabled AND y.enabled AND d.enabled AND it.enabled
   AND ds.enabled AND ds.can_enter FOR SHARE OF w,sem,y,d,it,ds`, command.Scope.SemesterID, command.Scope.WeekID,
			command.Scope.DepartmentID, command.Scope.InspectionTypeID).Scan(&typeCode, &academicYearID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrSubmissionScope
	}
	if err != nil {
		return workflow.Submission{}, err
	}
	if !activeSubmissionType(typeCode) {
		return workflow.Submission{}, workflow.ErrSubmissionScope
	}
	access := inspection.MutationAccess{ActorID: command.ActorID, Unrestricted: command.Unrestricted, Permissions: command.Permissions}
	if command.Kind == workflow.SubmissionKindZeroRecord {
		if err := requireZeroReceiptCell(ctx, tx, command.Scope, academicYearID, access, permission.ActionInspectionEnter); err != nil {
			return workflow.Submission{}, err
		}
	}

	id := uuid.NewString()
	result := workflow.Submission{ID: id, Scope: command.Scope, Status: workflow.PendingReview,
		Kind: command.Kind, RevisionNo: 1, RowVersion: 1, CreatedBy: command.ActorID, SubmittedBy: command.ActorID}
	err = tx.QueryRow(ctx, `INSERT INTO department_submissions(
 id,semester_id,week_id,department_id,inspection_type_id,submission_kind,status,revision_no,row_version,
 created_by,submitted_by,idempotency_key,request_fingerprint,submitted_at)
 VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,'PENDING_REVIEW',1,1,$7,$7,$8,$9,now())
 ON CONFLICT (created_by,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
 RETURNING created_at,submitted_at`, id, command.Scope.SemesterID, command.Scope.WeekID,
		command.Scope.DepartmentID, command.Scope.InspectionTypeID, command.Kind, command.ActorID,
		command.IdempotencyKey, command.Fingerprint).Scan(&result.CreatedAt, &result.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var fingerprint string
		err = tx.QueryRow(ctx, `SELECT id,semester_id,week_id,COALESCE(department_id::text,''),inspection_type_id,submission_kind,status,
 revision_no,row_version,created_by,submitted_by,created_at,submitted_at,request_fingerprint
 FROM department_submissions WHERE created_by=$1 AND idempotency_key=$2`, command.ActorID, command.IdempotencyKey).
			Scan(&result.ID, &result.Scope.SemesterID, &result.Scope.WeekID, &result.Scope.DepartmentID,
				&result.Scope.InspectionTypeID, &result.Kind, &result.Status, &result.RevisionNo, &result.RowVersion,
				&result.CreatedBy, &result.SubmittedBy, &result.CreatedAt, &result.SubmittedAt, &fingerprint)
		if err != nil {
			return workflow.Submission{}, err
		}
		if fingerprint != command.Fingerprint {
			return workflow.Submission{}, workflow.ErrIdempotencyReuse
		}
		if result.Kind != command.Kind {
			return workflow.Submission{}, workflow.ErrIdempotencyReuse
		}
		if command.Kind == workflow.SubmissionKindZeroRecord {
			if err := requireZeroReceiptCell(ctx, tx, command.Scope, academicYearID, access, permission.ActionInspectionEnter); err != nil {
				return workflow.Submission{}, err
			}
		}
		// A replay must still satisfy current per-record permissions. Otherwise a
		// revoked member could use an old key to read a batch's current state.
		for _, requested := range command.Records {
			args := []any{requested.ID, result.ID}
			guard := inspectionAccessSQL(access, "mutate", &args)
			var allowed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inspection_records ir
 JOIN semesters sem ON sem.id=ir.semester_id WHERE ir.id=$1 AND ir.submission_id=$2
 AND ir.deleted_at IS NULL AND `+guard+`)`, args...).Scan(&allowed); err != nil {
				return workflow.Submission{}, err
			}
			if !allowed {
				return workflow.Submission{}, workflow.ErrRecordUnavailable
			}
		}
		return commitDepartmentBatch(ctx, tx, result.ID)
	}
	if err != nil {
		return workflow.Submission{}, err
	}

	recordIDs := make([]string, 0, len(command.Records))
	snapshots := make([]json.RawMessage, 0, len(command.Records))
	for _, requested := range command.Records {
		args := []any{requested.ID, command.Scope.SemesterID, command.Scope.WeekID, command.Scope.DepartmentID, command.Scope.InspectionTypeID}
		guard := inspectionAccessSQL(access, "mutate", &args)
		var version int
		var snapshot json.RawMessage
		err = tx.QueryRow(ctx, `SELECT ir.row_version,
 jsonb_build_object('record',to_jsonb(ir),'attendanceDetails',to_jsonb(ad),
 'morningDetails',to_jsonb(md),'lateReturnDetails',to_jsonb(ld),'dormitoryHygieneDetails',to_jsonb(dh))
 FROM inspection_records ir
 JOIN semesters sem ON sem.id=ir.semester_id
 LEFT JOIN attendance_details ad ON ad.record_id=ir.id
 LEFT JOIN morning_exercise_details md ON md.record_id=ir.id
 LEFT JOIN late_return_details ld ON ld.record_id=ir.id
 LEFT JOIN dorm_hygiene_details dh ON dh.record_id=ir.id
 WHERE ir.id=$1 AND ir.semester_id=$2 AND ir.week_id=$3 AND ir.department_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid AND ir.inspection_type_id=$5
   AND ir.status='DRAFT' AND ir.submission_id IS NULL AND ir.locked_by_report_id IS NULL
   AND ir.deleted_at IS NULL AND `+guard+` FOR UPDATE OF ir`, args...).Scan(&version, &snapshot)
		if errors.Is(err, pgx.ErrNoRows) {
			return workflow.Submission{}, workflow.ErrRecordUnavailable
		}
		if err != nil {
			return workflow.Submission{}, err
		}
		if version != requested.Version {
			return workflow.Submission{}, workflow.ErrVersionConflict
		}
		recordIDs = append(recordIDs, requested.ID)
		snapshots = append(snapshots, snapshot)
	}

	for _, record := range command.Records {
		tag, err := tx.Exec(ctx, `UPDATE inspection_records SET status='PENDING_REVIEW',submission_id=$1,
 row_version=row_version+1,updated_at=now() WHERE id=$2 AND row_version=$3 AND status='DRAFT'
 AND submission_id IS NULL AND locked_by_report_id IS NULL AND deleted_at IS NULL`, id, record.ID, record.Version)
		if err != nil {
			return workflow.Submission{}, err
		}
		if tag.RowsAffected() != 1 {
			return workflow.Submission{}, workflow.ErrVersionConflict
		}
	}
	encodedSnapshots, err := json.Marshal(snapshots)
	if err != nil {
		return workflow.Submission{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO department_submission_revisions(
	 id,submission_id,revision_no,submitted_by,submission_kind,record_ids,records_snapshot)
	VALUES($1,$2,1,$3,$4,$5,$6)`, uuid.NewString(), id, command.ActorID, command.Kind, recordIDs, encodedSnapshots)
	if err != nil {
		return workflow.Submission{}, err
	}
	if err = insertAuditTx(ctx, tx, command.Metadata, audit.Entry{Action: audit.ActionDepartmentSubmitted,
		ResourceType: "departmentSubmission", ResourceID: id, After: map[string]any{
			"scope": command.Scope, "submissionKind": command.Kind, "revisionNo": 1, "recordIds": recordIDs,
			"explanation": command.Explanation,
		}, CreatedAt: time.Now().UTC()}); err != nil {
		return workflow.Submission{}, err
	}
	return commitDepartmentBatch(ctx, tx, result.ID)
}

func requireZeroReceiptCell(ctx context.Context, tx pgx.Tx, scope workflow.Scope, academicYearID string,
	access inspection.MutationAccess, action string) error {
	if !access.Unrestricted && !permission.AllowsWholeScope(access.Permissions, action, permission.Scope{
		AcademicYearID: academicYearID, SemesterID: scope.SemesterID, WeekID: scope.WeekID,
		InspectionTypeID: scope.InspectionTypeID, DepartmentID: scope.DepartmentID,
	}) {
		return workflow.ErrSubmissionScope
	}
	var obligationID string
	err := tx.QueryRow(ctx, `SELECT id FROM weekly_report_obligations
 WHERE semester_id=$1 AND week_id=$2 AND department_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid
   AND inspection_type_id=$4 AND removed_at IS NULL FOR SHARE`, scope.SemesterID, scope.WeekID,
		scope.DepartmentID, scope.InspectionTypeID).Scan(&obligationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.ErrSubmissionScope
	}
	return err
}

func activeSubmissionType(code string) bool {
	switch code {
	case inspection.InspectionTypeAttendance, inspection.InspectionTypeUnattended,
		inspection.InspectionTypeMorning, inspection.InspectionTypeDormitoryHygiene,
		inspection.InspectionTypeLateReturn:
		return true
	default:
		return false
	}
}

// DecideDepartmentBatch locks the submission and every reviewable record before
// changing their states. A partial authorization never changes any row.
func (p *Postgres) DecideDepartmentBatch(ctx context.Context, command workflow.DecisionCommand) (workflow.Submission, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return workflow.Submission{}, err
	}
	defer tx.Rollback(ctx)
	result := workflow.Submission{}
	var academicYearID string
	err = tx.QueryRow(ctx, `SELECT ds.id,ds.semester_id,ds.week_id,COALESCE(ds.department_id::text,''),ds.inspection_type_id,
	 ds.submission_kind,ds.status,ds.revision_no,ds.row_version,ds.created_by,ds.submitted_by,ds.created_at,ds.submitted_at,sem.academic_year_id
	 FROM department_submissions ds JOIN semesters sem ON sem.id=ds.semester_id
	 WHERE ds.id=$1 FOR UPDATE OF ds`, command.SubmissionID).
		Scan(&result.ID, &result.Scope.SemesterID, &result.Scope.WeekID, &result.Scope.DepartmentID,
			&result.Scope.InspectionTypeID, &result.Kind, &result.Status, &result.RevisionNo, &result.RowVersion,
			&result.CreatedBy, &result.SubmittedBy, &result.CreatedAt, &result.SubmittedAt, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrSubmissionMissing
	}
	if err != nil {
		return workflow.Submission{}, err
	}
	var expected int
	var revisionKind workflow.SubmissionKind
	if err = tx.QueryRow(ctx, `SELECT submission_kind,cardinality(record_ids) FROM department_submission_revisions
	 WHERE submission_id=$1 AND revision_no=$2`, result.ID, result.RevisionNo).Scan(&revisionKind, &expected); err != nil {
		return workflow.Submission{}, err
	}
	if revisionKind != result.Kind {
		return workflow.Submission{}, workflow.ErrSubmissionScope
	}
	access := inspection.MutationAccess{ActorID: command.ActorID, Unrestricted: command.Unrestricted, Permissions: command.Permissions}
	IDs := make([]string, 0, expected)
	if result.Kind == workflow.SubmissionKindZeroRecord {
		if expected != 0 || (!access.Unrestricted && !permission.AllowsWholeScope(access.Permissions, permission.ActionInspectionReview, permission.Scope{
			AcademicYearID: academicYearID, SemesterID: result.Scope.SemesterID, WeekID: result.Scope.WeekID,
			InspectionTypeID: result.Scope.InspectionTypeID, DepartmentID: result.Scope.DepartmentID,
		})) {
			return workflow.Submission{}, workflow.ErrSubmissionScope
		}
	} else if expected > 0 {
		args := []any{result.ID, string(result.Status)}
		guard := inspectionAccessSQL(access, "review", &args)
		rows, err := tx.Query(ctx, `SELECT ir.id FROM inspection_records ir
 JOIN semesters sem ON sem.id=ir.semester_id
 WHERE ir.submission_id=$1 AND ir.status=$2 AND ir.deleted_at IS NULL
 AND ir.locked_by_report_id IS NULL AND `+guard+` ORDER BY ir.id FOR UPDATE OF ir`, args...)
		if err != nil {
			return workflow.Submission{}, err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return workflow.Submission{}, err
			}
			IDs = append(IDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return workflow.Submission{}, err
		}
	}
	if (result.Kind == workflow.SubmissionKindRecords && expected == 0) || len(IDs) != expected {
		return workflow.Submission{}, workflow.ErrSubmissionScope
	}
	if result.RowVersion != command.Version {
		return workflow.Submission{}, workflow.ErrVersionConflict
	}
	next, err := workflow.Transition(result.Status, command.Action)
	if err != nil {
		return workflow.Submission{}, err
	}
	if result.Kind == workflow.SubmissionKindRecords {
		tag, err := tx.Exec(ctx, `UPDATE inspection_records SET status=$1,row_version=row_version+1,updated_at=now()
 WHERE id=ANY($2::uuid[]) AND submission_id=$3 AND status='PENDING_REVIEW' AND deleted_at IS NULL
 AND locked_by_report_id IS NULL`, string(next), IDs, result.ID)
		if err != nil {
			return workflow.Submission{}, err
		}
		if int(tag.RowsAffected()) != expected {
			return workflow.Submission{}, workflow.ErrVersionConflict
		}
	}
	err = tx.QueryRow(ctx, `UPDATE department_submissions SET status=$2,row_version=row_version+1,
 reviewed_by=$3,reviewed_at=now(),return_reason=$4,updated_at=now()
 WHERE id=$1 AND row_version=$5 AND status='PENDING_REVIEW'
 RETURNING row_version,reviewed_at`, result.ID, string(next), command.ActorID,
		command.Reason, command.Version).Scan(&result.RowVersion, &result.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrVersionConflict
	}
	if err != nil {
		return workflow.Submission{}, err
	}
	result.Status, result.ReviewedBy, result.ReturnReason = next, command.ActorID, command.Reason
	action := audit.ActionDepartmentReturned
	if command.Action == workflow.Confirm {
		action = audit.ActionDepartmentConfirmed
	}
	if err = insertAuditTx(ctx, tx, command.Metadata, audit.Entry{Action: action,
		ResourceType: "departmentSubmission", ResourceID: result.ID,
		Before: map[string]any{"status": workflow.PendingReview, "version": command.Version},
		After:  map[string]any{"status": next, "version": result.RowVersion, "submissionKind": result.Kind, "recordIds": IDs},
	}); err != nil {
		return workflow.Submission{}, err
	}
	return commitDepartmentBatch(ctx, tx, result.ID)
}

// ResubmitDepartmentBatch preserves the batch and its original record set.
// Every returned record must still be in scope and carry the supplied version.
func (p *Postgres) ResubmitDepartmentBatch(ctx context.Context, command workflow.ResubmitCommand) (workflow.Submission, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return workflow.Submission{}, err
	}
	defer tx.Rollback(ctx)
	result := workflow.Submission{}
	var academicYearID string
	err = tx.QueryRow(ctx, `SELECT ds.id,ds.semester_id,ds.week_id,COALESCE(ds.department_id::text,''),ds.inspection_type_id,
	 ds.submission_kind,ds.status,ds.revision_no,ds.row_version,ds.created_by,ds.submitted_by,ds.created_at,ds.submitted_at,sem.academic_year_id
	 FROM department_submissions ds JOIN semesters sem ON sem.id=ds.semester_id
	 WHERE ds.id=$1 FOR UPDATE OF ds`, command.SubmissionID).
		Scan(&result.ID, &result.Scope.SemesterID, &result.Scope.WeekID, &result.Scope.DepartmentID,
			&result.Scope.InspectionTypeID, &result.Kind, &result.Status, &result.RevisionNo, &result.RowVersion,
			&result.CreatedBy, &result.SubmittedBy, &result.CreatedAt, &result.SubmittedAt, &academicYearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrSubmissionMissing
	}
	if err != nil {
		return workflow.Submission{}, err
	}
	if result.Kind != command.Kind {
		return workflow.Submission{}, workflow.ErrInvalidSubmit
	}
	var priorRecordIDs []string
	var revisionKind workflow.SubmissionKind
	err = tx.QueryRow(ctx, `SELECT submission_kind,record_ids FROM department_submission_revisions
	 WHERE submission_id=$1 AND revision_no=$2`, result.ID, result.RevisionNo).Scan(&revisionKind, &priorRecordIDs)
	if err != nil {
		return workflow.Submission{}, err
	}
	if revisionKind != result.Kind {
		return workflow.Submission{}, workflow.ErrSubmissionScope
	}
	if len(priorRecordIDs) != len(command.Records) {
		return workflow.Submission{}, workflow.ErrInvalidSubmit
	}
	for index, record := range command.Records {
		if priorRecordIDs[index] != record.ID {
			return workflow.Submission{}, workflow.ErrInvalidSubmit
		}
	}
	access := inspection.MutationAccess{ActorID: command.ActorID, Unrestricted: command.Unrestricted, Permissions: command.Permissions}
	if result.Kind == workflow.SubmissionKindZeroRecord {
		if err := requireZeroReceiptCell(ctx, tx, result.Scope, academicYearID, access, permission.ActionInspectionEnter); err != nil {
			return workflow.Submission{}, err
		}
	}
	snapshots := make([]json.RawMessage, 0, len(command.Records))
	for _, requested := range command.Records {
		args := []any{requested.ID, result.ID, string(result.Status)}
		guard := inspectionAccessSQL(access, "mutate", &args)
		var version int
		var snapshot json.RawMessage
		err = tx.QueryRow(ctx, `SELECT ir.row_version,
 jsonb_build_object('record',to_jsonb(ir),'attendanceDetails',to_jsonb(ad),
 'morningDetails',to_jsonb(md),'lateReturnDetails',to_jsonb(ld),'dormitoryHygieneDetails',to_jsonb(dh))
 FROM inspection_records ir
 JOIN semesters sem ON sem.id=ir.semester_id
 LEFT JOIN attendance_details ad ON ad.record_id=ir.id
 LEFT JOIN morning_exercise_details md ON md.record_id=ir.id
 LEFT JOIN late_return_details ld ON ld.record_id=ir.id
 LEFT JOIN dorm_hygiene_details dh ON dh.record_id=ir.id
 WHERE ir.id=$1 AND ir.submission_id=$2 AND ir.status=$3 AND ir.deleted_at IS NULL
 AND ir.locked_by_report_id IS NULL AND `+guard+` FOR UPDATE OF ir`, args...).Scan(&version, &snapshot)
		if errors.Is(err, pgx.ErrNoRows) {
			return workflow.Submission{}, workflow.ErrSubmissionScope
		}
		if err != nil {
			return workflow.Submission{}, err
		}
		if version != requested.Version {
			return workflow.Submission{}, workflow.ErrVersionConflict
		}
		snapshots = append(snapshots, snapshot)
	}
	if result.RowVersion != command.Version {
		return workflow.Submission{}, workflow.ErrVersionConflict
	}
	if _, err = workflow.Transition(result.Status, workflow.Submit); err != nil {
		return workflow.Submission{}, err
	}
	for _, record := range command.Records {
		tag, err := tx.Exec(ctx, `UPDATE inspection_records SET status='PENDING_REVIEW',row_version=row_version+1,updated_at=now()
 WHERE id=$1 AND submission_id=$2 AND row_version=$3 AND status='RETURNED' AND deleted_at IS NULL
 AND locked_by_report_id IS NULL`, record.ID, result.ID, record.Version)
		if err != nil {
			return workflow.Submission{}, err
		}
		if tag.RowsAffected() != 1 {
			return workflow.Submission{}, workflow.ErrVersionConflict
		}
	}
	err = tx.QueryRow(ctx, `UPDATE department_submissions SET status='PENDING_REVIEW',revision_no=revision_no+1,
 row_version=row_version+1,submitted_by=$2,submitted_at=now(),reviewed_by=NULL,reviewed_at=NULL,
 return_reason='',updated_at=now() WHERE id=$1 AND row_version=$3 AND status='RETURNED'
 RETURNING revision_no,row_version,submitted_at`, result.ID, command.ActorID, command.Version).
		Scan(&result.RevisionNo, &result.RowVersion, &result.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrVersionConflict
	}
	if err != nil {
		return workflow.Submission{}, err
	}
	encodedSnapshots, err := json.Marshal(snapshots)
	if err != nil {
		return workflow.Submission{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO department_submission_revisions(
	 id,submission_id,revision_no,submitted_by,submission_kind,record_ids,records_snapshot)
	 VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.NewString(), result.ID, result.RevisionNo,
		command.ActorID, result.Kind, priorRecordIDs, encodedSnapshots)
	if err != nil {
		return workflow.Submission{}, err
	}
	result.Status, result.SubmittedBy = workflow.PendingReview, command.ActorID
	if err = insertAuditTx(ctx, tx, command.Metadata, audit.Entry{Action: audit.ActionDepartmentResubmitted,
		ResourceType: "departmentSubmission", ResourceID: result.ID,
		Before: map[string]any{"status": workflow.Returned, "revisionNo": result.RevisionNo - 1, "submissionKind": result.Kind},
		After:  map[string]any{"status": workflow.PendingReview, "revisionNo": result.RevisionNo, "submissionKind": result.Kind, "recordIds": priorRecordIDs, "explanation": command.Explanation},
	}); err != nil {
		return workflow.Submission{}, err
	}
	return commitDepartmentBatch(ctx, tx, result.ID)
}
