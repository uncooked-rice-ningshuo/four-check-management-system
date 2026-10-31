package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5"
)

// GetReportWorkflow uses one read-only snapshot for authorization and history
// loading so a concurrently appended version cannot bypass the scope check.
func (p *Postgres) GetReportWorkflow(ctx context.Context, semesterID, weekID string,
	permissions []permission.EffectivePermission) (reporting.ReportWorkflow, error) {
	return p.GetReportWorkflowForAction(ctx, semesterID, weekID, permissions, permission.ActionReportView)
}

func (p *Postgres) GetReportWorkflowForAction(ctx context.Context, semesterID, weekID string,
	permissions []permission.EffectivePermission, action string) (reporting.ReportWorkflow, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return reporting.ReportWorkflow{}, err
	}
	defer tx.Rollback(ctx)

	args := []any{semesterID, weekID}
	access := permissionPredicateForAction(action, permissions, reportScopeColumns{
		academicYear:   "sem.academic_year_id",
		semester:       "wr.semester_id",
		week:           "wr.week_id",
		inspectionType: "cell->>'inspectionTypeId'",
		department:     "NULLIF(cell->>'departmentId','')",
	}, &args)
	var reportExists, canRead bool
	query := `WITH target AS (
 SELECT wr.id
 FROM weekly_reports wr
 JOIN semesters sem ON sem.id=wr.semester_id
 WHERE wr.semester_id=$1::uuid AND wr.week_id=$2::uuid
), visibility AS (
 SELECT target.id,
 NOT EXISTS (
  SELECT 1
  FROM weekly_report_versions version
  CROSS JOIN LATERAL jsonb_array_elements(
   CASE WHEN jsonb_typeof(version.content_json->'rows')='array'
    THEN version.content_json->'rows' ELSE '[]'::jsonb END
  ) AS cells(cell)
  JOIN weekly_reports wr ON wr.id=version.report_id
  JOIN semesters sem ON sem.id=wr.semester_id
  WHERE version.report_id=target.id
   AND (cell->>'inspectionTypeId' IS NULL OR NOT (` + access + `))
 ) AS can_read
 FROM target
)
SELECT EXISTS(SELECT 1 FROM target),COALESCE((SELECT can_read FROM visibility),FALSE)`
	if err := tx.QueryRow(ctx, query, args...).Scan(&reportExists, &canRead); err != nil {
		return reporting.ReportWorkflow{}, err
	}
	workflow := reporting.ReportWorkflow{
		Versions:    make([]reporting.ReportVersion, 0),
		Reviews:     make([]reporting.ReportReview, 0),
		Corrections: make([]reporting.ReportCorrectionLog, 0),
	}
	if !reportExists {
		if err := tx.Commit(ctx); err != nil {
			return reporting.ReportWorkflow{}, err
		}
		return workflow, nil
	}
	if !canRead {
		return reporting.ReportWorkflow{}, reporting.ErrForbidden
	}

	var state reporting.ReportState
	if err := tx.QueryRow(ctx, `SELECT id::text,semester_id::text,week_id::text,status,current_version,row_version,updated_at
FROM weekly_reports WHERE semester_id=$1::uuid AND week_id=$2::uuid`, semesterID, weekID).Scan(
		&state.ID, &state.SemesterID, &state.WeekID, &state.Status, &state.VersionNo, &state.RowVersion, &state.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reporting.ReportWorkflow{}, reporting.ErrReportNotFound
		}
		return reporting.ReportWorkflow{}, err
	}
	workflow.Report = &state

	versionRows, err := tx.Query(ctx, `SELECT id::text,version_no,content_json::text,created_by::text,created_at
FROM weekly_report_versions WHERE report_id=$1::uuid ORDER BY version_no DESC`, state.ID)
	if err != nil {
		return reporting.ReportWorkflow{}, err
	}
	for versionRows.Next() {
		var version reporting.ReportVersion
		var content string
		if err := versionRows.Scan(&version.ID, &version.VersionNo, &content, &version.CreatedBy, &version.CreatedAt); err != nil {
			versionRows.Close()
			return reporting.ReportWorkflow{}, err
		}
		if err := json.Unmarshal([]byte(content), &version.Content); err != nil {
			versionRows.Close()
			return reporting.ReportWorkflow{}, err
		}
		workflow.Versions = append(workflow.Versions, version)
	}
	if err := versionRows.Err(); err != nil {
		versionRows.Close()
		return reporting.ReportWorkflow{}, err
	}
	versionRows.Close()

	reviewRows, err := tx.Query(ctx, `SELECT review.id::text,version.version_no,review.action,review.comment,review.actor_id::text,review.created_at
FROM weekly_report_reviews review
JOIN weekly_report_versions version ON version.id=review.report_version_id
WHERE review.report_id=$1::uuid ORDER BY review.created_at,review.id`, state.ID)
	if err != nil {
		return reporting.ReportWorkflow{}, err
	}
	for reviewRows.Next() {
		var review reporting.ReportReview
		if err := reviewRows.Scan(&review.ID, &review.VersionNo, &review.Action, &review.Comment, &review.ActorID, &review.CreatedAt); err != nil {
			reviewRows.Close()
			return reporting.ReportWorkflow{}, err
		}
		workflow.Reviews = append(workflow.Reviews, review)
	}
	if err := reviewRows.Err(); err != nil {
		reviewRows.Close()
		return reporting.ReportWorkflow{}, err
	}
	reviewRows.Close()

	correctionRows, err := tx.Query(ctx, `SELECT correction.id::text,version.version_no,correction.inspection_record_id::text,
 correction.after_json->>'field',correction.reason,correction.fact_changed,correction.department_reconfirmation_status,
 correction.corrected_by::text,correction.corrected_at,
 COALESCE(response.decision,''),COALESCE(response.comment,''),COALESCE(response.actor_id::text,''),response.created_at
FROM weekly_report_corrections correction
JOIN weekly_report_versions version ON version.id=correction.report_version_id
LEFT JOIN weekly_report_correction_responses response ON response.correction_id=correction.id
WHERE correction.report_id=$1::uuid ORDER BY correction.corrected_at,correction.id`, state.ID)
	if err != nil {
		return reporting.ReportWorkflow{}, err
	}
	for correctionRows.Next() {
		var correction reporting.ReportCorrectionLog
		if err := correctionRows.Scan(&correction.ID, &correction.VersionNo, &correction.RecordID, &correction.Field,
			&correction.Reason, &correction.FactChanged, &correction.Reconfirmation, &correction.CorrectedBy,
			&correction.CorrectedAt, &correction.Decision, &correction.DecisionComment, &correction.DecisionActorID,
			&correction.DecisionCreatedAt); err != nil {
			correctionRows.Close()
			return reporting.ReportWorkflow{}, err
		}
		workflow.Corrections = append(workflow.Corrections, correction)
	}
	if err := correctionRows.Err(); err != nil {
		correctionRows.Close()
		return reporting.ReportWorkflow{}, err
	}
	correctionRows.Close()
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportWorkflow{}, err
	}
	return workflow, nil
}

// ListReportVersionRecords returns only the immutable, report-safe projection
// saved with the selected version. The SQL predicate repeats REPORT_VIEW scope
// checks before reading personal record details.
func (p *Postgres) ListReportVersionRecords(ctx context.Context, semesterID, weekID string, versionNo int,
	query reporting.ReportVersionRecordsQuery, permissions []permission.EffectivePermission) (reporting.RecordPage, error) {
	args := []any{semesterID, weekID, versionNo}
	access := permissionPredicateForAction(permission.ActionReportView, permissions, reportScopeColumns{
		academicYear:   "sem.academic_year_id",
		semester:       "wr.semester_id",
		week:           "wr.week_id",
		inspectionType: "record.inspection_type_id::text",
		department:     "record.department_id::text",
	}, &args)
	base := `FROM weekly_report_version_records version_record
	 JOIN weekly_report_versions version ON version.id=version_record.report_version_id
	 JOIN weekly_reports wr ON wr.id=version.report_id
	 JOIN semesters sem ON sem.id=wr.semester_id
	 JOIN inspection_records record ON record.id=version_record.inspection_record_id
	 WHERE wr.semester_id=$1::uuid AND wr.week_id=$2::uuid AND version.version_no=$3 AND (` + access + `)`
	if query.InspectionTypeID != "" {
		args = append(args, query.InspectionTypeID)
		typePosition := len(args)
		if query.CollegeLevel {
			base += fmt.Sprintf(" AND record.inspection_type_id=$%d::uuid AND record.department_id IS NULL", typePosition)
		} else {
			args = append(args, query.DepartmentID)
			base += fmt.Sprintf(" AND record.inspection_type_id=$%d::uuid AND record.department_id=$%d::uuid", typePosition, len(args))
		}
	}
	var total int
	if err := p.Pool.QueryRow(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return reporting.RecordPage{}, err
	}
	pageArgs := append([]any(nil), args...)
	pageArgs = append(pageArgs, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := p.Pool.Query(ctx, `SELECT version_record.included_data_json::text `+base+
		fmt.Sprintf(" ORDER BY version_record.inspection_record_id LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), pageArgs...)
	if err != nil {
		return reporting.RecordPage{}, err
	}
	defer rows.Close()
	page := reporting.RecordPage{Items: make([]reporting.Record, 0), Total: total, Page: query.Page, PageSize: query.PageSize}
	for rows.Next() {
		var encoded string
		var record reporting.Record
		if err := rows.Scan(&encoded); err != nil {
			return reporting.RecordPage{}, err
		}
		if err := json.Unmarshal([]byte(encoded), &record); err != nil {
			return reporting.RecordPage{}, err
		}
		page.Items = append(page.Items, record)
	}
	if err := rows.Err(); err != nil {
		return reporting.RecordPage{}, err
	}
	return page, nil
}
