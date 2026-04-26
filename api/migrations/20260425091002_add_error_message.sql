-- +goose Up
ALTER TABLE deployments
    ADD COLUMN error_message TEXT;

-- +goose Down
ALTER TABLE deployments
    DROP COLUMN error_message;
