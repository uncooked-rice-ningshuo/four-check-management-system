-- Keep previous group-level releases as read-only sources. An administrator
-- must adapt one to a specific tier before it can generate new documents.
ALTER TABLE discipline_document_templates
    ADD COLUMN tier_code text,
    ADD COLUMN source_template_id uuid REFERENCES discipline_document_templates(id),
    DROP CONSTRAINT discipline_document_templates_group_key_revision_key,
    ADD UNIQUE NULLS NOT DISTINCT (group_key,tier_code,revision),
    ADD UNIQUE (group_key,tier_code,id);

CREATE TABLE discipline_tier_template_bindings (
    rule_id uuid NOT NULL,
    group_key text NOT NULL,
    tier_code text NOT NULL,
    template_id uuid NOT NULL,
    PRIMARY KEY(rule_id,group_key,tier_code),
    FOREIGN KEY(rule_id,group_key) REFERENCES discipline_rule_groups(version_id,group_key),
    FOREIGN KEY(group_key,tier_code,template_id) REFERENCES discipline_document_templates(group_key,tier_code,id)
);
