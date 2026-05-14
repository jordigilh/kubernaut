-- +goose Up
-- +goose StatementBegin

-- Phase 13: Align all timestamp columns to TIMESTAMP WITH TIME ZONE (UTC)
-- Prevents session-timezone-dependent interpretation of stored instants.
ALTER TABLE audit_events
    ALTER COLUMN legal_hold_placed_at TYPE TIMESTAMP WITH TIME ZONE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE audit_events
    ALTER COLUMN legal_hold_placed_at TYPE TIMESTAMP;

-- +goose StatementEnd
