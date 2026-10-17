-- +goose Up
ALTER TABLE phone_numbers ADD COLUMN IF NOT EXISTS provider text NOT NULL DEFAULT 'jambonz';
ALTER TABLE phone_numbers ADD COLUMN IF NOT EXISTS twilio_account_sid text NOT NULL DEFAULT '';
ALTER TABLE phone_numbers ADD COLUMN IF NOT EXISTS twilio_auth_token text NOT NULL DEFAULT '';
ALTER TABLE phone_numbers ADD COLUMN IF NOT EXISTS twilio_phone_sid text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE phone_numbers DROP COLUMN IF EXISTS twilio_phone_sid;
ALTER TABLE phone_numbers DROP COLUMN IF EXISTS twilio_auth_token;
ALTER TABLE phone_numbers DROP COLUMN IF EXISTS twilio_account_sid;
ALTER TABLE phone_numbers DROP COLUMN IF EXISTS provider;
