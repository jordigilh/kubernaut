-- Integration Test Vector Store Database Initialization
-- This script sets up a dedicated vector store database for integration testing

\echo 'Setting up dedicated vector store database for integration testing...'

-- Create vector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Create schemas for vector store operations
CREATE SCHEMA IF NOT EXISTS embeddings;
CREATE SCHEMA IF NOT EXISTS similarity_search;

-- Grant permissions for vector user
GRANT USAGE ON SCHEMA embeddings TO vector_user;
GRANT USAGE ON SCHEMA similarity_search TO vector_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA embeddings TO vector_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA similarity_search TO vector_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA embeddings TO vector_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA similarity_search TO vector_user;

-- Create embedding storage table
CREATE TABLE IF NOT EXISTS embeddings.document_embeddings (
    id SERIAL PRIMARY KEY,
    document_id VARCHAR(255) UNIQUE NOT NULL,
    content TEXT NOT NULL,
    embedding vector(384), -- Standard dimension for local embeddings
    metadata JSONB,
    source_type VARCHAR(100) NOT NULL DEFAULT 'integration_test',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create workflow pattern embeddings table
CREATE TABLE IF NOT EXISTS embeddings.workflow_pattern_embeddings (
    id SERIAL PRIMARY KEY,
    pattern_id VARCHAR(255) UNIQUE NOT NULL,
    workflow_type VARCHAR(100) NOT NULL,
    pattern_description TEXT,
    success_rate FLOAT NOT NULL DEFAULT 0.0,
    usage_count INTEGER NOT NULL DEFAULT 0,
    embedding vector(384),
    context JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create alert pattern embeddings table
CREATE TABLE IF NOT EXISTS embeddings.alert_pattern_embeddings (
    id SERIAL PRIMARY KEY,
    alert_pattern_id VARCHAR(255) UNIQUE NOT NULL,
    alert_type VARCHAR(100) NOT NULL,
    resolution_pattern TEXT,
    effectiveness_score FLOAT NOT NULL DEFAULT 0.0,
    embedding vector(384),
    alert_metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient vector operations
CREATE INDEX IF NOT EXISTS document_embeddings_vector_idx
    ON embeddings.document_embeddings USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 50);

CREATE INDEX IF NOT EXISTS workflow_pattern_embeddings_vector_idx
    ON embeddings.workflow_pattern_embeddings USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 50);

CREATE INDEX IF NOT EXISTS alert_pattern_embeddings_vector_idx
    ON embeddings.alert_pattern_embeddings USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 50);

-- Create additional indexes for filtering
CREATE INDEX IF NOT EXISTS document_embeddings_source_type_idx ON embeddings.document_embeddings(source_type);
CREATE INDEX IF NOT EXISTS workflow_pattern_embeddings_type_idx ON embeddings.workflow_pattern_embeddings(workflow_type);
CREATE INDEX IF NOT EXISTS alert_pattern_embeddings_type_idx ON embeddings.alert_pattern_embeddings(alert_type);

-- Create similarity search functions
CREATE OR REPLACE FUNCTION similarity_search.find_similar_documents(
    query_embedding vector(384),
    similarity_threshold float DEFAULT 0.7,
    max_results integer DEFAULT 10
)
RETURNS TABLE (
    document_id varchar,
    content text,
    similarity_score float,
    metadata jsonb
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        d.document_id,
        d.content,
        1 - (d.embedding <=> query_embedding) as similarity_score,
        d.metadata
    FROM embeddings.document_embeddings d
    WHERE 1 - (d.embedding <=> query_embedding) > similarity_threshold
    ORDER BY d.embedding <=> query_embedding
    LIMIT max_results;
END;
$$ LANGUAGE plpgsql;

-- Insert test data for integration testing
INSERT INTO embeddings.document_embeddings (document_id, content, embedding, metadata) VALUES
    ('test-doc-1', 'High memory usage detected in production deployment', '[0.1,0.2,0.3]'::vector, '{"type": "memory_alert", "severity": "high"}'),
    ('test-doc-2', 'CPU throttling observed in kubernetes pods', '[0.4,0.5,0.6]'::vector, '{"type": "cpu_alert", "severity": "medium"}'),
    ('test-doc-3', 'Disk space running low on worker nodes', '[0.7,0.8,0.9]'::vector, '{"type": "disk_alert", "severity": "high"}')
ON CONFLICT (document_id) DO NOTHING;

INSERT INTO embeddings.workflow_pattern_embeddings (pattern_id, workflow_type, pattern_description, success_rate, embedding, context) VALUES
    ('workflow-pattern-1', 'scaling', 'Horizontal pod autoscaling for high memory usage', 0.85, '[0.2,0.3,0.4]'::vector, '{"trigger": "memory_threshold", "action": "scale_out"}'),
    ('workflow-pattern-2', 'resource_management', 'Increase resource limits for CPU intensive workloads', 0.75, '[0.5,0.6,0.7]'::vector, '{"trigger": "cpu_throttling", "action": "increase_limits"}'),
    ('workflow-pattern-3', 'maintenance', 'Node drain and replacement for disk space issues', 0.65, '[0.8,0.9,0.1]'::vector, '{"trigger": "disk_full", "action": "node_maintenance"}')
ON CONFLICT (pattern_id) DO NOTHING;

\echo 'Vector store database setup complete for integration testing.'
