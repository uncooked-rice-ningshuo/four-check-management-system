package store

import (
	"context"
	"fmt"
	"strings"

	"four-check-management-system/server/internal/dashboard"
)

// Aggregate calculates dashboard counts only for the exact department/type
// pairs authorized by the application service. Personal counts are additionally
// constrained by created_by and therefore cannot absorb another member's work.
func (p *Postgres) Aggregate(ctx context.Context, query dashboard.AggregateQuery) ([]dashboard.ScopeCounts, error) {
	if len(query.Scopes) == 0 {
		return make([]dashboard.ScopeCounts, 0), nil
	}
	departments := make([]string, 0, len(query.Scopes))
	types := make([]string, 0, len(query.Scopes))
	for _, scope := range query.Scopes {
		departments = append(departments, scope.DepartmentID)
		types = append(types, scope.InspectionTypeID)
	}
	rows, err := p.Pool.Query(ctx, dashboardAggregateSQL, query.SemesterID, query.WeekID, departments, types)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dashboard.ScopeCounts, 0, len(query.Scopes))
	for rows.Next() {
		var value dashboard.ScopeCounts
		if err := rows.Scan(
			&value.DepartmentID, &value.InspectionTypeID, &value.Total, &value.Draft,
			&value.PendingReview, &value.Returned, &value.Confirmed, &value.Locked,
			&value.BatchTotal, &value.BatchPending, &value.BatchReturned, &value.BatchConfirmed,
		); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) PersonalTodos(ctx context.Context, query dashboard.PersonalTodoQuery) (dashboard.MyTodos, error) {
	if !query.Unrestricted && len(query.Allowed) == 0 {
		return dashboard.MyTodos{}, nil
	}
	sql, args := buildPersonalTodoQuery(query)
	result := dashboard.MyTodos{}
	if err := p.Pool.QueryRow(ctx, sql, args...).Scan(&result.Draft, &result.Returned); err != nil {
		return dashboard.MyTodos{}, err
	}
	result.Total = result.Draft + result.Returned
	return result, nil
}

func buildPersonalTodoQuery(query dashboard.PersonalTodoQuery) (string, []any) {
	args := []any{query.AccountID, query.SemesterID, query.WeekID}
	conditions := []string{
		"ir.created_by = $1::uuid", "ir.semester_id = $2::uuid", "ir.week_id = $3::uuid",
		"ir.deleted_at IS NULL", "ir.status IN ('DRAFT', 'RETURNED')",
	}
	if !query.Unrestricted {
		conditions = append(conditions, "("+dashboardScopeSetSQL(query.Allowed, &args)+")")
	}
	if len(query.Denied) > 0 {
		conditions = append(conditions, "NOT ("+dashboardScopeSetSQL(query.Denied, &args)+")")
	}
	return `SELECT
	COUNT(*) FILTER (WHERE ir.status = 'DRAFT')::int,
	COUNT(*) FILTER (WHERE ir.status = 'RETURNED')::int
FROM inspection_records ir WHERE ` + strings.Join(conditions, " AND "), args
}

func dashboardScopeSetSQL(scopes []dashboard.DataScope, args *[]any) string {
	clauses := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		parts := make([]string, 0, 4)
		for _, dimension := range []struct {
			column string
			value  string
		}{
			{"ir.inspection_type_id", scope.InspectionTypeID}, {"ir.department_id", scope.DepartmentID},
			{"ir.class_id", scope.ClassID}, {"ir.grade_snapshot", scope.Grade},
		} {
			if dimension.value == "" {
				continue
			}
			*args = append(*args, dimension.value)
			if dimension.column == "ir.grade_snapshot" {
				parts = append(parts, fmt.Sprintf("%s = $%d", dimension.column, len(*args)))
			} else {
				parts = append(parts, fmt.Sprintf("%s = $%d::uuid", dimension.column, len(*args)))
			}
		}
		if len(parts) == 0 {
			clauses = append(clauses, "TRUE")
		} else {
			clauses = append(clauses, "("+strings.Join(parts, " AND ")+")")
		}
	}
	return strings.Join(clauses, " OR ")
}

const dashboardAggregateSQL = `WITH authorized(department_id, inspection_type_id) AS (
	SELECT * FROM unnest($3::text[], $4::text[])
), record_counts AS (
	SELECT a.department_id, a.inspection_type_id,
	COUNT(ir.id)::int AS total,
	COUNT(ir.id) FILTER (WHERE ir.status = 'DRAFT')::int AS draft,
	COUNT(ir.id) FILTER (WHERE ir.status = 'PENDING_REVIEW')::int AS pending_review,
	COUNT(ir.id) FILTER (WHERE ir.status = 'RETURNED')::int AS returned,
	COUNT(ir.id) FILTER (WHERE ir.status = 'CONFIRMED')::int AS confirmed,
	COUNT(ir.id) FILTER (WHERE ir.status = 'LOCKED')::int AS locked
	FROM authorized a LEFT JOIN inspection_records ir
	ON COALESCE(ir.department_id::text,'') = a.department_id AND ir.inspection_type_id::text = a.inspection_type_id
	AND ir.semester_id = $1::uuid AND ir.week_id = $2::uuid AND ir.deleted_at IS NULL
	GROUP BY a.department_id, a.inspection_type_id
), batch_counts AS (
	SELECT COALESCE(ds.department_id::text,'') AS department_id, ds.inspection_type_id::text AS inspection_type_id,
	COUNT(*)::int AS total,
	COUNT(*) FILTER (WHERE ds.status='PENDING_REVIEW')::int AS pending_review,
	COUNT(*) FILTER (WHERE ds.status='RETURNED')::int AS returned,
	COUNT(*) FILTER (WHERE ds.status='CONFIRMED')::int AS confirmed
	FROM department_submissions ds JOIN authorized a ON a.department_id=COALESCE(ds.department_id::text,'')
	AND a.inspection_type_id=ds.inspection_type_id::text
	WHERE ds.semester_id=$1::uuid AND ds.week_id=$2::uuid
	GROUP BY ds.department_id, ds.inspection_type_id
)
SELECT r.department_id,r.inspection_type_id,r.total,r.draft,r.pending_review,r.returned,r.confirmed,r.locked,
	COALESCE(b.total,0),COALESCE(b.pending_review,0),COALESCE(b.returned,0),COALESCE(b.confirmed,0)
FROM record_counts r LEFT JOIN batch_counts b ON b.department_id=r.department_id AND b.inspection_type_id=r.inspection_type_id
ORDER BY r.department_id,r.inspection_type_id`
