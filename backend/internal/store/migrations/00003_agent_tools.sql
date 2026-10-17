-- +goose Up
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tools jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE agents DROP COLUMN IF EXISTS tools;
