-- +goose Up
-- Migration: 014_rename_audit_cluster_name_to_cluster_id
-- Issue #1651: audit_events.cluster_name was never populated by any shipped
-- release (v1.1.0 through v1.5.2) — verified via git history on every
-- SetClusterName call site. The column's actual intent (SOC2 CC8.1 fleet
-- provenance, DD-AUDIT-003 v2.2) is to store the unique cluster identifier,
-- not a non-unique display name. Safe to rename in place; no production
-- data migration required.
--
-- FedRAMP AU-9/AU-3: audit record content — renaming, not changing semantics
-- of what is captured (cluster provenance for reconstruction).

ALTER TABLE audit_events RENAME COLUMN cluster_name TO cluster_id;

COMMENT ON COLUMN audit_events.cluster_id IS
    'Unique cluster identifier for fleet provenance (SOC2 CC8.1, DD-AUDIT-003 v2.2). Renamed from cluster_name in #1651 — never populated as a display name in any shipped release.';

-- +goose Down
ALTER TABLE audit_events RENAME COLUMN cluster_id TO cluster_name;
