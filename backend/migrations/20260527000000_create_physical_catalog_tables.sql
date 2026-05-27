-- +goose Up
-- +goose StatementBegin

CREATE TABLE physical_full_backups (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id         UUID NOT NULL,
    storage_id          UUID NOT NULL,
    timeline_id         INT  NOT NULL,
    status              TEXT NOT NULL,
    error_reason        TEXT,
    file_name           TEXT,
    start_lsn           PG_LSN,
    stop_lsn            PG_LSN,
    backup_size_mb      DOUBLE PRECISION,
    raw_size_mb         DOUBLE PRECISION,
    backup_duration_ms  BIGINT,
    encryption          TEXT NOT NULL DEFAULT 'NONE',
    encryption_salt     TEXT,
    encryption_iv       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

ALTER TABLE physical_full_backups
    ADD CONSTRAINT fk_physical_full_backups_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

ALTER TABLE physical_full_backups
    ADD CONSTRAINT fk_physical_full_backups_storage_id
    FOREIGN KEY (storage_id)
    REFERENCES storages (id);

CREATE INDEX idx_physical_full_backups_database_id_status_created_at
    ON physical_full_backups (database_id, status, created_at DESC);

CREATE INDEX idx_physical_full_backups_database_id_timeline_id
    ON physical_full_backups (database_id, timeline_id);


CREATE TABLE physical_incremental_backups (
    id                            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id                   UUID NOT NULL,
    storage_id                    UUID NOT NULL,
    timeline_id                   INT  NOT NULL,
    status                        TEXT NOT NULL,
    error_reason                  TEXT,
    file_name                     TEXT,
    start_lsn                     PG_LSN,
    stop_lsn                      PG_LSN,
    backup_size_mb                DOUBLE PRECISION,
    raw_size_mb                   DOUBLE PRECISION,
    backup_duration_ms            BIGINT,
    encryption                    TEXT NOT NULL DEFAULT 'NONE',
    encryption_salt               TEXT,
    encryption_iv                 TEXT,
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at                  TIMESTAMPTZ,
    root_full_backup_id           UUID NOT NULL,
    parent_incremental_backup_id  UUID
);

ALTER TABLE physical_incremental_backups
    ADD CONSTRAINT fk_physical_incremental_backups_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

ALTER TABLE physical_incremental_backups
    ADD CONSTRAINT fk_physical_incremental_backups_storage_id
    FOREIGN KEY (storage_id)
    REFERENCES storages (id);

ALTER TABLE physical_incremental_backups
    ADD CONSTRAINT fk_physical_incremental_backups_root_full_backup_id
    FOREIGN KEY (root_full_backup_id)
    REFERENCES physical_full_backups (id)
    ON DELETE RESTRICT;

ALTER TABLE physical_incremental_backups
    ADD CONSTRAINT fk_physical_incremental_backups_parent_incremental_backup_id
    FOREIGN KEY (parent_incremental_backup_id)
    REFERENCES physical_incremental_backups (id)
    ON DELETE RESTRICT;

CREATE INDEX idx_physical_incremental_backups_root_full_start_lsn
    ON physical_incremental_backups (root_full_backup_id, start_lsn);

CREATE INDEX idx_physical_incremental_backups_database_id_status_created_at
    ON physical_incremental_backups (database_id, status, created_at DESC);

CREATE INDEX idx_physical_incremental_backups_parent_incremental_backup_id
    ON physical_incremental_backups (parent_incremental_backup_id);


CREATE TABLE physical_in_flight_backups (
    database_id  UUID        PRIMARY KEY,
    backup_type  TEXT        NOT NULL,
    backup_id    UUID        NOT NULL,
    claimed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE physical_in_flight_backups
    ADD CONSTRAINT chk_physical_in_flight_backups_backup_type
    CHECK (backup_type IN ('FULL', 'INCREMENTAL'));

ALTER TABLE physical_in_flight_backups
    ADD CONSTRAINT fk_physical_in_flight_backups_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;


CREATE TABLE physical_wal_segments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id        UUID NOT NULL,
    storage_id         UUID NOT NULL,
    timeline_id        INT  NOT NULL,
    file_name          TEXT,
    wal_filename       TEXT   NOT NULL,
    start_lsn          PG_LSN NOT NULL,
    end_lsn            PG_LSN NOT NULL,
    compressed_size_mb DOUBLE PRECISION NOT NULL DEFAULT 0,
    received_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    encryption         TEXT NOT NULL DEFAULT 'NONE',
    encryption_salt    TEXT,
    encryption_iv      TEXT
);

ALTER TABLE physical_wal_segments
    ADD CONSTRAINT fk_physical_wal_segments_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

ALTER TABLE physical_wal_segments
    ADD CONSTRAINT fk_physical_wal_segments_storage_id
    FOREIGN KEY (storage_id)
    REFERENCES storages (id);

ALTER TABLE physical_wal_segments
    ADD CONSTRAINT uk_physical_wal_segments_database_id_timeline_id_start_lsn
    UNIQUE (database_id, timeline_id, start_lsn);

CREATE INDEX idx_physical_wal_segments_database_id_received_at
    ON physical_wal_segments (database_id, received_at);


CREATE TABLE physical_wal_history_files (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id         UUID NOT NULL,
    storage_id          UUID NOT NULL,
    timeline_id         INT  NOT NULL,
    file_name           TEXT NOT NULL,
    history_filename    TEXT NOT NULL,
    compressed_size_mb  DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE physical_wal_history_files
    ADD CONSTRAINT fk_physical_wal_history_files_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

ALTER TABLE physical_wal_history_files
    ADD CONSTRAINT fk_physical_wal_history_files_storage_id
    FOREIGN KEY (storage_id)
    REFERENCES storages (id);

ALTER TABLE physical_wal_history_files
    ADD CONSTRAINT uk_physical_wal_history_files_database_id_timeline_id
    UNIQUE (database_id, timeline_id);


CREATE TABLE physical_wal_streamers (
    database_id        UUID PRIMARY KEY,
    started_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status             TEXT NOT NULL
);

ALTER TABLE physical_wal_streamers
    ADD CONSTRAINT fk_physical_wal_streamers_database_id
    FOREIGN KEY (database_id)
    REFERENCES databases (id)
    ON DELETE CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'no-op: physical catalog tables are forward-only';
-- +goose StatementEnd
