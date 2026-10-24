package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/academicimport"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
)

func TestAcademicWeekImportBatchIsAtomicAndIdempotentAgainstPostgres(t *testing.T) {
	database := openTestPostgres(t)
	ctx := context.Background()
	actorID, yearID, semesterID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	unique := uuid.NewString()[:8]
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
		VALUES($1,$2,'周次导入测试','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, actorID, "week-import-"+unique); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,$2,'2098-01-01','2098-12-31')`, yearID, "周次导入学年-"+unique); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,'FIRST',$3,'2098-01-01','2098-06-30')`, semesterID, yearID, "周次导入学期-"+unique); err != nil {
		t.Fatal(err)
	}
	seedFile := func() string {
		fileID := uuid.NewString()
		if _, err := database.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by)
			VALUES($1,'test',$2,'weeks.xlsx',1,$3,'AVAILABLE','IMPORT_SOURCE',$4)`, fileID, "academic-week-import/"+fileID, strings.Repeat("a", 64), actorID); err != nil {
			t.Fatal(err)
		}
		return fileID
	}
	metadata := audit.Metadata{ActorID: actorID, RequestID: "academic-week-import-test"}
	makeBatch := func(weeks ...academic.Week) academicimport.Batch {
		rows := make([]academicimport.PreviewRow, len(weeks))
		for index, week := range weeks {
			week.SemesterID, week.Enabled = semesterID, true
			if week.ID == "" {
				week.ID = uuid.NewString()
			}
			if week.AttendancePenaltyMultiplier == nil {
				multiplier := 1
				week.AttendancePenaltyMultiplier = &multiplier
			}
			rows[index] = academicimport.PreviewRow{RowNo: index + 5, Status: academicimport.RowValid, Errors: []academicimport.RowError{}, Week: &week}
		}
		return academicimport.Batch{
			ID: uuid.NewString(), Status: academicimport.StatusPreviewReady, Version: 1, TemplateVersion: academicimport.TemplateVersion,
			SemesterID: semesterID, SemesterName: "周次导入学期-" + unique, AcademicYearName: "周次导入学年-" + unique,
			TotalRows: len(rows), ValidRows: len(rows), Rows: rows, SourceFileID: seedFile(), CreatedBy: actorID, CreatedAt: time.Now().UTC(),
		}
	}
	date := func(value string) time.Time { result, _ := time.Parse("2006-01-02", value); return result }
	first := makeBatch(
		academic.Week{WeekNo: 1, StartDate: date("2098-02-01"), EndDate: date("2098-02-07")},
		academic.Week{WeekNo: 2, StartDate: date("2098-02-08"), EndDate: date("2098-02-14"), Note: "第二周"},
	)
	if err := database.CreateAcademicWeekImportBatch(ctx, first, metadata); err != nil {
		t.Fatal(err)
	}
	input := academicimport.ConfirmInput{BatchID: first.ID, ActorID: actorID, ExpectedVersion: 1, IdempotencyKey: "week-import-key-" + unique, Metadata: metadata}
	result, err := database.ConfirmAcademicWeekImportBatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := database.ConfirmAcademicWeekImportBatch(ctx, input)
	if err != nil || retry != result {
		t.Fatalf("idempotent retry = %+v, %v; first result = %+v", retry, err, result)
	}
	if _, err := database.ConfirmAcademicWeekImportBatch(ctx, academicimport.ConfirmInput{
		BatchID: first.ID, ActorID: actorID, ExpectedVersion: 1, IdempotencyKey: "different-week-import-key-" + unique, Metadata: metadata,
	}); !errors.Is(err, academicimport.ErrAlreadyConfirmed) {
		t.Fatalf("different idempotency key error = %v", err)
	}
	if _, err := database.FindAcademicWeekImportBatch(ctx, first.ID, uuid.NewString()); !errors.Is(err, academicimport.ErrBatchNotFound) {
		t.Fatalf("other creator lookup error = %v", err)
	}
	var imported, weekAudit, batchAudit int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM semester_weeks WHERE semester_id=$1 AND week_no IN (1,2)`, semesterID).Scan(&imported); err != nil || imported != 2 {
		t.Fatalf("imported weeks = %d, %v", imported, err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND resource_type='semesterWeek' AND action=$2`, actorID, audit.ActionAcademicCalendarChanged).Scan(&weekAudit); err != nil || weekAudit != 2 {
		t.Fatalf("week audit count = %d, %v", weekAudit, err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND resource_type='academic_week_import_batch' AND resource_id=$2`, actorID, first.ID).Scan(&batchAudit); err != nil || batchAudit != 2 {
		t.Fatalf("batch audit count = %d, %v", batchAudit, err)
	}
	file, err := database.FindFile(ctx, first.SourceFileID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MarkImportSourceDiscarded(ctx, file, audit.Mutation{Metadata: metadata, Entry: audit.Entry{Action: audit.ActionFileUploadFailed, ResourceType: "file", ResourceID: file.ID}}); !errors.Is(err, storage.ErrFileUnavailable) {
		t.Fatalf("bound import source was discardable: %v", err)
	}

	rollback := makeBatch(
		academic.Week{WeekNo: 7, StartDate: date("2098-03-01"), EndDate: date("2098-03-07")},
		academic.Week{WeekNo: 8, StartDate: date("2098-03-08"), EndDate: date("2098-03-14")},
	)
	if err := database.CreateAcademicWeekImportBatch(ctx, rollback, metadata); err != nil {
		t.Fatal(err)
	}
	functionName, triggerName := "reject_week_audit_"+strings.ReplaceAll(rollback.ID, "-", ""), "reject_week_audit_trigger_"+strings.ReplaceAll(rollback.ID, "-", "")
	secondWeekID := rollback.Rows[1].Week.ID
	createFunction := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$ BEGIN
		IF NEW.resource_type='semesterWeek' AND NEW.resource_id='%s' THEN RAISE EXCEPTION 'isolated audit failure'; END IF;
		RETURN NEW; END $body$`, functionName, secondWeekID)
	if _, err := database.Pool.Exec(ctx, createFunction); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION %s()", triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	dropTrigger, dropFunction := fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON audit_logs", triggerName), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName)
	cleanupFault := func() error {
		if _, err := database.Pool.Exec(ctx, dropTrigger); err != nil {
			return err
		}
		_, err := database.Pool.Exec(ctx, dropFunction)
		return err
	}
	defer func() { _ = cleanupFault() }()
	_, rollbackErr := database.ConfirmAcademicWeekImportBatch(ctx, academicimport.ConfirmInput{
		BatchID: rollback.ID, ActorID: actorID, ExpectedVersion: 1, IdempotencyKey: "week-import-audit-failure-" + unique, Metadata: metadata,
	})
	if rollbackErr == nil {
		t.Fatal("confirmation succeeded despite the injected second-week audit failure")
	}
	if err := cleanupFault(); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM semester_weeks WHERE id=$1 OR id=$2`, rollback.Rows[0].Week.ID, rollback.Rows[1].Week.ID).Scan(&imported); err != nil || imported != 0 {
		t.Fatalf("audit failure left partial weeks: count=%d err=%v", imported, err)
	}
	rollbackPreview, err := database.FindAcademicWeekImportBatch(ctx, rollback.ID, actorID)
	if err != nil || rollbackPreview.Status != academicimport.StatusPreviewReady || rollbackPreview.Version != 1 {
		t.Fatalf("audit failure changed batch state: %+v, %v", rollbackPreview, err)
	}

	stale := makeBatch(academic.Week{WeekNo: 3, StartDate: date("2098-02-15"), EndDate: date("2098-02-21")})
	if err := database.CreateAcademicWeekImportBatch(ctx, stale, metadata); err != nil {
		t.Fatal(err)
	}
	manualMultiplier := 1
	manualConflict := academic.Week{
		ID: uuid.NewString(), SemesterID: semesterID, WeekNo: 9, StartDate: date("2098-02-15"), EndDate: date("2098-02-21"),
		Enabled: true, AttendancePenaltyMultiplier: &manualMultiplier,
	}
	if _, err := database.CreateWeek(ctx, manualConflict, audit.Mutation{
		Metadata: metadata,
		Entry:    audit.Entry{Action: audit.ActionAcademicCalendarChanged, ResourceType: "semesterWeek", ResourceID: manualConflict.ID, After: manualConflict},
	}); err != nil {
		t.Fatalf("manual edit after preview should establish the simulated conflicting state: %v", err)
	}
	_, err = database.ConfirmAcademicWeekImportBatch(ctx, academicimport.ConfirmInput{
		BatchID: stale.ID, ActorID: actorID, ExpectedVersion: 1, IdempotencyKey: "stale-week-import-" + unique, Metadata: metadata,
	})
	if !errors.Is(err, academicimport.ErrPreviewStale) {
		t.Fatalf("database conflict created after preview was not rejected: %v", err)
	}
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM semester_weeks WHERE semester_id=$1 AND week_no=3`, semesterID).Scan(&imported); err != nil || imported != 0 {
		t.Fatalf("stale batch partially inserted rows: count=%d err=%v", imported, err)
	}

	manual := academic.Week{ID: uuid.NewString(), SemesterID: semesterID, WeekNo: 4, StartDate: date("2098-06-30"), EndDate: date("2098-07-01"), Enabled: true}
	_, err = database.CreateWeek(ctx, manual, audit.Mutation{Metadata: metadata, Entry: audit.Entry{Action: audit.ActionAcademicCalendarChanged, ResourceType: "semesterWeek", ResourceID: manual.ID, After: manual}})
	if !errors.Is(err, academic.ErrInvalidInput) {
		t.Fatalf("manual week outside semester error = %v", err)
	}
}

func TestConcurrentAcademicWeekImportsCannotCreateOverlappingWeeks(t *testing.T) {
	database := openTestPostgres(t)
	ctx := context.Background()
	actorID, yearID, semesterID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	unique := uuid.NewString()[:8]
	if _, err := database.Pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
		VALUES($1,$2,'周次并发测试','unused','ACTIVE','SYSTEM','SUPER_ADMIN',true,false)`, actorID, "week-race-"+unique); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO academic_years(id,name,start_date,end_date) VALUES($1,$2,'2097-01-01','2097-12-31')`, yearID, "周次并发学年-"+unique); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES($1,$2,'FIRST',$3,'2097-01-01','2097-06-30')`, semesterID, yearID, "周次并发学期-"+unique); err != nil {
		t.Fatal(err)
	}
	metadata := audit.Metadata{ActorID: actorID, RequestID: "academic-week-import-race"}
	makeBatch := func(weekNo int) academicimport.Batch {
		fileID := uuid.NewString()
		if _, err := database.Pool.Exec(ctx, `INSERT INTO files(id,bucket,object_key,original_name,size_bytes,sha256,status,retention_type,created_by)
			VALUES($1,'test',$2,'weeks.xlsx',1,$3,'AVAILABLE','IMPORT_SOURCE',$4)`, fileID, "academic-week-race/"+fileID, strings.Repeat("c", 64), actorID); err != nil {
			t.Fatal(err)
		}
		multiplier := 1
		week := academic.Week{ID: uuid.NewString(), SemesterID: semesterID, WeekNo: weekNo, StartDate: time.Date(2097, 3, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2097, 3, 7, 0, 0, 0, 0, time.UTC), Enabled: true, AttendancePenaltyMultiplier: &multiplier}
		return academicimport.Batch{ID: uuid.NewString(), Status: academicimport.StatusPreviewReady, Version: 1, TemplateVersion: academicimport.TemplateVersion,
			SemesterID: semesterID, SemesterName: "并发测试学期", AcademicYearName: "并发测试学年", TotalRows: 1, ValidRows: 1,
			Rows:         []academicimport.PreviewRow{{RowNo: 5, Status: academicimport.RowValid, Errors: []academicimport.RowError{}, Week: &week}},
			SourceFileID: fileID, CreatedBy: actorID, CreatedAt: time.Now().UTC()}
	}
	first, second := makeBatch(1), makeBatch(2)
	for _, batch := range []academicimport.Batch{first, second} {
		if err := database.CreateAcademicWeekImportBatch(ctx, batch, metadata); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	errorsByBatch := make([]error, 2)
	var wait sync.WaitGroup
	for index, batch := range []academicimport.Batch{first, second} {
		wait.Add(1)
		go func(index int, batch academicimport.Batch) {
			defer wait.Done()
			<-start
			_, errorsByBatch[index] = database.ConfirmAcademicWeekImportBatch(ctx, academicimport.ConfirmInput{
				BatchID: batch.ID, ActorID: actorID, ExpectedVersion: 1, IdempotencyKey: "week-import-race-key-" + batch.ID[:12], Metadata: metadata,
			})
		}(index, batch)
	}
	close(start)
	wait.Wait()
	successes, stale := 0, 0
	for _, err := range errorsByBatch {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, academicimport.ErrPreviewStale):
			stale++
		default:
			t.Fatalf("concurrent import confirmation error = %v", err)
		}
	}
	var persisted int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM semester_weeks WHERE semester_id=$1 AND start_date='2097-03-01'`, semesterID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || stale != 1 || persisted != 1 {
		t.Fatalf("concurrent outcomes successes=%d stale=%d persisted=%d", successes, stale, persisted)
	}
}
