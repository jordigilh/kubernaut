-- +goose Up
-- +goose StatementBegin
-- November 2025 partition
CREATE TABLE IF NOT EXISTS audit_events_2025_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
-- +goose StatementEnd

-- +goose StatementBegin
-- December 2025 partition
CREATE TABLE IF NOT EXISTS audit_events_2025_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
-- +goose StatementEnd

-- +goose StatementBegin
-- January 2026 partition
CREATE TABLE IF NOT EXISTS audit_events_2026_01
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
-- +goose StatementEnd

-- +goose StatementBegin
-- February 2026 partition
CREATE TABLE IF NOT EXISTS audit_events_2026_02
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_events_2025_11;
DROP TABLE IF EXISTS audit_events_2025_12;
DROP TABLE IF NOT EXISTS audit_events_2026_01;
DROP TABLE IF EXISTS audit_events_2026_02;
-- +goose StatementEnd

