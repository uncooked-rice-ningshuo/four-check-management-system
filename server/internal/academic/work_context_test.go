package academic

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"testing"
	"time"
)

func TestWorkContextPriority(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Second)
	for _, tc := range []struct {
		s          WorkContextSnapshot
		source, id string
	}{
		{WorkContextSnapshot{Automatic: &Week{ID: "auto"}}, "CALENDAR", "auto"},
		{WorkContextSnapshot{Automatic: &Week{ID: "auto"}, Override: &Week{ID: "override"}, ExpiresAt: &future}, "OVERRIDE", "override"},
		{WorkContextSnapshot{Automatic: &Week{ID: "auto"}, Override: &Week{ID: "override"}, ExpiresAt: &past}, "CALENDAR", "auto"},
		{WorkContextSnapshot{Override: &Week{ID: "override"}, ExpiresAt: &now}, "NONE", ""},
		{WorkContextSnapshot{}, "NONE", ""},
	} {
		r := ResolveWorkContext(tc.s, now)
		if r.Source != tc.source || (r.Week != nil && r.Week.ID != tc.id) {
			t.Fatal(r)
		}
	}
}
func TestWeekOverrideUnauthorized(t *testing.T) {
	s := NewWorkContextService(nil)
	if _, e := s.Set(context.Background(), false, WeekOverrideInput{}, time.Now(), audit.Metadata{}); !errors.Is(e, ErrWorkContextForbidden) {
		t.Fatal(e)
	}
}
