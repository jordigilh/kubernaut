-- Context API Compatibility Fields
-- Migration: 008_context_api_compatibility.sql
-- 
-- Adds fields required by Context API for complete incident querying:
-- - alert_fingerprint: Unique identifier for alert correlation
-- - cluster_name: Kubernetes cluster identifier for multi-cluster filtering
-- - environment: Deployment environment (production, staging, development)
--
-- These fields enable Context API to provide LLM with rich filtering capabilities
-- without requiring complex JSONB extraction from alert_labels.

-- Add alert_fingerprint to resource_action_traces
-- This uniquely identifies an alert instance for correlation
ALTER TABLE resource_action_traces
ADD COLUMN alert_fingerprint VARCHAR(64);

-- Add cluster_name to resource_action_traces
-- Supports multi-cluster Kubernaut deployments
ALTER TABLE resource_action_traces
ADD COLUMN cluster_name VARCHAR(100);

-- Add environment to resource_action_traces
-- Enables filtering by deployment environment
ALTER TABLE resource_action_traces
ADD COLUMN environment VARCHAR(20);

-- Create index on alert_fingerprint for fast lookups
CREATE INDEX idx_rat_alert_fingerprint ON resource_action_traces (alert_fingerprint);

-- Create index on cluster_name for multi-cluster filtering
CREATE INDEX idx_rat_cluster_name ON resource_action_traces (cluster_name);

-- Create index on environment for environment-specific queries
CREATE INDEX idx_rat_environment ON resource_action_traces (environment);

-- Composite index for common Context API query patterns
-- (cluster + environment + severity filtering)
CREATE INDEX idx_rat_context_filters ON resource_action_traces (
    cluster_name, 
    environment, 
    alert_severity
) WHERE cluster_name IS NOT NULL;

-- Add check constraint for environment values
ALTER TABLE resource_action_traces
ADD CONSTRAINT check_environment_valid 
CHECK (environment IS NULL OR environment IN ('production', 'staging', 'development', 'test'));

-- Comment on new columns
COMMENT ON COLUMN resource_action_traces.alert_fingerprint IS 
'Unique fingerprint for alert correlation across multiple remediation attempts';

COMMENT ON COLUMN resource_action_traces.cluster_name IS 
'Kubernetes cluster name for multi-cluster deployments (e.g., prod-us-west, staging-eu-central)';

COMMENT ON COLUMN resource_action_traces.environment IS 
'Deployment environment: production, staging, development, or test';

