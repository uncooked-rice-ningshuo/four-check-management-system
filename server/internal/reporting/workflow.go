package reporting

import "errors"

const (
	ReportStatusDraft      = "DRAFT"
	ReportStatusPending    = "PENDING_REVIEW"
	ReportStatusReturned   = "RETURNED"
	ReportStatusFinalizing = "FINALIZING"
	ReportStatusFinalized  = "FINALIZED"
	ReportStatusPublished  = "PUBLISHED"
	ReportStatusArchived   = "ARCHIVED"
)

const (
	ReportActionSubmit    = "SUBMITTED"
	ReportActionApprove   = "APPROVED"
	ReportActionReturn    = "RETURNED"
	ReportActionFinalized = "FINALIZED"
	ReportActionPublished = "PUBLISHED"
	ReportActionArchived  = "ARCHIVED"
)

var ErrInvalidReportTransition = errors.New("invalid weekly report state transition")

// NextReportStatus defines the report state machine in one place so handlers
// cannot create state transitions by assigning arbitrary status strings.
func NextReportStatus(current, action string) (string, error) {
	switch action {
	case ReportActionSubmit:
		if current == ReportStatusDraft || current == ReportStatusReturned {
			return ReportStatusPending, nil
		}
	case ReportActionReturn:
		if current == ReportStatusPending {
			return ReportStatusReturned, nil
		}
	case ReportActionApprove:
		if current == ReportStatusPending {
			return ReportStatusFinalizing, nil
		}
	case ReportActionFinalized:
		if current == ReportStatusFinalizing {
			return ReportStatusFinalized, nil
		}
	case ReportActionPublished:
		if current == ReportStatusFinalized {
			return ReportStatusPublished, nil
		}
	case ReportActionArchived:
		if current == ReportStatusPublished {
			return ReportStatusArchived, nil
		}
	}
	return "", ErrInvalidReportTransition
}
