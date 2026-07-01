-- +goose Up
-- Migration: 013_audit_hash_algorithm
-- GAP-05 (Issue #1505): Keyed HMAC-SHA256 hash chain algorithm tracking
-- FedRAMP AU-9: Protection of Audit Information (keyed MAC vs. unkeyed hash)
-- SOC2 CC8.1: Tamper-evident audit trail
--
-- Records which hashing scheme produced event_hash for each row, enabling a
-- mixed-algorithm chain during HMAC key rollout: existing rows keep using
-- sha256-unkeyed (the only algorithm that existed before this migration);
-- new writes use hmac-sha256 once a datastorage audit HMAC key is configured,
-- or continue using sha256-unkeyed otherwise (backward-compatible default).
--
-- A plain (unkeyed) SHA256 hash chain can be recomputed by anyone with
-- read/write access to the database — an attacker who tampers with a row can
-- also recompute a self-consistent chain. HMAC-SHA256 requires a secret key
-- stored outside the database (Kubernetes Secret), so forging a valid hash
-- without that key is computationally infeasible.

ALTER TABLE audit_events
    ADD COLUMN hash_algorithm VARCHAR(20) NOT NULL DEFAULT 'sha256-unkeyed';

COMMENT ON COLUMN audit_events.hash_algorithm IS
    'Hash algorithm used to compute event_hash: sha256-unkeyed (legacy, unkeyed) or hmac-sha256 (keyed, GAP-05/Issue #1505)';

-- +goose Down
ALTER TABLE audit_events DROP COLUMN IF EXISTS hash_algorithm;
