-- +goose Up
-- Issue #1048: Remove ADR-033 resource_action_traces table.
-- This table was never exposed via API (V1.0 disabled endpoints) and the
-- aggregation feature has been removed from the codebase.
-- No backward compatibility needed per user decision.

DROP TABLE IF EXISTS resource_action_traces CASCADE;

-- +goose Down
-- Re-create the table schema from 001_v1_schema.sql.
-- This is a best-effort restoration; the original static partitions
-- (2026_03 through 2028_12) are NOT recreated because the application
-- now uses EnsureMonthlyPartitions at startup for dynamic provisioning.

CREATE TABLE IF NOT EXISTS resource_action_traces (
    id BIGSERIAL,
    action_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255) NOT NULL,
    resource_kind VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    cluster_name VARCHAR(255),
    signal_name VARCHAR(255) NOT NULL,
    signal_severity VARCHAR(50),
    action_type VARCHAR(255) NOT NULL,
    action_timestamp TIMESTAMPTZ NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    duration_seconds INTEGER,
    error_message TEXT,
    metadata JSONB DEFAULT '{}',
    workflow_id VARCHAR(255),
    workflow_version VARCHAR(255),
    workflow_step_number INTEGER,
    incident_type VARCHAR(255),
    alert_name VARCHAR(255),
    environment VARCHAR(255),
    ai_selected_workflow BOOLEAN DEFAULT FALSE,
    ai_confidence_score DECIMAL(5,4),
    ai_chained_workflows BOOLEAN DEFAULT FALSE,
    ai_manual_escalation BOOLEAN DEFAULT FALSE,
    CONSTRAINT resource_action_traces_pkey PRIMARY KEY (id, action_timestamp)
) PARTITION BY RANGE (action_timestamp);
