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

var inspectionMemberDefaults = &InspectionRoleDefaults{CanView: true, CanEnter: true}
var inspectionLeadershipDefaults = &InspectionRoleDefaults{CanView: true, CanEnter: true, CanManage: true, CanReview: true, CanExport: true}
var inspectionPresidiumDefaults = &InspectionRoleDefaults{CanView: true}
var officeMemberDefaults = &ReportRoleDefaults{CanView: true, CanGenerate: true, CanCorrect: true, CanArchive: true}
var officeLeadershipDefaults = &ReportRoleDefaults{CanView: true, CanGenerate: true, CanCorrect: true, CanReview: true, CanReturn: true, CanFinalize: true, CanPublish: true, CanOutputSettings: true, CanArchive: true}
var officeReportScope = &ReportScope{Enabled: true, ReportRoleDefaults: *officeLeadershipDefaults}

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
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MEMBER", InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": inspectionMemberDefaults}, DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, CanView: true, CanReview: true, Enabled: true}}}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "member")
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{DepartmentID: "study", InspectionTypeID: "attendance"}
	if !result.Allowed(ActionInspectionEnter, scope) || !result.Allowed(ActionInspectionView, scope) {
		t.Fatalf("mapped member permissions = %+v", result.Permissions)
	}
	if !result.Allowed(ActionInspectionEnter, Scope{DepartmentID: "sports", InspectionTypeID: "attendance"}) || !result.Allowed(ActionInspectionView, Scope{DepartmentID: "sports", InspectionTypeID: "attendance"}) {
		t.Fatalf("mapped inspection actions should apply across departments: %+v", result.Permissions)
	}
	for _, effective := range result.Permissions {
		if effective.Scope.DepartmentID != "" || effective.Scope.ClassID != "" || effective.Scope.Grade != "" {
			t.Fatalf("mapped permissions must not carry removed scope dimensions: %+v", effective.Scope)
		}
	}
	if result.Allowed(ActionInspectionReview, scope) {
		t.Fatal("department member must not inherit review permission")
	}
}

func TestInspectionRoleDefaultsAreIndependentByType(t *testing.T) {
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MEMBER",
		InspectionDefaults: map[string]*InspectionRoleDefaults{
			"attendance": {CanView: true, CanEnter: true},
			"hygiene":    {CanView: true, CanReview: true},
		},
		DepartmentScopes: []DepartmentScope{
			{DepartmentID: "study", InspectionTypeID: "attendance", CanView: true, CanEnter: true, Enabled: true},
			{DepartmentID: "study", InspectionTypeID: "hygiene", CanView: true, CanEnter: true, CanReview: true, Enabled: true},
		},
	}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed(ActionInspectionEnter, Scope{InspectionTypeID: "attendance"}) || result.Allowed(ActionInspectionEnter, Scope{InspectionTypeID: "hygiene"}) || !result.Allowed(ActionInspectionView, Scope{InspectionTypeID: "hygiene"}) || !result.Allowed(ActionInspectionReview, Scope{InspectionTypeID: "hygiene"}) {
		t.Fatalf("per-type role defaults not enforced: %+v", result.Permissions)
	}
}

func TestOfficeConfiguredInspectionWorkOverridesReadOnlyBaseline(t *testing.T) {
	profile := AccessProfile{AccountID: "office", SystemRole: SystemRoleMember, DepartmentID: "office", DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MEMBER",
		InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": {CanView: true, CanManage: true, CanReview: true}},
		DepartmentScopes:   []DepartmentScope{{DepartmentID: "office", InspectionTypeID: "attendance", CanView: true, CanEnter: true, CanManage: true, CanReview: true, Enabled: true}},
	}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil || result.ConfirmedInspectionsOnly || !result.Allowed(ActionInspectionManage, Scope{InspectionTypeID: "attendance"}) || !result.Allowed(ActionInspectionReview, Scope{InspectionTypeID: "attendance"}) {
		t.Fatalf("configured office work did not take effect: result=%+v err=%v", result, err)
	}
}

func TestDepartmentMinisterReceivesMappedReview(t *testing.T) {
	profile := AccessProfile{AccountID: "minister", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MINISTER", InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": inspectionLeadershipDefaults}, DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanReview: true, Enabled: true}}}
	result, _ := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "minister")
	if !result.Allowed(ActionInspectionReview, Scope{DepartmentID: "study", InspectionTypeID: "attendance"}) {
		t.Fatalf("permissions = %+v", result.Permissions)
	}
}

func TestPresidiumGetsOnlyPrimaryDepartmentInspectionView(t *testing.T) {
	profile := AccessProfile{AccountID: "presidium", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "PRESIDIUM", InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": inspectionPresidiumDefaults}, DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, CanView: true, CanReview: true, CanExport: true, Enabled: true}}}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "presidium")
	if err != nil || !result.Allowed(ActionInspectionView, Scope{InspectionTypeID: "attendance"}) || result.PrimaryDepartmentID != "study" {
		t.Fatalf("presidium view or department missing: result=%+v err=%v", result, err)
	}
	for _, action := range []string{ActionInspectionEnter, ActionInspectionManage, ActionInspectionReview, ActionInspectionExport} {
		if result.Allowed(action, Scope{InspectionTypeID: "attendance"}) {
			t.Errorf("presidium inherited %s", action)
		}
	}
}

func TestOfficeLeadershipReceivesReportWorkflowActions(t *testing.T) {
	profile := AccessProfile{AccountID: "office", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "VICE_MINISTER", ReportDefaults: officeLeadershipDefaults}
	result, _ := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "office")
	for _, action := range []string{ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish} {
		if !result.Allowed(action, Scope{}) {
			t.Errorf("missing %s in %+v", action, result.Permissions)
		}
	}
	if !result.CanReviseFinalizedReport {
		t.Fatal("office deputy cannot initiate finalized correction")
	}
}

func TestOfficeMemberReceivesDraftAndArchiveActionsOnly(t *testing.T) {
	profile := AccessProfile{AccountID: "office-member", SystemRole: SystemRoleMember, DepartmentID: "office", DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MEMBER", ReportDefaults: officeMemberDefaults}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{ActionReportView, ActionReportGenerate, ActionReportCorrect, ActionArchiveView} {
		if !result.Allowed(action, Scope{}) {
			t.Errorf("missing %s", action)
		}
	}
	for _, action := range []string{ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionReportOutputSettingsManage, ActionReportForceGenerate, ActionReportObligationManage} {
		if result.Allowed(action, Scope{}) {
			t.Errorf("unexpected %s", action)
		}
	}
	if result.CanReviseFinalizedReport {
		t.Fatal("office member can initiate finalized correction")
	}
}

func TestReportDefaultsRequireMatchingDepartmentAction(t *testing.T) {
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentID: "dormitory", DepartmentCode: "DORMITORY", OrganizationRoleCode: "MEMBER",
		ReportDefaults: officeMemberDefaults, ReportScope: &ReportScope{Enabled: true, ReportRoleDefaults: ReportRoleDefaults{CanView: true, CanGenerate: true}}}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed(ActionReportView, Scope{}) || !result.Allowed(ActionReportGenerate, Scope{}) || result.Allowed(ActionReportCorrect, Scope{}) || result.Allowed(ActionArchiveView, Scope{}) {
		t.Fatalf("department action intersection failed: %+v", result.Permissions)
	}
	for _, grant := range result.Permissions {
		if grant.Action == ActionReportGenerate && (len(grant.Sources) != 2 || grant.Sources[0].Type != SourceDepartmentMap || grant.Sources[1].Type != SourceOrganizationRole) {
			t.Fatalf("report grant must explain both configuration layers: %+v", grant.Sources)
		}
	}
	profile.ReportScope.Enabled = false
	result, err = NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed(ActionReportView, Scope{}) || result.Allowed(ActionReportGenerate, Scope{}) {
		t.Fatalf("disabled report module granted action: %+v", result.Permissions)
	}
}

func TestInspectionManageRequiresMatchingDepartmentAction(t *testing.T) {
	profile := AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: "MEMBER",
		InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": {CanManage: true}},
		DepartmentScopes:   []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, Enabled: true}}}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed(ActionInspectionManage, Scope{InspectionTypeID: "attendance"}) {
		t.Fatal("manage granted without department action")
	}
	profile.DepartmentScopes[0].CanManage = true
	result, err = NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed(ActionInspectionManage, Scope{InspectionTypeID: "attendance"}) {
		t.Fatal("manage denied after both actions enabled")
	}
}

func TestOfficeMemberCannotGainLeadershipActionThroughPersonalRule(t *testing.T) {
	profile := AccessProfile{AccountID: "office-member", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MEMBER", ReportDefaults: officeMemberDefaults,
		Rules: []Rule{{Action: ActionReportReview, Effect: EffectAllow, Enabled: true}}}
	service := NewService(&memoryRepository{profile: profile})
	result, err := service.Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed(ActionReportReview, Scope{}) {
		t.Fatal("legacy personal rule bypassed office leadership requirement")
	}
	if _, err := service.CreateRule(context.Background(), "admin", Rule{AccountID: profile.AccountID, Action: ActionReportFinalize, Effect: EffectAllow}); !errors.Is(err, ErrReportMaintenanceRequired) {
		t.Fatalf("new personal rule error = %v", err)
	}
}

func TestOfficeAccountWithoutOrganizationRoleHasNoReportDefaults(t *testing.T) {
	profile := AccessProfile{AccountID: "office-unassigned", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE"}
	result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed(ActionReportGenerate, Scope{}) || result.Allowed(ActionReportReview, Scope{}) {
		t.Fatalf("unassigned office account inherited report defaults: %+v", result.Permissions)
	}
}

func TestInspectionExportIsNotInheritedByMemberOrOffice(t *testing.T) {
	for _, test := range []struct {
		name, department, role string
		want                   bool
	}{
		{"business member", "STUDY", "MEMBER", false},
		{"business minister", "STUDY", "MINISTER", true},
		{"business deputy", "STUDY", "VICE_MINISTER", true},
		{"office minister", "OFFICE", "MINISTER", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := AccessProfile{AccountID: "actor", SystemRole: SystemRoleMember, DepartmentID: "department", DepartmentCode: test.department, OrganizationRoleCode: test.role, InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": inspectionLeadershipDefaults},
				DepartmentScopes: []DepartmentScope{{DepartmentID: "department", InspectionTypeID: "attendance", CanExport: true, Enabled: true}}}
			result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Allowed(ActionInspectionExport, Scope{InspectionTypeID: "attendance"}) != test.want {
				t.Error("export permission mismatch")
			}
		})
	}
}

func TestReportOutputSettingsPermissionIsGlobalAndIndependent(t *testing.T) {
	for _, test := range []struct {
		name, role, department, organizationRole string
		want                                     bool
	}{
		{"office minister", SystemRoleMember, "OFFICE", "MINISTER", true},
		{"office deputy", SystemRoleMember, "OFFICE", "VICE_MINISTER", true},
		{"office member", SystemRoleMember, "OFFICE", "MEMBER", false},
		{"other minister with report defaults", SystemRoleMember, "STUDY", "MINISTER", true},
		{"system admin", SystemRoleAdmin, "", "", false},
		{"super admin", SystemRoleSuperAdmin, "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewService(&memoryRepository{profile: AccessProfile{AccountID: "user", SystemRole: test.role, DepartmentCode: test.department, OrganizationRoleCode: test.organizationRole, ReportDefaults: officeLeadershipDefaults, ReportScope: officeReportScope}}).Effective(context.Background(), "user")
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Allowed(ActionReportOutputSettingsManage, Scope{}); got != test.want {
				t.Fatalf("allowed = %v, want %v", got, test.want)
			}
		})
	}
	repository := &memoryRepository{profile: AccessProfile{AccountID: "member", SystemRole: SystemRoleMember}}
	service := NewService(repository)
	if _, err := service.CreateRule(context.Background(), "admin", Rule{AccountID: "member", Action: ActionReportOutputSettingsManage, Effect: EffectAllow}); !errors.Is(err, ErrReportMaintenanceRequired) {
		t.Fatalf("non-office grant error = %v", err)
	}
	result, err := service.Effective(context.Background(), "member")
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed(ActionReportOutputSettingsManage, Scope{}) || result.Allowed(ActionReportView, Scope{}) {
		t.Fatalf("unexpected permission: %+v", result.Permissions)
	}
	repository.profile = AccessProfile{AccountID: "member", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MINISTER"}
	for _, scope := range []Scope{{AcademicYearID: "year"}, {SemesterID: "semester"}, {WeekID: "week"}, {InspectionTypeID: "type"}} {
		_, err := service.CreateRule(context.Background(), "admin", Rule{AccountID: "member", Action: ActionReportOutputSettingsManage, Effect: EffectAllow, Scope: scope})
		if !errors.Is(err, ErrInvalidRule) {
			t.Fatalf("scope %+v: %v", scope, err)
		}
	}
}

func TestOnlySportsLeadershipReceivesMorningScheduleManagement(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		departmentCode   string
		organizationRole string
		want             bool
	}{
		{name: "sports minister", departmentCode: "SPORTS", organizationRole: "MINISTER", want: true},
		{name: "sports vice minister", departmentCode: "SPORTS", organizationRole: "VICE_MINISTER", want: true},
		{name: "other department minister", departmentCode: "STUDY", organizationRole: "MINISTER"},
		{name: "sports member", departmentCode: "SPORTS", organizationRole: "MEMBER"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			profile := AccessProfile{AccountID: "user", SystemRole: SystemRoleMember, DepartmentCode: testCase.departmentCode, OrganizationRoleCode: testCase.organizationRole}
			result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), profile.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Allowed(ActionMorningScheduleManage, Scope{}); got != testCase.want {
				t.Fatalf("morning schedule permission = %t, want %t; permissions=%+v", got, testCase.want, result.Permissions)
			}
		})
	}
}

func TestOfficeLeadershipCanReceiveSeparatelyConfiguredForceGeneratePermission(t *testing.T) {
	profile := AccessProfile{AccountID: "office-deputy", SystemRole: SystemRoleMember,
		DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "VICE_MINISTER"}
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
		{AccountID: "member", SystemRole: SystemRoleMember, DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MEMBER"},
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
		DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "MEMBER",
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
		DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "VICE_MINISTER"}
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
				DepartmentCode: "OFFICE", ReportScope: officeReportScope, OrganizationRoleCode: "VICE_MINISTER",
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
		profile := AccessProfile{AccountID: "user", SystemRole: SystemRoleMember, DepartmentID: "study", OrganizationRoleCode: role, InspectionDefaults: map[string]*InspectionRoleDefaults{"attendance": inspectionLeadershipDefaults}, DepartmentScopes: []DepartmentScope{{DepartmentID: "study", InspectionTypeID: "attendance", CanEnter: true, CanManage: true, Enabled: true}}}
		result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "user")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Allowed("INSPECTION_MANAGE", Scope{DepartmentID: "study", InspectionTypeID: "attendance"}) {
			t.Fatalf("management permission for %s", role)
		}
	}
}

func TestHasAnyAllowedForScopedReportDefaults(t *testing.T) {
	result := EffectiveResult{Permissions: []EffectivePermission{
		{Action: ActionReportGenerate, Effect: EffectAllow, Scope: Scope{WeekID: "week-a"}},
		{Action: ActionReportGenerate, Effect: EffectDeny, Scope: Scope{WeekID: "week-b"}},
	}}
	if result.Allowed(ActionReportGenerate, Scope{}) || !result.HasAnyAllowed(ActionReportGenerate) {
		t.Fatal("scoped generator must be able to read defaults without gaining global generation")
	}
	result.Permissions = append(result.Permissions, EffectivePermission{Action: ActionReportGenerate, Effect: EffectDeny, Scope: Scope{}})
	if result.HasAnyAllowed(ActionReportGenerate) {
		t.Fatal("global denial must block reading generation defaults")
	}
}

func TestOfficeInspectionRulesCanGrantWorkWhenExplicitlyConfigured(t *testing.T) {
	for _, role := range []string{"MEMBER", "MINISTER", "VICE_MINISTER", "PRESIDIUM"} {
		t.Run(role, func(t *testing.T) {
			profile := AccessProfile{AccountID: "office", DepartmentID: "office", DepartmentCode: "OFFICE", ReportScope: officeReportScope, SystemRole: SystemRoleMember, OrganizationRoleCode: role}
			for _, action := range []string{ActionInspectionEnter, ActionInspectionReview, ActionInspectionManage, ActionInspectionView} {
				profile.Rules = append(profile.Rules, Rule{Action: action, Enabled: true, Effect: EffectAllow})
			}
			result, err := NewService(&memoryRepository{profile: profile}).Effective(context.Background(), "office")
			if err != nil || result.ConfirmedInspectionsOnly || !result.Allowed(ActionInspectionView, Scope{}) {
				t.Fatalf("office configured view %v", err)
			}
			for _, action := range []string{ActionInspectionEnter, ActionInspectionReview, ActionInspectionManage} {
				if !result.Allowed(action, Scope{}) {
					t.Errorf("missing configured %s", action)
				}
			}
		})
	}
}

func TestNonMaintainerCannotUsePersonalReportWriteGrant(t *testing.T) {
	for _, action := range []string{ActionReportGenerate, ActionReportCorrect, ActionReportFinalize, ActionReportOutputSettingsManage} {
		t.Run(action, func(t *testing.T) {
			repo := &memoryRepository{profile: AccessProfile{AccountID: "other", DepartmentID: "other", OrganizationRoleCode: "MINISTER", ReportDefaults: officeLeadershipDefaults, Rules: []Rule{{Action: action, Effect: EffectAllow, Enabled: true}}}}
			result, err := NewService(repo).Effective(context.Background(), "other")
			if err != nil {
				t.Fatal(err)
			}
			if result.Allowed(action, Scope{}) {
				t.Fatal("non-maintainer received write permission")
			}
		})
	}
}

func TestReportPersonalGrantEligibilityAtWriteAndRead(t *testing.T) {
	for _, action := range []string{ActionReportGenerate, ActionReportCorrect, ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportOutputSettingsManage} {
		for _, role := range []string{SystemRoleMember, SystemRoleAdmin} {
			t.Run(role+action, func(t *testing.T) {
				repo := &memoryRepository{profile: AccessProfile{AccountID: "target", SystemRole: role, DepartmentCode: "STUDY"}}
				svc := NewService(repo)
				before, _ := svc.Effective(context.Background(), "target")
				if before.Allowed(action, Scope{}) {
					t.Fatal("default grant")
				}
				rule, err := svc.CreateRule(context.Background(), "operator", Rule{AccountID: "target", Action: action, Effect: EffectAllow})
				if role == SystemRoleMember {
					if !errors.Is(err, ErrReportMaintenanceRequired) {
						t.Fatalf("grant: %v", err)
					}
					if len(repo.mutations) != 0 {
						t.Fatal("rejected grant persisted")
					}
					_, err = svc.UpdateRule(context.Background(), Rule{ID: "existing", AccountID: "target", Action: action, Effect: EffectAllow})
					if !errors.Is(err, ErrReportMaintenanceRequired) {
						t.Fatalf("update: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				result, _ := svc.Effective(context.Background(), "target")
				if !result.Allowed(action, Scope{}) {
					t.Fatal("explicit admin grant missing")
				}
				repo.profile.SystemRole = SystemRoleMember
				result, _ = svc.Effective(context.Background(), "target")
				if result.Allowed(action, Scope{}) {
					t.Fatal("old grant survived role change")
				}
				_ = rule
			})
		}
	}
}
