CREATE TABLE IF NOT EXISTS gateway_token (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    runtime_id   UUID,
    token_hash   BYTEA NOT NULL,
    label        TEXT,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS gateway_token_hash_idx ON gateway_token (token_hash);
