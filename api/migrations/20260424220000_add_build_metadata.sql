-- +goose Up
ALTER TABLE deployments
    ADD COLUMN detected_lang TEXT,
    ADD COLUMN start_cmd     TEXT;

-- +goose Down
ALTER TABLE deployments
    DROP COLUMN detected_lang,
    DROP COLUMN start_cmd;
