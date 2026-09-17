package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/jackc/pgx/v5/pgxpool"
)

const recordFixturePrefix = "a0000000-0000-0000-0000-"

func rf(n string) string { return recordFixturePrefix + "0000000000" + n }

type recordPermissions struct {
	rules []permission.EffectivePermission
}

func (p *recordPermissions) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: p.rules}, nil
}
func TestInspectionPostgresCRUD(t *testing.T) {
	url := os.Getenv("INSPECTION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("INSPECTION_TEST_DATABASE_URL must point to an empty disposable database")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := &Postgres{Pool: pool}
	var exists bool
	if err = pool.QueryRow(ctx, "SELECT to_regclass('public.accounts') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("requires empty disposable database", err)
	}
	files, _ := filepath.Glob("../../migrations/*.sql")
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(b)); e != nil {
			t.Fatal(file, e)
		}
	}
	seed, err := os.ReadFile("testdata/phase_two_extension_seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(seed)); err != nil {
		t.Fatal(err)
	}
	sql := `UPDATE students SET dormitory_id='a0000000-0000-0000-0000-000000000012';
 INSERT INTO course_classes(course_id,class_id) VALUES('a0000000-0000-0000-0000-000000000013','a0000000-0000-0000-0000-000000000006');
 INSERT INTO course_schedules(id,course_id,semester_id,classroom_id,weekday,start_section,end_section) VALUES(gen_random_uuid(),'a0000000-0000-0000-0000-000000000013','a0000000-0000-0000-0000-000000000003','a0000000-0000-0000-0000-000000000009',3,1,2);
 INSERT INTO classroom_hygiene_areas(id,classroom_id,name) VALUES('a0000000-0000-0000-0000-000000000020','a0000000-0000-0000-0000-000000000009','Floor');
 INSERT INTO public_dormitory_hygiene_areas(id,dormitory_building_id,floor_unit,name) VALUES('a0000000-0000-0000-0000-000000000021','a0000000-0000-0000-0000-000000000011','1','Corridor');
 INSERT INTO department_inspection_scopes(department_id,inspection_type_id,enabled,can_enter,can_review) SELECT '10000000-0000-0000-0000-000000000002',id,true,true,true FROM inspection_types ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET enabled=true,can_enter=true;
 INSERT INTO dictionary_types(id,code,name) SELECT gen_random_uuid(),code || '_REASON',name || '原因' FROM inspection_types WHERE code LIKE '%HYGIENE';
 INSERT INTO dictionary_items(id,type_id,value,label) SELECT gen_random_uuid(),id,'FLOOR','地面' FROM dictionary_types WHERE code LIKE '%HYGIENE_REASON';`
	if _, err = pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	perms := &recordPermissions{}
	svc := inspection.NewRecordService(db, perms)
	admin := inspection.ManualActor{ID: rf("01"), IsSuperAdmin: true}
	member := inspection.ManualActor{ID: rf("01")}
	types := map[string]string{}
	rows, err := pool.Query(ctx, "SELECT code,id FROM inspection_types")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code, id string
		if err = rows.Scan(&code, &id); err != nil {
			t.Fatal(err)
		}
		types[code] = id
	}
	rows.Close()
	makeInput := func(code string, extra map[string]any) inspection.InspectionInput {
		fields := map[string]any{"date": "2026-09-03", "remark": "Synthetic private text"}
		switch code {
		case inspection.InspectionTypeAttendance, inspection.InspectionTypeUnattended:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["courseId"] = rf("13")
			fields["classroomId"] = rf("09")
			fields["time"] = "08:00"
			fields["period"] = "1-2"
			fields["absenceHours"] = 2
		case inspection.InspectionTypeMorning:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
		case inspection.InspectionTypeLateReturn:
			fields["studentId"] = rf("07")
			fields["classId"] = rf("06")
			fields["dormitoryId"] = rf("12")
			fields["time"] = "23:00"
		default:
			fields["round"] = "first"
			fields["result"] = "QUALIFIED"
			fields["responsibleClassIds"] = []string{rf("06")}
			fields["responsibleStudentIds"] = []string{rf("07")}
			if code == inspection.InspectionTypeDormitoryHygiene {
				fields["dormitoryId"] = rf("12")
			} else if code == inspection.InspectionTypeClassroomHygiene {
				fields["classroomId"] = rf("09")
				fields["areaId"] = rf("20")
			} else {
				fields["areaId"] = rf("21")
				fields["responsibleDormitoryIds"] = []string{rf("12")}
			}
		}
		for k, v := range extra {
			fields[k] = v
		}
		raw, _ := json.Marshal(fields)
		return inspection.InspectionInput{SemesterID: rf("03"), WeekID: rf("04"), InspectionTypeID: types[code], DepartmentID: "10000000-0000-0000-0000-000000000002", Details: raw}
	}
	catalog, _ := inspection.InspectionCatalog()
	for _, invalid := range []inspection.InspectionInput{
		makeInput(inspection.InspectionTypeMorning, map[string]any{"studentId": rf("16")}),
		makeInput(inspection.InspectionTypeMorning, map[string]any{"classId": rf("16")}),
		makeInput(inspection.InspectionTypeClassroomHygiene, map[string]any{"classroomId": rf("19")}),
		makeInput(inspection.InspectionTypePublicDormitoryHygiene, map[string]any{"areaId": rf("20")}),
		makeInput(inspection.InspectionTypeDormitoryHygiene, map[string]any{"result": "UNQUALIFIED", "reasonCode": "UNKNOWN"}),
	} {
		if _, e := svc.Create(ctx, admin, invalid, audit.Metadata{}); !errors.Is(e, inspection.ErrInvalidAttendanceRecord) {
			t.Fatal("invalid reference accepted", e)
		}
	}
	for _, def := range catalog {
		t.Run(def.Code, func(t *testing.T) {
			input := makeInput(def.Code, nil)
			r, e := svc.Create(ctx, admin, input, audit.Metadata{})
			if e != nil {
				t.Fatal("create", e)
			}
			if _, e = svc.Create(ctx, admin, input, audit.Metadata{}); !errors.Is(e, inspection.ErrAttendanceDuplicate) {
				t.Fatal("duplicate", e)
			}
			read, e := svc.Detail(ctx, admin, r.ID)
			if e != nil || read.TypeCode != def.Code || read.Details == nil {
				t.Fatal("read", e)
			}
			page, e := svc.List(ctx, admin, inspection.InspectionFilter{InspectionTypeID: types[def.Code], Page: 1, PageSize: 1})
			if e != nil || len(page.Items) != 1 || page.Total < 1 {
				t.Fatal("list", e)
			}
			input = makeInput(def.Code, map[string]any{"remark": "corrected"})
			updated, e := svc.Update(ctx, admin, r.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: input}, audit.Metadata{})
			if e != nil || updated.RowVersion != 2 {
				t.Fatal("update", e)
			}
			if _, e = svc.Update(ctx, admin, r.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: input}, audit.Metadata{}); !errors.Is(e, inspection.ErrRecordVersionConflict) {
				t.Fatal("stale", e)
			}
			if e = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: r.ID, Version: 2}}, Reason: "重复"}, audit.Metadata{}); e != nil {
				t.Fatal("delete", e)
			}
			if _, e = svc.Detail(ctx, admin, r.ID); !errors.Is(e, inspection.ErrRecordNotFound) {
				t.Fatal("deleted visible", e)
			}
			if _, e = svc.Create(ctx, admin, input, audit.Metadata{}); e != nil {
				t.Fatal("recreate", e)
			}
			var auditText string
			if e = pool.QueryRow(ctx, `SELECT string_agg(after_json::text,' ') FROM audit_logs WHERE resource_id=$1`, r.ID).Scan(&auditText); e != nil {
				t.Fatal(e)
			}
			if strings.Contains(auditText, "Synthetic private text") || strings.Contains(auditText, "corrected") {
				t.Fatal("audit leaked text")
			}
		})
	}
	allow := func(action, class string) permission.EffectivePermission {
		return permission.EffectivePermission{Action: action, Effect: permission.EffectAllow, Scope: permission.Scope{ClassID: class}}
	}
	hygiene := makeInput(inspection.InspectionTypeDormitoryHygiene, map[string]any{"round": "cross", "responsibleClassIds": []string{rf("06"), rf("16")}})
	cross, err := svc.Create(ctx, admin, hygiene, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	perms.rules = []permission.EffectivePermission{allow(permission.ActionInspectionView, rf("06")), allow(permission.ActionInspectionEnter, rf("06"))}
	if _, err = svc.Detail(ctx, member, cross.ID); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("partial read", err)
	}
	page, err := svc.List(ctx, member, inspection.InspectionFilter{InspectionTypeID: types[inspection.InspectionTypeDormitoryHygiene]})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Items {
		if r.ID == cross.ID {
			t.Fatal("partial list leaked")
		}
	}
	if _, err = svc.Update(ctx, member, cross.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: hygiene}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("partial edit", err)
	}
	ownInput := makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-04"})
	own, err := svc.Create(ctx, member, ownInput, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	outside := makeInput(inspection.InspectionTypeMorning, map[string]any{"classId": rf("16"), "date": "2026-09-04"})
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: outside}, audit.Metadata{}); !errors.Is(err, inspection.ErrScopeMismatch) {
		t.Fatal("target scope", err)
	}
	// One inaccessible item must not delete the authorized item in a batch.
	err = svc.Delete(ctx, member, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 1}, {ID: cross.ID, Version: 1}}, Reason: "核对"}, audit.Metadata{})
	if !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("batch permission", err)
	}
	if _, err = svc.Detail(ctx, admin, own.ID); err != nil {
		t.Fatal("partial batch write", err)
	}
	// Independent manager permission may mutate another creator; ENTER alone may not.
	if _, err = pool.Exec(ctx, `INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role) VALUES($1,'other-fixture','Other fixture','not-a-login-hash','ACTIVE','SYSTEM','MEMBER')`, rf("30")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE inspection_records SET created_by=$1 WHERE id=$2", rf("30"), own.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: ownInput}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("enter edits other", err)
	}
	perms.rules = []permission.EffectivePermission{allow(permission.ActionInspectionManage, rf("06"))}
	if _, err = svc.Update(ctx, member, own.ID, inspection.InspectionEdit{Version: 1, Reason: "核对", Record: ownInput}, audit.Metadata{}); err != nil {
		t.Fatal("manager", err)
	}
	perms.rules = append(perms.rules, permission.EffectivePermission{Action: permission.ActionInspectionManage, Effect: permission.EffectDeny, Scope: permission.Scope{ClassID: rf("06")}})
	if err = svc.Delete(ctx, member, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 2}}, Reason: "核对"}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotFound) {
		t.Fatal("deny", err)
	}
	// Two simultaneous writers: exactly one advances the version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := svc.Update(ctx, admin, own.ID, inspection.InspectionEdit{Version: 2, Reason: "并发核对", Record: ownInput}, audit.Metadata{})
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	good, conflicts := 0, 0
	for e := range results {
		if e == nil {
			good++
		} else if errors.Is(e, inspection.ErrRecordVersionConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if good != 1 || conflicts != 1 {
		t.Fatal("lost update", good, conflicts)
	}
	// Audit storage failure must roll back the record write.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION fail_record_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture audit failure'; END $$; CREATE TRIGGER fail_record_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_record_audit();`); err != nil {
		t.Fatal(err)
	}
	failedInput := makeInput(inspection.InspectionTypeMorning, map[string]any{"date": "2026-09-05"})
	if _, e := svc.Update(ctx, admin, own.ID, inspection.InspectionEdit{Version: 3, Reason: "核对", Record: ownInput}, audit.Metadata{}); e == nil {
		t.Fatal("audit failure accepted update")
	}
	if e := svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 3}}, Reason: "核对"}, audit.Metadata{}); e == nil {
		t.Fatal("audit failure accepted delete")
	}
	if r, e := svc.Detail(ctx, admin, own.ID); e != nil || r.RowVersion != 3 {
		t.Fatal("audit rollback changed record", e)
	}
	if _, err = svc.Create(ctx, admin, failedInput, audit.Metadata{}); err == nil {
		t.Fatal("audit failure accepted")
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER fail_record_audit ON audit_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Create(ctx, admin, failedInput, audit.Metadata{}); err != nil {
		t.Fatal("audit failure left duplicate", err)
	}
	for _, column := range []string{"status='CONFIRMED'", "status='RETURNED'", "status='LOCKED'", "status='DRAFT',submission_id=gen_random_uuid()", "status='DRAFT',submission_id=NULL,locked_by_report_id=gen_random_uuid()"} {
		if _, err = pool.Exec(ctx, "UPDATE inspection_records SET "+column+" WHERE id=$1", own.ID); err != nil {
			t.Fatal(err)
		}
		if err = svc.Delete(ctx, admin, inspection.DeleteAttendanceInput{Items: []inspection.VersionedRecord{{ID: own.ID, Version: 3}}, Reason: "核对"}, audit.Metadata{}); !errors.Is(err, inspection.ErrRecordNotEditable) {
			t.Fatal("protected record", err)
		}
	}
}
