-- Data Storage Service - Minimal Schema V1
-- Purpose: Unblock Context API integration tests (TDD Phase 1)
-- Authority: Derived from Context API query requirements
-- Status: Minimal schema - will evolve with Write API (BR-STORAGE-001 to BR-STORAGE-020)
--
-- Tables:
--   1. resource_references - Kubernetes resource metadata
--   2. action_histories - Action history metadata
--   3. resource_action_traces - Main audit trail with embeddings
--
-- Note: This schema is derived from Context API's SQL queries in:
--   - pkg/contextapi/sqlbuilder/builder.go
--   - pkg/contextapi/query/executor.go
--   - pkg/contextapi/query/aggregation.go
-- V1.0: Label-only architecture (DD-WORKFLOW-015). No pgvector/semantic search.

-- Table 1: resource_references
-- Stores Kubernetes resource metadata (namespace, kind, name)
CREATE TABLE IF NOT EXISTS resource_references (
    id BIGSERIAL PRIMARY KEY,
    resource_uid VARCHAR(255) NOT NULL UNIQUE,  -- Unique resource identifier
    api_version VARCHAR(100),
    kind VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for resource_references
CREATE INDEX IF NOT EXISTS idx_rr_resource_uid ON resource_references(resource_uid);
CREATE INDEX IF NOT EXISTS idx_rr_namespace ON resource_references(namespace);
CREATE INDEX IF NOT EXISTS idx_rr_kind ON resource_references(kind);
CREATE INDEX IF NOT EXISTS idx_rr_name ON resource_references(name);

-- Table 2: action_histories
-- Stores action history metadata linking to resource_references
CREATE TABLE IF NOT EXISTS action_histories (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,
    max_actions INTEGER DEFAULT 1000,
    max_age_days INTEGER DEFAULT 30,
    total_actions INTEGER DEFAULT 0,
    last_action_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for action_histories
CREATE INDEX IF NOT EXISTS idx_ah_resource_id ON action_histories(resource_id);
CREATE INDEX IF NOT EXISTS idx_ah_last_action_at ON action_histories(last_action_at DESC);

-- Table 3: resource_action_traces
-- Main audit trail table
-- Contains all incident/action data queried by Context API
CREATE TABLE IF NOT EXISTS resource_action_traces (
    id BIGSERIAL PRIMARY KEY,

    -- Foreign key to action_histories
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,

    -- Primary identification
    action_id VARCHAR(255),  -- Remediation request ID
    alert_name VARCHAR(255) NOT NULL,
    alert_fingerprint VARCHAR(255),

    -- Severity and classification
    alert_severity VARCHAR(50),  -- critical, high, medium, low
    action_type VARCHAR(100),    -- scale, restart, delete, etc.

    -- Timing
    action_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Execution tracking
    execution_status VARCHAR(50),  -- completed, failed, rolled-back, pending, executing
    execution_end_time TIMESTAMP WITH TIME ZONE,
    execution_duration_ms INTEGER,
    execution_error TEXT,

    -- Metadata
    action_parameters JSONB,  -- Metadata stored as JSON
    cluster_name VARCHAR(255),
    environment VARCHAR(50),

    -- Audit timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for resource_action_traces (optimized for Context API queries)
CREATE INDEX IF NOT EXISTS idx_rat_action_history_id ON resource_action_traces(action_history_id);
CREATE INDEX IF NOT EXISTS idx_rat_alert_name ON resource_action_traces(alert_name);
CREATE INDEX IF NOT EXISTS idx_rat_alert_severity ON resource_action_traces(alert_severity);
CREATE INDEX IF NOT EXISTS idx_rat_action_type ON resource_action_traces(action_type);
CREATE INDEX IF NOT EXISTS idx_rat_execution_status ON resource_action_traces(execution_status);
CREATE INDEX IF NOT EXISTS idx_rat_action_timestamp ON resource_action_traces(action_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_rat_cluster_name ON resource_action_traces(cluster_name);
CREATE INDEX IF NOT EXISTS idx_rat_environment ON resource_action_traces(environment);

-- Trigger function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers to automatically update updated_at on all tables
DROP TRIGGER IF EXISTS trigger_rr_updated_at ON resource_references;
CREATE TRIGGER trigger_rr_updated_at
    BEFORE UPDATE ON resource_references
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_timestamp();

DROP TRIGGER IF EXISTS trigger_ah_updated_at ON action_histories;
CREATE TRIGGER trigger_ah_updated_at
    BEFORE UPDATE ON action_histories
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_timestamp();

DROP TRIGGER IF EXISTS trigger_rat_updated_at ON resource_action_traces;
CREATE TRIGGER trigger_rat_updated_at
    BEFORE UPDATE ON resource_action_traces
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_timestamp();

-- Verification query (should return 3)
SELECT COUNT(*) as table_count FROM information_schema.tables
WHERE table_schema = 'public'
AND table_name IN ('resource_references', 'action_histories', 'resource_action_traces');

