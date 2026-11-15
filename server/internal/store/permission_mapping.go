package store

import (
	"context"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// requireStageConfiguration only diagnoses a wholly unavailable mapped grant.
// An independent configured or unrestricted clause must remain usable.
func requireStageConfiguration(ctx context.Context, tx pgx.Tx, semesterID, inspectionTypeID string, entries []permission.EffectivePermission) error {
	if semesterID == "" {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT mapping.stage,mapping.grade_id::text FROM academic_year_grade_stages mapping JOIN semesters sem ON sem.academic_year_id=mapping.academic_year_id WHERE sem.id=$1`, semesterID)
	if err != nil {
		return err
	}
	stages := map[int]string{}
	for rows.Next() {
		var stage int
		var grade string
		if err := rows.Scan(&stage, &grade); err != nil {
			rows.Close()
			return err
		}
		stages[stage] = grade
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	missing := false
	for _, entry := range entries {
		if entry.Effect != permission.EffectAllow || !permission.SupportsOperationRange(entry.Action) || (entry.Scope.InspectionTypeID != "" && entry.Scope.InspectionTypeID != inspectionTypeID) {
			continue
		}
		if len(entry.Ranges) == 0 {
			return nil
		}
		for _, clause := range entry.Ranges {
			if clause.GradeStage == 0 || stages[clause.GradeStage] != "" {
				return nil
			}
			missing = true
		}
	}
	if missing {
		return permission.ErrGradeMappingMissing
	}
	return nil
}

// suggestedInspectionFilters is a convenience only. It never changes SQL visibility.
// Only a union expressible as one existing class/grade filter is suggested.
func suggestedInspectionFilters(ctx context.Context, tx pgx.Tx, semesterID, inspectionTypeID string, entries []permission.EffectivePermission) (map[string]string, error) {
	result := map[string]string{}
	var selected *permission.OperationRange
	for _, entry := range entries {
		if !permission.SupportsOperationRange(entry.Action) || (entry.Scope.InspectionTypeID != "" && entry.Scope.InspectionTypeID != inspectionTypeID) {
			continue
		}
		if entry.Effect == permission.EffectDeny || len(entry.Ranges) == 0 {
			return result, nil
		}
		for _, clause := range entry.Ranges {
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
	} else if selected.GradeStage > 0 && semesterID != "" {
		err := tx.QueryRow(ctx, `SELECT g.name FROM academic_year_grade_stages m JOIN semesters sem ON sem.academic_year_id=m.academic_year_id JOIN grades g ON g.id=m.grade_id AND g.enabled WHERE sem.id=$1 AND m.stage=$2`, semesterID, selected.GradeStage).Scan(&grade)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
	}
	if grade != "" {
		result["grade"] = grade
	}
	return result, nil
}
