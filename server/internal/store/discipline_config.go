package store

import (
	"context"
	"errors"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/disciplineconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) ListInspectionTypes(ctx context.Context) ([]disciplineconfig.InspectionType, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id::text,code,name,enabled FROM inspection_types ORDER BY sort_order,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []disciplineconfig.InspectionType{}
	for rows.Next() {
		var item disciplineconfig.InspectionType
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Enabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) ListDisciplineTierDefinitions(ctx context.Context) ([]disciplineconfig.TierDefinition, error) {
	rows, err := p.Pool.Query(ctx, `SELECT i.value,i.label,i.sort_order FROM dictionary_items i JOIN dictionary_types t ON t.id=i.type_id WHERE t.code='DISCIPLINE_LEVEL' AND t.enabled AND i.enabled ORDER BY i.sort_order,i.value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []disciplineconfig.TierDefinition{}
	for rows.Next() {
		var item disciplineconfig.TierDefinition
		if err := rows.Scan(&item.Code, &item.Name, &item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) Current(ctx context.Context) (disciplineconfig.Version, bool, error) {
	var id string
	err := p.Pool.QueryRow(ctx, `SELECT version_id::text FROM discipline_rule_current WHERE singleton=true`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return disciplineconfig.Version{}, false, nil
	}
	if err != nil {
		return disciplineconfig.Version{}, false, err
	}
	v, err := p.GetVersion(ctx, id)
	return v, err == nil, err
}
func (p *Postgres) ListVersions(ctx context.Context) ([]disciplineconfig.Version, error) {
	rows, err := p.Pool.Query(ctx, `SELECT v.id::text,v.version,v.status,v.row_version,v.created_by::text,v.created_at,creator.name,v.published_by::text,v.published_at,publisher.name FROM discipline_rule_versions v JOIN accounts creator ON creator.id=v.created_by LEFT JOIN accounts publisher ON publisher.id=v.published_by ORDER BY v.version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []disciplineconfig.Version{}
	for rows.Next() {
		v, err := scanDisciplineVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Groups, err = p.readDisciplineGroups(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (p *Postgres) GetVersion(ctx context.Context, id string) (disciplineconfig.Version, error) {
	row := p.Pool.QueryRow(ctx, `SELECT v.id::text,v.version,v.status,v.row_version,v.created_by::text,v.created_at,creator.name,v.published_by::text,v.published_at,publisher.name FROM discipline_rule_versions v JOIN accounts creator ON creator.id=v.created_by LEFT JOIN accounts publisher ON publisher.id=v.published_by WHERE v.id=$1`, id)
	v, err := scanDisciplineVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, disciplineconfig.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.Groups, err = p.readDisciplineGroups(ctx, id)
	return v, err
}

type rowScanner interface{ Scan(...any) error }

func scanDisciplineVersion(row rowScanner) (disciplineconfig.Version, error) {
	var v disciplineconfig.Version
	var created time.Time
	var publisher *string
	var published *time.Time
	var publisherName *string
	err := row.Scan(&v.ID, &v.Version, &v.Status, &v.RowVersion, &v.CreatedBy, &created, &v.CreatedByName, &publisher, &published, &publisherName)
	if err != nil {
		return v, err
	}
	v.CreatedAt = created.UTC().Format(time.RFC3339)
	if publisher != nil {
		v.PublishedBy = *publisher
	}
	if publisherName != nil {
		v.PublishedByName = *publisherName
	}
	if published != nil {
		v.PublishedAt = published.UTC().Format(time.RFC3339)
	}
	return v, nil
}
func (p *Postgres) readDisciplineGroups(ctx context.Context, versionID string) ([]disciplineconfig.Group, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id::text,group_key,name,result_unit,violation_name FROM discipline_rule_groups WHERE version_id=$1 ORDER BY sort_order,id`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []disciplineconfig.Group{}
	ids := []string{}
	for rows.Next() {
		var g disciplineconfig.Group
		var id string
		if err := rows.Scan(&id, &g.Key, &g.Name, &g.Unit, &g.ViolationName); err != nil {
			return nil, err
		}
		g.Terms = []disciplineconfig.Term{}
		g.Tiers = []disciplineconfig.Tier{}
		groups = append(groups, g)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, id := range ids {
		tr, err := p.Pool.Query(ctx, `SELECT inspection_type_id::text,inspection_type_name,source_unit,operator,conversion_factor FROM discipline_rule_group_terms WHERE group_id=$1 ORDER BY sort_order,id`, id)
		if err != nil {
			return nil, err
		}
		for tr.Next() {
			var t disciplineconfig.Term
			if err := tr.Scan(&t.InspectionTypeID, &t.InspectionTypeName, &t.SourceUnit, &t.Operator, &t.Factor); err != nil {
				tr.Close()
				return nil, err
			}
			groups[i].Terms = append(groups[i].Terms, t)
		}
		if err := tr.Err(); err != nil {
			tr.Close()
			return nil, err
		}
		tr.Close()
		r, err := p.Pool.Query(ctx, `SELECT code,name,lower_bound,upper_bound,teacher_confirmation_required,authority_number,clause_reference FROM discipline_rule_tiers WHERE group_id=$1 ORDER BY sort_order,id`, id)
		if err != nil {
			return nil, err
		}
		for r.Next() {
			var t disciplineconfig.Tier
			if err := r.Scan(&t.Code, &t.Name, &t.Lower, &t.Upper, &t.TeacherConfirmationRequired, &t.AuthorityNumber, &t.ClauseReference); err != nil {
				r.Close()
				return nil, err
			}
			groups[i].Tiers = append(groups[i].Tiers, t)
		}
		if err := r.Err(); err != nil {
			r.Close()
			return nil, err
		}
		r.Close()
	}
	return groups, nil
}
func (p *Postgres) CreateDraft(ctx context.Context, actor string) (disciplineconfig.Version, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return disciplineconfig.Version{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM discipline_rule_versions WHERE status='DRAFT')`).Scan(&exists); err != nil {
		return disciplineconfig.Version{}, err
	}
	if exists {
		return disciplineconfig.Version{}, disciplineconfig.ErrConflict
	}
	var base disciplineconfig.Version
	var has bool
	var current string
	err = tx.QueryRow(ctx, `SELECT version_id::text FROM discipline_rule_current WHERE singleton=true FOR UPDATE`).Scan(&current)
	if err == nil {
		base, err = p.GetVersion(ctx, current)
		if err != nil {
			return base, err
		}
		has = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return base, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM discipline_rule_versions`).Scan(&version); err != nil {
		return base, err
	}
	id := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO discipline_rule_versions(id,version,status,created_by) VALUES($1,$2,'DRAFT',$3)`, id, version, actor)
	if err != nil {
		return base, err
	}
	if has {
		for groupIndex, g := range base.Groups {
			gid := uuid.NewString()
			_, err = tx.Exec(ctx, `INSERT INTO discipline_rule_groups(id,version_id,group_key,name,result_unit,sort_order,violation_name) VALUES($1,$2,$3,$4,$5,$6,$7)`, gid, id, g.Key, g.Name, g.Unit, groupIndex, g.ViolationName)
			if err != nil {
				return base, err
			}
			for i, t := range g.Terms {
				_, err = tx.Exec(ctx, `INSERT INTO discipline_rule_group_terms(group_id,version_id,inspection_type_id,inspection_type_name,source_unit,operator,conversion_factor,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, gid, id, t.InspectionTypeID, t.InspectionTypeName, t.SourceUnit, t.Operator, t.Factor, i)
				if err != nil {
					return base, err
				}
			}
			for i, t := range g.Tiers {
				_, err = tx.Exec(ctx, `INSERT INTO discipline_rule_tiers(group_id,code,name,lower_bound,upper_bound,teacher_confirmation_required,sort_order,authority_number,clause_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, gid, t.Code, t.Name, t.Lower, t.Upper, t.TeacherConfirmationRequired, i, t.AuthorityNumber, t.ClauseReference)
				if err != nil {
					return base, err
				}
			}
		}
	}
	if err := insertAuditTx(ctx, tx, audit.Metadata{ActorID: actor}, audit.Entry{Action: audit.ActionDisciplineRuleChanged, ResourceType: "disciplineRuleVersion", ResourceID: id, After: map[string]any{"version": version, "status": disciplineconfig.StatusDraft}}); err != nil {
		return base, err
	}
	if err := tx.Commit(ctx); err != nil {
		return base, err
	}
	return p.GetVersion(ctx, id)
}

func (p *Postgres) SaveDraft(ctx context.Context, v disciplineconfig.Version, expected int, actor string) (disciplineconfig.Version, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	var status string
	var current int
	err = tx.QueryRow(ctx, `SELECT status,row_version FROM discipline_rule_versions WHERE id=$1 FOR UPDATE`, v.ID).Scan(&status, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, disciplineconfig.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if status != "DRAFT" {
		return v, disciplineconfig.ErrImmutable
	}
	if current != expected {
		return v, disciplineconfig.ErrConflict
	}
	_, err = tx.Exec(ctx, `DELETE FROM discipline_rule_groups WHERE version_id=$1`, v.ID)
	if err != nil {
		return v, err
	}
	for gi, g := range v.Groups {
		gid := uuid.NewString()
		if _, err = tx.Exec(ctx, `INSERT INTO discipline_rule_groups(id,version_id,group_key,name,result_unit,sort_order,violation_name) VALUES($1,$2,$3,$4,$5,$6,$7)`, gid, v.ID, g.Key, g.Name, g.Unit, gi, g.ViolationName); err != nil {
			return v, err
		}
		for i, t := range g.Terms {
			if _, err = tx.Exec(ctx, `INSERT INTO discipline_rule_group_terms(group_id,version_id,inspection_type_id,inspection_type_name,source_unit,operator,conversion_factor,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, gid, v.ID, t.InspectionTypeID, t.InspectionTypeName, t.SourceUnit, t.Operator, t.Factor, i); err != nil {
				return v, err
			}
		}
		for i, t := range g.Tiers {
			if _, err = tx.Exec(ctx, `INSERT INTO discipline_rule_tiers(group_id,code,name,lower_bound,upper_bound,teacher_confirmation_required,sort_order,authority_number,clause_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, gid, t.Code, t.Name, t.Lower, t.Upper, t.TeacherConfirmationRequired, i, t.AuthorityNumber, t.ClauseReference); err != nil {
				return v, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE discipline_rule_versions SET row_version=row_version+1 WHERE id=$1 AND row_version=$2`, v.ID, expected); err != nil {
		return v, err
	}
	before, _ := p.GetVersion(ctx, v.ID)
	if err := insertAuditTx(ctx, tx, audit.Metadata{ActorID: actor}, audit.Entry{Action: audit.ActionDisciplineRuleChanged, ResourceType: "disciplineRuleVersion", ResourceID: v.ID, Before: map[string]any{"rowVersion": expected, "groups": before.Groups}, After: map[string]any{"rowVersion": expected + 1, "groups": v.Groups}}); err != nil {
		return v, err
	}
	if err = tx.Commit(ctx); err != nil {
		return v, err
	}
	return p.GetVersion(ctx, v.ID)
}

func (p *Postgres) Publish(ctx context.Context, id string, expected int, actor string) (disciplineconfig.Version, error) {
	var current disciplineconfig.Version
	v, err := p.GetVersion(ctx, id)
	if err != nil {
		return current, err
	}
	current = v
	previous, hasPrevious, _ := p.Current(ctx)
	var before any
	if hasPrevious {
		before = map[string]any{"currentVersion": previous.Version, "versionId": previous.ID}
	}
	entry := audit.Entry{Action: audit.ActionDisciplineRulePublished, ResourceType: "disciplineRuleVersion", ResourceID: id, Before: before, After: map[string]any{"version": v.Version, "status": "PUBLISHED"}}
	err = p.withAudit(ctx, audit.Metadata{ActorID: actor}, entry, func(tx pgx.Tx) error {
		var status string
		var rowVersion int
		if err := tx.QueryRow(ctx, `SELECT status,row_version FROM discipline_rule_versions WHERE id=$1 FOR UPDATE`, id).Scan(&status, &rowVersion); errors.Is(err, pgx.ErrNoRows) {
			return disciplineconfig.ErrNotFound
		} else if err != nil {
			return err
		}
		if status != "DRAFT" {
			return disciplineconfig.ErrImmutable
		}
		if rowVersion != expected {
			return disciplineconfig.ErrConflict
		}
		// Finalization takes the shared side of this lock before reading current rules.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(74077)`); err != nil {
			return err
		}
		var old string
		err := tx.QueryRow(ctx, `SELECT version_id::text FROM discipline_rule_current WHERE singleton=true FOR UPDATE`).Scan(&old)
		if err == nil {
			// Compare membership and conversion settings independently of row IDs and display order.
			if _, err = tx.Exec(ctx, `WITH groups AS (
                SELECT g.id,g.version_id,g.group_key,g.result_unit,
                    jsonb_agg(jsonb_build_array(t.inspection_type_id,t.source_unit,t.operator,t.conversion_factor)
                        ORDER BY t.inspection_type_id) AS terms
                FROM discipline_rule_groups g JOIN discipline_rule_group_terms t ON t.group_id=g.id
                WHERE g.version_id IN ($1,$2) GROUP BY g.id
            )
            INSERT INTO discipline_tier_template_bindings(rule_id,group_key,tier_code,template_id)
            SELECT successor.version_id, successor.group_key, binding.tier_code,binding.template_id
            FROM discipline_tier_template_bindings binding
            JOIN groups previous ON previous.version_id=binding.rule_id AND previous.group_key=binding.group_key
            JOIN groups successor ON successor.version_id=$2 AND successor.group_key=previous.group_key
                AND successor.result_unit=previous.result_unit AND successor.terms=previous.terms
            JOIN discipline_rule_tiers tier ON tier.group_id=successor.id AND tier.code=binding.tier_code
            WHERE binding.rule_id=$1`, old, id); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE discipline_rule_versions SET status='SUPERSEDED' WHERE id=$1`, old); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE discipline_rule_current SET version_id=$1,row_version=row_version+1 WHERE singleton=true`, id); err != nil {
				return err
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			if _, err = tx.Exec(ctx, `INSERT INTO discipline_rule_current(singleton,version_id) VALUES(true,$1)`, id); err != nil {
				return err
			}
		} else {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE discipline_rule_versions SET status='PUBLISHED',published_by=$2,published_at=now() WHERE id=$1`, id, actor)
		if err != nil || old == "" {
			return err
		}
		if _, err := tx.Exec(ctx, changedRuleCasesSQL, old, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT DISTINCT c.semester_id::text FROM changed_rule_cases c ORDER BY 1`)
		if err != nil {
			return err
		}
		semesters, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, semester := range semesters {
			var report string
			err := tx.QueryRow(ctx, `SELECT r.report_version_id::text FROM weekly_report_reviews r
 JOIN weekly_reports w ON w.id=r.report_id WHERE w.semester_id=$1 AND r.action='FINALIZED'
 ORDER BY r.created_at DESC,r.report_version_id LIMIT 1`, semester).Scan(&report)
			if err != nil {
				return err
			}
			if err := changeAndNotifyCases(ctx, tx, actor, `UPDATE discipline_cases d SET status='VOID',row_version=row_version+1,
 void_reason='处分规则更新',voided_at=statement_timestamp(),reassessment_rule_version_id=$2,reassessment_report_version_id=$3
 FROM changed_rule_cases c WHERE c.id=d.id AND c.semester_id=$1 AND d.status IN ('PENDING','DEFERRED') RETURNING d.id::text`, semester, id, report); err != nil {
				return err
			}
			if err := changeAndNotifyCases(ctx, tx, actor, semesterDisciplineAmountsSQL+disciplineGroupAmountsSQL+generateDisciplineCasesSQL+
				` WHERE EXISTS(SELECT 1 FROM changed_rule_cases c WHERE c.semester_id=$1::uuid AND c.student_number=a.number AND c.group_key=a.group_key)`+activeTierConflictSQL+" RETURNING id::text", semester, report); err != nil {
				return err
			}
		}
		return err
	})
	if err != nil {
		return current, err
	}
	return p.GetVersion(ctx, id)
}
