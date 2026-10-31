package store

import (
	"context"
	"fmt"
	"strings"

	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"github.com/jackc/pgx/v5"
)

// ListReportArchive returns reports with at least one generated data version.
// The visibility predicate checks every frozen version before exposing even
// period-level metadata, including reports that have not been finalized yet.
func (p *Postgres) ListReportArchive(ctx context.Context, query reporting.ReportArchiveQuery,
	permissions []permission.EffectivePermission) (reporting.ReportArchivePage, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return reporting.ReportArchivePage{}, err
	}
	defer tx.Rollback(ctx)

	args := make([]any, 0)
	access := permissionPredicateForAction(permission.ActionArchiveView, permissions, reportScopeColumns{
		academicYear:   "sem.academic_year_id",
		semester:       "wr.semester_id",
		week:           "wr.week_id",
		inspectionType: "cell->>'inspectionTypeId'",
		department:     "NULLIF(cell->>'departmentId','')",
	}, &args)
	filters := []string{
		"wr.current_version >= 1",
		`NOT EXISTS (
		 SELECT 1
		 FROM weekly_report_versions history
		 CROSS JOIN LATERAL jsonb_array_elements(
		  CASE WHEN jsonb_typeof(history.content_json->'rows')='array'
		   THEN history.content_json->'rows' ELSE '[]'::jsonb END
		 ) AS cells(cell)
		 WHERE history.report_id=wr.id
		  AND (cell->>'inspectionTypeId' IS NULL OR NOT (` + access + `))
		)`,
	}
	if query.Status != "" {
		args = append(args, query.Status)
		filters = append(filters, fmt.Sprintf("wr.status=$%d", len(args)))
	}
	base := `FROM weekly_reports wr
	 JOIN semesters sem ON sem.id=wr.semester_id
	 JOIN academic_years ay ON ay.id=sem.academic_year_id
	 JOIN semester_weeks week ON week.id=wr.week_id AND week.semester_id=wr.semester_id
	 JOIN weekly_report_versions current_version ON current_version.report_id=wr.id AND current_version.version_no=wr.current_version
	 WHERE ` + strings.Join(filters, " AND ")
	var total int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return reporting.ReportArchivePage{}, err
	}

	pageArgs := append([]any(nil), args...)
	pageArgs = append(pageArgs, query.PageSize, (query.Page-1)*query.PageSize)
	pageQuery := `SELECT wr.id::text,wr.status,wr.current_version,
	 (SELECT COUNT(*) FROM weekly_report_versions versions WHERE versions.report_id=wr.id),
		COALESCE((current_version.content_json->>'confirmedRecordCount')::integer,0),
		COALESCE((SELECT SUM((cell->>'confirmedZeroReceiptCount')::integer)
		 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(current_version.content_json->'rows')='array' THEN current_version.content_json->'rows' ELSE '[]'::jsonb END) AS cells(cell)),0),
		COALESCE(jsonb_array_length(CASE WHEN jsonb_typeof(current_version.content_json->'rows')='array' THEN current_version.content_json->'rows' ELSE '[]'::jsonb END),0),current_version.scope_limited,
	 wr.finalized_at,wr.published_at,wr.updated_at,
	 ay.id::text,ay.name,sem.id::text,sem.name,week.id::text,week.week_no,week.start_date,week.end_date
	 ` + base + fmt.Sprintf(" ORDER BY wr.finalized_at DESC NULLS LAST,wr.updated_at DESC,week.week_no DESC,wr.id LIMIT $%d OFFSET $%d",
		len(args)+1, len(args)+2)
	rows, err := tx.Query(ctx, pageQuery, pageArgs...)
	if err != nil {
		return reporting.ReportArchivePage{}, err
	}
	page := reporting.ReportArchivePage{Items: make([]reporting.ReportArchiveEntry, 0), Total: total, Page: query.Page, PageSize: query.PageSize}
	for rows.Next() {
		var item reporting.ReportArchiveEntry
		if err := rows.Scan(&item.ID, &item.Status, &item.VersionNo, &item.VersionCount,
			&item.ConfirmedRecordCount, &item.ConfirmedZeroReceiptCount, &item.CellCount, &item.ScopeLimited,
			&item.FinalizedAt, &item.PublishedAt, &item.UpdatedAt,
			&item.Period.AcademicYearID, &item.Period.AcademicYearName,
			&item.Period.SemesterID, &item.Period.SemesterName,
			&item.Period.WeekID, &item.Period.WeekNo, &item.Period.StartDate, &item.Period.EndDate); err != nil {
			rows.Close()
			return reporting.ReportArchivePage{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return reporting.ReportArchivePage{}, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return reporting.ReportArchivePage{}, err
	}
	return page, nil
}
