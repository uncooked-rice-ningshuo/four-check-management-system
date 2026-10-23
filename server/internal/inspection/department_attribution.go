package inspection

import "strings"

// CollegeDepartmentOptionID is a transport-only selection value. Persistence
// stores college ownership as SQL NULL, never as a synthetic department row.
const CollegeDepartmentOptionID = "00000000-0000-0000-0000-000000000000"

// ResolveEntryDepartment takes a member's department from trusted account data.
// Scope and enabled-state checks still run in the existing authorization path.
func ResolveEntryDepartment(actor ManualActor, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if !actor.IsMember || actor.IsSuperAdmin {
		if requested == CollegeDepartmentOptionID {
			return "", nil
		}
		return requested, nil
	}
	primary, ok := canonicalDetailID(actor.PrimaryDepartmentID)
	if !ok {
		return "", ErrScopeMismatch
	}
	if requested != "" {
		provided, valid := canonicalDetailID(requested)
		if !valid || provided != primary {
			return "", ErrScopeMismatch
		}
	}
	return primary, nil
}
