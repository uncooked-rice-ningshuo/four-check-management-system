package store

import (
	"context"
	"encoding/json"
	"time"

	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

// LoadAccessProfile reads assigned snapshots; current templates never rewrite existing grants.
func (p *Postgres) LoadAccessProfile(ctx context.Context, accountID string) (permission.AccessProfile, error) {
	return loadAccessProfile(ctx, p.Pool, accountID)
}

func loadAccessProfile(ctx context.Context, reader departmentDefaultsReader, accountID string) (permission.AccessProfile, error) {
	profile := permission.AccessProfile{PendingDepartmentIDs: []string{}}
	err := reader.QueryRow(ctx, `SELECT id,system_role,COALESCE(primary_department_id::text,'') FROM accounts WHERE id=$1`, accountID).Scan(&profile.AccountID, &profile.SystemRole, &profile.DepartmentID)
	if err != nil {
		return profile, err
	}
	rows, err := reader.Query(ctx, `SELECT d.id,d.code,r.code,u.start_at,u.end_at,u.template_version,u.default_permissions FROM user_org_roles u JOIN departments d ON d.id=u.department_id AND d.enabled JOIN organization_roles r ON r.id=u.organization_role_id AND r.enabled WHERE u.account_id=$1 ORDER BY d.id`, accountID)
	if err != nil {
		return profile, err
	}
	type membership struct {
		id, code, role string
		start          *time.Time
		end            *time.Time
		version        int
		snapshot       organization.DepartmentDefaultPermissions
	}
	members := []membership{}
	for rows.Next() {
		var m membership
		var raw []byte
		if err := rows.Scan(&m.id, &m.code, &m.role, &m.start, &m.end, &m.version, &raw); err != nil {
			rows.Close()
			return profile, err
		}
		if err := json.Unmarshal(raw, &m.snapshot); err != nil {
			rows.Close()
			return profile, err
		}
		if m.start == nil || m.start.IsZero() || m.snapshot.Version < 1 {
			profile.PendingDepartmentIDs = append(profile.PendingDepartmentIDs, m.id)
			continue
		}
		members = append(members, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return profile, err
	}
	for _, m := range members {
		current, err := getDepartmentDefaults(ctx, reader, m.id)
		if err != nil {
			return profile, err
		}
		department := permission.AccessProfile{AccountID: accountID, SystemRole: permission.SystemRoleMember, DepartmentID: m.id, DepartmentCode: m.code, OrganizationRoleCode: m.role, StartsAt: *m.start, EndsAt: m.end, TemplateVersion: m.version, CanManageMorningSchedule: current.CanManageMorningSchedule, InspectionDefaults: map[string]*permission.InspectionRoleDefaults{}}
		for _, scope := range current.Scopes {
			department.DepartmentScopes = append(department.DepartmentScopes, permission.DepartmentScope{DepartmentID: m.id, InspectionTypeID: scope.InspectionTypeID, CanEnter: scope.CanEnter, CanView: scope.CanView, CanManage: scope.CanManage, CanReview: scope.CanReview, CanExport: scope.CanExport, Enabled: scope.Enabled})
		}
		for _, role := range m.snapshot.InspectionRoles {
			if role.RoleCode == m.role {
				department.InspectionDefaults[role.InspectionTypeID] = &permission.InspectionRoleDefaults{CanView: role.CanView, CanEnter: role.CanEnter, CanManage: role.CanManage, CanReview: role.CanReview, CanExport: role.CanExport}
			}
		}
		live := current.ReportScope
		department.ReportScope = &permission.ReportScope{Enabled: live.Enabled, ReportRoleDefaults: reportDefaults(live.CanView, live.CanGenerate, live.CanFileOutput, live.CanCorrect, live.CanReview, live.CanReturn, live.CanFinalize, live.CanPublish, live.CanOutputSettings, live.CanArchive, live.CanForceGenerate, live.CanObligationManage)}
		for _, role := range m.snapshot.ReportRoles {
			if role.RoleCode == m.role {
				defaults := reportDefaults(role.CanView, role.CanGenerate, role.CanFileOutput, role.CanCorrect, role.CanReview, role.CanReturn, role.CanFinalize, role.CanPublish, role.CanOutputSettings, role.CanArchive, role.CanForceGenerate, role.CanObligationManage)
				department.ReportDefaults = &defaults
			}
		}
		for _, policy := range m.snapshot.RolePolicies {
			if policy.RoleCode == m.role {
				if current.DisciplineEnabled {
					department.DisciplineScope = current.DisciplineScope
				}
				department.DefaultDisciplineActions = policy.DisciplineActions
				department.DefaultMorningSchedule = policy.CanManageMorningSchedule
				department.ModuleValidity = policy.ModuleValidity
				department.OperationRanges = policy.OperationRanges
			}
		}

		department.AssignedDepartmentScopes = []permission.DepartmentScope{}
		for _, scope := range m.snapshot.Scopes {
			department.AssignedDepartmentScopes = append(department.AssignedDepartmentScopes, permission.DepartmentScope{DepartmentID: m.id, InspectionTypeID: scope.InspectionTypeID, CanEnter: scope.CanEnter, CanView: scope.CanView, CanManage: scope.CanManage, CanReview: scope.CanReview, CanExport: scope.CanExport, Enabled: scope.Enabled})
		}
		saved := m.snapshot.ReportScope
		department.AssignedReportScope = &permission.ReportScope{Enabled: saved.Enabled, ReportRoleDefaults: reportDefaults(saved.CanView, saved.CanGenerate, saved.CanFileOutput, saved.CanCorrect, saved.CanReview, saved.CanReturn, saved.CanFinalize, saved.CanPublish, saved.CanOutputSettings, saved.CanArchive, saved.CanForceGenerate, saved.CanObligationManage)}
		department.AssignedMorningSchedule = m.snapshot.CanManageMorningSchedule
		if err := reader.QueryRow(ctx, `SELECT id::text FROM inspection_types WHERE code='MORNING_ATTENDANCE'`).Scan(&department.MorningInspectionTypeID); err != nil {
			return profile, err
		}
		profile.Departments = append(profile.Departments, department)
		profile.OrganizationRoleCode = m.role
		if m.id == profile.DepartmentID {
			profile.DepartmentCode = m.code
		}
	}
	profile.Rules, err = listPermissionRules(ctx, reader, accountID)
	return profile, err
}

func reportDefaults(view, generate, fileOutput, correct, review, returnReport, finalize, publish, settings, archive, force, obligation bool) permission.ReportRoleDefaults {
	return permission.ReportRoleDefaults{CanView: view, CanGenerate: generate, CanFileOutput: fileOutput, CanCorrect: correct, CanReview: review, CanReturn: returnReport, CanFinalize: finalize, CanPublish: publish, CanOutputSettings: settings, CanArchive: archive, CanForceGenerate: force, CanObligationManage: obligation}
}
