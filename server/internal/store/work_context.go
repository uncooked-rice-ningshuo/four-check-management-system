package store

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"github.com/jackc/pgx/v5"
	"time"
)

const workWeekSQL = `SELECT w.id,w.semester_id,w.week_no,w.start_date,w.end_date,w.enabled,w.note FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id JOIN academic_years y ON y.id=s.academic_year_id WHERE w.enabled AND s.enabled AND y.enabled AND `

func scanWorkWeek(row pgx.Row) (*academic.Week, error) {
	var w academic.Week
	e := row.Scan(&w.ID, &w.SemesterID, &w.WeekNo, &w.StartDate, &w.EndDate, &w.Enabled, &w.Note)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &w, e
}
func (p *Postgres) ReadWorkContext(ctx context.Context, now time.Time) (academic.WorkContextSnapshot, error) {
	var result academic.WorkContextSnapshot
	tx, e := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return result, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if e = tx.QueryRow(ctx, `SELECT value_json,row_version FROM system_settings WHERE key='work_week_override'`).Scan(&raw, &result.Version); e != nil {
		return result, e
	}
	var setting struct {
		WeekID    string     `json:"weekId"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if e = json.Unmarshal(raw, &setting); e != nil {
		return result, e
	}
	result.ExpiresAt = setting.ExpiresAt
	result.Automatic, e = scanWorkWeek(tx.QueryRow(ctx, workWeekSQL+`($1::timestamptz AT TIME ZONE 'Asia/Shanghai')::date BETWEEN w.start_date AND w.end_date ORDER BY w.start_date,w.id LIMIT 1`, now))
	if e != nil {
		return result, e
	}
	if setting.WeekID != "" && setting.ExpiresAt != nil && now.Before(*setting.ExpiresAt) {
		result.Override, e = scanWorkWeek(tx.QueryRow(ctx, workWeekSQL+`w.id=$1::uuid`, setting.WeekID))
		if e != nil {
			return result, e
		}
	}
	return result, tx.Commit(ctx)
}
func (p *Postgres) SaveWeekOverride(ctx context.Context, i academic.WeekOverrideInput, now time.Time, m audit.Metadata) error {
	tx, e := p.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var version int
	var old json.RawMessage
	if e = tx.QueryRow(ctx, `SELECT row_version,value_json FROM system_settings WHERE key='work_week_override' FOR UPDATE`).Scan(&version, &old); e != nil {
		return e
	}
	if version != i.Version {
		return academic.ErrWorkContextConflict
	}
	if i.WeekID != "" {
		w, err := scanWorkWeek(tx.QueryRow(ctx, workWeekSQL+`w.id=$1::uuid FOR SHARE OF w,s,y`, i.WeekID))
		if err != nil {
			return err
		}
		if w == nil || i.ExpiresAt == nil || !i.ExpiresAt.After(now) {
			return academic.ErrInvalidInput
		}
	}
	next := struct {
		WeekID    string     `json:"weekId,omitempty"`
		ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	}{i.WeekID, i.ExpiresAt}
	raw, e := json.Marshal(next)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE system_settings SET value_json=$1,row_version=row_version+1,updated_by=$2,updated_at=now() WHERE key='work_week_override' AND row_version=$3`, raw, m.ActorID, i.Version); e != nil {
		return e
	}
	if e = insertAuditTx(ctx, tx, m, audit.Entry{Action: audit.ActionAcademicCalendarChanged, ResourceType: "workContext", ResourceID: "work_week_override", Before: old, After: next}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
