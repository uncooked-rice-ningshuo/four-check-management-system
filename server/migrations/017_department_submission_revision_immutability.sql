-- A submitted revision is historical evidence for review and later reports.
-- Corrections create a new revision; no application path may rewrite or remove
-- a previous record set or snapshot.
CREATE FUNCTION prevent_department_submission_revision_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'department submission revisions are append-only' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER department_submission_revisions_append_only
    BEFORE UPDATE OR DELETE ON department_submission_revisions
    FOR EACH ROW EXECUTE FUNCTION prevent_department_submission_revision_mutation();
