-- +goose Up
CREATE TABLE IF NOT EXISTS phone_numbers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    e164            text NOT NULL,
    label           text NOT NULL DEFAULT '',
    agent_id        uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    direction       text NOT NULL DEFAULT 'inbound', -- inbound | outbound | both
    active          boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT phone_numbers_org_e164_uniq UNIQUE (organization_id, e164)
);

CREATE INDEX IF NOT EXISTS phone_numbers_e164_idx ON phone_numbers (e164);
CREATE INDEX IF NOT EXISTS phone_numbers_org_idx ON phone_numbers (organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS phone_numbers_agent_idx ON phone_numbers (agent_id);

ALTER TABLE conversations ADD COLUMN IF NOT EXISTS direction text NOT NULL DEFAULT 'web';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS from_number text NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS to_number text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS direction;
ALTER TABLE conversations DROP COLUMN IF EXISTS from_number;
ALTER TABLE conversations DROP COLUMN IF EXISTS to_number;

DROP TABLE IF EXISTS phone_numbers;
