package importjob_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
)

type rollbackFileReader struct {
	file    storage.File
	content []byte
}

func (r rollbackFileReader) ReadVerified(context.Context, string, audit.Metadata) (storage.File, []byte, error) {
	return r.file, r.content, nil
}

func TestImportRollbackPostgres(t *testing.T) {
	url := os.Getenv("ROLLBACK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ROLLBACK_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	fileID, templateID := uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by) VALUES($1,'isolated','rollback/source','source.xlsx',1,$2,'AVAILABLE','IMPORT_SOURCE',$3)`, fileID, strings.Repeat("a", 64), fixture.accountID)
	exec(`INSERT INTO import_templates(id,code,name,inspection_type_id,version_no,file_id,status,created_by) VALUES($1,'ATTENDANCE_IMPORT','rollback',$2,1,$3,'DRAFT',$4)`, templateID, integrationInspectionTypeID, fileID, fixture.accountID)
	source := storage.File{ID: fileID, OriginalName: "source.xlsx", Status: storage.StatusAvailable, SHA256: strings.Repeat("a", 64)}
	imports := importjob.NewService(db, rollbackFileReader{source, fixture.workbook}, importjob.NewAttendanceXLSXParser(16<<20), importjob.NewAttendanceAdapter(db))
	batch, err := imports.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: integrationInspectionTypeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: integrationDepartmentID, SourceFile: source, TemplateID: templateID, TemplateVersion: 1, CreatedBy: fixture.accountID})
	if err != nil {
		t.Fatal(err)
	}
	batch, err = imports.ParseBatch(ctx, importjob.ParseBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ValidRows != 2 {
		t.Fatalf("valid rows %d", batch.ValidRows)
	}
	confirmed, err := imports.ConfirmBatch(ctx, importjob.ConfirmBatchInput{BatchID: batch.ID, ExpectedVersion: batch.Version, IdempotencyKey: uuid.NewString(), Scope: importjob.ConfirmScope{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, ActorID: fixture.accountID}, AuditMetadata: audit.Metadata{ActorID: fixture.accountID}})
	if err != nil {
		t.Fatal(err)
	}
	var id, classID string
	if err = db.Pool.QueryRow(ctx, `SELECT id,class_id FROM inspection_records WHERE import_batch_id=$1 ORDER BY id LIMIT 1`, batch.ID).Scan(&id, &classID); err != nil {
		t.Fatal(err)
	}
	service := importjob.NewRollbackService(db, permission.NewService(db))
	actor := inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}
	input := importjob.RollbackInput{BatchID: batch.ID, Version: confirmed.Version, Reason: "整批撤销误导入"}
	unchanged := func() {
		t.Helper()
		var count, version int
		var status string
		if err := db.Pool.QueryRow(ctx, `SELECT b.status,b.version,(SELECT count(*) FROM inspection_records WHERE import_batch_id=b.id AND deleted_at IS NULL) FROM import_batches b WHERE b.id=$1`, batch.ID).Scan(&status, &version, &count); err != nil || status != "CONFIRMED" || version != confirmed.Version || count != 2 {
			t.Fatalf("partial mutation status=%s version=%d count=%d err=%v", status, version, count, err)
		}
	}
	stale := input
	stale.Version--
	if _, err = service.Rollback(ctx, actor, stale, audit.Metadata{}); !errors.Is(err, importjob.ErrBatchVersionConflict) {
		t.Fatal(err)
	}
	for _, status := range []string{"PENDING_REVIEW", "RETURNED", "CONFIRMED", "LOCKED"} {
		exec(`UPDATE inspection_records SET status=$2 WHERE id=$1`, id, status)
		if _, err = service.Rollback(ctx, actor, input, audit.Metadata{}); !errors.Is(err, importjob.ErrRollbackUnsafe) {
			t.Fatalf("state %s: %v", status, err)
		}
		unchanged()
	}
	exec(`UPDATE inspection_records SET status='DRAFT',row_version=2 WHERE id=$1`, id)
	if _, err = service.Rollback(ctx, actor, input, audit.Metadata{}); !errors.Is(err, importjob.ErrRollbackUnsafe) {
		t.Fatal(err)
	}
	unchanged()
	submissionID := uuid.NewString()
	exec(`INSERT INTO department_submissions(id,semester_id,week_id,department_id,inspection_type_id,created_by,request_fingerprint)
			VALUES($1,$2,$3,$4,$5,$6,repeat('a',64))`, submissionID, fixture.semesterID, fixture.weekID, integrationDepartmentID, integrationInspectionTypeID, fixture.accountID)
	exec(`UPDATE inspection_records SET row_version=1,submission_id=$2 WHERE id=$1`, id, submissionID)
	if _, err = service.Rollback(ctx, actor, input, audit.Metadata{}); !errors.Is(err, importjob.ErrRollbackUnsafe) {
		t.Fatal(err)
	}
	unchanged()
	exec(`UPDATE inspection_records SET submission_id=NULL,locked_by_report_id=$2 WHERE id=$1`, id, uuid.NewString())
	if _, err = service.Rollback(ctx, actor, input, audit.Metadata{}); !errors.Is(err, importjob.ErrRollbackUnsafe) {
		t.Fatal(err)
	}
	unchanged()
	exec(`UPDATE inspection_records SET locked_by_report_id=NULL WHERE id=$1`, id)
	exec(`UPDATE import_rows SET record_id=NULL WHERE record_id=$1`, id)
	if _, err = service.Rollback(ctx, actor, input, audit.Metadata{}); !errors.Is(err, importjob.ErrRollbackUnsafe) {
		t.Fatal(err)
	}
	unchanged()
	exec(`UPDATE import_rows SET record_id=$2 WHERE batch_id=$1 AND record_id IS NULL`, batch.ID, id)
	restricted := mutationPermissions{permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: integrationDepartmentID}}, {Action: permission.ActionInspectionEnter, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: classID}}}}
	if _, err = importjob.NewRollbackService(db, restricted).Rollback(ctx, inspection.ManualActor{ID: fixture.accountID}, input, audit.Metadata{}); !errors.Is(err, importjob.ErrBatchScopeMismatch) {
		t.Fatalf("partial deny: %v", err)
	}
	unchanged()
	if _, err = service.Rollback(ctx, actor, input, audit.Metadata{IP: "invalid-ip"}); err == nil {
		t.Fatal("audit failure accepted")
	}
	unchanged()
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := service.Rollback(ctx, actor, input, audit.Metadata{RequestID: uuid.NewString()})
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, importjob.ErrBatchVersionConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrency successes=%d conflicts=%d", success, conflict)
	}
	var status string
	var version, deleted, links, files, audits int
	if err = db.Pool.QueryRow(ctx, `SELECT status,version FROM import_batches WHERE id=$1 AND rolled_back_at IS NOT NULL`, batch.ID).Scan(&status, &version); err != nil || status != "ROLLED_BACK" || version != confirmed.Version+1 {
		t.Fatal(status, version, err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM inspection_records WHERE import_batch_id=$1 AND deleted_at IS NOT NULL AND row_version=2),(SELECT count(*) FROM import_rows WHERE batch_id=$1 AND record_id IS NOT NULL),(SELECT count(*) FROM files WHERE id=$2 AND status='AVAILABLE'),(SELECT count(*) FROM audit_logs WHERE action='IMPORT_BATCH_ROLLED_BACK' AND resource_id=$1::text)`, batch.ID, fileID).Scan(&deleted, &links, &files, &audits); err != nil || deleted != 2 || links != 2 || files != 1 || audits != 1 {
		t.Fatal(deleted, links, files, audits, err)
	}
}
