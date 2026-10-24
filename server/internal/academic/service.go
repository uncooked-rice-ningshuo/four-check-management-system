package academic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid academic calendar input")
	ErrDateOverlap  = errors.New("academic date range overlaps")
)

type AcademicYear struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	Enabled   bool      `json:"enabled"`
}

type Semester struct {
	ID             string    `json:"id"`
	AcademicYearID string    `json:"academicYearId"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	StartDate      time.Time `json:"startDate"`
	EndDate        time.Time `json:"endDate"`
	Enabled        bool      `json:"enabled"`
}

type Week struct {
	ID         string    `json:"id"`
	SemesterID string    `json:"semesterId"`
	WeekNo     int       `json:"weekNo"`
	StartDate  time.Time `json:"startDate"`
	EndDate    time.Time `json:"endDate"`
	Enabled    bool      `json:"enabled"`
	Note       string    `json:"note"`
	// Nil input means an ordinary week; the repository stores the effective 1x value.
	AttendancePenaltyMultiplier *int `json:"attendancePenaltyMultiplier"`
}

type Calendar struct {
	AcademicYears []AcademicYear `json:"academicYears"`
	Semesters     []Semester     `json:"semesters"`
	Weeks         []Week         `json:"weeks"`
}

type Repository interface {
	List(context.Context) (Calendar, error)
	CreateYear(context.Context, AcademicYear, audit.Mutation) (AcademicYear, error)
	UpdateYear(context.Context, AcademicYear, audit.Mutation) (AcademicYear, error)
	CreateSemester(context.Context, Semester, audit.Mutation) (Semester, error)
	UpdateSemester(context.Context, Semester, audit.Mutation) (Semester, error)
	CreateWeek(context.Context, Week, audit.Mutation) (Week, error)
	UpdateWeek(context.Context, Week, audit.Mutation) (Week, error)
	CurrentWeek(context.Context, time.Time) (*Week, error)
}

func (s *Service) UpdateYear(ctx context.Context, year AcademicYear, metadata ...audit.Metadata) (AcademicYear, error) {
	if year.ID == "" || invalidAcademicYearRange(year.StartDate, year.EndDate) {
		return AcademicYear{}, ErrInvalidInput
	}
	year.Name = academicYearName(year.StartDate)
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return AcademicYear{}, err
	}
	return s.repository.UpdateYear(ctx, year, calendarMutation(metadata, "academicYear", year.ID, findYear(calendar, year.ID), year))
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context) (Calendar, error) {
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return Calendar{}, err
	}
	if calendar.AcademicYears == nil {
		calendar.AcademicYears = []AcademicYear{}
	}
	if calendar.Semesters == nil {
		calendar.Semesters = []Semester{}
	}
	if calendar.Weeks == nil {
		calendar.Weeks = []Week{}
	}
	return calendar, nil
}

func (s *Service) CreateYear(ctx context.Context, year AcademicYear, metadata ...audit.Metadata) (AcademicYear, error) {
	if invalidAcademicYearRange(year.StartDate, year.EndDate) {
		return AcademicYear{}, ErrInvalidInput
	}
	year.Name = academicYearName(year.StartDate)
	year.ID, year.Enabled = uuid.NewString(), true
	return s.repository.CreateYear(ctx, year, calendarMutation(metadata, "academicYear", year.ID, nil, year))
}

func (s *Service) UpdateSemester(ctx context.Context, semester Semester, metadata ...audit.Metadata) (Semester, error) {
	semester.Code = strings.TrimSpace(semester.Code)
	if semester.ID == "" || semester.AcademicYearID == "" || invalidRange(semester.StartDate, semester.EndDate) {
		return Semester{}, ErrInvalidInput
	}
	if err := s.applySemesterRules(ctx, &semester); err != nil {
		return Semester{}, err
	}
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return Semester{}, err
	}
	for _, week := range calendar.Weeks {
		if week.SemesterID == semester.ID && (week.StartDate.Before(semester.StartDate) || week.EndDate.After(semester.EndDate)) {
			return Semester{}, ErrInvalidInput
		}
	}
	return s.repository.UpdateSemester(ctx, semester, calendarMutation(metadata, "semester", semester.ID, findSemester(calendar, semester.ID), semester))
}

func (s *Service) CreateSemester(ctx context.Context, semester Semester, metadata ...audit.Metadata) (Semester, error) {
	semester.Code = strings.TrimSpace(semester.Code)
	if semester.AcademicYearID == "" || invalidRange(semester.StartDate, semester.EndDate) {
		return Semester{}, ErrInvalidInput
	}
	if err := s.applySemesterRules(ctx, &semester); err != nil {
		return Semester{}, err
	}
	semester.ID, semester.Enabled = uuid.NewString(), true
	return s.repository.CreateSemester(ctx, semester, calendarMutation(metadata, "semester", semester.ID, nil, semester))
}

func (s *Service) CreateWeek(ctx context.Context, week Week, metadata ...audit.Metadata) (Week, error) {
	defaultAttendancePenaltyMultiplier(&week)
	if week.SemesterID == "" || week.WeekNo < 1 || week.WeekNo > 60 || invalidRange(week.StartDate, week.EndDate) || !validAttendancePenaltyMultiplier(week.AttendancePenaltyMultiplier) {
		return Week{}, ErrInvalidInput
	}
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return Week{}, err
	}
	if err := ValidateWeekWithinSemester(calendar, week); err != nil {
		return Week{}, err
	}
	for _, existing := range calendar.Weeks {
		if existing.Enabled && existing.ID != week.ID && rangesOverlap(existing.StartDate, existing.EndDate, week.StartDate, week.EndDate) {
			return Week{}, ErrDateOverlap
		}
	}
	week.ID, week.Enabled = uuid.NewString(), true
	return s.repository.CreateWeek(ctx, week, calendarMutation(metadata, "semesterWeek", week.ID, nil, week))
}

func (s *Service) UpdateWeek(ctx context.Context, week Week, metadata ...audit.Metadata) (Week, error) {
	defaultAttendancePenaltyMultiplier(&week)
	if week.ID == "" || week.SemesterID == "" || week.WeekNo < 1 || week.WeekNo > 60 || invalidRange(week.StartDate, week.EndDate) || !validAttendancePenaltyMultiplier(week.AttendancePenaltyMultiplier) {
		return Week{}, ErrInvalidInput
	}
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return Week{}, err
	}
	if err := ValidateWeekWithinSemester(calendar, week); err != nil {
		return Week{}, err
	}
	for _, existing := range calendar.Weeks {
		if existing.Enabled && existing.ID != week.ID && rangesOverlap(existing.StartDate, existing.EndDate, week.StartDate, week.EndDate) {
			return Week{}, ErrDateOverlap
		}
	}
	return s.repository.UpdateWeek(ctx, week, calendarMutation(metadata, "semesterWeek", week.ID, findWeek(calendar, week.ID), week))
}

func validAttendancePenaltyMultiplier(value *int) bool {
	return value != nil && (*value == 1 || *value == 2)
}

func defaultAttendancePenaltyMultiplier(week *Week) {
	if week.AttendancePenaltyMultiplier == nil {
		ordinary := 1
		week.AttendancePenaltyMultiplier = &ordinary
	}
}

func (s *Service) CurrentWeek(ctx context.Context, date time.Time) (*Week, error) {
	return s.repository.CurrentWeek(ctx, date)
}

func invalidRange(start, end time.Time) bool {
	return start.IsZero() || end.IsZero() || start.After(end)
}

func invalidAcademicYearRange(start, end time.Time) bool {
	return invalidRange(start, end) || end.Year() != start.Year()+1
}

// ValidateWeekWithinSemester enforces the same calendar boundary for manual edits and imports.
func ValidateWeekWithinSemester(calendar Calendar, week Week) error {
	for _, semester := range calendar.Semesters {
		if semester.ID == week.SemesterID {
			if week.StartDate.Before(semester.StartDate) || week.EndDate.After(semester.EndDate) {
				return ErrInvalidInput
			}
			return nil
		}
	}
	return ErrInvalidInput
}

func academicYearName(start time.Time) string {
	return fmt.Sprintf("%d-%d 学年", start.Year(), start.Year()+1)
}

func (s *Service) applySemesterRules(ctx context.Context, semester *Semester) error {
	if semester.Code != "FIRST" && semester.Code != "SECOND" {
		return ErrInvalidInput
	}
	calendar, err := s.repository.List(ctx)
	if err != nil {
		return err
	}
	for _, year := range calendar.AcademicYears {
		if year.ID != semester.AcademicYearID {
			continue
		}
		if semester.StartDate.Before(year.StartDate) || semester.EndDate.After(year.EndDate) {
			return ErrInvalidInput
		}
		suffix := "上学期"
		if semester.Code == "SECOND" {
			suffix = "下学期"
		}
		semester.Name = academicYearName(year.StartDate) + suffix
		return nil
	}
	return ErrInvalidInput
}
func rangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aEnd.Before(bStart) && !bEnd.Before(aStart)
}

func calendarMutation(metadata []audit.Metadata, resourceType, resourceID string, before, after any) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: audit.ActionAcademicCalendarChanged, ResourceType: resourceType, ResourceID: resourceID, Before: before, After: after,
	}}
}

func findYear(calendar Calendar, id string) any {
	for _, value := range calendar.AcademicYears {
		if value.ID == id {
			return value
		}
	}
	return nil
}

func findSemester(calendar Calendar, id string) any {
	for _, value := range calendar.Semesters {
		if value.ID == id {
			return value
		}
	}
	return nil
}

func findWeek(calendar Calendar, id string) any {
	for _, value := range calendar.Weeks {
		if value.ID == id {
			return value
		}
	}
	return nil
}
