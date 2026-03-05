-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 015: Remediation Workflow Catalog (V1.0 - Label-Only)
-- ========================================
-- Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
-- Business Requirement: BR-STORAGE-012 (Workflow Catalog - Label-Only Search in V1.0)
-- Design Decision: DD-NAMING-001 (Remediation Workflow Terminology)
-- Immutability: DD-WORKFLOW-012 (Workflow Immutability Constraints)
-- NOTE: V1.0 uses label-only search (deterministic), semantic search removed
-- ========================================
--
-- Purpose: Create remediation_workflow_catalog table for label-based workflow matching
--
-- Key Features:
-- 1. Composite primary key (workflow_id, version) for immutability (DD-WORKFLOW-012)
-- 2. JSONB labels for flexible filtering (mandatory + detected labels)
-- 3. Lifecycle management (active/disabled/deprecated/archived)
-- 4. Version management with history tracking
-- 5. Success metrics tracking
--
-- IMMUTABILITY (DD-WORKFLOW-012):
-- - PRIMARY KEY (workflow_id, version) enforces immutability
-- - Content fields (description, content, labels) CANNOT be updated
-- - Lifecycle fields (status, metrics) CAN be updated
-- - To change content, create a new version
--
-- ========================================

-- Create remediation_workflow_catalog table
CREATE TABLE remediation_workflow_catalog (
    -- ========================================
    -- IDENTITY (Composite Primary Key)
    -- ========================================
    workflow_id VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,           -- MUST be semantic version (e.g., v1.0.0, v1.2.3)

    -- ========================================
    -- METADATA
    -- ========================================
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    owner VARCHAR(255),                      -- Team or user responsible
    maintainer VARCHAR(255),                 -- Contact email

    -- ========================================
    -- CONTENT
    -- ========================================
    content TEXT NOT NULL,                   -- Full workflow YAML/JSON (Tekton Pipeline, Ansible Playbook, etc.)
    content_hash VARCHAR(64) NOT NULL,       -- SHA-256 hash for integrity

    -- ========================================
    -- LABELS (JSONB for flexible filtering)
    -- ========================================
    -- DD-CONTEXT-005: Filter Before LLM pattern
    -- V1.0: Label-only matching (deterministic, no semantic search)
    -- Examples:
    -- {
    --   "signal_types": ["MemoryLeak", "OOMKilled"],
    --   "business_category": "payments",
    --   "risk_tolerance": "low",
    --   "environment": "production",
    --   "detected_labels": {
    --     "gitOpsTool": "argocd",
    --     "pdbProtected": "true"
    --   }
    -- }
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- ========================================
    -- LIFECYCLE MANAGEMENT
    -- ========================================
    -- User Requirement: "disable workflows and keep historical versions"
    status VARCHAR(20) NOT NULL DEFAULT 'active',  -- 'active', 'disabled', 'deprecated', 'archived'
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,

    -- ========================================
    -- VERSION MANAGEMENT
    -- ========================================
    -- User Requirement: traceability + immutability
    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),            -- Link to previous version
    deprecation_notice TEXT,                 -- Reason for deprecation

    -- ========================================
    -- VERSION CHANGE METADATA
    -- ========================================
    -- DD-STORAGE-008: Version validation & traceability
    version_notes TEXT,                      -- Release notes / changelog
    change_summary TEXT,                     -- Auto-generated summary of changes
    approved_by VARCHAR(255),                -- Who approved this version
    approved_at TIMESTAMP WITH TIME ZONE,    -- When was this version approved

    -- ========================================
    -- SUCCESS METRICS (ADR-033)
    -- ========================================
    expected_success_rate DECIMAL(4,3),      -- Expected success rate (0.000-1.000)
    expected_duration_seconds INTEGER,       -- Expected execution time
    actual_success_rate DECIMAL(4,3),        -- Calculated from execution history
    total_executions INTEGER DEFAULT 0,      -- Number of times executed
    successful_executions INTEGER DEFAULT 0, -- Number of successful executions

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
    -- IMMUTABILITY ENFORCEMENT (DD-WORKFLOW-012)
    -- This PRIMARY KEY constraint is the database-level enforcement mechanism
    -- for workflow immutability. Once a (workflow_id, version) pair is created,
    -- it cannot be overwritten. To change workflow content, create a new version.
    PRIMARY KEY (workflow_id, version),      -- IMMUTABILITY: Cannot overwrite existing version (DD-WORKFLOW-012)
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    CHECK (total_executions >= 0),
    CHECK (successful_executions >= 0 AND successful_executions <= total_executions)
);

-- ========================================
-- INDEXES FOR QUERY PERFORMANCE
-- ========================================

-- Index for status filtering (most common query)
CREATE INDEX idx_workflow_catalog_status
    ON remediation_workflow_catalog(status);

-- Index for latest version queries (critical for workflow search)
CREATE INDEX idx_workflow_catalog_latest
    ON remediation_workflow_catalog(workflow_id, is_latest_version)
    WHERE is_latest_version = true;

-- GIN index for JSONB label filtering (DD-CONTEXT-005)
-- V1.0: Primary search mechanism (label matching with wildcard weighting)
CREATE INDEX idx_workflow_catalog_labels
    ON remediation_workflow_catalog USING GIN (labels);

-- Index for created_at (for sorting by newest)
CREATE INDEX idx_workflow_catalog_created_at
    ON remediation_workflow_catalog(created_at DESC);

-- Index for success rate (for filtering high-performing workflows)
CREATE INDEX idx_workflow_catalog_success_rate
    ON remediation_workflow_catalog(actual_success_rate DESC)
    WHERE status = 'active';

-- ========================================
-- TRIGGER FOR updated_at
-- ========================================
CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_workflow_catalog_updated_at
    BEFORE UPDATE ON remediation_workflow_catalog
    FOR EACH ROW
    EXECUTE FUNCTION update_workflow_catalog_updated_at();

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================
COMMENT ON TABLE remediation_workflow_catalog IS 'Remediation workflow catalog for label-based search and version management (DD-STORAGE-008 v2.0, V1.0 label-only)';
COMMENT ON COLUMN remediation_workflow_catalog.workflow_id IS 'Unique workflow identifier (e.g., "pod-oom-recovery")';
COMMENT ON COLUMN remediation_workflow_catalog.version IS 'Semantic version (e.g., "v1.0.0", "v1.2.3")';
COMMENT ON COLUMN remediation_workflow_catalog.labels IS 'JSONB labels for filtering (mandatory + detected labels with wildcard support)';
COMMENT ON COLUMN remediation_workflow_catalog.status IS 'Lifecycle status: active, disabled, deprecated, archived';
COMMENT ON COLUMN remediation_workflow_catalog.is_latest_version IS 'Flag indicating if this is the latest version of the workflow';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop table and all dependent objects
DROP TABLE IF EXISTS remediation_workflow_catalog CASCADE;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_workflow_catalog_updated_at() CASCADE;

-- +goose StatementEnd
