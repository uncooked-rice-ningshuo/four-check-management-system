-- Singleton global work-week override. Expiry does not rewrite calendar or historical records.
CREATE TABLE system_settings (
 key varchar(64) PRIMARY KEY,
 value_json jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(value_json)='object'),
 row_version integer NOT NULL DEFAULT 1 CHECK (row_version>0),
 updated_by uuid REFERENCES accounts(id),
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO system_settings(key) VALUES('work_week_override');
