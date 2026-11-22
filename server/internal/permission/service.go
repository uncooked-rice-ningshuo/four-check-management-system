package permission

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

const (
	SystemRoleSuperAdmin = "SUPER_ADMIN"
	SystemRoleAdmin      = "ADMIN"
	SystemRoleMember     = "MEMBER"

	EffectAllow = "ALLOW"
	EffectDeny  = "DENY"

	SourceSystemRole       = "SYSTEM_ROLE"
	SourceOrganizationRole = "ORGANIZATION_ROLE"
	SourceDepartmentMap    = "DEPARTMENT_MAPPING"
	SourceUserRule         = "USER_RULE"

	ActionUserManage                 = "USER_MANAGE"
	ActionPermissionManage           = "PERMISSION_MANAGE"
	ActionLedgerExport               = "DISCIPLINE_LEDGER_EXPORT"
	ActionDisciplineConfirm          = "DISCIPLINE_CASE_CONFIRM"
	ActionLedgerView                 = "DISCIPLINE_LEDGER_VIEW"
	ActionMasterDataManage           = "MASTER_DATA_MANAGE"
	ActionDictionaryManage           = "DICTIONARY_MANAGE"
	ActionConfigurationView          = "CONFIGURATION_VIEW"
	ActionInspectionManage           = "INSPECTION_MANAGE"
	ActionInspectionEnter            = "INSPECTION_ENTER"
	ActionInspectionView             = "INSPECTION_VIEW"
	ActionInspectionReview           = "INSPECTION_REVIEW"
	ActionInspectionExport           = "INSPECTION_EXPORT"
	ActionMorningScheduleManage      = "MORNING_SCHEDULE_MANAGE"
	ActionReportView                 = "REPORT_VIEW"
	ActionReportGenerate             = "REPORT_GENERATE"
	ActionReportFileOutput           = "REPORT_FILE_OUTPUT"
	ActionReportCorrect              = "REPORT_CORRECT"
	ActionReportForceGenerate        = "REPORT_FORCE_GENERATE"
	ActionReportObligationManage     = "REPORT_OBLIGATION_MANAGE"
	ActionReportOutputSettingsManage = "REPORT_OUTPUT_SETTINGS_MANAGE"
	ActionReportReview               = "REPORT_REVIEW"
	ActionReportReturn               = "REPORT_RETURN"
	ActionReportFinalize             = "REPORT_FINALIZE"
	ActionReportPublish              = "REPORT_PUBLISH"
	ActionArchiveView                = "ARCHIVE_VIEW"
)

var ErrInvalidRule = errors.New("invalid permission rule")
var ErrForceGenerateRoleRequired = errors.New("force generation permission requires report maintenance responsibility")
var ErrReportObligationRoleRequired = errors.New("weekly obligation permission requires report maintenance responsibility")
var ErrReportMaintenanceRequired = errors.New("report maintenance department required")

var disciplineActions = []string{"DISCIPLINE_RULE_VIEW", "DISCIPLINE_RULE_EDIT", "DISCIPLINE_RULE_PUBLISH", "DISCIPLINE_DOCUMENT_TEMPLATE_EDIT", "DISCIPLINE_LEDGER_VIEW", "DISCIPLINE_LEDGER_EXPORT", "DISCIPLINE_CASE_VIEW", "DISCIPLINE_CASE_CONFIRM", "DISCIPLINE_ARCHIVE_VIEW", "DISCIPLINE_ARCHIVE_EXPORT", "DISCIPLINE_DOCUMENT_EXPORT"}

// IsDisciplineAction identifies the configurable discipline capabilities.
func IsDisciplineAction(action string) bool { return slices.Contains(disciplineActions, action) }

var allActions = append([]string{
	ActionUserManage, ActionPermissionManage, ActionMasterDataManage, ActionDictionaryManage, ActionConfigurationView,
	ActionInspectionEnter, ActionInspectionManage, ActionInspectionView, ActionInspectionReview, ActionInspectionExport, ActionMorningScheduleManage,
	ActionReportView, ActionReportGenerate, ActionReportFileOutput, ActionReportCorrect, ActionReportForceGenerate, ActionReportObligationManage, ActionReportOutputSettingsManage, ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionArchiveView,
}, disciplineActions...)

type Scope struct {
	GradeID          string `json:"-"`
	MajorID          string `json:"-"`
	AcademicYearID   string `json:"academicYearId,omitempty"`
	SemesterID       string `json:"semesterId,omitempty"`
	WeekID           string `json:"weekId,omitempty"`
	InspectionTypeID string `json:"inspectionTypeId,omitempty"`
	DepartmentID     string `json:"departmentId,omitempty"`
	ClassID          string `json:"classId,omitempty"`
	Grade            string `json:"grade,omitempty"`
}

type Source struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}

type Rule struct {
	SourceDepartmentID string           `json:"sourceDepartmentId,omitempty"`
	Mode               string           `json:"mode"`
	Ranges             []OperationRange `json:"ranges,omitempty"`
	StartsAt           time.Time        `json:"startsAt"`
	EndsAt             *time.Time       `json:"endsAt,omitempty"`

	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	Action     string `json:"action"`
	Effect     string `json:"effect"`
	SourceType string `json:"sourceType"`
	Scope      Scope  `json:"scope"`
	Note       string `json:"note"`
	Enabled    bool   `json:"enabled"`
	CreatedBy  string `json:"createdBy,omitempty"`
}

type DepartmentScope struct {
	DepartmentID     string
	InspectionTypeID string
	CanEnter         bool
	CanView          bool
	CanManage        bool
	CanReview        bool
	CanExport        bool
	Enabled          bool
}

type AccessProfile struct {
	DisciplineScope          []string
	DefaultDisciplineActions []string
	ModuleValidity           []ModuleValidity
	MorningInspectionTypeID  string
	AssignedDepartmentScopes []DepartmentScope
	AssignedReportScope      *ReportScope
	AssignedMorningSchedule  bool
	PendingDepartmentIDs     []string
	Departments              []AccessProfile
	StartsAt                 time.Time
	EndsAt                   *time.Time
	TemplateVersion          int
	OperationRanges          []GrantAction
	CanManageMorningSchedule bool
	DefaultMorningSchedule   bool

	AccountID            string
	SystemRole           string
	DepartmentID         string
	DepartmentCode       string
	OrganizationRoleCode string
	DepartmentScopes     []DepartmentScope
	Rules                []Rule
	ReportDefaults       *ReportRoleDefaults
	ReportScope          *ReportScope
	InspectionDefaults   map[string]*InspectionRoleDefaults
}

type InspectionRoleDefaults struct {
	CanView, CanEnter, CanManage, CanReview, CanExport bool
}

type ReportRoleDefaults struct {
	CanView, CanGenerate, CanFileOutput, CanCorrect, CanReview, CanReturn, CanFinalize bool
	CanForceGenerate, CanObligationManage                                              bool
	CanPublish, CanOutputSettings, CanArchive                                          bool
}

type ReportScope struct {
	Enabled bool
	ReportRoleDefaults
}

type EffectivePermission struct {
	Status             string               `json:"status,omitempty"`
	Current            *EffectivePermission `json:"current,omitempty"`
	SourceDepartmentID string               `json:"sourceDepartmentId,omitempty"`
	Ranges             []OperationRange     `json:"ranges,omitempty"`
	StartsAt           time.Time            `json:"startsAt,omitempty"`
	EndsAt             *time.Time           `json:"endsAt,omitempty"`
	TemplateVersion    int                  `json:"templateVersion,omitempty"`
	ConfirmedOnly      bool                 `json:"confirmedOnly,omitempty"`

	Action  string   `json:"action"`
	Effect  string   `json:"effect"`
	Scope   Scope    `json:"scope"`
	Sources []Source `json:"sources"`
}

type EffectiveResult struct {
	PendingDepartmentIDs     []string              `json:"pendingDepartmentIds,omitempty"`
	DefaultPermissions       []EffectivePermission `json:"defaultPermissions"`
	DepartmentIDs            []string              `json:"departmentIds"`
	ReportGrantableActions   []string              `json:"reportGrantableActions"`
	PrimaryDepartmentID      string                `json:"-"`
	DepartmentCode           string                `json:"-"`
	OrganizationRoleCode     string                `json:"-"`
	CanReviseFinalizedReport bool                  `json:"canReviseFinalizedReport"`
	ConfirmedInspectionsOnly bool                  `json:"confirmedInspectionsOnly,omitempty"`
	AccountID                string                `json:"accountId"`
	SystemRole               string                `json:"systemRole"`
	Permissions              []EffectivePermission `json:"permissions"`
}

// Allowed checks a concrete class when present; classless calls only check action eligibility.
// Record writes separately authorize every class in the repository transaction.
func (r EffectiveResult) Allowed(action string, requested Scope) bool {
	matchesRange := func(entry EffectivePermission) bool {
		return requested.ClassID == "" || MatchesOperationRanges(entry.Ranges, OperationTarget{GradeID: requested.GradeID, MajorID: requested.MajorID, ClassID: requested.ClassID})
	}
	for _, allow := range r.Permissions {
		if allow.Action != action || allow.Effect != EffectAllow || !scopeMatches(allow.Scope, requested) || !matchesRange(allow) {
			continue
		}
		denied := false
		for _, deny := range r.Permissions {
			if deny.Action == action && deny.Effect == EffectDeny && (requested.ClassID != "" || len(deny.Ranges) == 0) && (deny.SourceDepartmentID == "" || deny.SourceDepartmentID == allow.SourceDepartmentID) && scopesOverlap(deny.Scope, requested) && matchesRange(deny) {
				denied = true
				break
			}
		}
		if !denied {
			return true
		}
	}
	return false
}

// HasAnyAllowed reports whether at least one part of an action's scope remains
// available after a covering denial. It is for reading global defaults only;
// resource operations must continue to check their concrete scope.
func (r EffectiveResult) HasAnyAllowed(action string) bool {
	for _, allowed := range r.Permissions {
		if allowed.Action != action || allowed.Effect != EffectAllow {
			continue
		}
		covered := false
		for _, denied := range r.Permissions {
			if denied.Action == action && denied.Effect == EffectDeny && (denied.SourceDepartmentID == "" || denied.SourceDepartmentID == allowed.SourceDepartmentID) && len(denied.Ranges) == 0 && scopeMatches(denied.Scope, allowed.Scope) {
				covered = true
				break
			}
		}
		if !covered {
			return true
		}
	}
	return false
}

// AllowsWholeScope reports whether an action covers an entire resource scope.
// A class- or grade-limited allow cannot attest a department/type cell, while
// any overlapping deny continues to veto that cell as it does individual rows.
func AllowsWholeScope(permissions []EffectivePermission, action string, requested Scope) bool {
	for _, allowed := range permissions {
		if allowed.Action != action || allowed.Effect != EffectAllow || allowed.Scope.ClassID != "" || allowed.Scope.Grade != "" || len(allowed.Ranges) != 0 || !scopeMatches(allowed.Scope, requested) {
			continue
		}
		denied := false
		for _, rule := range permissions {
			if rule.Action == action && rule.Effect == EffectDeny && (rule.SourceDepartmentID == "" || rule.SourceDepartmentID == allowed.SourceDepartmentID) && scopesOverlap(rule.Scope, requested) {
				denied = true
				break
			}
		}
		if !denied {
			return true
		}
	}
	return false
}

func scopesOverlap(left, right Scope) bool {
	return overlaps(left.AcademicYearID, right.AcademicYearID) && overlaps(left.SemesterID, right.SemesterID) &&
		overlaps(left.WeekID, right.WeekID) && overlaps(left.InspectionTypeID, right.InspectionTypeID) &&
		overlaps(left.DepartmentID, right.DepartmentID) && overlaps(left.ClassID, right.ClassID) && overlaps(left.Grade, right.Grade)
}

func overlaps(left, right string) bool { return left == "" || right == "" || left == right }

type Repository interface {
	LoadAccessProfile(context.Context, string) (AccessProfile, error)
	ListRules(context.Context, string) ([]Rule, error)
	CreateRule(context.Context, Rule, audit.Mutation) (Rule, error)
	UpdateRule(context.Context, Rule, audit.Mutation) (Rule, error)
	DeleteRule(context.Context, string, string, audit.Mutation) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Effective(ctx context.Context, accountID string) (EffectiveResult, error) {
	profile, err := s.repository.LoadAccessProfile(ctx, accountID)
	if err != nil {
		return EffectiveResult{}, err
	}
	return EvaluateProfile(profile), nil
}

// EvaluateProfile applies the same authorization rules to a profile read inside a business transaction.
func EvaluateProfile(profile AccessProfile) EffectiveResult {
	rules := profile.Rules
	profile.Rules = nil
	result := effectiveProfile(profile)
	result.PendingDepartmentIDs = profile.PendingDepartmentIDs
	result.DepartmentIDs = []string{}
	result.DefaultPermissions = append([]EffectivePermission{}, result.Permissions...)
	now := time.Now().UTC()
	for _, department := range profile.Departments {
		result.DepartmentIDs = append(result.DepartmentIDs, department.DepartmentID)
		current := effectiveProfile(department)
		baseline := department
		if department.AssignedDepartmentScopes != nil {
			baseline.DepartmentScopes = department.AssignedDepartmentScopes
			baseline.ReportScope = department.AssignedReportScope
			baseline.CanManageMorningSchedule = department.AssignedMorningSchedule
		}
		for _, entry := range effectiveProfile(baseline).Permissions {
			overridden := false
			for _, rule := range rules {
				if rule.Enabled && (rule.Mode == "OVERRIDE" || rule.Mode == "DISABLE") && rule.SourceDepartmentID == department.DepartmentID && rule.Action == entry.Action && rule.Scope.InspectionTypeID == entry.Scope.InspectionTypeID {
					overridden = true
					break
				}
			}
			entry.SourceDepartmentID = department.DepartmentID
			entry.StartsAt, entry.EndsAt, entry.TemplateVersion = department.StartsAt, department.EndsAt, department.TemplateVersion
			moduleKey := entry.Scope.InspectionTypeID
			if strings.HasPrefix(entry.Action, "REPORT_") || entry.Action == ActionArchiveView {
				moduleKey = "REPORT"
			}
			if entry.Action == ActionMorningScheduleManage {
				moduleKey = department.MorningInspectionTypeID
			}
			for _, term := range department.ModuleValidity {
				if term.ModuleKey != moduleKey {
					continue
				}
				entry.EndsAt = term.EndsAt
				if term.EndsAt == nil && term.ValidityYears > 0 {
					end := department.StartsAt.AddDate(term.ValidityYears, 0, 0)
					entry.EndsAt = &end
				}
			}

			entry.ConfirmedOnly = entry.Action == ActionInspectionView && department.DepartmentCode == "OFFICE"
			for _, configured := range department.OperationRanges {
				if configured.Action == entry.Action && configured.InspectionTypeID == entry.Scope.InspectionTypeID {
					entry.Ranges = configured.Ranges
				}
			}
			result.DefaultPermissions = append(result.DefaultPermissions, entry)
			if !retainFixedRanges(&entry) {
				continue
			}
			if !departmentSupports(department, entry.Action, entry.Scope.InspectionTypeID) || overridden || now.Before(department.StartsAt) || (entry.EndsAt != nil && !now.Before(*entry.EndsAt)) {
				continue
			}
			result.Permissions = append(result.Permissions, entry)
		}
		result.ReportGrantableActions = append(result.ReportGrantableActions, current.ReportGrantableActions...)
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.Mode == "DISABLE" || now.Before(rule.StartsAt) || (rule.EndsAt != nil && !now.Before(*rule.EndsAt)) {
			continue
		}
		if rule.Effect == EffectAllow && requiresReportMaintenance(rule.Action) && !reportGrantEligible(profile, rule.Action) {
			continue
		}
		entry := userRulePermission(rule)
		if !retainFixedRanges(&entry) {
			continue
		}
		if rule.SourceDepartmentID != "" {
			found := false
			for _, department := range profile.Departments {
				if department.DepartmentID == rule.SourceDepartmentID && departmentSupports(department, rule.Action, rule.Scope.InspectionTypeID) {
					found = true
					entry.ConfirmedOnly = rule.Action == ActionInspectionView && department.DepartmentCode == "OFFICE"
					break
				}
			}
			if !found {
				continue
			}
		} else if rule.Effect == EffectAllow && profile.SystemRole == SystemRoleMember && requiresDepartmentSource(rule.Action) {
			continue
		}
		result.Permissions = append(result.Permissions, entry)
	}
	result.ConfirmedInspectionsOnly = false
	hasView, unconfirmedView := false, false
	for i := range result.Permissions {
		entry := &result.Permissions[i]
		if entry.Action != ActionInspectionView || entry.Effect != EffectAllow {
			continue
		}
		hasView = true
		for _, write := range result.Permissions {
			if write.SourceDepartmentID == entry.SourceDepartmentID && write.Effect == EffectAllow && (write.Action == ActionInspectionEnter || write.Action == ActionInspectionManage || write.Action == ActionInspectionReview) {
				if (entry.Scope.InspectionTypeID == "" || write.Scope.InspectionTypeID == "" || entry.Scope.InspectionTypeID == write.Scope.InspectionTypeID) && hasAllowedInspectionWrite(result.ForDepartment(entry.SourceDepartmentID), write.Action, entry.Scope.InspectionTypeID) {
					entry.ConfirmedOnly = false
				}
			}
		}
		if !entry.ConfirmedOnly {
			unconfirmedView = true
		}
	}
	result.ConfirmedInspectionsOnly = hasView && !unconfirmedView
	result.CanReviseFinalizedReport = result.HasAnyAllowed(ActionReportCorrect)
	for i := range result.DefaultPermissions {
		entry := &result.DefaultPermissions[i]
		entry.Status = "当前未开放"
		if now.Before(entry.StartsAt) {
			entry.Status = "未生效"
		} else if entry.EndsAt != nil && !now.Before(*entry.EndsAt) {
			entry.Status = "已到期"
		}
		for _, rule := range rules {
			if rule.Enabled && rule.SourceDepartmentID == entry.SourceDepartmentID && rule.Action == entry.Action && rule.Scope.InspectionTypeID == entry.Scope.InspectionTypeID && (rule.Mode == "OVERRIDE" || rule.Mode == "DISABLE") {
				entry.Status = "个人设置已禁用"
				if rule.Mode != "DISABLE" && rule.Effect != EffectDeny {
					entry.Status = "个人设置未生效"
					if rule.EndsAt != nil && !now.Before(*rule.EndsAt) {
						entry.Status = "个人设置已到期"
					}
				}
			}
		}
		scoped := result.ForDepartment(entry.SourceDepartmentID)
		for _, active := range scoped.Permissions {
			if active.SourceDepartmentID == entry.SourceDepartmentID && active.Action == entry.Action && active.Effect == EffectAllow && active.Scope.InspectionTypeID == entry.Scope.InspectionTypeID {
				entry.Current = &active
				entry.Status = "有效"
				for _, deny := range scoped.Permissions {
					if deny.Action != entry.Action || deny.Effect != EffectDeny || !scopesOverlap(deny.Scope, active.Scope) {
						continue
					}
					entry.Status = "部分范围受限"
					if rangesCover(deny.Ranges, active.Ranges) && scopeMatches(deny.Scope, active.Scope) {
						entry.Status = "已禁用"
						break
					}
				}
				break
			}
		}
	}
	return result
}

func effectiveProfile(profile AccessProfile) EffectiveResult {
	result := EffectiveResult{PrimaryDepartmentID: profile.DepartmentID, DepartmentCode: profile.DepartmentCode, OrganizationRoleCode: profile.OrganizationRoleCode, AccountID: profile.AccountID, SystemRole: profile.SystemRole, Permissions: make([]EffectivePermission, 0)}
	result.ReportGrantableActions = []string{}
	for _, action := range allActions {
		if requiresReportMaintenance(action) && reportGrantEligible(profile, action) {
			result.ReportGrantableActions = append(result.ReportGrantableActions, action)
		}
	}
	if profile.SystemRole == SystemRoleSuperAdmin {
		result.CanReviseFinalizedReport = true
		for _, action := range allActions {
			if action == ActionReportForceGenerate || action == ActionReportObligationManage {
				continue
			}
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "超级管理员"}))
		}
		return result
	}
	if profile.SystemRole == SystemRoleAdmin {
		for _, action := range []string{ActionConfigurationView, ActionMasterDataManage, ActionDictionaryManage} {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "系统管理员"}))
		}
	}
	for _, action := range profile.DefaultDisciplineActions {
		if IsDisciplineAction(action) && slices.Contains(profile.DisciplineScope, action) {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceOrganizationRole, Label: "角色模板处分管理"}))
		}
	}
	if profile.CanManageMorningSchedule && profile.DefaultMorningSchedule {
		result.Permissions = append(result.Permissions, allow(ActionMorningScheduleManage, Scope{}, Source{Type: SourceOrganizationRole, Label: "早签计划默认权限"}))
	}
	for _, mapping := range profile.DepartmentScopes {
		if !mapping.Enabled || mapping.DepartmentID != profile.DepartmentID {
			continue
		}
		// The user's primary-department mapping grants actions for an inspection
		// type; the action applies to that type across departments.
		scope := Scope{InspectionTypeID: mapping.InspectionTypeID}
		inspectionDefaults := profile.InspectionDefaults[mapping.InspectionTypeID]
		source := Source{Type: SourceDepartmentMap, Label: "部门检查项功能"}
		roleSource := Source{Type: SourceOrganizationRole, Label: "检查项角色默认权限"}
		if inspectionDefaults != nil && inspectionDefaults.CanManage && mapping.CanManage {
			result.Permissions = append(result.Permissions, allow(ActionInspectionManage, scope, source, roleSource))
		}
		if inspectionDefaults != nil && inspectionDefaults.CanEnter && mapping.CanEnter {
			result.Permissions = append(result.Permissions, allow(ActionInspectionEnter, scope, source, roleSource))
		}
		if inspectionDefaults != nil && inspectionDefaults.CanView && mapping.CanView {
			result.Permissions = append(result.Permissions, allow(ActionInspectionView, scope, source, roleSource))
		}
		if inspectionDefaults != nil && inspectionDefaults.CanReview && mapping.CanReview {
			result.Permissions = append(result.Permissions, allow(ActionInspectionReview, scope, source, roleSource))
		}
		if inspectionDefaults != nil && inspectionDefaults.CanExport && (profile.DepartmentCode == "STUDY" || profile.DepartmentCode == "SPORTS" || profile.DepartmentCode == "DORMITORY") && mapping.CanExport {
			result.Permissions = append(result.Permissions, allow(ActionInspectionExport, scope, source, roleSource))
		}
	}
	reportScope := profile.ReportScope
	result.CanReviseFinalizedReport = reportScope != nil && reportScope.Enabled && reportScope.CanCorrect && profile.ReportDefaults != nil && profile.ReportDefaults.CanCorrect
	if reportScope != nil && reportScope.Enabled && profile.ReportDefaults != nil {
		defaults := profile.ReportDefaults
		for _, entry := range []struct {
			action  string
			enabled bool
		}{
			{ActionReportForceGenerate, reportScope.CanForceGenerate && defaults.CanForceGenerate}, {ActionReportObligationManage, reportScope.CanObligationManage && defaults.CanObligationManage},
			{ActionReportFileOutput, reportScope.CanFileOutput && defaults.CanFileOutput},
			{ActionReportView, reportScope.CanView && defaults.CanView}, {ActionReportGenerate, reportScope.CanGenerate && defaults.CanGenerate},
			{ActionReportCorrect, reportScope.CanCorrect && defaults.CanCorrect}, {ActionArchiveView, reportScope.CanArchive && defaults.CanArchive},
			{ActionReportReview, reportScope.CanReview && defaults.CanReview}, {ActionReportReturn, reportScope.CanReturn && defaults.CanReturn},
			{ActionReportFinalize, reportScope.CanFinalize && defaults.CanFinalize}, {ActionReportPublish, reportScope.CanPublish && defaults.CanPublish},
			{ActionReportOutputSettingsManage, reportScope.CanOutputSettings && defaults.CanOutputSettings},
		} {
			if entry.enabled {
				result.Permissions = append(result.Permissions, allow(entry.action, Scope{}, Source{Type: SourceDepartmentMap, Label: "部门周通报功能"}, Source{Type: SourceOrganizationRole, Label: "周通报角色默认权限"}))
			}
		}
	}
	// The office starts with a read-only template. Its confirmed-source view
	// policy follows effective write actions, so administrators can later
	// assign inspection work without a code change.
	result.ConfirmedInspectionsOnly = profile.DepartmentCode == "OFFICE" &&
		!result.HasAnyAllowed(ActionInspectionEnter) && !result.HasAnyAllowed(ActionInspectionManage) && !result.HasAnyAllowed(ActionInspectionReview)
	result.CanReviseFinalizedReport = (result.CanReviseFinalizedReport || profile.SystemRole == SystemRoleAdmin) && result.HasAnyAllowed(ActionReportCorrect)
	return result
}

func (s *Service) ListRules(ctx context.Context, accountID string) ([]Rule, error) {
	rules, err := s.repository.ListRules(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if rules == nil {
		rules = make([]Rule, 0)
	}
	return rules, nil
}

func (s *Service) CreateRule(ctx context.Context, actorID string, rule Rule, metadata ...audit.Metadata) (Rule, error) {
	if rule.Ranges == nil {
		rule.Ranges = []OperationRange{}
	}
	rule.Action = strings.TrimSpace(rule.Action)
	if rule.Mode == "" {
		rule.Mode = "ADD"
	}
	if rule.StartsAt.IsZero() {
		rule.StartsAt = time.Now().UTC()
	}
	rule.Enabled = true
	if !validRule(rule) {
		return Rule{}, ErrInvalidRule
	}
	if err := s.validateForceGenerateTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportObligationTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportMaintenanceTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateRuleSource(ctx, rule); err != nil {
		return Rule{}, err
	}
	rule.ID = uuid.NewString()
	rule.Note = strings.TrimSpace(rule.Note)
	rule.SourceType = SourceUserRule
	rule.CreatedBy = actorID
	return s.repository.CreateRule(ctx, rule, permissionMutation(metadata, actorID, rule.ID, nil, rule))
}

func (s *Service) UpdateRule(ctx context.Context, rule Rule, metadata ...audit.Metadata) (Rule, error) {
	if rule.Ranges == nil {
		rule.Ranges = []OperationRange{}
	}
	rule.Action = strings.TrimSpace(rule.Action)
	if rule.Mode == "" {
		rule.Mode = "ADD"
	}
	if rule.StartsAt.IsZero() {
		rule.StartsAt = time.Now().UTC()
	}
	if rule.ID == "" || !validRule(rule) {
		return Rule{}, ErrInvalidRule
	}
	if err := s.validateForceGenerateTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportObligationTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportMaintenanceTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateRuleSource(ctx, rule); err != nil {
		return Rule{}, err
	}
	rule.Note = strings.TrimSpace(rule.Note)
	rules, err := s.repository.ListRules(ctx, rule.AccountID)
	if err != nil {
		return Rule{}, err
	}
	var before any
	for _, existing := range rules {
		if existing.ID == rule.ID {
			if widensLegacyRange(existing.Ranges, rule) {
				return Rule{}, ErrInvalidRule
			}
			before = existing
			break
		}
	}
	return s.repository.UpdateRule(ctx, rule, permissionMutation(metadata, "", rule.ID, before, rule))
}

func (s *Service) DeleteRule(ctx context.Context, accountID, ruleID string, metadata ...audit.Metadata) error {
	if accountID == "" || ruleID == "" {
		return ErrInvalidRule
	}
	rules, err := s.repository.ListRules(ctx, accountID)
	if err != nil {
		return err
	}
	var before any
	for _, existing := range rules {
		if existing.ID == ruleID {
			before = existing
			break
		}
	}
	return s.repository.DeleteRule(ctx, accountID, ruleID, permissionMutation(metadata, "", ruleID, before, nil))
}

func permissionMutation(metadata []audit.Metadata, actorID, resourceID string, before, after any) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	if request.ActorID == "" {
		request.ActorID = actorID
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: audit.ActionPermissionRuleChanged, ResourceType: "permissionRule", ResourceID: resourceID, Before: before, After: after,
	}}
}

func (s *Service) validateForceGenerateTarget(ctx context.Context, rule Rule) error {
	if rule.Action != ActionReportForceGenerate || rule.Effect != EffectAllow {
		return nil
	}
	profile, err := s.repository.LoadAccessProfile(ctx, rule.AccountID)
	if err != nil {
		return err
	}
	if !isForceGenerateEligible(profile) {
		return ErrForceGenerateRoleRequired
	}
	return nil
}

func (s *Service) validateReportObligationTarget(ctx context.Context, rule Rule) error {
	if rule.Action != ActionReportObligationManage || rule.Effect != EffectAllow {
		return nil
	}
	profile, err := s.repository.LoadAccessProfile(ctx, rule.AccountID)
	if err != nil {
		return err
	}
	if !isReportObligationManagerEligible(profile) {
		return ErrReportObligationRoleRequired
	}
	return nil
}

// Write grants never bypass the exclusive department qualification.
func isReportMaintainer(profile AccessProfile) bool {
	if profile.ReportScope != nil && profile.ReportScope.Enabled {
		return true
	}
	for _, department := range profile.Departments {
		if isReportMaintainer(department) {
			return true
		}
	}
	return false
}
func reportGrantEligible(profile AccessProfile, action string) bool {
	if profile.SystemRole == SystemRoleAdmin || profile.SystemRole == SystemRoleSuperAdmin {
		return action != ActionReportForceGenerate && action != ActionReportObligationManage || isReportMaintainer(profile)
	}
	return isReportMaintainer(profile)
}
func requiresReportMaintenance(action string) bool {
	switch action {
	case ActionReportGenerate, ActionReportFileOutput, ActionReportCorrect, ActionReportForceGenerate, ActionReportObligationManage, ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionReportOutputSettingsManage:
		return true
	default:
		return false
	}
}
func (s *Service) validateReportMaintenanceTarget(ctx context.Context, rule Rule) error {
	if rule.Effect != EffectAllow || !requiresReportMaintenance(rule.Action) {
		return nil
	}
	profile, err := s.repository.LoadAccessProfile(ctx, rule.AccountID)
	if err != nil {
		return err
	}
	if !reportGrantEligible(profile, rule.Action) {
		return ErrReportMaintenanceRequired
	}
	return nil
}
func isForceGenerateEligible(profile AccessProfile) bool { return isReportMaintainer(profile) }
func isReportObligationManagerEligible(profile AccessProfile) bool {
	return isReportMaintainer(profile)
}

func userRulePermission(rule Rule) EffectivePermission {
	label := "用户补充授权"
	if rule.Effect == EffectDeny {
		label = "用户单独限制"
	}
	return EffectivePermission{SourceDepartmentID: rule.SourceDepartmentID, Ranges: rule.Ranges, StartsAt: rule.StartsAt, EndsAt: rule.EndsAt, Action: rule.Action, Effect: rule.Effect, Scope: rule.Scope, Sources: []Source{{Type: SourceUserRule, Label: label}}}
}

func allow(action string, scope Scope, sources ...Source) EffectivePermission {
	return EffectivePermission{Action: action, Effect: EffectAllow, Scope: scope, Sources: sources}
}

func validRule(rule Rule) bool {
	if rule.AccountID == "" || (rule.Effect != EffectAllow && rule.Effect != EffectDeny) {
		return false
	}
	if !ValidOperationRanges(rule.Ranges) || hasRemovedScopeDimensions(rule.Scope) || rule.Scope.AcademicYearID != "" || rule.Scope.SemesterID != "" || rule.Scope.WeekID != "" || (len(rule.Ranges) > 0 && !SupportsOperationRange(rule.Action)) || (rule.Mode != "" && rule.Mode != "ADD" && rule.Mode != "OVERRIDE" && rule.Mode != "DISABLE") || (rule.EndsAt != nil && !rule.EndsAt.After(rule.StartsAt)) {
		return false
	}
	if rule.Action == ActionInspectionExport {
		return false
	}
	if rule.Action == ActionReportOutputSettingsManage && !isGlobalScope(rule.Scope) {
		return false
	}
	for _, action := range allActions {
		if action == strings.TrimSpace(rule.Action) {
			return true
		}
	}
	return false
}

func isGlobalScope(scope Scope) bool {
	return scope.AcademicYearID == "" && scope.SemesterID == "" && scope.WeekID == "" && scope.InspectionTypeID == "" && !hasRemovedScopeDimensions(scope)
}

func hasRemovedScopeDimensions(scope Scope) bool {
	return scope.DepartmentID != "" || scope.ClassID != "" || scope.Grade != ""
}

func scopeMatches(rule, requested Scope) bool {
	return matches(rule.AcademicYearID, requested.AcademicYearID) &&
		matches(rule.SemesterID, requested.SemesterID) &&
		matches(rule.WeekID, requested.WeekID) &&
		matches(rule.InspectionTypeID, requested.InspectionTypeID) &&
		matches(rule.DepartmentID, requested.DepartmentID) &&
		matches(rule.ClassID, requested.ClassID) && matches(rule.Grade, requested.Grade)
}

func matches(ruleValue, requestedValue string) bool {
	return ruleValue == "" || ruleValue == requestedValue
}

func departmentSupports(profile AccessProfile, action, inspectionType string) bool {
	if IsDisciplineAction(action) {
		return slices.Contains(profile.DisciplineScope, action)
	}
	if action == ActionMorningScheduleManage {
		return profile.CanManageMorningSchedule
	}
	if requiresReportMaintenance(action) || action == ActionReportView || action == ActionArchiveView {
		if profile.ReportScope == nil || !profile.ReportScope.Enabled {
			return false
		}
		scope := profile.ReportScope
		switch action {
		case ActionReportView:
			return scope.CanView
		case ActionReportGenerate:
			return scope.CanGenerate
		case ActionReportFileOutput:
			return scope.CanFileOutput
		case ActionReportCorrect:
			return scope.CanCorrect
		case ActionReportReview:
			return scope.CanReview
		case ActionReportReturn:
			return scope.CanReturn
		case ActionReportFinalize:
			return scope.CanFinalize
		case ActionReportPublish:
			return scope.CanPublish
		case ActionArchiveView:
			return scope.CanArchive
		case ActionReportForceGenerate:
			return scope.CanForceGenerate
		case ActionReportObligationManage:
			return scope.CanObligationManage
		case ActionReportOutputSettingsManage:
			return scope.CanOutputSettings
		}
		return false
	}
	for _, scope := range profile.DepartmentScopes {
		if scope.Enabled && scope.InspectionTypeID == inspectionType {
			switch action {
			case ActionInspectionEnter:
				return scope.CanEnter
			case ActionInspectionManage:
				return scope.CanManage
			case ActionInspectionReview:
				return scope.CanReview
			case ActionInspectionView:
				return scope.CanView
			case ActionInspectionExport:
				return scope.CanExport
			}
		}
	}
	return false
}

func requiresDepartmentSource(action string) bool {
	return IsDisciplineAction(action) || strings.HasPrefix(action, "INSPECTION_") || strings.HasPrefix(action, "REPORT_") || action == ActionArchiveView || action == ActionMorningScheduleManage
}
func (s *Service) validateRuleSource(ctx context.Context, rule Rule) error {
	profile, err := s.repository.LoadAccessProfile(ctx, rule.AccountID)
	if err != nil {
		return err
	}
	if rule.SourceDepartmentID == "" {
		if rule.Mode == "OVERRIDE" || rule.Mode == "DISABLE" {
			return ErrInvalidRule
		}
		if rule.Effect == EffectAllow && profile.SystemRole == SystemRoleMember && requiresDepartmentSource(rule.Action) {
			return ErrInvalidRule
		}
		return nil
	}
	for _, department := range profile.Departments {
		if department.DepartmentID == rule.SourceDepartmentID && departmentSupports(department, rule.Action, rule.Scope.InspectionTypeID) {
			if rule.Mode == "OVERRIDE" {
				for _, action := range department.OperationRanges {
					if action.Action == rule.Action && action.InspectionTypeID == rule.Scope.InspectionTypeID && widensLegacyRange(action.Ranges, rule) {
						return ErrInvalidRule
					}
				}
			}
			return nil
		}
	}
	return ErrInvalidRule
}

// Restrict the availability check to this inspection; an unrelated allow must not cancel its denial.
func hasAllowedInspectionWrite(result EffectiveResult, action, inspectionTypeID string) bool {
	filtered := EffectiveResult{}
	for _, entry := range result.Permissions {
		if entry.Action == action && (entry.Scope.InspectionTypeID == "" || entry.Scope.InspectionTypeID == inspectionTypeID) {
			filtered.Permissions = append(filtered.Permissions, entry)
		}
	}
	return filtered.HasAnyAllowed(action)
}
