package inspection

import (
	"testing"

	"four-check-management-system/server/internal/permission"
)

func TestMaintenanceChecksActionAndRangeOfOriginalDepartment(t *testing.T) {
	access := MutationAccess{ActorID: "me", Permissions: []permission.EffectivePermission{
		{SourceDepartmentID: "a", Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow},
		{SourceDepartmentID: "b", Action: permission.ActionInspectionManage, Effect: permission.EffectAllow, Ranges: []permission.OperationRange{{GradeID: "26"}}},
	}}
	for _, tc := range []struct {
		department, grade, owner string
		want                     bool
	}{
		{"a", "24", "me", true}, {"a", "26", "other", false}, {"a", "24", "other", false},
		{"b", "26", "other", true}, {"b", "24", "other", false}, {"b", "24", "me", false},
	} {
		scope := permission.Scope{DepartmentID: tc.department, GradeID: tc.grade, ClassID: "class"}
		if got := AllowsInspectionScope(access, scope, tc.owner); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}
