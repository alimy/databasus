-- +goose Up
-- +goose StatementBegin

UPDATE databases SET type = 'POSTGRES_LOGICAL' WHERE type = 'POSTGRES';

ALTER TABLE postgresql_logical_databases
    RENAME CONSTRAINT postgresql_databases_pkey TO postgresql_logical_databases_pkey;

ALTER TABLE postgresql_logical_databases
    RENAME CONSTRAINT uk_postgresql_databases_database_id TO uk_postgresql_logical_databases_database_id;

ALTER INDEX idx_postgresql_databases_database_id RENAME TO idx_postgresql_logical_databases_database_id;

CREATE TABLE postgresql_physical_databases (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id              UUID,
    version                  TEXT NOT NULL,
    host                     TEXT NOT NULL,
    port                     INT NOT NULL,
    username                 TEXT NOT NULL,
    password                 TEXT NOT NULL,
    ssl_mode                 TEXT NOT NULL DEFAULT 'disable',
    ssl_client_cert          TEXT NOT NULL DEFAULT '',
    ssl_client_key           TEXT NOT NULL DEFAULT '',
    ssl_root_cert            TEXT NOT NULL DEFAULT '',
    replication_slot_name    TEXT NOT NULL,
    system_identifier        TEXT,
    backup_type              TEXT NOT NULL DEFAULT 'FULL'
);

ALTER TABLE postgresql_physical_databases
    ADD CONSTRAINT uk_postgresql_physical_databases_database_id
    UNIQUE (database_id);

ALTER TABLE postgresql_physical_databases
    ADD CONSTRAINT fk_postgresql_physical_databases_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

CREATE INDEX idx_postgresql_physical_databases_database_id
    ON postgresql_physical_databases (database_id);

CREATE TABLE physical_backup_configs (
    database_id                    UUID PRIMARY KEY,

    is_backups_enabled             BOOLEAN NOT NULL DEFAULT FALSE,

    full_interval_type             TEXT NOT NULL DEFAULT '',
    full_time_of_day               TEXT,
    full_weekday                   INT,
    full_day_of_month              INT,
    full_cron_expression           TEXT,

    incremental_interval_type      TEXT NOT NULL DEFAULT '',
    incremental_time_of_day        TEXT,
    incremental_weekday            INT,
    incremental_day_of_month       INT,
    incremental_cron_expression    TEXT,

    retention                              TEXT NOT NULL DEFAULT 'FULL_BACKUPS',

    chains_retention_count                 INT  NOT NULL DEFAULT 0,

    full_backups_retention_policy          TEXT NOT NULL DEFAULT '',
    full_backups_retention_count           INT  NOT NULL DEFAULT 0,
    full_backups_retention_gfs_hours       INT  NOT NULL DEFAULT 0,
    full_backups_retention_gfs_days        INT  NOT NULL DEFAULT 0,
    full_backups_retention_gfs_weeks       INT  NOT NULL DEFAULT 0,
    full_backups_retention_gfs_months      INT  NOT NULL DEFAULT 0,
    full_backups_retention_gfs_years       INT  NOT NULL DEFAULT 0,

    wal_fallback_strategy          TEXT   NOT NULL DEFAULT 'INCREMENTAL_WITH_LOSS',
    wal_lag_threshold_bytes        BIGINT NOT NULL DEFAULT 0,

    storage_id                     UUID,
    encryption                     TEXT NOT NULL DEFAULT 'NONE',
    send_notifications_on          TEXT NOT NULL DEFAULT ''
);

ALTER TABLE physical_backup_configs
    ADD CONSTRAINT fk_physical_backup_configs_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

ALTER TABLE physical_backup_configs
    ADD CONSTRAINT fk_physical_backup_configs_storage_id
    FOREIGN KEY (storage_id)
    REFERENCES storages (id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'no-op: physical backup infrastructure is forward-only';
-- +goose StatementEnd
