package importjob_test

import (
	"bytes"
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOrphanSourceCompensationPostgresAndMinIO(t *testing.T) {
	environment := requireIntegrationEnvironment(t)
	// This test applies all migrations and must not reuse the import test's schema.
	environment.databaseURL = os.Getenv("ORPHAN_SOURCE_TEST_DATABASE_URL")
	if environment.databaseURL == "" {
		t.Skip("ORPHAN_SOURCE_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, environment.databaseURL)
	db, err := store.Open(ctx, environment.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	admin, err := minio.New(environment.endpoint, &minio.Options{Creds: credentials.NewStaticV4(environment.accessKey, environment.secretKey, ""), Secure: environment.useTLS})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "orphan-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := admin.RemoveBucket(ctx, bucket); e != nil {
			t.Errorf("remove isolated bucket: %v", e)
		}
	})
	config := storage.MinIOConfig{Endpoint: environment.endpoint, AccessKey: environment.accessKey, SecretKey: environment.secretKey, Bucket: bucket, UseTLS: environment.useTLS, ConnectTimeout: time.Second, OperationTimeout: 3 * time.Second}
	objects, err := storage.NewMinIO(config)
	if err != nil {
		t.Fatal(err)
	}
	files := storage.NewService(db, objects, 16<<20)
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	metadata := audit.Metadata{ActorID: fixture.accountID, RequestID: "orphan-" + uuid.NewString()}
	track := func(file storage.File) {
		t.Cleanup(func() {
			if e := objects.Remove(ctx, file.ObjectKey); e != nil {
				t.Errorf("remove isolated object: %v", e)
			}
		})
	}
	makeFile := func(retention string) storage.File {
		t.Helper()
		f, e := files.Store(ctx, storage.StoreInput{OriginalName: "isolated.xlsx", DeclaredContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Size: int64(len(fixture.workbook)), RetentionType: retention, CreatedBy: fixture.accountID, Body: bytes.NewReader(fixture.workbook)}, metadata)
		if e != nil {
			t.Fatal(e)
		}
		track(f)
		return f
	}
	published, err := importjob.NewAttendanceTemplatePublisher(db, files).Publish(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	templateFile, err := db.FindFile(ctx, published.FileID)
	if err != nil {
		t.Fatal(err)
	}
	track(templateFile)
	imports := importjob.NewCourseImportService(db, files, importjob.NewAttendanceAdapter(db), 16<<20)
	createBatch := func(f storage.File) (importjob.Batch, error) {
		return imports.CreateBatch(ctx, importjob.CreateBatchInput{InspectionTypeID: integrationInspectionTypeID, SemesterID: fixture.semesterID, WeekID: fixture.weekID, DepartmentID: integrationDepartmentID, SourceFile: f, TemplateID: published.ID, TemplateVersion: published.Version, CreatedBy: fixture.accountID})
	}

	t.Run("unreferenced available source is recoverable and removed", func(t *testing.T) {
		file := makeFile(storage.RetentionImportSource)
		if e := files.Discard(ctx, file.ID, metadata); e != nil {
			t.Fatal("discard available orphan", e)
		}
		stored, e := db.FindFile(ctx, file.ID)
		if e != nil || stored.Status != storage.StatusFailed || stored.FailureCode != storage.FailureOrphanedImport {
			t.Fatal("wrong compensated state", stored.Status, e)
		}
		if _, e = objects.Stat(ctx, file.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
			t.Fatal("orphan object retained", e)
		}
		if _, e = createBatch(file); e == nil {
			t.Fatal("discarded source was linked using stale AVAILABLE metadata")
		}
	})
	t.Run("referenced template and other-owner files are protected", func(t *testing.T) {
		file := makeFile(storage.RetentionImportSource)
		if _, e := createBatch(file); e != nil {
			t.Fatal(e)
		}
		other := makeFile(storage.RetentionImportSource)
		for _, attempt := range []struct {
			file  storage.File
			actor string
		}{{file, fixture.accountID}, {templateFile, fixture.accountID}, {other, uuid.NewString()}, {other, ""}} {
			if e := files.Discard(ctx, attempt.file.ID, audit.Metadata{ActorID: attempt.actor, RequestID: metadata.RequestID}); e == nil {
				t.Fatal("protected file discarded")
			}
			if _, _, e := files.ReadVerified(ctx, attempt.file.ID, metadata); e != nil {
				t.Fatal("protected file is no longer readable", e)
			}
		}
	})
	t.Run("audit failure rolls back before deleting object", func(t *testing.T) {
		file := makeFile(storage.RetentionImportSource)
		if _, e := db.Pool.Exec(ctx, `CREATE FUNCTION reject_orphan_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='FILE_UPLOAD_FAILED' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_orphan_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_orphan_audit()`); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if _, e := db.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_orphan_audit ON audit_logs; DROP FUNCTION IF EXISTS reject_orphan_audit()`); e != nil {
				t.Errorf("remove audit fault: %v", e)
			}
		})
		if e := files.Discard(ctx, file.ID, metadata); e == nil {
			t.Fatal("audit fault ignored")
		}
		if _, _, e := files.ReadVerified(ctx, file.ID, metadata); e != nil {
			t.Fatal("audit failure removed source", e)
		}
		if _, e := db.Pool.Exec(ctx, `DROP TRIGGER reject_orphan_audit ON audit_logs; DROP FUNCTION reject_orphan_audit()`); e != nil {
			t.Fatal(e)
		}
		if e := files.Discard(ctx, file.ID, metadata); e != nil {
			t.Fatal("audit recovery retry failed", e)
		}
	})
	t.Run("object removal failure remains recoverable", func(t *testing.T) {
		file := makeFile(storage.RetentionImportSource)
		bad := config
		bad.AccessKey = "wrong-key"
		bad.SecretKey = "wrong-secret-key"
		unavailable, e := storage.NewMinIO(bad)
		if e != nil {
			t.Fatal(e)
		}
		if e = storage.NewService(db, unavailable, 16<<20).Discard(ctx, file.ID, metadata); e == nil {
			t.Fatal("real MinIO denial ignored")
		}
		stored, e := db.FindFile(ctx, file.ID)
		if e != nil || stored.Status != storage.StatusFailed {
			t.Fatal("cleanup failure lost recoverable state", e)
		}
		if _, e = objects.Stat(ctx, file.ObjectKey); e != nil {
			t.Fatal("object unexpectedly absent", e)
		}
		result, e := files.Reconcile(ctx, time.Now().Add(time.Hour), 100, metadata)
		if e != nil || result.Failed != 0 {
			t.Fatal("recovery failed", e)
		}
		if _, e = objects.Stat(ctx, file.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
			t.Fatal("recovery did not remove orphan", e)
		}
	})
	t.Run("batch creation and discard have one winner", func(t *testing.T) {
		for i := 0; i < 6; i++ {
			file := makeFile(storage.RetentionImportSource)
			start := make(chan struct{})
			var wg sync.WaitGroup
			var batchErr, discardErr error
			wg.Add(2)
			go func() { defer wg.Done(); <-start; _, batchErr = createBatch(file) }()
			go func() { defer wg.Done(); <-start; discardErr = files.Discard(ctx, file.ID, metadata) }()
			close(start)
			wg.Wait()
			if (batchErr == nil) == (discardErr == nil) {
				t.Fatal("expected one winner", batchErr, discardErr)
			}
			if batchErr == nil {
				if _, _, e := files.ReadVerified(ctx, file.ID, metadata); e != nil {
					t.Fatal("bound source deleted", e)
				}
			} else {
				if _, e := objects.Stat(ctx, file.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
					t.Fatal("unbound source retained", e)
				}
			}
		}
	})
	t.Run("bounded reconciliation advances beyond cleaned and failed candidates", func(t *testing.T) {
		if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status='FAILED'`); e != nil {
			t.Fatal(e)
		}
		var sources []storage.File
		bad := config
		bad.AccessKey = "wrong-key"
		bad.SecretKey = "wrong-secret-key"
		denied, e := storage.NewMinIO(bad)
		if e != nil {
			t.Fatal(e)
		}
		deniedFiles := storage.NewService(db, denied, 16<<20)
		for i := 0; i < 3; i++ {
			f := makeFile(storage.RetentionImportSource)
			if e := deniedFiles.Discard(ctx, f.ID, metadata); e == nil {
				t.Fatal("expected object denial")
			}
			if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()-($2*interval '1 hour') WHERE id=$1`, f.ID, 3-i); e != nil {
				t.Fatal(e)
			}
			sources = append(sources, f)
		}
		cutoff := time.Now().Add(time.Hour)
		result, e := deniedFiles.Reconcile(ctx, cutoff, 1, metadata)
		if e != nil || result.Failed != 1 {
			t.Fatal("expected bounded failure", e, result)
		}
		candidates, e := db.ListFileReconciliationCandidates(ctx, bucket, cutoff, 1)
		if e != nil || len(candidates) != 1 || candidates[0].ID != sources[1].ID {
			t.Fatal("failed oldest candidate starved later files", e)
		}
		for i := 0; i < 3; i++ {
			result, e := files.Reconcile(ctx, cutoff, 1, metadata)
			if e != nil || result.Cleaned != 1 || result.Failed != 0 {
				t.Fatal("queue did not advance", e, result)
			}
		}
		candidates, e = db.ListFileReconciliationCandidates(ctx, bucket, cutoff, 1)
		if e != nil || len(candidates) != 0 {
			t.Fatal("cleaned files remain eligible", e, len(candidates))
		}
		for _, f := range sources {
			if _, e := objects.Stat(ctx, f.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
				t.Fatal("later object retained", e)
			}
		}
	})

	t.Run("cleanup marker and audit roll back together and retry missing object", func(t *testing.T) {
		file := makeFile(storage.RetentionImportSource)
		bad := config
		bad.AccessKey = "wrong-key"
		bad.SecretKey = "wrong-secret-key"
		denied, e := storage.NewMinIO(bad)
		if e != nil {
			t.Fatal(e)
		}
		if e := storage.NewService(db, denied, 16<<20).Discard(ctx, file.ID, metadata); e == nil {
			t.Fatal("expected denial")
		}
		if _, e := db.Pool.Exec(ctx, `CREATE FUNCTION reject_cleanup_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='FILE_OBJECT_REMOVED' THEN RAISE EXCEPTION 'isolated cleanup audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_cleanup_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_cleanup_audit()`); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if _, e := db.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_cleanup_audit ON audit_logs; DROP FUNCTION IF EXISTS reject_cleanup_audit()`); e != nil {
				t.Error(e)
			}
		})
		result, e := files.Reconcile(ctx, time.Now().Add(time.Hour), 100, metadata)
		if e != nil || result.Failed != 1 || result.Cleaned != 0 {
			t.Fatal("marker fault was hidden", e, result)
		}
		var marked bool
		if e := db.Pool.QueryRow(ctx, `SELECT object_removed_at IS NOT NULL FROM files WHERE id=$1`, file.ID).Scan(&marked); e != nil || marked {
			t.Fatal("marker survived audit rollback", e)
		}
		if _, e := objects.Stat(ctx, file.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
			t.Fatal("physical removal not exercised", e)
		}
		if _, e := db.Pool.Exec(ctx, `DROP TRIGGER reject_cleanup_audit ON audit_logs; DROP FUNCTION reject_cleanup_audit()`); e != nil {
			t.Fatal(e)
		}
		result, e = files.Reconcile(ctx, time.Now().Add(time.Hour), 100, metadata)
		if e != nil || result.Cleaned != 1 || result.Failed != 0 {
			t.Fatal("missing-object retry did not complete", e, result)
		}
		var count int
		if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='FILE_OBJECT_REMOVED'`, file.ID).Scan(&count); e != nil || count != 1 {
			t.Fatal("cleanup audit mismatch", e, count)
		}
	})

	t.Run("metadata bucket prevents cross-bucket reads and deletion", func(t *testing.T) {
		otherBucket := "other-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if e := admin.MakeBucket(ctx, otherBucket, minio.MakeBucketOptions{}); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if e := admin.RemoveBucket(ctx, otherBucket); e != nil {
				t.Error(e)
			}
		})
		otherConfig := config
		otherConfig.Bucket = otherBucket
		otherObjects, e := storage.NewMinIO(otherConfig)
		if e != nil {
			t.Fatal(e)
		}
		wrongFiles := storage.NewService(db, otherObjects, 16<<20)
		for _, operation := range []string{"open", "verified", "discard", "failed", "pending"} {
			t.Run(operation, func(t *testing.T) {
				file := makeFile(storage.RetentionImportSource)
				if _, e := otherObjects.Put(ctx, file.ObjectKey, bytes.NewReader(fixture.workbook), int64(len(fixture.workbook)), file.DetectedContentType); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					if e := otherObjects.Remove(ctx, file.ObjectKey); e != nil {
						t.Error(e)
					}
				})
				expectedStatus := storage.StatusAvailable
				if operation == "failed" || operation == "pending" {
					if operation == "failed" {
						expectedStatus = storage.StatusFailed
					} else {
						expectedStatus = storage.StatusPending
					}
					if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status IN ('FAILED','PENDING')`); e != nil {
						t.Fatal(e)
					}
					if _, e := db.Pool.Exec(ctx, `UPDATE files SET status=$2,updated_at=now()-interval '1 hour' WHERE id=$1`, file.ID, expectedStatus); e != nil {
						t.Fatal(e)
					}
				}
				switch operation {
				case "open":
					_, object, e := wrongFiles.Open(ctx, file.ID, metadata)
					if object.Body != nil {
						object.Body.Close()
					}
					if !errors.Is(e, storage.ErrFileUnavailable) {
						t.Fatal("read accepted wrong bucket", e)
					}
				case "verified":
					if _, _, e := wrongFiles.ReadVerified(ctx, file.ID, metadata); !errors.Is(e, storage.ErrFileUnavailable) {
						t.Fatal("verified read accepted wrong bucket", e)
					}
				case "discard":
					if e := wrongFiles.Discard(ctx, file.ID, metadata); !errors.Is(e, storage.ErrFileUnavailable) {
						t.Fatal("discard accepted wrong bucket", e)
					}
				default:
					result, e := wrongFiles.Reconcile(ctx, time.Now(), 1, metadata)
					if e != nil || result.Failed != 0 || result.Cleaned != 0 || result.Recovered != 0 {
						t.Fatal("reconciliation accepted wrong bucket", e, result)
					}
				}
				if _, e := otherObjects.Stat(ctx, file.ObjectKey); e != nil {
					t.Fatal("other bucket object removed", e)
				}
				if _, e := objects.Stat(ctx, file.ObjectKey); e != nil {
					t.Fatal("original bucket object removed", e)
				}
				current, e := db.FindFile(ctx, file.ID)
				if e != nil || current.Status != expectedStatus {
					t.Fatal("metadata changed on bucket mismatch", e)
				}
			})
		}
		t.Run("foreign oldest candidate does not starve current bucket", func(t *testing.T) {
			if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status IN ('FAILED','PENDING')`); e != nil {
				t.Fatal(e)
			}
			foreign := makeFile(storage.RetentionImportSource)
			local := makeFile(storage.RetentionImportSource)
			if _, e := otherObjects.Put(ctx, foreign.ObjectKey, bytes.NewReader(fixture.workbook), int64(len(fixture.workbook)), foreign.DetectedContentType); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				if e := otherObjects.Remove(ctx, foreign.ObjectKey); e != nil {
					t.Error(e)
				}
			})
			if _, e := db.Pool.Exec(ctx, `UPDATE files SET bucket=$2,status='FAILED',updated_at=now()-interval '2 hours' WHERE id=$1`, foreign.ID, otherBucket); e != nil {
				t.Fatal(e)
			}
			if _, e := db.Pool.Exec(ctx, `UPDATE files SET status='FAILED',updated_at=now()-interval '1 hour' WHERE id=$1`, local.ID); e != nil {
				t.Fatal(e)
			}
			result, e := files.Reconcile(ctx, time.Now(), 1, metadata)
			if e != nil || result.Cleaned != 1 || result.Failed != 0 {
				t.Fatal("foreign bucket starved current bucket", e, result)
			}
			if _, e := otherObjects.Stat(ctx, foreign.ObjectKey); e != nil {
				t.Fatal("foreign object touched", e)
			}
			if _, e := objects.Stat(ctx, local.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
				t.Fatal("local candidate not cleaned", e)
			}
		})

	})

	t.Run("upload state conflict preserves object recovered by another worker", func(t *testing.T) {
		for _, transition := range []string{"available", "failed"} {
			t.Run(transition, func(t *testing.T) {
				if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status IN ('FAILED','PENDING')`); e != nil {
					t.Fatal(e)
				}
				hook := func(file storage.File) error {
					track(file)
					if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()-interval '1 hour' WHERE id=$1`, file.ID); e != nil {
						return e
					}
					result, e := files.Reconcile(ctx, time.Now(), 1, metadata)
					if e != nil {
						return e
					}
					if result.Recovered != 1 {
						return errors.New("other worker did not recover pending upload")
					}
					return nil
				}
				repository := &interleavedFileRepository{Postgres: db}
				var uploadObjects storage.ObjectStorage = objects
				if transition == "available" {
					repository.beforeAvailable = hook
				} else {
					repository.beforeFailed = hook
					uploadObjects = statFailureObjects{ObjectStorage: objects}
				}
				file, e := storage.NewService(repository, uploadObjects, 16<<20).Store(ctx, storage.StoreInput{OriginalName: "concurrent.xlsx", Size: int64(len(fixture.workbook)), RetentionType: storage.RetentionImportSource, CreatedBy: fixture.accountID, Body: bytes.NewReader(fixture.workbook)}, metadata)
				if !errors.Is(e, storage.ErrFileUnavailable) {
					t.Fatal("expected losing state transition", e)
				}
				if _, _, e := files.ReadVerified(ctx, file.ID, metadata); e != nil {
					t.Fatal("losing uploader deleted recovered object", e)
				}
			})
		}
	})

	t.Run("pending object above configured size becomes audited failure", func(t *testing.T) {
		if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status IN ('FAILED','PENDING')`); e != nil {
			t.Fatal(e)
		}
		file := makeFile(storage.RetentionImportSource)
		if _, e := db.Pool.Exec(ctx, `UPDATE files SET status='PENDING',updated_at=now()-interval '1 hour' WHERE id=$1`, file.ID); e != nil {
			t.Fatal(e)
		}
		result, e := storage.NewService(db, objects, 8).Reconcile(ctx, time.Now(), 1, metadata)
		if e != nil || result.Failed != 1 || result.Recovered != 0 {
			t.Fatal("oversize recovery accepted", e, result)
		}
		current, e := db.FindFile(ctx, file.ID)
		if e != nil || current.Status != storage.StatusFailed || current.FailureCode != storage.FailureFileTooLarge {
			t.Fatal("missing stable size failure", e, current.Status, current.FailureCode)
		}
		if _, e := objects.Stat(ctx, file.ObjectKey); !errors.Is(e, storage.ErrObjectMissing) {
			t.Fatal("failed oversized object retained", e)
		}
	})

	t.Run("transient open and stream errors preserve pending object for retry", func(t *testing.T) {
		for _, failure := range []string{"open", "stream"} {
			t.Run(failure, func(t *testing.T) {
				if _, e := db.Pool.Exec(ctx, `UPDATE files SET updated_at=now()+interval '1 day' WHERE status IN ('FAILED','PENDING')`); e != nil {
					t.Fatal(e)
				}
				file := makeFile(storage.RetentionImportSource)
				if _, e := db.Pool.Exec(ctx, `UPDATE files SET status='PENDING',updated_at=now()-interval '1 hour' WHERE id=$1`, file.ID); e != nil {
					t.Fatal(e)
				}
				var failingObjects storage.ObjectStorage
				var e error
				if failure == "open" {
					bad := config
					bad.AccessKey = "wrong-key"
					bad.SecretKey = "wrong-secret-key"
					failingObjects, e = storage.NewMinIO(bad)
					if e != nil {
						t.Fatal(e)
					}
				} else {
					failingObjects = streamFailureObjects{ObjectStorage: objects}
				}
				result, e := storage.NewService(db, failingObjects, 16<<20).Reconcile(ctx, time.Now(), 1, metadata)
				if e != nil || result.Failed != 1 {
					t.Fatal("expected transient read failure", result, e)
				}
				current, e := db.FindFile(ctx, file.ID)
				if e != nil || current.Status != storage.StatusPending {
					t.Fatal("transient read destroyed pending metadata", e, current.Status)
				}
				if _, e := objects.Stat(ctx, file.ObjectKey); e != nil {
					t.Fatal("transient read deleted object", e)
				}
				candidates, e := db.ListFileReconciliationCandidates(ctx, bucket, time.Now().Add(-time.Minute), 1)
				if e != nil || len(candidates) != 0 {
					t.Fatal("deferred candidate did not yield stale queue", e)
				}
				result, e = files.Reconcile(ctx, time.Now().Add(time.Hour), 1, metadata)
				if e != nil || result.Recovered != 1 {
					t.Fatal("read retry did not recover", result, e)
				}
				if _, _, e := files.ReadVerified(ctx, file.ID, metadata); e != nil {
					t.Fatal("recovered source unreadable", e)
				}
			})
		}
	})

}

// Pause one upload transition while a real second service recovers its PENDING row.
type interleavedFileRepository struct {
	*store.Postgres
	beforeAvailable func(storage.File) error
	beforeFailed    func(storage.File) error
}

func (r *interleavedFileRepository) MarkFileAvailable(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	if r.beforeAvailable != nil {
		if e := r.beforeAvailable(file); e != nil {
			return e
		}
	}
	return r.Postgres.MarkFileAvailable(ctx, file, mutation)
}
func (r *interleavedFileRepository) MarkFileFailed(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	if r.beforeFailed != nil {
		if e := r.beforeFailed(file); e != nil {
			return e
		}
	}
	return r.Postgres.MarkFileFailed(ctx, file, mutation)
}

type statFailureObjects struct{ storage.ObjectStorage }

func (statFailureObjects) Stat(context.Context, string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, storage.ErrUnavailable
}

type streamFailureObjects struct{ storage.ObjectStorage }

func (s streamFailureObjects) Open(ctx context.Context, key string) (storage.Object, error) {
	object, err := s.ObjectStorage.Open(ctx, key)
	if err != nil {
		return object, err
	}
	object.Body = &interruptedBody{ReadCloser: object.Body, remaining: 512}
	return object, nil
}

type interruptedBody struct {
	io.ReadCloser
	remaining int
}

func (r *interruptedBody) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, storage.ErrUnavailable
	}
	n, err := r.ReadCloser.Read(p[:min(len(p), r.remaining)])
	r.remaining -= n
	return n, err
}
