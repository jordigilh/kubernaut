-- Vector Database Schema for AI-driven Action Pattern Recognition
-- Date: 2025-01-03
-- Description: Adds pgvector extension and action_patterns table for vector-based pattern storage

-- Enable pgvector extension for vector operations
CREATE EXTENSION IF NOT EXISTS vector;

-- Create action_patterns table for storing action patterns as vectors
CREATE TABLE action_patterns (
    id VARCHAR(255) PRIMARY KEY,
    action_type VARCHAR(255) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    alert_severity VARCHAR(50) NOT NULL,
    namespace VARCHAR(255),
    resource_type VARCHAR(255),
    resource_name VARCHAR(255),

    -- Complex data stored as JSONB
    action_parameters JSONB,
    context_labels JSONB,
    pre_conditions JSONB,
    post_conditions JSONB,
    effectiveness_data JSONB,
    metadata JSONB,

    -- Vector embedding for similarity search (384 dimensions for sentence-transformers/all-MiniLM-L6-v2)
    embedding vector(384),

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create vector similarity index using IVFFlat for efficient similarity search
-- Lists parameter is set to 100, good for up to 100K vectors
CREATE INDEX action_patterns_embedding_idx
ON action_patterns
USING ivfflat (embedding vector_l2_ops)
WITH (lists = 100);

-- Traditional indexes for filtering and performance
CREATE INDEX action_patterns_action_type_idx ON action_patterns(action_type);
CREATE INDEX action_patterns_alert_name_idx ON action_patterns(alert_name);
CREATE INDEX action_patterns_alert_severity_idx ON action_patterns(alert_severity);
CREATE INDEX action_patterns_namespace_idx ON action_patterns(namespace);
CREATE INDEX action_patterns_resource_type_idx ON action_patterns(resource_type);
CREATE INDEX action_patterns_created_at_idx ON action_patterns(created_at);

-- Partial index for patterns with effectiveness data
CREATE INDEX action_patterns_effectiveness_score_idx
ON action_patterns((effectiveness_data->>'score'))
WHERE effectiveness_data->>'score' IS NOT NULL;

-- GIN indexes for JSONB fields to enable efficient JSON queries
CREATE INDEX action_patterns_action_parameters_gin_idx ON action_patterns USING gin(action_parameters);
CREATE INDEX action_patterns_context_labels_gin_idx ON action_patterns USING gin(context_labels);
CREATE INDEX action_patterns_metadata_gin_idx ON action_patterns USING gin(metadata);

-- Function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_action_patterns_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language plpgsql;

-- Trigger to automatically update updated_at on row updates
CREATE TRIGGER action_patterns_updated_at_trigger
    BEFORE UPDATE ON action_patterns
    FOR EACH ROW
    EXECUTE FUNCTION update_action_patterns_updated_at();

-- Create view for pattern analytics
CREATE VIEW pattern_analytics_summary AS
SELECT
    COUNT(*) as total_patterns,
    COUNT(DISTINCT action_type) as unique_action_types,
    COUNT(DISTINCT alert_name) as unique_alert_names,
    COUNT(DISTINCT alert_severity) as unique_severities,
    COUNT(DISTINCT namespace) as unique_namespaces,
    COUNT(DISTINCT resource_type) as unique_resource_types,
    AVG((effectiveness_data->>'score')::float) as avg_effectiveness_score,
    COUNT(*) FILTER (WHERE effectiveness_data->>'score' IS NOT NULL) as patterns_with_effectiveness,
    MIN(created_at) as oldest_pattern,
    MAX(created_at) as newest_pattern
FROM action_patterns;

-- Add comments for documentation
COMMENT ON TABLE action_patterns IS 'Stores action patterns with vector embeddings for AI-driven pattern recognition';
COMMENT ON COLUMN action_patterns.embedding IS 'Vector embedding (384-dimensional) for similarity search using pgvector';
COMMENT ON COLUMN action_patterns.effectiveness_data IS 'JSONB containing effectiveness metrics and scoring data';
COMMENT ON INDEX action_patterns_embedding_idx IS 'IVFFlat index for efficient vector similarity search using L2 distance';
