-- +goose Up
ALTER TABLE phone_numbers DROP CONSTRAINT IF EXISTS phone_numbers_agent_id_fkey;
ALTER TABLE phone_numbers ALTER COLUMN agent_id DROP NOT NULL;
ALTER TABLE phone_numbers
    ADD CONSTRAINT phone_numbers_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE phone_numbers DROP CONSTRAINT IF EXISTS phone_numbers_agent_id_fkey;
DELETE FROM phone_numbers WHERE agent_id IS NULL;
ALTER TABLE phone_numbers ALTER COLUMN agent_id SET NOT NULL;
ALTER TABLE phone_numbers
    ADD CONSTRAINT phone_numbers_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
