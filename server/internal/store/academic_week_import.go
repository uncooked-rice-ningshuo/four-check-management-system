package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/academicimport"
	"four-check-management-system/server/internal/audit"
	"github.com/jackc/pgx/v5"
)

const academicWeekWriteLockKey = "academic_semester_weeks_write_lock"

func lockAcademicWeekWrites(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, academicWeekWriteLockKey)
	return err
}

func validateAcademicWeekWrite(ctx context.Context, tx pgx.Tx, value academic.Week, excludeID string) error {
	var semesterStart, semesterEnd sql.NullTime
	err := tx.QueryRow(ctx, `SELECT start_date,end_date FROM semesters WHERE id=$1 FOR UPDATE`, value.SemesterID).Scan(&semesterStart, &semesterEnd)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!semesterStart.Valid || !semesterEnd.Valid || value.StartDate.Before(semesterStart.Time) || value.EndDate.After(semesterEnd.Time))) {
		return academic.ErrInvalidInput
	}
	if err != nil {
		return err
	}
	var duplicateNumber bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semester_weeks WHERE semester_id=$1 AND week_no=$2 AND id<>COALESCE(NULLIF($3,'')::uuid,'00000000-0000-0000-0000-000000000000'::uuid))`, value.SemesterID, value.WeekNo, excludeID).Scan(&duplicateNumber); err != nil {
		return err
	}
	if duplicateNumber {
		return academic.ErrInvalidInput
	}
	if !value.Enabled {
		return nil
	}
	var overlaps bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semester_weeks WHERE enabled AND id<>COALESCE(NULLIF($1,'')::uuid,'00000000-0000-0000-0000-000000000000'::uuid) AND start_date <= $3 AND end_date >= $2)`, excludeID, value.StartDate, value.EndDate).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps {
		return academic.ErrDateOverlap
	}
	return nil
}

func (p *Postgres) CreateAcademicWeekImportBatch(ctx context.Context, batch academicimport.Batch, metadata audit.Metadata) error {
	preview, err := json.Marshal(batch.Rows)
	if err != nil {
		return fmt.Errorf("encode academic week import preview: %w", err)
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO academic_week_import_batches(
		id,semester_id,semester_name,academic_year_name,source_file_id,created_by,template_version,status,total_rows,
		valid_rows,invalid_rows,affects_today,version,preview_json,created_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, batch.ID, batch.SemesterID,
		batch.SemesterName, batch.AcademicYearName, batch.SourceFileID, batch.CreatedBy, batch.TemplateVersion,
		batch.Status, batch.TotalRows, batch.ValidRows, batch.InvalidRows, batch.AffectsToday, batch.Version, preview, batch.CreatedAt)
	if err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionImportBatchPreviewed, ResourceType: "academic_week_import_batch", ResourceID: batch.ID,
		After: map[string]any{"semesterId": batch.SemesterID, "status": batch.Status, "totalRows": batch.TotalRows, "validRows": batch.ValidRows, "invalidRows": batch.InvalidRows, "templateVersion": batch.TemplateVersion},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) FindAcademicWeekImportBatch(ctx context.Context, id, actorID string) (academicimport.Batch, error) {
	var batch academicimport.Batch
	var preview []byte
	var confirmedAt sql.NullTime
	err := p.Pool.QueryRow(ctx, `SELECT id::text,status,version,template_version,semester_id::text,semester_name,academic_year_name,
		total_rows,valid_rows,invalid_rows,affects_today,preview_json,created_at,confirmed_at
		FROM academic_week_import_batches WHERE id=$1 AND created_by=$2`, id, actorID).Scan(
		&batch.ID, &batch.Status, &batch.Version, &batch.TemplateVersion, &batch.SemesterID, &batch.SemesterName, &batch.AcademicYearName,
		&batch.TotalRows, &batch.ValidRows, &batch.InvalidRows, &batch.AffectsToday, &preview, &batch.CreatedAt, &confirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return academicimport.Batch{}, academicimport.ErrBatchNotFound
	}
	if err != nil {
		return academicimport.Batch{}, err
	}
	if err := json.Unmarshal(preview, &batch.Rows); err != nil {
		return academicimport.Batch{}, fmt.Errorf("decode academic week import preview: %w", err)
	}
	if confirmedAt.Valid {
		batch.ConfirmedAt = &confirmedAt.Time
	}
	return batch, nil
}

func (p *Postgres) ConfirmAcademicWeekImportBatch(ctx context.Context, input academicimport.ConfirmInput) (academicimport.ConfirmResult, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return academicimport.ConfirmResult{}, err
	}
	defer tx.Rollback(ctx)
	var batch academicimport.Batch
	var preview []byte
	err = tx.QueryRow(ctx, `SELECT id::text,semester_id::text,status,total_rows,valid_rows,invalid_rows,version,preview_json,
		created_by::text,confirm_idempotency_key,template_version
		FROM academic_week_import_batches WHERE id=$1 AND created_by=$2 FOR UPDATE`, input.BatchID, input.ActorID).Scan(
		&batch.ID, &batch.SemesterID, &batch.Status, &batch.TotalRows, &batch.ValidRows, &batch.InvalidRows, &batch.Version,
		&preview, &batch.CreatedBy, &batch.ConfirmIdempotencyKey, &batch.TemplateVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return academicimport.ConfirmResult{}, academicimport.ErrBatchNotFound
	}
	if err != nil {
		return academicimport.ConfirmResult{}, err
	}
	if batch.Status == academicimport.StatusConfirmed {
		if batch.ConfirmIdempotencyKey != input.IdempotencyKey {
			return academicimport.ConfirmResult{}, academicimport.ErrAlreadyConfirmed
		}
		return academicimport.ConfirmResult{BatchID: batch.ID, Status: batch.Status, ImportedRows: batch.ValidRows, Version: batch.Version}, nil
	}
	if batch.Version != input.ExpectedVersion {
		return academicimport.ConfirmResult{}, academicimport.ErrBatchVersionConflict
	}
	if batch.Status != academicimport.StatusPreviewReady || batch.InvalidRows != 0 {
		return academicimport.ConfirmResult{}, academicimport.ErrBatchHasErrors
	}
	if err := json.Unmarshal(preview, &batch.Rows); err != nil {
		return academicimport.ConfirmResult{}, fmt.Errorf("decode academic week import preview: %w", err)
	}
	if err := validateAcademicWeekImportPreview(batch); err != nil {
		return academicimport.ConfirmResult{}, err
	}
	if err := lockAcademicWeekWrites(ctx, tx); err != nil {
		return academicimport.ConfirmResult{}, err
	}
	var semesterStart, semesterEnd sql.NullTime
	err = tx.QueryRow(ctx, `SELECT s.start_date,s.end_date FROM semesters s JOIN academic_years ay ON ay.id=s.academic_year_id
		WHERE s.id=$1 FOR UPDATE OF s,ay`, batch.SemesterID).Scan(&semesterStart, &semesterEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return academicimport.ConfirmResult{}, academicimport.ErrPreviewStale
	}
	if err != nil {
		return academicimport.ConfirmResult{}, err
	}
	for index, row := range batch.Rows {
		week := *row.Week
		if !semesterStart.Valid || !semesterEnd.Valid || week.StartDate.Before(semesterStart.Time) || week.EndDate.After(semesterEnd.Time) {
			return academicimport.ConfirmResult{}, academicimport.ErrPreviewStale
		}
		if err := validateWeekAgainstDatabase(ctx, tx, batch.SemesterID, week, batch.Rows[:index]); err != nil {
			return academicimport.ConfirmResult{}, err
		}
	}
	for _, row := range batch.Rows {
		week := *row.Week
		_, err := tx.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date,enabled,note,attendance_penalty_multiplier)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, week.ID, week.SemesterID, week.WeekNo, week.StartDate, week.EndDate,
			week.Enabled, week.Note, week.AttendancePenaltyMultiplier)
		if err != nil {
			return academicimport.ConfirmResult{}, err
		}
		if err := insertAuditTx(ctx, tx, input.Metadata, audit.Entry{
			Action: audit.ActionAcademicCalendarChanged, ResourceType: "semesterWeek", ResourceID: week.ID, After: week,
		}); err != nil {
			return academicimport.ConfirmResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE academic_week_import_batches SET status='CONFIRMED',confirm_idempotency_key=$2,version=version+1,confirmed_at=now() WHERE id=$1`, batch.ID, input.IdempotencyKey); err != nil {
		return academicimport.ConfirmResult{}, err
	}
	if err := insertAuditTx(ctx, tx, input.Metadata, audit.Entry{
		Action: audit.ActionImportBatchConfirmed, ResourceType: "academic_week_import_batch", ResourceID: batch.ID,
		Before: map[string]any{"status": batch.Status, "version": batch.Version},
		After:  map[string]any{"semesterId": batch.SemesterID, "status": academicimport.StatusConfirmed, "importedRows": batch.ValidRows, "templateVersion": batch.TemplateVersion},
	}); err != nil {
		return academicimport.ConfirmResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return academicimport.ConfirmResult{}, err
	}
	return academicimport.ConfirmResult{BatchID: batch.ID, Status: academicimport.StatusConfirmed, ImportedRows: batch.ValidRows, Version: batch.Version + 1}, nil
}

func validateAcademicWeekImportPreview(batch academicimport.Batch) error {
	if batch.TemplateVersion != academicimport.TemplateVersion || batch.Version < 1 || batch.TotalRows < 1 ||
		batch.TotalRows != batch.ValidRows || batch.TotalRows != len(batch.Rows) || batch.InvalidRows != 0 {
		return academicimport.ErrPreviewStale
	}
	seenIDs := make(map[string]struct{}, len(batch.Rows))
	for _, row := range batch.Rows {
		if row.Status != academicimport.RowValid || len(row.Errors) != 0 || row.Week == nil || row.Week.ID == "" ||
			row.Week.SemesterID != batch.SemesterID || !row.Week.Enabled || row.Week.WeekNo < 1 || row.Week.WeekNo > 60 ||
			row.Week.StartDate.IsZero() || row.Week.EndDate.IsZero() || row.Week.StartDate.After(row.Week.EndDate) ||
			utf8.RuneCountInString(row.Week.Note) > 255 ||
			row.Week.AttendancePenaltyMultiplier == nil || (*row.Week.AttendancePenaltyMultiplier != 1 && *row.Week.AttendancePenaltyMultiplier != 2) {
			return academicimport.ErrPreviewStale
		}
		if _, exists := seenIDs[row.Week.ID]; exists {
			return academicimport.ErrPreviewStale
		}
		seenIDs[row.Week.ID] = struct{}{}
	}
	return nil
}

func validateWeekAgainstDatabase(ctx context.Context, tx pgx.Tx, semesterID string, week academic.Week, earlier []academicimport.PreviewRow) error {
	for _, row := range earlier {
		if row.Week == nil {
			return academicimport.ErrPreviewStale
		}
		if row.Week.WeekNo == week.WeekNo || (!week.EndDate.Before(row.Week.StartDate) && !row.Week.EndDate.Before(week.StartDate)) {
			return academicimport.ErrPreviewStale
		}
	}
	var duplicateNumber, overlap bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semester_weeks WHERE semester_id=$1 AND week_no=$2)`, semesterID, week.WeekNo).Scan(&duplicateNumber); err != nil {
		return err
	}
	if duplicateNumber {
		return academicimport.ErrPreviewStale
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semester_weeks WHERE enabled AND start_date <= $2 AND end_date >= $1)`, week.StartDate, week.EndDate).Scan(&overlap); err != nil {
		return err
	}
	if overlap {
		return academicimport.ErrPreviewStale
	}
	return nil
}
