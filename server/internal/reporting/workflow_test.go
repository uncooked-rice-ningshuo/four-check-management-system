package reporting

import (
	"errors"
	"testing"
)

func TestNextReportStatus(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		action        string
		want          string
		wantErr       bool
	}{
		{name: "submit draft", current: ReportStatusDraft, action: ReportActionSubmit, want: ReportStatusPending},
		{name: "resubmit returned", current: ReportStatusReturned, action: ReportActionSubmit, want: ReportStatusPending},
		{name: "return review", current: ReportStatusPending, action: ReportActionReturn, want: ReportStatusReturned},
		{name: "approve enters finalization confirmation", current: ReportStatusPending, action: ReportActionApprove, want: ReportStatusFinalizing},
		{name: "finalize structured data without output files", current: ReportStatusFinalizing, action: ReportActionFinalized, want: ReportStatusFinalized},
		{name: "publish finalized", current: ReportStatusFinalized, action: ReportActionPublished, want: ReportStatusPublished},
		{name: "archive published", current: ReportStatusPublished, action: ReportActionArchived, want: ReportStatusArchived},
		{name: "cannot edit finalized", current: ReportStatusFinalized, action: ReportActionSubmit, wantErr: true},
		{name: "cannot publish draft", current: ReportStatusDraft, action: ReportActionPublished, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextReportStatus(test.current, test.action)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidReportTransition) {
					t.Fatalf("NextReportStatus() error = %v, want invalid transition", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("NextReportStatus() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
