-- All values are synthetic. This fixture is written only to an empty disposable DB.
INSERT INTO accounts(id,login_name,name,password_hash,status,account_kind,system_role,is_super_admin,must_change_password)
VALUES('a0000000-0000-0000-0000-000000000001','extension-fixture','Schema fixture','not-a-login-hash','ACTIVE','SYSTEM','MEMBER',false,false);
INSERT INTO academic_years(id,name,start_date,end_date) VALUES('a0000000-0000-0000-0000-000000000002','Fixture year','2026-01-01','2026-12-31');
INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date) VALUES('a0000000-0000-0000-0000-000000000003','a0000000-0000-0000-0000-000000000002','FIRST','Fixture semester','2026-01-01','2026-12-31');
INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date) VALUES('a0000000-0000-0000-0000-000000000004','a0000000-0000-0000-0000-000000000003',1,'2026-09-01','2026-09-07');
INSERT INTO grades(id,name) VALUES('a0000000-0000-0000-0000-000000000005','2026');
INSERT INTO classes(id,name,grade_id) VALUES('a0000000-0000-0000-0000-000000000006','Fixture class','a0000000-0000-0000-0000-000000000005'),('a0000000-0000-0000-0000-000000000016','Other fixture class','a0000000-0000-0000-0000-000000000005');
INSERT INTO students(id,student_no,name,grade_id,class_id) VALUES('a0000000-0000-0000-0000-000000000007','S-FIXTURE','Synthetic student','a0000000-0000-0000-0000-000000000005','a0000000-0000-0000-0000-000000000006');
INSERT INTO teaching_buildings(id,name) VALUES('a0000000-0000-0000-0000-000000000008','Fixture teaching building');
INSERT INTO classrooms(id,teaching_building_id,room_no) VALUES('a0000000-0000-0000-0000-000000000009','a0000000-0000-0000-0000-000000000008','101'),('a0000000-0000-0000-0000-000000000019','a0000000-0000-0000-0000-000000000008','102');
INSERT INTO dormitory_areas(id,name) VALUES('a0000000-0000-0000-0000-000000000010','Fixture dormitory area');
INSERT INTO dormitory_buildings(id,dormitory_area_id,name) VALUES('a0000000-0000-0000-0000-000000000011','a0000000-0000-0000-0000-000000000010','Fixture dormitory building');
INSERT INTO dormitories(id,dormitory_building_id,room_no) VALUES('a0000000-0000-0000-0000-000000000012','a0000000-0000-0000-0000-000000000011','101');
INSERT INTO courses(id,name) VALUES('a0000000-0000-0000-0000-000000000013','Fixture course');
INSERT INTO inspection_records(id,inspection_type_id,semester_id,week_id,department_id,occurred_at,source,dedup_key,created_by,remark)
SELECT 'a0000000-0000-0000-0000-000000000014',id,'a0000000-0000-0000-0000-000000000003','a0000000-0000-0000-0000-000000000004','10000000-0000-0000-0000-000000000002','2026-09-02 08:00+08','MANUAL',repeat('a',64),'a0000000-0000-0000-0000-000000000001','migration-preservation-fixture'
FROM inspection_types WHERE code='ATTENDANCE_RATE';
INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id)
VALUES('a0000000-0000-0000-0000-000000000014','a0000000-0000-0000-0000-000000000013',2,'1-2','a0000000-0000-0000-0000-000000000009');
