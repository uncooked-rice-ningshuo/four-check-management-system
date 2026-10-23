package store

import (
	"context"
	"encoding/json"
	"errors"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CheckTemplatePublisher restricts the local operational command to an active
// super administrator; a selected account ID is an audit attribution, not a new API.
func (p *Postgres) CheckTemplatePublisher(ctx context.Context, actor string) error {
	if _, err := uuid.Parse(actor); err != nil {
		return auth.ErrForbidden
	}
	var allowed bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND status='ACTIVE' AND is_super_admin)`, actor).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

func scanPublication(rows pgx.Rows) (importjob.Template, error) {
	defer rows.Close()
	var result importjob.Template
	count := 0
	for rows.Next() {
		count++
		if err := rows.Scan(&result.ID, &result.Code, &result.Name, &result.InspectionTypeID, &result.Version, &result.FileID, &result.Status); err != nil {
			return result, err
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if count == 0 {
		return result, importjob.ErrTemplateNotFound
	}
	if count != 1 {
		return result, importjob.ErrTemplatePublicationConflict
	}
	return result, nil
}

const inspectionPublicationQuery = `SELECT t.id,t.code,t.name,t.inspection_type_id,t.version_no,t.file_id,t.status FROM import_templates t JOIN inspection_types i ON i.id=t.inspection_type_id WHERE (i.code=$1 AND (t.version_no=1 OR t.status='PUBLISHED')) OR (t.code=$2 AND t.version_no=1)`

func (p *Postgres) FindInspectionTemplate(ctx context.Context, typeCode string) (importjob.Template, error) {
	definition, err := importjob.InspectionTemplate(typeCode)
	if err != nil {
		return importjob.Template{}, err
	}
	rows, err := p.Pool.Query(ctx, inspectionPublicationQuery, typeCode, definition.Code)
	if err != nil {
		return importjob.Template{}, err
	}
	result, err := scanPublication(rows)
	if err != nil {
		return result, err
	}
	var validType bool
	if err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM inspection_types WHERE id=$1 AND code=$2 AND enabled)`, result.InspectionTypeID, typeCode).Scan(&validType); err != nil {
		return importjob.Template{}, err
	}
	if !validType {
		return importjob.Template{}, importjob.ErrTemplatePublicationConflict
	}
	return result, nil
}

func (p *Postgres) FindReusableTemplateFile(ctx context.Context, digest string) (storage.File, error) {
	var id string
	err := p.Pool.QueryRow(ctx, `SELECT id FROM files WHERE retention_type='TEMPLATE' AND status='AVAILABLE' AND sha256=$1 ORDER BY created_at,id LIMIT 1`, digest).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.File{}, storage.ErrFileNotFound
	}
	if err != nil {
		return storage.File{}, err
	}
	return p.FindFile(ctx, id)
}

// PublishInspectionTemplate serializes competing publishers and atomically writes
// the template and audit. No historical template is retired or overwritten here.
func (p *Postgres) PublishInspectionTemplate(ctx context.Context, typeCode string, file storage.File, fields []importjob.TemplateField, metadata audit.Metadata) (importjob.Template, error) {
	definition, err := importjob.InspectionTemplate(typeCode)
	if err != nil {
		return importjob.Template{}, err
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return importjob.Template{}, err
	}
	defer tx.Rollback(ctx)
	lockKey := typeCode + "-import-template-v1"
	if typeCode == "ATTENDANCE_RATE" {
		// Keep the lock shared with already deployed attendance publishers.
		lockKey = "attendance-import-template-v1"
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return importjob.Template{}, err
	}
	var actor string
	if err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE id=$1 AND status='ACTIVE' AND is_super_admin FOR SHARE`, metadata.ActorID).Scan(&actor); errors.Is(err, pgx.ErrNoRows) {
		return importjob.Template{}, auth.ErrForbidden
	} else if err != nil {
		return importjob.Template{}, err
	}
	var typeID string
	if err = tx.QueryRow(ctx, `SELECT id FROM inspection_types WHERE code=$1 AND enabled FOR SHARE`, typeCode).Scan(&typeID); err != nil {
		return importjob.Template{}, err
	}
	rows, err := tx.Query(ctx, inspectionPublicationQuery, typeCode, definition.Code)
	if err != nil {
		return importjob.Template{}, err
	}
	existing, err := scanPublication(rows)
	if err == nil {
		if existing.Code != definition.Code || existing.Version != 1 || existing.Status != "PUBLISHED" || existing.InspectionTypeID != typeID {
			return importjob.Template{}, importjob.ErrTemplatePublicationConflict
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, importjob.ErrTemplateNotFound) {
		return importjob.Template{}, err
	}
	var validFile string
	if err = tx.QueryRow(ctx, `SELECT id FROM files WHERE id=$1 AND status='AVAILABLE' AND retention_type='TEMPLATE' AND sha256=$2 AND size_bytes=$3 FOR SHARE`, file.ID, file.SHA256, file.SizeBytes).Scan(&validFile); err != nil {
		return importjob.Template{}, err
	}
	fieldDefinition, err := json.Marshal(fields)
	if err != nil {
		return importjob.Template{}, err
	}
	result := importjob.Template{ID: uuid.NewString(), Code: definition.Code, Name: definition.Name, InspectionTypeID: typeID, Version: 1, FileID: file.ID, Status: "PUBLISHED"}
	if _, err = tx.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,field_definition_json,status,published_at,created_by) VALUES($1,$2,$3,$4,1,$5,$6,'PUBLISHED',now(),$7)`, result.ID, result.Code, result.Name, typeID, file.ID, fieldDefinition, actor); err != nil {
		return importjob.Template{}, err
	}
	entry := audit.Entry{Action: audit.Action("IMPORT_TEMPLATE_PUBLISHED"), ResourceType: "import_template", ResourceID: result.ID, After: map[string]any{"code": result.Code, "version": 1, "fileId": file.ID, "sha256": file.SHA256, "status": result.Status}}
	if err = insertAuditTx(ctx, tx, metadata, entry); err != nil {
		return importjob.Template{}, err
	}
	return result, tx.Commit(ctx)
}
