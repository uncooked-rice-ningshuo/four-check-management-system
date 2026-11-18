package store

import (
	"context"
	"four-check-management-system/server/internal/permission"
)

func (p *Postgres) LegacyStages(ctx context.Context, page, size int) ([]permission.LegacyStageItem, int, error) {
	var total int
	if err := p.Pool.QueryRow(ctx, "SELECT count(*) FROM legacy_stage_grant_inventory").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT i.source,COALESCE(i.account_id::text,''),COALESCE(a.name,''),COALESCE(d.name,''),i.action,COALESCE(t.name,''),(i.range_clause->>'gradeStage')::int, COALESCE(role.name,''), COALESCE(m.name,''), COALESCE(c.name,'')
 FROM legacy_stage_grant_inventory i LEFT JOIN accounts a ON a.id=i.account_id LEFT JOIN departments d ON d.id=i.department_id LEFT JOIN inspection_types t ON t.id=i.inspection_type_id
 LEFT JOIN organization_roles role ON role.code=i.role_code LEFT JOIN majors m ON m.id=NULLIF(i.range_clause->>'majorId','')::uuid LEFT JOIN classes c ON c.id=NULLIF(i.range_clause->>'classId','')::uuid
 ORDER BY i.source,i.source_id,i.action,i.inspection_type_id,i.range_clause LIMIT $1 OFFSET $2`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []permission.LegacyStageItem{}
	for rows.Next() {
		var item permission.LegacyStageItem
		if err := rows.Scan(&item.Source, &item.AccountID, &item.AccountName, &item.DepartmentName, &item.Action, &item.TypeName, &item.Stage, &item.RoleName, &item.MajorName, &item.ClassName); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
