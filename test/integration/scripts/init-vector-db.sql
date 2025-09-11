-- Integration Test Vector Database Initialization
-- This script sets up the vector database extensions and schemas for integration testing

\echo 'Setting up vector database for integration testing...'

-- Create vector extension if it doesn't exist
CREATE EXTENSION IF NOT EXISTS vector;

-- Create vector-specific schemas
CREATE SCHEMA IF NOT EXISTS vector_operations;
CREATE SCHEMA IF NOT EXISTS pattern_analysis;

-- Grant permissions for integration test user
GRANT USAGE ON SCHEMA vector_operations TO slm_user;
GRANT USAGE ON SCHEMA pattern_analysis TO slm_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA vector_operations TO slm_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA pattern_analysis TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA vector_operations TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA pattern_analysis TO slm_user;

-- Create test tables for vector operations
CREATE TABLE IF NOT EXISTS action_patterns (
    id SERIAL PRIMARY KEY,
    pattern_id VARCHAR(255) UNIQUE NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    effectiveness_score FLOAT NOT NULL DEFAULT 0.0,
    confidence_score FLOAT NOT NULL DEFAULT 0.0,
    context_metadata JSONB,
    embedding vector(384), -- Default dimension for integration tests
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient vector searches
CREATE INDEX IF NOT EXISTS action_patterns_embedding_idx
    ON action_patterns USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX IF NOT EXISTS action_patterns_action_type_idx ON action_patterns(action_type);
CREATE INDEX IF NOT EXISTS action_patterns_effectiveness_idx ON action_patterns(effectiveness_score DESC);
CREATE INDEX IF NOT EXISTS action_patterns_created_idx ON action_patterns(created_at DESC);

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for updated_at
DROP TRIGGER IF EXISTS update_action_patterns_updated_at ON action_patterns;
CREATE TRIGGER update_action_patterns_updated_at
    BEFORE UPDATE ON action_patterns
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert some test data for integration tests
INSERT INTO action_patterns (pattern_id, action_type, effectiveness_score, confidence_score, context_metadata, embedding) VALUES
    ('test-pattern-1', 'scale_deployment', 0.85, 0.9, '{"namespace": "production", "resource_type": "deployment"}', '[0.1,0.2,0.3]'::vector),
    ('test-pattern-2', 'increase_resources', 0.75, 0.8, '{"namespace": "staging", "resource_type": "pod"}', '[0.4,0.5,0.6]'::vector),
    ('test-pattern-3', 'restart_service', 0.65, 0.7, '{"namespace": "development", "resource_type": "service"}', '[0.7,0.8,0.9]'::vector)
ON CONFLICT (pattern_id) DO NOTHING;

\echo 'Vector database setup complete for integration testing.'
