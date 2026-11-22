package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"four-check-management-system/server/internal/audit"
	"github.com/xuri/excelize/v2"
)

// LedgerFilter restricts both the table and the exported workbook to the same students.
type LedgerFilter struct {
	Search string `json:"search"`
	Grade  string `json:"grade"`
	Major  string `json:"major"`
	Class  string `json:"class"`
}

// LedgerReader reads one consistent view of the finalized semester sources.
type LedgerReader interface {
	SemesterLedger(context.Context, string, LedgerFilter, int, int) (json.RawMessage, error)
}

func (s *Service) SemesterLedger(ctx context.Context, semester string, filter LedgerFilter, page, size int) (json.RawMessage, error) {
	reader, ok := s.repository.(LedgerReader)
	if !ok {
		return nil, errors.New("semester ledger unavailable")
	}
	return reader.SemesterLedger(ctx, semester, filter, page, size)
}

// ExportSemesterLedger renders a single query snapshot, then records the file access before delivery.
func (s *Service) ExportSemesterLedger(ctx context.Context, semester string, filter LedgerFilter, metadata audit.Metadata) ([]byte, error) {
	raw, err := s.SemesterLedger(ctx, semester, filter, 1, 0)
	if err != nil {
		return nil, err
	}
	var data struct {
		Columns []struct{ Code, Name, Unit string }
		Items   []struct {
			Number, Name, Grade, ClassName string
			Values                         map[string]*float64
		}
		Weeks       []int
		Unassigned  int
		RuleVersion *int
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer book.Close()
	cols := []interface{}{"姓名", "学号", "年级", "班级"}
	for _, c := range data.Columns {
		cols = append(cols, c.Name+"（"+c.Unit+"）")
	}
	if err = book.SetSheetRow("Sheet1", "A1", &cols); err != nil {
		return nil, err
	}
	for i, r := range data.Items {
		row := []interface{}{r.Name, r.Number, r.Grade, r.ClassName}
		for _, c := range data.Columns {
			v, exists := r.Values[c.Code]
			switch {
			case !exists:
				row = append(row, 0)
			case v == nil:
				row = append(row, "待核对")
			default:
				row = append(row, *v)
			}
		}
		if err = book.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &row); err != nil {
			return nil, err
		}
	}
	if err = book.SetSheetName("Sheet1", "学期违纪台账"); err != nil {
		return nil, err
	}
	if _, err = book.NewSheet("统计口径"); err != nil {
		return nil, err
	}
	rule := "未发布"
	if data.RuleVersion != nil {
		rule = strconv.Itoa(*data.RuleVersion)
	}
	info := [][]interface{}{{"学期标识", semester}, {"定稿周次", fmt.Sprint(data.Weeks)}, {"规则版本", rule}, {"未归属个人的明细", data.Unassigned}}
	for i, row := range info {
		if err = book.SetSheetRow("统计口径", fmt.Sprintf("A%d", i+1), &row); err != nil {
			return nil, err
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	verify, err := excelize.OpenReader(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		return nil, err
	}
	defer verify.Close()
	header, err := verify.GetCellValue("学期违纪台账", "A1")
	if err != nil || header != "姓名" {
		return nil, errors.New("invalid ledger workbook")
	}
	recorder, ok := s.repository.(interface {
		RecordLedgerExport(context.Context, string, int, audit.Metadata) error
	})
	if !ok {
		return nil, errors.New("ledger export audit unavailable")
	}
	if err = recorder.RecordLedgerExport(ctx, semester, len(data.Items), metadata); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// LedgerDetailFilter identifies the student and the requested subset of finalized records.
type LedgerDetailFilter struct {
	Semester string `json:"semesterId"`
	Student  string `json:"studentNumber"`
	Kind     string `json:"kind"`
	Group    string `json:"groupKey"`
	Code     string `json:"code"`
	Week     int    `json:"week"`
	Tier     string `json:"tier"`
	Status   string `json:"status"`
	Page     int    `json:"page"`
	Size     int    `json:"pageSize"`
}
type LedgerDetailReader interface {
	SemesterLedgerDetail(context.Context, LedgerDetailFilter) (json.RawMessage, error)
}

func (s *Service) SemesterLedgerDetail(ctx context.Context, f LedgerDetailFilter) (json.RawMessage, error) {
	r, ok := s.repository.(LedgerDetailReader)
	if !ok {
		return nil, errors.New("ledger detail unavailable")
	}
	return r.SemesterLedgerDetail(ctx, f)
}
