package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"four-check-management-system/server/internal/audit"
	"strings"
	"testing"
)

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
