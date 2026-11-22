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
	t.Run("published discipline template and stale save", func(t *testing.T) {
		rule := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO discipline_rule_versions(id,version,status,created_by,published_by,published_at) VALUES($1,1,'PUBLISHED',$2,$2,now())`, rule, actorID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO discipline_rule_current(version_id) VALUES($1)`, rule); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO discipline_rule_groups(version_id,group_key,name,sort_order) VALUES($1,'evening','晚自习累计',1)`, rule); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO discipline_rule_tiers(group_id,code,name,lower_bound,sort_order,teacher_confirmation_required)
 SELECT id,k,k,n,n,false FROM discipline_rule_groups CROSS JOIN (VALUES('FIRST',1),('SECOND',2)) AS tiers(k,n) WHERE version_id=$1`, rule); err != nil {
			t.Fatal(err)
		}
		entry := reportoutput.DisciplineTemplate{RuleID: rule, GroupKey: "evening", TierCode: "FIRST", Document: json.RawMessage(`{"schemaVersion":2,"content":[{"type":"paragraph","content":[{"type":"templateField","attrs":{"key":"discipline.studentName"}}]}]}`)}
		if err := store.SaveDocumentTemplate(ctx, entry, audit.Metadata{ActorID: actorID}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveDocumentTemplate(ctx, entry, audit.Metadata{ActorID: actorID}); !errors.Is(err, reportoutput.ErrConfigConflict) {
			t.Fatalf("stale save: %v", err)
		}
		list, err := store.ListDocumentTemplates(ctx)
		if err != nil || len(list) != 2 || list[0].Revision != 1 {
			t.Fatalf("saved revision: %+v %v", list, err)
		}
		entry.TierCode = "SECOND"
		entry.SourceTemplateID = list[0].TemplateID
		if err := store.SaveDocumentTemplate(ctx, entry, audit.Metadata{ActorID: actorID}); !errors.Is(err, reportoutput.ErrTemplateUnchanged) {
			t.Fatalf("unchanged copy: %v", err)
		}
		entry.Document = json.RawMessage(`{"schemaVersion":2,"orientation":"landscape","content":[{"type":"paragraph","attrs":{"border":"topBottom"},"content":[{"type":"templateField","attrs":{"key":"discipline.studentName"}}]}]}`)
		if err := store.SaveDocumentTemplate(ctx, entry, audit.Metadata{ActorID: actorID}); err != nil {
			t.Fatal(err)
		}
		list, err = store.ListDocumentTemplates(ctx)
		if err != nil || list[1].Revision != 1 || list[0].TemplateID == list[1].TemplateID {
			t.Fatalf("tier copy: %v %v", list, err)
		}

	})
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
	t.Run("approved case generates Word from snapshot; pending case is refused", func(t *testing.T) {
		report := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO weekly_reports(id,semester_id,week_id,created_by,updated_by) VALUES($1,$2,$3,$4,$4)`, report, semesterID, weekID, actorID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO weekly_report_versions(id,report_id,version_no,content_json,source_fingerprint,created_by) VALUES($1,$2,1,'{}',repeat('0',64),$3)`, versionID, report, actorID); err != nil {
			t.Fatal(err)
		}
		for _, status := range []string{"APPROVED", "PENDING"} {
			id := uuid.NewString()
			if _, err := pool.Exec(ctx, `INSERT INTO discipline_cases(id,semester_id,student_number,group_key,tier_code,report_version_id,rule_version_id,accumulated_amount,evidence,status,decision_source,decided_at,due_at)
                SELECT $1,$2,$3,'evening','FIRST',$4,version_id,3,'{"student":{"name":"虚构学生"}}',$5,CASE WHEN $5='APPROVED' THEN 'RULE' END,CASE WHEN $5='APPROVED' THEN now() END,CASE WHEN $5='PENDING' THEN now()+interval '48 hours' END FROM discipline_rule_current`, id, semesterID, status, versionID, status); err != nil {
				t.Fatal(err)
			}
			_, data, err := store.ProcessDisciplineDocument(ctx, id, reportoutput.DocumentRequest{}, audit.Metadata{ActorID: actorID}, true)
			if status == "PENDING" {
				if !errors.Is(err, reportoutput.ErrNotFound) {
					t.Fatalf("pending case: %v", err)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := reportoutput.ValidateGeneratedFile("WORD", data); err != nil {
				t.Fatal(err)
			}
			if path := os.Getenv("DISCIPLINE_WORD_SAMPLE_PATH"); path != "" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
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
