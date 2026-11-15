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
func TestAssignedStageMapping(t *testing.T) {
	ranges := []OperationRange{{GradeStage: 1}}
	if MatchesOperationRanges(ranges, OperationTarget{GradeID: "2026"}) {
		t.Fatal("missing mapping widened access")
	}
	target := OperationTarget{GradeID: "2026", StageGrades: map[int]string{1: "2026"}}
	if !MatchesOperationRanges(ranges, target) {
		t.Fatal("valid mapping rejected")
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

func TestMissingStageMappingDiagnostic(t *testing.T) {
	entries := []EffectivePermission{{Action: ActionInspectionEnter, Effect: EffectAllow, Ranges: []OperationRange{{GradeStage: 1}}}}
	if !MissingStageMapping(entries, Scope{}) {
		t.Fatal("missing configuration not identified")
	}
	if MissingStageMapping(entries, Scope{StageGrades: map[int]string{1: "grade"}}) {
		t.Fatal("configured stage incorrectly reported missing")
	}
	entries[0].Effect = EffectDeny
	if MissingStageMapping(entries, Scope{}) {
		t.Fatal("deny rule should not require a mapping")
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
