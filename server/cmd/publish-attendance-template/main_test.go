package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestPreviewRequiresNoDatabaseAndWritesParserCompatibleFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "template.xlsx")
	var output bytes.Buffer
	if err := run([]string{"--output", path}, &output); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Mode   string `json:"mode"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || report.Mode != "preview" || len(report.SHA256) != 64 {
		t.Fatal(output.String(), err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("template.xlsx", content); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--output", path}, &output); err == nil {
		t.Fatal("overwrote an existing file")
	}
}

func TestApplyRequiresExplicitAuditIdentityAndReason(t *testing.T) {
	for _, args := range [][]string{{"--apply"}, {"--apply", "--actor-id", "not-a-uuid", "--reason", "test"}, {"--apply", "--actor-id", "10000000-0000-0000-0000-000000000001"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatal("accepted incomplete publication request")
		}
	}
}

func TestApplyAgainstDisposablePostgresAndMinIO(t *testing.T) {
	databaseURL := os.Getenv("TEMPLATE_CLI_TEST_DATABASE_URL")
	endpoint, access, secret := os.Getenv("TEST_MINIO_ENDPOINT"), os.Getenv("TEST_MINIO_ACCESS_KEY"), os.Getenv("TEST_MINIO_SECRET_KEY")
	if databaseURL == "" || endpoint == "" || access == "" || secret == "" {
		t.Skip("requires disposable PostgreSQL/MinIO")
	}
	ctx := context.Background()
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var exists bool
	if err := database.Pool.QueryRow(ctx, `SELECT to_regclass('public.accounts') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("requires empty disposable database", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatal("missing migrations", err)
	}
	for _, path := range migrations {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Pool.Exec(ctx, string(content)); err != nil {
			t.Fatal(err)
		}
	}
	actor := uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,'template-cli-fixture','Synthetic operator','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, actor); err != nil {
		t.Fatal(err)
	}
	admin, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "template-cli-" + uuid.NewString()
	if err := admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for object := range admin.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err != nil {
				t.Error("list disposable objects failed", object.Err)
				continue
			}
			if err := admin.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Error("remove disposable object failed", err)
			}
		}
		if err := admin.RemoveBucket(ctx, bucket); err != nil {
			t.Error("remove disposable bucket failed", err)
		}
	}()
	for key, value := range map[string]string{"DATABASE_URL": databaseURL, "MINIO_ENDPOINT": endpoint, "MINIO_ACCESS_KEY": access, "MINIO_SECRET_KEY": secret, "MINIO_BUCKET": bucket, "MINIO_USE_TLS": "false"} {
		t.Setenv(key, value)
	}
	var first bytes.Buffer
	args := []string{"--apply", "--actor-id", actor, "--reason", "disposable CLI verification"}
	if err := run(args, &first); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := run(args, &second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("CLI repeat changed publication result")
	}
	var templates, files, audits int
	if err := database.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM import_templates WHERE status='PUBLISHED'),(SELECT count(*) FROM files),(SELECT count(*) FROM audit_logs WHERE action='IMPORT_TEMPLATE_PUBLISHED')`).Scan(&templates, &files, &audits); err != nil || templates != 1 || files != 1 || audits != 1 {
		t.Fatal("unexpected publication state", templates, files, audits, err)
	}
	if _, err := database.Pool.Exec(ctx, `UPDATE accounts SET status='DISABLED' WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &bytes.Buffer{}); err == nil {
		t.Fatal("disabled operator was accepted")
	}
}

func TestUnattendedPreviewIsTypeIsolated(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "unattended.xlsx")
	if err := run([]string{"--type-code", "UNATTENDED_CLASSROOM", "--output", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importjob.NewUnattendedXLSXParser(16<<20).Parse("unattended.xlsx", content); err != nil {
		t.Fatal(err)
	}
	if _, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("unattended.xlsx", content); err == nil {
		t.Fatal("unattended accepted as attendance")
	}
	if err := run([]string{"--type-code", "PUBLIC_DORMITORY_HYGIENE", "--output", filepath.Join(t.TempDir(), "unsupported.xlsx")}, &bytes.Buffer{}); err == nil {
		t.Fatal("unsupported type accepted")
	}
}

func TestMorningPreviewKeepsDateOnlyContract(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "morning.xlsx")
	if err := run([]string{"--type-code", "MORNING_ATTENDANCE", "--output", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := importjob.NewMorningXLSXParser(16<<20).Parse("morning.xlsx", content)
	if err != nil || len(parsed.Fields) != 5 {
		t.Fatal(parsed, err)
	}
}

func TestLateReturnPreviewKeepsEventContract(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "late-return.xlsx")
	if err := run([]string{"--type-code", "LATE_RETURN", "--output", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := importjob.NewLateReturnXLSXParser(16<<20).Parse("late-return.xlsx", content)
	if err != nil || len(parsed.Fields) != 7 {
		t.Fatal(parsed, err)
	}
}

func TestDormitoryHygienePreviewKeepsObjectContract(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "hygiene.xlsx")
	if err := run([]string{"--type-code", "DORMITORY_HYGIENE", "--output", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := importjob.NewInspectionXLSXParser("DORMITORY_HYGIENE", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("hygiene.xlsx", content)
	if err != nil || len(parsed.Fields) != 10 || parsed.TemplateCode != "DORMITORY_HYGIENE_IMPORT" {
		t.Fatal(parsed, err)
	}
}

func TestClassroomHygienePreviewKeepsObjectContract(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "hygiene.xlsx")
	if err := run([]string{"--type-code", "CLASSROOM_HYGIENE", "--output", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := importjob.NewInspectionXLSXParser("CLASSROOM_HYGIENE", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.Parse("hygiene.xlsx", content)
	if err != nil || len(parsed.Fields) != 11 || parsed.TemplateCode != "CLASSROOM_HYGIENE_IMPORT" {
		t.Fatal(parsed, err)
	}
}
