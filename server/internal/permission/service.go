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

	ActionUserManage        = "USER_MANAGE"
	ActionPermissionManage  = "PERMISSION_MANAGE"
	ActionMasterDataManage  = "MASTER_DATA_MANAGE"
	ActionDictionaryManage  = "DICTIONARY_MANAGE"
	ActionConfigurationView = "CONFIGURATION_VIEW"
	ActionInspectionEnter   = "INSPECTION_ENTER"
	ActionInspectionView    = "INSPECTION_VIEW"
	ActionInspectionReview  = "INSPECTION_REVIEW"
	ActionInspectionExport  = "INSPECTION_EXPORT"
	ActionInspectionSummary = "INSPECTION_SUMMARIZE"
	ActionReportReview      = "REPORT_REVIEW"
	ActionReportReturn      = "REPORT_RETURN"
	ActionReportFinalize    = "REPORT_FINALIZE"
	ActionReportPublish     = "REPORT_PUBLISH"
	ActionArchiveView       = "ARCHIVE_VIEW"
)

var ErrInvalidRule = errors.New("invalid permission rule")

var allActions = []string{
	ActionUserManage, ActionPermissionManage, ActionMasterDataManage, ActionDictionaryManage, ActionConfigurationView,
	ActionInspectionEnter, ActionInspectionView, ActionInspectionReview, ActionInspectionExport, ActionInspectionSummary,
	ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish, ActionArchiveView,
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
	CanReview        bool
	CanExport        bool
	CanSummarize     bool
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
}

type EffectivePermission struct {
	Action  string   `json:"action"`
	Effect  string   `json:"effect"`
	Scope   Scope    `json:"scope"`
	Sources []Source `json:"sources"`
}

type EffectiveResult struct {
	AccountID   string                `json:"accountId"`
	SystemRole  string                `json:"systemRole"`
	Permissions []EffectivePermission `json:"permissions"`
}

func (r EffectiveResult) Allowed(action string, requested Scope) bool {
	allowed := false
	for _, permission := range r.Permissions {
		if permission.Action != action || !scopeMatches(permission.Scope, requested) {
			continue
		}
		if permission.Effect == EffectDeny {
			return false
		}
		if permission.Effect == EffectAllow {
			allowed = true
		}
	}
	return allowed
}

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
	result := EffectiveResult{AccountID: profile.AccountID, SystemRole: profile.SystemRole, Permissions: make([]EffectivePermission, 0)}
	if profile.SystemRole == SystemRoleSuperAdmin {
		for _, action := range allActions {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "超级管理员"}))
		}
		return result, nil
	}
	if profile.SystemRole == SystemRoleAdmin {
		for _, action := range []string{ActionConfigurationView, ActionMasterDataManage, ActionDictionaryManage} {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceSystemRole, Label: "系统管理员"}))
		}
	}
	leadership := profile.OrganizationRoleCode == "MINISTER" || profile.OrganizationRoleCode == "VICE_MINISTER"
	for _, mapping := range profile.DepartmentScopes {
		if !mapping.Enabled || mapping.DepartmentID != profile.DepartmentID {
			continue
		}
		scope := Scope{DepartmentID: mapping.DepartmentID, InspectionTypeID: mapping.InspectionTypeID}
		source := Source{Type: SourceDepartmentMap, Label: "部门与检查类型映射"}
		if mapping.CanEnter {
			result.Permissions = append(result.Permissions, allow(ActionInspectionEnter, scope, source))
		}
		if mapping.CanView {
			result.Permissions = append(result.Permissions, allow(ActionInspectionView, scope, source))
		}
		if leadership && mapping.CanReview {
			result.Permissions = append(result.Permissions, allow(ActionInspectionReview, scope, source))
		}
		if mapping.CanExport {
			result.Permissions = append(result.Permissions, allow(ActionInspectionExport, scope, source))
		}
		if leadership && mapping.CanSummarize {
			result.Permissions = append(result.Permissions, allow(ActionInspectionSummary, scope, source))
		}
	}
	if profile.DepartmentCode == "OFFICE" && leadership {
		for _, action := range []string{ActionReportReview, ActionReportReturn, ActionReportFinalize, ActionReportPublish} {
			result.Permissions = append(result.Permissions, allow(action, Scope{}, Source{Type: SourceOrganizationRole, Label: "学生会办公室部长/副部长"}))
		}
	}
	for _, rule := range profile.Rules {
		if !rule.Enabled {
			continue
		}
		label := "用户补充授权"
		if rule.Effect == EffectDeny {
			label = "用户单独限制"
		}
		result.Permissions = append(result.Permissions, EffectivePermission{Action: rule.Action, Effect: rule.Effect, Scope: rule.Scope, Sources: []Source{{Type: SourceUserRule, Label: label}}})
	}
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
	if !validRule(rule) {
		return Rule{}, ErrInvalidRule
	}
	rule.ID = uuid.NewString()
	rule.Action = strings.TrimSpace(rule.Action)
	rule.Note = strings.TrimSpace(rule.Note)
	rule.SourceType = SourceUserRule
	rule.CreatedBy = actorID
	rule.Enabled = true
	return s.repository.CreateRule(ctx, rule, permissionMutation(metadata, actorID, rule.ID, nil, rule))
}

func (s *Service) UpdateRule(ctx context.Context, rule Rule, metadata ...audit.Metadata) (Rule, error) {
	if rule.ID == "" || !validRule(rule) {
		return Rule{}, ErrInvalidRule
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

func allow(action string, scope Scope, source Source) EffectivePermission {
	return EffectivePermission{Action: action, Effect: EffectAllow, Scope: scope, Sources: []Source{source}}
}

func validRule(rule Rule) bool {
	if rule.AccountID == "" || (rule.Effect != EffectAllow && rule.Effect != EffectDeny) {
		return false
	}
	for _, action := range allActions {
		if action == strings.TrimSpace(rule.Action) {
			return true
		}
	}
	return false
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
