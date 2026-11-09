-- Preserve entry attribution, including historical drafts. Only newly attached
-- rows get an explicit route. A null routed department selects the admin channel.
ALTER TABLE inspection_records
 ADD COLUMN submission_routed boolean NOT NULL DEFAULT false,
 ADD COLUMN routed_department_id uuid REFERENCES departments(id),
 ADD CONSTRAINT inspection_routed_department_check CHECK (
   (submission_routed OR routed_department_id IS NULL) AND
   (NOT submission_routed OR submission_id IS NOT NULL)),
 ADD COLUMN submission_department_key uuid GENERATED ALWAYS AS
 (COALESCE(CASE WHEN submission_routed THEN routed_department_id ELSE department_id END,
 '00000000-0000-0000-0000-000000000000'::uuid)) STORED;
ALTER TABLE inspection_records DROP CONSTRAINT inspection_records_submission_scope_fkey,
 ADD CONSTRAINT inspection_records_submission_scope_fkey FOREIGN KEY
 (submission_id,semester_id,week_id,submission_department_key,inspection_type_id)
 REFERENCES department_submissions(id,semester_id,week_id,scope_department_key,inspection_type_id);
-- Existing null-department submissions retain their historical college meaning.
ALTER TABLE department_submissions ADD COLUMN administrator_channel boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT administrator_channel_department_check CHECK (NOT administrator_channel OR department_id IS NULL);
