package store

import (
	"context"
	"encoding/json"
	"errors"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func hasReportRevisionIntent(ctx context.Context, tx pgx.Tx, reportID string, baseVersion int) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM weekly_report_revision_intents
		WHERE report_id=$1::uuid AND base_version=$2)`, reportID, baseVersion).Scan(&exists)
	return exists, err
}

// StartReportRevision binds the authorization decision to the current frozen version under the report row lock.
func (p *Postgres) StartReportRevision(ctx context.Context, reportID, actorID string, rowVersion int, grants []permission.EffectivePermission, metadata audit.Metadata) (reporting.ReportRevisionIntent, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reporting.ReportRevisionIntent{}, err
	}
	defer tx.Rollback(ctx)
	var state reporting.ReportState
	var contentJSON []byte
	err = tx.QueryRow(ctx, `SELECT wr.id::text,wr.semester_id::text,wr.week_id::text,wr.status,wr.current_version,wr.row_version,
		wv.content_json FROM weekly_reports wr JOIN weekly_report_versions wv ON wv.report_id=wr.id AND wv.version_no=wr.current_version
		WHERE wr.id=$1::uuid FOR UPDATE OF wr`, reportID).
		Scan(&state.ID, &state.SemesterID, &state.WeekID, &state.Status, &state.VersionNo, &state.RowVersion, &contentJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportRevisionIntent{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportRevisionIntent{}, err
	}
	if state.RowVersion != rowVersion {
		return reporting.ReportRevisionIntent{}, reporting.ErrReportVersionConflict
	}
	if state.Status != reporting.ReportStatusFinalized && state.Status != reporting.ReportStatusPublished && state.Status != reporting.ReportStatusArchived {
		return reporting.ReportRevisionIntent{}, reporting.ErrInvalidReportTransition
	}
	var content reporting.ReportVersionContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return reporting.ReportRevisionIntent{}, err
	}
	if len(content.Rows) == 0 && !permission.AllowsWholeScope(grants, permission.ActionReportCorrect,
		permission.Scope{AcademicYearID: content.Period.AcademicYearID, SemesterID: state.SemesterID, WeekID: state.WeekID}) {
		return reporting.ReportRevisionIntent{}, reporting.ErrForbidden
	}
	for _, row := range content.Rows {
		scope := permission.Scope{AcademicYearID: content.Period.AcademicYearID, SemesterID: state.SemesterID,
			WeekID: state.WeekID, InspectionTypeID: row.InspectionTypeID, DepartmentID: row.DepartmentID}
		if !permission.AllowsWholeScope(grants, permission.ActionReportCorrect, scope) {
			return reporting.ReportRevisionIntent{}, reporting.ErrForbidden
		}
	}
	var intent reporting.ReportRevisionIntent
	err = tx.QueryRow(ctx, `SELECT id::text,created_at FROM weekly_report_revision_intents WHERE report_id=$1::uuid AND base_version=$2`, reportID, state.VersionNo).Scan(&intent.ID, &intent.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return reporting.ReportRevisionIntent{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		intent.ID = uuid.NewString()
		if err := tx.QueryRow(ctx, `INSERT INTO weekly_report_revision_intents(id,report_id,base_version,initiated_by)
			VALUES($1::uuid,$2::uuid,$3,$4::uuid) RETURNING created_at`, intent.ID, reportID, state.VersionNo, actorID).Scan(&intent.CreatedAt); err != nil {
			return reporting.ReportRevisionIntent{}, err
		}
		if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportRevisionStarted,
			ResourceType: "weeklyReport", ResourceID: reportID,
			Before: map[string]any{"status": state.Status, "versionNo": state.VersionNo},
			After:  map[string]any{"status": state.Status, "revisionIntentId": intent.ID, "baseVersion": state.VersionNo}}); err != nil {
			return reporting.ReportRevisionIntent{}, err
		}
	}
	intent.ReportID, intent.BaseVersion = reportID, state.VersionNo
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportRevisionIntent{}, err
	}
	return intent, nil
}
