package store

import (
	"context"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// suggestedInspectionFilters is a convenience only. It never changes SQL visibility.
// Only a union expressible as one existing class/grade filter is suggested.
func suggestedInspectionFilters(ctx context.Context, tx pgx.Tx, semesterID, inspectionTypeID string, entries []permission.EffectivePermission) (map[string]string, error) {
	result := map[string]string{}
	var selected *permission.OperationRange
	for _, entry := range entries {
		if permission.IsDisciplineAction(entry.Action) || !permission.SupportsOperationRange(entry.Action) || (entry.Scope.InspectionTypeID != "" && entry.Scope.InspectionTypeID != inspectionTypeID) {
			continue
		}
		if entry.Effect == permission.EffectDeny || len(entry.Ranges) == 0 {
			return result, nil
		}
		for _, clause := range entry.Ranges {
			if clause.GradeStage != 0 {
				continue
			}
			if selected != nil && *selected != clause {
				return result, nil
			}
			copy := clause
			selected = &copy
		}
	}
	if selected == nil || (selected.MajorID != "" && selected.ClassID == "") {
		return result, nil
	}
	if selected.ClassID != "" {
		result["classId"] = selected.ClassID
		return result, nil
	}
	var grade string
	if selected.GradeID != "" {
		err := tx.QueryRow(ctx, `SELECT name FROM grades WHERE id=$1 AND enabled`, selected.GradeID).Scan(&grade)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
	}
	if grade != "" {
		result["grade"] = grade
	}
	return result, nil
}
