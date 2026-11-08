package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/reportoutput"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestReportOutputSettingsPostgresMigrationAndVersion(t *testing.T) {
	databaseURL := os.Getenv("REPORT_OUTPUT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REPORT_OUTPUT_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatal("requires an empty disposable database", err)
	}
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range migrations {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(contents)); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), err)
		}
	}
	store := &Postgres{Pool: pool}
	settings, err := store.GetOutputSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Version != 1 || len(settings.IntroductionDocument) != 0 {
		t.Fatalf("legacy settings: %+v", settings)
	}
	settings.Introduction = "虚构说明"
	settings.IntroductionDocument = json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","attrs":{"style":"body","textAlign":"right"},"content":[{"type":"text","text":"虚构说明"}]}]}`)
	updated, err := store.ReplaceOutputSettings(ctx, settings, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d", updated.Version)
	}
	read, err := store.GetOutputSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var savedDocument, requestedDocument any
	if err := json.Unmarshal(read.IntroductionDocument, &savedDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(settings.IntroductionDocument, &requestedDocument); err != nil {
		t.Fatal(err)
	}
	if read.Version != 2 || !reflect.DeepEqual(savedDocument, requestedDocument) {
		t.Fatalf("stored settings: %+v", read)
	}
	if _, err := store.ReplaceOutputSettings(ctx, settings, audit.Metadata{}); !errors.Is(err, reportoutput.ErrConfigConflict) {
		t.Fatalf("stale save: %v", err)
	}
	actorID := uuid.NewString()
	hash, err := bcrypt.GenerateFromPassword([]byte("FictionalPreviewPassword123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password) VALUES($1,$2,'预览测试账号',$3,'ACTIVE','SYSTEM','MEMBER',false,false)`, actorID, "preview-"+actorID, string(hash)); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		if err := store.RecordOutputPreview(ctx, []string{"LATE_RETURN"}, audit.Metadata{ActorID: actorID}); err != nil {
			t.Fatalf("preview %d: %v", index, err)
		}
	}
	if err := store.RecordOutputPreview(ctx, []string{"LATE_RETURN"}, audit.Metadata{ActorID: actorID}); !errors.Is(err, reportoutput.ErrPreviewRateLimited) {
		t.Fatalf("eleventh preview: %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='REPORT_OUTPUT_PREVIEWED' AND after_json::text NOT LIKE '%虚构说明%'`, actorID).Scan(&auditCount); err != nil || auditCount != 10 {
		t.Fatalf("preview audits=%d err=%v", auditCount, err)
	}
}
