-- Migration: 011_rename_alert_to_signal.sql
-- Purpose: Rename all "alert" terminology to "signal" for domain model consistency
-- Rationale: The project uses "signal" as the abstract term for events (alerts, logs, traces, metrics)
-- Related: BR-STORAGE-001, BR-STORAGE-030 (architectural consistency)

-- ============================================================================
-- STEP 1: Drop dependent views
-- ============================================================================

DROP VIEW IF EXISTS pattern_analytics_summary;
DROP VIEW IF EXISTS incident_summary_view;

-- ============================================================================
-- STEP 2: Drop indexes that will be renamed
-- ============================================================================

DROP INDEX IF EXISTS idx_rat_alert_name;
DROP INDEX IF EXISTS idx_rat_alert_labels_gin;
DROP INDEX IF EXISTS idx_rat_alert_fingerprint;
DROP INDEX IF EXISTS action_patterns_alert_name_idx;
DROP INDEX IF EXISTS action_patterns_alert_severity_idx;

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

ALTER TABLE resource_action_traces
    RENAME COLUMN alert_fingerprint TO signal_fingerprint;

-- Update column comments
COMMENT ON COLUMN resource_action_traces.signal_fingerprint IS
'SHA-256 fingerprint of signal for deduplication and correlation (Context API compatibility)';

-- ============================================================================
-- STEP 4: Rename columns in action_patterns
-- ============================================================================

ALTER TABLE action_patterns
    RENAME COLUMN alert_name TO signal_name;

ALTER TABLE action_patterns
    RENAME COLUMN alert_severity TO signal_severity;

-- ============================================================================
-- STEP 5: Rename columns in action_assessments (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'action_assessments') THEN
        ALTER TABLE action_assessments RENAME COLUMN alert_name TO signal_name;
    END IF;
END $$;

-- ============================================================================
-- STEP 6: Rename columns in effectiveness_results (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'effectiveness_results') THEN
        ALTER TABLE effectiveness_results RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 7: Rename columns in action_outcomes (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'action_outcomes') THEN
        ALTER TABLE action_outcomes RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 8: Rename columns in cascade_analysis (if exists)
-- ============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'cascade_analysis') THEN
        ALTER TABLE cascade_analysis RENAME COLUMN alert_resolved TO signal_resolved;
    END IF;
END $$;

-- ============================================================================
-- STEP 9: Recreate indexes with new names
-- ============================================================================

CREATE INDEX idx_rat_signal_name ON resource_action_traces (signal_name, action_timestamp);
CREATE INDEX idx_rat_signal_labels_gin ON resource_action_traces USING GIN (signal_labels);
CREATE INDEX idx_rat_signal_fingerprint ON resource_action_traces (signal_fingerprint);

CREATE INDEX action_patterns_signal_name_idx ON action_patterns(signal_name);
CREATE INDEX action_patterns_signal_severity_idx ON action_patterns(signal_severity);

-- ============================================================================
-- STEP 10: Recreate pattern_analytics_summary view
-- ============================================================================

CREATE VIEW pattern_analytics_summary AS
SELECT
    COUNT(*) as total_patterns,
    COUNT(DISTINCT action_type) as unique_action_types,
    COUNT(DISTINCT signal_name) as unique_signal_names,
    COUNT(DISTINCT signal_severity) as unique_severities,
    COUNT(DISTINCT namespace) as unique_namespaces,
    COUNT(DISTINCT resource_type) as unique_resource_types,
    COUNT(DISTINCT context) as unique_contexts,
    AVG((effectiveness_data->>'score')::float) as avg_effectiveness_score,
    COUNT(*) FILTER (WHERE effectiveness_data->>'score' IS NOT NULL) as patterns_with_effectiveness,
    COUNT(*) FILTER (WHERE context IS NOT NULL) as patterns_with_context,
    MIN(created_at) as oldest_pattern,
    MAX(created_at) as newest_pattern
FROM action_patterns;

-- ============================================================================
-- STEP 11: Recreate incident_summary_view
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
-- STEP 12: Update stored procedures/functions
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

-- ============================================================================
-- STEP 13: Update trigger function for action pattern generation
-- ============================================================================

CREATE OR REPLACE FUNCTION update_action_patterns()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO action_patterns (
        pattern_hash,
        pattern_name,
        action_type,
        resource_type,
        signal_name,
        signal_severity,
        namespace,
        cluster_name,
        context,
        success_count,
        failure_count,
        avg_execution_time,
        last_seen,
        effectiveness_data
    ) VALUES (
        encode(sha256(CONCAT(NEW.action_type, ':', COALESCE(NEW.signal_name, 'no-signal'))::bytea), 'hex'),
        COALESCE(NEW.signal_name, 'no-signal'),
        NEW.action_type,
        COALESCE(NEW.resource_type, 'unknown'),
        COALESCE(NEW.signal_name, 'no-signal'),
        NEW.signal_severity,
        NEW.namespace,
        NEW.cluster_name,
        'default',
        CASE WHEN NEW.execution_status = 'completed' THEN 1 ELSE 0 END,
        CASE WHEN NEW.execution_status = 'failed' THEN 1 ELSE 0 END,
        EXTRACT(EPOCH FROM (NEW.action_end_time - NEW.action_timestamp)),
        NEW.action_timestamp,
        '{}'::jsonb
    )
    ON CONFLICT (pattern_hash) DO UPDATE SET
        success_count = action_patterns.success_count + CASE WHEN NEW.execution_status = 'completed' THEN 1 ELSE 0 END,
        failure_count = action_patterns.failure_count + CASE WHEN NEW.execution_status = 'failed' THEN 1 ELSE 0 END,
        avg_execution_time = (
            action_patterns.avg_execution_time * (action_patterns.success_count + action_patterns.failure_count) +
            EXTRACT(EPOCH FROM (NEW.action_end_time - NEW.action_timestamp))
        ) / (action_patterns.success_count + action_patterns.failure_count + 1),
        last_seen = NEW.action_timestamp;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

