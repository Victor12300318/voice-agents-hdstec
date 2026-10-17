-- +goose Up
-- Conexões Gemini Live duram ~10 min; sem session resumption o limite seguro é 540 s.
ALTER TABLE agents ALTER COLUMN max_duration_seconds SET DEFAULT 540;
UPDATE agents SET max_duration_seconds = 540 WHERE max_duration_seconds > 540;

-- +goose Down
ALTER TABLE agents ALTER COLUMN max_duration_seconds SET DEFAULT 600;
