-- Enhanced Integration Test Vector Store Database Initialization
-- This script sets up a comprehensive vector store database for integration testing
-- Following project guidelines: structured field values, business requirement alignment

\echo 'Setting up enhanced vector store database for integration testing...'

-- Create vector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Create schemas for vector store operations
CREATE SCHEMA IF NOT EXISTS embeddings;
CREATE SCHEMA IF NOT EXISTS similarity_search;
CREATE SCHEMA IF NOT EXISTS action_patterns;

-- Grant permissions for vector user
GRANT USAGE ON SCHEMA embeddings TO vector_user;
GRANT USAGE ON SCHEMA similarity_search TO vector_user;
GRANT USAGE ON SCHEMA action_patterns TO vector_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA embeddings TO vector_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA similarity_search TO vector_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA action_patterns TO vector_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA embeddings TO vector_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA similarity_search TO vector_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA action_patterns TO vector_user;

-- Create the main action_patterns table (matching the Go ActionPattern struct)
CREATE TABLE IF NOT EXISTS action_patterns (
    id VARCHAR(255) PRIMARY KEY,
    description TEXT,  -- Added: matches ActionPattern.Description
    action_type VARCHAR(100) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    alert_severity VARCHAR(50) NOT NULL,
    namespace VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_name VARCHAR(255),
    context TEXT,
    action_parameters JSONB DEFAULT '{}',
    context_labels JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    effectiveness_data JSONB DEFAULT '{"score": 0.0, "success_count": 0, "failure_count": 0}',  -- Fixed: match Go struct
    pre_conditions JSONB DEFAULT '{}',  -- Added: missing column causing the error
    post_conditions JSONB DEFAULT '{}', -- Added: commonly expected field
    tags TEXT[] DEFAULT '{}',  -- Added: commonly used for pattern categorization
    embedding vector(1536),  -- Updated: match controlled embedding generator dimension
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient vector operations on action_patterns
CREATE INDEX IF NOT EXISTS action_patterns_embedding_idx
    ON action_patterns USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 50);

CREATE INDEX IF NOT EXISTS action_patterns_action_type_idx ON action_patterns(action_type);
CREATE INDEX IF NOT EXISTS action_patterns_alert_name_idx ON action_patterns(alert_name);
CREATE INDEX IF NOT EXISTS action_patterns_alert_severity_idx ON action_patterns(alert_severity);
CREATE INDEX IF NOT EXISTS action_patterns_namespace_idx ON action_patterns(namespace);
CREATE INDEX IF NOT EXISTS action_patterns_resource_type_idx ON action_patterns(resource_type);
CREATE INDEX IF NOT EXISTS action_patterns_created_at_idx ON action_patterns(created_at);

-- Create GIN indexes for JSONB fields
CREATE INDEX IF NOT EXISTS action_patterns_action_parameters_gin_idx ON action_patterns USING GIN (action_parameters);
CREATE INDEX IF NOT EXISTS action_patterns_context_labels_gin_idx ON action_patterns USING GIN (context_labels);
CREATE INDEX IF NOT EXISTS action_patterns_metadata_gin_idx ON action_patterns USING GIN (metadata);

-- Create effectiveness score index for performance queries
CREATE INDEX IF NOT EXISTS action_patterns_effectiveness_score_idx
    ON action_patterns((effectiveness_data->>'score')::float);

-- Create embedding storage table for documents
CREATE TABLE IF NOT EXISTS embeddings.document_embeddings (
    id SERIAL PRIMARY KEY,
    document_id VARCHAR(255) UNIQUE NOT NULL,
    content TEXT NOT NULL,
    embedding vector(384),
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

-- Create function to find similar action patterns
CREATE OR REPLACE FUNCTION similarity_search.find_similar_action_patterns(
    query_embedding vector(384),
    similarity_threshold float DEFAULT 0.7,
    max_results integer DEFAULT 10
)
RETURNS TABLE (
    id varchar,
    action_type varchar,
    alert_name varchar,
    similarity_score float,
    effectiveness_score float
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        ap.id,
        ap.action_type,
        ap.alert_name,
        1 - (ap.embedding <=> query_embedding) as similarity_score,
        (ap.effectiveness_data->>'score')::float as effectiveness_score
    FROM action_patterns ap
    WHERE ap.embedding IS NOT NULL
      AND 1 - (ap.embedding <=> query_embedding) > similarity_threshold
    ORDER BY ap.embedding <=> query_embedding
    LIMIT max_results;
END;
$$ LANGUAGE plpgsql;

-- Create function to generate a sample 384-dimensional vector for testing
CREATE OR REPLACE FUNCTION generate_test_embedding(seed_text text)
RETURNS vector(384) AS $$
DECLARE
    result_vector float[];
    i integer;
    hash_value bigint;
BEGIN
    -- Generate a deterministic hash from the seed text
    hash_value := abs(hashtext(seed_text));

    -- Initialize array
    result_vector := array_fill(0.0, ARRAY[384]);

    -- Generate pseudo-random values based on the hash
    FOR i IN 1..384 LOOP
        result_vector[i] := (((hash_value * i * 31) % 1000000) / 1000000.0) - 0.5;
    END LOOP;

    RETURN result_vector::vector(384);
END;
$$ LANGUAGE plpgsql;

-- Insert comprehensive test data for integration testing
INSERT INTO embeddings.document_embeddings (document_id, content, embedding, metadata) VALUES
    ('test-doc-1', 'High memory usage detected in production deployment', generate_test_embedding('memory_alert_high'), '{"type": "memory_alert", "severity": "high", "namespace": "production"}'),
    ('test-doc-2', 'CPU throttling observed in kubernetes pods', generate_test_embedding('cpu_throttling_medium'), '{"type": "cpu_alert", "severity": "medium", "namespace": "staging"}'),
    ('test-doc-3', 'Disk space running low on worker nodes', generate_test_embedding('disk_space_low'), '{"type": "disk_alert", "severity": "high", "namespace": "production"}'),
    ('test-doc-4', 'Network connectivity issues between services', generate_test_embedding('network_connectivity'), '{"type": "network_alert", "severity": "critical", "namespace": "production"}'),
    ('test-doc-5', 'Pod restart loop detected in deployment', generate_test_embedding('pod_restart_loop'), '{"type": "pod_alert", "severity": "high", "namespace": "staging"}')
ON CONFLICT (document_id) DO NOTHING;

INSERT INTO embeddings.workflow_pattern_embeddings (pattern_id, workflow_type, pattern_description, success_rate, embedding, context) VALUES
    ('workflow-pattern-1', 'scaling', 'Horizontal pod autoscaling for high memory usage', 0.85, generate_test_embedding('scaling_memory'), '{"trigger": "memory_threshold", "action": "scale_out", "min_replicas": 2, "max_replicas": 10}'),
    ('workflow-pattern-2', 'resource_management', 'Increase resource limits for CPU intensive workloads', 0.75, generate_test_embedding('resource_cpu'), '{"trigger": "cpu_throttling", "action": "increase_limits", "cpu_limit": "2000m", "memory_limit": "4Gi"}'),
    ('workflow-pattern-3', 'maintenance', 'Node drain and replacement for disk space issues', 0.65, generate_test_embedding('maintenance_disk'), '{"trigger": "disk_full", "action": "node_maintenance", "drain_timeout": "300s"}'),
    ('workflow-pattern-4', 'network_recovery', 'Service mesh reconfiguration for connectivity issues', 0.80, generate_test_embedding('network_recovery'), '{"trigger": "network_failure", "action": "mesh_reconfigure", "timeout": "60s"}'),
    ('workflow-pattern-5', 'pod_recovery', 'Rolling restart for pod crash loops', 0.90, generate_test_embedding('pod_recovery'), '{"trigger": "crash_loop", "action": "rolling_restart", "max_unavailable": "25%"}')
ON CONFLICT (pattern_id) DO NOTHING;

INSERT INTO embeddings.alert_pattern_embeddings (alert_pattern_id, alert_type, resolution_pattern, effectiveness_score, embedding, alert_metadata) VALUES
    ('alert-pattern-1', 'HighMemoryUsage', 'Scale deployment horizontally', 0.85, generate_test_embedding('alert_memory'), '{"threshold": "80%", "duration": "5m", "severity": "warning"}'),
    ('alert-pattern-2', 'HighCPUUsage', 'Increase CPU limits and requests', 0.75, generate_test_embedding('alert_cpu'), '{"threshold": "90%", "duration": "3m", "severity": "critical"}'),
    ('alert-pattern-3', 'DiskSpaceLow', 'Clean up logs and temporary files', 0.70, generate_test_embedding('alert_disk'), '{"threshold": "85%", "duration": "1m", "severity": "warning"}'),
    ('alert-pattern-4', 'PodCrashLooping', 'Investigate and restart with updated configuration', 0.80, generate_test_embedding('alert_crash'), '{"restart_count": 5, "duration": "10m", "severity": "critical"}'),
    ('alert-pattern-5', 'ServiceUnavailable', 'Check service endpoints and restart if necessary', 0.88, generate_test_embedding('alert_service'), '{"timeout": "30s", "duration": "2m", "severity": "critical"}'
ON CONFLICT (alert_pattern_id) DO NOTHING;

-- Insert test action patterns for integration testing
INSERT INTO action_patterns (id, action_type, alert_name, alert_severity, namespace, resource_type, resource_name, context, action_parameters, effectiveness_data, embedding) VALUES
    ('test-action-1', 'scale_deployment', 'HighMemoryUsage', 'warning', 'production', 'Deployment', 'web-app', 'Memory usage exceeded threshold', '{"replicas": 5, "target_cpu": "70%"}', '{"score": 0.85, "executions": 10, "successes": 8}', generate_test_embedding('action_scale_memory')),
    ('test-action-2', 'restart_pods', 'PodCrashLooping', 'critical', 'staging', 'Pod', 'api-service', 'Pod in crash loop backoff', '{"grace_period": "30s", "force": false}', '{"score": 0.75, "executions": 8, "successes": 6}', generate_test_embedding('action_restart_pod')),
    ('test-action-3', 'increase_resources', 'HighCPUUsage', 'critical', 'production', 'Deployment', 'worker', 'CPU usage consistently high', '{"cpu_limit": "2000m", "memory_limit": "4Gi"}', '{"score": 0.90, "executions": 12, "successes": 11}', generate_test_embedding('action_increase_cpu')),
    ('test-action-4', 'drain_node', 'DiskSpaceLow', 'warning', 'production', 'Node', 'worker-node-1', 'Disk space critically low', '{"timeout": "300s", "force": false}', '{"score": 0.65, "executions": 5, "successes": 3}', generate_test_embedding('action_drain_node')),
    ('test-action-5', 'update_configmap', 'ServiceUnavailable', 'critical', 'staging', 'ConfigMap', 'app-config', 'Service configuration needs update', '{"key": "timeout", "value": "60s"}', '{"score": 0.80, "executions": 6, "successes": 5}', generate_test_embedding('action_update_config'))
ON CONFLICT (id) DO NOTHING;

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for action_patterns updated_at
DROP TRIGGER IF EXISTS action_patterns_updated_at_trigger ON action_patterns;
CREATE TRIGGER action_patterns_updated_at_trigger
    BEFORE UPDATE ON action_patterns
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create analytics view for pattern effectiveness
CREATE OR REPLACE VIEW pattern_analytics_summary AS
SELECT
    action_type,
    alert_name,
    COUNT(*) as pattern_count,
    AVG((effectiveness_data->>'score')::float) as avg_effectiveness_score,
    SUM((effectiveness_data->>'executions')::int) as total_executions,
    SUM((effectiveness_data->>'successes')::int) as total_successes,
    CASE
        WHEN SUM((effectiveness_data->>'executions')::int) > 0
        THEN SUM((effectiveness_data->>'successes')::int)::float / SUM((effectiveness_data->>'executions')::int)::float
        ELSE 0
    END as overall_success_rate
FROM action_patterns
GROUP BY action_type, alert_name
ORDER BY avg_effectiveness_score DESC;

-- Grant permissions on the view
GRANT SELECT ON pattern_analytics_summary TO vector_user;

-- Create test validation function
CREATE OR REPLACE FUNCTION validate_vector_database_setup()
RETURNS TABLE (
    component varchar,
    status varchar,
    count_or_message varchar
) AS $$
BEGIN
    -- Check extension
    RETURN QUERY
    SELECT 'pgvector_extension'::varchar, 'installed'::varchar, extversion::varchar
    FROM pg_extension WHERE extname = 'vector';

    -- Check schemas
    RETURN QUERY
    SELECT 'schemas'::varchar, 'created'::varchar, COUNT(*)::varchar
    FROM information_schema.schemata
    WHERE schema_name IN ('embeddings', 'similarity_search', 'action_patterns');

    -- Check tables
    RETURN QUERY
    SELECT 'tables'::varchar, 'created'::varchar, COUNT(*)::varchar
    FROM information_schema.tables
    WHERE table_schema IN ('embeddings', 'public')
    AND table_name IN ('document_embeddings', 'workflow_pattern_embeddings', 'alert_pattern_embeddings', 'action_patterns');

    -- Check test data
    RETURN QUERY
    SELECT 'test_documents'::varchar, 'inserted'::varchar, COUNT(*)::varchar
    FROM embeddings.document_embeddings;

    RETURN QUERY
    SELECT 'test_workflows'::varchar, 'inserted'::varchar, COUNT(*)::varchar
    FROM embeddings.workflow_pattern_embeddings;

    RETURN QUERY
    SELECT 'test_alerts'::varchar, 'inserted'::varchar, COUNT(*)::varchar
    FROM embeddings.alert_pattern_embeddings;

    RETURN QUERY
    SELECT 'test_actions'::varchar, 'inserted'::varchar, COUNT(*)::varchar
    FROM action_patterns;
END;
$$ LANGUAGE plpgsql;

-- Grant execute permission on validation function
GRANT EXECUTE ON FUNCTION validate_vector_database_setup() TO vector_user;
GRANT EXECUTE ON FUNCTION generate_test_embedding(text) TO vector_user;
GRANT EXECUTE ON FUNCTION similarity_search.find_similar_documents(vector(384), float, integer) TO vector_user;
GRANT EXECUTE ON FUNCTION similarity_search.find_similar_action_patterns(vector(384), float, integer) TO vector_user;

\echo 'Enhanced vector store database setup complete for integration testing.'
\echo 'Run SELECT * FROM validate_vector_database_setup(); to verify the setup.'
