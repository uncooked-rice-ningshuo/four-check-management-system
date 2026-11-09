package store

import (
	"context"
	"encoding/json"
	"errors"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reporting"
	"four-check-management-system/server/internal/reportoutput"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) GetOutputSettings(ctx context.Context) (reportoutput.Settings, error) {
	var result reportoutput.Settings
	err := p.Pool.QueryRow(ctx, `SELECT introduction,word_type_codes,excel_type_codes,version,introduction_document_json::text FROM report_output_settings WHERE id=true`).Scan(&result.Introduction, &result.WordTypeCodes, &result.ExcelTypeCodes, &result.Version, &result.IntroductionDocument)
	return result, err
}

func (p *Postgres) ListWeekDates(ctx context.Context) ([]reportoutput.WeekDate, error) {
	rows, err := p.Pool.Query(ctx, `SELECT w.semester_id::text,s.name,w.id::text,w.week_no,COALESCE(d.issue_date::text,''),COALESCE(d.version,0)
		FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id
		LEFT JOIN report_output_week_dates d ON d.week_id=w.id
		ORDER BY w.start_date DESC,w.week_no DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reportoutput.WeekDate, 0)
	for rows.Next() {
		var item reportoutput.WeekDate
		if err := rows.Scan(&item.SemesterID, &item.SemesterName, &item.WeekID, &item.WeekNo, &item.IssueDate, &item.Version); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (p *Postgres) GetWeekDate(ctx context.Context, semesterID, weekID string) (reportoutput.WeekDate, error) {
	var item reportoutput.WeekDate
	err := p.Pool.QueryRow(ctx, `SELECT w.semester_id::text,s.name,w.id::text,w.week_no,COALESCE(d.issue_date::text,''),COALESCE(d.version,0)
		FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id
		LEFT JOIN report_output_week_dates d ON d.week_id=w.id
		WHERE w.id=$1::uuid AND w.semester_id=$2::uuid`, weekID, semesterID).Scan(&item.SemesterID, &item.SemesterName, &item.WeekID, &item.WeekNo, &item.IssueDate, &item.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return reportoutput.WeekDate{}, reportoutput.ErrWeekNotFound
	}
	return item, err
}

func (p *Postgres) SaveWeekDate(ctx context.Context, input reportoutput.WeekDate, metadata audit.Metadata) (reportoutput.WeekDate, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reportoutput.WeekDate{}, err
	}
	defer tx.Rollback(ctx)
	var semesterName string
	var weekNo int
	if err := tx.QueryRow(ctx, `SELECT s.name,w.week_no FROM semester_weeks w JOIN semesters s ON s.id=w.semester_id WHERE w.id=$1::uuid AND w.semester_id=$2::uuid FOR KEY SHARE OF w`, input.WeekID, input.SemesterID).Scan(&semesterName, &weekNo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return reportoutput.WeekDate{}, reportoutput.ErrWeekNotFound
		}
		return reportoutput.WeekDate{}, err
	}
	var beforeDate string
	var version int
	if input.Version > 0 {
		var beforeVersion int
		err = tx.QueryRow(ctx, `SELECT issue_date::text,version FROM report_output_week_dates WHERE week_id=$1::uuid FOR UPDATE`, input.WeekID).Scan(&beforeDate, &beforeVersion)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && beforeVersion != input.Version {
			return reportoutput.WeekDate{}, reportoutput.ErrConfigConflict
		}
		if err != nil {
			return reportoutput.WeekDate{}, err
		}
	}
	if input.Version == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO report_output_week_dates(week_id,semester_id,issue_date,updated_by) VALUES($1::uuid,$2::uuid,$3::date,$4::uuid) ON CONFLICT (week_id) DO NOTHING RETURNING version`, input.WeekID, input.SemesterID, input.IssueDate, metadata.ActorID).Scan(&version)
	} else {
		err = tx.QueryRow(ctx, `UPDATE report_output_week_dates SET issue_date=$3::date,version=version+1,updated_by=$4::uuid,updated_at=now() WHERE week_id=$1::uuid AND semester_id=$2::uuid AND version=$5 RETURNING version`, input.WeekID, input.SemesterID, input.IssueDate, metadata.ActorID, input.Version).Scan(&version)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return reportoutput.WeekDate{}, reportoutput.ErrConfigConflict
	}
	if err != nil {
		return reportoutput.WeekDate{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutputWeekDate", ResourceID: input.WeekID, Before: map[string]any{"version": input.Version, "date": beforeDate}, After: map[string]any{"version": version, "date": input.IssueDate}}); err != nil {
		return reportoutput.WeekDate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reportoutput.WeekDate{}, err
	}
	input.SemesterName, input.WeekNo, input.Version = semesterName, weekNo, version
	return input, nil
}

func (p *Postgres) ListOutputTypes(ctx context.Context) ([]reportoutput.OutputType, error) {
	rows, err := p.Pool.Query(ctx, `SELECT code,name FROM inspection_types WHERE enabled=true AND participates_in_report=true ORDER BY sort_order,code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := make([]reportoutput.OutputType, 0)
	for rows.Next() {
		var item reportoutput.OutputType
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, err
		}
		types = append(types, item)
	}
	return types, rows.Err()
}

// RecordOutputPreview serializes each actor's preview requests so the per-minute
// limit and audit append remain one transaction. The document body is never logged.
func (p *Postgres) RecordOutputPreview(ctx context.Context, codes []string, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, metadata.ActorID); err != nil {
		return err
	}
	var recent int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action=$2 AND created_at>now()-interval '1 minute'`, metadata.ActorID, string(audit.ActionReportOutputPreviewed)).Scan(&recent); err != nil {
		return err
	}
	if recent >= 10 {
		return reportoutput.ErrPreviewRateLimited
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputPreviewed, ResourceType: "reportOutputPreview", ResourceID: uuid.NewString(), After: map[string]any{"typeCodes": codes}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordTransientOutput audits a verified download without retaining the generated bytes.
func (p *Postgres) RecordTransientOutput(ctx context.Context, output reportoutput.File, size int, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutputDownload", ResourceID: output.ReportVersionID, After: map[string]any{"format": output.Format, "typeCodes": output.TypeCodes, "combined": output.Combined, "size": size, "settingsVersion": output.SettingsVersion}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) ReplaceOutputSettings(ctx context.Context, input reportoutput.Settings, metadata audit.Metadata) (reportoutput.Settings, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return reportoutput.Settings{}, err
	}
	defer tx.Rollback(ctx)
	var before reportoutput.Settings
	err = tx.QueryRow(ctx, `SELECT introduction,version FROM report_output_settings WHERE id=true FOR UPDATE`).Scan(&before.Introduction, &before.Version)
	if err != nil {
		return reportoutput.Settings{}, err
	}
	if before.Version != input.Version {
		return reportoutput.Settings{}, reportoutput.ErrConfigConflict
	}
	input.Version++
	_, err = tx.Exec(ctx, `UPDATE report_output_settings SET introduction=$1,introduction_document_json=$2::jsonb,word_type_codes=$3,excel_type_codes=$4,version=$5,updated_by=NULLIF($6,'')::uuid,updated_at=now() WHERE id=true`, input.Introduction, string(input.IntroductionDocument), input.WordTypeCodes, input.ExcelTypeCodes, input.Version, metadata.ActorID)
	if err != nil {
		return reportoutput.Settings{}, err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutputSettings", ResourceID: "singleton", Before: map[string]any{"version": before.Version}, After: map[string]any{"version": input.Version, "wordTypeCodes": input.WordTypeCodes, "excelTypeCodes": input.ExcelTypeCodes}}); err != nil {
		return reportoutput.Settings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reportoutput.Settings{}, err
	}
	return input, nil
}
func (p *Postgres) StartReportOutput(ctx context.Context, output reportoutput.File, actorID string, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentVersion int
	if err := tx.QueryRow(ctx, `SELECT version FROM report_output_settings WHERE id=true FOR SHARE`).Scan(&currentVersion); err != nil {
		return err
	}
	if currentVersion != output.SettingsVersion {
		return reportoutput.ErrConfigConflict
	}
	var snapshot any
	if len(output.IntroductionSnapshot) > 0 {
		snapshot = string(output.IntroductionSnapshot)
	}
	_, err = tx.Exec(ctx, `INSERT INTO report_output_files(id,report_version_id,format,type_codes,combined,status,created_by,settings_version,layout_version,introduction_document_snapshot_json,issue_date) VALUES($1::uuid,$2::uuid,$3,$4,$5,'PENDING',$6::uuid,$7,$8,$9::jsonb,NULLIF($10,'')::date)`, output.ID, output.ReportVersionID, output.Format, output.TypeCodes, output.Combined, actorID, output.SettingsVersion, output.LayoutVersion, snapshot, output.IssueDate)
	if err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutput", ResourceID: output.ID, After: output}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) FinishReportOutput(ctx context.Context, id, fileID string, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE report_output_files SET file_id=$2::uuid,status='AVAILABLE',updated_at=now() WHERE id=$1::uuid AND status='PENDING'`, id, fileID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return reportoutput.ErrUnavailable
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutput", ResourceID: id, After: map[string]any{"status": "AVAILABLE", "fileId": fileID}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) FailReportOutput(ctx context.Context, id, code string, metadata audit.Metadata) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE report_output_files SET status='FAILED',failure_code=$2,updated_at=now() WHERE id=$1::uuid AND status='PENDING'`, id, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return reportoutput.ErrUnavailable
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{Action: audit.ActionReportOutputChanged, ResourceType: "reportOutput", ResourceID: id, After: map[string]any{"status": "FAILED", "failureCode": code}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func scanReportOutput(row interface{ Scan(...any) error }) (reportoutput.File, error) {
	var out reportoutput.File
	err := row.Scan(&out.ID, &out.ReportVersionID, &out.Format, &out.TypeCodes, &out.Combined, &out.Status, &out.FileID, &out.FailureCode, &out.CreatedAt, &out.SettingsVersion, &out.LayoutVersion, &out.IssueDate)
	return out, err
}
func (p *Postgres) ListReportOutputs(ctx context.Context, versionID string) ([]reportoutput.File, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id::text,report_version_id::text,format,type_codes,combined,status,COALESCE(file_id::text,''),failure_code,created_at,COALESCE(settings_version,0),COALESCE(layout_version,''),COALESCE(issue_date::text,'') FROM report_output_files WHERE report_version_id=$1::uuid ORDER BY created_at DESC,id DESC`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outputs := make([]reportoutput.File, 0)
	for rows.Next() {
		out, err := scanReportOutput(rows)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	return outputs, rows.Err()
}
func (p *Postgres) GetReportOutput(ctx context.Context, id string) (reportoutput.File, error) {
	out, err := scanReportOutput(p.Pool.QueryRow(ctx, `SELECT id::text,report_version_id::text,format,type_codes,combined,status,COALESCE(file_id::text,''),failure_code,created_at,COALESCE(settings_version,0),COALESCE(layout_version,''),COALESCE(issue_date::text,'') FROM report_output_files WHERE id=$1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return reportoutput.File{}, reportoutput.ErrNotFound
	}
	return out, err
}
func (p *Postgres) LoadReportOutputRecords(ctx context.Context, versionID string) ([]reporting.Record, error) {
	rows, err := p.Pool.Query(ctx, `SELECT included_data_json::text FROM weekly_report_version_records WHERE report_version_id=$1::uuid ORDER BY inspection_record_id`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]reporting.Record, 0)
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var record reporting.Record
		if err := json.Unmarshal([]byte(encoded), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
