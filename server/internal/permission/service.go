package permission

import (
	"context"
	"errors"
	"strings"

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
var ErrForceGenerateRoleRequired = errors.New("force generation permission requires student union office leadership")
var ErrReportObligationRoleRequired = errors.New("weekly obligation permission requires student union office leadership")
var ErrReportLeadershipRequired = errors.New("report approval and output settings require department leadership")

var allActions = []string{
	ActionUserManage, ActionPermissionManage, ActionMasterDataManage, ActionDictionaryManage, ActionConfigurationView,
	ActionInspectionEnter, ActionInspectionManage, ActionInspectionView, ActionInspectionReview, ActionInspectionExport, ActionMorningScheduleManage,
	ActionReportView, ActionReportGenerate, ActionReportCorrect, ActionReportForceGenerate, ActionReportObligationManage, ActionReportOutputSettingsManage, ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionArchiveView,
}

type Scope struct {
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
	CanView, CanGenerate, CanCorrect, CanReview, CanReturn, CanFinalize bool
	CanPublish, CanOutputSettings, CanArchive                           bool
}

type ReportScope struct {
	Enabled bool
	ReportRoleDefaults
}

type EffectivePermission struct {
	Action  string   `json:"action"`
	Effect  string   `json:"effect"`
	Scope   Scope    `json:"scope"`
	Sources []Source `json:"sources"`
}

type EffectiveResult struct {
	PrimaryDepartmentID      string                `json:"-"`
	DepartmentCode           string                `json:"-"`
	OrganizationRoleCode     string                `json:"-"`
	CanReviseFinalizedReport bool                  `json:"canReviseFinalizedReport"`
	ConfirmedInspectionsOnly bool                  `json:"confirmedInspectionsOnly,omitempty"`
	AccountID                string                `json:"accountId"`
	SystemRole               string                `json:"systemRole"`
	Permissions              []EffectivePermission `json:"permissions"`
}

func (r EffectiveResult) Allowed(action string, requested Scope) bool {
	allowed := false
	for _, permission := range r.Permissions {
		if permission.Action != action {
			continue
		}
		if permission.Effect == EffectDeny && scopesOverlap(permission.Scope, requested) {
			return false
		}
		if permission.Effect == EffectAllow && scopeMatches(permission.Scope, requested) {
			allowed = true
		}
	}
	return allowed
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
			if denied.Action == action && denied.Effect == EffectDeny && scopeMatches(denied.Scope, allowed.Scope) {
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
	allowed := false
	for _, rule := range permissions {
		if rule.Action != action {
			continue
		}
		if rule.Effect == EffectDeny && scopesOverlap(rule.Scope, requested) {
			return false
		}
		if rule.Effect == EffectAllow && rule.Scope.ClassID == "" && rule.Scope.Grade == "" && scopeMatches(rule.Scope, requested) {
			allowed = true
		}
	}
	return allowed
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
	result := EffectiveResult{PrimaryDepartmentID: profile.DepartmentID, DepartmentCode: profile.DepartmentCode, OrganizationRoleCode: profile.OrganizationRoleCode, AccountID: profile.AccountID, SystemRole: profile.SystemRole, Permissions: make([]EffectivePermission, 0)}
	if profile.SystemRole == SystemRoleSuperAdmin {
		result.CanReviseFinalizedReport = true
		for _, action := range allActions {
			if action == ActionReportForceGenerate || action == ActionReportObligationManage {
				continue
			}
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "超级管理员"}))
		}
		if isForceGenerateEligible(profile) {
			for _, rule := range profile.Rules {
				if rule.Enabled && rule.Action == ActionReportForceGenerate {
					result.Permissions = append(result.Permissions, userRulePermission(rule))
				}
			}
		}
		if isReportObligationManagerEligible(profile) {
			for _, rule := range profile.Rules {
				if rule.Enabled && rule.Action == ActionReportObligationManage {
					result.Permissions = append(result.Permissions, userRulePermission(rule))
				}
			}
		}
		return result, nil
	}
	if profile.SystemRole == SystemRoleAdmin {
		for _, action := range []string{ActionConfigurationView, ActionMasterDataManage, ActionDictionaryManage} {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "系统管理员"}))
		}
	}
	leadership := profile.OrganizationRoleCode == "MINISTER" || profile.OrganizationRoleCode == "VICE_MINISTER"
	if profile.DepartmentCode == "SPORTS" && leadership {
		result.Permissions = append(result.Permissions, allow(ActionMorningScheduleManage, Scope{}, Source{Type: SourceOrganizationRole, Label: "文体部部长/副部长早签计划管理"}))
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
		roleSource := Source{Type: SourceOrganizationRole, Label: "检查项岗位默认权限"}
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
		if leadership && inspectionDefaults != nil && inspectionDefaults.CanExport && (profile.DepartmentCode == "STUDY" || profile.DepartmentCode == "SPORTS" || profile.DepartmentCode == "DORMITORY") && mapping.CanExport {
			result.Permissions = append(result.Permissions, allow(ActionInspectionExport, scope, source, roleSource))
		}
	}
	reportScope := profile.ReportScope
	result.CanReviseFinalizedReport = leadership && reportScope != nil && reportScope.Enabled && reportScope.CanCorrect && profile.ReportDefaults != nil && profile.ReportDefaults.CanCorrect
	if reportScope != nil && reportScope.Enabled && profile.ReportDefaults != nil {
		defaults := profile.ReportDefaults
		for _, entry := range []struct {
			action  string
			enabled bool
		}{
			{ActionReportView, reportScope.CanView && defaults.CanView}, {ActionReportGenerate, reportScope.CanGenerate && defaults.CanGenerate},
			{ActionReportCorrect, reportScope.CanCorrect && defaults.CanCorrect}, {ActionArchiveView, reportScope.CanArchive && defaults.CanArchive},
			{ActionReportReview, reportScope.CanReview && defaults.CanReview && leadership}, {ActionReportReturn, reportScope.CanReturn && defaults.CanReturn && leadership},
			{ActionReportFinalize, reportScope.CanFinalize && defaults.CanFinalize && leadership}, {ActionReportPublish, reportScope.CanPublish && defaults.CanPublish && leadership},
			{ActionReportOutputSettingsManage, reportScope.CanOutputSettings && defaults.CanOutputSettings && leadership},
		} {
			if entry.enabled {
				result.Permissions = append(result.Permissions, allow(entry.action, Scope{}, Source{Type: SourceDepartmentMap, Label: "部门周通报功能"}, Source{Type: SourceOrganizationRole, Label: "周通报岗位默认权限"}))
			}
		}
	}
	for _, rule := range profile.Rules {
		if !rule.Enabled {
			continue
		}
		if requiresReportLeadership(rule.Action) && !isReportLeadership(profile) {
			continue
		}
		if rule.Action == ActionMorningScheduleManage && !(profile.DepartmentCode == "SPORTS" && leadership) {
			continue
		}
		if rule.Effect == EffectDeny && hasRemovedScopeDimensions(rule.Scope) {
			// A legacy scoped denial cannot safely become a global denial. The
			// migration audits and disables it; ignore it during rollout too.
			continue
		}
		if rule.Action == ActionReportOutputSettingsManage && !isGlobalScope(rule.Scope) {
			continue
		}
		rule.Scope.DepartmentID = ""
		rule.Scope.ClassID = ""
		rule.Scope.Grade = ""
		if rule.Action == ActionReportForceGenerate && !isForceGenerateEligible(profile) {
			continue
		}
		if rule.Action == ActionReportObligationManage && !isReportObligationManagerEligible(profile) {
			continue
		}
		result.Permissions = append(result.Permissions, userRulePermission(rule))
	}
	// The office starts with a read-only template. Its confirmed-source view
	// policy follows effective write actions, so administrators can later
	// assign inspection work without a code change.
	result.ConfirmedInspectionsOnly = profile.DepartmentCode == "OFFICE" &&
		!result.HasAnyAllowed(ActionInspectionEnter) && !result.HasAnyAllowed(ActionInspectionManage) && !result.HasAnyAllowed(ActionInspectionReview)
	result.CanReviseFinalizedReport = result.CanReviseFinalizedReport && result.HasAnyAllowed(ActionReportCorrect)
	return result, nil
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
	rule.Action = strings.TrimSpace(rule.Action)
	if !validRule(rule) {
		return Rule{}, ErrInvalidRule
	}
	if err := s.validateForceGenerateTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportObligationTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportLeadershipTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	rule.ID = uuid.NewString()
	rule.Note = strings.TrimSpace(rule.Note)
	rule.SourceType = SourceUserRule
	rule.CreatedBy = actorID
	rule.Enabled = true
	return s.repository.CreateRule(ctx, rule, permissionMutation(metadata, actorID, rule.ID, nil, rule))
}

func (s *Service) UpdateRule(ctx context.Context, rule Rule, metadata ...audit.Metadata) (Rule, error) {
	rule.Action = strings.TrimSpace(rule.Action)
	if rule.ID == "" || !validRule(rule) {
		return Rule{}, ErrInvalidRule
	}
	if err := s.validateForceGenerateTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportObligationTarget(ctx, rule); err != nil {
		return Rule{}, err
	}
	if err := s.validateReportLeadershipTarget(ctx, rule); err != nil {
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

func requiresReportLeadership(action string) bool {
	switch action {
	case ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionReportOutputSettingsManage:
		return true
	default:
		return false
	}
}

func isReportLeadership(profile AccessProfile) bool {
	return profile.SystemRole == SystemRoleSuperAdmin ||
		(profile.ReportDefaults != nil && (profile.OrganizationRoleCode == "MINISTER" || profile.OrganizationRoleCode == "VICE_MINISTER"))
}

func (s *Service) validateReportLeadershipTarget(ctx context.Context, rule Rule) error {
	if rule.Effect != EffectAllow || !requiresReportLeadership(rule.Action) {
		return nil
	}
	profile, err := s.repository.LoadAccessProfile(ctx, rule.AccountID)
	if err != nil {
		return err
	}
	if !isReportLeadership(profile) {
		return ErrReportLeadershipRequired
	}
	return nil
}

func isForceGenerateEligible(profile AccessProfile) bool {
	return profile.DepartmentCode == "OFFICE" &&
		(profile.OrganizationRoleCode == "MINISTER" || profile.OrganizationRoleCode == "VICE_MINISTER")
}

func isReportObligationManagerEligible(profile AccessProfile) bool {
	return profile.DepartmentCode == "OFFICE" &&
		(profile.OrganizationRoleCode == "MINISTER" || profile.OrganizationRoleCode == "VICE_MINISTER")
}

func userRulePermission(rule Rule) EffectivePermission {
	label := "用户补充授权"
	if rule.Effect == EffectDeny {
		label = "用户单独限制"
	}
	return EffectivePermission{Action: rule.Action, Effect: rule.Effect, Scope: rule.Scope, Sources: []Source{{Type: SourceUserRule, Label: label}}}
}

func allow(action string, scope Scope, sources ...Source) EffectivePermission {
	return EffectivePermission{Action: action, Effect: EffectAllow, Scope: scope, Sources: sources}
}

func validRule(rule Rule) bool {
	if rule.AccountID == "" || (rule.Effect != EffectAllow && rule.Effect != EffectDeny) {
		return false
	}
	if hasRemovedScopeDimensions(rule.Scope) {
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
