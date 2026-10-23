package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestRecoveryCLIAgainstPostgresAndMinIO(t *testing.T) {
	databaseURL := os.Getenv("FILE_RECOVERY_CLI_TEST_DATABASE_URL")
	endpoint, access, secret := os.Getenv("TEST_MINIO_ENDPOINT"), os.Getenv("TEST_MINIO_ACCESS_KEY"), os.Getenv("TEST_MINIO_SECRET_KEY")
	if databaseURL == "" || endpoint == "" || access == "" || secret == "" {
		t.Skip("requires empty disposable PostgreSQL and MinIO")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var exists bool
	if err := db.Pool.QueryRow(ctx, `SELECT to_regclass('public.accounts') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("requires empty database", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal(err)
	}
	for _, path := range migrations {
		if filepath.Base(path) == "015_file_cleanup_progress.sql" {
			t.Setenv("DATABASE_URL", databaseURL)
			t.Setenv("MINIO_BUCKET", "isolated-preview")
			if err := run(nil, &bytes.Buffer{}); err == nil {
				t.Fatal("preview accepted incomplete migration")
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Pool.Exec(ctx, string(content)); err != nil {
			t.Fatal(err)
		}
	}
	actor := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,'recovery-cli','Synthetic recovery operator','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, actor); err != nil {
		t.Fatal(err)
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "recovery-cli-" + uuid.NewString()
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.RemoveBucket(ctx, bucket); err != nil {
			t.Error(err)
		}
	})
	objects, err := storage.NewMinIO(storage.MinIOConfig{Endpoint: endpoint, AccessKey: access, SecretKey: secret, Bucket: bucket, ConnectTimeout: time.Second, OperationTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	makeFile := func(status string, age time.Duration, foreign bool) storage.File {
		t.Helper()
		now := time.Now().UTC().Add(-age)
		file := storage.File{ID: uuid.NewString(), StorageProvider: "MINIO", Bucket: bucket, OriginalName: "private-synthetic-name.txt", SizeBytes: 3, Status: status, RetentionType: storage.RetentionTemporary, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
		file.ObjectKey = "synthetic-private/" + file.ID
		if foreign {
			file.Bucket = "another-bucket"
		}
		if err := db.CreateFile(ctx, file); err != nil {
			t.Fatal(err)
		}
		if _, err := objects.Put(ctx, file.ObjectKey, strings.NewReader("abc"), 3, "text/plain"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := objects.Remove(ctx, file.ObjectKey); err != nil {
				t.Error(err)
			}
		})
		return file
	}
	failed := makeFile(storage.StatusFailed, 3*time.Hour, false)
	pending := makeFile(storage.StatusPending, 2*time.Hour, false)
	protected := []storage.File{makeFile(storage.StatusFailed, time.Minute, false), makeFile(storage.StatusAvailable, 4*time.Hour, false), makeFile(storage.StatusMissing, 4*time.Hour, false), makeFile(storage.StatusFailed, 4*time.Hour, true)}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := db.Pool.QueryRow(ctx, `SELECT md5((SELECT string_agg(to_jsonb(f)::text,'' ORDER BY id) FROM files f)) || ':' || (SELECT count(*) FROM audit_logs)::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("MINIO_BUCKET", bucket)
	t.Setenv("MINIO_ENDPOINT", "preview-must-not-connect.invalid")
	t.Setenv("MINIO_SECRET_KEY", "")
	binary := filepath.Join(t.TempDir(), "reconcile-files")
	build := exec.Command("go", "build", "-o", binary, ".")
	if diagnostic, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build recovery command: %v %s", err, diagnostic)
	}
	before := snapshot()
	var output bytes.Buffer
	previewCommand := exec.CommandContext(ctx, binary)
	previewCommand.Stdout = &output
	if err := previewCommand.Run(); err != nil {
		t.Fatal("preview command failed", err)
	}
	var preview struct {
		Mode       string
		Count      int
		Candidates []candidateSummary
	}
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil || preview.Mode != "preview" || preview.Count != 2 || len(preview.Candidates) != 2 || preview.Candidates[0].ID != failed.ID || preview.Candidates[1].ID != pending.ID {
		t.Fatal("incorrect preview", err)
	}
	if snapshot() != before || strings.Contains(output.String(), "private-synthetic") || strings.Contains(output.String(), "synthetic-private/") {
		t.Fatal("preview changed state or exposed file details")
	}
	for key, value := range map[string]string{"MINIO_ENDPOINT": endpoint, "MINIO_ACCESS_KEY": access, "MINIO_SECRET_KEY": secret, "MINIO_USE_TLS": "false", "MINIO_MAX_FILE_SIZE_BYTES": "16777216"} {
		t.Setenv(key, value)
	}
	args := []string{"--apply", "--actor-id", actor, "--reason", "isolated recovery verification", "--limit", "1"}
	for _, state := range []struct {
		status            string
		super, mustChange bool
	}{{"DISABLED", true, false}, {"ACTIVE", false, false}, {"ACTIVE", true, true}} {
		if _, err := db.Pool.Exec(ctx, `UPDATE accounts SET status=$2,is_super_admin=$3,must_change_password=$4 WHERE id=$1`, actor, state.status, state.super, state.mustChange); err != nil {
			t.Fatal(err)
		}
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatal("unauthorized operator accepted")
		}
		if snapshot() != before {
			t.Fatal("unauthorized execution changed files or audit")
		}
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE accounts SET status='ACTIVE',is_super_admin=true,must_change_password=false WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `CREATE FUNCTION reject_recovery_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='FILE_OBJECT_REMOVED' THEN RAISE EXCEPTION 'private-diagnostic'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_recovery_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_recovery_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_recovery_audit ON audit_logs; DROP FUNCTION IF EXISTS reject_recovery_audit()`); err != nil {
			t.Error(err)
		}
	})
	output.Reset()
	applyCommand := exec.CommandContext(ctx, binary, args...)
	applyCommand.Stdout = &output
	var diagnostics bytes.Buffer
	applyCommand.Stderr = &diagnostics
	runErr := applyCommand.Run()
	var exitError *exec.ExitError
	if !errors.As(runErr, &exitError) || exitError.ExitCode() != 1 || strings.Contains(diagnostics.String(), "private-diagnostic") {
		t.Fatal("command did not report safe failing exit status", runErr)
	}
	var result struct {
		Mode                       string
		Recovered, Cleaned, Failed int
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || runErr == nil || result.Failed != 1 || result.Cleaned != 0 || strings.Contains(runErr.Error(), "private-diagnostic") {
		t.Fatal("partial failure not safely reported", err, runErr)
	}
	if _, err := objects.Stat(ctx, failed.ObjectKey); !errors.Is(err, storage.ErrObjectMissing) {
		t.Fatal("physical deletion not exercised", err)
	}
	if _, err := db.Pool.Exec(ctx, `DROP TRIGGER reject_recovery_audit ON audit_logs; DROP FUNCTION reject_recovery_audit()`); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Cleaned != 1 || result.Recovered != 0 || result.Failed != 0 {
		t.Fatal("failed-file retry did not complete", err)
	}
	output.Reset()
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Recovered != 1 || result.Cleaned != 0 || result.Failed != 0 {
		t.Fatal("bounded pending recovery failed", err)
	}
	stored, err := db.FindFile(ctx, pending.ID)
	if err != nil || stored.Status != storage.StatusAvailable || len(stored.SHA256) != 64 {
		t.Fatal("pending source not recovered", err)
	}
	for _, file := range protected {
		stored, err := db.FindFile(ctx, file.ID)
		if err != nil || stored.Status != file.Status {
			t.Fatal("out-of-scope metadata changed", err)
		}
		if _, err := objects.Stat(ctx, file.ObjectKey); err != nil {
			t.Fatal("out-of-scope object removed", err)
		}
	}
	var audited int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND reason='isolated recovery verification' AND request_id<>'' AND action IN ('FILE_OBJECT_REMOVED','FILE_UPLOAD_SUCCEEDED')`, actor).Scan(&audited); err != nil || audited != 2 {
		t.Fatal("missing recovery audit", err, audited)
	}
	output.Reset()
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Recovered+result.Cleaned+result.Failed != 0 {
		t.Fatal("repeat reprocessed completed files", err)
	}
}
