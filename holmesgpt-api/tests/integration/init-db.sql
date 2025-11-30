-- Database Initialization for Workflow Catalog Integration Tests
-- DD-WORKFLOW-002 v3.0: workflow_id is UUID (auto-generated primary key)

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS remediation_workflow_catalog (
    -- DD-WORKFLOW-002 v3.0: workflow_id is UUID, auto-generated
    workflow_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Human-readable identifier and version (UNIQUE constraint)
    workflow_name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,

    -- Display title
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,

    owner VARCHAR(255),
    maintainer VARCHAR(255),

    -- ADR-043 workflow content
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,

    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- DD-WORKFLOW-001 v1.6: Customer-defined labels for hard filtering
    custom_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- DD-WORKFLOW-001 v1.6: Auto-detected labels from Kubernetes resources
    detected_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding vector(768),

    parameters JSONB,
    execution_engine VARCHAR(50) NOT NULL DEFAULT 'tekton',

    container_image TEXT,
    container_digest VARCHAR(71),

    status VARCHAR(20) NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,

    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),
    deprecation_notice TEXT,
    version_notes TEXT,
    change_summary TEXT,
    approved_by VARCHAR(255),
    approved_at TIMESTAMP WITH TIME ZONE,

    expected_success_rate DECIMAL(4,3),
    expected_duration_seconds INTEGER,
    actual_success_rate DECIMAL(4,3),
    total_executions INTEGER DEFAULT 0,
    successful_executions INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),

    -- DD-WORKFLOW-002 v3.0: UNIQUE on (workflow_name, version)
    UNIQUE (workflow_name, version),
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived'))
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_status ON remediation_workflow_catalog(status);
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_labels ON remediation_workflow_catalog USING GIN (labels);
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_custom_labels ON remediation_workflow_catalog USING GIN (custom_labels);
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_detected_labels ON remediation_workflow_catalog USING GIN (detected_labels);
CREATE INDEX IF NOT EXISTS idx_workflow_catalog_embedding ON remediation_workflow_catalog USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);

-- Trigger for updated_at
CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_workflow_catalog_updated_at ON remediation_workflow_catalog;
CREATE TRIGGER trigger_workflow_catalog_updated_at
    BEFORE UPDATE ON remediation_workflow_catalog
    FOR EACH ROW EXECUTE FUNCTION update_workflow_catalog_updated_at();

SELECT 'Schema created successfully (DD-WORKFLOW-002 v3.0)' AS status;
