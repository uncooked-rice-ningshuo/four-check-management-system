package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWeeklyReportSummaryPostgres(t *testing.T) {
	databaseURL := os.Getenv("REPORTING_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REPORTING_TEST_DATABASE_URL must point to an empty disposable database")
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
	for _, migration := range migrations {
		contents, readErr := os.ReadFile(migration)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := pool.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("migration %s: %v", filepath.Base(migration), execErr)
		}
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	assertReportPermissionPredicate(t, pool)

	const (
		semesterID = "a0000000-0000-0000-0000-000000000003"
		weekID     = "a0000000-0000-0000-0000-000000000004"
		department = "10000000-0000-0000-0000-000000000002"
		attendance = "30000000-0000-0000-0000-000000000001"
	)
	_, err = pool.Exec(ctx, `
INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES
 ('b0000000-0000-0000-0000-000000000001',$1,$2,$3,$4,'CONFIRMED',1,'a0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000001',repeat('a',64),now()),
 ('b0000000-0000-0000-0000-000000000002',$1,$2,$3,$4,'PENDING_REVIEW',1,'a0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000001',repeat('b',64),now()),
 ('b0000000-0000-0000-0000-000000000003',$1,$2,$3,$4,'RETURNED',1,'a0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000001',repeat('c',64),now()),
 ('b0000000-0000-0000-0000-000000000004',$1,$2,$3,$4,'DRAFT',0,'a0000000-0000-0000-0000-000000000001',NULL,repeat('d',64),NULL),
 ('b0000000-0000-0000-0000-000000000005',$1,$2,NULL,$4,'CONFIRMED',1,'a0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000001',repeat('e',64),now())`, semesterID, weekID, department, attendance)
	if err != nil {
		t.Fatal("insert synthetic submission batches:", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES
 ('c0000000-0000-0000-0000-000000000001','b0000000-0000-0000-0000-000000000001',1,'a0000000-0000-0000-0000-000000000001',ARRAY['a0000000-0000-0000-0000-000000000014'::uuid,'c0000000-0000-0000-0000-000000000011'::uuid],'[]'::jsonb),
 ('c0000000-0000-0000-0000-000000000002','b0000000-0000-0000-0000-000000000002',1,'a0000000-0000-0000-0000-000000000001',ARRAY['c0000000-0000-0000-0000-000000000021'::uuid,'c0000000-0000-0000-0000-000000000022'::uuid,'c0000000-0000-0000-0000-000000000023'::uuid],'[]'::jsonb),
 ('c0000000-0000-0000-0000-000000000003','b0000000-0000-0000-0000-000000000003',1,'a0000000-0000-0000-0000-000000000001',ARRAY['c0000000-0000-0000-0000-000000000031'::uuid,'c0000000-0000-0000-0000-000000000032'::uuid],'[]'::jsonb),
 ('c0000000-0000-0000-0000-000000000005','b0000000-0000-0000-0000-000000000005',1,'a0000000-0000-0000-0000-000000000001',ARRAY['c0000000-0000-0000-0000-000000000051'::uuid],'[]'::jsonb)`)
	if err != nil {
		t.Fatal("insert immutable synthetic revisions:", err)
	}

	repository := &Postgres{Pool: pool}
	profile, err := repository.LoadAccessProfile(ctx, "a0000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("load access profile without a primary department: %v", err)
	}
	if profile.DepartmentID != "" || len(profile.DepartmentScopes) != 0 {
		t.Fatalf("department-less account profile = %+v, want no department scopes", profile)
	}
	departmentGrant := []permission.EffectivePermission{{
		Action: permission.ActionReportView,
		Effect: permission.EffectAllow,
		Scope:  permission.Scope{SemesterID: semesterID, WeekID: weekID, InspectionTypeID: attendance, DepartmentID: department},
	}}
	periods, err := repository.ListReportPeriods(ctx, departmentGrant)
	if err != nil || len(periods) != 1 || periods[0].WeekID != weekID {
		t.Fatalf("department-scoped period list = %+v, %v", periods, err)
	}

	departmentSummary, err := repository.ReportSummary(ctx, semesterID, weekID, departmentGrant)
	if err != nil {
		t.Fatal(err)
	}
	if len(departmentSummary.Rows) != 1 {
		t.Fatalf("department-scoped rows = %+v, want only one department/type cell", departmentSummary.Rows)
	}
	row := departmentSummary.Rows[0]
	if row.DepartmentID != department || row.CollegeLevel || row.SubmissionCount != 4 || row.DraftBatchCount != 1 ||
		row.PendingReviewBatchCount != 1 || row.ReturnedBatchCount != 1 || row.ConfirmedBatchCount != 1 ||
		row.ConfirmedRecordCount != 2 || row.PendingReviewOrReturnedRecordCount != 5 {
		t.Fatalf("department summary row = %+v", row)
	}

	broadGrant := []permission.EffectivePermission{{Action: permission.ActionReportView, Effect: permission.EffectAllow}}
	const collegeOnlyWeekID = "a0000000-0000-0000-0000-000000000015"
	_, err = pool.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date)
VALUES ($1,$2,2,'2026-09-08','2026-09-14')`, collegeOnlyWeekID, semesterID)
	if err != nil {
		t.Fatal("insert college-only reporting week:", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO department_submissions
 (id,semester_id,week_id,department_id,inspection_type_id,status,revision_no,created_by,submitted_by,request_fingerprint,submitted_at)
VALUES ('b0000000-0000-0000-0000-000000000006',$1,$2,NULL,$3,'CONFIRMED',1,
 'a0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000001',repeat('f',64),now())`, semesterID, collegeOnlyWeekID, attendance)
	if err != nil {
		t.Fatal("insert college-only submission:", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO department_submission_revisions(id,submission_id,revision_no,submitted_by,record_ids,records_snapshot)
VALUES ('c0000000-0000-0000-0000-000000000006','b0000000-0000-0000-0000-000000000006',1,
 'a0000000-0000-0000-0000-000000000001',ARRAY['a0000000-0000-0000-0000-000000000014'::uuid],'[]'::jsonb)`)
	if err != nil {
		t.Fatal("insert college-only immutable revision:", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE department_inspection_scopes SET enabled=false`); err != nil {
		t.Fatal("disable department scope for college-only period fixture:", err)
	}
	broadPeriods, err := repository.ListReportPeriods(ctx, broadGrant)
	if err != nil {
		t.Fatal("list broad report periods with a college-only week:", err)
	}
	collegeOnlyWeekVisible := false
	for _, period := range broadPeriods {
		collegeOnlyWeekVisible = collegeOnlyWeekVisible || period.WeekID == collegeOnlyWeekID
	}
	if !collegeOnlyWeekVisible {
		t.Fatalf("broad report periods omitted week with only a college-level submission: %+v", broadPeriods)
	}
	departmentPeriods, err := repository.ListReportPeriods(ctx, []permission.EffectivePermission{{
		Action: permission.ActionReportView,
		Effect: permission.EffectAllow,
		Scope:  permission.Scope{SemesterID: semesterID, InspectionTypeID: attendance, DepartmentID: department},
	}})
	if err != nil {
		t.Fatal("list department-scoped report periods:", err)
	}
	for _, period := range departmentPeriods {
		if period.WeekID == collegeOnlyWeekID {
			t.Fatalf("department-scoped report periods exposed college-only week: %+v", departmentPeriods)
		}
	}

	broadSummary, err := repository.ReportSummary(ctx, semesterID, weekID, broadGrant)
	if err != nil {
		t.Fatal(err)
	}
	var collegeRow *reportSummaryRowSnapshot
	for _, candidate := range broadSummary.Rows {
		if candidate.CollegeLevel && candidate.InspectionTypeID == attendance {
			collegeRow = &reportSummaryRowSnapshot{
				SubmissionCount:      candidate.SubmissionCount,
				ConfirmedBatchCount:  candidate.ConfirmedBatchCount,
				ConfirmedRecordCount: candidate.ConfirmedRecordCount,
			}
		}
		if candidate.CollegeLevel && candidate.InspectionTypeID != attendance {
			t.Fatalf("summary invented a college-level cell without a college submission: %+v", candidate)
		}
		if candidate.InspectionTypeID == "30000000-0000-0000-0000-000000000003" {
			t.Fatalf("paused inspection type appeared in summary: %+v", candidate)
		}
	}
	if collegeRow == nil || collegeRow.SubmissionCount != 1 || collegeRow.ConfirmedBatchCount != 1 || collegeRow.ConfirmedRecordCount != 1 {
		t.Fatalf("college-level batch summary = %+v, want one separate confirmed batch and record", collegeRow)
	}

	departmentDeny := append(append([]permission.EffectivePermission{}, broadGrant...), permission.EffectivePermission{
		Action: permission.ActionReportView,
		Effect: permission.EffectDeny,
		Scope:  permission.Scope{DepartmentID: department},
	})
	deniedSummary, err := repository.ReportSummary(ctx, semesterID, weekID, departmentDeny)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range deniedSummary.Rows {
		if candidate.DepartmentID == department {
			t.Fatalf("department-specific deny failed to hide department cell: %+v", candidate)
		}
	}
	collegeVisible := false
	for _, candidate := range deniedSummary.Rows {
		collegeVisible = collegeVisible || (candidate.CollegeLevel && candidate.InspectionTypeID == attendance && candidate.ConfirmedRecordCount == 1)
	}
	if !collegeVisible {
		t.Fatalf("department-specific deny unexpectedly hid the independent college cell: %+v", deniedSummary.Rows)
	}
}

type reportSummaryRowSnapshot struct {
	SubmissionCount      int
	ConfirmedBatchCount  int
	ConfirmedRecordCount int
}

func assertReportPermissionPredicate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	type cell struct {
		name, academicYearID, semesterID, weekID, inspectionTypeID, departmentID string
	}
	const (
		yearOne       = "10000000-0000-0000-0000-000000000001"
		yearTwo       = "10000000-0000-0000-0000-000000000002"
		semesterOne   = "20000000-0000-0000-0000-000000000001"
		semesterTwo   = "20000000-0000-0000-0000-000000000002"
		weekOne       = "30000000-0000-0000-0000-000000000001"
		weekTwo       = "30000000-0000-0000-0000-000000000002"
		typeOne       = "40000000-0000-0000-0000-000000000001"
		typeTwo       = "40000000-0000-0000-0000-000000000002"
		departmentOne = "50000000-0000-0000-0000-000000000001"
		departmentTwo = "50000000-0000-0000-0000-000000000002"
	)
	cells := []cell{
		{name: "a-base", academicYearID: yearOne, semesterID: semesterOne, weekID: weekOne, inspectionTypeID: typeOne, departmentID: departmentOne},
		{name: "b-year", academicYearID: yearTwo, semesterID: semesterOne, weekID: weekOne, inspectionTypeID: typeOne, departmentID: departmentOne},
		{name: "c-semester", academicYearID: yearOne, semesterID: semesterTwo, weekID: weekOne, inspectionTypeID: typeOne, departmentID: departmentOne},
		{name: "d-week", academicYearID: yearOne, semesterID: semesterOne, weekID: weekTwo, inspectionTypeID: typeOne, departmentID: departmentOne},
		{name: "e-type", academicYearID: yearOne, semesterID: semesterOne, weekID: weekOne, inspectionTypeID: typeTwo, departmentID: departmentOne},
		{name: "f-department", academicYearID: yearOne, semesterID: semesterOne, weekID: weekOne, inspectionTypeID: typeOne, departmentID: departmentTwo},
		{name: "g-college", academicYearID: yearOne, semesterID: semesterOne, weekID: weekOne, inspectionTypeID: typeOne},
		{name: "h-second-grant", academicYearID: yearOne, semesterID: semesterOne, weekID: weekTwo, inspectionTypeID: typeTwo, departmentID: departmentTwo},
	}
	valueRows := make([]string, 0, len(cells))
	cellArgs := make([]any, 0, len(cells)*6)
	for _, item := range cells {
		first := len(cellArgs) + 1
		cellArgs = append(cellArgs, item.name, item.academicYearID, item.semesterID, item.weekID, item.inspectionTypeID)
		if item.departmentID == "" {
			cellArgs = append(cellArgs, nil)
		} else {
			cellArgs = append(cellArgs, item.departmentID)
		}
		departmentArgument := fmt.Sprintf("$%d::uuid", len(cellArgs))
		valueRows = append(valueRows, fmt.Sprintf("($%d::text,$%d::uuid,$%d::uuid,$%d::uuid,$%d::uuid,%s)",
			first, first+1, first+2, first+3, first+4, departmentArgument))
	}
	queryPrefix := "WITH cells(name,academic_year_id,semester_id,week_id,inspection_type_id,department_id) AS (VALUES " +
		" " + valueRows[0]
	for _, value := range valueRows[1:] {
		queryPrefix += "," + value
	}
	queryPrefix += ") SELECT c.name FROM cells c WHERE "

	allow := func(scope permission.Scope) permission.EffectivePermission {
		return permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow, Scope: scope}
	}
	deny := func(scope permission.Scope) permission.EffectivePermission {
		return permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectDeny, Scope: scope}
	}
	exactScope := permission.Scope{AcademicYearID: yearOne, SemesterID: semesterOne, WeekID: weekOne,
		InspectionTypeID: typeOne, DepartmentID: departmentOne}
	testCases := []struct {
		name        string
		permissions []permission.EffectivePermission
		want        []string
	}{
		{name: "allow intersects year semester week type and department", permissions: []permission.EffectivePermission{allow(exactScope)}, want: []string{"a-base"}},
		{name: "deny subtracts matching scope and preserves independent college row", permissions: []permission.EffectivePermission{
			allow(permission.Scope{}), deny(permission.Scope{WeekID: weekTwo}), deny(permission.Scope{InspectionTypeID: typeTwo}), deny(permission.Scope{DepartmentID: departmentTwo}),
		}, want: []string{"a-base", "b-year", "c-semester", "g-college"}},
		{name: "class and grade denies hide only matching aggregate cells", permissions: []permission.EffectivePermission{
			allow(permission.Scope{}),
			deny(permission.Scope{AcademicYearID: yearOne, SemesterID: semesterOne, WeekID: weekOne, InspectionTypeID: typeOne, DepartmentID: departmentOne, ClassID: "class-1"}),
			deny(permission.Scope{AcademicYearID: yearOne, SemesterID: semesterOne, WeekID: weekTwo, InspectionTypeID: typeTwo, DepartmentID: departmentTwo, Grade: "2026"}),
		}, want: []string{"b-year", "c-semester", "d-week", "e-type", "f-department", "g-college"}},
		{name: "class or grade allow cannot authorize an aggregate cell", permissions: []permission.EffectivePermission{
			allow(permission.Scope{ClassID: "class-1"}),
		}, want: []string{}},
	}
	columns := reportScopeColumns{
		academicYear: "c.academic_year_id", semester: "c.semester_id", week: "c.week_id",
		inspectionType: "c.inspection_type_id", department: "c.department_id",
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			args := append([]any(nil), cellArgs...)
			predicate := reportPermissionPredicate(testCase.permissions, columns, &args)
			rows, err := pool.Query(context.Background(), queryPrefix+predicate+" ORDER BY c.name", args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			got := make([]string, 0)
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					t.Fatal(err)
				}
				got = append(got, name)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("visible cells = %v, want %v", got, testCase.want)
			}
		})
	}
}
