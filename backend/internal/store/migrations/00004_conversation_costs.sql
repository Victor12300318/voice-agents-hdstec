-- +goose Up
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS cost_usd numeric(10, 6) NOT NULL DEFAULT 0;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS cost_brl numeric(10, 4) NOT NULL DEFAULT 0;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS tokens_usage jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS cost_usd;
ALTER TABLE conversations DROP COLUMN IF EXISTS cost_brl;
ALTER TABLE conversations DROP COLUMN IF EXISTS tokens_usage;
