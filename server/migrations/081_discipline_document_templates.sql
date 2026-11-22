-- Template revisions are immutable. Publication can bind the same revision
-- to a successor rule group while keeping the old rule's binding intact.
CREATE TABLE discipline_document_templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_key text NOT NULL,
    source_rule_id uuid NOT NULL,
    revision integer NOT NULL CHECK (revision > 0),
    document jsonb NOT NULL CHECK (jsonb_typeof(document) = 'object' AND document @> '{"schemaVersion":2}'),
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (source_rule_id, group_key) REFERENCES discipline_rule_groups(version_id, group_key),
    UNIQUE (group_key, revision),
    UNIQUE (group_key, id)
);

CREATE TABLE discipline_document_template_bindings (
    rule_id uuid NOT NULL,
    group_key text NOT NULL,
    template_id uuid NOT NULL,
    PRIMARY KEY (rule_id, group_key),
    FOREIGN KEY (rule_id, group_key) REFERENCES discipline_rule_groups(version_id, group_key),
    FOREIGN KEY (group_key, template_id) REFERENCES discipline_document_templates(group_key, id)
);
