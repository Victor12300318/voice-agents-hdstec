-- +goose Up
ALTER TABLE agents ADD COLUMN IF NOT EXISTS system_tools_config jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE agents DROP COLUMN IF EXISTS system_tools_config;
