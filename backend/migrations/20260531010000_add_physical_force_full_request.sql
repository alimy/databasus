-- +goose Up
ALTER TABLE physical_backup_configs
    ADD COLUMN force_full_requested_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE physical_backup_configs
    DROP COLUMN force_full_requested_at;
