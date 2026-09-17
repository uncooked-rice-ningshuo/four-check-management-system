package inspection

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"testing"
)

func TestAttendanceMutationRequiresEditableDraftAndVersion(t *testing.T) {
	for _, status := range []string{RecordStatusPendingReview, RecordStatusReturned, RecordStatusConfirmed, RecordStatusLocked, "UNKNOWN"} {
		if err := validateMutationRecord(AttendanceRecord{Status: status, RowVersion: 1}, 1); !errors.Is(err, ErrRecordNotEditable) {
			t.Fatalf("status %s: %v", status, err)
		}
	}
	for _, r := range []AttendanceRecord{{Status: RecordStatusDraft, RowVersion: 1, HasSubmission: true}, {Status: RecordStatusDraft, RowVersion: 1, HasReportLock: true}} {
		if err := validateMutationRecord(r, 1); !errors.Is(err, ErrRecordNotEditable) {
			t.Fatal(err)
		}
	}
	if err := validateMutationRecord(AttendanceRecord{Status: RecordStatusDraft, RowVersion: 2}, 1); !errors.Is(err, ErrRecordVersionConflict) {
		t.Fatal(err)
	}
	if err := validateMutationRecord(AttendanceRecord{Status: RecordStatusDraft, RowVersion: 2}, 2); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRejectsMissingVersionsReasonsAndDuplicateTargets(t *testing.T) {
	s := NewMutationService(nil, nil, nil)
	for _, input := range []DeleteAttendanceInput{
		{Reason: "reason"}, {Items: []VersionedRecord{{ID: "a", Version: 1}}},
		{Reason: "reason", Items: []VersionedRecord{{ID: "a", Version: 0}}},
		{Reason: "reason", Items: []VersionedRecord{{ID: "a", Version: 1}, {ID: "a", Version: 2}}},
	} {
		if err := s.Delete(context.Background(), ManualActor{ID: "actor"}, input, audit.Metadata{}); !errors.Is(err, ErrInvalidAttendanceRecord) {
			t.Fatalf("input accepted: %v", err)
		}
	}
}
