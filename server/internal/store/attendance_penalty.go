package store

import (
	"context"
	"errors"

	"four-check-management-system/server/internal/inspection"
	"github.com/jackc/pgx/v5"
)

// attendancePenaltyMultiplier locks the selected teaching week so a concurrent
// calendar edit cannot change the rule between validation and snapshot storage.
func attendancePenaltyMultiplier(ctx context.Context, tx pgx.Tx, weekID string) (int, error) {
	var multiplier *int
	err := tx.QueryRow(ctx, `SELECT attendance_penalty_multiplier FROM semester_weeks WHERE id=$1 FOR SHARE`, weekID).Scan(&multiplier)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, inspection.ErrInvalidAttendanceRecord
	}
	if err != nil {
		return 0, err
	}
	if multiplier == nil {
		return 0, inspection.ErrAttendancePenaltyUnconfigured
	}
	return *multiplier, nil
}
