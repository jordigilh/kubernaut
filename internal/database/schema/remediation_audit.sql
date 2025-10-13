-- Remediation Audit Schema
-- BR-STORAGE-001: Audit trail for remediation workflows

-- Enable pgvector extension for embedding storage
CREATE EXTENSION IF NOT EXISTS vector;

-- Create remediation_audit table
CREATE TABLE IF NOT EXISTS remediation_audit (
    -- Primary key
    id BIGSERIAL PRIMARY KEY,

    -- Core identification
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    phase VARCHAR(50) NOT NULL, -- pending, processing, completed, failed
    action_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,

    -- Timing information
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    duration BIGINT, -- milliseconds

    -- Relationships
    remediation_request_id VARCHAR(255) NOT NULL,
    alert_fingerprint VARCHAR(255) NOT NULL,

    -- Context
    severity VARCHAR(50) NOT NULL,
    environment VARCHAR(50) NOT NULL,
    cluster_name VARCHAR(255) NOT NULL,
    target_resource VARCHAR(512) NOT NULL,

    -- Error tracking
    error_message TEXT,

    -- Metadata (JSON)
    metadata TEXT NOT NULL DEFAULT '{}',

    -- Embedding for semantic search
    embedding vector(384),

    -- Audit timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_remediation_audit_namespace ON remediation_audit(namespace);
CREATE INDEX IF NOT EXISTS idx_remediation_audit_status ON remediation_audit(status);
CREATE INDEX IF NOT EXISTS idx_remediation_audit_phase ON remediation_audit(phase);
CREATE INDEX IF NOT EXISTS idx_remediation_audit_start_time ON remediation_audit(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_remediation_audit_request_id ON remediation_audit(remediation_request_id);

-- Create HNSW index for vector similarity search
-- Using HNSW (Hierarchical Navigable Small World) for fast approximate nearest neighbor search
CREATE INDEX IF NOT EXISTS idx_remediation_audit_embedding ON remediation_audit
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Create trigger function to auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_remediation_audit_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger
DROP TRIGGER IF EXISTS trigger_remediation_audit_updated_at ON remediation_audit;
CREATE TRIGGER trigger_remediation_audit_updated_at
    BEFORE UPDATE ON remediation_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_remediation_audit_updated_at();
