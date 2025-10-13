-- AI Analysis Audit Schema
-- BR-STORAGE-002: Audit trail for AI analysis

CREATE TABLE IF NOT EXISTS ai_analysis_audit (
    -- Primary key
    id BIGSERIAL PRIMARY KEY,

    -- Relationships
    remediation_request_id VARCHAR(255) NOT NULL,
    analysis_id VARCHAR(255) NOT NULL UNIQUE,

    -- AI Provider information
    provider VARCHAR(100) NOT NULL, -- holmesgpt, openai, claude
    model VARCHAR(255) NOT NULL,

    -- Analysis metrics
    confidence_score DOUBLE PRECISION NOT NULL CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0),
    tokens_used INTEGER NOT NULL CHECK (tokens_used >= 0),
    analysis_duration BIGINT NOT NULL CHECK (analysis_duration >= 0), -- milliseconds

    -- Metadata (JSON)
    metadata TEXT NOT NULL DEFAULT '{}',

    -- Audit timestamp
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_request_id ON ai_analysis_audit(remediation_request_id);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_provider ON ai_analysis_audit(provider);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_created_at ON ai_analysis_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_confidence ON ai_analysis_audit(confidence_score DESC);
