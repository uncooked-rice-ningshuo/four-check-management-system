package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/workflow"
	"github.com/jackc/pgx/v5"
)

const submissionSelect = `SELECT ds.id,ds.semester_id,ds.week_id,COALESCE(ds.department_id::text,''),ds.inspection_type_id,
 ds.status,ds.revision_no,ds.row_version,ds.created_by,COALESCE(ds.submitted_by::text,''),
 COALESCE(ds.reviewed_by::text,''),ds.return_reason,ds.created_at,
 COALESCE(ds.submitted_at,ds.created_at),ds.reviewed_at,rev.record_ids,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',ir.id,'rowVersion',ir.row_version,
 'status',ir.status,'createdBy',ir.created_by) ORDER BY ir.id)
 FROM inspection_records ir WHERE ir.submission_id=ds.id AND ir.deleted_at IS NULL),'[]'::jsonb)
 ,sem.name,w.week_no,COALESCE(d.name,'校级')
 FROM department_submissions ds JOIN department_submission_revisions rev
 ON rev.submission_id=ds.id AND rev.revision_no=ds.revision_no
 JOIN semesters sem ON sem.id=ds.semester_id
 JOIN semester_weeks w ON w.id=ds.week_id
 LEFT JOIN departments d ON d.id=ds.department_id`

func submissionVisibilitySQL(access workflow.ViewAccess, args *[]any) string {
	a := inspection.MutationAccess{ActorID: access.ActorID, Unrestricted: access.Unrestricted, Permissions: access.Permissions}
	view := inspectionAccessSQL(a, "read", args)
	mutate := inspectionAccessSQL(a, "mutate", args)
	review := inspectionAccessSQL(a, "review", args)
	return `cardinality(rev.record_ids)=(SELECT count(*) FROM inspection_records existing
 WHERE existing.submission_id=ds.id AND existing.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM inspection_records ir JOIN semesters sem ON sem.id=ir.semester_id
 WHERE ir.submission_id=ds.id AND ir.deleted_at IS NULL AND NOT COALESCE((` + view + ` OR ` + mutate + ` OR ` + review + `),FALSE))`
}

func scanSubmission(row pgx.Row) (workflow.Submission, error) {
	var value workflow.Submission
	var recordsJSON []byte
	err := row.Scan(&value.ID, &value.Scope.SemesterID, &value.Scope.WeekID, &value.Scope.DepartmentID,
		&value.Scope.InspectionTypeID, &value.Status, &value.RevisionNo, &value.RowVersion,
		&value.CreatedBy, &value.SubmittedBy, &value.ReviewedBy, &value.ReturnReason,
		&value.CreatedAt, &value.SubmittedAt, &value.ReviewedAt, &value.RecordIDs, &recordsJSON,
		&value.SemesterName, &value.WeekNo, &value.DepartmentName)
	if err != nil {
		return workflow.Submission{}, err
	}
	if err := json.Unmarshal(recordsJSON, &value.Records); err != nil {
		return workflow.Submission{}, err
	}
	return value, nil
}

// commitDepartmentBatch reads the complete response before commit, so a
// successful mutation never returns a partial batch envelope.
func commitDepartmentBatch(ctx context.Context, tx pgx.Tx, id string) (workflow.Submission, error) {
	value, err := scanSubmission(tx.QueryRow(ctx, submissionSelect+` WHERE ds.id=$1`, id))
	if err != nil {
		return workflow.Submission{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workflow.Submission{}, err
	}
	return value, nil
}

// ListDepartmentBatches applies permission checks to every record in SQL.
func (p *Postgres) ListDepartmentBatches(ctx context.Context, access workflow.ViewAccess, query workflow.ListQuery) (workflow.Page, error) {
	args := []any{query.SemesterID, query.WeekID, query.DepartmentID, query.InspectionTypeID, string(query.Status)}
	visible := submissionVisibilitySQL(access, &args)
	filter := ` WHERE ($1::text='' OR ds.semester_id=NULLIF($1,'')::uuid)
 AND ($2::text='' OR ds.week_id=NULLIF($2,'')::uuid)
 AND ($3::text='' OR ds.department_id=NULLIF($3,'')::uuid)
 AND ($4::text='' OR ds.inspection_type_id=NULLIF($4,'')::uuid)
 AND ($5::text='' OR ds.status=$5) AND ` + visible
	if query.CollegeOnly {
		filter += " AND ds.department_id IS NULL"
	}
	from := ` FROM department_submissions ds JOIN department_submission_revisions rev
 ON rev.submission_id=ds.id AND rev.revision_no=ds.revision_no`
	page := workflow.Page{Items: make([]workflow.Submission, 0), Page: query.Page, PageSize: query.PageSize}
	if err := p.Pool.QueryRow(ctx, `SELECT count(*)`+from+filter, args...).Scan(&page.Total); err != nil {
		return workflow.Page{}, err
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := p.Pool.Query(ctx, submissionSelect+filter+fmt.Sprintf(" ORDER BY ds.updated_at DESC,ds.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return workflow.Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		value, err := scanSubmission(rows)
		if err != nil {
			return workflow.Page{}, err
		}
		page.Items = append(page.Items, value)
	}
	return page, rows.Err()
}

func (p *Postgres) GetDepartmentBatch(ctx context.Context, access workflow.ViewAccess, id string) (workflow.Submission, error) {
	args := []any{id}
	visible := submissionVisibilitySQL(access, &args)
	result, err := scanSubmission(p.Pool.QueryRow(ctx, submissionSelect+` WHERE ds.id=$1 AND `+visible, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return workflow.Submission{}, workflow.ErrSubmissionMissing
	}
	return result, err
}
