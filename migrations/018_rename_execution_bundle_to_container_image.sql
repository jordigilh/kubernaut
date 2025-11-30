-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 018: Rename execution_bundle to container_image, Add container_digest
-- ========================================
-- Authority: DD-WORKFLOW-002 v2.4 (MCP Workflow Catalog Architecture)
-- Authority: DD-CONTRACT-001 v1.2 (AIAnalysis ↔ WorkflowExecution Alignment)
-- Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
-- ========================================
--
-- Purpose: Align database schema with API contract terminology
--
-- Changes:
-- 1. Rename execution_bundle → container_image (align with DD-WORKFLOW-002)
-- 2. Add container_digest column for audit trail
--
-- V1.0 Behavior:
-- - container_image: Base image with optional tag (e.g., quay.io/org/img:v1.0.0)
-- - container_digest: SHA256 digest (e.g., sha256:abc123...)
-- - Full pullspec = container_image + "@" + container_digest
-- - Both fields MANDATORY for V1.0 (digest must be provided in input pullspec)
--
-- V1.1+ Behavior:
-- - Accept tag-only pullspec, system resolves digest from registry
--
-- ========================================

-- Rename execution_bundle to container_image
-- This aligns with DD-WORKFLOW-002 v2.4 and DD-CONTRACT-001 v1.2 terminology
ALTER TABLE remediation_workflow_catalog
RENAME COLUMN execution_bundle TO container_image;

-- Add container_digest column for audit trail and immutability
-- Format: sha256:64_hex_characters (71 chars total)
ALTER TABLE remediation_workflow_catalog
ADD COLUMN container_digest VARCHAR(71);

-- Create index for digest lookups (audit trail queries)
CREATE INDEX idx_workflow_catalog_container_digest
    ON remediation_workflow_catalog(container_digest)
    WHERE container_digest IS NOT NULL;

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================
COMMENT ON COLUMN remediation_workflow_catalog.container_image IS 'DD-WORKFLOW-002 v2.4: OCI image reference (base + optional tag). Full pullspec = container_image@container_digest';
COMMENT ON COLUMN remediation_workflow_catalog.container_digest IS 'DD-WORKFLOW-002 v2.4: SHA256 digest for audit trail and immutability (e.g., sha256:abc123...)';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop the digest index
DROP INDEX IF EXISTS idx_workflow_catalog_container_digest;

-- Remove container_digest column
ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS container_digest;

-- Rename back to execution_bundle
ALTER TABLE remediation_workflow_catalog
RENAME COLUMN container_image TO execution_bundle;

-- +goose StatementEnd



