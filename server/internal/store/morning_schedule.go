package store

import (
	"context"
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) LoadMorningSchedule(ctx context.Context, weekID string) (inspection.MorningSchedule, permission.Scope, error) {
	var schedule inspection.MorningSchedule
	var scope permission.Scope
	var dates []time.Time
	var updatedAt *time.Time
	err := p.Pool.QueryRow(ctx, `
SELECT sem.id::text,w.id::text,w.start_date::text,w.end_date::text,it.id::text,d.id::text,
       COALESCE(s.planned_dates,ARRAY[]::date[]),COALESCE(s.version,0),
       COALESCE(s.updated_by::text,''),s.updated_at
FROM semester_weeks w
JOIN semesters sem ON sem.id=w.semester_id
JOIN academic_years y ON y.id=sem.academic_year_id
JOIN inspection_types it ON it.code='MORNING_ATTENDANCE' AND it.enabled
JOIN departments d ON d.code='SPORTS' AND d.enabled
LEFT JOIN morning_attendance_week_schedules s ON s.week_id=w.id
WHERE w.id=$1::uuid AND w.enabled AND sem.enabled AND y.enabled`, weekID).Scan(
		&schedule.SemesterID, &schedule.WeekID, &schedule.StartDate, &schedule.EndDate,
		&scope.InspectionTypeID, &scope.DepartmentID, &dates, &schedule.Version, &schedule.UpdatedBy, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return inspection.MorningSchedule{}, permission.Scope{}, inspection.ErrInvalidAttendanceRecord
	}
	if err != nil {
		return inspection.MorningSchedule{}, permission.Scope{}, err
	}
	scope.SemesterID, scope.WeekID = schedule.SemesterID, schedule.WeekID
	if schedule.Version == 0 {
		schedule.PlannedDates = inspection.DefaultMorningPlannedDates(schedule.StartDate, schedule.EndDate)
		schedule.IsDefault = true
	} else {
		schedule.PlannedDates = make([]string, 0, len(dates))
		for _, date := range dates {
			schedule.PlannedDates = append(schedule.PlannedDates, date.Format("2006-01-02"))
		}
	}
	if updatedAt != nil {
		schedule.UpdatedAt = *updatedAt
	}
	return schedule, scope, nil
}

func (p *Postgres) SaveMorningSchedule(ctx context.Context, weekID string, expectedVersion int, plannedDates []string, actorID string, metadata audit.Metadata) (inspection.MorningSchedule, error) {
	parsed := make([]time.Time, 0, len(plannedDates))
	for _, value := range plannedDates {
		date, err := time.Parse("2006-01-02", value)
		if err != nil {
			return inspection.MorningSchedule{}, inspection.ErrInspectionRequest
		}
		parsed = append(parsed, date)
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return inspection.MorningSchedule{}, err
	}
	defer tx.Rollback(ctx)
	var currentVersion int
	var beforeDates []time.Time
	err = tx.QueryRow(ctx, `SELECT COALESCE(s.version,0),COALESCE(s.planned_dates,ARRAY[]::date[])
FROM semester_weeks w LEFT JOIN morning_attendance_week_schedules s ON s.week_id=w.id
WHERE w.id=$1::uuid AND w.enabled FOR UPDATE OF w`, weekID).Scan(&currentVersion, &beforeDates)
	if errors.Is(err, pgx.ErrNoRows) {
		return inspection.MorningSchedule{}, inspection.ErrInvalidAttendanceRecord
	}
	if err != nil {
		return inspection.MorningSchedule{}, err
	}
	if currentVersion != expectedVersion {
		return inspection.MorningSchedule{}, inspection.ErrMorningScheduleVersionConflict
	}
	if expectedVersion == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO morning_attendance_week_schedules(week_id,planned_dates,version,updated_by)
VALUES($1::uuid,$2::date[],1,$3::uuid)`, weekID, parsed, actorID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE morning_attendance_week_schedules
SET planned_dates=$2::date[],version=version+1,updated_by=$3::uuid,updated_at=now()
WHERE week_id=$1::uuid AND version=$4`, weekID, parsed, actorID, expectedVersion)
	}
	if err != nil {
		return inspection.MorningSchedule{}, err
	}
	before := make([]string, 0, len(beforeDates))
	for _, date := range beforeDates {
		before = append(before, date.Format("2006-01-02"))
	}
	if expectedVersion == 0 {
		before = nil
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionMorningScheduleChanged, ResourceType: "morningAttendanceWeekSchedule", ResourceID: weekID,
		Before: map[string]any{"version": expectedVersion, "plannedDates": before},
		After:  map[string]any{"version": expectedVersion + 1, "plannedDates": plannedDates},
	}); err != nil {
		return inspection.MorningSchedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return inspection.MorningSchedule{}, err
	}
	schedule, _, err := p.LoadMorningSchedule(ctx, weekID)
	return schedule, err
}
