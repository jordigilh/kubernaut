-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 017: Add ADR-043 Workflow Schema Fields
-- ========================================
-- Authority: ADR-043 (Workflow Schema Definition Standard)
-- Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
-- Design Decision: DD-WORKFLOW-005 (Automated Schema Extraction)
-- ========================================
--
-- Purpose: Add columns for ADR-043 workflow schema support
--
-- New Columns:
-- 1. parameters (JSONB) - Rich parameter definitions for LLM guidance
-- 2. execution_engine (VARCHAR) - Execution engine type (default: 'tekton')
-- 3. execution_bundle (TEXT) - OCI bundle reference (V1.1+)
--
-- V1.0 Behavior:
-- - parameters: Extracted from workflow-schema.yaml content at creation time
-- - execution_engine: Always 'tekton' (default)
-- - execution_bundle: NULL (not used in V1.0)
--
-- V1.1+ Behavior:
-- - WorkflowRegistration CRD controller extracts schema from container
-- - execution_bundle: OCI bundle URL from container image
--
-- ========================================

-- Add parameters column (JSONB for rich parameter definitions)
-- ADR-043: Parameters include name, type, required, description, enum, pattern, min/max
ALTER TABLE remediation_workflow_catalog
ADD COLUMN parameters JSONB;

-- Add execution_engine column (default 'tekton' for V1.0)
-- ADR-043: V1 values: "tekton", V2 values: "tekton", "ansible", "lambda", "shell"
ALTER TABLE remediation_workflow_catalog
ADD COLUMN execution_engine VARCHAR(50) NOT NULL DEFAULT 'tekton';

-- Add execution_bundle column (OCI bundle reference for V1.1+)
-- ADR-043: Container image or bundle reference
ALTER TABLE remediation_workflow_catalog
ADD COLUMN execution_bundle TEXT;

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================
COMMENT ON COLUMN remediation_workflow_catalog.parameters IS 'ADR-043: Rich parameter definitions for LLM guidance (JSONB array of {name, type, required, description, enum, pattern, min, max, default})';
COMMENT ON COLUMN remediation_workflow_catalog.execution_engine IS 'ADR-043: Execution engine type (tekton, ansible, lambda, shell). Default: tekton';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle IS 'ADR-043: OCI bundle or execution reference (V1.1+). NULL for V1.0';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Remove ADR-043 columns
ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS parameters;

ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS execution_engine;

ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS execution_bundle;

-- +goose StatementEnd

