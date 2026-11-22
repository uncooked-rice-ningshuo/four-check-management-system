-- Issuance wording belongs to a published rule revision; existing decisions
-- retain their original evidence, including historically missing values.
ALTER TABLE discipline_rule_groups ADD COLUMN violation_name varchar(120) NOT NULL DEFAULT '';
ALTER TABLE discipline_rule_tiers ADD COLUMN authority_number varchar(120) NOT NULL DEFAULT '',
    ADD COLUMN clause_reference varchar(120) NOT NULL DEFAULT '';

-- Keep every edition that actually applied to a tier, including editions
-- inherited from a preceding rule before a later local edit.
CREATE TABLE discipline_tier_template_history (
    rule_id uuid NOT NULL,
    group_key text NOT NULL,
    tier_code text NOT NULL,
    template_id uuid NOT NULL,
    PRIMARY KEY(rule_id,group_key,tier_code,template_id),
    FOREIGN KEY(rule_id,group_key) REFERENCES discipline_rule_groups(version_id,group_key),
    FOREIGN KEY(group_key,tier_code,template_id) REFERENCES discipline_document_templates(group_key,tier_code,id)
);
INSERT INTO discipline_tier_template_history
SELECT rule_id,group_key,tier_code,template_id FROM discipline_tier_template_bindings
UNION
SELECT source_rule_id,group_key,tier_code,id FROM discipline_document_templates WHERE tier_code IS NOT NULL;
CREATE FUNCTION record_discipline_template_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO discipline_tier_template_history VALUES(NEW.rule_id,NEW.group_key,NEW.tier_code,NEW.template_id)
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END
$$;
CREATE TRIGGER discipline_template_binding_history AFTER INSERT OR UPDATE OF template_id
ON discipline_tier_template_bindings FOR EACH ROW EXECUTE FUNCTION record_discipline_template_binding();
