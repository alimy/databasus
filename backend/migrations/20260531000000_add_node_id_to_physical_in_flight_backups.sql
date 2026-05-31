-- +goose Up
-- +goose StatementBegin

-- node_id records which backup node owns the in-flight backup, so a restarted
-- primary can tell a still-running backup (live owner) from an orphaned one
-- (dead owner) instead of failing every IN_PROGRESS row blindly. Nullable: a
-- claim exists briefly before the node is assigned, and pre-existing rows have
-- no owner — both are treated as "unknown owner" and failed on recovery.
ALTER TABLE physical_in_flight_backups ADD COLUMN node_id UUID;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE physical_in_flight_backups DROP COLUMN node_id;

-- +goose StatementEnd
