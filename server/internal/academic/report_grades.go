package academic

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrReportGradesNotFound   = errors.New("semester report grade scope not found")
	ErrReportGradesConflict   = errors.New("semester report grade scope version conflict")
	ErrReportGradeUnavailable = errors.New("report grade unavailable")
)

// ReportGradeScope is the academic-year mapping inherited by every week in its semesters.
type ReportGradeScope struct {
	SemesterID string   `json:"semesterId"`
	GradeIDs   []string `json:"gradeIds"`
	Version    int      `json:"version"`
}

// ReportGradePreview describes the current roster that a newly generated report would snapshot.
type ReportGradePreview struct {
	Grades          []ReportGradePreviewRow `json:"grades"`
	ClassCount      int                     `json:"classCount"`
	StudentCount    int                     `json:"studentCount"`
	UnassignedCount int                     `json:"unassignedCount"`
}

type ReportGradePreviewRow struct {
	GradeID         string `json:"gradeId"`
	GradeName       string `json:"gradeName"`
	ClassCount      int    `json:"classCount"`
	StudentCount    int    `json:"studentCount"`
	UnassignedCount int    `json:"unassignedCount"`
}

type ReportGradeRepository interface {
	GetReportGrades(context.Context, string) (ReportGradeScope, error)
	PreviewReportGrades(context.Context, []string) (ReportGradePreview, error)
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

func (s *ReportGradeService) Preview(ctx context.Context, gradeIDs []string) (ReportGradePreview, error) {
	if len(gradeIDs) == 0 || len(gradeIDs) > 100 {
		return ReportGradePreview{}, ErrInvalidInput
	}
	seen := make(map[string]bool, len(gradeIDs))
	for _, id := range gradeIDs {
		if _, err := uuid.Parse(id); err != nil || seen[id] {
			return ReportGradePreview{}, ErrInvalidInput
		}
		seen[id] = true
	}
	return s.repository.PreviewReportGrades(ctx, gradeIDs)
}
