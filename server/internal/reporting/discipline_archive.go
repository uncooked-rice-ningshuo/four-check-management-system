package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"github.com/xuri/excelize/v2"
)

// ArchiveFilter selects historical decisions independently of the pending review queue.
type ArchiveFilter struct {
	CaseID     string `json:"caseId"`
	Student    string `json:"student"`
	Status     string `json:"status"`
	SemesterID string `json:"semesterId"`
	Group      string `json:"group"`
	Tier       string `json:"tier"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
}

func (s *Service) DisciplineArchive(ctx context.Context, actor string, f ArchiveFilter) (json.RawMessage, error) {
	if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 || (f.Status != "" && f.Status != "APPROVED" && f.Status != "VOID") {
		return nil, ErrInvalidReportRequest
	}
	effective, err := s.permissions.Effective(ctx, actor)
	if err != nil {
		return nil, err
	}
	if effective.SystemRole != permission.SystemRoleSuperAdmin && !effective.Allowed("DISCIPLINE_ARCHIVE_VIEW", permission.Scope{}) {
		return nil, ErrForbidden
	}
	repo, ok := s.repository.(interface {
		DisciplineArchive(context.Context, ArchiveFilter) (json.RawMessage, error)
	})
	if !ok {
		return nil, errors.New("discipline archive repository unavailable")
	}
	return repo.DisciplineArchive(ctx, f)
}

// ExportDisciplineArchive writes the entire filtered result from a single query snapshot.
func (s *Service) ExportDisciplineArchive(ctx context.Context, actor string, f ArchiveFilter, meta audit.Metadata) ([]byte, error) {
	effective, err := s.permissions.Effective(ctx, actor)
	if err != nil {
		return nil, err
	}
	if effective.SystemRole != permission.SystemRoleSuperAdmin && (!effective.Allowed("DISCIPLINE_ARCHIVE_VIEW", permission.Scope{}) || !effective.Allowed("DISCIPLINE_ARCHIVE_EXPORT", permission.Scope{})) {
		return nil, ErrForbidden
	}
	if f.Status != "" && f.Status != "APPROVED" && f.Status != "VOID" {
		return nil, ErrInvalidReportRequest
	}
	repo, ok := s.repository.(interface {
		DisciplineArchive(context.Context, ArchiveFilter) (json.RawMessage, error)
		RecordDisciplineArchiveExport(context.Context, int, audit.Metadata) error
	})
	if !ok {
		return nil, errors.New("archive export unavailable")
	}
	f.Page = 1
	f.PageSize = 0
	raw, err := repo.DisciplineArchive(ctx, f)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			StudentNumber, Semester, Tier, Status, DecisionSource, DecidedAt string
			Amount                                                           float64
		}
	}
	if err = json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	headers := []string{"学号", "学期", "处分档位", "累计值", "状态", "确定方式", "确定时间"}
	if err = book.SetSheetRow("Sheet1", "A1", &headers); err != nil {
		return nil, err
	}
	sources := map[string]string{"TIMEOUT": "到期自动确认", "RULE": "规则直接确定", "SUPER_ADMIN": "超管确认", "TEACHER": "人工确认", "AUTHORIZED_USER": "人工确认"}
	for i, item := range page.Items {
		status := "已确定"
		if item.Status == "VOID" {
			status = "已作废"
		}
		row := []any{item.StudentNumber, item.Semester, item.Tier, item.Amount, status, sources[item.DecisionSource], item.DecidedAt}
		if err = book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &row); err != nil {
			return nil, err
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	check, err := excelize.OpenReader(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		return nil, err
	}
	defer check.Close()
	meta.ActorID = actor
	if err = repo.RecordDisciplineArchiveExport(ctx, len(page.Items), meta); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
