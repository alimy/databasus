-- +goose Up
-- +goose StatementBegin

ALTER TABLE telegram_notifiers
    ADD COLUMN is_http_proxy_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN http_proxy_url TEXT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE telegram_notifiers
    DROP COLUMN IF EXISTS http_proxy_url,
    DROP COLUMN IF EXISTS is_http_proxy_enabled;

-- +goose StatementEnd
