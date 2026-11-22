package permission

import (
	"testing"
	"time"
)

func TestAssignedGrantLifetime(t *testing.T) {
	start := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	grant := AssignedGrant{DepartmentID: "study", StartsAt: start, EndsAt: &end}
	for _, tc := range []struct {
		name, department string
		at               time.Time
		want             bool
	}{
		{"before", "study", start.Add(-time.Second), false}, {"start", "study", start, true},
		{"last", "study", end.Add(-time.Nanosecond), true}, {"end", "study", end, false}, {"other", "year-one", start, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := grant.ActiveFor(tc.department, tc.at); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
func TestAssignedSnapshotIsolation(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	template := GrantTemplate{Version: 1, ValidityYears: 1, Actions: []GrantAction{{Action: ActionInspectionEnter, Ranges: []OperationRange{{GradeID: "2026"}}}}}
	grants, err := template.Assign("study", now)
	if err != nil {
		t.Fatal(err)
	}
	template.Version = 2
	template.ValidityYears = 3
	template.Actions[0].Ranges[0].GradeID = "2025"
	if grants[0].TemplateVersion != 1 || grants[0].Ranges[0].GradeID != "2026" || !grants[0].EndsAt.Equal(now.AddDate(1, 0, 0)) {
		t.Fatal("snapshot changed")
	}
}
func TestAssignedScopeConjunction(t *testing.T) {
	ranges := []OperationRange{{GradeID: "2026", MajorID: "software"}, {GradeID: "2025", MajorID: "network"}}
	if !MatchesOperationRanges(ranges, OperationTarget{GradeID: "2026", MajorID: "software"}) {
		t.Fatal("valid range rejected")
	}
	if MatchesOperationRanges(ranges, OperationTarget{GradeID: "2026", MajorID: "network"}) {
		t.Fatal("dimensions mixed across grants")
	}
}
func TestAssignedStageNeverAuthorizesClass(t *testing.T) {
	ranges := []OperationRange{{GradeStage: 1}}
	target := OperationTarget{GradeID: "2026"}
	if MatchesOperationRanges(ranges, target) {
		t.Fatal("retired stage granted access")
	}
	target.GradeID = "2025"
	if MatchesOperationRanges(ranges, target) {
		t.Fatal("second-year record admitted")
	}
}

func TestRouteDenialDoesNotBlockAnotherDepartment(t *testing.T) {
	result := EffectiveResult{SystemRole: SystemRoleMember, Permissions: []EffectivePermission{
		{Action: ActionInspectionReview, Effect: EffectAllow, SourceDepartmentID: "study"},
		{Action: ActionInspectionReview, Effect: EffectAllow, SourceDepartmentID: "year-one", Ranges: []OperationRange{{GradeStage: 1}}},
		{Action: ActionInspectionReview, Effect: EffectDeny, SourceDepartmentID: "year-one"},
	}}
	if !result.HasAnyAllowed(ActionInspectionReview) || !AllowsWholeScope(result.Permissions, ActionInspectionReview, Scope{}) {
		t.Fatal("unrelated route denial leaked")
	}
	if result.ForDepartment("year-one").HasAnyAllowed(ActionInspectionReview) {
		t.Fatal("denied route remained usable")
	}
	result.Permissions = append(result.Permissions, EffectivePermission{Action: ActionInspectionReview, Effect: EffectDeny})
	if result.HasAnyAllowed(ActionInspectionReview) || AllowsWholeScope(result.Permissions, ActionInspectionReview, Scope{}) {
		t.Fatal("global denial did not apply")
	}
}

func TestRangedGrantCannotAttestWholeDepartment(t *testing.T) {
	entries := []EffectivePermission{{Action: ActionInspectionReview, Effect: EffectAllow, Ranges: []OperationRange{{GradeStage: 1}}}}
	if AllowsWholeScope(entries, ActionInspectionReview, Scope{}) {
		t.Fatal("limited permission attested entire department")
	}
}

func TestPartialRangeDenyKeepsActionAvailable(t *testing.T) {
	result := EffectiveResult{Permissions: []EffectivePermission{
		{Action: ActionInspectionEnter, Effect: EffectAllow, SourceDepartmentID: "study"},
		{Action: ActionInspectionEnter, Effect: EffectDeny, SourceDepartmentID: "study", Ranges: []OperationRange{{ClassID: "blocked"}}},
	}}
	if !result.Allowed(ActionInspectionEnter, Scope{}) {
		t.Fatal("partial denial hid entire action")
	}
	if result.Allowed(ActionInspectionEnter, Scope{ClassID: "blocked"}) {
		t.Fatal("denied class accepted")
	}
	if !result.Allowed(ActionInspectionEnter, Scope{ClassID: "other"}) {
		t.Fatal("unrelated class denied")
	}
}

func TestDeniedRangesCoverAllGrantedClauses(t *testing.T) {
	if !rangesCover([]OperationRange{{ClassID: "a"}}, []OperationRange{{ClassID: "a"}}) {
		t.Fatal("same complete range must be disabled")
	}
	if rangesCover([]OperationRange{{ClassID: "a"}}, []OperationRange{{ClassID: "a"}, {ClassID: "b"}}) {
		t.Fatal("partially denied union must remain partial")
	}
	if rangesCover([]OperationRange{{ClassID: "a"}}, nil) {
		t.Fatal("single class must not disable unrestricted grant")
	}
	if !rangesCover(nil, []OperationRange{{GradeStage: 1}}) {
		t.Fatal("global denial must cover stage grant")
	}
}

func TestNewGrantRejectsLegacyStage(t *testing.T) {
	ranges := []OperationRange{{GradeStage: 1}}
	if ValidOperationRanges(ranges) {
		t.Fatal("legacy stage accepted for a new rule")
	}
	template := GrantTemplate{Version: 1, Actions: []GrantAction{{Action: ActionInspectionEnter, Ranges: ranges}}}
	if _, err := template.Assign("department", time.Now()); err == nil {
		t.Fatal("legacy template assigned")
	}
}

func TestLegacyRangeReplacementCannotBecomeUnrestricted(t *testing.T) {
	previous := []OperationRange{{GradeStage: 2}}
	rule := Rule{Enabled: true, Mode: "OVERRIDE"}
	if !widensLegacyRange(previous, rule) {
		t.Fatal("removing stage widened scope")
	}
	rule.Ranges = []OperationRange{{GradeID: "00000000-0000-0000-0000-000000000023"}}
	if widensLegacyRange(previous, rule) {
		t.Fatal("explicit replacement rejected")
	}
	rule.Ranges = nil
	rule.Mode = "DISABLE"
	if widensLegacyRange(previous, rule) {
		t.Fatal("termination rejected")
	}
}

func TestFixedAlternativesPreserveSnapshotAndBoundaries(t *testing.T) {
	original := []OperationRange{{GradeStage: 1}, {GradeID: "2026", MajorID: "cs"}}
	entry := EffectivePermission{Ranges: original}
	if !retainFixedRanges(&entry) || len(entry.Ranges) != 1 || original[0].GradeStage != 1 {
		t.Fatal("fixed alternative or historical snapshot lost")
	}
	for _, tc := range []struct {
		grade, major, class string
		allowed             bool
	}{
		{"2026", "cs", "A", true}, {"2026", "cs", "B", true}, {"2025", "cs", "A", false}, {"2026", "other", "B", false},
	} {
		if MatchesOperationRanges(entry.Ranges, OperationTarget{GradeID: tc.grade, MajorID: tc.major, ClassID: tc.class}) != tc.allowed {
			t.Fatalf("unexpected authorization for %+v", tc)
		}
	}
	entry.Ranges = original[:1]
	if retainFixedRanges(&entry) {
		t.Fatal("retired-only grant became unrestricted")
	}
}
