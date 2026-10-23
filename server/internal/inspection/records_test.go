package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/audit"
	"strings"
	"testing"
)

func TestReturnedInspectionCanBeCorrectedButNotDeleted(t *testing.T) {
	returned := InspectionRecord{Status: RecordStatusReturned, HasSubmission: true, RowVersion: 3}
	if err := validateInspectionUpdate(returned, 3); err != nil {
		t.Fatal(err)
	}
	if err := validateInspectionUpdate(returned, 2); !errors.Is(err, ErrRecordVersionConflict) {
		t.Fatalf("stale returned record: %v", err)
	}
	if err := validateInspectionMutation(returned, 3); !errors.Is(err, ErrRecordNotEditable) {
		t.Fatalf("returned record deletion accepted: %v", err)
	}
	for _, record := range []InspectionRecord{
		{Status: RecordStatusPendingReview, HasSubmission: true, RowVersion: 3},
		{Status: RecordStatusConfirmed, HasSubmission: true, RowVersion: 3},
		{Status: RecordStatusReturned, HasSubmission: false, RowVersion: 3},
		{Status: RecordStatusReturned, HasSubmission: true, HasReportLock: true, RowVersion: 3},
	} {
		if err := validateInspectionUpdate(record, 3); !errors.Is(err, ErrRecordNotEditable) {
			t.Fatalf("unsafe correction accepted: %+v %v", record, err)
		}
	}
}

func TestInspectionInvalidMutationsStopBeforeRepository(t *testing.T) {
	s := NewRecordService(nil, nil)
	ctx := context.Background()
	actor := ManualActor{ID: detailID, IsSuperAdmin: true}
	for _, i := range []DeleteAttendanceInput{
		{}, {Items: []VersionedRecord{{ID: detailID, Version: 0}}, Reason: "核对"},
		{Items: []VersionedRecord{{ID: detailID, Version: 1}, {ID: strings.ToUpper(detailID), Version: 1}}, Reason: "核对"},
		{Items: []VersionedRecord{{ID: detailID, Version: 1}}, Reason: " "},
	} {
		if e := s.Delete(ctx, actor, i, audit.Metadata{}); !errors.Is(e, ErrInspectionRequest) {
			t.Fatal(e)
		}
	}
	if _, e := s.Update(ctx, actor, detailID, InspectionEdit{Version: 0, Reason: "核对"}, audit.Metadata{}); !errors.Is(e, ErrInspectionRequest) {
		t.Fatal(e)
	}
	if _, e := s.List(ctx, actor, InspectionFilter{Page: -1}); !errors.Is(e, ErrInspectionRequest) {
		t.Fatal(e)
	}
	if _, e := s.List(ctx, actor, InspectionFilter{Sort: "id; drop table"}); !errors.Is(e, ErrInspectionRequest) {
		t.Fatal(e)
	}
	if _, e := s.Detail(ctx, ManualActor{}, detailID); !errors.Is(e, ErrScopeMismatch) {
		t.Fatal(e)
	}
}
func TestInspectionAuditPrivacy(t *testing.T) {
	r := InspectionRecord{Details: &DormitoryHygieneDetails{HygieneDetails: HygieneDetails{Description: "private name", Remark: "private number", Round: "private round", Result: HygieneQualified}}}
	raw, e := json.Marshal(InspectionAuditSnapshot(r, false))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "private") || !strings.Contains(string(raw), HygieneQualified) {
		t.Fatal("audit privacy")
	}
}
