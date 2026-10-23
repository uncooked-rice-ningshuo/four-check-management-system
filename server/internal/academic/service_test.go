package academic

import (
	"context"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
)

type memoryRepository struct {
	calendar  Calendar
	mutations []audit.Mutation
}

func (m *memoryRepository) List(context.Context) (Calendar, error) { return m.calendar, nil }
func (m *memoryRepository) CreateYear(_ context.Context, year AcademicYear, mutation audit.Mutation) (AcademicYear, error) {
	m.mutations = append(m.mutations, mutation)
	return year, nil
}
func (m *memoryRepository) UpdateYear(_ context.Context, year AcademicYear, mutation audit.Mutation) (AcademicYear, error) {
	m.mutations = append(m.mutations, mutation)
	return year, nil
}
func (m *memoryRepository) CreateSemester(_ context.Context, semester Semester, mutation audit.Mutation) (Semester, error) {
	m.mutations = append(m.mutations, mutation)
	return semester, nil
}
func (m *memoryRepository) UpdateSemester(_ context.Context, semester Semester, mutation audit.Mutation) (Semester, error) {
	m.mutations = append(m.mutations, mutation)
	return semester, nil
}
func (m *memoryRepository) CreateWeek(_ context.Context, week Week, mutation audit.Mutation) (Week, error) {
	m.calendar.Weeks = append(m.calendar.Weeks, week)
	m.mutations = append(m.mutations, mutation)
	return week, nil
}
func (m *memoryRepository) UpdateWeek(_ context.Context, week Week, mutation audit.Mutation) (Week, error) {
	m.mutations = append(m.mutations, mutation)
	return week, nil
}
func (m *memoryRepository) CurrentWeek(_ context.Context, date time.Time) (*Week, error) {
	for _, week := range m.calendar.Weeks {
		if !date.Before(week.StartDate) && !date.After(week.EndDate) {
			copy := week
			return &copy, nil
		}
	}
	return nil, nil
}

func TestCreateWeekRejectsInvalidRange(t *testing.T) {
	svc := NewService(&memoryRepository{})
	_, err := svc.CreateWeek(context.Background(), Week{SemesterID: "semester", WeekNo: 1, StartDate: date("2026-09-07"), EndDate: date("2026-09-06")})
	if err != ErrInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func TestWeekPenaltyMultiplierDefaultsToOrdinaryWeek(t *testing.T) {
	svc := NewService(&memoryRepository{})
	input := Week{SemesterID: "semester", WeekNo: 1, StartDate: date("2026-09-07"), EndDate: date("2026-09-13")}
	defaulted, err := svc.CreateWeek(context.Background(), input)
	if err != nil || defaulted.AttendancePenaltyMultiplier == nil || *defaulted.AttendancePenaltyMultiplier != 1 {
		t.Fatalf("ordinary week did not default to 1x: %+v, %v", defaulted, err)
	}
	invalid := 3
	input.AttendancePenaltyMultiplier = &invalid
	if _, err := svc.CreateWeek(context.Background(), input); err != ErrInvalidInput {
		t.Fatalf("invalid multiplier accepted: %v", err)
	}
}

func TestCurrentWeekCanBeEmpty(t *testing.T) {
	week, err := NewService(&memoryRepository{}).CurrentWeek(context.Background(), date("2026-08-23"))
	if err != nil || week != nil {
		t.Fatalf("week=%+v error=%v", week, err)
	}
}

func TestListNormalizesEmptyCollections(t *testing.T) {
	calendar, err := NewService(&memoryRepository{}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calendar.AcademicYears == nil || calendar.Semesters == nil || calendar.Weeks == nil {
		t.Fatalf("calendar contains nil collections: %+v", calendar)
	}
}

func TestCreateYearDerivesCanonicalAcademicYearName(t *testing.T) {
	year, err := NewService(&memoryRepository{}).CreateYear(context.Background(), AcademicYear{
		Name: "任意名称", StartDate: date("2025-09-01"), EndDate: date("2026-08-31"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if year.Name != "2025-2026 学年" {
		t.Fatalf("name = %q", year.Name)
	}
}

func TestCreateYearProvidesCalendarAuditMutation(t *testing.T) {
	repository := &memoryRepository{}
	created, err := NewService(repository).CreateYear(context.Background(), AcademicYear{
		StartDate: date("2025-09-01"), EndDate: date("2026-08-31"),
	}, audit.Metadata{ActorID: "admin-1", RequestID: "calendar-request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Entry.Action != audit.ActionAcademicCalendarChanged || mutation.Entry.ResourceID != created.ID || mutation.Metadata.ActorID != "admin-1" {
		t.Fatalf("mutation = %+v", mutation)
	}
}

func TestCreateSemesterDerivesUpperAndLowerSemesterName(t *testing.T) {
	repo := &memoryRepository{calendar: Calendar{AcademicYears: []AcademicYear{{ID: "year-1", Name: "2025-2026 学年", StartDate: date("2025-09-01"), EndDate: date("2026-08-31")}}}}
	semester, err := NewService(repo).CreateSemester(context.Background(), Semester{
		AcademicYearID: "year-1", Code: "FIRST", Name: "任意名称", StartDate: date("2025-09-01"), EndDate: date("2026-01-31"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if semester.Name != "2025-2026 学年上学期" {
		t.Fatalf("name = %q", semester.Name)
	}
}

func TestCreateSemesterRejectsUnknownSemesterType(t *testing.T) {
	repo := &memoryRepository{calendar: Calendar{AcademicYears: []AcademicYear{{ID: "year-1", Name: "2025-2026 学年", StartDate: date("2025-09-01"), EndDate: date("2026-08-31")}}}}
	_, err := NewService(repo).CreateSemester(context.Background(), Semester{
		AcademicYearID: "year-1", Code: "SUMMER", Name: "暑期", StartDate: date("2026-07-01"), EndDate: date("2026-08-01"),
	})
	if err != ErrInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func date(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}
