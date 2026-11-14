package organization

import (
	"context"
	"errors"
	"strings"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"github.com/google/uuid"
)

var ErrInvalidInput = errors.New("invalid organization input")

var ErrUserNotFound = errors.New("organization assignment account not found")

type Department struct {
	ID                string `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	PermissionVersion int    `json:"permissionVersion"`
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
	CanManage        bool   `json:"canManage"`
	CanReview        bool   `json:"canReview"`
	CanExport        bool   `json:"canExport"`
	Enabled          bool   `json:"enabled"`
}

// ReportRoleDefaults stores the weekly-report baseline for a role in one department.
type ReportRoleDefaults struct {
	RoleCode          string `json:"roleCode"`
	CanView           bool   `json:"canView"`
	CanGenerate       bool   `json:"canGenerate"`
	CanCorrect        bool   `json:"canCorrect"`
	CanReview         bool   `json:"canReview"`
	CanReturn         bool   `json:"canReturn"`
	CanFinalize       bool   `json:"canFinalize"`
	CanPublish        bool   `json:"canPublish"`
	CanOutputSettings bool   `json:"canOutputSettings"`
	CanArchive        bool   `json:"canArchive"`
}

// DepartmentReportScope is the department's default weekly-report capability
// ceiling. A role default grants an action only when this scope also enables it.
type DepartmentReportScope struct {
	Enabled           bool `json:"enabled"`
	CanView           bool `json:"canView"`
	CanGenerate       bool `json:"canGenerate"`
	CanCorrect        bool `json:"canCorrect"`
	CanReview         bool `json:"canReview"`
	CanReturn         bool `json:"canReturn"`
	CanFinalize       bool `json:"canFinalize"`
	CanPublish        bool `json:"canPublish"`
	CanOutputSettings bool `json:"canOutputSettings"`
	CanArchive        bool `json:"canArchive"`
}

type InspectionRoleDefaults struct {
	InspectionTypeID string `json:"inspectionTypeId"`
	RoleCode         string `json:"roleCode"`
	CanView          bool   `json:"canView"`
	CanEnter         bool   `json:"canEnter"`
	CanManage        bool   `json:"canManage"`
	CanReview        bool   `json:"canReview"`
	CanExport        bool   `json:"canExport"`
}

type DepartmentDefaultPermissions struct {
	DepartmentID    string                      `json:"departmentId"`
	Version         int                         `json:"version"`
	ReportScope     DepartmentReportScope       `json:"reportScope"`
	Scopes          []DepartmentInspectionScope `json:"scopes"`
	InspectionRoles []InspectionRoleDefaults    `json:"inspectionRoles"`
	ReportRoles     []ReportRoleDefaults        `json:"reportRoles"`
}

var ErrPermissionVersionConflict = errors.New("department permission version conflict")

type defaultPermissionRepository interface {
	GetDepartmentDefaults(context.Context, string) (DepartmentDefaultPermissions, error)
	SaveDepartmentDefaults(context.Context, DepartmentDefaultPermissions, audit.Mutation) (DepartmentDefaultPermissions, error)
}

type UserAssignment struct {
	// SessionVersion is the internal authentication epoch; Version is its one-based client concurrency token.
	SessionVersion     int    `json:"-"`
	Version            int    `json:"version"`
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
	if err != nil {
		return catalogs, err
	}
	if enabledOnly {
		catalogs.Departments = filterDepartments(catalogs.Departments)
		catalogs.OrganizationRoles = filterRoles(catalogs.OrganizationRoles)
		catalogs.InspectionTypes = filterInspectionTypes(catalogs.InspectionTypes)
	} else {
		catalogs.Departments = nonNil(catalogs.Departments)
		catalogs.OrganizationRoles = nonNil(catalogs.OrganizationRoles)
		catalogs.InspectionTypes = nonNil(catalogs.InspectionTypes)
	}
	return catalogs, nil
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return make([]T, 0)
	}
	return values
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

func (s *Service) GetDepartmentDefaults(ctx context.Context, departmentID string) (DepartmentDefaultPermissions, error) {
	repository, ok := s.repository.(defaultPermissionRepository)
	if !ok {
		return DepartmentDefaultPermissions{}, errors.New("department defaults repository unavailable")
	}
	return repository.GetDepartmentDefaults(ctx, departmentID)
}

func (s *Service) SaveDepartmentDefaults(ctx context.Context, input DepartmentDefaultPermissions, metadata audit.Metadata) (DepartmentDefaultPermissions, error) {
	if input.DepartmentID == "" || input.Version < 1 {
		return DepartmentDefaultPermissions{}, ErrInvalidInput
	}
	catalogs, err := s.repository.ListCatalogs(ctx, false)
	if err != nil {
		return DepartmentDefaultPermissions{}, err
	}
	departmentCode := ""
	for _, department := range catalogs.Departments {
		if department.ID == input.DepartmentID {
			departmentCode = department.Code
			break
		}
	}
	if departmentCode == "" {
		return DepartmentDefaultPermissions{}, ErrInvalidInput
	}
	knownTypes := map[string]bool{}
	for _, inspectionType := range catalogs.InspectionTypes {
		knownTypes[inspectionType.ID] = true
	}
	seenTypes := map[string]bool{}
	for _, scope := range input.Scopes {
		if scope.DepartmentID != input.DepartmentID || !knownTypes[scope.InspectionTypeID] || seenTypes[scope.InspectionTypeID] ||
			(scope.CanExport && departmentCode != "STUDY" && departmentCode != "SPORTS" && departmentCode != "DORMITORY") {
			return DepartmentDefaultPermissions{}, ErrInvalidInput
		}
		seenTypes[scope.InspectionTypeID] = true
	}
	if len(seenTypes) != len(knownTypes) {
		return DepartmentDefaultPermissions{}, ErrInvalidInput
	}
	seenInspectionRoles := map[string]bool{}
	for _, role := range input.InspectionRoles {
		key := role.InspectionTypeID + ":" + role.RoleCode
		if !knownTypes[role.InspectionTypeID] || seenInspectionRoles[key] || (role.RoleCode != "MEMBER" && role.RoleCode != "LEADERSHIP" && role.RoleCode != "PRESIDIUM") ||
			(role.CanExport && departmentCode != "STUDY" && departmentCode != "SPORTS" && departmentCode != "DORMITORY") {
			return DepartmentDefaultPermissions{}, ErrInvalidInput
		}
		seenInspectionRoles[key] = true
	}
	if len(seenInspectionRoles) != len(knownTypes)*3 {
		return DepartmentDefaultPermissions{}, ErrInvalidInput
	}
	seenRoles := map[string]bool{}
	for _, role := range input.ReportRoles {
		if seenRoles[role.RoleCode] ||
			(role.RoleCode != "MEMBER" && role.RoleCode != "MINISTER" && role.RoleCode != "VICE_MINISTER") ||
			(role.RoleCode == "MEMBER" && (role.CanReview || role.CanReturn || role.CanFinalize || role.CanPublish || role.CanOutputSettings)) {
			return DepartmentDefaultPermissions{}, ErrInvalidInput
		}
		seenRoles[role.RoleCode] = true
	}
	if len(seenRoles) != 3 {
		return DepartmentDefaultPermissions{}, ErrInvalidInput
	}
	repository, ok := s.repository.(defaultPermissionRepository)
	if !ok {
		return DepartmentDefaultPermissions{}, errors.New("department defaults repository unavailable")
	}
	before, err := repository.GetDepartmentDefaults(ctx, input.DepartmentID)
	if err != nil {
		return DepartmentDefaultPermissions{}, err
	}
	return repository.SaveDepartmentDefaults(ctx, input, organizationMutation([]audit.Metadata{metadata}, "departmentDefaultPermissions", input.DepartmentID, before, input))
}

func (s *Service) UpsertScope(ctx context.Context, scope DepartmentInspectionScope, metadata ...audit.Metadata) error {
	if scope.DepartmentID == "" || scope.InspectionTypeID == "" {
		return ErrInvalidInput
	}
	catalogs, err := s.repository.ListCatalogs(ctx, false)
	if err != nil {
		return err
	}
	for _, department := range catalogs.Departments {
		if department.ID == scope.DepartmentID && scope.CanExport && department.Code != "STUDY" && department.Code != "SPORTS" && department.Code != "DORMITORY" {
			return ErrInvalidInput
		}
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
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].Version = users[i].SessionVersion + 1
	}
	return nonNil(users), nil
}

func (s *Service) AssignUserRole(ctx context.Context, assignment UserAssignment, metadata ...audit.Metadata) error {
	if assignment.AccountID == "" || assignment.DepartmentID == "" || assignment.OrganizationRoleID == "" {
		return ErrInvalidInput
	}
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return err
	}
	var before *UserAssignment
	for _, existing := range users {
		if existing.AccountID == assignment.AccountID {
			before = &existing
			break
		}
	}
	if before == nil {
		return ErrUserNotFound
	}
	if assignment.Version <= 0 {
		return ErrInvalidInput
	}
	if assignment.Version != before.SessionVersion+1 {
		return auth.ErrVersionConflict
	}
	// Only department and organization role are writable here; audit account fields
	// must describe the stored account rather than untrusted client display values.
	before.Version = before.SessionVersion + 1
	after := *before
	after.DepartmentID, after.OrganizationRoleID = assignment.DepartmentID, assignment.OrganizationRoleID
	after.Version++
	return s.repository.AssignUserRole(ctx, after, organizationMutation(metadata, "userAssignment", after.AccountID, *before, after))
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
