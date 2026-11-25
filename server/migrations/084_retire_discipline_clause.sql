-- Serialize the cutover with rule publication and report finalization.
SELECT pg_advisory_xact_lock(74077);
DELETE FROM discipline_rule_current;
ALTER TABLE discipline_rule_versions ADD COLUMN legacy_basis_retired boolean NOT NULL DEFAULT false;
UPDATE discipline_rule_versions SET status='SUPERSEDED', legacy_basis_retired=true, row_version=row_version+1
WHERE status IN ('PUBLISHED','SUPERSEDED');
DELETE FROM discipline_tier_template_bindings;
DELETE FROM discipline_document_template_bindings;
ALTER TABLE discipline_rule_tiers DROP COLUMN clause;
