CREATE TABLE discipline_rule_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version integer NOT NULL UNIQUE CHECK (version > 0),
    status text NOT NULL CHECK (status IN ('DRAFT', 'PUBLISHED', 'SUPERSEDED')),
    row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    published_by uuid REFERENCES accounts(id),
    published_at timestamptz,
    CHECK ((status = 'DRAFT' AND published_by IS NULL AND published_at IS NULL)
        OR (status IN ('PUBLISHED', 'SUPERSEDED') AND published_by IS NOT NULL AND published_at IS NOT NULL))
);

CREATE UNIQUE INDEX discipline_rule_one_draft_idx
    ON discipline_rule_versions ((status)) WHERE status = 'DRAFT';

CREATE TABLE discipline_rule_current (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    version_id uuid NOT NULL UNIQUE REFERENCES discipline_rule_versions(id),
    row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0)
);

CREATE TABLE discipline_rule_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES discipline_rule_versions(id) ON DELETE CASCADE,
    group_key text NOT NULL,
    name text NOT NULL,
    result_unit text NOT NULL DEFAULT '',
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    UNIQUE (version_id, group_key),
    UNIQUE (version_id, name),
    UNIQUE (id, version_id)
);

CREATE TABLE discipline_rule_group_terms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES discipline_rule_groups(id) ON DELETE CASCADE,
    version_id uuid NOT NULL REFERENCES discipline_rule_versions(id) ON DELETE CASCADE,
    inspection_type_id uuid NOT NULL REFERENCES inspection_types(id),
    inspection_type_name text NOT NULL,
    source_unit text NOT NULL DEFAULT '',
    operator text NOT NULL CHECK (operator IN ('+', '-')),
    conversion_factor numeric(12, 6) NOT NULL CHECK (conversion_factor > 0),
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    UNIQUE (group_id, inspection_type_id),
    UNIQUE (version_id, inspection_type_id),
    FOREIGN KEY (group_id, version_id) REFERENCES discipline_rule_groups(id, version_id) ON DELETE CASCADE
);

CREATE TABLE discipline_rule_tiers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES discipline_rule_groups(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    lower_bound numeric(12, 4) NOT NULL CHECK (lower_bound >= 0),
    upper_bound numeric(12, 4),
    clause text NOT NULL DEFAULT '',
    teacher_confirmation_required boolean NOT NULL,
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    CHECK (upper_bound IS NULL OR upper_bound > lower_bound),
    UNIQUE (group_id, code),
    UNIQUE (group_id, sort_order)
);

CREATE INDEX discipline_rule_versions_history_idx ON discipline_rule_versions (created_at DESC, version DESC);
