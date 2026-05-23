CREATE TABLE vcs_provider_binding (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    provider     TEXT NOT NULL,
    base_url     TEXT NOT NULL,
    token_encrypted BYTEA NOT NULL,
    display_name TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON vcs_provider_binding (workspace_id);
