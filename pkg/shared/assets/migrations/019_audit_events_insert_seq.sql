-- +goose Up
-- Migration: 019_audit_events_insert_seq
-- Issue #2318: hash-chain verification/write-ordering false positives
-- FedRAMP AU-9: Protection of Audit Information
-- SOC2 CC7.2 / CC8.1: Monitoring for anomalies; tamper-evident audit trail
--
-- getPreviousEventHash (pkg/datastorage/repository/audit_events_hashchain.go)
-- picks the hash-chain predecessor via
-- `ORDER BY event_timestamp DESC, event_id DESC LIMIT 1`. event_id is a
-- client-generated UUID (unrelated to insertion order), so on a same-second
-- write burst for one correlation_id, the DESC tiebreak can select the wrong
-- (not-actually-latest) predecessor, silently forking the chain at write
-- time. insert_seq replaces that ambiguous tiebreak with a true, monotonic
-- write-order column.
--
-- Sequence lives on the parent partitioned table and is shared across all
-- partitions (a per-partition BIGSERIAL/IDENTITY would create independent
-- sequences per partition and reintroduce the same ordering ambiguity this
-- migration exists to close).
--
-- No backfill: this migration does not support upgrading an existing
-- installation in place. Adopting it requires a fresh database. See
-- DD-AUDIT-009 and the migrations/README.md note on CREATE INDEX vs.
-- CREATE INDEX CONCURRENTLY inside Goose's transaction envelope.
CREATE SEQUENCE audit_events_insert_seq;

ALTER TABLE audit_events
    ADD COLUMN insert_seq BIGINT NOT NULL DEFAULT nextval('audit_events_insert_seq');

ALTER SEQUENCE audit_events_insert_seq OWNED BY audit_events.insert_seq;

COMMENT ON COLUMN audit_events.insert_seq IS
    'Monotonic write-order tiebreak for hash-chain predecessor lookup and verify-chain ordering (Issue #2318). Shared sequence across all partitions -- do not replace with a per-partition serial/identity column.';

CREATE INDEX idx_audit_events_correlation_insert_seq
    ON audit_events (correlation_id, insert_seq);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_events_correlation_insert_seq;
ALTER TABLE audit_events DROP COLUMN IF EXISTS insert_seq;
DROP SEQUENCE IF EXISTS audit_events_insert_seq;
