package importjob_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/store"

	"github.com/google/uuid"
)

func TestManualAttendancePostgresIntegration(t *testing.T) {
	url := os.Getenv("MANUAL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MANUAL_TEST_DATABASE_URL must name an empty disposable database")
	}
	ctx := context.Background()
	applyIntegrationMigrations(t, ctx, url)
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := seedAttendanceIntegrationFixture(t, ctx, db)
	workbook, err := importjob.NewAttendanceXLSXParser(16<<20).Parse("manual.xlsx", fixture.workbook)
	if err != nil {
		t.Fatal(err)
	}
	row := workbook.Rows[0].Values
	var classID string
	if err = db.Pool.QueryRow(ctx, `SELECT id FROM classes WHERE name=$1`, row["class_name"]).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	input := inspection.ManualAttendanceInput{SemesterID: fixture.semesterID, WeekID: fixture.weekID, InspectionTypeID: integrationInspectionTypeID, DepartmentID: integrationDepartmentID, ClassID: classID, Grade: "2026级", ClassName: row["class_name"], StudentNo: row["student_no"], StudentName: row["student_name"], CourseName: row["course_name"], Classroom: row["classroom_code"], Date: row["absence_date"], Time: row["absence_time"], Period: row["period"], AbsenceHours: 2}
	service := inspection.NewManualService(db, importjob.NewAttendanceAdapter(db), academic.NewService(db), organization.NewService(db), permission.NewService(db))
	actor := inspection.ManualActor{ID: fixture.accountID, IsSuperAdmin: true}
	created, err := service.Create(ctx, actor, input, audit.Metadata{RequestID: "manual-integration"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Source != "MANUAL" || created.Status != "DRAFT" || created.ImportBatchID != "" || created.CreatedAt.IsZero() {
		t.Fatalf("wrong draft metadata: %+v", created)
	}
	if created.PenaltyMultiplier == nil || *created.PenaltyMultiplier != 1 || created.PenaltyHours == nil || *created.PenaltyHours != 2 {
		t.Fatalf("manual penalty snapshot: %+v", created)
	}
	if _, err = service.Create(ctx, actor, input, audit.Metadata{}); !errors.Is(err, inspection.ErrAttendanceDuplicate) {
		t.Fatalf("duplicate=%v", err)
	}
	// Bypass the preflight duplicate lookup to exercise the database race guard.
	duplicate := created
	duplicate.ID = uuid.NewString()
	if _, err = db.SaveManualAttendance(ctx, duplicate, audit.Mutation{}); !errors.Is(err, inspection.ErrAttendanceDuplicate) {
		t.Fatalf("unique index=%v", err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE accounts SET system_role='MEMBER', is_super_admin=false WHERE id=$1`, fixture.accountID); err != nil {
		t.Fatal(err)
	}
	actor.IsSuperAdmin = false
	if _, err = service.Create(ctx, actor, input, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
		t.Fatalf("unauthorized=%v", err)
	}
	actor.IsSuperAdmin = true
	invalid := input
	invalid.StudentName = "不匹配的虚构姓名"
	var validation *inspection.ManualValidationError
	if _, err = service.Create(ctx, actor, invalid, audit.Metadata{}); !errors.As(err, &validation) {
		t.Fatalf("reference mismatch=%v", err)
	}
	invalid = input
	invalid.Date = "2026-07-01"
	if _, err = service.Create(ctx, actor, invalid, audit.Metadata{}); !errors.As(err, &validation) {
		t.Fatalf("out of week=%v", err)
	}
	invalid = input
	invalid.DepartmentID = uuid.NewString()
	if _, err = service.Create(ctx, actor, invalid, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
		t.Fatalf("unconfigured department=%v", err)
	}
	// Revalidate a reference changed after application-level validation.
	if _, err = db.Pool.Exec(ctx, `UPDATE classrooms SET enabled=false WHERE id=$1`, created.ClassroomID); err != nil {
		t.Fatal(err)
	}
	stale := created
	stale.ID = uuid.NewString()
	if _, err = db.SaveManualAttendance(ctx, stale, audit.Mutation{}); !errors.Is(err, inspection.ErrInvalidAttendanceRecord) {
		t.Fatalf("stale classroom=%v", err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE classrooms SET enabled=true WHERE id=$1`, created.ClassroomID); err != nil {
		t.Fatal(err)
	}
	// Audit failure must roll back both inserted record and attendance detail.
	rollback := created
	rollback.ID = uuid.NewString()
	rollback.PeriodText = "第1节"
	rollback.DedupKey = inspection.AttendanceDedupKey(rollback)
	if _, err = db.SaveManualAttendance(ctx, rollback, audit.Mutation{}); err == nil {
		t.Fatal("expected audit validation failure")
	}
	var records, details, audits int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM inspection_records WHERE created_by=$1`, fixture.accountID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM attendance_details a JOIN inspection_records r ON r.id=a.record_id WHERE r.created_by=$1`, fixture.accountID).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='INSPECTION_RECORD_CREATED'`, created.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if records != 1 || details != 1 || audits != 1 {
		t.Fatalf("records=%d details=%d audits=%d", records, details, audits)
	}
}
