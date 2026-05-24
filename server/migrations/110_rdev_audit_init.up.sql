CREATE TABLE audit_event (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL,
    actor_type     TEXT NOT NULL,
    actor_id       UUID,
    action         TEXT NOT NULL,
    resource_type  TEXT,
    resource_id    TEXT,
    client_ip      TEXT,
    correlation_id UUID,
    metadata       JSONB NOT NULL DEFAULT '{}',
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON audit_event (workspace_id, occurred_at DESC);
CREATE INDEX ON audit_event (workspace_id, action);
