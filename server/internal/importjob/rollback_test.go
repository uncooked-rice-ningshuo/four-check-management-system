package importjob

import (
	"context"
	"errors"
	"strings"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
)

func TestRollbackRejectsInvalidInputBeforeTransaction(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	service := NewRollbackService(nil, nil)
	for _, input := range []RollbackInput{{BatchID: "invalid", Version: 1, Reason: "reason"}, {BatchID: id, Version: 0, Reason: "reason"}, {BatchID: id, Version: 1, Reason: " "}, {BatchID: id, Version: 1, Reason: strings.Repeat("理", 256)}} {
		if _, err := service.Rollback(context.Background(), inspection.ManualActor{ID: id, IsSuperAdmin: true}, input, audit.Metadata{}); !errors.Is(err, ErrInvalidBatch) {
			t.Errorf("invalid request reached transaction: %v", err)
		}
	}
}

func TestRollbackRequiresEntireUntouchedDraftBatch(t *testing.T) {
	batch := Batch{Status: BatchStatusConfirmed, Version: 4, ImportedRows: 2}
	records := []RollbackRecord{{ID: "one", Status: "DRAFT", Version: 1}, {ID: "two", Status: "DRAFT", Version: 1}}
	if err := ValidateRollback(batch, 4, records); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRollback(batch, 3, records); !errors.Is(err, ErrBatchVersionConflict) {
		t.Fatal(err)
	}
	if err := ValidateRollback(batch, 4, records[:1]); !errors.Is(err, ErrRollbackUnsafe) {
		t.Fatal(err)
	}
	for _, record := range []RollbackRecord{
		{Status: "PENDING_REVIEW", Version: 1}, {Status: "RETURNED", Version: 1},
		{Status: "CONFIRMED", Version: 1}, {Status: "LOCKED", Version: 1},
		{Status: "DRAFT", Version: 2}, {Status: "DRAFT", Version: 1, HasSubmission: true},
		{Status: "DRAFT", Version: 1, HasReportLock: true}, {Status: "DRAFT", Version: 1, Deleted: true},
	} {
		if err := ValidateRollback(batch, 4, []RollbackRecord{records[0], record}); !errors.Is(err, ErrRollbackUnsafe) {
			t.Errorf("accepted unsafe record %+v: %v", record, err)
		}
	}
	batch.Status = BatchStatusCancelled
	if err := ValidateRollback(batch, 4, records); !errors.Is(err, ErrInvalidBatchTransition) {
		t.Fatal(err)
	}
}
