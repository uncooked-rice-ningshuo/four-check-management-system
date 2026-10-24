package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/masterdataimport"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) FindMasterDataImportRecords(ctx context.Context, kind string, keys []masterdataimport.LookupKey, includeDisabled bool) ([]masterdata.Record, error) {
	if _, ok := masterdataimport.SpecFor(kind); !ok {
		return nil, masterdataimport.ErrInvalidKind
	}
	selectSQL, _, identifierColumn, nameColumn, ok := serverManagedMasterRecordSelect(kind)
	if !ok {
		return nil, masterdataimport.ErrInvalidKind
	}
	parentColumn := ""
	switch kind {
	case masterdata.KindMajor:
		parentColumn = "faculty_id"
	case masterdata.KindDormitoryBuilding:
		parentColumn = "dormitory_area_id"
	case masterdata.KindDormitory:
		parentColumn = "dormitory_building_id"
	case masterdata.KindClassroom:
		parentColumn = "teaching_building_id"
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		condition := ""
		if key.Code != "" {
			args = append(args, strings.ToUpper(strings.TrimSpace(key.Code)))
			condition = fmt.Sprintf("%s=$%d", identifierColumn, len(args))
		} else if key.RoomNo != "" && parentColumn != "" {
			args = append(args, key.ParentID, strings.TrimSpace(key.RoomNo))
			condition = fmt.Sprintf("%s=$%d AND room_no=$%d", parentColumn, len(args)-1, len(args))
		} else if key.Name != "" && key.ParentID != "" && parentColumn != "" {
			args = append(args, key.ParentID, strings.TrimSpace(key.Name))
			condition = fmt.Sprintf("%s=$%d AND name=$%d", parentColumn, len(args)-1, len(args))
		} else if key.Name != "" {
			column := nameColumn
			if kind == masterdata.KindDormitory || kind == masterdata.KindClassroom {
				column = "room_no"
			}
			args = append(args, strings.TrimSpace(key.Name))
			condition = fmt.Sprintf("%s=$%d", column, len(args))
		}
		if condition != "" {
			clauses = append(clauses, "("+condition+")")
		}
	}
	if len(clauses) == 0 {
		return []masterdata.Record{}, nil
	}
	where := "(" + strings.Join(clauses, " OR ") + ")"
	if !includeDisabled {
		where += " AND enabled"
	}
	rows, err := p.Pool.Query(ctx, selectSQL+` WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]masterdata.Record, 0)
	for rows.Next() {
		record, scanErr := scanMasterRecord(rows, kind)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (p *Postgres) EnabledDictionaryOptions(ctx context.Context, code string) ([]masterdataimport.DictionaryOption, error) {
	rows, err := p.Pool.Query(ctx, `SELECT i.value, i.label
		FROM dictionary_items i JOIN dictionary_types t ON t.id=i.type_id
		WHERE t.code=$1 AND t.enabled AND i.enabled ORDER BY i.sort_order,i.value`, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := make([]masterdataimport.DictionaryOption, 0)
	for rows.Next() {
		var option masterdataimport.DictionaryOption
		if err := rows.Scan(&option.Value, &option.Label); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func (p *Postgres) CreateMasterDataImportBatch(ctx context.Context, batch masterdataimport.Batch, metadata audit.Metadata) error {
	preview, err := json.Marshal(batch.Rows)
	if err != nil {
		return fmt.Errorf("encode master data import preview: %w", err)
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO master_data_import_batches(
		id,kind,source_file_id,created_by,template_version,status,total_rows,valid_rows,invalid_rows,version,preview_json,created_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, batch.ID, batch.Kind, batch.SourceFileID, batch.CreatedBy,
		batch.TemplateVersion, batch.Status, batch.TotalRows, batch.ValidRows, batch.InvalidRows, batch.Version, preview, batch.CreatedAt)
	if err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, metadata, audit.Entry{
		Action: audit.ActionImportBatchPreviewed, ResourceType: "master_data_import_batch", ResourceID: batch.ID,
		After: map[string]any{"kind": batch.Kind, "status": batch.Status, "totalRows": batch.TotalRows, "validRows": batch.ValidRows, "invalidRows": batch.InvalidRows},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) FindMasterDataImportBatch(ctx context.Context, kind, id, actorID string) (masterdataimport.Batch, error) {
	var batch masterdataimport.Batch
	var preview []byte
	var confirmedAt sql.NullTime
	err := p.Pool.QueryRow(ctx, `SELECT id::text,kind,status,template_version,source_file_id::text,total_rows,valid_rows,invalid_rows,
		version,preview_json,created_by::text,created_at,confirmed_at
		FROM master_data_import_batches WHERE id=$1 AND kind=$2 AND created_by=$3`, id, kind, actorID).
		Scan(&batch.ID, &batch.Kind, &batch.Status, &batch.TemplateVersion, &batch.SourceFileID, &batch.TotalRows, &batch.ValidRows,
			&batch.InvalidRows, &batch.Version, &preview, &batch.CreatedBy, &batch.CreatedAt, &confirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return masterdataimport.Batch{}, masterdataimport.ErrBatchNotFound
	}
	if err != nil {
		return masterdataimport.Batch{}, err
	}
	if err := json.Unmarshal(preview, &batch.Rows); err != nil {
		return masterdataimport.Batch{}, fmt.Errorf("decode master data import preview: %w", err)
	}
	if confirmedAt.Valid {
		batch.ConfirmedAt = &confirmedAt.Time
	}
	return batch, nil
}

func (p *Postgres) ConfirmMasterDataImportBatch(ctx context.Context, input masterdataimport.ConfirmInput) (masterdataimport.ConfirmResult, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return masterdataimport.ConfirmResult{}, err
	}
	defer tx.Rollback(ctx)
	var batch masterdataimport.Batch
	var preview []byte
	err = tx.QueryRow(ctx, `SELECT id::text,kind,status,total_rows,valid_rows,invalid_rows,version,preview_json,
		created_by::text,confirm_idempotency_key,template_version
		FROM master_data_import_batches WHERE id=$1 AND kind=$2 AND created_by=$3 FOR UPDATE`,
		input.BatchID, input.Kind, input.ActorID).Scan(&batch.ID, &batch.Kind, &batch.Status, &batch.TotalRows, &batch.ValidRows,
		&batch.InvalidRows, &batch.Version, &preview, &batch.CreatedBy, &batch.ConfirmIdempotencyKey, &batch.TemplateVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return masterdataimport.ConfirmResult{}, masterdataimport.ErrBatchNotFound
	}
	if err != nil {
		return masterdataimport.ConfirmResult{}, err
	}
	if batch.Status == masterdataimport.StatusConfirmed {
		if batch.ConfirmIdempotencyKey != input.IdempotencyKey {
			return masterdataimport.ConfirmResult{}, masterdataimport.ErrAlreadyConfirmed
		}
		return masterdataimport.ConfirmResult{BatchID: batch.ID, Kind: batch.Kind, Status: batch.Status, ImportedRows: batch.ValidRows, Version: batch.Version}, nil
	}
	if batch.Version != input.ExpectedVersion {
		return masterdataimport.ConfirmResult{}, masterdataimport.ErrBatchVersionConflict
	}
	if batch.Status != masterdataimport.StatusPreviewReady || batch.InvalidRows > 0 {
		return masterdataimport.ConfirmResult{}, masterdataimport.ErrBatchHasErrors
	}
	if err := json.Unmarshal(preview, &batch.Rows); err != nil {
		return masterdataimport.ConfirmResult{}, fmt.Errorf("decode master data import preview: %w", err)
	}
	if len(batch.Rows) != batch.ValidRows || len(batch.Rows) != batch.TotalRows {
		return masterdataimport.ConfirmResult{}, masterdataimport.ErrPreviewStale
	}
	for _, row := range batch.Rows {
		if row.Status != masterdataimport.RowValid || len(row.Errors) != 0 || row.Record == nil || row.Record.Kind != batch.Kind || !row.Record.Enabled || row.Record.ID == "" {
			return masterdataimport.ConfirmResult{}, masterdataimport.ErrPreviewStale
		}
		if err := validateImportRelationsTx(ctx, tx, *row.Record); err != nil {
			return masterdataimport.ConfirmResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE master_data_import_batches SET status='CONFIRMED',confirm_idempotency_key=$2,
		version=version+1,confirmed_at=now() WHERE id=$1`, batch.ID, input.IdempotencyKey); err != nil {
		return masterdataimport.ConfirmResult{}, err
	}
	for _, row := range batch.Rows {
		record := *row.Record
		if err := insertMasterRecord(ctx, tx, record); err != nil {
			return masterdataimport.ConfirmResult{}, masterDataError(err)
		}
		after, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return masterdataimport.ConfirmResult{}, marshalErr
		}
		if err := insertMasterChange(ctx, tx, input.ActorID, record.Kind, record.ID, "CREATE", nil, after); err != nil {
			return masterdataimport.ConfirmResult{}, err
		}
		if err := insertAuditTx(ctx, tx, input.Metadata, audit.Entry{
			Action: audit.ActionMasterDataChanged, ResourceType: "masterData", ResourceID: record.ID,
			After: record,
		}); err != nil {
			return masterdataimport.ConfirmResult{}, err
		}
	}
	if err := insertAuditTx(ctx, tx, input.Metadata, audit.Entry{
		Action: audit.ActionImportBatchConfirmed, ResourceType: "master_data_import_batch", ResourceID: batch.ID,
		Before: map[string]any{"status": masterdataimport.StatusPreviewReady, "version": batch.Version},
		After:  map[string]any{"kind": batch.Kind, "status": masterdataimport.StatusConfirmed, "importedRows": batch.ValidRows, "templateVersion": batch.TemplateVersion},
	}); err != nil {
		return masterdataimport.ConfirmResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return masterdataimport.ConfirmResult{}, err
	}
	return masterdataimport.ConfirmResult{BatchID: batch.ID, Kind: batch.Kind, Status: masterdataimport.StatusConfirmed, ImportedRows: batch.ValidRows, Version: batch.Version + 1}, nil
}

func validateImportRelationsTx(ctx context.Context, tx pgx.Tx, record masterdata.Record) error {
	references := []struct {
		kind, id string
		required bool
	}{
		{masterdata.KindFaculty, record.FacultyID, record.Kind == masterdata.KindMajor},
		{masterdata.KindMajor, record.MajorID, false},
		{masterdata.KindStaff, record.HeadTeacherID, false},
		{masterdata.KindClass, record.ClassID, false},
		{masterdata.KindDormitory, record.DormitoryID, false},
		{masterdata.KindTeachingBuilding, record.TeachingBuildingID, record.Kind == masterdata.KindClassroom},
		{masterdata.KindGrade, record.GradeID, record.Kind == masterdata.KindClass || record.Kind == masterdata.KindStudent},
		{masterdata.KindDormitoryArea, record.DormitoryAreaID, record.Kind == masterdata.KindDormitoryBuilding},
		{masterdata.KindDormitoryBuilding, record.DormitoryBuildingID, record.Kind == masterdata.KindDormitory},
	}
	for _, reference := range references {
		if reference.id == "" {
			if reference.required {
				return masterdataimport.ErrPreviewStale
			}
			continue
		}
		if err := lockEnabledImportReference(ctx, tx, reference.kind, reference.id); err != nil {
			return err
		}
	}
	if record.Kind == masterdata.KindStudent && record.ClassID != "" {
		var gradeID string
		var facultyID, majorID string
		err := tx.QueryRow(ctx, `SELECT grade_id::text,COALESCE(faculty_id::text,''),COALESCE(major_id::text,'') FROM classes WHERE id=$1 AND enabled FOR SHARE`, record.ClassID).
			Scan(&gradeID, &facultyID, &majorID)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (gradeID != record.GradeID || facultyID != "" && record.FacultyID != "" && facultyID != record.FacultyID || majorID != "" && record.MajorID != "" && majorID != record.MajorID) {
			return masterdataimport.ErrPreviewStale
		}
		if err != nil {
			return err
		}
	}
	if (record.Kind == masterdata.KindClass || record.Kind == masterdata.KindStudent) && record.MajorID != "" {
		var facultyID string
		err := tx.QueryRow(ctx, `SELECT faculty_id::text FROM majors WHERE id=$1 AND enabled FOR SHARE`, record.MajorID).Scan(&facultyID)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && record.FacultyID != "" && facultyID != record.FacultyID {
			return masterdataimport.ErrPreviewStale
		}
		if err != nil {
			return err
		}
	}
	if record.Kind == masterdata.KindStaff || record.Kind == masterdata.KindStudent {
		if err := lockEnabledImportDictionaryValue(ctx, tx, "GENDER", record.Gender); err != nil {
			return err
		}
	}
	if record.Kind == masterdata.KindStudent {
		if err := lockEnabledImportDictionaryValue(ctx, tx, "ENROLLMENT_STATUS", record.EnrollmentStatus); err != nil {
			return err
		}
	}
	return nil
}

func lockEnabledImportDictionaryValue(ctx context.Context, tx pgx.Tx, typeCode, value string) error {
	var found string
	err := tx.QueryRow(ctx, `SELECT i.value FROM dictionary_items i JOIN dictionary_types t ON t.id=i.type_id
		WHERE t.code=$1 AND t.enabled AND i.enabled AND i.value=$2 FOR SHARE OF t,i`, typeCode, value).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return masterdataimport.ErrPreviewStale
	}
	return err
}

func lockEnabledImportReference(ctx context.Context, tx pgx.Tx, kind, id string) error {
	tables := map[string]string{
		masterdata.KindFaculty: "faculties", masterdata.KindMajor: "majors", masterdata.KindStaff: "staff",
		masterdata.KindClass: "classes", masterdata.KindDormitory: "dormitories", masterdata.KindTeachingBuilding: "teaching_buildings",
		masterdata.KindGrade: "grades", masterdata.KindDormitoryArea: "dormitory_areas", masterdata.KindDormitoryBuilding: "dormitory_buildings",
	}
	table, ok := tables[kind]
	if !ok {
		return masterdataimport.ErrPreviewStale
	}
	var found string
	err := tx.QueryRow(ctx, `SELECT id::text FROM `+table+` WHERE id=$1 AND enabled FOR SHARE`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return masterdataimport.ErrPreviewStale
	}
	return err
}
