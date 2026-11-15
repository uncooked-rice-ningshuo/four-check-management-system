package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/organization"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) AssignUserRole(ctx context.Context, value organization.UserAssignment, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		var roleCode string
		if err := tx.QueryRow(ctx, `SELECT code FROM organization_roles WHERE id=$1 AND enabled FOR SHARE`, value.OrganizationRoleID).Scan(&roleCode); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return organization.ErrInvalidInput
			}
			return err
		}
		if len(value.DepartmentIDs) == 0 || (len(value.DepartmentIDs) > 1 && roleCode != "TEACHER" && roleCode != "PRESIDIUM") || !slices.Contains(value.DepartmentIDs, value.DepartmentID) {
			return organization.ErrInvalidInput
		}
		seen := map[string]bool{}
		for _, id := range value.DepartmentIDs {
			if uuid.Validate(id) != nil || seen[id] {
				return organization.ErrInvalidInput
			}
			seen[id] = true
			var enabled bool
			if err := tx.QueryRow(ctx, `SELECT enabled FROM departments WHERE id=$1 FOR SHARE`, id).Scan(&enabled); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return organization.ErrInvalidInput
				}
				return err
			}
			if !enabled {
				return organization.ErrInvalidInput
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE accounts SET primary_department_id=$2,session_version=session_version+1,updated_at=now() WHERE id=$1 AND session_version=$3`, value.AccountID, value.DepartmentID, value.SessionVersion)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrVersionConflict
		}
		var previousRole string
		err = tx.QueryRow(ctx, `SELECT organization_role_id::text FROM user_org_roles WHERE account_id=$1 LIMIT 1`, value.AccountID).Scan(&previousRole)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if previousRole != value.OrganizationRoleID {
			rows, err := tx.Query(ctx, `SELECT id::text FROM user_permission_rules WHERE account_id=$1 FOR UPDATE`, value.AccountID)
			if err != nil {
				return err
			}
			ids := []string{}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(ids) != len(value.PersonalRuleDecisions) {
				return organization.ErrInvalidInput
			}
			for _, id := range ids {
				keep, ok := value.PersonalRuleDecisions[id]
				if !ok {
					return organization.ErrInvalidInput
				}
				if !keep {
					if _, err := tx.Exec(ctx, `DELETE FROM user_permission_rules WHERE id=$1 AND account_id=$2`, id, value.AccountID); err != nil {
						return err
					}
				}
			}
		} else if len(value.PersonalRuleDecisions) != 0 {
			return organization.ErrInvalidInput
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_org_roles WHERE account_id=$1 AND (organization_role_id<>$2::uuid OR NOT (department_id::text=ANY($3::text[])))`, value.AccountID, value.OrganizationRoleID, value.DepartmentIDs); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, id := range value.DepartmentIDs {
			if err := assignDepartmentSnapshot(ctx, tx, value.AccountID, id, value.OrganizationRoleID, roleCode, now); err != nil {
				return err
			}
		}
		return revokeAccountSessionsTx(ctx, tx, value.AccountID)
	})
}

// Existing memberships retain their original template and dates on ordinary saves.
func assignDepartmentSnapshot(ctx context.Context, tx pgx.Tx, accountID, departmentID, roleID, roleCode string, now time.Time) error {
	var exists, complete bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_org_roles WHERE account_id=$1 AND department_id=$2 AND organization_role_id=$3), EXISTS(SELECT 1 FROM user_org_roles WHERE account_id=$1 AND department_id=$2 AND organization_role_id=$3 AND start_at IS NOT NULL AND default_permissions ? 'version')`, accountID, departmentID, roleID).Scan(&exists, &complete); err != nil {
		return err
	}
	if exists && complete {
		return nil
	}
	defaults, err := getDepartmentDefaults(ctx, tx, departmentID)
	if err != nil {
		return err
	}
	var end *time.Time
	found := false
	for _, policy := range defaults.RolePolicies {
		if policy.RoleCode == roleCode {
			found = true
			end = policy.EndsAt
			if end == nil && policy.ValidityYears > 0 {
				date := now.AddDate(policy.ValidityYears, 0, 0)
				end = &date
			}
			break
		}
	}
	if !found || (end != nil && !end.After(now)) {
		return organization.ErrInvalidInput
	}
	snapshot, err := json.Marshal(defaults)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id,start_at,end_at,template_version,default_permissions) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (account_id,department_id,organization_role_id) DO UPDATE SET start_at=EXCLUDED.start_at,end_at=EXCLUDED.end_at,template_version=EXCLUDED.template_version,default_permissions=EXCLUDED.default_permissions`, uuid.NewString(), accountID, departmentID, roleID, now, end, defaults.Version, snapshot)
	return err
}
