-- Migration: 011_rename_alert_to_signal.sql
-- Purpose: Rename all "alert" terminology to "signal" for domain model consistency
-- Rationale: The project uses "signal" as the abstract term for events (alerts, logs, traces, metrics)
-- Related: BR-STORAGE-001, BR-STORAGE-030 (architectural consistency)
-- NOTE: V1.0 version - only handles existing tables (vector tables removed)

-- ============================================================================
-- STEP 1: Drop dependent views
-- ============================================================================

DROP VIEW IF EXISTS incident_summary_view;

-- ============================================================================
-- STEP 2: Drop indexes that will be renamed
-- ============================================================================

DROP INDEX IF EXISTS idx_rat_alert_name;
DROP INDEX IF EXISTS idx_rat_alert_labels_gin;

-- ============================================================================
-- STEP 3: Rename columns in resource_action_traces
-- ============================================================================

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_name TO signal_name;

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_severity TO signal_severity;

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_labels TO signal_labels;

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_annotations TO signal_annotations;

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_firing_time TO signal_firing_time;

-- ============================================================================
-- STEP 4: Rename columns in action_assessments (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'action_assessments') THEN
        ALTER TABLE action_assessments RENAME COLUMN alert_name TO signal_name;
    END IF;
END $$;

-- ============================================================================
-- STEP 5: Rename columns in effectiveness_results (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'effectiveness_results') THEN
        ALTER TABLE effectiveness_results RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 6: Rename columns in action_outcomes (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'action_outcomes') THEN
        ALTER TABLE action_outcomes RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 7: Rename columns in cascade_analysis (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'cascade_analysis') THEN
        ALTER TABLE cascade_analysis RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 8: Recreate indexes with new names
-- ============================================================================

CREATE INDEX idx_rat_signal_name ON resource_action_traces (signal_name, action_timestamp);
CREATE INDEX idx_rat_signal_labels_gin ON resource_action_traces USING GIN (signal_labels);

-- ============================================================================
-- STEP 9: Recreate incident_summary_view
-- ============================================================================

CREATE OR REPLACE VIEW incident_summary_view AS
SELECT
    signal_severity as severity,
    COUNT(*) as incident_count
FROM resource_action_traces
GROUP BY signal_severity
ORDER BY
    CASE signal_severity
        WHEN 'critical' THEN 1
        WHEN 'high' THEN 2
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
        ELSE 5
    END;

-- ============================================================================
-- STEP 10: Update stored procedures/functions
-- ============================================================================

-- Drop and recreate analyze_cascade_effects function
DROP FUNCTION IF EXISTS analyze_cascade_effects(INTEGER, INTERVAL, INTEGER);

CREATE OR REPLACE FUNCTION analyze_cascade_effects(
    p_days_back INTEGER DEFAULT 7,
    p_time_window INTERVAL DEFAULT '1 hour'::interval,
    p_max_signals INTEGER DEFAULT NULL
)
RETURNS TABLE (
    action_type VARCHAR,
    avg_new_signals NUMERIC,
    max_signals_triggered INTEGER,
    actions_causing_cascades INTEGER,
    total_actions INTEGER,
    cascade_rate NUMERIC
) AS $$
BEGIN
    RETURN QUERY
    WITH action_outcomes AS (
        SELECT
            rat.action_type,
            rat.action_id,
            rat.action_timestamp,
            rat.signal_name as original_signal,
            rat.execution_status,
            (
                SELECT COUNT(DISTINCT rat2.signal_name)
                FROM resource_action_traces rat2
                WHERE rat2.action_timestamp BETWEEN
                    rat.action_timestamp AND
                    rat.action_timestamp + p_time_window
                AND rat2.signal_name != rat.signal_name
            ) as new_signals_triggered,
            (
                SELECT COUNT(*)
                FROM resource_action_traces rat3
                WHERE rat3.action_timestamp > rat.action_timestamp
                AND rat3.action_timestamp <= rat.action_timestamp + INTERVAL '24 hours'
                AND rat3.signal_name = rat.signal_name
            ) as recurrence_count
        FROM resource_action_traces rat
        WHERE rat.action_timestamp >= NOW() - (p_days_back || ' days')::INTERVAL
        AND rat.execution_status = 'completed'
    )
    SELECT
        ao.action_type::VARCHAR,
        ROUND(AVG(ao.new_signals_triggered::float), 2) as avg_new_signals,
        MAX(ao.new_signals_triggered)::INTEGER as max_signals_triggered,
        SUM(CASE WHEN ao.new_signals_triggered > 0 THEN 1 ELSE 0 END)::INTEGER as actions_causing_cascades,
        COUNT(*)::INTEGER as total_actions,
        ROUND((SUM(CASE WHEN ao.new_signals_triggered > 0 THEN 1 ELSE 0 END)::float / COUNT(*)) * 100, 2) as cascade_rate
    FROM action_outcomes ao
    GROUP BY ao.action_type
    HAVING p_max_signals IS NULL OR MAX(ao.new_signals_triggered) <= p_max_signals
    ORDER BY cascade_rate DESC;
END;
$$ LANGUAGE plpgsql;

-- Drop and recreate get_recent_actions function
DROP FUNCTION IF EXISTS get_recent_actions(INTEGER, VARCHAR, VARCHAR);

CREATE OR REPLACE FUNCTION get_recent_actions(
    p_limit INTEGER DEFAULT 100,
    p_signal_name VARCHAR(200) DEFAULT NULL,
    p_signal_severity VARCHAR(20) DEFAULT NULL
)
RETURNS TABLE (
    action_id VARCHAR,
    action_timestamp TIMESTAMP WITH TIME ZONE,
    signal_name VARCHAR,
    signal_severity VARCHAR,
    execution_status VARCHAR
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        rat.action_id::VARCHAR,
        rat.action_timestamp,
        rat.signal_name::VARCHAR,
        rat.signal_severity::VARCHAR,
        rat.execution_status::VARCHAR
    FROM resource_action_traces rat
    WHERE (p_signal_name IS NULL OR rat.signal_name = p_signal_name)
    AND (p_signal_severity IS NULL OR rat.signal_severity = p_signal_severity)
    ORDER BY rat.action_timestamp DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;
