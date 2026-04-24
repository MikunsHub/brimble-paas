-- +goose Up
-- +goose StatementBegin

CREATE TABLE deployments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_url         TEXT,
    s3_key          TEXT,
    subdomain       TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'pending',
    image_tag       TEXT,
    container_id    TEXT,
    container_addr  TEXT,
    live_url        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deployment_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id   UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    stream          TEXT NOT NULL,
    phase           TEXT NOT NULL,
    content         TEXT NOT NULL
);

CREATE INDEX idx_deployment_logs_deployment_id ON deployment_logs(deployment_id);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_deployments_updated_at
    BEFORE UPDATE ON deployments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS trg_deployments_updated_at ON deployments;
DROP FUNCTION IF EXISTS set_updated_at;
DROP TABLE IF EXISTS deployment_logs;
DROP TABLE IF EXISTS deployments;

-- +goose StatementEnd
