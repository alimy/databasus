-- +goose Up
-- +goose StatementBegin

ALTER TABLE postgresql_physical_databases
    ADD COLUMN wal_segment_size_bytes BIGINT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'no-op: wal_segment_size_bytes column is forward-only';
-- +goose StatementEnd
