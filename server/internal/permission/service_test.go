package permission

import (
	"context"
	"errors"
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
	m.profile.Rules = append(m.profile.Rules, rule)
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

func TestSuperAdminReceivesGeneralActionsWithSystemSource(t *testing.T) {
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

func TestOfficeLeadershipCanReceiveSeparatelyConfiguredForceGeneratePermission(t *testing.T) {
	profile := AccessProfile{AccountID: "office-deputy", SystemRole: SystemRoleMember,
		DepartmentCode: "OFFICE", OrganizationRoleCode: "VICE_MINISTER"}
	repository := &memoryRepository{profile: profile}
	service := NewService(repository)
	defaultPermissions, err := service.Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if defaultPermissions.Allowed("REPORT_FORCE_GENERATE", Scope{}) {
		t.Fatal("office leadership must not inherit force generation without a separate rule")
	}

	created, err := service.CreateRule(context.Background(), "admin", Rule{
		AccountID: profile.AccountID, Action: "REPORT_FORCE_GENERATE", Effect: EffectAllow,
		Note: "值班负责人单独授权",
	})
	if err != nil {
		t.Fatalf("create separately configured force permission: %v", err)
	}
	if created.Action != "REPORT_FORCE_GENERATE" || created.Effect != EffectAllow {
		t.Fatalf("created rule = %+v", created)
	}
	effectivePermissions, err := service.Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !effectivePermissions.Allowed(ActionReportForceGenerate, Scope{}) {
		t.Fatalf("separately configured force permission was not effective: %+v", effectivePermissions.Permissions)
	}
	if len(repository.mutations) != 1 || repository.mutations[0].Entry.Action != audit.ActionPermissionRuleChanged {
		t.Fatalf("permission audit mutations = %+v", repository.mutations)
	}
}

func TestForceGeneratePermissionRejectsAccountsOutsideOfficeLeadership(t *testing.T) {
	for _, profile := range []AccessProfile{
		{AccountID: "member", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", OrganizationRoleCode: "MEMBER"},
		{AccountID: "other-minister", SystemRole: SystemRoleMember, DepartmentCode: "STUDY", OrganizationRoleCode: "MINISTER"},
		{AccountID: "superadmin", SystemRole: SystemRoleSuperAdmin},
	} {
		_, err := NewService(&memoryRepository{profile: profile}).CreateRule(context.Background(), "admin", Rule{
			AccountID: profile.AccountID, Action: "REPORT_FORCE_GENERATE", Effect: EffectAllow,
		})
		if err == nil || errors.Is(err, ErrInvalidRule) {
			t.Fatalf("force permission for %s should fail with an eligibility error, got %v", profile.AccountID, err)
		}
	}
}

func TestUpdateRuleCannotGrantForcePermissionToIneligibleAccount(t *testing.T) {
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember,
		DepartmentCode: "OFFICE", OrganizationRoleCode: "MEMBER",
		Rules: []Rule{{ID: "existing-rule", AccountID: "member", Action: ActionReportView, Effect: EffectAllow, Enabled: true}}}
	_, err := NewService(&memoryRepository{profile: profile}).UpdateRule(context.Background(), Rule{
		ID: "existing-rule", AccountID: profile.AccountID,
		Action: "REPORT_FORCE_GENERATE", Effect: EffectAllow,
	})
	if err == nil || errors.Is(err, ErrInvalidRule) {
		t.Fatalf("ineligible target update should fail with an eligibility error, got %v", err)
	}
}

func TestForceGeneratePermissionCannotBeLimitedToDepartmentOrClass(t *testing.T) {
	profile := AccessProfile{AccountID: "office-vice", SystemRole: SystemRoleMember,
		DepartmentCode: "OFFICE", OrganizationRoleCode: "VICE_MINISTER"}
	for _, scope := range []Scope{
		{DepartmentID: "office"},
		{ClassID: "class-1"},
		{Grade: "2026"},
	} {
		_, err := NewService(&memoryRepository{profile: profile}).CreateRule(context.Background(), "admin", Rule{
			AccountID: profile.AccountID, Action: "REPORT_FORCE_GENERATE", Effect: EffectAllow, Scope: scope,
		})
		if !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("force permission with unsupported scope %+v should be invalid, got %v", scope, err)
		}
	}
}

func TestForceGeneratePermissionIsNotImplicitForSuperAdmin(t *testing.T) {
	result, err := NewService(&memoryRepository{profile: AccessProfile{
		AccountID: "superadmin", SystemRole: SystemRoleSuperAdmin,
	}}).Effective(context.Background(), "superadmin")
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed("REPORT_FORCE_GENERATE", Scope{}) {
		t.Fatal("force generation must remain separately configured even for the super admin")
	}
}

func TestForceGenerateGrantStopsWorkingWhenOfficeLeadershipChanges(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		departmentCode   string
		organizationRole string
	}{
		{name: "loses office leadership", departmentCode: "OFFICE", organizationRole: "MEMBER"},
		{name: "moves to another department", departmentCode: "STUDY", organizationRole: "MINISTER"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &memoryRepository{profile: AccessProfile{
				AccountID: "office-deputy", SystemRole: SystemRoleMember,
				DepartmentCode: "OFFICE", OrganizationRoleCode: "VICE_MINISTER",
				Rules: []Rule{{AccountID: "office-deputy", Action: ActionReportForceGenerate, Effect: EffectAllow, Enabled: true}},
			}}
			service := NewService(repository)

			beforeChange, err := service.Effective(context.Background(), repository.profile.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			if !beforeChange.Allowed(ActionReportForceGenerate, Scope{}) {
				t.Fatal("eligible office leadership should retain the separately configured grant")
			}

			repository.profile.DepartmentCode = testCase.departmentCode
			repository.profile.OrganizationRoleCode = testCase.organizationRole
			afterChange, err := service.Effective(context.Background(), repository.profile.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			if afterChange.Allowed(ActionReportForceGenerate, Scope{}) {
				t.Fatal("stored grant must stop being effective after office leadership eligibility changes")
			}
		})
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
	if result.Allowed(ActionInspectionView, Scope{SemesterID: "semester-1", InspectionTypeID: "attendance"}) {
		t.Fatal("an unbounded request must not encompass a narrower explicit deny")
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

func TestInspectionManageIsSeparateFromEntry(t *testing.T) {
	for _, role := range []string{"MEMBER", "MINISTER", "VICE_MINISTER"} {
		profile := AccessProfile{AccountID: "user", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: role, DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, Enabled: true}}}
		result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "user")
		if err != nil {
			t.Fatal(err)
		}
		if result.Allowed("INSPECTION_MANAGE", Scope{DepartmentID: "study", InspectionTypeID: "attendance"}) != (role != "MEMBER") {
			t.Fatalf("management permission for %s", role)
		}
	}
}
