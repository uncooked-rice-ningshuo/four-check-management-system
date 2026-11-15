package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	year, err := service.CreateYear(ctx, academic.AcademicYear{StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2027, 8, 31, 0, 0, 0, 0, time.UTC), StageGrades: map[int]string{1: grade}})
	if err != nil {
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
}
