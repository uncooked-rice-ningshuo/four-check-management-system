package store

import (
	"context"
	"four-check-management-system/server/internal/academic"
	"github.com/jackc/pgx/v5"
)

// Keep omitted rosters intact for calendar clients predating roster configuration.
func saveYearRoster(ctx context.Context, tx pgx.Tx, year academic.AcademicYear) error {
	r := year.Roster
	if r == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO academic_year_rosters VALUES ($1,1) ON CONFLICT (academic_year_id) DO UPDATE SET version=academic_year_rosters.version+1`, year.ID)
	if err != nil {
		return err
	}
	for _, table := range []string{"academic_year_roster_classes", "academic_year_roster_grade_majors", "academic_year_roster_grades", "academic_year_roster_majors"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE academic_year_id=$1", year.ID); err != nil {
			return err
		}
	}
	insert := func(sql string, args ...any) error {
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return academic.ErrInvalidInput
		}
		return nil
	}
	for i, id := range r.MajorIDs {
		if err := insert(`INSERT INTO academic_year_roster_majors SELECT $1,id,$3 FROM majors WHERE id=$2 AND enabled`, year.ID, id, i); err != nil {
			return err
		}
	}
	for i, g := range r.Grades {
		if err := insert(`INSERT INTO academic_year_roster_grades SELECT $1,id,$3 FROM grades WHERE id=$2 AND enabled`, year.ID, g.GradeID, i); err != nil {
			return err
		}
		for majorOrder, m := range g.Majors {
			if err := insert(`INSERT INTO academic_year_roster_grade_majors (academic_year_id,grade_id,major_id,sort_order) VALUES ($1,$2,$3,$4)`, year.ID, g.GradeID, m.MajorID, majorOrder); err != nil {
				return err
			}
			for j, id := range m.ClassIDs {
				if err := insert(`INSERT INTO academic_year_roster_classes SELECT $1,grade_id,major_id,id,$5 FROM classes WHERE grade_id=$2 AND major_id=$3 AND id=$4 AND enabled`, year.ID, g.GradeID, m.MajorID, id, j); err != nil {
					return err
				}
			}
		}
	}
	var missing bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM inspection_records ir JOIN semesters s ON s.id=ir.semester_id
 WHERE s.academic_year_id=$1 AND ir.deleted_at IS NULL AND EXISTS (
 SELECT 1 FROM (SELECT ir.class_id AS id UNION SELECT rc.class_id FROM inspection_responsibility_classes rc WHERE rc.record_id=ir.id) used
 WHERE used.id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM academic_year_roster_classes c WHERE c.academic_year_id=$1 AND c.class_id=used.id)))`, year.ID).Scan(&missing)
	if err != nil {
		return err
	}
	if missing {
		return academic.ErrRosterInUse
	}
	return nil
}

const calendarYearRows = `SELECT y.id,y.name,y.start_date,y.end_date,y.enabled,y.version,
 (SELECT jsonb_build_object('majorIds',(SELECT jsonb_agg(m.major_id ORDER BY m.sort_order) FROM academic_year_roster_majors m WHERE m.academic_year_id=y.id),
 'grades',(SELECT jsonb_agg(jsonb_build_object('gradeId',g.grade_id,'majors',(
 SELECT jsonb_agg(jsonb_build_object('majorId',gm.major_id,'classIds',COALESCE((
 SELECT jsonb_agg(c.class_id ORDER BY c.sort_order) FROM academic_year_roster_classes c
 WHERE (c.academic_year_id,c.grade_id,c.major_id)=(gm.academic_year_id,gm.grade_id,gm.major_id)), '[]'::jsonb)) ORDER BY gm.sort_order)
 FROM academic_year_roster_grade_majors gm
 WHERE (gm.academic_year_id,gm.grade_id)=(g.academic_year_id,g.grade_id))) ORDER BY g.sort_order)
 FROM academic_year_roster_grades g WHERE g.academic_year_id=y.id))
 FROM academic_year_rosters r WHERE r.academic_year_id=y.id)
 FROM academic_years y ORDER BY y.start_date DESC`
