package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"four-check-management-system/server/internal/academic"
	"github.com/google/uuid"
)

func TestAcademicYearStagesPostgres(t *testing.T) {
	url := os.Getenv("ACADEMIC_STAGES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires empty disposable ACADEMIC_STAGES_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	database, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var exists bool
	if err := database.Pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("requires empty database", err)
	}
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	grade := uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO grades(id,name) VALUES($1,'26')`, grade); err != nil {
		t.Fatal(err)
	}
	service := academic.NewService(database)
	year, err := service.CreateYear(ctx, academic.AcademicYear{StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2027, 8, 31, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	// Model a mapping that already existed before migration 066.
	if _, err := database.Pool.Exec(ctx, `INSERT INTO academic_year_grade_stages VALUES($1,2,$2)`, year.ID, grade); err != nil {
		t.Fatal(err)
	}
	stale := year
	year.StageGrades = map[int]string{2: grade}
	saved, err := service.UpdateYear(ctx, year)
	if err != nil || saved.Version != 2 {
		t.Fatalf("save=%+v err=%v", saved, err)
	}
	if _, err := service.UpdateYear(ctx, stale); !errors.Is(err, academic.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	saved.StageGrades = map[int]string{3: uuid.NewString()}
	if _, err := service.UpdateYear(ctx, saved); !errors.Is(err, academic.ErrInvalidInput) {
		t.Fatalf("unknown grade: %v", err)
	}
	calendar, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(calendar.AcademicYears) != 1 || calendar.AcademicYears[0].Version != 2 || calendar.AcademicYears[0].StageGrades[2] != grade {
		t.Fatalf("failed save did not roll back: %+v", calendar.AcademicYears)
	}
	var count int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1`, year.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("audit count=%d, want create and successful update only", count)
	}
	major, faculty := uuid.NewString(), uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO faculties(id,name) VALUES($1,'名单测试学院')`, faculty); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO majors(id,name,faculty_id) VALUES($1,'名单测试专业',$2)`, major, faculty); err != nil {
		t.Fatal(err)
	}
	configured := calendar.AcademicYears[0]
	secondGrade, secondMajor := uuid.NewString(), uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO grades(id,name) VALUES($1,'27')`, secondGrade); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO majors(id,name,faculty_id) VALUES($1,'名单测试第二专业',$2)`, secondMajor, faculty); err != nil {
		t.Fatal(err)
	}
	first := academic.RosterMajor{MajorID: major, ClassIDs: []string{}}
	second := academic.RosterMajor{MajorID: secondMajor, ClassIDs: []string{}}
	configured.Roster = &academic.YearRoster{MajorIDs: []string{major, secondMajor}, Grades: []academic.RosterGrade{
		{GradeID: grade, Majors: []academic.RosterMajor{second, first}},
		{GradeID: secondGrade, Majors: []academic.RosterMajor{first, second}},
	}}

	if _, err := service.UpdateYear(ctx, configured); err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.List(ctx)
	if err != nil || !reflect.DeepEqual(reloaded.AcademicYears[0].Roster, configured.Roster) {
		t.Fatalf("roster round trip: %+v %v", reloaded, err)
	}

	inventory, total, err := database.LegacyStages(ctx, 1, 20)
	if err != nil || len(inventory) != min(total, 20) {
		t.Fatalf("inventory: %d %v", total, err)
	}
	semester, err := service.CreateSemester(ctx, academic.Semester{AcademicYearID: year.ID, Code: "FIRST", StartDate: year.StartDate, EndDate: year.StartDate.AddDate(0, 4, 0)})
	if err != nil {
		t.Fatal(err)
	}
	week, err := service.CreateWeek(ctx, academic.Week{SemesterID: semester.ID, WeekNo: 1, StartDate: year.StartDate, EndDate: year.StartDate.AddDate(0, 0, 6)})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := database.ReportIndicatorInputs(ctx, semester.ID, week.ID)
	if err != nil || !inputs.OrderedRoster || len(inputs.GradeMajors) != 4 || inputs.GradeMajors[0].MajorID != secondMajor || inputs.GradeMajors[1].MajorID != major || inputs.GradeMajors[2].MajorID != major || inputs.GradeMajors[3].MajorID != secondMajor || len(inputs.Grades) != 2 || inputs.Grades[0].GradeID != grade {
		t.Fatalf("report roster: %+v %v", inputs, err)
	}

	// A failed class assignment must preserve the saved roster and its version.
	invalid := reloaded.AcademicYears[0]
	invalid.Roster.Grades[0].Majors[0].ClassIDs = []string{uuid.NewString()}
	if _, err := service.UpdateYear(ctx, invalid); !errors.Is(err, academic.ErrInvalidInput) {
		t.Fatalf("unknown class: %v", err)
	}
	after, err := service.List(ctx)
	if err != nil || after.AcademicYears[0].Version != invalid.Version || len(after.AcademicYears[0].Roster.Grades[0].Majors[0].ClassIDs) != 0 {
		t.Fatalf("failed roster replacement was not rolled back: %v", err)
	}

}
