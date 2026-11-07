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

// scanPublications returns every template version recorded for one inspection
// type so callers can distinguish "never published" from "needs a version bump".
func scanPublications(rows pgx.Rows) ([]importjob.Template, error) {
	defer rows.Close()
	var result []importjob.Template
	for rows.Next() {
		var template importjob.Template
		if err := rows.Scan(&template.ID, &template.Code, &template.Name, &template.InspectionTypeID, &template.Version, &template.FileID, &template.Status); err != nil {
			return nil, err
		}
		result = append(result, template)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// currentPublication picks the single PUBLISHED version. Any other combination
// is ambiguous and must be resolved by an explicit migration instead of guessing.
func currentPublication(templates []importjob.Template) (importjob.Template, error) {
	var published []importjob.Template
	for _, template := range templates {
		if template.Status == "PUBLISHED" {
			published = append(published, template)
		}
	}
	if len(published) == 0 {
		if len(templates) != 0 {
			return importjob.Template{}, importjob.ErrTemplatePublicationConflict
		}
		return importjob.Template{}, importjob.ErrTemplateNotFound
	}
	if len(published) != 1 {
		return importjob.Template{}, importjob.ErrTemplatePublicationConflict
	}
	return published[0], nil
}

const inspectionPublicationQuery = `SELECT t.id,t.code,t.name,t.inspection_type_id,t.version_no,t.file_id,t.status FROM import_templates t JOIN inspection_types i ON i.id=t.inspection_type_id WHERE i.code=$1 OR t.code=$2 ORDER BY t.version_no`

func (p *Postgres) FindInspectionTemplate(ctx context.Context, typeCode string) (importjob.Template, error) {
	definition, err := importjob.InspectionTemplate(typeCode)
	if err != nil {
		return importjob.Template{}, err
	}
	rows, err := p.Pool.Query(ctx, inspectionPublicationQuery, typeCode, definition.Code)
	if err != nil {
		return importjob.Template{}, err
	}
	templates, err := scanPublications(rows)
	if err != nil {
		return importjob.Template{}, err
	}
	result, err := currentPublication(templates)
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

// PublishInspectionTemplate serializes competing publishers and atomically
// retires the superseded version before publishing the current one.
func (p *Postgres) PublishInspectionTemplate(ctx context.Context, typeCode string, file storage.File, fields []importjob.TemplateField, metadata audit.Metadata) (importjob.Template, error) {
	definition, err := importjob.InspectionTemplate(typeCode)
	if err != nil {
		return importjob.Template{}, err
	}
	templateVersion, err := importjob.InspectionTemplateVersion(typeCode)
	if err != nil {
		return importjob.Template{}, err
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return importjob.Template{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "import-template-"+typeCode); err != nil {
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
	templates, err := scanPublications(rows)
	if err != nil {
		return importjob.Template{}, err
	}
	retiredID, retiredVersion := "", 0
	published, currentErr := currentPublication(templates)
	switch {
	case currentErr == nil:
		if published.Code != definition.Code || published.InspectionTypeID != typeID {
			return importjob.Template{}, importjob.ErrTemplatePublicationConflict
		}
		if published.Version > templateVersion {
			return importjob.Template{}, importjob.ErrTemplatePublicationConflict
		}
		if published.Version == templateVersion {
			// Idempotent retry: the requested version is already live.
			for _, template := range templates {
				if template.Version >= templateVersion && template.ID != published.ID {
					return importjob.Template{}, importjob.ErrTemplatePublicationConflict
				}
			}
			return published, tx.Commit(ctx)
		}
		retiredID, retiredVersion = published.ID, published.Version
	case !errors.Is(currentErr, importjob.ErrTemplateNotFound):
		return importjob.Template{}, currentErr
	}
	// A superseded draft already occupying the target version blocks the publish.
	for _, template := range templates {
		if template.Version == templateVersion && template.ID != retiredID {
			return importjob.Template{}, importjob.ErrTemplatePublicationConflict
		}
	}
	if retiredID != "" {
		if _, err = tx.Exec(ctx, `UPDATE import_templates SET status='RETIRED', updated_at=now() WHERE id=$1`, retiredID); err != nil {
			return importjob.Template{}, err
		}
		retired := audit.Entry{Action: audit.Action("IMPORT_TEMPLATE_RETIRED"), ResourceType: "import_template", ResourceID: retiredID, Before: map[string]any{"version": retiredVersion, "status": "PUBLISHED"}, After: map[string]any{"version": retiredVersion, "status": "RETIRED", "supersededBy": templateVersion}}
		if err = insertAuditTx(ctx, tx, metadata, retired); err != nil {
			return importjob.Template{}, err
		}
	}
	var validFile string
	if err = tx.QueryRow(ctx, `SELECT id FROM files WHERE id=$1 AND status='AVAILABLE' AND retention_type='TEMPLATE' AND sha256=$2 AND size_bytes=$3 FOR SHARE`, file.ID, file.SHA256, file.SizeBytes).Scan(&validFile); err != nil {
		return importjob.Template{}, err
	}
	fieldDefinition, err := json.Marshal(fields)
	if err != nil {
		return importjob.Template{}, err
	}
	result := importjob.Template{ID: uuid.NewString(), Code: definition.Code, Name: definition.Name, InspectionTypeID: typeID, Version: templateVersion, FileID: file.ID, Status: "PUBLISHED"}
	if _, err = tx.Exec(ctx, `INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,field_definition_json,status,published_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,'PUBLISHED',now(),$8)`, result.ID, result.Code, result.Name, typeID, result.Version, file.ID, fieldDefinition, actor); err != nil {
		return importjob.Template{}, err
	}
	entry := audit.Entry{Action: audit.Action("IMPORT_TEMPLATE_PUBLISHED"), ResourceType: "import_template", ResourceID: result.ID, After: map[string]any{"code": result.Code, "version": result.Version, "fileId": file.ID, "sha256": file.SHA256, "status": result.Status}}
	if err = insertAuditTx(ctx, tx, metadata, entry); err != nil {
		return importjob.Template{}, err
	}
	return result, tx.Commit(ctx)
}
