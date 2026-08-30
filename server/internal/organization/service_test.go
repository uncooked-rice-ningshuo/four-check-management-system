package organization

import (
	"context"
	"testing"

	"four-check-management-system/server/internal/audit"
)

type memoryRepository struct {
	catalogs  Catalogs
	scopes    []DepartmentInspectionScope
	mutations []audit.Mutation
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
func (m *memoryRepository) ListUsers(context.Context) ([]UserAssignment, error) { return nil, nil }
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

func TestScopeRequiresDepartmentAndInspectionType(t *testing.T) {
	svc := NewService(&memoryRepository{})
	err := svc.UpsertScope(context.Background(), DepartmentInspectionScope{CanView: true})
	if err != ErrInvalidInput {
		t.Fatalf("error = %v", err)
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
