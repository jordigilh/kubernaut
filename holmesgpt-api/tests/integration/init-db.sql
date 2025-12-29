-- Database Initialization for HAPI Integration Tests
-- V1.0 Complete Schema (migrations 015 + 019 + 020)
-- Authority: Data Storage migrations (V1.0 label-only architecture)
-- DD-WORKFLOW-002 v3.0: workflow_id is UUID, workflow_name is human-readable identifier
-- DD-WORKFLOW-001 v1.6: 3 label columns (labels, custom_labels, detected_labels)

-- ========================================
-- ENABLE UUID EXTENSION
-- ========================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ========================================
-- REMEDIATION WORKFLOW CATALOG (V1.0 COMPLETE)
-- ========================================
-- Includes: Migration 015 (base), 019 (UUID PK), 020 (label columns)

CREATE TABLE IF NOT EXISTS remediation_workflow_catalog (
    -- ========================================
    -- IDENTITY (Migration 019: UUID Primary Key)
    -- ========================================
    workflow_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_name VARCHAR(255) NOT NULL,  -- Migration 019: Human-readable ID
    version VARCHAR(50) NOT NULL,

    -- ========================================
    -- METADATA
    -- ========================================
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    owner VARCHAR(255),
    maintainer VARCHAR(255),

    -- ========================================
    -- CONTENT
    -- ========================================
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,

    -- ========================================
    -- LABELS (V1.0 LABEL-ONLY SEARCH)
    -- ========================================
    -- DD-WORKFLOW-001 v1.6: Three label columns for different purposes

    -- Mandatory labels (signal_type, severity, component, environment, priority)
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Customer-defined labels (DD-WORKFLOW-001 v1.5) - Migration 020
    -- Format: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
    custom_labels JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Auto-detected labels (DD-WORKFLOW-001 v1.6) - Migration 020
    -- 9 fields: gitOpsManaged, pdbProtected, hpaEnabled, stateful, etc.
    detected_labels JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- ========================================
    -- CONTAINER IMAGE (DD-CONTRACT-001 v1.2)
    -- ========================================
    container_image TEXT,
    container_digest VARCHAR(71),

    -- ========================================
    -- PARAMETERS
    -- ========================================
    parameters JSONB,
    execution_engine VARCHAR(50) NOT NULL DEFAULT 'tekton',

    -- ========================================
    -- LIFECYCLE MANAGEMENT
    -- ========================================
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,

    -- ========================================
    -- VERSION MANAGEMENT
    -- ========================================
    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),
    deprecation_notice TEXT,
    version_notes TEXT,
    change_summary TEXT,
    approved_by VARCHAR(255),
    approved_at TIMESTAMP WITH TIME ZONE,

    -- ========================================
    -- SUCCESS METRICS
    -- ========================================
    expected_success_rate DECIMAL(4,3),
    expected_duration_seconds INTEGER,
    actual_success_rate DECIMAL(4,3),
    total_executions INTEGER DEFAULT 0,
    successful_executions INTEGER DEFAULT 0,

    -- ========================================
    -- AUDIT TRAIL
    -- ========================================
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),

    -- ========================================
    -- CONSTRAINTS
    -- ========================================
    -- Migration 019: UNIQUE on (workflow_name, version)
    CONSTRAINT uq_workflow_name_version UNIQUE (workflow_name, version),
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    CHECK (total_executions >= 0),
    CHECK (successful_executions >= 0 AND successful_executions <= total_executions)
);

-- ========================================
-- INDEXES
-- ========================================

CREATE INDEX IF NOT EXISTS idx_workflow_catalog_status
    ON remediation_workflow_catalog(status);

-- Migration 019: Latest version by workflow_name
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_latest_by_name
    ON remediation_workflow_catalog(workflow_name, is_latest_version)
    WHERE is_latest_version = true;

-- Migration 019: Workflow name lookups
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_workflow_name
    ON remediation_workflow_catalog(workflow_name);

-- GIN indexes for JSONB label filtering (V1.0 primary search mechanism)
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_labels
    ON remediation_workflow_catalog USING GIN (labels);

-- Migration 020: Label column indexes
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_custom_labels
    ON remediation_workflow_catalog USING GIN (custom_labels);

CREATE INDEX IF NOT EXISTS idx_workflow_catalog_detected_labels
    ON remediation_workflow_catalog USING GIN (detected_labels);

CREATE INDEX IF NOT EXISTS idx_workflow_catalog_created_at
    ON remediation_workflow_catalog(created_at DESC);

-- ========================================
-- TRIGGER FOR updated_at
-- ========================================

CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_workflow_catalog_updated_at ON remediation_workflow_catalog;

CREATE TRIGGER trigger_workflow_catalog_updated_at
    BEFORE UPDATE ON remediation_workflow_catalog
    FOR EACH ROW
    EXECUTE FUNCTION update_workflow_catalog_updated_at();

-- ========================================
-- COMMENTS
-- ========================================

COMMENT ON TABLE remediation_workflow_catalog IS 'V1.0 Label-Only Workflow Catalog (migrations 015+019+020)';
COMMENT ON COLUMN remediation_workflow_catalog.workflow_id IS 'UUID primary key (auto-generated, DD-WORKFLOW-002 v3.0)';
COMMENT ON COLUMN remediation_workflow_catalog.workflow_name IS 'Human-readable workflow identifier (e.g., "pod-oom-recovery", DD-WORKFLOW-002 v3.0)';
COMMENT ON COLUMN remediation_workflow_catalog.labels IS 'DD-WORKFLOW-001 v1.6: Mandatory labels (signal_type, severity, component, environment, priority)';
COMMENT ON COLUMN remediation_workflow_catalog.custom_labels IS 'DD-WORKFLOW-001 v1.5: Customer-defined labels for hard filtering';
COMMENT ON COLUMN remediation_workflow_catalog.detected_labels IS 'DD-WORKFLOW-001 v1.6: Auto-detected labels from Kubernetes resources (9 fields)';

SELECT 'Schema created successfully (V1.0 Complete - DD-WORKFLOW-002 v3.0 + migrations 015+019+020)' AS status;
