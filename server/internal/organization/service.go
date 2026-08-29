package organization

import (
	"context"
	"errors"
	"strings"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

var ErrInvalidInput = errors.New("invalid organization input")

type Department struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type OrganizationRole struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

type InspectionType struct {
	ID                   string `json:"id"`
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	ExtensionKind        string `json:"extensionKind"`
	Enabled              bool   `json:"enabled"`
	ParticipatesInReport bool   `json:"participatesInReport"`
	SortOrder            int    `json:"sortOrder"`
}

type DepartmentInspectionScope struct {
	DepartmentID     string `json:"departmentId"`
	InspectionTypeID string `json:"inspectionTypeId"`
	CanEnter         bool   `json:"canEnter"`
	CanView          bool   `json:"canView"`
	CanReview        bool   `json:"canReview"`
	CanExport        bool   `json:"canExport"`
	CanSummarize     bool   `json:"canSummarize"`
	Enabled          bool   `json:"enabled"`
}

type UserAssignment struct {
	AccountID          string `json:"accountId"`
	LoginName          string `json:"loginName"`
	StudentNo          string `json:"studentNo"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	SystemRole         string `json:"systemRole"`
	IsSuperAdmin       bool   `json:"isSuperAdmin"`
	MustChangePassword bool   `json:"mustChangePassword"`
	DepartmentID       string `json:"departmentId"`
	OrganizationRoleID string `json:"organizationRoleId"`
}

type Catalogs struct {
	Departments       []Department       `json:"departments"`
	OrganizationRoles []OrganizationRole `json:"organizationRoles"`
	InspectionTypes   []InspectionType   `json:"inspectionTypes"`
}

type Repository interface {
	ListCatalogs(context.Context, bool) (Catalogs, error)
	ListScopes(context.Context) ([]DepartmentInspectionScope, error)
	UpsertScope(context.Context, DepartmentInspectionScope, audit.Mutation) error
	CreateDepartment(context.Context, Department, audit.Mutation) (Department, error)
	UpdateDepartment(context.Context, Department, audit.Mutation) (Department, error)
	UpdateInspectionType(context.Context, InspectionType, audit.Mutation) (InspectionType, error)
	ListUsers(context.Context) ([]UserAssignment, error)
	AssignUserRole(context.Context, UserAssignment, audit.Mutation) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) ListCatalogs(ctx context.Context, enabledOnly bool) (Catalogs, error) {
	catalogs, err := s.repository.ListCatalogs(ctx, enabledOnly)
	if err != nil || !enabledOnly {
		return catalogs, err
	}
	catalogs.Departments = filterDepartments(catalogs.Departments)
	catalogs.OrganizationRoles = filterRoles(catalogs.OrganizationRoles)
	catalogs.InspectionTypes = filterInspectionTypes(catalogs.InspectionTypes)
	return catalogs, nil
}

func (s *Service) CreateDepartment(ctx context.Context, department Department, metadata ...audit.Metadata) (Department, error) {
	department.Code = strings.ToUpper(strings.TrimSpace(department.Code))
	department.Name = strings.TrimSpace(department.Name)
	if department.Code == "" || department.Name == "" {
		return Department{}, ErrInvalidInput
	}
	department.ID = uuid.NewString()
	department.Enabled = true
	return s.repository.CreateDepartment(ctx, department, organizationMutation(metadata, "department", department.ID, nil, department))
}

func (s *Service) UpdateDepartment(ctx context.Context, department Department, metadata ...audit.Metadata) (Department, error) {
	department.Name = strings.TrimSpace(department.Name)
	if department.ID == "" || department.Name == "" {
		return Department{}, ErrInvalidInput
	}
	catalogs, err := s.repository.ListCatalogs(ctx, false)
	if err != nil {
		return Department{}, err
	}
	var before any
	for _, existing := range catalogs.Departments {
		if existing.ID == department.ID {
			before = existing
			break
		}
	}
	return s.repository.UpdateDepartment(ctx, department, organizationMutation(metadata, "department", department.ID, before, department))
}

func (s *Service) UpdateInspectionType(ctx context.Context, inspectionType InspectionType, metadata ...audit.Metadata) (InspectionType, error) {
	inspectionType.Name = strings.TrimSpace(inspectionType.Name)
	if inspectionType.ID == "" || inspectionType.Name == "" {
		return InspectionType{}, ErrInvalidInput
	}
	catalogs, err := s.repository.ListCatalogs(ctx, false)
	if err != nil {
		return InspectionType{}, err
	}
	var before any
	for _, existing := range catalogs.InspectionTypes {
		if existing.ID == inspectionType.ID {
			before = existing
			break
		}
	}
	return s.repository.UpdateInspectionType(ctx, inspectionType, organizationMutation(metadata, "inspectionType", inspectionType.ID, before, inspectionType))
}

func (s *Service) ListScopes(ctx context.Context) ([]DepartmentInspectionScope, error) {
	return s.repository.ListScopes(ctx)
}

func (s *Service) UpsertScope(ctx context.Context, scope DepartmentInspectionScope, metadata ...audit.Metadata) error {
	if scope.DepartmentID == "" || scope.InspectionTypeID == "" {
		return ErrInvalidInput
	}
	scopes, err := s.repository.ListScopes(ctx)
	if err != nil {
		return err
	}
	var before any
	for _, existing := range scopes {
		if existing.DepartmentID == scope.DepartmentID && existing.InspectionTypeID == scope.InspectionTypeID {
			before = existing
			break
		}
	}
	resourceID := scope.DepartmentID + ":" + scope.InspectionTypeID
	return s.repository.UpsertScope(ctx, scope, organizationMutation(metadata, "departmentInspectionScope", resourceID, before, scope))
}

func (s *Service) ListUsers(ctx context.Context) ([]UserAssignment, error) {
	return s.repository.ListUsers(ctx)
}

func (s *Service) AssignUserRole(ctx context.Context, assignment UserAssignment, metadata ...audit.Metadata) error {
	if assignment.AccountID == "" || assignment.DepartmentID == "" || assignment.OrganizationRoleID == "" {
		return ErrInvalidInput
	}
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return err
	}
	var before any
	for _, existing := range users {
		if existing.AccountID == assignment.AccountID {
			before = existing
			break
		}
	}
	return s.repository.AssignUserRole(ctx, assignment, organizationMutation(metadata, "userAssignment", assignment.AccountID, before, assignment))
}

func organizationMutation(metadata []audit.Metadata, resourceType, resourceID string, before, after any) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: audit.ActionOrganizationUpdated, ResourceType: resourceType, ResourceID: resourceID, Before: before, After: after,
	}}
}

func filterDepartments(values []Department) []Department {
	result := make([]Department, 0, len(values))
	for _, value := range values {
		if value.Enabled {
			result = append(result, value)
		}
	}
	return result
}

func filterRoles(values []OrganizationRole) []OrganizationRole {
	result := make([]OrganizationRole, 0, len(values))
	for _, value := range values {
		if value.Enabled {
			result = append(result, value)
		}
	}
	return result
}

func filterInspectionTypes(values []InspectionType) []InspectionType {
	result := make([]InspectionType, 0, len(values))
	for _, value := range values {
		if value.Enabled {
			result = append(result, value)
		}
	}
	return result
}
