-- +goose Up
-- +goose StatementBegin
-- ========================================
-- ADR-033: Multi-Dimensional Success Tracking
-- Migration: Add columns for incident-type, playbook, and AI execution mode tracking
-- Date: November 4, 2025
-- ========================================

-- ========================================
-- DIMENSION 1: INCIDENT TYPE (PRIMARY)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    incident_type VARCHAR(100);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    alert_name VARCHAR(255);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    incident_severity VARCHAR(20);

COMMENT ON COLUMN resource_action_traces.incident_type IS 
'ADR-033: PRIMARY dimension for success tracking. Examples: pod-oom-killer, high-cpu, disk-pressure';

COMMENT ON COLUMN resource_action_traces.alert_name IS 
'ADR-033: Prometheus alert name. Can be used as proxy for incident_type';

COMMENT ON COLUMN resource_action_traces.incident_severity IS 
'ADR-033: Incident severity level. Values: critical, warning, info';

-- ========================================
-- DIMENSION 2: PLAYBOOK (SECONDARY)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_id VARCHAR(64);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_version VARCHAR(20);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_step_number INT;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_execution_id VARCHAR(64);

COMMENT ON COLUMN resource_action_traces.playbook_id IS 
'ADR-033: SECONDARY dimension. Playbook identifier from catalog. Examples: pod-oom-recovery, disk-cleanup';

COMMENT ON COLUMN resource_action_traces.playbook_version IS 
'ADR-033: Semantic version of playbook. Examples: v1.0, v1.2, v2.0';

COMMENT ON COLUMN resource_action_traces.playbook_step_number IS 
'ADR-033: Step position within playbook execution (1, 2, 3, ...). NULL for non-playbook actions';

COMMENT ON COLUMN resource_action_traces.playbook_execution_id IS 
'ADR-033: Groups all actions in a single playbook execution. Same ID across all steps';

-- ========================================
-- AI EXECUTION MODE (HYBRID MODEL)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_selected_playbook BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_chained_playbooks BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_manual_escalation BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_playbook_customization JSONB;

COMMENT ON COLUMN resource_action_traces.ai_selected_playbook IS 
'ADR-033: TRUE if AI selected single playbook from catalog (90-95% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_chained_playbooks IS 
'ADR-033: TRUE if AI chained multiple catalog playbooks (4-9% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_manual_escalation IS 
'ADR-033: TRUE if AI escalated to human operator (<1% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_playbook_customization IS 
'ADR-033: Parameters customized by AI for incident-specific needs. Format: {"param": "value"}';

-- ========================================
-- INDEXES FOR MULTI-DIMENSIONAL QUERIES
-- ========================================

-- Incident-Type Success Rate (PRIMARY dimension)
CREATE INDEX IF NOT EXISTS idx_incident_type_success 
ON resource_action_traces(incident_type, status, action_timestamp DESC)
WHERE incident_type IS NOT NULL;

-- Playbook Success Rate (SECONDARY dimension)
CREATE INDEX IF NOT EXISTS idx_playbook_success 
ON resource_action_traces(playbook_id, playbook_version, status, action_timestamp DESC)
WHERE playbook_id IS NOT NULL;

-- Action-Type Success Rate (TERTIARY dimension - already have index on action_type)
-- No new index needed, existing indexes suffice

-- Multi-dimensional composite index (incident_type + playbook_id + action_type)
CREATE INDEX IF NOT EXISTS idx_multidimensional_success 
ON resource_action_traces(incident_type, playbook_id, action_type, status, action_timestamp DESC)
WHERE incident_type IS NOT NULL AND playbook_id IS NOT NULL;

-- Playbook execution grouping (for chained playbook tracking)
CREATE INDEX IF NOT EXISTS idx_playbook_execution 
ON resource_action_traces(playbook_execution_id, playbook_step_number, action_timestamp DESC)
WHERE playbook_execution_id IS NOT NULL;

-- AI execution mode filtering
CREATE INDEX IF NOT EXISTS idx_ai_execution_mode 
ON resource_action_traces(incident_type, ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation, action_timestamp DESC)
WHERE incident_type IS NOT NULL;

-- Alert name lookup (for incident-type proxy)
CREATE INDEX IF NOT EXISTS idx_alert_name_lookup 
ON resource_action_traces(alert_name, status, action_timestamp DESC)
WHERE alert_name IS NOT NULL;

-- ========================================
-- BACKWARD COMPATIBILITY VALIDATION
-- ========================================

-- All new columns are nullable - existing rows remain valid
-- Existing queries continue to work without modification
-- New queries can filter WHERE incident_type IS NOT NULL

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- ========================================
-- ROLLBACK: Remove ADR-033 columns and indexes
-- ========================================

-- Drop indexes
DROP INDEX IF EXISTS idx_alert_name_lookup;
DROP INDEX IF EXISTS idx_ai_execution_mode;
DROP INDEX IF EXISTS idx_playbook_execution;
DROP INDEX IF EXISTS idx_multidimensional_success;
DROP INDEX IF EXISTS idx_playbook_success;
DROP INDEX IF EXISTS idx_incident_type_success;

-- Drop AI execution mode columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_playbook_customization;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_manual_escalation;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_chained_playbooks;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_selected_playbook;

-- Drop playbook columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_execution_id;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_step_number;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_version;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_id;

-- Drop incident-type columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS incident_severity;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS alert_name;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS incident_type;

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- ADR-033: Multi-Dimensional Success Tracking
-- Migration: Add columns for incident-type, playbook, and AI execution mode tracking
-- Date: November 4, 2025
-- ========================================

-- ========================================
-- DIMENSION 1: INCIDENT TYPE (PRIMARY)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    incident_type VARCHAR(100);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    alert_name VARCHAR(255);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    incident_severity VARCHAR(20);

COMMENT ON COLUMN resource_action_traces.incident_type IS 
'ADR-033: PRIMARY dimension for success tracking. Examples: pod-oom-killer, high-cpu, disk-pressure';

COMMENT ON COLUMN resource_action_traces.alert_name IS 
'ADR-033: Prometheus alert name. Can be used as proxy for incident_type';

COMMENT ON COLUMN resource_action_traces.incident_severity IS 
'ADR-033: Incident severity level. Values: critical, warning, info';

-- ========================================
-- DIMENSION 2: PLAYBOOK (SECONDARY)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_id VARCHAR(64);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_version VARCHAR(20);

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_step_number INT;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    playbook_execution_id VARCHAR(64);

COMMENT ON COLUMN resource_action_traces.playbook_id IS 
'ADR-033: SECONDARY dimension. Playbook identifier from catalog. Examples: pod-oom-recovery, disk-cleanup';

COMMENT ON COLUMN resource_action_traces.playbook_version IS 
'ADR-033: Semantic version of playbook. Examples: v1.0, v1.2, v2.0';

COMMENT ON COLUMN resource_action_traces.playbook_step_number IS 
'ADR-033: Step position within playbook execution (1, 2, 3, ...). NULL for non-playbook actions';

COMMENT ON COLUMN resource_action_traces.playbook_execution_id IS 
'ADR-033: Groups all actions in a single playbook execution. Same ID across all steps';

-- ========================================
-- AI EXECUTION MODE (HYBRID MODEL)
-- ========================================
ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_selected_playbook BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_chained_playbooks BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_manual_escalation BOOLEAN DEFAULT false;

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS 
    ai_playbook_customization JSONB;

COMMENT ON COLUMN resource_action_traces.ai_selected_playbook IS 
'ADR-033: TRUE if AI selected single playbook from catalog (90-95% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_chained_playbooks IS 
'ADR-033: TRUE if AI chained multiple catalog playbooks (4-9% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_manual_escalation IS 
'ADR-033: TRUE if AI escalated to human operator (<1% of cases)';

COMMENT ON COLUMN resource_action_traces.ai_playbook_customization IS 
'ADR-033: Parameters customized by AI for incident-specific needs. Format: {"param": "value"}';

-- ========================================
-- INDEXES FOR MULTI-DIMENSIONAL QUERIES
-- ========================================

-- Incident-Type Success Rate (PRIMARY dimension)
CREATE INDEX IF NOT EXISTS idx_incident_type_success 
ON resource_action_traces(incident_type, status, action_timestamp DESC)
WHERE incident_type IS NOT NULL;

-- Playbook Success Rate (SECONDARY dimension)
CREATE INDEX IF NOT EXISTS idx_playbook_success 
ON resource_action_traces(playbook_id, playbook_version, status, action_timestamp DESC)
WHERE playbook_id IS NOT NULL;

-- Action-Type Success Rate (TERTIARY dimension - already have index on action_type)
-- No new index needed, existing indexes suffice

-- Multi-dimensional composite index (incident_type + playbook_id + action_type)
CREATE INDEX IF NOT EXISTS idx_multidimensional_success 
ON resource_action_traces(incident_type, playbook_id, action_type, status, action_timestamp DESC)
WHERE incident_type IS NOT NULL AND playbook_id IS NOT NULL;

-- Playbook execution grouping (for chained playbook tracking)
CREATE INDEX IF NOT EXISTS idx_playbook_execution 
ON resource_action_traces(playbook_execution_id, playbook_step_number, action_timestamp DESC)
WHERE playbook_execution_id IS NOT NULL;

-- AI execution mode filtering
CREATE INDEX IF NOT EXISTS idx_ai_execution_mode 
ON resource_action_traces(incident_type, ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation, action_timestamp DESC)
WHERE incident_type IS NOT NULL;

-- Alert name lookup (for incident-type proxy)
CREATE INDEX IF NOT EXISTS idx_alert_name_lookup 
ON resource_action_traces(alert_name, status, action_timestamp DESC)
WHERE alert_name IS NOT NULL;

-- ========================================
-- BACKWARD COMPATIBILITY VALIDATION
-- ========================================

-- All new columns are nullable - existing rows remain valid
-- Existing queries continue to work without modification
-- New queries can filter WHERE incident_type IS NOT NULL

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- ========================================
-- ROLLBACK: Remove ADR-033 columns and indexes
-- ========================================

-- Drop indexes
DROP INDEX IF EXISTS idx_alert_name_lookup;
DROP INDEX IF EXISTS idx_ai_execution_mode;
DROP INDEX IF EXISTS idx_playbook_execution;
DROP INDEX IF EXISTS idx_multidimensional_success;
DROP INDEX IF EXISTS idx_playbook_success;
DROP INDEX IF EXISTS idx_incident_type_success;

-- Drop AI execution mode columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_playbook_customization;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_manual_escalation;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_chained_playbooks;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS ai_selected_playbook;

-- Drop playbook columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_execution_id;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_step_number;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_version;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS playbook_id;

-- Drop incident-type columns
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS incident_severity;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS alert_name;
ALTER TABLE resource_action_traces DROP COLUMN IF EXISTS incident_type;

-- +goose StatementEnd

