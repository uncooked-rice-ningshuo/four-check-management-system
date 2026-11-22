package store

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/reporting"
	"strings"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"github.com/jackc/pgx/v5"
)

var errDisciplineReviewConflict = errors.New("discipline review is no longer available")
var errDisciplineReviewInput = errors.New("invalid discipline review input")

// applyDisciplineReviewTx requires a class already authorized by the application.
// Recheck the deadline after taking the lock, including after waiting for another reviewer.
func applyDisciplineReviewTx(ctx context.Context, tx pgx.Tx, id, authorizedClass, decision, reason, source string, version int, metadata audit.Metadata) error {
	reason = strings.TrimSpace(reason)
	if metadata.ActorID == "" || authorizedClass == "" || version < 1 || (decision != "APPROVED" && decision != "DEFERRED") || (decision == "DEFERRED" && reason == "") || utf8.RuneCountInString(reason) > 500 {
		return errDisciplineReviewInput
	}
	var before string
	err := tx.QueryRow(ctx, `SELECT status FROM discipline_cases WHERE id=$1 AND responsibility_class_id=$2 AND row_version=$3 FOR UPDATE`, id, authorizedClass, version).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		return errDisciplineReviewConflict
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE discipline_cases SET status=$2,row_version=row_version+1,
 delay_reason=CASE WHEN $2='DEFERRED' THEN $3 ELSE delay_reason END,
 decided_by=CASE WHEN $2='APPROVED' THEN $4::uuid ELSE NULL END,
 decided_at=CASE WHEN $2='APPROVED' THEN statement_timestamp() ELSE NULL END,
 decision_source=CASE WHEN $2='APPROVED' THEN $5 ELSE NULL END
 WHERE id=$1 AND ((status='PENDING' AND due_at>statement_timestamp()) OR (status='DEFERRED' AND $2='APPROVED'))`, id, decision, reason, metadata.ActorID, source)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errDisciplineReviewConflict
	}
	metadata.Reason = reason
	return insertAuditTx(ctx, tx, metadata, audit.Entry{Action: "DISCIPLINE_MANUAL_REVIEW", ResourceType: "discipline_case", ResourceID: id, Before: map[string]any{"status": before, "rowVersion": version}, After: map[string]any{"status": decision, "rowVersion": version + 1, "source": source}})
}

func (p *Postgres) ReviewDisciplineCase(ctx context.Context, id string, input reporting.DisciplineReview, effective permission.EffectiveResult, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var scope permission.Scope
	err = tx.QueryRow(ctx, `SELECT c.id,c.grade_id,coalesce(c.major_id::text,'') FROM discipline_cases d JOIN classes c ON c.id=d.responsibility_class_id WHERE d.id=$1 FOR UPDATE OF d FOR SHARE OF c`, id).Scan(&scope.ClassID, &scope.GradeID, &scope.MajorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.ErrForbidden
	}
	if err != nil {
		return err
	}
	source := "AUTHORIZED_USER"
	if effective.OrganizationRoleCode == "TEACHER" {
		source = "TEACHER"
	}
	if effective.SystemRole == permission.SystemRoleSuperAdmin {
		source = "SUPER_ADMIN"
	} else if !effective.Allowed(permission.ActionDisciplineConfirm, scope) {
		return reporting.ErrForbidden
	}
	err = applyDisciplineReviewTx(ctx, tx, id, scope.ClassID, input.Decision, input.Reason, source, input.Version, metadata)
	if errors.Is(err, errDisciplineReviewConflict) {
		return reporting.ErrReportVersionConflict
	}
	if errors.Is(err, errDisciplineReviewInput) {
		return reporting.ErrInvalidReportRequest
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DisciplineReviewQueue filters rows before pagination, retaining source-specific denials.
func (p *Postgres) DisciplineReviewQueue(ctx context.Context, f reporting.ReviewQueueFilter, effective permission.EffectiveResult) (json.RawMessage, error) {
	if effective.SystemRole != permission.SystemRoleSuperAdmin && !effective.HasAnyAllowed(permission.ActionDisciplineConfirm) && !effective.HasAnyAllowed("DISCIPLINE_CASE_VIEW") {
		return nil, reporting.ErrForbidden
	}
	args := []any{f.Status, f.Student, f.PageSize, (f.Page - 1) * f.PageSize}
	processPredicate := disciplineScopeSQL(effective, permission.ActionDisciplineConfirm, &args)
	// Older processing-only grants retain access to their own actionable rows.
	visiblePredicate := processPredicate
	for _, grant := range effective.Permissions {
		if grant.Action == "DISCIPLINE_CASE_VIEW" {
			visiblePredicate = disciplineScopeSQL(effective, "DISCIPLINE_CASE_VIEW", &args)
			break
		}
	}
	var result json.RawMessage
	err := p.Pool.QueryRow(ctx, `WITH allowed AS (SELECT (`+processPredicate+`) AS can_process,d.id,d.student_number,c.name AS class_name,d.evidence->'tier'->>'name' AS tier,
 d.accumulated_amount,d.status,d.delay_reason,d.row_version,d.due_at FROM discipline_cases d JOIN classes c ON c.id=d.responsibility_class_id
 WHERE d.status IN ('PENDING','DEFERRED') AND ($1='' OR d.status=$1) AND strpos(d.student_number,$2)>0 AND (`+visiblePredicate+`)),
 page AS (SELECT * FROM allowed ORDER BY due_at,id LIMIT $3 OFFSET $4)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM allowed),'items',coalesce((SELECT jsonb_agg(jsonb_build_object(
 'canProcess',can_process,'id',id,'studentNumber',student_number,'className',class_name,'tier',tier,'amount',accumulated_amount,'status',status,
 'reason',delay_reason,'version',row_version,'dueAt',due_at) ORDER BY due_at,id) FROM page),'[]'))`, args...).Scan(&result)
	return result, err
}

func disciplineScopeSQL(effective permission.EffectiveResult, action string, args *[]any) string {
	var grants []string
	for _, allow := range effective.Permissions {
		if allow.Action != action || allow.Effect != permission.EffectAllow || allow.Scope != (permission.Scope{}) {
			continue
		}
		terms := []string{operationRangeSQL(allow.Ranges, "c.id", args)}
		for _, deny := range effective.Permissions {
			if deny.Action == allow.Action && deny.Effect == permission.EffectDeny && (deny.SourceDepartmentID == "" || deny.SourceDepartmentID == allow.SourceDepartmentID) {
				terms = append(terms, "NOT "+operationRangeSQL(deny.Ranges, "c.id", args))
			}
		}
		grants = append(grants, "("+strings.Join(terms, " AND ")+")")
	}
	predicate := "FALSE"
	if effective.SystemRole == permission.SystemRoleSuperAdmin {
		predicate = "TRUE"
	} else if len(grants) > 0 {
		predicate = strings.Join(grants, " OR ")
	}
	return predicate
}
