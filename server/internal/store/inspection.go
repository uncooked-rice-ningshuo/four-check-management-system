package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// inspectionAccessSQL requires authorization for every responsibility class.
// This prevents an allowed class from exposing another class's students or permitting partial edits.
func inspectionAccessSQL(a inspection.MutationAccess, mode string, args *[]any) string {
	if a.Unrestricted {
		return "TRUE"
	}
	one := func(class, grade string) string {
		action := func(name string) string {
			rules := func(effect string) string {
				var ors []string
				for _, r := range a.Permissions {
					if r.Action != name || r.Effect != effect {
						continue
					}
					var ands []string
					for _, d := range []struct{ col, value string }{{"sem.academic_year_id", r.Scope.AcademicYearID}, {"ir.semester_id", r.Scope.SemesterID}, {"ir.week_id", r.Scope.WeekID}, {"ir.inspection_type_id", r.Scope.InspectionTypeID}, {"ir.department_id", r.Scope.DepartmentID}, {class, r.Scope.ClassID}, {grade, r.Scope.Grade}} {
						if d.value != "" {
							*args = append(*args, d.value)
							cast := "::uuid"
							if d.col == grade {
								cast = "::text"
							}
							term := fmt.Sprintf("%s=$%d%s", d.col, len(*args), cast)
							// A missing legacy scope dimension overlaps DENY, and never matches a scoped ALLOW.
							if effect == permission.EffectDeny {
								term = "(" + term + " OR " + d.col + " IS NULL"
								if d.col == grade {
									term += " OR " + d.col + "=''"
								}
								term += ")"
							}
							ands = append(ands, term)
						}
					}
					if len(ands) == 0 {
						ors = append(ors, "TRUE")
					} else {
						ors = append(ors, "("+strings.Join(ands, " AND ")+")")
					}
				}
				if len(ors) == 0 {
					return "FALSE"
				}
				return "(" + strings.Join(ors, " OR ") + ")"
			}
			allow, deny := rules(permission.EffectAllow), rules(permission.EffectDeny)
			return "(" + allow + " AND NOT " + deny + ")"
		}
		if mode == "read" {
			return action(permission.ActionInspectionView)
		}
		if mode == "review" {
			return action(permission.ActionInspectionReview)
		}
		*args = append(*args, a.ActorID)
		owner := fmt.Sprintf("ir.created_by=$%d::uuid", len(*args))
		return "((" + owner + " AND " + action(permission.ActionInspectionEnter) + ") OR " + action(permission.ActionInspectionManage) + ")"
	}
	personal := one("ir.class_id", "ir.grade_snapshot")
	responsible := one("rc.class_id", "rc.grade_snapshot")
	return "((ir.class_id IS NOT NULL AND " + personal + ") OR (ir.class_id IS NULL AND EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=ir.id) AND NOT EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=ir.id AND NOT (" + responsible + "))))"
}

type inspectionTx struct{ tx pgx.Tx }

func (p *Postgres) WithInspectionTransaction(ctx context.Context, fn func(inspection.InspectionTransaction) error) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(&inspectionTx{tx}); err != nil {
		return inspectionDBError(err)
	}
	return inspectionDBError(tx.Commit(ctx))
}
func inspectionDBError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code == "23505" && pe.ConstraintName == "uq_inspection_records_active_dedup" {
			return inspection.ErrAttendanceDuplicate
		}
		if pe.Code == "40001" || pe.Code == "40P01" {
			return inspection.ErrRecordVersionConflict
		}
	}
	return err
}
func (t *inspectionTx) Audit(ctx context.Context, m audit.Metadata, e audit.Entry) error {
	return insertAuditTx(ctx, t.tx, m, e)
}
func (t *inspectionTx) Context(ctx context.Context, i inspection.InspectionInput) (inspection.InspectionContext, error) {
	c := inspection.InspectionContext{DetailContext: inspection.DetailContext{SemesterID: i.SemesterID, WeekID: i.WeekID, InspectionTypeID: i.InspectionTypeID}}
	if i.DepartmentID == "" {
		err := t.tx.QueryRow(ctx, `SELECT it.code,sem.academic_year_id,w.start_date::text,w.end_date::text,w.attendance_penalty_multiplier FROM semester_weeks w
 JOIN semesters sem ON sem.id=w.semester_id JOIN academic_years y ON y.id=sem.academic_year_id
 JOIN inspection_types it ON it.id=$3
 WHERE w.id=$1 AND sem.id=$2 AND w.enabled AND sem.enabled AND y.enabled AND it.enabled
 FOR SHARE OF w,sem,y,it`, i.WeekID, i.SemesterID, i.InspectionTypeID).Scan(&c.TypeCode, &c.AcademicYearID, &c.WeekStart, &c.WeekEnd, &c.AttendancePenaltyMultiplier)
		if errors.Is(err, pgx.ErrNoRows) {
			return c, inspection.ErrInvalidAttendanceRecord
		}
		return c, err
	}
	err := t.tx.QueryRow(ctx, `SELECT it.code,sem.academic_year_id,w.start_date::text,w.end_date::text,w.attendance_penalty_multiplier FROM semester_weeks w
 JOIN semesters sem ON sem.id=w.semester_id JOIN academic_years y ON y.id=sem.academic_year_id
 JOIN inspection_types it ON it.id=$3 JOIN departments d ON d.id=$4
 JOIN department_inspection_scopes ds ON ds.inspection_type_id=it.id AND ds.department_id=d.id
 WHERE w.id=$1 AND sem.id=$2 AND w.enabled AND sem.enabled AND y.enabled AND it.enabled AND d.enabled AND ds.enabled AND ds.can_enter
	 FOR SHARE OF w,sem,y,it,d,ds`, i.WeekID, i.SemesterID, i.InspectionTypeID, i.DepartmentID).Scan(&c.TypeCode, &c.AcademicYearID, &c.WeekStart, &c.WeekEnd, &c.AttendancePenaltyMultiplier)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, inspection.ErrInvalidAttendanceRecord
	}
	return c, err
}
func (t *inspectionTx) References(ctx context.Context, r inspection.InspectionRecord, c inspection.InspectionContext, a inspection.MutationAccess, owner string) (inspection.InspectionReferences, error) {
	return t.lockReferences(ctx, r, c, func(scope permission.Scope) bool { return inspection.AllowsInspectionScope(a, scope, owner) })
}

// lockReferences separates reference integrity from the caller's already compiled scope.
func (t *inspectionTx) lockReferences(ctx context.Context, r inspection.InspectionRecord, c inspection.InspectionContext, allowed func(permission.Scope) bool) (inspection.InspectionReferences, error) {
	var refs inspection.InspectionReferences
	student, class, course, classroom, dorm, area, h, rooms := inspection.InspectionDetailReferences(r.Details)
	classes := []string{class}
	students := []string{student}
	if h != nil {
		classes = append([]string(nil), h.ResponsibleClassIDs...)
		students = append([]string(nil), h.ResponsibleStudentIDs...)
	}
	sort.Strings(classes)
	sort.Strings(students)
	for _, id := range classes {
		var v inspection.ClassReference
		err := t.tx.QueryRow(ctx, `SELECT c.id,c.name,g.name FROM classes c JOIN grades g ON g.id=c.grade_id WHERE c.id=$1 AND c.enabled AND g.enabled FOR SHARE OF c,g`, id).Scan(&v.ID, &v.Name, &v.Grade)
		if err != nil {
			return refs, referenceError(err)
		}
		scope := permission.Scope{AcademicYearID: c.AcademicYearID, SemesterID: r.SemesterID, WeekID: r.WeekID, InspectionTypeID: r.InspectionTypeID, DepartmentID: r.DepartmentID, ClassID: v.ID, Grade: v.Grade}
		if !allowed(scope) {
			return refs, inspection.ErrScopeMismatch
		}
		refs.Classes = append(refs.Classes, v)
	}
	// Student identifiers are fetched only within already-authorized classes, in SQL.
	for _, id := range students {
		if id == "" {
			continue
		}
		var v inspection.StudentReference
		err := t.tx.QueryRow(ctx, `SELECT s.id,s.class_id,s.student_no,s.name FROM students s WHERE s.id=$1 AND s.class_id=ANY($2::uuid[]) AND s.enabled FOR SHARE OF s`, id, classes).Scan(&v.ID, &v.ClassID, &v.Number, &v.Name)
		if err != nil {
			return refs, referenceError(err)
		}
		refs.Students = append(refs.Students, v)
	}
	check := func(sql string, args ...any) error {
		var yes bool
		err := t.tx.QueryRow(ctx, sql, args...).Scan(&yes)
		return referenceError(err)
	}
	if classroom != "" {
		if err := check(`SELECT true FROM classrooms cr JOIN teaching_buildings b ON b.id=cr.teaching_building_id WHERE cr.id=$1 AND cr.enabled AND b.enabled FOR SHARE OF cr,b`, classroom); err != nil {
			return refs, err
		}
	}
	if course != "" {
		if err := check(`SELECT true FROM courses co JOIN course_classes cc ON cc.course_id=co.id WHERE co.id=$1 AND cc.class_id=$2 AND co.enabled FOR SHARE OF co,cc`, course, class); err != nil {
			return refs, err
		}
		if err := check(`SELECT true FROM course_schedules WHERE course_id=$1 AND semester_id=$2 AND classroom_id=$3 LIMIT 1 FOR SHARE`, course, r.SemesterID, classroom); err != nil {
			return refs, err
		}
		if r.TypeCode == inspection.InspectionTypeAttendance {
			// Attendance records must match the actual teaching day; a semester-level
			// relationship alone permits a wrong-day or unscheduled-section absence.
			details, ok := r.Details.(*inspection.AttendanceDetails)
			if !ok {
				return refs, inspection.ErrInvalidAttendanceRecord
			}
			first, last, valid := inspection.ParseCoursePeriod(details.Period)
			if !valid {
				return refs, inspection.ErrInvalidAttendanceRecord
			}
			if err := check(`SELECT true FROM course_schedules WHERE course_id=$1 AND semester_id=$2 AND classroom_id=$3 AND weekday=EXTRACT(ISODOW FROM $4::date) AND start_section<=$5 AND end_section>=$6 LIMIT 1 FOR SHARE`, course, r.SemesterID, classroom, r.Normalized.Date, first, last); err != nil {
				return refs, err
			}
		}
	}
	lockDorm := func(id string, building string) error {
		return check(`SELECT true FROM dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE d.id=$1 AND d.enabled AND b.enabled AND a.enabled AND ($2::text='' OR b.id=NULLIF($2,'')::uuid) FOR SHARE OF d,b,a`, id, building)
	}
	if dorm != "" {
		if err := lockDorm(dorm, ""); err != nil {
			return refs, err
		}
	}
	if r.TypeCode == inspection.InspectionTypeLateReturn {
		if err := check(`SELECT true FROM students WHERE id=$1 AND dormitory_id=$2 FOR SHARE`, student, dorm); err != nil {
			return refs, err
		}
	}
	if r.TypeCode == inspection.InspectionTypeClassroomHygiene {
		if err := check(`SELECT true FROM classroom_hygiene_areas WHERE id=$1 AND classroom_id=$2 AND enabled FOR SHARE`, area, classroom); err != nil {
			return refs, err
		}
	}
	if r.TypeCode == inspection.InspectionTypePublicDormitoryHygiene {
		var building string
		err := t.tx.QueryRow(ctx, `SELECT ar.dormitory_building_id FROM public_dormitory_hygiene_areas ar JOIN dormitory_buildings b ON b.id=ar.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE ar.id=$1 AND ar.enabled AND b.enabled AND a.enabled FOR SHARE OF ar,b,a`, area).Scan(&building)
		if err != nil {
			return refs, referenceError(err)
		}
		for _, id := range rooms {
			if err = lockDorm(id, building); err != nil {
				return refs, err
			}
		}
	}
	if h != nil && h.ReasonCode != "" {
		if err := check(`SELECT true FROM dictionary_items i JOIN dictionary_types t ON t.id=i.type_id WHERE t.code=$1 AND i.value=$2 AND t.enabled AND i.enabled FOR SHARE OF i,t`, r.TypeCode+"_REASON", h.ReasonCode); err != nil {
			return refs, err
		}
	}
	return refs, nil
}
func referenceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return inspection.ErrInvalidAttendanceRecord
	}
	return err
}

const inspectionFrom = ` FROM inspection_records ir JOIN semesters sem ON sem.id=ir.semester_id JOIN inspection_types it ON it.id=ir.inspection_type_id `
const inspectionColumns = `SELECT ir.id,ir.semester_id,ir.week_id,ir.inspection_type_id,it.code,COALESCE(ir.department_id::text,''),ir.status,ir.source,COALESCE(ir.import_batch_id::text,''),ir.row_version,ir.created_by,ir.created_at,ir.updated_at,ir.submission_id IS NOT NULL,ir.locked_by_report_id IS NOT NULL,ir.student_no_snapshot,ir.student_name_snapshot,ir.class_name_snapshot,ir.grade_snapshot`
const inspectionEditableSQL = `ir.locked_by_report_id IS NULL AND (
 (ir.status='DRAFT' AND ir.submission_id IS NULL)
 OR (ir.status='RETURNED' AND ir.submission_id IS NOT NULL AND EXISTS(
  SELECT 1 FROM department_submissions ds WHERE ds.id=ir.submission_id AND ds.status='RETURNED')))`

func (t *inspectionTx) Read(ctx context.Context, id string, a inspection.MutationAccess, mode string, lock bool) (inspection.InspectionRecord, error) {
	args := []any{id}
	editable := inspectionAccessSQL(a, "mutate", &args)
	sql := inspectionColumns + `,(` + inspectionEditableSQL + ` AND ` + editable + `)` + inspectionFrom + ` WHERE ir.id=$1 AND ir.deleted_at IS NULL AND ` + inspectionAccessSQL(a, mode, &args)
	if lock {
		sql += " FOR UPDATE OF ir"
	}
	var r inspection.InspectionRecord
	err := t.tx.QueryRow(ctx, sql, args...).Scan(&r.ID, &r.SemesterID, &r.WeekID, &r.InspectionTypeID, &r.TypeCode, &r.DepartmentID, &r.Status, &r.Source, &r.ImportBatchID, &r.RowVersion, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.HasSubmission, &r.HasReportLock, &r.StudentNumber, &r.StudentName, &r.ClassName, &r.Grade, &r.CanEdit)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, inspection.ErrRecordNotFound
	}
	if err != nil {
		return r, err
	}
	raw, err := t.readDetails(ctx, r)
	if err != nil {
		return r, err
	}
	r.Details, err = inspection.DecodeStoredInspectionDetails(r.TypeCode, raw)
	if err != nil {
		return r, err
	}
	if err = t.responsibilityLabels(ctx, &r); err != nil {
		return r, err
	}
	err = t.currentReferenceLabels(ctx, &r)
	return r, err
}
func (p *Postgres) GetInspection(ctx context.Context, id string, a inspection.MutationAccess) (inspection.InspectionRecord, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return inspection.InspectionRecord{}, err
	}
	defer tx.Rollback(ctx)
	r, err := (&inspectionTx{tx}).Read(ctx, id, a, "read", false)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (p *Postgres) ListInspections(ctx context.Context, f inspection.InspectionFilter, a inspection.MutationAccess) (inspection.InspectionPage, error) {
	result := inspection.InspectionPage{Items: []inspection.InspectionRecord{}, Page: f.Page, PageSize: f.PageSize}
	args := []any{}
	where := ` WHERE ir.deleted_at IS NULL AND ` + inspectionAccessSQL(a, "read", &args)
	for _, filter := range []struct{ column, value string }{{"ir.semester_id", f.SemesterID}, {"ir.week_id", f.WeekID}, {"ir.inspection_type_id", f.InspectionTypeID}, {"ir.department_id", f.DepartmentID}, {"ir.status", f.Status}, {"ir.source", f.Source}} {
		if filter.value != "" {
			args = append(args, filter.value)
			where += fmt.Sprintf(" AND %s=$%d", filter.column, len(args))
		}
	}
	if f.CollegeOnly {
		where += " AND ir.department_id IS NULL"
	}
	if f.ClassID != "" {
		args = append(args, f.ClassID)
		where += fmt.Sprintf(` AND (ir.class_id=$%d::uuid OR EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=ir.id AND rc.class_id=$%d::uuid))`, len(args), len(args))
	}
	if f.Grade != "" {
		args = append(args, f.Grade)
		where += fmt.Sprintf(` AND (ir.grade_snapshot=$%d OR EXISTS(SELECT 1 FROM inspection_responsibility_classes rc WHERE rc.record_id=ir.id AND rc.grade_snapshot=$%d))`, len(args), len(args))
	}
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT count(*)"+inspectionFrom+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	direction := "DESC"
	if f.Sort == inspection.SortOccurredAtAscending {
		direction = "ASC"
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := tx.Query(ctx, `SELECT ir.id`+inspectionFrom+where+` ORDER BY COALESCE(ir.occurred_at,ir.inspection_date::timestamp AT TIME ZONE 'Asia/Shanghai') `+direction+`,ir.id `+direction+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	for _, id := range ids {
		r, e := (&inspectionTx{tx}).Read(ctx, id, a, "read", false)
		if e != nil {
			return result, e
		}
		result.Items = append(result.Items, r)
	}
	return result, tx.Commit(ctx)
}

func (t *inspectionTx) readDetails(ctx context.Context, r inspection.InspectionRecord) ([]byte, error) {
	common := `jsonb_build_object('date',COALESCE(ir.inspection_date,(ir.occurred_at AT TIME ZONE 'Asia/Shanghai')::date)::text,'time',CASE WHEN ir.occurred_at IS NOT NULL THEN to_char(ir.occurred_at AT TIME ZONE 'Asia/Shanghai','HH24:MI:SS') ELSE '' END,'round',COALESCE(ir.inspection_round,''),'remark',ir.remark)`
	personal := `jsonb_build_object('studentId',ir.student_id,'classId',ir.class_id)`
	var table, extra string
	switch r.TypeCode {
	case inspection.InspectionTypeAttendance, inspection.InspectionTypeUnattended:
		table = "attendance_details"
		extra = personal + ` || jsonb_build_object('courseId',d.course_id,'classroomId',d.classroom_id,'absenceHours',d.absence_hours,'period',d.period_text,'penaltyMultiplier',d.penalty_multiplier,'penaltyHours',d.penalty_hours)`
	case inspection.InspectionTypeMorning:
		table = "morning_exercise_details"
		extra = personal + ` || jsonb_build_object('absenceCount',d.absence_count)`
	case inspection.InspectionTypeLateReturn:
		table = "late_return_details"
		extra = personal + ` || jsonb_build_object('dormitoryId',d.dormitory_id)`
	case inspection.InspectionTypeDormitoryHygiene:
		table = "dorm_hygiene_details"
		extra = `jsonb_build_object('dormitoryId',d.dormitory_id)`
	case inspection.InspectionTypeClassroomHygiene:
		table = "classroom_hygiene_details"
		extra = `jsonb_build_object('classroomId',d.classroom_id,'areaId',d.area_id)`
	case inspection.InspectionTypePublicDormitoryHygiene:
		table = "public_dormitory_hygiene_details"
		extra = `jsonb_build_object('areaId',d.area_id,'responsibleDormitoryIds',COALESCE((SELECT jsonb_agg(dormitory_id ORDER BY dormitory_id) FROM inspection_responsibility_dormitories WHERE record_id=ir.id),'[]'::jsonb))`
	default:
		return nil, inspection.ErrUnknownInspectionType
	}
	if strings.Contains(table, "hygiene_details") {
		extra += ` || jsonb_build_object('result',d.result,'reasonCode',COALESCE(d.reason_code,''),'description',d.description,'responsibleClassIds',COALESCE((SELECT jsonb_agg(class_id ORDER BY class_id) FROM inspection_responsibility_classes WHERE record_id=ir.id),'[]'::jsonb),'responsibleStudentIds',COALESCE((SELECT jsonb_agg(student_id ORDER BY student_id) FROM inspection_responsibility_students WHERE record_id=ir.id),'[]'::jsonb))`
	}
	var raw []byte
	err := t.tx.QueryRow(ctx, `SELECT `+common+` || `+extra+` FROM inspection_records ir JOIN `+table+` d ON d.record_id=ir.id WHERE ir.id=$1`, r.ID).Scan(&raw)
	return raw, err
}
func (t *inspectionTx) Save(ctx context.Context, r inspection.InspectionRecord, old *inspection.InspectionRecord, a inspection.MutationAccess) (inspection.InspectionRecord, error) {
	student, class, _, _, _, _, h, _ := inspection.InspectionDetailReferences(r.Details)
	var sn, name, cn, grade, remark string
	if h == nil {
		if len(r.References.Students) != 1 || len(r.References.Classes) != 1 {
			return r, inspection.ErrInvalidAttendanceRecord
		}
		sn, name = r.References.Students[0].Number, r.References.Students[0].Name
		cn, grade = r.References.Classes[0].Name, r.References.Classes[0].Grade
	}
	r.StudentNumber, r.StudentName, r.ClassName, r.Grade = sn, name, cn, grade
	r.CanEdit = true
	raw, err := json.Marshal(r.Details)
	if err != nil {
		return r, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return r, err
	}
	if v, ok := fields["remark"]; ok {
		if err = json.Unmarshal(v, &remark); err != nil {
			return r, err
		}
	}
	var occurred any
	if !r.Normalized.OccurredAt.IsZero() {
		occurred = r.Normalized.OccurredAt
	}
	date := ""
	if r.Normalized.OccurrenceKind != inspection.OccurrenceTime {
		date = r.Normalized.Date
	}
	args := []any{r.ID, r.InspectionTypeID, r.SemesterID, r.WeekID, r.DepartmentID, student, sn, name, class, cn, grade, occurred, r.Normalized.ViolationCount, remark, r.Normalized.DuplicateKey, r.CreatedBy, r.Normalized.OccurrenceKind, date, r.Normalized.Round}
	if old == nil {
		if r.Source != inspection.RecordSourceManual && r.Source != inspection.RecordSourceExcel {
			return r, inspection.ErrInspectionRequest
		}
		if (r.Source == inspection.RecordSourceExcel) != (r.ImportBatchID != "") {
			return r, inspection.ErrInspectionRequest
		}
		args = append(args, r.Source, r.ImportBatchID)
		err = t.tx.QueryRow(ctx, `INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,student_name_snapshot,class_id,class_name_snapshot,grade_snapshot,occurred_at,violation_count,remark,dedup_key,created_by,occurrence_kind,inspection_date,inspection_round,source,status,row_version,import_batch_id)
 VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,NULLIF($9,'')::uuid,$10,$11,$12,$13,$14,$15,$16,$17,NULLIF($18,'')::date,NULLIF($19,''),$20,'DRAFT',1,NULLIF($21,'')::uuid) RETURNING created_at,updated_at,row_version`, args...).Scan(&r.CreatedAt, &r.UpdatedAt, &r.RowVersion)
	} else {
		args = append(args, old.RowVersion)
		guard := inspectionAccessSQL(a, "mutate", &args)
		err = t.tx.QueryRow(ctx, `UPDATE inspection_records ir SET inspection_type_id=$2,semester_id=$3,week_id=$4,department_id=NULLIF($5,'')::uuid,student_id=NULLIF($6,'')::uuid,student_no_snapshot=$7,student_name_snapshot=$8,class_id=NULLIF($9,'')::uuid,class_name_snapshot=$10,grade_snapshot=$11,occurred_at=$12,violation_count=$13,remark=$14,dedup_key=$15,created_by=$16,occurrence_kind=$17,inspection_date=NULLIF($18,'')::date,inspection_round=NULLIF($19,''),row_version=ir.row_version+1,updated_at=now()
	 FROM semesters sem WHERE sem.id=ir.semester_id AND ir.id=$1 AND ir.row_version=$20 AND ir.deleted_at IS NULL AND `+inspectionEditableSQL+` AND `+guard+` RETURNING ir.updated_at,ir.row_version`, args...).Scan(&r.UpdatedAt, &r.RowVersion)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return r, inspection.ErrRecordVersionConflict
	}
	if err != nil {
		return r, inspectionDBError(err)
	}
	if err = t.writeDetails(ctx, r, old != nil); err != nil {
		return r, err
	}
	if r.TypeCode == inspection.InspectionTypeAttendance {
		stored, readErr := t.readDetails(ctx, r)
		if readErr != nil {
			return r, readErr
		}
		if readErr = json.Unmarshal(stored, r.Details); readErr != nil {
			return r, readErr
		}
	}
	if err = t.responsibilityLabels(ctx, &r); err != nil {
		return r, err
	}
	err = t.currentReferenceLabels(ctx, &r)
	return r, err
}
func (t *inspectionTx) writeDetails(ctx context.Context, r inspection.InspectionRecord, updating bool) error {
	var sql string
	var args []any
	switch v := r.Details.(type) {
	case *inspection.AttendanceDetails:
		sql, args = courseDetailSQL(r.ID, v.CourseAbsenceDetails, r.PenaltyMultiplier)
	case *inspection.UnattendedClassroomDetails:
		sql, args = courseDetailSQL(r.ID, v.CourseAbsenceDetails, nil)
	case *inspection.MorningAttendanceDetails:
		sql = `INSERT INTO morning_exercise_details(record_id,absence_count) VALUES($1,$2) ON CONFLICT(record_id) DO UPDATE SET absence_count=excluded.absence_count`
		args = []any{r.ID, v.AbsenceCount}
	case *inspection.LateReturnDetails:
		sql = `INSERT INTO late_return_details(record_id,dormitory_id) VALUES($1,$2) ON CONFLICT(record_id) DO UPDATE SET dormitory_id=excluded.dormitory_id`
		args = []any{r.ID, v.DormitoryID}
	case *inspection.DormitoryHygieneDetails:
		sql = `INSERT INTO dorm_hygiene_details(record_id,dormitory_id,result,reason_code,description) VALUES($1,$2,$3,NULLIF($4,''),$5) ON CONFLICT(record_id) DO UPDATE SET dormitory_id=excluded.dormitory_id,result=excluded.result,reason_code=excluded.reason_code,description=excluded.description`
		args = []any{r.ID, v.DormitoryID, v.Result, v.ReasonCode, v.Description}
	case *inspection.ClassroomHygieneDetails:
		sql = `INSERT INTO classroom_hygiene_details(record_id,classroom_id,area_id,result,reason_code,description) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) ON CONFLICT(record_id) DO UPDATE SET classroom_id=excluded.classroom_id,area_id=excluded.area_id,result=excluded.result,reason_code=excluded.reason_code,description=excluded.description`
		args = []any{r.ID, v.ClassroomID, v.AreaID, v.Result, v.ReasonCode, v.Description}
	case *inspection.PublicDormitoryHygieneDetails:
		sql = `INSERT INTO public_dormitory_hygiene_details(record_id,area_id,result,reason_code,description) VALUES($1,$2,$3,NULLIF($4,''),$5) ON CONFLICT(record_id) DO UPDATE SET area_id=excluded.area_id,result=excluded.result,reason_code=excluded.reason_code,description=excluded.description`
		args = []any{r.ID, v.AreaID, v.Result, v.ReasonCode, v.Description}
	default:
		return inspection.ErrUnknownInspectionType
	}
	if _, err := t.tx.Exec(ctx, sql, args...); err != nil {
		return err
	}
	_, _, _, _, _, _, h, rooms := inspection.InspectionDetailReferences(r.Details)
	if h == nil {
		return nil
	}
	if updating {
		for _, table := range []string{"inspection_responsibility_students", "inspection_responsibility_dormitories", "inspection_responsibility_classes"} {
			if _, err := t.tx.Exec(ctx, "DELETE FROM "+table+" WHERE record_id=$1", r.ID); err != nil {
				return err
			}
		}
	}
	for _, c := range r.References.Classes {
		if _, err := t.tx.Exec(ctx, `INSERT INTO inspection_responsibility_classes(record_id,class_id,class_name_snapshot,grade_snapshot) VALUES($1,$2,$3,$4)`, r.ID, c.ID, c.Name, c.Grade); err != nil {
			return err
		}
	}
	for _, s := range r.References.Students {
		if _, err := t.tx.Exec(ctx, `INSERT INTO inspection_responsibility_students(record_id,student_id,class_id,student_no_snapshot,student_name_snapshot) VALUES($1,$2,$3,$4,$5)`, r.ID, s.ID, s.ClassID, s.Number, s.Name); err != nil {
			return err
		}
	}
	for _, id := range rooms {
		if _, err := t.tx.Exec(ctx, `INSERT INTO inspection_responsibility_dormitories(record_id,dormitory_id) VALUES($1,$2)`, r.ID, id); err != nil {
			return err
		}
	}
	return nil
}
func courseDetailSQL(id string, v inspection.CourseAbsenceDetails, multiplier *int) (string, []any) {
	return `INSERT INTO attendance_details(record_id,course_id,classroom_id,absence_hours,period_text,penalty_multiplier) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(record_id) DO UPDATE SET course_id=excluded.course_id,classroom_id=excluded.classroom_id,absence_hours=excluded.absence_hours,period_text=excluded.period_text,penalty_multiplier=excluded.penalty_multiplier`, []any{id, v.CourseID, v.ClassroomID, v.AbsenceHours, v.Period, multiplier}
}
func (t *inspectionTx) Delete(ctx context.Context, r inspection.InspectionRecord, a inspection.MutationAccess) error {
	args := []any{r.ID, r.RowVersion}
	guard := inspectionAccessSQL(a, "mutate", &args)
	tag, err := t.tx.Exec(ctx, `UPDATE inspection_records ir SET deleted_at=now(),updated_at=now(),row_version=ir.row_version+1 FROM semesters sem WHERE sem.id=ir.semester_id AND ir.id=$1 AND ir.row_version=$2 AND ir.deleted_at IS NULL AND ir.status='DRAFT' AND ir.submission_id IS NULL AND ir.locked_by_report_id IS NULL AND `+guard, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return inspection.ErrRecordVersionConflict
	}
	return nil
}

// currentReferenceLabels runs only after record authorization or validated writes.
// Disabled referenced objects remain readable; these labels do not enter audit snapshots.
func (t *inspectionTx) currentReferenceLabels(ctx context.Context, r *inspection.InspectionRecord) error {
	_, _, course, classroom, dormitory, area, _, _ := inspection.InspectionDetailReferences(r.Details)
	if course == "" && classroom == "" && dormitory == "" && area == "" {
		return nil
	}
	return t.tx.QueryRow(ctx, `SELECT
 COALESCE((SELECT name FROM courses WHERE id=NULLIF($1,'')::uuid),''),
 COALESCE((SELECT b.name || ' ' || cr.room_no FROM classrooms cr JOIN teaching_buildings b ON b.id=cr.teaching_building_id WHERE cr.id=NULLIF($2,'')::uuid),''),
 COALESCE((SELECT a.name || ' ' || b.name || ' ' || d.room_no FROM dormitories d JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas a ON a.id=b.dormitory_area_id WHERE d.id=NULLIF($3,'')::uuid),''),
 COALESCE((SELECT b.name || ' ' || cr.room_no || ' / ' || ar.name FROM classroom_hygiene_areas ar JOIN classrooms cr ON cr.id=ar.classroom_id JOIN teaching_buildings b ON b.id=cr.teaching_building_id WHERE ar.id=NULLIF($4,'')::uuid AND $5='CLASSROOM_HYGIENE'),
 (SELECT da.name || ' ' || b.name || ' / ' || ar.floor_unit || ' / ' || ar.name FROM public_dormitory_hygiene_areas ar JOIN dormitory_buildings b ON b.id=ar.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id WHERE ar.id=NULLIF($4,'')::uuid AND $5='PUBLIC_DORMITORY_HYGIENE'),'')`, course, classroom, dormitory, area, r.TypeCode).Scan(&r.CurrentCourseName, &r.CurrentClassroomName, &r.CurrentDormitoryName, &r.CurrentAreaName)
}

// responsibilityLabels reads only snapshots attached to an already authorized record.
func (t *inspectionTx) responsibilityLabels(ctx context.Context, r *inspection.InspectionRecord) error {
	if r.TypeCode != inspection.InspectionTypeDormitoryHygiene &&
		r.TypeCode != inspection.InspectionTypeClassroomHygiene &&
		r.TypeCode != inspection.InspectionTypePublicDormitoryHygiene {
		return nil
	}
	read := func(query string) ([]inspection.EntryOption, error) {
		rows, err := t.tx.Query(ctx, query, r.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		items := make([]inspection.EntryOption, 0)
		for rows.Next() {
			var item inspection.EntryOption
			if err = rows.Scan(&item.ID, &item.Label); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, rows.Err()
	}
	var err error
	r.ResponsibleClasses, err = read(`SELECT class_id,class_name_snapshot || '（' || grade_snapshot || '级）' FROM inspection_responsibility_classes WHERE record_id=$1 ORDER BY class_id`)
	if err != nil {
		return err
	}
	r.ResponsibleStudents, err = read(`SELECT student_id,student_name_snapshot || '（' || student_no_snapshot || '）' FROM inspection_responsibility_students WHERE record_id=$1 ORDER BY student_id`)
	if err != nil {
		return err
	}
	if r.TypeCode == inspection.InspectionTypePublicDormitoryHygiene {
		r.ResponsibleDormitories, err = read(`SELECT d.id,da.name || ' ' || b.name || ' ' || d.room_no
 FROM inspection_responsibility_dormitories rd JOIN dormitories d ON d.id=rd.dormitory_id
 JOIN dormitory_buildings b ON b.id=d.dormitory_building_id JOIN dormitory_areas da ON da.id=b.dormitory_area_id
 WHERE rd.record_id=$1 ORDER BY d.id`)
		if err != nil {
			return err
		}
	}
	_, _, _, _, _, _, hygiene, _ := inspection.InspectionDetailReferences(r.Details)
	if hygiene != nil && hygiene.ReasonCode != "" {
		return t.tx.QueryRow(ctx, `SELECT COALESCE((SELECT di.label FROM dictionary_items di JOIN dictionary_types dt ON dt.id=di.type_id
 WHERE dt.code=$1 AND di.value=$2),'')`, r.TypeCode+"_REASON", hygiene.ReasonCode).Scan(&r.CurrentReasonLabel)
	}
	return nil
}
