-- HAPI Integration Test Fixtures: Workflow Catalog Test Data
-- Owner: HAPI Team
-- Purpose: Provide workflow data for HAPI integration tests
-- Schema: DD-WORKFLOW-002 v3.0
--
-- Usage:
--   podman exec -i <postgres_container> psql -U slm_user -d action_history < tests/integration/fixtures/workflow_catalog_test_data.sql

BEGIN;

-- Insert test workflows for HAPI integration tests
INSERT INTO remediation_workflow_catalog (
    workflow_id,
    workflow_name,
    version,
    name,
    description,
    owner,
    maintainer,
    content,
    content_hash,
    container_image,
    status,
    labels,
    custom_labels,
    detected_labels,
    is_latest_version,
    execution_engine,
    created_at,
    updated_at
) VALUES
-- OOMKilled workflow (used by mock LLM tests)
(
    'a1b2c3d4-e5f6-4a5b-8c9d-0e1f2a3b4c5d'::uuid,
    'oomkill-increase-memory',
    'v1.0.0',
    'OOMKill Memory Increase',
    'Increases memory limits for pods experiencing OOMKilled events',
    'platform-team',
    'platform@example.com',
    '{"steps": [{"name": "analyze", "action": "get_memory_metrics"}, {"name": "remediate", "action": "patch_memory_limit"}]}',
    'sha256:oom1',
    'kubernaut/workflow-oomkill:v1.0.0',
    'active',
    '{"signal_type": ["OOMKilled"], "severity": ["critical"]}',
    '{"signal_type": ["OOMKilled"]}',
    '{"namespace": ["*"], "pod": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
),
-- CrashLoopBackOff workflow
(
    'b2c3d4e5-f6a7-5b6c-9d0e-1f2a3b4c5d6e'::uuid,
    'crashloop-config-fix',
    'v1.0.0',
    'CrashLoop Config Fix',
    'Analyzes and fixes configuration issues causing CrashLoopBackOff',
    'platform-team',
    'platform@example.com',
    '{"steps": [{"name": "analyze", "action": "get_pod_logs"}, {"name": "remediate", "action": "fix_config"}]}',
    'sha256:crash1',
    'kubernaut/workflow-crashloop:v1.0.0',
    'active',
    '{"signal_type": ["CrashLoopBackOff"], "severity": ["high"]}',
    '{"signal_type": ["CrashLoopBackOff"]}',
    '{"namespace": ["*"], "pod": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
),
-- ImagePullBackOff workflow
(
    'c3d4e5f6-a7b8-6c7d-0e1f-2a3b4c5d6e7f'::uuid,
    'image-fix',
    'v1.0.0',
    'Image Pull Fix',
    'Resolves ImagePullBackOff by checking registry credentials',
    'platform-team',
    'platform@example.com',
    '{"steps": [{"name": "analyze", "action": "check_image"}, {"name": "remediate", "action": "update_secret"}]}',
    'sha256:image1',
    'kubernaut/workflow-imagepull:v1.0.0',
    'active',
    '{"signal_type": ["ImagePullBackOff"], "severity": ["high"]}',
    '{"signal_type": ["ImagePullBackOff"]}',
    '{"namespace": ["*"], "pod": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
),
-- NodeNotReady workflow
(
    'd4e5f6a7-b8c9-7d8e-1f2a-3b4c5d6e7f8a'::uuid,
    'node-drain-reboot',
    'v1.0.0',
    'Node Drain and Reboot',
    'Handles NodeNotReady by draining and rebooting',
    'infrastructure-team',
    'infra@example.com',
    '{"steps": [{"name": "cordon", "action": "cordon_node"}, {"name": "drain", "action": "drain_node"}]}',
    'sha256:node1',
    'kubernaut/workflow-node:v1.0.0',
    'active',
    '{"signal_type": ["NodeNotReady"], "severity": ["critical"]}',
    '{"signal_type": ["NodeNotReady"]}',
    '{"node": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
),
-- Generic restart workflow
(
    'e5f6a7b8-c9d0-8e9f-2a3b-4c5d6e7f8a9b'::uuid,
    'generic-restart',
    'v1.0.0',
    'Generic Pod Restart',
    'Generic workflow for restarting unhealthy pods',
    'platform-team',
    'platform@example.com',
    '{"steps": [{"name": "restart", "action": "delete_pod"}]}',
    'sha256:generic1',
    'kubernaut/workflow-generic:v1.0.0',
    'active',
    '{"signal_type": ["Unknown"], "severity": ["low"]}',
    '{"signal_type": ["Unknown"]}',
    '{"namespace": ["*"], "pod": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
),
-- Eviction recovery workflow
(
    'f6a7b8c9-d0e1-9f0a-3b4c-5d6e7f8a9b0c'::uuid,
    'eviction-recovery',
    'v1.0.0',
    'Eviction Recovery',
    'Handles pod evictions by rescheduling',
    'platform-team',
    'platform@example.com',
    '{"steps": [{"name": "analyze", "action": "check_pressure"}, {"name": "remediate", "action": "reschedule"}]}',
    'sha256:evict1',
    'kubernaut/workflow-eviction:v1.0.0',
    'active',
    '{"signal_type": ["Evicted"], "severity": ["medium"]}',
    '{"signal_type": ["Evicted"]}',
    '{"namespace": ["*"], "pod": ["*"]}',
    true,
    'tekton',
    NOW(),
    NOW()
);

-- Verify data was inserted
SELECT workflow_id, workflow_name, version, status, container_image
FROM remediation_workflow_catalog ORDER BY workflow_name;

COMMIT;
