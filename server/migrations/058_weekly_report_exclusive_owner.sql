-- Fail closed on conflicting legacy configuration; never silently choose an owner.
DO $$ BEGIN
 IF (SELECT count(*) FROM department_report_scopes WHERE enabled) <> 1
 OR NOT EXISTS (SELECT 1 FROM department_report_scopes s JOIN departments d ON d.id=s.department_id WHERE s.enabled AND d.enabled AND d.code='OFFICE') THEN
  RAISE EXCEPTION 'weekly report maintenance configuration requires explicit reconciliation';
 END IF;
END $$;
CREATE UNIQUE INDEX department_report_scopes_one_enabled ON department_report_scopes ((1)) WHERE enabled;
-- Existing personal grants must be recalculated after the new qualification gate.
UPDATE accounts SET session_version=session_version+1 WHERE id IN
 (SELECT account_id FROM user_permission_rules WHERE enabled AND effect='ALLOW' AND action IN ('REPORT_GENERATE','REPORT_CORRECT','REPORT_REVIEW','REPORT_RETURN','REPORT_FINALIZE','REPORT_PUBLISH','REPORT_OUTPUT_SETTINGS_MANAGE'));
UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE revoked_at IS NULL AND account_id IN
 (SELECT account_id FROM user_permission_rules WHERE enabled AND effect='ALLOW' AND action IN ('REPORT_GENERATE','REPORT_CORRECT','REPORT_REVIEW','REPORT_RETURN','REPORT_FINALIZE','REPORT_PUBLISH','REPORT_OUTPUT_SETTINGS_MANAGE'));
