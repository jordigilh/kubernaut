-- Migration 017: Add btree index on cluster_id for fleet-scoped remediation
-- history queries (Issue #1802, main only).
--
-- QueryROEventsBySpecHash (pkg/datastorage/repository/remediation_history_repository.go)
-- now filters on `cluster_id = $2` alongside the existing target_resource
-- expression-index predicate whenever a fleet deployment supplies a
-- non-empty ClusterID. Without this index, that added predicate is a
-- sequential scan filter on every row already matched by
-- idx_audit_events_target_resource; with it, the planner can combine both
-- via a BitmapAnd instead.
--
-- Partial index (WHERE cluster_id IS NOT NULL) keeps it small: unscoped rows
-- (hub-local deployments, or release/v1.5 which has no cluster_id concept at
-- all) never match the `cluster_id = $2` branch and gain nothing from being
-- indexed here.
--
-- SOC2 CC8.1 / FedRAMP AU-9: index-only change, no semantic change to what
-- is captured or how audit records are reconstructed.

-- +goose Up
CREATE INDEX IF NOT EXISTS idx_audit_events_cluster_id
  ON audit_events (cluster_id)
  WHERE cluster_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_audit_events_cluster_id;
