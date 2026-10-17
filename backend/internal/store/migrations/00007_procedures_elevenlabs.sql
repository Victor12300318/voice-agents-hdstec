-- +goose Up
ALTER TABLE procedures ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT 'free_form';

-- +goose Down
ALTER TABLE procedures DROP COLUMN IF EXISTS type;
