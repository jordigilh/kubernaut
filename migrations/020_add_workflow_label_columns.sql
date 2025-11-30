-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 020: Add DD-WORKFLOW-001 v1.6 Label Columns
-- ========================================
-- Authority: DD-WORKFLOW-001 v1.6 (Mandatory Workflow Label Schema)
-- Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
-- ========================================
--
-- Purpose: Add columns for DD-WORKFLOW-001 v1.6 label schema
--
-- New Columns:
-- 1. custom_labels (JSONB) - Customer-defined labels for hard filtering
-- 2. detected_labels (JSONB) - Auto-detected labels from Kubernetes resources
--
-- DD-WORKFLOW-001 v1.6 Schema:
-- - 5 Mandatory labels: signal_type, severity, component, environment, priority
--   (These are stored in the existing 'labels' JSONB column)
-- - 9 Detected labels: git_ops_managed, pdb_protected, hpa_enabled, stateful,
--   helm_managed, network_isolated, git_ops_tool, pod_security_level, service_mesh
-- - Custom labels: map[subdomain][]string format
--
-- ========================================

-- Add custom_labels column (JSONB for customer-defined labels)
-- DD-WORKFLOW-001 v1.5: Subdomain-based extraction design
-- Format: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
ALTER TABLE remediation_workflow_catalog
ADD COLUMN custom_labels JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Add detected_labels column (JSONB for auto-detected labels)
-- DD-WORKFLOW-001 v1.6: 9 auto-detected fields
ALTER TABLE remediation_workflow_catalog
ADD COLUMN detected_labels JSONB NOT NULL DEFAULT '{}'::jsonb;

-- ========================================
-- INDEXES FOR QUERY PERFORMANCE
-- ========================================

-- GIN index for custom_labels filtering
CREATE INDEX idx_workflow_catalog_custom_labels
    ON remediation_workflow_catalog USING GIN (custom_labels);

-- GIN index for detected_labels filtering
CREATE INDEX idx_workflow_catalog_detected_labels
    ON remediation_workflow_catalog USING GIN (detected_labels);

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================
COMMENT ON COLUMN remediation_workflow_catalog.custom_labels IS 'DD-WORKFLOW-001 v1.5: Customer-defined labels for hard filtering. Format: map[subdomain][]string';
COMMENT ON COLUMN remediation_workflow_catalog.detected_labels IS 'DD-WORKFLOW-001 v1.6: Auto-detected labels from Kubernetes resources. 9 fields with wildcard support';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Remove DD-WORKFLOW-001 v1.6 columns
DROP INDEX IF EXISTS idx_workflow_catalog_custom_labels;
DROP INDEX IF EXISTS idx_workflow_catalog_detected_labels;

ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS custom_labels;

ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS detected_labels;

-- +goose StatementEnd

