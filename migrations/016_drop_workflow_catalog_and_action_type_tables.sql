-- +goose Up
-- Issue #1661 Phase G (DD-WORKFLOW-018): etcd/Kubernetes CRDs (RemediationWorkflow,
-- ActionType) are the sole source of truth for workflow/action-type data. Postgres's
-- mirror of this data is fully retired: AuthWebhook owns the CRD admission/lifecycle
-- path entirely locally (no DS round-trip, Phase 8a-8d), and DS's read path is a pure
-- informer-backed cache over the CRDs (pkg/datastorage/workflowcache), with zero
-- remaining SQL queries against either table -- Phases A-C already deleted every
-- CRUD/read code path that touched them, including pkg/datastorage/repository/
-- actiontype in full.
--
-- Drop order is child-before-parent to avoid needing CASCADE: remediation_workflow_
-- catalog (holds the fk_workflow_action_type FK) before action_type_taxonomy (the
-- FK's referenced table). Dropping the table implicitly drops its own indexes,
-- constraints, and trigger.

DROP TABLE IF EXISTS remediation_workflow_catalog;
DROP TABLE IF EXISTS action_type_taxonomy;

-- update_workflow_catalog_updated_at() (migration 001) was remediation_workflow_
-- catalog's own trigger function -- safe to drop now that its only table is gone.
-- update_updated_at() is NOT dropped: it's shared with action_histories/
-- resource_action_traces/oscillation_patterns (migration 001), all still live.
DROP FUNCTION IF EXISTS update_workflow_catalog_updated_at();

-- +goose Down
-- Best-effort schema restoration reflecting the final pre-drop state: migration
-- 001's base schema + migration 002's service_account_name column + migration 003's
-- PascalCase status convention + migration 015's success-metrics column removal.
-- Data is NOT restored -- restore from a pre-migration Postgres backup if needed.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS action_type_taxonomy (
    action_type TEXT PRIMARY KEY,
    description JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'Active',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE action_type_taxonomy IS 'Curated taxonomy of remediation action types (DD-WORKFLOW-016)';
COMMENT ON COLUMN action_type_taxonomy.action_type IS 'Action type identifier (e.g., ScaleReplicas, RestartPod)';
COMMENT ON COLUMN action_type_taxonomy.description IS 'JSONB with camelCase keys: what, whenToUse, whenNotToUse, preconditions';
COMMENT ON COLUMN action_type_taxonomy.status IS 'Lifecycle status: Active, Disabled, Deprecated, Archived, or Superseded';
COMMENT ON COLUMN action_type_taxonomy.disabled_at IS 'Timestamp when action type was soft-disabled';
COMMENT ON COLUMN action_type_taxonomy.disabled_by IS 'Identity (K8s SA or user) who disabled the action type';

CREATE TRIGGER trigger_action_type_taxonomy_updated_at BEFORE UPDATE ON action_type_taxonomy FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS remediation_workflow_catalog (
    workflow_id UUID NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner VARCHAR(255),
    maintainer VARCHAR(255),
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    parameters JSONB,
    execution_engine VARCHAR(50) NOT NULL DEFAULT 'tekton',
    schema_image TEXT,
    schema_digest VARCHAR(71),
    execution_bundle TEXT,
    execution_bundle_digest VARCHAR(71),
    custom_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    detected_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    engine_config JSONB,
    action_type TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'Active',
    status_reason TEXT,
    schema_version VARCHAR(10) NOT NULL DEFAULT '1.0',
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
    service_account_name TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    CONSTRAINT remediation_workflow_catalog_status_check CHECK (status IN ('Active', 'Disabled', 'Deprecated', 'Archived', 'Superseded')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CONSTRAINT fk_workflow_action_type FOREIGN KEY (action_type) REFERENCES action_type_taxonomy(action_type)
);

CREATE INDEX idx_workflow_catalog_status ON remediation_workflow_catalog(status);
CREATE INDEX idx_workflow_catalog_latest_by_name ON remediation_workflow_catalog(workflow_name, is_latest_version) WHERE is_latest_version = true;
CREATE INDEX idx_workflow_catalog_workflow_name ON remediation_workflow_catalog(workflow_name);
CREATE INDEX idx_workflow_catalog_labels ON remediation_workflow_catalog USING GIN (labels);
CREATE INDEX idx_workflow_catalog_created_at ON remediation_workflow_catalog(created_at DESC);
CREATE INDEX idx_workflow_catalog_schema_digest ON remediation_workflow_catalog(schema_digest) WHERE schema_digest IS NOT NULL;
CREATE INDEX idx_workflow_catalog_execution_bundle_digest ON remediation_workflow_catalog(execution_bundle_digest) WHERE execution_bundle_digest IS NOT NULL;
CREATE INDEX idx_workflow_catalog_custom_labels ON remediation_workflow_catalog USING GIN (custom_labels);
CREATE INDEX idx_workflow_catalog_detected_labels ON remediation_workflow_catalog USING GIN (detected_labels);
CREATE INDEX idx_workflow_action_type_status_version ON remediation_workflow_catalog(action_type, status, is_latest_version);
CREATE UNIQUE INDEX uq_workflow_name_version_active ON remediation_workflow_catalog (workflow_name, version) WHERE status = 'Active';

COMMENT ON COLUMN remediation_workflow_catalog.description IS 'JSONB with camelCase keys (what, whenToUse, whenNotToUse, preconditions)';
COMMENT ON COLUMN remediation_workflow_catalog.schema_image IS 'OCI image pulled at registration to extract /workflow-schema.yaml';
COMMENT ON COLUMN remediation_workflow_catalog.schema_digest IS 'SHA256 digest of the schema image';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle IS 'OCI execution bundle reference (digest-pinned) for Tekton/Job runtime';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle_digest IS 'SHA256 digest portion of execution_bundle';
COMMENT ON COLUMN remediation_workflow_catalog.labels IS 'JSONB labels use signalName key for semantic signal matching';
COMMENT ON COLUMN remediation_workflow_catalog.engine_config IS 'Engine-specific configuration as JSONB (e.g., ansible playbookPath, inventoryName, jobTemplateName). NULL for tekton/job.';

CREATE TRIGGER trigger_workflow_catalog_updated_at BEFORE UPDATE ON remediation_workflow_catalog FOR EACH ROW EXECUTE FUNCTION update_workflow_catalog_updated_at();
