-- Context API Integration Test Data
-- Purpose: Insert test data for Context API integration testing using Data Storage Service schema
-- Database: PostgreSQL 15+ with pgvector extension
-- Schema: resource_action_traces + action_histories + resource_references (Data Storage Service)
--
-- Schema Authority: Data Storage Service (DD-SCHEMA-001)
-- Migration Dependencies: 001-007 (base schema) + 008 (Context API compatibility fields)
--
-- NOTE: This script assumes Data Storage schema already exists
-- Schema is created by Data Storage Service migrations
-- See: migrations/001_initial_schema.sql through 008_context_api_compatibility.sql

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- TEST DATA INSERTION (Data Storage Service Schema)
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- Clean up any existing test data (for test isolation)
DELETE FROM resource_action_traces WHERE action_id LIKE 'test-rr-%';
DELETE FROM action_histories WHERE id IN (
    SELECT ah.id FROM action_histories ah
    JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.resource_uid LIKE 'test-uid-%'
);
DELETE FROM resource_references WHERE resource_uid LIKE 'test-uid-%';

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- STEP 1: Insert Resource References (Kubernetes resources)
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INSERT INTO resource_references (resource_uid, api_version, kind, name, namespace, created_at, last_seen)
VALUES
    -- Production resources
    ('test-uid-001', 'apps/v1', 'Deployment', 'webapp', 'production', NOW(), NOW()),
    ('test-uid-002', 'v1', 'Pod', 'webapp-7d9f6b', 'production', NOW(), NOW()),
    ('test-uid-003', 'v1', 'Node', 'worker-01', 'production', NOW(), NOW()),
    ('test-uid-004', 'apps/v1', 'Deployment', 'api-service', 'production', NOW(), NOW()),
    ('test-uid-005', 'v1', 'Service', 'frontend', 'production', NOW(), NOW()),

    -- Staging resources
    ('test-uid-006', 'apps/v1', 'Deployment', 'database-proxy', 'staging', NOW(), NOW()),
    ('test-uid-007', 'v1', 'ConfigMap', 'rate-limits', 'staging', NOW(), NOW()),

    -- Development resources
    ('test-uid-008', 'v1', 'Secret', 'registry-creds', 'development', NOW(), NOW()),

    -- Monitoring/Logging resources
    ('test-uid-009', 'v1', 'Pod', 'prometheus-0', 'monitoring', NOW(), NOW()),
    ('test-uid-010', 'v1', 'Pod', 'logstash-0', 'logging', NOW(), NOW()),

    -- Additional production resources for trend analysis
    ('test-uid-011', 'v1', 'Node', 'worker-03', 'production', NOW(), NOW()),
    ('test-uid-012', 'v1', 'PersistentVolumeClaim', 'data-volume', 'production', NOW(), NOW());

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- STEP 2: Insert Action Histories (per-resource aggregated metrics)
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INSERT INTO action_histories (
    resource_id,
    max_actions,
    max_age_days,
    total_actions,
    last_action_at,
    created_at,
    updated_at
)
SELECT
    id,
    1000, -- max_actions
    30,   -- max_age_days
    1,    -- total_actions (will be updated by triggers in production)
    NOW(),
    NOW(),
    NOW()
FROM resource_references
WHERE resource_uid LIKE 'test-uid-%';

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- STEP 3: Insert Resource Action Traces (individual remediation actions)
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- Scenario 1: Successful remediations in production namespace
INSERT INTO resource_action_traces (
    action_history_id,
    action_id,
    action_timestamp,
    execution_start_time,
    execution_end_time,
    execution_duration_ms,
    alert_name,
    alert_severity,
    alert_fingerprint,
    alert_firing_time,
    model_used,
    model_confidence,
    action_type,
    action_parameters,
    execution_status,
    cluster_name,
    environment,
    embedding,
    created_at,
    updated_at
)
VALUES
-- High memory usage - successful remediation (webapp deployment)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-001'),
    'test-rr-001',
    NOW() - INTERVAL '2 hours',
    NOW() - INTERVAL '2 hours',
    NOW() - INTERVAL '1 hour 55 minutes',
    300000,
    'HighMemoryUsage',
    'critical',
    'fp-001',
    NOW() - INTERVAL '2 hours 5 minutes',
    'gpt-4',
    0.95,
    'scale_deployment',
    '{"description": "Pod memory usage exceeded 90% threshold", "replicas": 5}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Pod crash loop - successful restart (webapp pod)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-002'),
    'test-rr-002',
    NOW() - INTERVAL '1 hour',
    NOW() - INTERVAL '1 hour',
    NOW() - INTERVAL '55 minutes',
    180000,
    'PodCrashLoop',
    'critical',
    'fp-002',
    NOW() - INTERVAL '1 hour 5 minutes',
    'gpt-4',
    0.92,
    'restart_pod',
    '{"description": "Pod crash loop detected with exit code 137"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Node disk pressure - successful cleanup (worker-01 node)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-003'),
    'test-rr-003',
    NOW() - INTERVAL '30 minutes',
    NOW() - INTERVAL '30 minutes',
    NOW() - INTERVAL '25 minutes',
    120000,
    'NodeDiskPressure',
    'warning',
    'fp-003',
    NOW() - INTERVAL '35 minutes',
    'gpt-3.5-turbo',
    0.88,
    'cleanup_disk',
    '{"description": "Node disk usage exceeded 85% threshold"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 2: Failed remediations for failure analysis
-- Deployment failure (api-service deployment)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-004'),
    'test-rr-004',
    NOW() - INTERVAL '45 minutes',
    NOW() - INTERVAL '45 minutes',
    NOW() - INTERVAL '40 minutes',
    60000,
    'DeploymentFailure',
    'critical',
    'fp-004',
    NOW() - INTERVAL '50 minutes',
    'gpt-4',
    0.85,
    'rollback_deployment',
    '{"description": "Deployment rollback failed due to missing previous revision", "error": "revision not found"}'::jsonb,
    'failed',
    'prod-us-west-2',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Network connectivity issue (frontend service)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-005'),
    'test-rr-005',
    NOW() - INTERVAL '20 minutes',
    NOW() - INTERVAL '20 minutes',
    NOW() - INTERVAL '18 minutes',
    45000,
    'ServiceUnavailable',
    'critical',
    'fp-005',
    NOW() - INTERVAL '25 minutes',
    'gpt-4',
    0.90,
    'restart_service',
    '{"description": "Service restart failed due to network connectivity"}'::jsonb,
    'failed',
    'prod-us-west-2',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 3: Staging environment incidents
-- Database connection pool exhaustion (database-proxy deployment)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-006'),
    'test-rr-006',
    NOW() - INTERVAL '3 hours',
    NOW() - INTERVAL '3 hours',
    NOW() - INTERVAL '2 hours 50 minutes',
    240000,
    'DatabaseConnectionPoolExhaustion',
    'warning',
    'fp-006',
    NOW() - INTERVAL '3 hours 5 minutes',
    'gpt-3.5-turbo',
    0.87,
    'scale_deployment',
    '{"description": "Database connection pool reached 95% capacity"}'::jsonb,
    'completed',
    'staging-us-east-1',
    'staging',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- API rate limit exceeded (rate-limits configmap)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-007'),
    'test-rr-007',
    NOW() - INTERVAL '4 hours',
    NOW() - INTERVAL '4 hours',
    NOW() - INTERVAL '3 hours 45 minutes',
    180000,
    'APIRateLimitExceeded',
    'warning',
    'fp-007',
    NOW() - INTERVAL '4 hours 5 minutes',
    'gpt-3.5-turbo',
    0.85,
    'adjust_rate_limit',
    '{"description": "API rate limit exceeded 1000 requests per minute"}'::jsonb,
    'completed',
    'staging-us-east-1',
    'staging',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 4: Development environment incidents
-- Container image pull failure (registry-creds secret)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-008'),
    'test-rr-008',
    NOW() - INTERVAL '6 hours',
    NOW() - INTERVAL '6 hours',
    NOW() - INTERVAL '5 hours 50 minutes',
    120000,
    'ImagePullBackOff',
    'info',
    'fp-008',
    NOW() - INTERVAL '6 hours 5 minutes',
    'gpt-3.5-turbo',
    0.80,
    'update_image_pull_secret',
    '{"description": "Container image pull failed due to expired credentials"}'::jsonb,
    'completed',
    'dev-local',
    'development',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 5: Multiple namespaces for grouping tests
-- Namespace: monitoring (prometheus pod)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-009'),
    'test-rr-009',
    NOW() - INTERVAL '12 hours',
    NOW() - INTERVAL '12 hours',
    NOW() - INTERVAL '11 hours 55 minutes',
    150000,
    'PrometheusDown',
    'critical',
    'fp-009',
    NOW() - INTERVAL '12 hours 5 minutes',
    'gpt-4',
    0.93,
    'restart_pod',
    '{"description": "Prometheus server unreachable"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Namespace: logging (logstash pod)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-010'),
    'test-rr-010',
    NOW() - INTERVAL '8 hours',
    NOW() - INTERVAL '8 hours',
    NOW() - INTERVAL '7 hours 55 minutes',
    200000,
    'LogstashDown',
    'warning',
    'fp-010',
    NOW() - INTERVAL '8 hours 5 minutes',
    'gpt-3.5-turbo',
    0.86,
    'restart_pod',
    '{"description": "Logstash pipeline stopped processing"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 6: Recent incidents for trend analysis (last 7 days) - all on webapp deployment
-- Day 1
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-001'),
    'test-rr-011',
    NOW() - INTERVAL '1 day',
    NOW() - INTERVAL '1 day',
    NOW() - INTERVAL '1 day' + INTERVAL '5 minutes',
    300000,
    'HighCPUUsage',
    'warning',
    'fp-011',
    NOW() - INTERVAL '1 day' - INTERVAL '5 minutes',
    'gpt-3.5-turbo',
    0.84,
    'scale_deployment',
    '{"description": "CPU usage exceeded 80% for 5 minutes"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Day 2
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-001'),
    'test-rr-012',
    NOW() - INTERVAL '2 days',
    NOW() - INTERVAL '2 days',
    NOW() - INTERVAL '2 days' + INTERVAL '5 minutes',
    310000,
    'HighCPUUsage',
    'warning',
    'fp-012',
    NOW() - INTERVAL '2 days' - INTERVAL '5 minutes',
    'gpt-3.5-turbo',
    0.86,
    'scale_deployment',
    '{"description": "CPU usage exceeded 80% for 5 minutes"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),
-- Day 3
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-001'),
    'test-rr-013',
    NOW() - INTERVAL '3 days',
    NOW() - INTERVAL '3 days',
    NOW() - INTERVAL '3 days' + INTERVAL '5 minutes',
    320000,
    'HighMemoryUsage',
    'critical',
    'fp-013',
    NOW() - INTERVAL '3 days' - INTERVAL '5 minutes',
    'gpt-4',
    0.94,
    'scale_deployment',
    '{"description": "Memory usage exceeded 90% threshold"}'::jsonb,
    'completed',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 7: In-progress incidents for status filtering (worker-03 node)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-011'),
    'test-rr-014',
    NOW() - INTERVAL '5 minutes',
    NOW() - INTERVAL '5 minutes',
    NULL,
    NULL,
    'NodeNotReady',
    'critical',
    'fp-014',
    NOW() - INTERVAL '10 minutes',
    'gpt-4',
    0.91,
    'cordon_drain_node',
    '{"description": "Node marked NotReady due to network issue"}'::jsonb,
    'executing',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
),

-- Scenario 8: Pending incidents for phase filtering (data-volume PVC)
(
    (SELECT ah.id FROM action_histories ah JOIN resource_references rr ON ah.resource_id = rr.id WHERE rr.resource_uid = 'test-uid-012'),
    'test-rr-015',
    NOW() - INTERVAL '2 minutes',
    NULL,
    NULL,
    NULL,
    'PersistentVolumeClaimPending',
    'warning',
    'fp-015',
    NOW() - INTERVAL '5 minutes',
    'gpt-3.5-turbo',
    0.82,
    'create_persistent_volume',
    '{"description": "PersistentVolumeClaim in pending state"}'::jsonb,
    'pending',
    'prod-us-east-1',
    'production',
    (SELECT array_agg(random())::vector FROM generate_series(1, 384)),
    NOW(),
    NOW()
);

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- VALIDATION QUERIES
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

-- Verify data insertion
DO $$
DECLARE
    resource_count INTEGER;
    history_count INTEGER;
    trace_count INTEGER;
    embedding_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO resource_count FROM resource_references WHERE resource_uid LIKE 'test-uid-%';
    SELECT COUNT(*) INTO history_count FROM action_histories ah
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.resource_uid LIKE 'test-uid-%';
    SELECT COUNT(*) INTO trace_count FROM resource_action_traces WHERE action_id LIKE 'test-rr-%';
    SELECT COUNT(*) INTO embedding_count FROM resource_action_traces
        WHERE action_id LIKE 'test-rr-%' AND embedding IS NOT NULL;

    RAISE NOTICE '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━';
    RAISE NOTICE 'Test Data Insertion Summary';
    RAISE NOTICE '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━';
    RAISE NOTICE 'Resource References: %', resource_count;
    RAISE NOTICE 'Action Histories: %', history_count;
    RAISE NOTICE 'Resource Action Traces: %', trace_count;
    RAISE NOTICE 'Traces with Embeddings: %', embedding_count;

    IF resource_count < 12 THEN
        RAISE EXCEPTION 'Resource references insertion failed: expected 12, got %', resource_count;
    END IF;

    IF trace_count < 15 THEN
        RAISE EXCEPTION 'Action traces insertion failed: expected 15, got %', trace_count;
    END IF;

    IF embedding_count < 15 THEN
        RAISE EXCEPTION 'Embedding generation failed: expected 15, got %', embedding_count;
    END IF;
END $$;

-- Display test data summary (using Data Storage schema)
SELECT
    rr.namespace,
    COUNT(*) as incident_count,
    COUNT(CASE WHEN rat.execution_status = 'completed' THEN 1 END) as completed,
    COUNT(CASE WHEN rat.execution_status = 'failed' THEN 1 END) as failed,
    COUNT(CASE WHEN rat.execution_status = 'executing' THEN 1 END) as in_progress,
    COUNT(CASE WHEN rat.execution_status = 'pending' THEN 1 END) as pending
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id
WHERE rat.action_id LIKE 'test-rr-%'
GROUP BY rr.namespace
ORDER BY incident_count DESC;

-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- TEST DATA INSERTION COMPLETE
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
-- Infrastructure: Reuses Data Storage Service PostgreSQL
-- Schema: Data Storage Service (resource_action_traces + action_histories + resource_references)
-- Test Data: 12 resources, 12 action histories, 15 action traces with embeddings (vector(384))
-- Usage: Execute via suite_test.go BeforeEach to populate test data
-- Isolation: Uses test-uid-* and test-rr-* prefixes for cleanup
-- Ready for Context API integration testing with Data Storage schema!
-- ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
