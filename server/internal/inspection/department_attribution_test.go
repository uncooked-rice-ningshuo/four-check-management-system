package inspection

import (
	"errors"
	"testing"
)

func TestResolveEntryDepartmentUsesTrustedMemberAssignment(t *testing.T) {
	const primary = "10000000-0000-0000-0000-000000000002"
	member := ManualActor{IsMember: true, PrimaryDepartmentID: primary}
	if got, err := ResolveEntryDepartment(member, ""); err != nil || got != primary {
		t.Fatalf("empty member department = %q, %v", got, err)
	}
	if got, err := ResolveEntryDepartment(member, primary); err != nil || got != primary {
		t.Fatalf("matching member department = %q, %v", got, err)
	}
	for _, requested := range []string{"10000000-0000-0000-0000-000000000003", "not-a-uuid"} {
		if _, err := ResolveEntryDepartment(member, requested); !errors.Is(err, ErrScopeMismatch) {
			t.Fatalf("member chose %q: %v", requested, err)
		}
	}
	if _, err := ResolveEntryDepartment(ManualActor{IsMember: true}, ""); !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("unassigned member entered inspection: %v", err)
	}
	if got, err := ResolveEntryDepartment(ManualActor{IsSuperAdmin: true}, ""); err != nil || got != "" {
		t.Fatalf("super admin behavior changed: %q, %v", got, err)
	}
}
