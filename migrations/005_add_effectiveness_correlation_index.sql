-- Migration 005: Add partial index for effectiveness correlation queries
--
-- Supports QueryEffectivenessEventsBatch and queryEffectivenessEvents which
-- filter by event_category = 'effectiveness' and join on correlation_id.
-- The partial index limits the scan to ~30% of audit_events (effectiveness
-- events only) and provides an efficient access path for ORDER BY event_timestamp.
--
-- F6 (DS Due Diligence): Performance optimization for effectiveness queries.

-- +goose Up
CREATE INDEX IF NOT EXISTS idx_audit_events_effectiveness_correlation
  ON audit_events (correlation_id, event_timestamp ASC)
  WHERE event_category = 'effectiveness';

-- +goose Down
DROP INDEX IF EXISTS idx_audit_events_effectiveness_correlation;
