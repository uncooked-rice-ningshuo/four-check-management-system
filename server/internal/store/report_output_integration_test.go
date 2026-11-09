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
	versionID := uuid.NewString()
	if err := store.RecordTransientOutput(ctx, reportoutput.File{ReportVersionID: versionID, Format: "WORD", TypeCodes: []string{"LATE_RETURN"}, Combined: true, SettingsVersion: 1}, 1024, audit.Metadata{ActorID: actorID}); err != nil {
		t.Fatal(err)
	}
	var outputRows, outputAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM report_output_files`).Scan(&outputRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='REPORT_OUTPUT_CHANGED' AND resource_type='reportOutputDownload' AND resource_id=$2`, actorID, versionID).Scan(&outputAudits); err != nil || outputRows != 0 || outputAudits != 1 {
		t.Fatalf("transient output rows=%d audits=%d err=%v", outputRows, outputAudits, err)
	}
	yearID, semesterID, weekID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,'虚构测试学年','2025-09-01','2026-08-31')`, yearID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,'TEST','虚构第二学期','2026-02-01','2026-07-01')`, semesterID, yearID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date) VALUES($1,$2,8,'2026-03-22','2026-03-28')`, weekID, semesterID); err != nil {
		t.Fatal(err)
	}
	weekDates, err := store.ListWeekDates(ctx)
	if err != nil || len(weekDates) != 1 || weekDates[0].IssueDate != "" || weekDates[0].Version != 0 {
		t.Fatalf("unconfigured week dates: %+v err=%v", weekDates, err)
	}
	date, err := store.SaveWeekDate(ctx, reportoutput.WeekDate{SemesterID: semesterID, WeekID: weekID, IssueDate: "2026-03-30", Version: 0}, audit.Metadata{ActorID: actorID})
	if err != nil || date.Version != 1 {
		t.Fatalf("save week date: %+v err=%v", date, err)
	}
	if _, err := store.SaveWeekDate(ctx, reportoutput.WeekDate{SemesterID: semesterID, WeekID: weekID, IssueDate: "2026-04-01", Version: 0}, audit.Metadata{ActorID: actorID}); !errors.Is(err, reportoutput.ErrConfigConflict) {
		t.Fatalf("stale week date: %v", err)
	}
	date.IssueDate = "2026-04-02"
	updatedDate, err := store.SaveWeekDate(ctx, date, audit.Metadata{ActorID: actorID})
	if err != nil || updatedDate.Version != 2 {
		t.Fatalf("update week date: %+v err=%v", updatedDate, err)
	}
	readDate, err := store.GetWeekDate(ctx, semesterID, weekID)
	if err != nil || readDate.IssueDate != "2026-04-02" || readDate.Version != 2 {
		t.Fatalf("read week date: %+v err=%v", readDate, err)
	}
	var dateAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1::uuid AND action='REPORT_OUTPUT_CHANGED' AND resource_type='reportOutputWeekDate' AND resource_id=$2::text`, actorID, weekID).Scan(&dateAudits); err != nil || dateAudits != 2 {
		t.Fatalf("week date audits=%d err=%v", dateAudits, err)
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
