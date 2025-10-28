-- Seed Test Data for Context API Integration Testing
-- Creates realistic remediation scenarios across multiple namespaces and clusters

BEGIN;

-- Insert resource references (Kubernetes resources)
-- Let IDs auto-generate via sequence
WITH inserted_resources AS (
    INSERT INTO resource_references (namespace, kind, name, resource_uid, api_version, created_at, last_seen) VALUES
    ('production', 'Deployment', 'frontend-web', 'seed-uid-frontend-001', 'apps/v1', NOW() - INTERVAL '30 days', NOW()),
    ('production', 'StatefulSet', 'database-primary', 'seed-uid-db-001', 'apps/v1', NOW() - INTERVAL '30 days', NOW()),
    ('staging', 'Deployment', 'api-server', 'seed-uid-api-001', 'apps/v1', NOW() - INTERVAL '20 days', NOW()),
    ('staging', 'Deployment', 'cache-redis', 'seed-uid-redis-001', 'apps/v1', NOW() - INTERVAL '20 days', NOW()),
    ('production', 'Deployment', 'backend-workers', 'seed-uid-workers-001', 'apps/v1', NOW() - INTERVAL '25 days', NOW()),
    ('development', 'Deployment', 'test-service', 'seed-uid-test-001', 'apps/v1', NOW() - INTERVAL '15 days', NOW()),
    ('production', 'Pod', 'nginx-ingress-controller', 'seed-uid-nginx-001', 'v1', NOW() - INTERVAL '30 days', NOW()),
    ('kube-system', 'DaemonSet', 'node-exporter', 'seed-uid-exporter-001', 'apps/v1', NOW() - INTERVAL '30 days', NOW())
    ON CONFLICT (resource_uid) DO NOTHING
    RETURNING id, resource_uid, namespace
)
SELECT * FROM inserted_resources;

-- Insert action histories using the generated resource IDs
WITH inserted_actions AS (
    INSERT INTO action_histories (resource_id, total_actions, last_action_at, next_analysis_at, last_analysis_at, created_at, updated_at)
    SELECT
        rr.id,
        5, -- 5 actions per resource will be created below
        NOW() - INTERVAL '1 day',
        NOW() + INTERVAL '6 hours',
        NOW() - INTERVAL '2 days',
        NOW() - INTERVAL '30 days',
        NOW()
    FROM resource_references rr
    WHERE rr.resource_uid IN (
        'seed-uid-frontend-001', 'seed-uid-db-001', 'seed-uid-api-001', 'seed-uid-redis-001',
        'seed-uid-workers-001', 'seed-uid-test-001', 'seed-uid-nginx-001', 'seed-uid-exporter-001'
    )
    ON CONFLICT (resource_id) DO NOTHING
    RETURNING id, resource_id
)
SELECT * FROM inserted_actions;

-- Insert resource action traces using action_history_id references
INSERT INTO resource_action_traces (
    action_id, action_history_id, action_timestamp,
    alert_name, alert_fingerprint, alert_severity, alert_labels,
    cluster_name, environment,
    action_type, action_parameters, execution_status,
    execution_start_time, execution_end_time, execution_duration_ms,
    model_used, model_confidence, created_at, updated_at
)
SELECT
    'seed-action-' || ah.id || '-' || seq.n,
    ah.id,
    NOW() - (seq.n || ' days')::INTERVAL,
    CASE (seq.n % 5)
        WHEN 0 THEN 'HighMemoryUsage'
        WHEN 1 THEN 'PodCrashLooping'
        WHEN 2 THEN 'HighCPUUsage'
        WHEN 3 THEN 'DatabaseSlowQueries'
        WHEN 4 THEN 'ServiceUnavailable'
    END,
    'seed-fp-' || ah.id || '-' || seq.n,
    CASE (seq.n % 3)
        WHEN 0 THEN 'critical'
        WHEN 1 THEN 'high'
        WHEN 2 THEN 'low'
    END,
    '{"service":"test","team":"platform"}',
    CASE
        WHEN rr.namespace IN ('production', 'kube-system') THEN 'prod-us-west-2'
        WHEN rr.namespace = 'staging' THEN 'staging-us-east-1'
        ELSE 'dev-local'
    END,
    CASE
        WHEN rr.namespace IN ('production', 'kube-system') THEN 'production'
        WHEN rr.namespace = 'staging' THEN 'staging'
        ELSE 'development'
    END,
    CASE (seq.n % 4)
        WHEN 0 THEN 'scale'
        WHEN 1 THEN 'restart'
        WHEN 2 THEN 'analyze'
        WHEN 3 THEN 'rollback'
    END,
    '{"replicas":3}',
    CASE
        WHEN seq.n % 5 = 0 THEN 'failed'
        ELSE 'completed'
    END,
    NOW() - (seq.n || ' days')::INTERVAL,
    NOW() - (seq.n || ' days')::INTERVAL + (30 || ' seconds')::INTERVAL,
    30000,
    CASE
        WHEN rr.namespace = 'production' THEN 'gpt-4'
        ELSE 'gpt-3.5-turbo'
    END,
    CASE
        WHEN seq.n % 5 = 0 THEN 0.650  -- Lower confidence for failed actions
        ELSE 0.850  -- High confidence for successful actions
    END,
    NOW() - (seq.n || ' days')::INTERVAL,
    NOW() - (seq.n || ' days')::INTERVAL
FROM action_histories ah
JOIN resource_references rr ON ah.resource_id = rr.id
CROSS JOIN generate_series(1, 5) AS seq(n)
WHERE rr.resource_uid LIKE 'seed-uid-%'
ON CONFLICT (action_id, action_timestamp) DO NOTHING;

-- Verify data insertion
SELECT
    'resource_references' as table_name,
    COUNT(*) as row_count
FROM resource_references
WHERE resource_uid LIKE 'seed-uid-%'
UNION ALL
SELECT
    'action_histories',
    COUNT(*)
FROM action_histories ah
JOIN resource_references rr ON ah.resource_id = rr.id
WHERE rr.resource_uid LIKE 'seed-uid-%'
UNION ALL
SELECT
    'resource_action_traces',
    COUNT(*)
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id
WHERE rr.resource_uid LIKE 'seed-uid-%';

COMMIT;

