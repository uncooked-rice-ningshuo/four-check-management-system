package permission

import (
	"context"
	"testing"

	"four-check-management-system/server/internal/audit"
)

type memoryRepository struct {
	profile   AccessProfile
	mutations []audit.Mutation
}

func (m *memoryRepository) LoadAccessProfile(context.Context, string) (AccessProfile, error) {
	return m.profile, nil
}
func (m *memoryRepository) ListRules(context.Context, string) ([]Rule, error) {
	return m.profile.Rules, nil
}
func (m *memoryRepository) CreateRule(_ context.Context, rule Rule, mutation audit.Mutation) (Rule, error) {
	m.mutations = append(m.mutations, mutation)
	return rule, nil
}
func (m *memoryRepository) UpdateRule(_ context.Context, rule Rule, mutation audit.Mutation) (Rule, error) {
	m.mutations = append(m.mutations, mutation)
	return rule, nil
}
func (m *memoryRepository) DeleteRule(_ context.Context, _, _ string, mutation audit.Mutation) error {
	m.mutations = append(m.mutations, mutation)
	return nil
}

func TestSuperAdminReceivesEveryActionWithSystemSource(t *testing.T) {
	svc := NewService(&memoryRepository{profile: AccessProfile{AccountID: "admin", SystemRole: SystemRoleSuperAdmin}})
	result, err := svc.Effective(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed(ActionUserManage, Scope{}) || !result.Allowed(ActionReportFinalize, Scope{}) {
		t.Fatalf("permissions = %+v", result.Permissions)
	}
	if result.Permissions[0].Sources[0].Type != SourceSystemRole {
		t.Fatalf("source = %+v", result.Permissions[0].Sources)
	}
}

func TestDepartmentMemberReceivesMappedEnterAndViewOnly(t *testing.T) {
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MEMBER", DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, CanView: true, CanReview: true, Enabled: true}}}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "member")
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{DepartmentID: "study", InspectionTypeID: "attendance"}
	if !result.Allowed(ActionInspectionEnter, scope) || !result.Allowed(ActionInspectionView, scope) {
		t.Fatalf("mapped member permissions = %+v", result.Permissions)
	}
	if result.Allowed(ActionInspectionReview, scope) {
		t.Fatal("department member must not inherit review permission")
	}
}

func TestDepartmentMinisterReceivesMappedReview(t *testing.T) {
	profile := AccessProfile{AccountID: "minister", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MINISTER", DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanReview: true, Enabled: true}}}
	result, _ := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "minister")
	if !result.Allowed(ActionInspectionReview, Scope{DepartmentID: "study", InspectionTypeID: "attendance"}) {
		t.Fatalf("permissions = %+v", result.Permissions)
	}
}

func TestOfficeLeadershipReceivesReportWorkflowActions(t *testing.T) {
	profile := AccessProfile{AccountID: "office", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", OrganizationRoleCode: "VICE_MINISTER"}
	result, _ := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "office")
	for _, action := range []string{ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish} {
		if !result.Allowed(action, Scope{}) {
			t.Errorf("missing %s in %+v", action, result.Permissions)
		}
	}
}

func TestScopedAllowAndExplicitDenyPrecedence(t *testing.T) {
	profile := AccessProfile{AccountID: "user", SystemRole: SystemRoleMember, Rules: []Rule{
		{ID: "allow", AccountID: "user", Action: ActionInspectionView, Effect: EffectAllow, Scope: Scope{SemesterID: "semester-1", InspectionTypeID: "attendance"}, Enabled: true},
		{ID: "deny", AccountID: "user", Action: ActionInspectionView, Effect: EffectDeny, Scope: Scope{SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "attendance"}, Enabled: true},
	}}
	result, _ := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "user")
	if !result.Allowed(ActionInspectionView, Scope{SemesterID: "semester-1", WeekID: "week-2", InspectionTypeID: "attendance"}) {
		t.Fatal("scoped allow should match week 2")
	}
	if result.Allowed(ActionInspectionView, Scope{SemesterID: "semester-1", WeekID: "week-3", InspectionTypeID: "attendance"}) {
		t.Fatal("explicit deny must override matching allow")
	}
}

func TestCreateRuleRejectsUnknownAction(t *testing.T) {
	_, err := NewService(&memoryRepository{}).CreateRule(context.Background(), "admin", Rule{AccountID: "user", Action: "UNKNOWN", Effect: EffectAllow})
	if err != ErrInvalidRule {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRuleProvidesPermissionAuditMutation(t *testing.T) {
	repository := &memoryRepository{}
	created, err := NewService(repository).CreateRule(context.Background(), "admin-1", Rule{
		AccountID: "user-1", Action: ActionArchiveView, Effect: EffectAllow,
	}, audit.Metadata{RequestID: "permission-request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Metadata.ActorID != "admin-1" || mutation.Entry.Action != audit.ActionPermissionRuleChanged || mutation.Entry.ResourceID != created.ID {
		t.Fatalf("mutation = %+v", mutation)
	}
}

func TestEmptyPermissionCollectionsAreArrays(t *testing.T) {
	service := NewService(&memoryRepository{profile: AccessProfile{AccountID: "member", SystemRole: SystemRoleMember}})
	effective, err := service.Effective(context.Background(), "member")
	if err != nil {
		t.Fatal(err)
	}
	if effective.Permissions == nil {
		t.Fatal("empty effective permissions must serialize as [] instead of null")
	}
	rules, err := service.ListRules(context.Background(), "member")
	if err != nil {
		t.Fatal(err)
	}
	if rules == nil {
		t.Fatal("empty permission rules must serialize as [] instead of null")
	}
}
