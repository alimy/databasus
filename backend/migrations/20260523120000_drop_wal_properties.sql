-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_backups_pg_wal_segment_name;
DROP INDEX IF EXISTS idx_backups_pg_wal_backup_type_created;

ALTER TABLE backups
    DROP COLUMN IF EXISTS pg_wal_backup_type,
    DROP COLUMN IF EXISTS pg_wal_start_segment,
    DROP COLUMN IF EXISTS pg_wal_stop_segment,
    DROP COLUMN IF EXISTS pg_version,
    DROP COLUMN IF EXISTS pg_wal_segment_name,
    DROP COLUMN IF EXISTS upload_completed_at;

UPDATE postgresql_databases SET host     = 'localhost'    WHERE host     IS NULL OR host     = '';
UPDATE postgresql_databases SET port     = 5432           WHERE port     IS NULL OR port     = 0;
UPDATE postgresql_databases SET username = 'postgres'     WHERE username IS NULL OR username = '';
UPDATE postgresql_databases SET password = 'stubpassword' WHERE password IS NULL OR password = '';

ALTER TABLE postgresql_databases
    DROP COLUMN IF EXISTS backup_type;

ALTER TABLE postgresql_databases
    ALTER COLUMN host     SET NOT NULL,
    ALTER COLUMN port     SET NOT NULL,
    ALTER COLUMN username SET NOT NULL,
    ALTER COLUMN password SET NOT NULL;

DROP INDEX IF EXISTS idx_databases_agent_token;

ALTER TABLE databases
    DROP COLUMN IF EXISTS agent_token,
    DROP COLUMN IF EXISTS is_agent_token_generated;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'no-op: WAL backup type / backup agent removal is one-way';
-- +goose StatementEnd
