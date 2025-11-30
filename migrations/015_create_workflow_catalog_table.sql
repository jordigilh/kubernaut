-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 015: Remediation Workflow Catalog
-- ========================================
-- Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
-- Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
-- Design Decision: DD-NAMING-001 (Remediation Workflow Terminology)
-- Immutability: DD-WORKFLOW-012 (Workflow Immutability Constraints)
-- ========================================
--
-- Purpose: Create remediation_workflow_catalog table for semantic search
--
-- Key Features:
-- 1. Composite primary key (workflow_id, version) for immutability (DD-WORKFLOW-012)
-- 2. pgvector embedding column for semantic search
-- 3. JSONB labels for flexible filtering
-- 4. Lifecycle management (active/disabled/deprecated/archived)
-- 5. Version management with history tracking
-- 6. Success metrics tracking
--
-- IMMUTABILITY (DD-WORKFLOW-012):
-- - PRIMARY KEY (workflow_id, version) enforces immutability
-- - Content fields (description, content, labels, embedding) CANNOT be updated
-- - Lifecycle fields (status, metrics) CAN be updated
-- - To change content, create a new version
--
-- ========================================

-- Enable pgvector extension for semantic search
CREATE EXTENSION IF NOT EXISTS vector;

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
    -- Examples:
    -- {
    --   "signal_types": ["MemoryLeak", "OOMKilled"],
    --   "business_category": "payments",
    --   "risk_tolerance": "low",
    --   "environment": "production"
    -- }
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- ========================================
    -- SEMANTIC SEARCH (pgvector)
    -- ========================================
    -- BR-STORAGE-012: Vector embeddings for semantic search
    -- Model: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
    embedding vector(384),

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

-- Index for latest version queries (critical for semantic search)
CREATE INDEX idx_workflow_catalog_latest
    ON remediation_workflow_catalog(workflow_id, is_latest_version)
    WHERE is_latest_version = true;

-- GIN index for JSONB label filtering (DD-CONTEXT-005)
CREATE INDEX idx_workflow_catalog_labels
    ON remediation_workflow_catalog USING GIN (labels);

-- HNSW index for semantic search (pgvector)
-- BR-STORAGE-012: Fast approximate nearest neighbor search
-- Parameters:
-- - m = 16: Number of connections per layer (balance between recall and speed)
-- - ef_construction = 64: Size of dynamic candidate list during construction
CREATE INDEX idx_workflow_catalog_embedding
    ON remediation_workflow_catalog
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

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
COMMENT ON TABLE remediation_workflow_catalog IS 'Remediation workflow catalog for semantic search and version management (DD-STORAGE-008 v2.0)';
COMMENT ON COLUMN remediation_workflow_catalog.workflow_id IS 'Unique workflow identifier (e.g., "pod-oom-recovery")';
COMMENT ON COLUMN remediation_workflow_catalog.version IS 'Semantic version (e.g., "v1.0.0", "v1.2.3")';
COMMENT ON COLUMN remediation_workflow_catalog.embedding IS 'Vector embedding for semantic search (384 dimensions, sentence-transformers/all-MiniLM-L6-v2)';
COMMENT ON COLUMN remediation_workflow_catalog.labels IS 'JSONB labels for filtering (signal_types, business_category, risk_tolerance, environment)';
COMMENT ON COLUMN remediation_workflow_catalog.status IS 'Lifecycle status: active, disabled, deprecated, archived';
COMMENT ON COLUMN remediation_workflow_catalog.is_latest_version IS 'Flag indicating if this is the latest version of the workflow';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop table and all dependent objects
DROP TABLE IF EXISTS remediation_workflow_catalog CASCADE;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_workflow_catalog_updated_at() CASCADE;

-- Note: pgvector extension is NOT dropped as other tables may use it

-- +goose StatementEnd

