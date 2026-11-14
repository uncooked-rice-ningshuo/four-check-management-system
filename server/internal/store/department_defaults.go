package store

import (
	"context"
	"errors"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/organization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (p *Postgres) GetDepartmentDefaults(ctx context.Context, departmentID string) (organization.DepartmentDefaultPermissions, error) {
	result := organization.DepartmentDefaultPermissions{DepartmentID: departmentID, Scopes: []organization.DepartmentInspectionScope{}, InspectionRoles: []organization.InspectionRoleDefaults{}, ReportRoles: []organization.ReportRoleDefaults{}}
	if err := p.Pool.QueryRow(ctx, `SELECT permission_version FROM departments WHERE id=$1::uuid`, departmentID).Scan(&result.Version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return result, organization.ErrInvalidInput
		}
		return result, err
	}
	result.ReportMaintenance.State = "unassigned"
	maintenanceErr := p.Pool.QueryRow(ctx, `SELECT d.id::text,d.name FROM department_report_scopes s JOIN departments d ON d.id=s.department_id WHERE s.enabled`).Scan(&result.ReportMaintenance.DepartmentID, &result.ReportMaintenance.DepartmentName)
	if maintenanceErr != nil && !errors.Is(maintenanceErr, pgx.ErrNoRows) {
		return result, maintenanceErr
	}
	if maintenanceErr == nil {
		result.ReportMaintenance.State = "assigned"
	}
	err := p.Pool.QueryRow(ctx, `SELECT enabled,can_view,can_generate,can_correct,can_review,can_return,can_finalize,
		can_publish,can_output_settings,can_archive FROM department_report_scopes WHERE department_id=$1::uuid`, departmentID).
		Scan(&result.ReportScope.Enabled, &result.ReportScope.CanView, &result.ReportScope.CanGenerate, &result.ReportScope.CanCorrect,
			&result.ReportScope.CanReview, &result.ReportScope.CanReturn, &result.ReportScope.CanFinalize, &result.ReportScope.CanPublish,
			&result.ReportScope.CanOutputSettings, &result.ReportScope.CanArchive)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT department_id,inspection_type_id,can_enter,can_view,can_manage,can_review,can_export,enabled
		FROM department_inspection_scopes WHERE department_id=$1::uuid ORDER BY inspection_type_id`, departmentID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var scope organization.DepartmentInspectionScope
		if err := rows.Scan(&scope.DepartmentID, &scope.InspectionTypeID, &scope.CanEnter, &scope.CanView, &scope.CanManage, &scope.CanReview, &scope.CanExport, &scope.Enabled); err != nil {
			rows.Close()
			return result, err
		}
		result.Scopes = append(result.Scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.Pool.Query(ctx, `SELECT inspection_type_id::text,role_code,can_view,can_enter,can_manage,can_review,can_export
		FROM department_inspection_role_defaults WHERE department_id=$1::uuid ORDER BY inspection_type_id,role_code`, departmentID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var role organization.InspectionRoleDefaults
		if err := rows.Scan(&role.InspectionTypeID, &role.RoleCode, &role.CanView, &role.CanEnter, &role.CanManage, &role.CanReview, &role.CanExport); err != nil {
			rows.Close()
			return result, err
		}
		result.InspectionRoles = append(result.InspectionRoles, role)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = p.Pool.Query(ctx, `SELECT role_code,can_view,can_generate,can_correct,can_review,can_return,can_finalize,
		can_publish,can_output_settings,can_archive FROM department_report_role_defaults WHERE department_id=$1::uuid ORDER BY role_code`, departmentID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var role organization.ReportRoleDefaults
		if err := rows.Scan(&role.RoleCode, &role.CanView, &role.CanGenerate, &role.CanCorrect, &role.CanReview, &role.CanReturn,
			&role.CanFinalize, &role.CanPublish, &role.CanOutputSettings, &role.CanArchive); err != nil {
			return result, err
		}
		result.ReportRoles = append(result.ReportRoles, role)
	}
	return result, rows.Err()
}

func (p *Postgres) SaveDepartmentDefaults(ctx context.Context, input organization.DepartmentDefaultPermissions, mutation audit.Mutation) (organization.DepartmentDefaultPermissions, error) {
	result := input
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `UPDATE departments SET permission_version=permission_version+1,updated_at=now()
			WHERE id=$1::uuid AND permission_version=$2 RETURNING permission_version`, input.DepartmentID, input.Version).Scan(&result.Version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return organization.ErrPermissionVersionConflict
			}
			return err
		}
		// The version update locks the department row, also used by department deactivation.
		var enabled, maintaining bool
		if err := tx.QueryRow(ctx, `SELECT d.enabled,COALESCE(s.enabled,false) FROM departments d LEFT JOIN department_report_scopes s ON s.department_id=d.id WHERE d.id=$1::uuid`, input.DepartmentID).Scan(&enabled, &maintaining); err != nil {
			return err
		}
		if maintaining && !input.ReportScope.Enabled {
			return organization.ErrReportMaintenanceRequired
		}
		if input.ReportScope.Enabled && !enabled {
			return organization.ErrInvalidInput
		}
		_, err := tx.Exec(ctx, `INSERT INTO department_report_scopes
			(department_id,enabled,can_view,can_generate,can_correct,can_review,can_return,can_finalize,can_publish,can_output_settings,can_archive)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT(department_id) DO UPDATE SET enabled=EXCLUDED.enabled,can_view=EXCLUDED.can_view,
			can_generate=EXCLUDED.can_generate,can_correct=EXCLUDED.can_correct,can_review=EXCLUDED.can_review,
			can_return=EXCLUDED.can_return,can_finalize=EXCLUDED.can_finalize,can_publish=EXCLUDED.can_publish,
			can_output_settings=EXCLUDED.can_output_settings,can_archive=EXCLUDED.can_archive`, input.DepartmentID,
			input.ReportScope.Enabled, input.ReportScope.CanView, input.ReportScope.CanGenerate, input.ReportScope.CanCorrect,
			input.ReportScope.CanReview, input.ReportScope.CanReturn, input.ReportScope.CanFinalize, input.ReportScope.CanPublish,
			input.ReportScope.CanOutputSettings, input.ReportScope.CanArchive)
		if err != nil {
			return err
		}
		for _, scope := range input.Scopes {
			_, err := tx.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,can_enter,can_view,can_manage,can_review,can_export,enabled)
				VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8)
				ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET can_enter=EXCLUDED.can_enter,can_view=EXCLUDED.can_view,
				can_manage=EXCLUDED.can_manage,can_review=EXCLUDED.can_review,can_export=EXCLUDED.can_export,enabled=EXCLUDED.enabled,updated_at=now()`,
				scope.DepartmentID, scope.InspectionTypeID, scope.CanEnter, scope.CanView, scope.CanManage, scope.CanReview, scope.CanExport, scope.Enabled)
			if err != nil {
				return err
			}
		}
		for _, role := range input.InspectionRoles {
			_, err := tx.Exec(ctx, `INSERT INTO department_inspection_role_defaults
					(department_id,inspection_type_id,role_code,can_view,can_enter,can_manage,can_review,can_export)
					VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8)
					ON CONFLICT(department_id,inspection_type_id,role_code) DO UPDATE SET can_view=EXCLUDED.can_view,can_enter=EXCLUDED.can_enter,
				can_manage=EXCLUDED.can_manage,can_review=EXCLUDED.can_review,can_export=EXCLUDED.can_export`,
				input.DepartmentID, role.InspectionTypeID, role.RoleCode, role.CanView, role.CanEnter, role.CanManage, role.CanReview, role.CanExport)
			if err != nil {
				return err
			}
		}
		for _, role := range input.ReportRoles {
			_, err := tx.Exec(ctx, `INSERT INTO department_report_role_defaults
				(department_id,role_code,can_view,can_generate,can_correct,can_review,can_return,can_finalize,can_publish,can_output_settings,can_archive)
				VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
				ON CONFLICT(department_id,role_code) DO UPDATE SET can_view=EXCLUDED.can_view,can_generate=EXCLUDED.can_generate,
				can_correct=EXCLUDED.can_correct,can_review=EXCLUDED.can_review,can_return=EXCLUDED.can_return,
				can_finalize=EXCLUDED.can_finalize,can_publish=EXCLUDED.can_publish,can_output_settings=EXCLUDED.can_output_settings,
				can_archive=EXCLUDED.can_archive`, input.DepartmentID, role.RoleCode, role.CanView, role.CanGenerate, role.CanCorrect,
				role.CanReview, role.CanReturn, role.CanFinalize, role.CanPublish, role.CanOutputSettings, role.CanArchive)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE accounts SET session_version=session_version+1,updated_at=now() WHERE primary_department_id=$1::uuid`, input.DepartmentID); err != nil {
			return err
		}
		result.ReportMaintenance = organization.ReportMaintenance{State: "unassigned"}
		ownerErr := tx.QueryRow(ctx, `SELECT d.id::text,d.name FROM department_report_scopes s JOIN departments d ON d.id=s.department_id WHERE s.enabled`).Scan(&result.ReportMaintenance.DepartmentID, &result.ReportMaintenance.DepartmentName)
		if ownerErr != nil && !errors.Is(ownerErr, pgx.ErrNoRows) {
			return ownerErr
		}
		if ownerErr == nil {
			result.ReportMaintenance.State = "assigned"
		}
		_, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now())
			WHERE account_id IN (SELECT id FROM accounts WHERE primary_department_id=$1::uuid) AND revoked_at IS NULL`, input.DepartmentID)
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "department_report_scopes_one_enabled" {
			return result, organization.ErrReportMaintenanceAssigned
		}
		return result, err
	}
	return result, nil
}
