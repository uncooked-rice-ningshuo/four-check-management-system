BEGIN;
CREATE FUNCTION pg_temp.expect_failure(statement text, expected text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE rejected boolean:=false;
BEGIN
 BEGIN
  EXECUTE statement;
  SET CONSTRAINTS ALL IMMEDIATE;
 EXCEPTION WHEN OTHERS THEN
  IF SQLSTATE <> expected THEN RAISE EXCEPTION 'expected %, got %: %', expected, SQLSTATE, SQLERRM; END IF;
  rejected:=true;
 END;
 IF NOT rejected THEN RAISE EXCEPTION 'invalid statement accepted: %',statement; END IF;
 SET CONSTRAINTS ALL DEFERRED;
END $$;
CREATE FUNCTION pg_temp.new_record(code text, kind text, event_key text, n integer) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE rid uuid:=gen_random_uuid();
BEGIN
 INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,occurred_at,occurrence_kind,inspection_date,inspection_round,source,dedup_key,created_by,violation_count)
 SELECT rid,id,'a0000000-0000-0000-0000-000000000003','a0000000-0000-0000-0000-000000000004','10000000-0000-0000-0000-000000000002',CASE WHEN kind='TIME' THEN '2026-09-02 08:00+08'::timestamptz END,kind,CASE WHEN kind<>'TIME' THEN '2026-09-02'::date END,CASE WHEN kind='ROUND' THEN 'first' END,'MANUAL',md5(event_key)||md5(event_key),'a0000000-0000-0000-0000-000000000001',n FROM inspection_types WHERE inspection_types.code=new_record.code;
 RETURN rid;
END $$;
CREATE TEMP TABLE fixture_records(name text PRIMARY KEY,id uuid);
INSERT INTO fixture_records VALUES
 ('morning',pg_temp.new_record('MORNING_ATTENDANCE','DATE','morning',1)),
 ('late',pg_temp.new_record('LATE_RETURN','TIME','late',1)),
 ('dorm',pg_temp.new_record('DORMITORY_HYGIENE','ROUND','dorm',1)),
 ('classroom',pg_temp.new_record('CLASSROOM_HYGIENE','TIME','classroom',0)),
 ('public',pg_temp.new_record('PUBLIC_DORMITORY_HYGIENE','ROUND','public',0)),
 ('unattended',pg_temp.new_record('UNATTENDED_CLASSROOM','TIME','unattended',1));
INSERT INTO classroom_hygiene_areas(id,classroom_id,name) VALUES('b0000000-0000-0000-0000-000000000001','a0000000-0000-0000-0000-000000000009','Floor');
INSERT INTO public_dormitory_hygiene_areas(id,dormitory_building_id,floor_unit,name) VALUES('b0000000-0000-0000-0000-000000000002','a0000000-0000-0000-0000-000000000011','1','Corridor');
INSERT INTO morning_exercise_details SELECT id,1 FROM fixture_records WHERE name='morning';
INSERT INTO late_return_details SELECT id,'a0000000-0000-0000-0000-000000000012' FROM fixture_records WHERE name='late';
INSERT INTO dorm_hygiene_details(record_id,dormitory_id,result,reason_code) SELECT id,'a0000000-0000-0000-0000-000000000012','UNQUALIFIED','FLOOR' FROM fixture_records WHERE name='dorm';
INSERT INTO classroom_hygiene_details(record_id,classroom_id,area_id,result) SELECT id,'a0000000-0000-0000-0000-000000000009','b0000000-0000-0000-0000-000000000001','QUALIFIED' FROM fixture_records WHERE name='classroom';
INSERT INTO public_dormitory_hygiene_details(record_id,area_id,result) SELECT id,'b0000000-0000-0000-0000-000000000002','EXCELLENT' FROM fixture_records WHERE name='public';
INSERT INTO inspection_responsibility_classes SELECT id,'a0000000-0000-0000-0000-000000000006','Fixture class','2026' FROM fixture_records WHERE name IN ('dorm','classroom','public');
INSERT INTO inspection_responsibility_students SELECT id,'a0000000-0000-0000-0000-000000000007','a0000000-0000-0000-0000-000000000006','S-FIXTURE','Synthetic student' FROM fixture_records WHERE name='classroom';
INSERT INTO inspection_responsibility_dormitories SELECT id,'a0000000-0000-0000-0000-000000000012' FROM fixture_records WHERE name='public';
INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id) SELECT id,'a0000000-0000-0000-0000-000000000013',2,'1-2','a0000000-0000-0000-0000-000000000009' FROM fixture_records WHERE name='unattended';
SET CONSTRAINTS ALL IMMEDIATE;
SET CONSTRAINTS ALL DEFERRED;
SELECT pg_temp.expect_failure($q$SELECT pg_temp.new_record('MORNING_ATTENDANCE','TIME','bad',1)$q$,'23514');
SELECT pg_temp.expect_failure($q$SELECT pg_temp.new_record('LATE_RETURN','TIME','bad',2)$q$,'23514');
SELECT pg_temp.expect_failure($q$SELECT pg_temp.new_record('DORMITORY_HYGIENE','DATE','bad',1)$q$,'23514');
SELECT pg_temp.expect_failure($q$SELECT pg_temp.new_record('MORNING_ATTENDANCE','DATE','morning',1)$q$,'23505');
SELECT pg_temp.expect_failure($q$UPDATE inspection_records SET inspection_date='2026-09-02' WHERE id=(SELECT id FROM fixture_records WHERE name='late')$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE inspection_records SET inspection_round=' ' WHERE id=(SELECT id FROM fixture_records WHERE name='dorm')$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE inspection_records SET class_id='a0000000-0000-0000-0000-000000000006' WHERE id=(SELECT id FROM fixture_records WHERE name='dorm')$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE morning_exercise_details SET absence_count=2$q$,'23514');
SELECT pg_temp.expect_failure($q$INSERT INTO morning_exercise_details SELECT id,1 FROM fixture_records WHERE name='late'$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE dorm_hygiene_details SET reason_code=NULL$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE dorm_hygiene_details SET result='OTHER'$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE classroom_hygiene_details SET classroom_id='a0000000-0000-0000-0000-000000000019'$q$,'23503');
SELECT pg_temp.expect_failure($q$UPDATE inspection_responsibility_students SET class_id='a0000000-0000-0000-0000-000000000016'$q$,'23503');
SELECT pg_temp.expect_failure($q$DELETE FROM inspection_responsibility_classes WHERE record_id=(SELECT id FROM fixture_records WHERE name='dorm')$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE inspection_records SET violation_count=0 WHERE id=(SELECT id FROM fixture_records WHERE name='dorm')$q$,'23514');
SELECT pg_temp.expect_failure($q$UPDATE inspection_records SET inspection_type_id=(SELECT id FROM inspection_types WHERE code='LATE_RETURN') WHERE id=(SELECT id FROM fixture_records WHERE name='unattended')$q$,'23514');
SELECT pg_temp.expect_failure($q$DELETE FROM inspection_records WHERE id=(SELECT id FROM fixture_records WHERE name='morning')$q$,'23503');
SELECT pg_temp.expect_failure($q$DELETE FROM classroom_hygiene_areas$q$,'23503');
-- Both update orders are legal when the committed aggregate is consistent.
UPDATE inspection_records SET violation_count=0 WHERE id=(SELECT id FROM fixture_records WHERE name='dorm');
UPDATE dorm_hygiene_details SET result='QUALIFIED';
SET CONSTRAINTS ALL IMMEDIATE;
SET CONSTRAINTS ALL DEFERRED;
UPDATE dorm_hygiene_details SET result='UNQUALIFIED';
UPDATE inspection_records SET violation_count=1 WHERE id=(SELECT id FROM fixture_records WHERE name='dorm');
SET CONSTRAINTS ALL IMMEDIATE;
SET CONSTRAINTS ALL DEFERRED;
UPDATE inspection_records SET deleted_at=now() WHERE id=(SELECT id FROM fixture_records WHERE name='dorm');
SELECT pg_temp.new_record('DORMITORY_HYGIENE','ROUND','dorm',1);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM dorm_hygiene_details d JOIN inspection_responsibility_classes c USING(record_id) JOIN inspection_records r ON r.id=d.record_id WHERE r.deleted_at IS NOT NULL) THEN RAISE EXCEPTION 'soft delete lost history'; END IF;
END $$;
COMMIT;
