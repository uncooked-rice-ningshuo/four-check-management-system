package organization

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
)

type memoryRepository struct {
	catalogs  Catalogs
	scopes    []DepartmentInspectionScope
	mutations []audit.Mutation
	users     []UserAssignment
	defaults  DepartmentDefaultPermissions
}

func (m *memoryRepository) GetDepartmentDefaults(context.Context, string) (DepartmentDefaultPermissions, error) {
	return m.defaults, nil
}
func (m *memoryRepository) SaveDepartmentDefaults(_ context.Context, input DepartmentDefaultPermissions, mutation audit.Mutation) (DepartmentDefaultPermissions, error) {
	m.mutations = append(m.mutations, mutation)
	input.Version++
	m.defaults = input
	return input, nil
}

func (m *memoryRepository) ListCatalogs(context.Context, bool) (Catalogs, error) {
	return m.catalogs, nil
}
func (m *memoryRepository) ListScopes(context.Context) ([]DepartmentInspectionScope, error) {
	return m.scopes, nil
}
func (m *memoryRepository) UpsertScope(_ context.Context, scope DepartmentInspectionScope, mutation audit.Mutation) error {
	m.scopes = append(m.scopes, scope)
	m.mutations = append(m.mutations, mutation)
	return nil
}
func (m *memoryRepository) CreateDepartment(_ context.Context, d Department, mutation audit.Mutation) (Department, error) {
	m.mutations = append(m.mutations, mutation)
	return d, nil
}
func (m *memoryRepository) UpdateDepartment(_ context.Context, d Department, mutation audit.Mutation) (Department, error) {
	m.mutations = append(m.mutations, mutation)
	return d, nil
}
func (m *memoryRepository) UpdateInspectionType(_ context.Context, i InspectionType, mutation audit.Mutation) (InspectionType, error) {
	m.mutations = append(m.mutations, mutation)
	return i, nil
}
func (m *memoryRepository) ListUsers(context.Context) ([]UserAssignment, error) { return m.users, nil }
func (m *memoryRepository) AssignUserRole(_ context.Context, assignment UserAssignment, mutation audit.Mutation) error {
	m.mutations = append(m.mutations, mutation)
	return nil
}

func TestPublicCatalogsHideDisabledOptions(t *testing.T) {
	repo := &memoryRepository{catalogs: Catalogs{Departments: []Department{{ID: "1", Code: "A", Name: "启用", Enabled: true}, {ID: "2", Code: "B", Name: "禁用", Enabled: false}}}}
	got, err := NewService(repo).ListCatalogs(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Departments) != 1 || got.Departments[0].ID != "1" {
		t.Fatalf("departments = %+v", got.Departments)
	}
}

func TestAdminCatalogsReturnEmptyCollectionsAsArrays(t *testing.T) {
	catalogs, err := NewService(&memoryRepository{}).ListCatalogs(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if catalogs.Departments == nil || catalogs.OrganizationRoles == nil || catalogs.InspectionTypes == nil {
		t.Fatalf("organization catalogs contain nil collections: %+v", catalogs)
	}
}

func TestAdminUsersReturnEmptyCollectionAsArray(t *testing.T) {
	users, err := NewService(&memoryRepository{}).ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(users)
	if err != nil || string(encoded) != "[]" {
		t.Fatalf("empty users JSON = %s, error = %v", encoded, err)
	}
}

func TestScopeRequiresDepartmentAndInspectionType(t *testing.T) {
	svc := NewService(&memoryRepository{})
	err := svc.UpsertScope(context.Background(), DepartmentInspectionScope{CanView: true})
	if err != ErrInvalidInput {
		t.Fatalf("error = %v", err)
	}
}

func TestScopeAllowsConfiguredOfficeInspectionWorkButRejectsUndeliveredExport(t *testing.T) {
	repository := &memoryRepository{}
	repository.catalogs.Departments = []Department{{ID: "office", Code: "OFFICE"}, {ID: "other", Code: "OTHER"}}
	service := NewService(repository)
	for _, test := range []struct {
		scope DepartmentInspectionScope
		want  error
	}{
		{DepartmentInspectionScope{DepartmentID: "office", InspectionTypeID: "type", CanEnter: true}, nil},
		{DepartmentInspectionScope{DepartmentID: "office", InspectionTypeID: "type", CanReview: true}, nil},
		{DepartmentInspectionScope{DepartmentID: "office", InspectionTypeID: "type", CanExport: true}, ErrInvalidInput},
		{DepartmentInspectionScope{DepartmentID: "other", InspectionTypeID: "type", CanExport: true}, ErrInvalidInput},
	} {
		if err := service.UpsertScope(context.Background(), test.scope); err != test.want {
			t.Errorf("scope %+v: error = %v, want %v", test.scope, err, test.want)
		}
	}
}

func TestDepartmentDefaultsKeepReportMemberBoundaryAndConfigureInspectionWork(t *testing.T) {
	repository := &memoryRepository{catalogs: Catalogs{
		Departments: []Department{{ID: "office", Code: "OFFICE"}}, InspectionTypes: []InspectionType{{ID: "type"}},
	}}
	input := DepartmentDefaultPermissions{DepartmentID: "office", Version: 1,
		Scopes:          []DepartmentInspectionScope{{DepartmentID: "office", InspectionTypeID: "type", CanView: true, Enabled: true}},
		InspectionRoles: []InspectionRoleDefaults{{InspectionTypeID: "type", RoleCode: "MEMBER", CanView: true}, {InspectionTypeID: "type", RoleCode: "LEADERSHIP", CanView: true}, {InspectionTypeID: "type", RoleCode: "PRESIDIUM", CanView: true}},
		ReportRoles:     []ReportRoleDefaults{{RoleCode: "MEMBER", CanView: true}, {RoleCode: "MINISTER", CanReview: true}, {RoleCode: "VICE_MINISTER", CanReview: true}},
	}
	service := NewService(repository)
	if _, err := service.SaveDepartmentDefaults(context.Background(), input, audit.Metadata{}); err != nil {
		t.Fatalf("valid office defaults: %v", err)
	}
	if repository.defaults.Version != 2 || len(repository.mutations) != 1 {
		t.Fatalf("save result = %+v, mutations = %d", repository.defaults, len(repository.mutations))
	}
	input.ReportRoles[0].CanReview = true
	if _, err := service.SaveDepartmentDefaults(context.Background(), input, audit.Metadata{}); err != ErrInvalidInput {
		t.Fatalf("office member approval error = %v", err)
	}
	input.ReportRoles[0].CanReview = false
	input.Scopes[0].CanEnter = true
	input.InspectionRoles[0].CanManage = true
	if _, err := service.SaveDepartmentDefaults(context.Background(), input, audit.Metadata{}); err != nil {
		t.Fatalf("configured office inspection work error = %v", err)
	}
	if len(repository.mutations) != 2 {
		t.Fatal("configured inspection work was not persisted")
	}
}

func TestDepartmentDefaultsAllowReportRolesForAnotherDepartment(t *testing.T) {
	repository := &memoryRepository{catalogs: Catalogs{
		Departments: []Department{{ID: "other", Code: "OTHER"}}, InspectionTypes: []InspectionType{{ID: "type"}},
	}}
	input := DepartmentDefaultPermissions{DepartmentID: "other", Version: 1,
		Scopes:          []DepartmentInspectionScope{{DepartmentID: "other", InspectionTypeID: "type"}},
		InspectionRoles: []InspectionRoleDefaults{{InspectionTypeID: "type", RoleCode: "MEMBER"}, {InspectionTypeID: "type", RoleCode: "LEADERSHIP"}, {InspectionTypeID: "type", RoleCode: "PRESIDIUM"}},
		ReportRoles:     []ReportRoleDefaults{{RoleCode: "MEMBER", CanGenerate: true}, {RoleCode: "MINISTER", CanReview: true}, {RoleCode: "VICE_MINISTER", CanReview: true}},
	}
	if _, err := NewService(repository).SaveDepartmentDefaults(context.Background(), input, audit.Metadata{}); err != nil {
		t.Fatalf("non-office report defaults: %v", err)
	}
	input.ReportRoles[0].CanFinalize = true
	if _, err := NewService(repository).SaveDepartmentDefaults(context.Background(), input, audit.Metadata{}); err != ErrInvalidInput {
		t.Fatalf("non-office member finalize error = %v", err)
	}
}

func TestCreateDepartmentProvidesOrganizationAuditMutation(t *testing.T) {
	repository := &memoryRepository{}
	created, err := NewService(repository).CreateDepartment(context.Background(), Department{Code: "study", Name: "学习部"}, audit.Metadata{ActorID: "admin-1", RequestID: "organization-request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Metadata.ActorID != "admin-1" || mutation.Entry.Action != audit.ActionOrganizationUpdated || mutation.Entry.ResourceID != created.ID {
		t.Fatalf("mutation = %+v", mutation)
	}
}

func TestAssignmentAuditUsesStoredAccountFieldsAndRejectsMissingTarget(t *testing.T) {
	current := UserAssignment{SessionVersion: 12, AccountID: "user", LoginName: "real-login", StudentNo: "real-student", Name: "Real name", Status: "ACTIVE", SystemRole: "MEMBER"}
	repo := &memoryRepository{users: []UserAssignment{current}}
	svc := NewService(repo)
	requested := UserAssignment{SessionVersion: 999, Version: 13, AccountID: "user", LoginName: "forged", Name: "Forged", Status: "DISABLED", SystemRole: "SUPER_ADMIN", IsSuperAdmin: true, DepartmentID: "new-department", OrganizationRoleID: "new-role"}
	if err := svc.AssignUserRole(context.Background(), requested); err != nil {
		t.Fatal(err)
	}
	before, beforeOK := repo.mutations[0].Entry.Before.(UserAssignment)
	after, afterOK := repo.mutations[0].Entry.After.(UserAssignment)
	if !beforeOK || !afterOK || before.Version != 13 || after.Version != 14 || after.SessionVersion != current.SessionVersion || after.Name != current.Name || after.LoginName != current.LoginName || after.SystemRole != current.SystemRole || after.Status != current.Status || after.IsSuperAdmin || after.DepartmentID != "new-department" || after.OrganizationRoleID != "new-role" {
		t.Fatal("client forged authoritative audit fields")
	}
	requested.AccountID = "missing"
	if err := svc.AssignUserRole(context.Background(), requested); err == nil || len(repo.mutations) != 1 {
		t.Fatal("missing target mutated or audited")
	}
}

func TestAssignmentVersionCannotBeProvidedOrExposedAsJSON(t *testing.T) {
	var assignment UserAssignment
	if err := json.Unmarshal([]byte(`{"accountId":"user","SessionVersion":999,"sessionVersion":999,"version":12}`), &assignment); err != nil {
		t.Fatal(err)
	}
	if assignment.SessionVersion != 0 || assignment.Version != 12 {
		t.Fatal("client injected internal version")
	}
	assignment.SessionVersion = 12
	encoded, err := json.Marshal(assignment)
	if err != nil || strings.Contains(strings.ToLower(string(encoded)), "sessionversion") || !strings.Contains(string(encoded), `"version":12`) {
		t.Fatal("internal version exposed", err)
	}
}

func TestAdminUserListMapsInternalVersionToClientToken(t *testing.T) {
	users, err := NewService(&memoryRepository{users: []UserAssignment{{AccountID: "user", SessionVersion: 7}}}).ListUsers(context.Background())
	if err != nil || len(users) != 1 || users[0].Version != 8 {
		t.Fatalf("users = %+v, error = %v", users, err)
	}
}

func TestAssignmentRejectsVersionFromStaleEditor(t *testing.T) {
	current := UserAssignment{SessionVersion: 12, AccountID: "user", Name: "Current name", DepartmentID: "old-department", OrganizationRoleID: "old-role"}
	repo := &memoryRepository{users: []UserAssignment{current}}
	var stale UserAssignment
	if err := json.Unmarshal([]byte(`{"accountId":"user","departmentId":"new-department","organizationRoleId":"new-role","version":11}`), &stale); err != nil {
		t.Fatal(err)
	}
	if err := NewService(repo).AssignUserRole(context.Background(), stale); err != auth.ErrVersionConflict {
		t.Fatalf("stale assignment error = %v, want version conflict", err)
	}
	if len(repo.mutations) != 0 {
		t.Fatal("stale assignment appended an audit mutation")
	}
}
