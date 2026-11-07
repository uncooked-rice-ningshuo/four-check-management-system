package academic

import (
	"context"
	"errors"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

var (
	ErrReportGradesNotFound   = errors.New("semester report grade scope not found")
	ErrReportGradesConflict   = errors.New("semester report grade scope version conflict")
	ErrReportGradeUnavailable = errors.New("report grade unavailable")
)

// ReportGradeScope is the explicit set inherited by every week in a semester.
type ReportGradeScope struct {
	SemesterID string   `json:"semesterId"`
	GradeIDs   []string `json:"gradeIds"`
	Version    int      `json:"version"`
}

type ReportGradeRepository interface {
	GetReportGrades(context.Context, string) (ReportGradeScope, error)
	ReplaceReportGrades(context.Context, ReportGradeScope, audit.Metadata) (ReportGradeScope, error)
}

type ReportGradeService struct{ repository ReportGradeRepository }

func NewReportGradeService(repository ReportGradeRepository) *ReportGradeService {
	return &ReportGradeService{repository: repository}
}

func (s *ReportGradeService) Get(ctx context.Context, semesterID string) (ReportGradeScope, error) {
	if _, err := uuid.Parse(semesterID); err != nil {
		return ReportGradeScope{}, ErrInvalidInput
	}
	return s.repository.GetReportGrades(ctx, semesterID)
}

func (s *ReportGradeService) Replace(ctx context.Context, scope ReportGradeScope, metadata audit.Metadata) (ReportGradeScope, error) {
	if _, err := uuid.Parse(scope.SemesterID); err != nil {
		return ReportGradeScope{}, ErrInvalidInput
	}
	if scope.Version < 0 || len(scope.GradeIDs) == 0 {
		return ReportGradeScope{}, ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(scope.GradeIDs))
	for _, id := range scope.GradeIDs {
		if _, err := uuid.Parse(id); err != nil {
			return ReportGradeScope{}, ErrInvalidInput
		}
		if _, exists := seen[id]; exists {
			return ReportGradeScope{}, ErrInvalidInput
		}
		seen[id] = struct{}{}
	}
	return s.repository.ReplaceReportGrades(ctx, scope, metadata)
}
