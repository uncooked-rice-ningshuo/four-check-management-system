-- An absent roster means the academic year still needs manual configuration.
CREATE TABLE academic_year_rosters (
    academic_year_id uuid PRIMARY KEY REFERENCES academic_years(id),
    version integer NOT NULL CHECK (version > 0)
);

CREATE TABLE academic_year_roster_grades (
    academic_year_id uuid NOT NULL REFERENCES academic_year_rosters,
    grade_id uuid NOT NULL REFERENCES grades,
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    PRIMARY KEY (academic_year_id, grade_id),
    UNIQUE (academic_year_id, sort_order)
);

-- Major order is shared across cohorts, while membership is cohort-specific.
CREATE TABLE academic_year_roster_majors (
    academic_year_id uuid NOT NULL REFERENCES academic_year_rosters,
    major_id uuid NOT NULL REFERENCES majors,
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    PRIMARY KEY (academic_year_id, major_id),
    UNIQUE (academic_year_id, sort_order)
);

CREATE TABLE academic_year_roster_grade_majors (
    academic_year_id uuid NOT NULL,
    grade_id uuid NOT NULL,
    major_id uuid NOT NULL,
    PRIMARY KEY (academic_year_id, grade_id, major_id),
    FOREIGN KEY (academic_year_id, grade_id) REFERENCES academic_year_roster_grades,
    FOREIGN KEY (academic_year_id, major_id) REFERENCES academic_year_roster_majors
);

CREATE TABLE academic_year_roster_classes (
    academic_year_id uuid NOT NULL,
    grade_id uuid NOT NULL,
    major_id uuid NOT NULL,
    class_id uuid NOT NULL REFERENCES classes,
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    PRIMARY KEY (academic_year_id, class_id),
    UNIQUE (academic_year_id, grade_id, major_id, sort_order),
    FOREIGN KEY (academic_year_id, grade_id, major_id) REFERENCES academic_year_roster_grade_majors
);

-- Preserve the historical coverage until staff configure a year explicitly.
CREATE VIEW academic_report_grades AS
SELECT academic_year_id, grade_id, sort_order FROM academic_year_roster_grades
UNION ALL
SELECT s.academic_year_id, s.grade_id, 0 FROM academic_year_grade_stages s
WHERE NOT EXISTS (SELECT 1 FROM academic_year_rosters r WHERE r.academic_year_id=s.academic_year_id);

CREATE VIEW academic_report_classes AS
SELECT c.academic_year_id,c.grade_id,c.class_id,m.sort_order AS major_order,c.sort_order AS class_order
FROM academic_year_roster_classes c JOIN academic_year_roster_majors m USING(academic_year_id,major_id)
UNION ALL
SELECT s.academic_year_id,s.grade_id,c.id,0,0 FROM academic_year_grade_stages s JOIN classes c ON c.grade_id=s.grade_id
WHERE NOT EXISTS (SELECT 1 FROM academic_year_rosters r WHERE r.academic_year_id=s.academic_year_id);

CREATE FUNCTION academic_roster_allows_class(year_id uuid, candidate_id uuid, candidate_grade uuid, candidate_major uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS (SELECT 1 FROM academic_year_rosters WHERE academic_year_id=year_id)
 OR EXISTS (SELECT 1 FROM academic_year_roster_classes
            WHERE academic_year_id=year_id AND class_id=candidate_id
              AND grade_id=candidate_grade AND major_id=candidate_major)
$$;

-- Include assigned snapshots because later department edits never rewrite them.
CREATE VIEW legacy_stage_grant_inventory AS
SELECT 'PERSONAL'::text AS source, r.id AS source_id, r.account_id, r.source_department_id AS department_id,
       r.action, r.inspection_type_id, r.starts_at, r.ends_at, clause AS range_clause, NULL::text AS role_code
FROM user_permission_rules r CROSS JOIN LATERAL jsonb_array_elements(NULLIF(r.operation_ranges,'null'::jsonb)) clause
WHERE r.enabled AND (r.ends_at IS NULL OR r.ends_at>now()) AND clause->>'gradeStage' IN ('1','2','3','4')
UNION ALL
SELECT p.source,p.source_id,p.account_id,p.department_id,action->>'action',
       NULLIF(action->>'inspectionTypeId','')::uuid,p.starts_at,p.ends_at,clause,p.policy->>'roleCode'
FROM (
 SELECT 'TEMPLATE'::text AS source,d.id AS source_id,NULL::uuid AS account_id,d.id AS department_id,
        NULL::timestamptz AS starts_at,NULL::timestamptz AS ends_at,policy
 FROM departments d CROSS JOIN LATERAL jsonb_array_elements(NULLIF(d.role_policies,'null'::jsonb)) policy
 UNION ALL
 SELECT 'ASSIGNMENT',u.id,u.account_id,u.department_id,u.start_at,u.end_at,policy
 FROM user_org_roles u JOIN organization_roles role ON role.id=u.organization_role_id
 CROSS JOIN LATERAL jsonb_array_elements(NULLIF(u.default_permissions->'rolePolicies','null'::jsonb)) policy
 WHERE policy->>'roleCode'=role.code AND (u.end_at IS NULL OR u.end_at>now())
) p CROSS JOIN LATERAL jsonb_array_elements(NULLIF(p.policy->'operationRanges','null'::jsonb)) action
CROSS JOIN LATERAL jsonb_array_elements(NULLIF(action->'ranges','null'::jsonb)) clause
WHERE clause->>'gradeStage' IN ('1','2','3','4');
