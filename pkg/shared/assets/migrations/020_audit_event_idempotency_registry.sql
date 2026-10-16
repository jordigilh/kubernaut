-- +goose Up
-- DD-AUDIT-010: durable idempotency registry for client-supplied audit event IDs.
-- The audit_events table is date-partitioned, so event_id alone cannot be a
-- globally unique arbiter. This unpartitioned registry provides that key
-- without changing historical audit partitions.
CREATE TABLE IF NOT EXISTS audit_event_idempotency (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    event_action TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    event_fingerprint TEXT NOT NULL,
    event_hash TEXT NOT NULL,
    previous_event_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_event_idempotency_correlation
    ON audit_event_idempotency (correlation_id);

-- +goose Down
DROP TABLE IF EXISTS audit_event_idempotency;
