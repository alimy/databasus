-- +goose Up
-- +goose StatementBegin

ALTER TABLE physical_full_backups ADD COLUMN compression TEXT NOT NULL DEFAULT 'ZSTD';
ALTER TABLE physical_full_backups ADD COLUMN manifest_file_name TEXT;
ALTER TABLE physical_full_backups ADD COLUMN manifest_encryption_salt TEXT;
ALTER TABLE physical_full_backups ADD COLUMN manifest_encryption_iv TEXT;

ALTER TABLE physical_incremental_backups ADD COLUMN compression TEXT NOT NULL DEFAULT 'ZSTD';
ALTER TABLE physical_incremental_backups ADD COLUMN manifest_file_name TEXT;
ALTER TABLE physical_incremental_backups ADD COLUMN manifest_encryption_salt TEXT;
ALTER TABLE physical_incremental_backups ADD COLUMN manifest_encryption_iv TEXT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE physical_full_backups DROP COLUMN compression;
ALTER TABLE physical_full_backups DROP COLUMN manifest_file_name;
ALTER TABLE physical_full_backups DROP COLUMN manifest_encryption_salt;
ALTER TABLE physical_full_backups DROP COLUMN manifest_encryption_iv;

ALTER TABLE physical_incremental_backups DROP COLUMN compression;
ALTER TABLE physical_incremental_backups DROP COLUMN manifest_file_name;
ALTER TABLE physical_incremental_backups DROP COLUMN manifest_encryption_salt;
ALTER TABLE physical_incremental_backups DROP COLUMN manifest_encryption_iv;

-- +goose StatementEnd
