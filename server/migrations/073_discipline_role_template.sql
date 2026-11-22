ALTER TABLE discipline_cases DROP CONSTRAINT discipline_cases_decision_source_check;
ALTER TABLE discipline_cases ADD CONSTRAINT discipline_cases_decision_source_check
 CHECK (decision_source IN ('TEACHER','AUTHORIZED_USER','SUPER_ADMIN','TIMEOUT','RULE'));

-- Initialize editable templates only. Existing assignments retain their granted snapshot.
UPDATE departments d SET role_policies=(
 SELECT jsonb_agg(p || jsonb_build_object('canConfirmDiscipline',p->>'roleCode'='TEACHER') ORDER BY ord)
 FROM jsonb_array_elements(d.role_policies) WITH ORDINALITY AS policies(p,ord)
),permission_version=permission_version+1
WHERE jsonb_array_length(role_policies)>0;
