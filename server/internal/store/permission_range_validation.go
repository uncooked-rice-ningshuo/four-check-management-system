package store

import (
	"context"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

// validateRangeClasses checks fixed dimensions against authoritative class relationships.
func validateRangeClasses(ctx context.Context, tx pgx.Tx, ranges []permission.OperationRange) error {
	for _, item := range ranges {
		if item.ClassID == "" {
			continue
		}
		var matches bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM classes WHERE id=$1::uuid AND ($2='' OR grade_id::text=$2) AND ($3='' OR major_id::text=$3))`, item.ClassID, item.GradeID, item.MajorID).Scan(&matches); err != nil {
			return err
		}
		if !matches {
			return permission.ErrOperationRangeConflict
		}
	}
	return nil
}
