-- +goose Up
-- +goose StatementBegin
ALTER TABLE backup_configs       RENAME TO logical_backup_configs;
ALTER TABLE backups              RENAME TO logical_backups;
ALTER TABLE postgresql_databases RENAME TO postgresql_logical_databases;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'no-op: table renames for logical/physical split are one-way';
-- +goose StatementEnd
