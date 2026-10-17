-- +goose Up

ALTER TABLE agents ADD COLUMN IF NOT EXISTS versioning_enabled boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS agent_branches (
    id                    text PRIMARY KEY DEFAULT ('agtbrch_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 12)),
    organization_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id              uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name                  text NOT NULL,
    description           text NOT NULL DEFAULT '',
    is_main               boolean NOT NULL DEFAULT false,
    archived              boolean NOT NULL DEFAULT false,
    traffic_percentage    integer NOT NULL DEFAULT 0,
    parent_version_id     uuid REFERENCES agent_versions(id) ON DELETE SET NULL,
    head_version_id       uuid REFERENCES agent_versions(id) ON DELETE SET NULL,
    base_main_version_id  uuid REFERENCES agent_versions(id) ON DELETE SET NULL,
    merged_into_branch_id text,
    merged_version_id     uuid REFERENCES agent_versions(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, name)
);
CREATE INDEX IF NOT EXISTS agent_branches_agent_idx ON agent_branches (agent_id);

ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS version_code text NOT NULL DEFAULT ('agtvrsn_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 12));
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS branch_id text REFERENCES agent_branches(id) ON DELETE SET NULL;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS parent_version_id uuid REFERENCES agent_versions(id) ON DELETE SET NULL;
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS commit_message text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS agent_versions_code_idx ON agent_versions (version_code);
CREATE INDEX IF NOT EXISTS agent_versions_branch_idx ON agent_versions (branch_id, version DESC);

CREATE TABLE IF NOT EXISTS agent_branch_drafts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    branch_id       text NOT NULL REFERENCES agent_branches(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    snapshot        jsonb NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, branch_id, user_id)
);
CREATE INDEX IF NOT EXISTS agent_branch_drafts_lookup_idx ON agent_branch_drafts (agent_id, branch_id, user_id);

ALTER TABLE conversations ADD COLUMN IF NOT EXISTS branch_id text REFERENCES agent_branches(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS branch_id;
DROP TABLE IF EXISTS agent_branch_drafts;
DROP INDEX IF EXISTS agent_versions_branch_idx;
DROP INDEX IF EXISTS agent_versions_code_idx;
ALTER TABLE agent_versions DROP COLUMN IF EXISTS commit_message;
ALTER TABLE agent_versions DROP COLUMN IF EXISTS parent_version_id;
ALTER TABLE agent_versions DROP COLUMN IF EXISTS branch_id;
ALTER TABLE agent_versions DROP COLUMN IF EXISTS version_code;
DROP TABLE IF EXISTS agent_branches;
ALTER TABLE agents DROP COLUMN IF EXISTS versioning_enabled;
