package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (p *Postgres) ListReportPeriods(ctx context.Context, permissions []permission.EffectivePermission) ([]reporting.PeriodOption, error) {
	args := make([]any, 0, len(permissions)*7)
	access := reportPermissionPredicate(permissions, reportScopeColumns{
		academicYear: "ay.id", semester: "sem.id", week: "w.id", inspectionType: "it.id", department: "d.id",
	}, &args)
	query := `WITH report_cells AS (
 SELECT dis.department_id,dis.inspection_type_id,NULL::uuid AS semester_id,NULL::uuid AS week_id
 FROM department_inspection_scopes dis
 JOIN departments active_department ON active_department.id=dis.department_id AND active_department.enabled
 WHERE dis.enabled AND dis.can_enter
 UNION
 SELECT ds.department_id,ds.inspection_type_id,ds.semester_id,ds.week_id
 FROM department_submissions ds
 UNION
 SELECT o.department_id,o.inspection_type_id,o.semester_id,o.week_id
 FROM weekly_report_obligations o WHERE o.removed_at IS NULL
), visible_periods AS (
SELECT DISTINCT ay.id AS academic_year_id,ay.name AS academic_year_name,
       sem.id AS semester_id,sem.name AS semester_name,w.id AS week_id,w.week_no,
       w.start_date,w.end_date,ay.start_date AS academic_year_start,sem.start_date AS semester_start
FROM academic_years ay
JOIN semesters sem ON sem.academic_year_id=ay.id
JOIN semester_weeks w ON w.semester_id=sem.id
 WHERE EXISTS (
 SELECT 1 FROM report_cells rc
 LEFT JOIN departments d ON d.id=rc.department_id
 JOIN inspection_types it ON it.id=rc.inspection_type_id
 WHERE it.enabled AND it.participates_in_report
 AND (rc.semester_id IS NULL OR rc.semester_id=sem.id)
 AND (rc.week_id IS NULL OR rc.week_id=w.id)
 AND ` + access + `)
)
SELECT academic_year_id,academic_year_name,semester_id,semester_name,week_id,week_no,start_date,end_date
FROM visible_periods
ORDER BY academic_year_start DESC,semester_start DESC,week_no DESC`
	rows, err := p.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reporting.PeriodOption, 0)
	for rows.Next() {
		var item reporting.PeriodOption
		if err := rows.Scan(&item.AcademicYearID, &item.AcademicYearName, &item.SemesterID, &item.SemesterName,
			&item.WeekID, &item.WeekNo, &item.StartDate, &item.EndDate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (p *Postgres) ReportSummary(ctx context.Context, semesterID, weekID string, permissions []permission.EffectivePermission) (reporting.Summary, error) {
	args := []any{semesterID, weekID}
	departmentAccess := reportPermissionPredicate(permissions, reportScopeColumns{
		academicYear: "period.academic_year_id", semester: "period.semester_id", week: "period.week_id", inspectionType: "it.id", department: "d.id",
	}, &args)
	collegeAccess := reportPermissionPredicate(permissions, reportScopeColumns{
		academicYear: "period.academic_year_id", semester: "period.semester_id", week: "period.week_id", inspectionType: "it.id", department: "NULL::text",
	}, &args)
	query := `WITH period AS (
 SELECT ay.id AS academic_year_id,ay.name AS academic_year_name,sem.id AS semester_id,sem.name AS semester_name,
        w.id AS week_id,w.week_no,w.start_date,w.end_date
 FROM academic_years ay
 JOIN semesters sem ON sem.academic_year_id=ay.id
 JOIN semester_weeks w ON w.semester_id=sem.id
 WHERE sem.id=$1::uuid AND w.id=$2::uuid
), department_scope_candidates AS (
 SELECT dis.department_id,dis.inspection_type_id
 FROM department_inspection_scopes dis
 JOIN departments active_department ON active_department.id=dis.department_id AND active_department.enabled
 WHERE dis.enabled AND dis.can_enter
 UNION
 SELECT ds.department_id,ds.inspection_type_id
 FROM period
 JOIN department_submissions ds ON ds.semester_id=period.semester_id AND ds.week_id=period.week_id
 WHERE ds.department_id IS NOT NULL
 UNION
 SELECT o.department_id,o.inspection_type_id
 FROM period
 JOIN weekly_report_obligations o ON o.semester_id=period.semester_id AND o.week_id=period.week_id
 WHERE o.department_id IS NOT NULL AND o.removed_at IS NULL
), department_cells AS (
 SELECT d.id AS department_id,d.name AS department_name,FALSE AS college_level,
        it.id AS inspection_type_id,it.name AS inspection_type_name,it.sort_order AS inspection_type_order
 FROM period
 JOIN department_scope_candidates candidate ON TRUE
 JOIN departments d ON d.id=candidate.department_id
 JOIN inspection_types it ON it.id=candidate.inspection_type_id AND it.enabled AND it.participates_in_report
 WHERE ` + departmentAccess + `
), college_scope_candidates AS (
 SELECT college_submission.inspection_type_id
 FROM period
 JOIN department_submissions college_submission ON college_submission.semester_id=period.semester_id
  AND college_submission.week_id=period.week_id AND college_submission.department_id IS NULL
 UNION
 SELECT o.inspection_type_id
 FROM period
 JOIN weekly_report_obligations o ON o.semester_id=period.semester_id AND o.week_id=period.week_id
 WHERE o.department_id IS NULL AND o.removed_at IS NULL
), college_cells AS (
 SELECT NULL::uuid AS department_id,'校级'::text AS department_name,TRUE AS college_level,
        it.id AS inspection_type_id,it.name AS inspection_type_name,it.sort_order AS inspection_type_order
 FROM period
 JOIN college_scope_candidates candidate ON TRUE
 JOIN inspection_types it ON it.id=candidate.inspection_type_id AND it.enabled AND it.participates_in_report
 WHERE ` + collegeAccess + `
 GROUP BY it.id,it.name,it.sort_order
), cells AS (
 SELECT * FROM department_cells
 UNION ALL
 SELECT * FROM college_cells
)
SELECT period.academic_year_id,period.academic_year_name,period.semester_id,period.semester_name,
       period.week_id,period.week_no,period.start_date,period.end_date,
       COALESCE(c.department_id::text,''),c.department_name,c.college_level,c.inspection_type_id,c.inspection_type_name,
       EXISTS(SELECT 1 FROM weekly_report_obligations required
        WHERE required.semester_id=period.semester_id AND required.week_id=period.week_id
          AND required.department_id IS NOT DISTINCT FROM c.department_id
          AND required.inspection_type_id=c.inspection_type_id AND required.removed_at IS NULL),
       COUNT(ds.id)::int,
       COUNT(ds.id) FILTER (WHERE ds.status='DRAFT')::int,
       COUNT(ds.id) FILTER (WHERE ds.status='PENDING_REVIEW')::int,
       COUNT(ds.id) FILTER (WHERE ds.status='RETURNED')::int,
       COUNT(ds.id) FILTER (WHERE ds.status='CONFIRMED')::int,
       COALESCE(SUM(CASE WHEN ds.status='CONFIRMED' THEN COALESCE(cardinality(rev.record_ids),0) ELSE 0 END),0)::int,
       COALESCE(SUM(CASE WHEN ds.status IN ('PENDING_REVIEW','RETURNED') THEN COALESCE(cardinality(rev.record_ids),0) ELSE 0 END),0)::int,
       COUNT(ds.id) FILTER (WHERE ds.submission_kind='ZERO_RECORD' AND ds.status='PENDING_REVIEW')::int,
       COUNT(ds.id) FILTER (WHERE ds.submission_kind='ZERO_RECORD' AND ds.status='RETURNED')::int,
       COUNT(ds.id) FILTER (WHERE ds.submission_kind='ZERO_RECORD' AND ds.status='CONFIRMED')::int
FROM cells c
CROSS JOIN period
LEFT JOIN department_submissions ds ON ds.semester_id=period.semester_id AND ds.week_id=period.week_id
 AND ds.department_id IS NOT DISTINCT FROM c.department_id AND ds.inspection_type_id=c.inspection_type_id
LEFT JOIN department_submission_revisions rev ON rev.submission_id=ds.id AND rev.revision_no=ds.revision_no
GROUP BY period.academic_year_id,period.academic_year_name,period.semester_id,period.semester_name,
         period.week_id,period.week_no,period.start_date,period.end_date,
         c.department_id,c.department_name,c.college_level,c.inspection_type_id,c.inspection_type_name,c.inspection_type_order
ORDER BY c.inspection_type_order,c.college_level,c.department_name`
	rows, err := p.Pool.Query(ctx, query, args...)
	if err != nil {
		return reporting.Summary{}, err
	}
	defer rows.Close()
	result := reporting.Summary{Rows: make([]reporting.SummaryRow, 0)}
	for rows.Next() {
		var row reporting.SummaryRow
		if err := rows.Scan(&result.Period.AcademicYearID, &result.Period.AcademicYearName,
			&result.Period.SemesterID, &result.Period.SemesterName, &result.Period.WeekID,
			&result.Period.WeekNo, &result.Period.StartDate, &result.Period.EndDate,
			&row.DepartmentID, &row.DepartmentName, &row.CollegeLevel, &row.InspectionTypeID,
			&row.InspectionTypeName, &row.IsRequired, &row.SubmissionCount, &row.DraftBatchCount,
			&row.PendingReviewBatchCount, &row.ReturnedBatchCount, &row.ConfirmedBatchCount,
			&row.ConfirmedRecordCount, &row.PendingReviewOrReturnedRecordCount,
			&row.ZeroRecordPendingReviewBatchCount, &row.ZeroRecordReturnedBatchCount,
			&row.ZeroRecordConfirmedBatchCount); err != nil {
			return reporting.Summary{}, err
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

// ReportRecords reuses the inspection read model after translating only the
// caller's report-view rules. The query is restricted to the current confirmed
// submission revision before record details are loaded.
func (p *Postgres) ReportRecords(ctx context.Context, accountID string, query reporting.RecordsQuery, permissions []permission.EffectivePermission) ([]inspection.InspectionRecord, int, error) {
	inspectionPermissions := make([]permission.EffectivePermission, 0, len(permissions))
	for _, item := range permissions {
		if item.Action != permission.ActionReportView {
			continue
		}
		item.Action = permission.ActionInspectionView
		inspectionPermissions = append(inspectionPermissions, item)
	}
	// Records() validates this exact report cell first; the database query then
	// applies its period/type/department filters and report-view permissions.
	page, err := p.ListInspections(ctx, inspection.InspectionFilter{
		CollegeOnly: query.CollegeLevel, ConfirmedSubmissionOnly: true,
		SemesterID: query.SemesterID, WeekID: query.WeekID, DepartmentID: query.DepartmentID,
		InspectionTypeID: query.InspectionTypeID, Status: inspection.RecordStatusConfirmed,
		Page: query.Page, PageSize: query.PageSize,
	}, inspection.MutationAccess{ActorID: accountID, ReportScopeValidated: true, Permissions: inspectionPermissions})
	if err != nil {
		return nil, 0, err
	}
	return page.Items, page.Total, nil
}

type reportScopeColumns struct {
	academicYear   string
	semester       string
	week           string
	inspectionType string
	department     string
}

// reportPermissionPredicate compiles effective permissions into SQL so that
// department/type summaries are filtered before any aggregate is read. Class
// and grade grants cannot authorize a department-wide aggregate; a class- or
// grade-scoped deny conservatively hides the affected aggregate cell.
func reportPermissionPredicate(permissions []permission.EffectivePermission, columns reportScopeColumns, args *[]any) string {
	return permissionPredicateForAction(permission.ActionReportView, permissions, columns, args)
}

func reportObligationPermissionPredicate(permissions []permission.EffectivePermission, columns reportScopeColumns, args *[]any) string {
	return permissionPredicateForAction(permission.ActionReportObligationManage, permissions, columns, args)
}

func permissionPredicateForAction(action string, permissions []permission.EffectivePermission, columns reportScopeColumns, args *[]any) string {
	allows := make([]string, 0)
	denies := make([]string, 0)
	for _, item := range permissions {
		if item.Action != action {
			continue
		}
		switch item.Effect {
		case permission.EffectAllow:
			if item.Scope.ClassID != "" || item.Scope.Grade != "" ||
				(action == permission.ActionReportObligationManage && item.Scope.DepartmentID != "") {
				continue
			}
			allows = append(allows, reportScopeRulePredicate(item.Scope, columns, false, args))
		case permission.EffectDeny:
			denies = append(denies, reportScopeRulePredicate(item.Scope, columns, true, args))
		}
	}
	if len(allows) == 0 {
		return "FALSE"
	}
	result := "(" + strings.Join(allows, " OR ") + ")"
	if len(denies) > 0 {
		result += " AND NOT (" + strings.Join(denies, " OR ") + ")"
	}
	return result
}

func (p *Postgres) ObligationPeriod(ctx context.Context, semesterID, weekID string) (reporting.PeriodOption, error) {
	var period reporting.PeriodOption
	err := p.Pool.QueryRow(ctx, `SELECT ay.id,ay.name,sem.id,sem.name,w.id,w.week_no,w.start_date,w.end_date
		FROM academic_years ay JOIN semesters sem ON sem.academic_year_id=ay.id
		JOIN semester_weeks w ON w.semester_id=sem.id
		WHERE sem.id=$1::uuid AND w.id=$2::uuid AND ay.enabled AND sem.enabled AND w.enabled`, semesterID, weekID).Scan(
		&period.AcademicYearID, &period.AcademicYearName, &period.SemesterID, &period.SemesterName,
		&period.WeekID, &period.WeekNo, &period.StartDate, &period.EndDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.PeriodOption{}, reporting.ErrObligationPeriodUnavailable
	}
	return period, err
}

func (p *Postgres) ReportObligationOptions(ctx context.Context, semesterID, weekID string) (reporting.ReportObligationOptions, error) {
	options := reporting.ReportObligationOptions{
		InspectionTypes:  make([]reporting.ReportObligationInspectionType, 0),
		DepartmentScopes: make([]reporting.ReportObligationDepartmentScope, 0),
	}
	typeRows, err := p.Pool.Query(ctx, `SELECT id,name,sort_order FROM inspection_types
		WHERE enabled AND participates_in_report ORDER BY sort_order,name,id`)
	if err != nil {
		return reporting.ReportObligationOptions{}, err
	}
	for typeRows.Next() {
		var item reporting.ReportObligationInspectionType
		if err := typeRows.Scan(&item.ID, &item.Name, &item.SortOrder); err != nil {
			typeRows.Close()
			return reporting.ReportObligationOptions{}, err
		}
		options.InspectionTypes = append(options.InspectionTypes, item)
	}
	if err := typeRows.Err(); err != nil {
		typeRows.Close()
		return reporting.ReportObligationOptions{}, err
	}
	typeRows.Close()

	scopeRows, err := p.Pool.Query(ctx, `SELECT d.id,d.name,it.id,it.name
		FROM department_inspection_scopes dis
		JOIN departments d ON d.id=dis.department_id AND d.enabled
		JOIN inspection_types it ON it.id=dis.inspection_type_id AND it.enabled AND it.participates_in_report
		WHERE dis.enabled AND dis.can_enter
		ORDER BY d.name,it.sort_order,it.name,d.id`)
	if err != nil {
		return reporting.ReportObligationOptions{}, err
	}
	defer scopeRows.Close()
	for scopeRows.Next() {
		var item reporting.ReportObligationDepartmentScope
		if err := scopeRows.Scan(&item.DepartmentID, &item.DepartmentName, &item.InspectionTypeID, &item.InspectionTypeName); err != nil {
			return reporting.ReportObligationOptions{}, err
		}
		options.DepartmentScopes = append(options.DepartmentScopes, item)
	}
	return options, scopeRows.Err()
}

func (p *Postgres) ListReportObligations(ctx context.Context, semesterID, weekID string, permissions []permission.EffectivePermission) ([]reporting.ReportObligation, error) {
	args := []any{semesterID, weekID}
	access := reportObligationPermissionPredicate(permissions, reportScopeColumns{
		academicYear: "ay.id", semester: "sem.id", week: "w.id", inspectionType: "it.id", department: "d.id",
	}, &args)
	query := `SELECT o.id,o.semester_id,o.week_id,ay.id,COALESCE(o.department_id::text,''),COALESCE(d.name,'校级'),
		(o.department_id IS NULL),it.id,it.name,
		COALESCE(it.enabled AND it.participates_in_report AND (o.department_id IS NULL OR (d.enabled AND dis.enabled AND dis.can_enter)),FALSE),
		o.version,o.created_at,o.updated_at
		FROM weekly_report_obligations o
		JOIN semesters sem ON sem.id=o.semester_id AND sem.enabled
		JOIN academic_years ay ON ay.id=sem.academic_year_id AND ay.enabled
		JOIN semester_weeks w ON w.id=o.week_id AND w.semester_id=o.semester_id AND w.enabled
		LEFT JOIN departments d ON d.id=o.department_id
		LEFT JOIN department_inspection_scopes dis ON dis.department_id=o.department_id AND dis.inspection_type_id=o.inspection_type_id
		JOIN inspection_types it ON it.id=o.inspection_type_id
		WHERE o.semester_id=$1::uuid AND o.week_id=$2::uuid AND o.removed_at IS NULL AND ` + access + `
		ORDER BY it.sort_order,(o.department_id IS NULL),COALESCE(d.name,'校级'),o.id`
	rows, err := p.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]reporting.ReportObligation, 0)
	for rows.Next() {
		item, err := scanReportObligation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) CreateReportObligation(ctx context.Context, id string, input reporting.CreateReportObligationInput, metadata audit.Metadata) (reporting.ReportObligation, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportObligation{}, err
	}
	defer tx.Rollback(ctx)

	var period reporting.PeriodOption
	err = tx.QueryRow(ctx, `SELECT ay.id,ay.name,sem.id,sem.name,w.id,w.week_no,w.start_date,w.end_date
		FROM academic_years ay JOIN semesters sem ON sem.academic_year_id=ay.id
		JOIN semester_weeks w ON w.semester_id=sem.id
		WHERE sem.id=$1::uuid AND w.id=$2::uuid AND ay.enabled AND sem.enabled AND w.enabled
		FOR SHARE OF ay,sem,w`, input.SemesterID, input.WeekID).Scan(
		&period.AcademicYearID, &period.AcademicYearName, &period.SemesterID, &period.SemesterName,
		&period.WeekID, &period.WeekNo, &period.StartDate, &period.EndDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportObligation{}, reporting.ErrObligationPeriodUnavailable
	}
	if err != nil {
		return reporting.ReportObligation{}, err
	}
	var inspectionTypeName string
	if err := tx.QueryRow(ctx, `SELECT name FROM inspection_types
		WHERE id=$1::uuid AND enabled AND participates_in_report FOR SHARE`, input.InspectionTypeID).Scan(&inspectionTypeName); errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportObligation{}, reporting.ErrObligationScopeUnavailable
	} else if err != nil {
		return reporting.ReportObligation{}, err
	}
	departmentName := "校级"
	collegeLevel := input.DepartmentID == ""
	if !collegeLevel {
		if err := tx.QueryRow(ctx, `SELECT d.name FROM departments d
			JOIN department_inspection_scopes dis ON dis.department_id=d.id AND dis.inspection_type_id=$2::uuid
			WHERE d.id=$1::uuid AND d.enabled AND dis.enabled AND dis.can_enter
			FOR SHARE OF d,dis`, input.DepartmentID, input.InspectionTypeID).Scan(&departmentName); errors.Is(err, pgx.ErrNoRows) {
			return reporting.ReportObligation{}, reporting.ErrObligationScopeUnavailable
		} else if err != nil {
			return reporting.ReportObligation{}, err
		}
	}
	var item reporting.ReportObligation
	item.ID, item.SemesterID, item.WeekID, item.AcademicYearID = id, input.SemesterID, input.WeekID, period.AcademicYearID
	item.DepartmentID, item.DepartmentName, item.CollegeLevel = input.DepartmentID, departmentName, collegeLevel
	item.InspectionTypeID, item.InspectionTypeName, item.Eligible = input.InspectionTypeID, inspectionTypeName, true
	err = tx.QueryRow(ctx, `INSERT INTO weekly_report_obligations(
		id,semester_id,week_id,department_id,inspection_type_id,created_by,updated_by)
		VALUES($1::uuid,$2::uuid,$3::uuid,NULLIF($4,'')::uuid,$5::uuid,$6::uuid,$6::uuid)
		RETURNING version,created_at,updated_at`, id, input.SemesterID, input.WeekID,
		input.DepartmentID, input.InspectionTypeID, metadata.ActorID).Scan(&item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return reporting.ReportObligation{}, reportObligationWriteError(err)
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportObligationChanged, ResourceType: "weeklyReportObligation", ResourceID: item.ID,
		After: item, CreatedAt: time.Now().UTC(),
	}); err != nil {
		return reporting.ReportObligation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportObligation{}, reportObligationWriteError(err)
	}
	return item, nil
}

func (p *Postgres) GetReportObligation(ctx context.Context, id string) (reporting.ReportObligation, error) {
	item, err := scanReportObligation(p.Pool.QueryRow(ctx, `SELECT o.id,o.semester_id,o.week_id,ay.id,
		COALESCE(o.department_id::text,''),COALESCE(d.name,'校级'),(o.department_id IS NULL),
		it.id,it.name,COALESCE(it.enabled AND it.participates_in_report AND (o.department_id IS NULL OR (d.enabled AND dis.enabled AND dis.can_enter)),FALSE),
		o.version,o.created_at,o.updated_at
		FROM weekly_report_obligations o
		JOIN semesters sem ON sem.id=o.semester_id
		JOIN academic_years ay ON ay.id=sem.academic_year_id
		JOIN semester_weeks w ON w.id=o.week_id AND w.semester_id=o.semester_id
		LEFT JOIN departments d ON d.id=o.department_id
		LEFT JOIN department_inspection_scopes dis ON dis.department_id=o.department_id AND dis.inspection_type_id=o.inspection_type_id
		JOIN inspection_types it ON it.id=o.inspection_type_id
		WHERE o.id=$1::uuid AND o.removed_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportObligation{}, reporting.ErrObligationNotFound
	}
	return item, err
}

func (p *Postgres) RemoveReportObligation(ctx context.Context, id string, version int, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	before, err := scanReportObligation(tx.QueryRow(ctx, `SELECT o.id,o.semester_id,o.week_id,ay.id,
		COALESCE(o.department_id::text,''),COALESCE(d.name,'校级'),(o.department_id IS NULL),
		it.id,it.name,COALESCE(it.enabled AND it.participates_in_report AND (o.department_id IS NULL OR (d.enabled AND dis.enabled AND dis.can_enter)),FALSE),
		o.version,o.created_at,o.updated_at
		FROM weekly_report_obligations o
		JOIN semesters sem ON sem.id=o.semester_id
		JOIN academic_years ay ON ay.id=sem.academic_year_id
		JOIN semester_weeks w ON w.id=o.week_id AND w.semester_id=o.semester_id
		LEFT JOIN departments d ON d.id=o.department_id
		LEFT JOIN department_inspection_scopes dis ON dis.department_id=o.department_id AND dis.inspection_type_id=o.inspection_type_id
		JOIN inspection_types it ON it.id=o.inspection_type_id
		WHERE o.id=$1::uuid AND o.removed_at IS NULL FOR UPDATE OF o`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ErrObligationNotFound
	}
	if err != nil {
		return err
	}
	if before.Version != version {
		return reporting.ErrObligationVersionConflict
	}
	original := before
	var removedAt time.Time
	if err := tx.QueryRow(ctx, `UPDATE weekly_report_obligations
		SET removed_at=now(),updated_at=now(),updated_by=$2::uuid,version=version+1
		WHERE id=$1::uuid AND version=$3 AND removed_at IS NULL
		RETURNING removed_at,updated_at,version`, id, metadata.ActorID, version).Scan(&removedAt, &before.UpdatedAt, &before.Version); err != nil {
		return err
	}
	before.RemovedAt = &removedAt
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionReportObligationChanged, ResourceType: "weeklyReportObligation", ResourceID: id,
		Before: reportingObligationAuditValue(original, false), After: reportingObligationAuditValue(before, true), CreatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type reportObligationScanner interface {
	Scan(...any) error
}

func scanReportObligation(row reportObligationScanner) (reporting.ReportObligation, error) {
	var item reporting.ReportObligation
	err := row.Scan(&item.ID, &item.SemesterID, &item.WeekID, &item.AcademicYearID,
		&item.DepartmentID, &item.DepartmentName, &item.CollegeLevel, &item.InspectionTypeID,
		&item.InspectionTypeName, &item.Eligible, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func reportingObligationAuditValue(item reporting.ReportObligation, removed bool) map[string]any {
	value := map[string]any{
		"semesterId": item.SemesterID, "weekId": item.WeekID, "departmentId": item.DepartmentID,
		"collegeLevel": item.CollegeLevel, "inspectionTypeId": item.InspectionTypeID, "version": item.Version,
		"updatedAt": item.UpdatedAt,
	}
	if removed {
		value["removedAt"] = item.RemovedAt
	}
	return value
}

func reportObligationWriteError(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" &&
		(databaseError.ConstraintName == "uq_weekly_report_obligations_active_department" || databaseError.ConstraintName == "uq_weekly_report_obligations_active_college") {
		return reporting.ErrObligationAlreadyExists
	}
	return err
}

func reportScopeRulePredicate(scope permission.Scope, columns reportScopeColumns, overlap bool, args *[]any) string {
	dimensions := []struct {
		value  string
		column string
		isDept bool
	}{
		{scope.AcademicYearID, columns.academicYear, false},
		{scope.SemesterID, columns.semester, false},
		{scope.WeekID, columns.week, false},
		{scope.InspectionTypeID, columns.inspectionType, false},
		{scope.DepartmentID, columns.department, true},
	}
	conditions := make([]string, 0, len(dimensions)+1)
	for _, dimension := range dimensions {
		*args = append(*args, dimension.value)
		placeholder := fmt.Sprintf("$%d", len(*args))
		if overlap && dimension.isDept {
			if dimension.value == "" {
				conditions = append(conditions, "("+placeholder+"='' OR "+dimension.column+" IS NULL OR "+dimension.column+"::text="+placeholder+")")
			} else {
				conditions = append(conditions, "COALESCE("+dimension.column+"::text="+placeholder+",FALSE)")
			}
		} else {
			conditions = append(conditions, "("+placeholder+"='' OR "+dimension.column+"::text="+placeholder+")")
		}
	}
	if overlap && (scope.ClassID != "" || scope.Grade != "") {
		conditions = append(conditions, "TRUE")
	}
	return "(" + strings.Join(conditions, " AND ") + ")"
}
