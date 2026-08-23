-- +goose Up
-- Issue #2256: drop remaining pre-ADR-034 per-action ML-style learning/
-- effectiveness schema. Follow-up to #623 (resource_action_traces, dropped in
-- migration 009; action_effectiveness_metrics, flagged but deliberately
-- deprioritized). Confirmed via repo-wide search of pkg/datastorage and the
-- rest of the Go tree: zero code path reads or writes any of the 8 tables,
-- 3 views, or 13 stored functions dropped below. The live effectiveness
-- scoring flow (GetEffectivenessScore, ADR-EM-001 Principle 5, DD-017 v2.1)
-- computes on demand from audit_events and never touches this legacy
-- cluster.
--
-- Beyond the 10 functions named in the issue, preflight found 3 more
-- (get_action_traces, get_resource_actions_base, get_recent_actions) that
-- were already orphaned by migration 009 dropping resource_action_traces
-- (that migration dropped the table but not its dependent functions) and
-- have the same zero-Go-reference profile. Included here to fully retire
-- the cluster rather than leave a partial cleanup.
--
-- Drop order: views before the tables they select from; then children
-- before parents to avoid needing CASCADE (oscillation_detections before
-- oscillation_patterns/resource_references; action_histories before
-- resource_references). action_assessments, effectiveness_results,
-- action_confidence_scores, action_outcomes, and action_alternatives have
-- no FK relationships to anything else, so order among them is unconstrained.

-- Views (depend on the tables below)
DROP VIEW IF EXISTS low_confidence_actions;
DROP VIEW IF EXISTS effectiveness_trends;
DROP VIEW IF EXISTS oscillation_detection_summary;

-- Standalone effectiveness/learning tables (no FKs in or out)
DROP TABLE IF EXISTS action_assessments;
DROP TABLE IF EXISTS effectiveness_results;
DROP TABLE IF EXISTS action_confidence_scores;
DROP TABLE IF EXISTS action_outcomes;
DROP TABLE IF EXISTS action_alternatives;
DROP TABLE IF EXISTS action_effectiveness_metrics;

-- Oscillation tables (child before parent)
DROP TABLE IF EXISTS oscillation_detections;
DROP TABLE IF EXISTS oscillation_patterns;

-- Resource identity tables (child before parent)
DROP TABLE IF EXISTS action_histories;
DROP TABLE IF EXISTS resource_references;

-- Stored functions orphaned by the tables above (and, for the
-- resource_action_traces-only group, already orphaned since migration 009).
DROP FUNCTION IF EXISTS create_assessment_for_action_trace();
DROP FUNCTION IF EXISTS analyze_action_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_cascading_failures(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_ineffective_loops(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_resource_thrashing(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_scale_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS analyze_cascade_effects(INTEGER, INTERVAL, INTEGER);
DROP FUNCTION IF EXISTS store_oscillation_detection(INTEGER, VARCHAR, VARCHAR, VARCHAR, DECIMAL, INTEGER, INTEGER, JSONB, VARCHAR);
DROP FUNCTION IF EXISTS get_action_effectiveness(VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE);
DROP FUNCTION IF EXISTS get_resource_id(VARCHAR, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS get_action_traces(VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE, INTEGER, INTEGER);
DROP FUNCTION IF EXISTS get_resource_actions_base(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS get_recent_actions(INTEGER, VARCHAR, VARCHAR);

-- +goose Down
-- Best-effort schema restoration reflecting migration 001's original
-- definitions. Data is NOT restored -- restore from a pre-migration
-- Postgres backup if needed.

CREATE TABLE IF NOT EXISTS resource_references (
    id BIGSERIAL PRIMARY KEY,
    resource_uid TEXT NOT NULL UNIQUE,
    api_version VARCHAR(50) NOT NULL,
    kind VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(namespace, kind, name)
);

CREATE INDEX idx_resource_kind ON resource_references (kind);
CREATE INDEX idx_resource_namespace ON resource_references (namespace);
CREATE INDEX idx_resource_last_seen ON resource_references (last_seen);
CREATE INDEX idx_resource_uid ON resource_references (resource_uid);

CREATE TABLE IF NOT EXISTS action_histories (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,
    total_actions INTEGER NOT NULL DEFAULT 0,
    last_action_at TIMESTAMP WITH TIME ZONE,
    next_analysis_at TIMESTAMP WITH TIME ZONE,
    last_analysis_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(resource_id)
);

CREATE INDEX idx_ah_last_action ON action_histories (last_action_at);
CREATE INDEX idx_ah_next_analysis ON action_histories (next_analysis_at);
CREATE INDEX idx_ah_resource_id ON action_histories (resource_id);

CREATE TRIGGER update_action_histories_updated_at BEFORE UPDATE ON action_histories FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS oscillation_patterns (
    id BIGSERIAL PRIMARY KEY,
    pattern_type VARCHAR(50) NOT NULL,
    pattern_name VARCHAR(255) NOT NULL,
    description TEXT,
    min_occurrences INTEGER NOT NULL DEFAULT 3,
    time_window_minutes INTEGER NOT NULL DEFAULT 60,
    threshold_config JSONB DEFAULT '{}',
    prevention_strategy VARCHAR(100),
    prevention_parameters JSONB DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT true,
    last_detection_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_op_pattern_type ON oscillation_patterns (pattern_type);
CREATE INDEX idx_op_active_patterns ON oscillation_patterns (active);
CREATE INDEX idx_op_last_detection ON oscillation_patterns (last_detection_at);

CREATE TRIGGER update_oscillation_patterns_updated_at BEFORE UPDATE ON oscillation_patterns FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS oscillation_detections (
    id BIGSERIAL PRIMARY KEY,
    pattern_id BIGINT NOT NULL REFERENCES oscillation_patterns(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,
    detected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    confidence DECIMAL(4,3),
    action_count INTEGER,
    time_span_minutes INTEGER,
    pattern_evidence JSONB DEFAULT '{}',
    prevention_applied BOOLEAN NOT NULL DEFAULT false,
    prevention_action VARCHAR(50),
    resolved BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_od_pattern_resource ON oscillation_detections (pattern_id, resource_id);
CREATE INDEX idx_od_detected_at ON oscillation_detections (detected_at);
CREATE INDEX idx_od_unresolved ON oscillation_detections (resolved) WHERE resolved = false;

CREATE OR REPLACE VIEW oscillation_detection_summary AS
SELECT op.pattern_type, COUNT(*) as detection_count, AVG(od.confidence) as avg_confidence
FROM oscillation_detections od JOIN oscillation_patterns op ON od.pattern_id = op.id GROUP BY pattern_type;

CREATE TABLE IF NOT EXISTS action_effectiveness_metrics (
    id BIGSERIAL PRIMARY KEY,
    scope_type VARCHAR(50) NOT NULL,
    scope_value VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    metric_period VARCHAR(20) NOT NULL,
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    total_actions INTEGER NOT NULL DEFAULT 0,
    average_score DECIMAL(4,3),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_aem_scope_period ON action_effectiveness_metrics (scope_type, scope_value, metric_period);
CREATE INDEX idx_aem_period_range ON action_effectiveness_metrics (period_start, period_end);
CREATE INDEX idx_aem_action_effectiveness ON action_effectiveness_metrics (action_type, average_score);

CREATE TABLE IF NOT EXISTS action_assessments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255) NOT NULL,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    scheduled_for TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW() + INTERVAL '5 minutes',
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_action_assessments_status_scheduled ON action_assessments(status, scheduled_for) WHERE status = 'pending';
CREATE INDEX idx_action_assessments_trace_id ON action_assessments(trace_id);
CREATE INDEX idx_action_assessments_context ON action_assessments(action_type, context_hash);

CREATE TABLE IF NOT EXISTS effectiveness_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL UNIQUE,
    action_type VARCHAR(100) NOT NULL,
    overall_score FLOAT NOT NULL CHECK (overall_score >= 0 AND overall_score <= 1),
    alert_resolved BOOLEAN NOT NULL,
    metric_delta JSONB,
    side_effects INTEGER DEFAULT 0,
    confidence FLOAT NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    assessed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    recommended_adjustments JSONB,
    learning_contribution FLOAT NOT NULL DEFAULT 0.5 CHECK (learning_contribution >= 0 AND learning_contribution <= 1),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_effectiveness_results_action_type ON effectiveness_results(action_type);
CREATE INDEX idx_effectiveness_results_assessed_at ON effectiveness_results(assessed_at);
CREATE INDEX idx_effectiveness_results_score ON effectiveness_results(overall_score);
CREATE INDEX idx_effectiveness_results_learning_query ON effectiveness_results(action_type, assessed_at DESC);

CREATE TABLE IF NOT EXISTS action_confidence_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    base_confidence FLOAT NOT NULL CHECK (base_confidence >= 0 AND base_confidence <= 1),
    adjusted_confidence FLOAT NOT NULL CHECK (adjusted_confidence >= 0 AND adjusted_confidence <= 1),
    adjustment_reason TEXT,
    effectiveness_samples INTEGER DEFAULT 0,
    last_updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(action_type, context_hash)
);

CREATE INDEX idx_action_confidence_context ON action_confidence_scores(action_type, context_hash);
CREATE INDEX idx_action_confidence_updated ON action_confidence_scores(last_updated);

CREATE TABLE IF NOT EXISTS action_outcomes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    success BOOLEAN NOT NULL,
    alert_resolved BOOLEAN NOT NULL,
    side_effects INTEGER DEFAULT 0,
    effectiveness_score FLOAT NOT NULL CHECK (effectiveness_score >= 0 AND effectiveness_score <= 1),
    execution_time BIGINT,
    metrics_before JSONB,
    metrics_after JSONB,
    failure_reason TEXT,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    assessed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_action_outcomes_context ON action_outcomes(action_type, context_hash);
CREATE INDEX idx_action_outcomes_executed_at ON action_outcomes(executed_at);
CREATE INDEX idx_action_outcomes_success ON action_outcomes(success);
CREATE INDEX idx_action_outcomes_effectiveness ON action_outcomes(effectiveness_score);
CREATE INDEX idx_action_outcomes_learning_query ON action_outcomes(action_type, context_hash, executed_at DESC);

CREATE TABLE IF NOT EXISTS action_alternatives (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    failed_action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    alternative_action_type VARCHAR(100) NOT NULL,
    success_rate FLOAT NOT NULL DEFAULT 0.5 CHECK (success_rate >= 0 AND success_rate <= 1),
    sample_size INTEGER NOT NULL DEFAULT 0,
    last_success_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(failed_action_type, context_hash, alternative_action_type)
);

CREATE INDEX idx_action_alternatives_failed ON action_alternatives(failed_action_type, context_hash);
CREATE INDEX idx_action_alternatives_success_rate ON action_alternatives(success_rate DESC);

CREATE OR REPLACE VIEW effectiveness_trends AS
SELECT
    action_type,
    DATE_TRUNC('day', assessed_at) as assessment_date,
    COUNT(*) as total_assessments,
    AVG(overall_score) as avg_effectiveness,
    AVG(confidence) as avg_confidence,
    COUNT(CASE WHEN alert_resolved THEN 1 END) as alerts_resolved,
    COUNT(CASE WHEN alert_resolved THEN 1 END)::FLOAT / COUNT(*) as resolution_rate
FROM effectiveness_results
GROUP BY action_type, DATE_TRUNC('day', assessed_at)
ORDER BY action_type, assessment_date;

CREATE OR REPLACE VIEW low_confidence_actions AS
SELECT
    acs.action_type,
    acs.context_hash,
    acs.adjusted_confidence,
    acs.adjustment_reason,
    acs.effectiveness_samples,
    acs.last_updated,
    COALESCE(recent_outcomes.recent_success_rate, 0) as recent_success_rate,
    COALESCE(recent_outcomes.recent_samples, 0) as recent_samples
FROM action_confidence_scores acs
LEFT JOIN (
    SELECT
        action_type,
        context_hash,
        AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END) as recent_success_rate,
        COUNT(*) as recent_samples
    FROM action_outcomes
    WHERE executed_at > NOW() - INTERVAL '7 days'
    GROUP BY action_type, context_hash
) recent_outcomes ON acs.action_type = recent_outcomes.action_type
                 AND acs.context_hash = recent_outcomes.context_hash
WHERE acs.adjusted_confidence < 0.5
ORDER BY acs.adjusted_confidence ASC, acs.last_updated DESC;

COMMENT ON TABLE action_assessments IS 'Pending effectiveness assessments for completed actions';
COMMENT ON TABLE effectiveness_results IS 'Results of AI effectiveness assessments for learning';
COMMENT ON TABLE action_confidence_scores IS 'Dynamic confidence scores that improve through learning';
COMMENT ON TABLE action_outcomes IS 'Historical outcomes for training ML algorithms';
COMMENT ON TABLE action_alternatives IS 'Alternative actions for failed patterns';
COMMENT ON VIEW effectiveness_trends IS 'Daily trends in action effectiveness for monitoring';
COMMENT ON VIEW low_confidence_actions IS 'Actions requiring attention due to poor performance';

-- Stored functions (bodies restored verbatim from migration 001; note that
-- all of the resource_action_traces-referencing bodies below will raise at
-- execution time since that table was dropped by migration 009 and is not
-- restored here -- this Down block restores object presence, not a
-- functioning pre-#1048 runtime).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION store_oscillation_detection(p_pattern_id INTEGER, p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_confidence DECIMAL(4,3), p_action_count INTEGER, p_time_span_minutes INTEGER, p_pattern_evidence JSONB, p_prevention_action VARCHAR(50) DEFAULT NULL)
RETURNS INTEGER AS $$
DECLARE v_resource_id INTEGER; v_detection_id INTEGER;
BEGIN
    SELECT id INTO v_resource_id FROM resource_references WHERE namespace = p_namespace AND kind = p_kind AND name = p_name;
    IF v_resource_id IS NULL THEN
        INSERT INTO resource_references (resource_uid, api_version, kind, name, namespace, last_seen) VALUES (gen_random_uuid()::text, 'apps/v1', p_kind, p_name, p_namespace, NOW()) RETURNING id INTO v_resource_id;
    END IF;
    INSERT INTO oscillation_detections (pattern_id, resource_id, detected_at, confidence, action_count, time_span_minutes, pattern_evidence, prevention_applied, prevention_action)
    VALUES (p_pattern_id, v_resource_id, NOW(), p_confidence, p_action_count, p_time_span_minutes, p_pattern_evidence, p_prevention_action IS NOT NULL, p_prevention_action) RETURNING id INTO v_detection_id;
    RETURN v_detection_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION detect_scale_oscillation(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT 120)
RETURNS TABLE (direction_changes INTEGER, first_change TIMESTAMP WITH TIME ZONE, last_change TIMESTAMP WITH TIME ZONE, avg_effectiveness DECIMAL(4,3), duration_minutes DECIMAL(10,2), severity VARCHAR(20), action_sequence JSONB) AS $$
BEGIN
    RETURN QUERY
    WITH scale_actions AS (
        SELECT rat.id, rat.action_timestamp, rat.action_parameters->>'replicas' as replica_count,
            LAG(rat.action_parameters->>'replicas') OVER (PARTITION BY ah.resource_id ORDER BY rat.action_timestamp) as prev_replica_count,
            LAG(rat.action_timestamp) OVER (PARTITION BY ah.resource_id ORDER BY rat.action_timestamp) as prev_timestamp,
            COALESCE(rat.effectiveness_score, 0.0) as effectiveness_score
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rat.action_type = 'scale_deployment' AND rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    direction_changes AS (
        SELECT id, action_timestamp, replica_count::int, prev_replica_count::int, prev_timestamp, effectiveness_score,
            CASE WHEN replica_count::int > prev_replica_count::int THEN 'up' WHEN replica_count::int < prev_replica_count::int THEN 'down' ELSE 'none' END as direction,
            LAG(CASE WHEN replica_count::int > prev_replica_count::int THEN 'up' WHEN replica_count::int < prev_replica_count::int THEN 'down' ELSE 'none' END) OVER (ORDER BY action_timestamp) as prev_direction
        FROM scale_actions WHERE prev_replica_count IS NOT NULL
    ),
    oscillation_analysis AS (
        SELECT COUNT(*) FILTER (WHERE direction != prev_direction AND direction != 'none' AND prev_direction != 'none') as direction_changes,
            MIN(action_timestamp) as first_change, MAX(action_timestamp) as last_change, AVG(effectiveness_score) as avg_effectiveness,
            EXTRACT(EPOCH FROM (MAX(action_timestamp) - MIN(action_timestamp)))/60 as duration_minutes,
            array_agg(json_build_object('timestamp', action_timestamp, 'replica_count', replica_count, 'direction', direction, 'effectiveness', effectiveness_score) ORDER BY action_timestamp) as action_sequence
        FROM direction_changes
    )
    SELECT oa.direction_changes::INTEGER, oa.first_change, oa.last_change, oa.avg_effectiveness::DECIMAL(4,3), oa.duration_minutes::DECIMAL(10,2),
        CASE WHEN oa.direction_changes >= 4 AND oa.duration_minutes <= 60 AND oa.avg_effectiveness < 0.5 THEN 'critical'
             WHEN oa.direction_changes >= 3 AND oa.duration_minutes <= 120 AND oa.avg_effectiveness < 0.7 THEN 'high'
             WHEN oa.direction_changes >= 2 AND oa.duration_minutes <= 180 THEN 'medium' ELSE 'low' END::VARCHAR(20),
        to_jsonb(oa.action_sequence)
    FROM oscillation_analysis oa WHERE oa.direction_changes >= 2;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION detect_resource_thrashing(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT 120)
RETURNS TABLE (thrashing_transitions INTEGER, total_actions INTEGER, first_action TIMESTAMP WITH TIME ZONE, last_action TIMESTAMP WITH TIME ZONE, avg_effectiveness DECIMAL(4,3), avg_time_gap_minutes DECIMAL(10,2), severity VARCHAR(20)) AS $$
BEGIN
    RETURN QUERY
    WITH resource_actions AS (
        SELECT rat.action_timestamp, rat.action_type, rat.action_parameters, rat.effectiveness_score,
            LAG(rat.action_type) OVER (PARTITION BY ah.resource_id ORDER BY rat.action_timestamp) as prev_action_type,
            LAG(rat.action_timestamp) OVER (PARTITION BY ah.resource_id ORDER BY rat.action_timestamp) as prev_timestamp
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rat.action_type IN ('increase_resources', 'scale_deployment') AND rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    thrashing_patterns AS (
        SELECT action_timestamp, action_type, prev_action_type, COALESCE(effectiveness_score, 0.0) as effectiveness_score,
            EXTRACT(EPOCH FROM (action_timestamp - prev_timestamp))/60 as time_gap_minutes,
            CASE WHEN (action_type = 'increase_resources' AND prev_action_type = 'scale_deployment') OR (action_type = 'scale_deployment' AND prev_action_type = 'increase_resources') THEN 1 ELSE 0 END as is_thrashing_transition
        FROM resource_actions WHERE prev_action_type IS NOT NULL AND action_timestamp - prev_timestamp < INTERVAL '45 minutes'
    ),
    thrashing_analysis AS (
        SELECT COUNT(*) FILTER (WHERE is_thrashing_transition = 1) as thrashing_transitions, COUNT(*) as total_actions,
            MIN(action_timestamp) as first_action, MAX(action_timestamp) as last_action, AVG(effectiveness_score) as avg_effectiveness, AVG(time_gap_minutes) as avg_time_gap_minutes
        FROM thrashing_patterns
    )
    SELECT ta.thrashing_transitions::INTEGER, ta.total_actions::INTEGER, ta.first_action, ta.last_action, ta.avg_effectiveness::DECIMAL(4,3), ta.avg_time_gap_minutes::DECIMAL(10,2),
        CASE WHEN ta.thrashing_transitions >= 3 AND ta.avg_effectiveness < 0.6 THEN 'critical' WHEN ta.thrashing_transitions >= 2 AND ta.avg_effectiveness < 0.7 THEN 'high'
             WHEN ta.thrashing_transitions >= 1 AND ta.avg_time_gap_minutes < 15 THEN 'medium' ELSE 'low' END::VARCHAR(20)
    FROM thrashing_analysis ta WHERE ta.thrashing_transitions >= 1;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION detect_ineffective_loops(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT 120)
RETURNS TABLE (action_type VARCHAR(50), repetition_count INTEGER, avg_effectiveness DECIMAL(4,3), effectiveness_stddev DECIMAL(4,3), first_occurrence TIMESTAMP WITH TIME ZONE, last_occurrence TIMESTAMP WITH TIME ZONE, span_minutes DECIMAL(10,2), severity VARCHAR(20), effectiveness_trend DECIMAL(6,3), effectiveness_scores DECIMAL(4,3)[], timestamps TIMESTAMP WITH TIME ZONE[]) AS $$
BEGIN
    RETURN QUERY
    WITH repeated_actions AS (
        SELECT rat.action_type, COUNT(*) as repetition_count, AVG(COALESCE(rat.effectiveness_score, 0.0)) as avg_effectiveness,
            STDDEV(COALESCE(rat.effectiveness_score, 0.0)) as effectiveness_stddev, MIN(rat.action_timestamp) as first_occurrence, MAX(rat.action_timestamp) as last_occurrence,
            EXTRACT(EPOCH FROM (MAX(rat.action_timestamp) - MIN(rat.action_timestamp)))/60 as span_minutes,
            array_agg(COALESCE(rat.effectiveness_score, 0.0) ORDER BY rat.action_timestamp) as effectiveness_scores,
            array_agg(rat.action_timestamp ORDER BY rat.action_timestamp) as timestamps
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
        GROUP BY rat.action_type
    ),
    ineffective_patterns AS (
        SELECT ra.action_type, ra.repetition_count, ra.avg_effectiveness, COALESCE(ra.effectiveness_stddev, 0.0) as effectiveness_stddev,
            ra.first_occurrence, ra.last_occurrence, ra.span_minutes, ra.effectiveness_scores, ra.timestamps,
            CASE WHEN ra.repetition_count >= 5 AND ra.avg_effectiveness < 0.3 THEN 'critical' WHEN ra.repetition_count >= 4 AND ra.avg_effectiveness < 0.5 THEN 'high'
                 WHEN ra.repetition_count >= 3 AND ra.avg_effectiveness < 0.6 THEN 'medium' WHEN ra.repetition_count >= 2 AND ra.avg_effectiveness < 0.4 THEN 'low' ELSE 'none' END as severity,
            CASE WHEN ra.repetition_count >= 3 THEN (ra.effectiveness_scores[array_length(ra.effectiveness_scores, 1)] - ra.effectiveness_scores[1]) / GREATEST(ra.effectiveness_scores[1], 0.1) ELSE 0 END as effectiveness_trend
        FROM repeated_actions ra WHERE ra.repetition_count >= 2
    )
    SELECT ip.action_type, ip.repetition_count::INTEGER, ip.avg_effectiveness::DECIMAL(4,3), ip.effectiveness_stddev::DECIMAL(4,3),
        ip.first_occurrence, ip.last_occurrence, ip.span_minutes::DECIMAL(10,2), ip.severity::VARCHAR(20), ip.effectiveness_trend::DECIMAL(6,3), ip.effectiveness_scores::DECIMAL(4,3)[], ip.timestamps
    FROM ineffective_patterns ip WHERE ip.severity != 'none'
    ORDER BY CASE ip.severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END, ip.avg_effectiveness ASC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION detect_cascading_failures(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT 120)
RETURNS TABLE (action_type VARCHAR(50), total_actions INTEGER, avg_new_alerts DECIMAL(6,2), recurrence_rate DECIMAL(4,3), avg_effectiveness DECIMAL(4,3), actions_causing_cascades INTEGER, max_alerts_triggered INTEGER, severity VARCHAR(20)) AS $$
BEGIN
    RETURN QUERY
    WITH action_outcomes AS (
        SELECT rat.id, rat.action_timestamp, rat.action_type, rat.signal_name as original_alert, COALESCE(rat.effectiveness_score, 0.0) as effectiveness_score,
            (SELECT COUNT(DISTINCT rat2.signal_name) FROM resource_action_traces rat2 JOIN action_histories ah2 ON rat2.action_history_id = ah2.id
             WHERE ah2.resource_id = ah.resource_id AND rat2.action_timestamp BETWEEN rat.action_timestamp AND rat.action_timestamp + INTERVAL '30 minutes' AND rat2.signal_name != rat.signal_name) as new_alerts_triggered,
            (SELECT COUNT(*) FROM resource_action_traces rat3 JOIN action_histories ah3 ON rat3.action_history_id = ah3.id
             WHERE ah3.resource_id = ah.resource_id AND rat3.action_timestamp > rat.action_timestamp AND rat3.signal_name = rat.signal_name LIMIT 1) as original_alert_recurred
        FROM resource_action_traces rat JOIN action_histories ah ON rat.action_history_id = ah.id JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    cascading_analysis AS (
        SELECT ao.action_type, COUNT(*) as total_actions, AVG(ao.new_alerts_triggered::float) as avg_new_alerts,
            AVG(CASE WHEN ao.original_alert_recurred > 0 THEN 1.0 ELSE 0.0 END) as recurrence_rate, AVG(ao.effectiveness_score) as avg_effectiveness,
            SUM(CASE WHEN ao.new_alerts_triggered > 0 THEN 1 ELSE 0 END) as actions_causing_cascades, MAX(ao.new_alerts_triggered) as max_alerts_triggered
        FROM action_outcomes ao GROUP BY ao.action_type
    )
    SELECT ca.action_type, ca.total_actions::INTEGER, ca.avg_new_alerts::DECIMAL(6,2), ca.recurrence_rate::DECIMAL(4,3), ca.avg_effectiveness::DECIMAL(4,3),
        ca.actions_causing_cascades::INTEGER, ca.max_alerts_triggered::INTEGER,
        CASE WHEN ca.avg_new_alerts > 2.0 AND ca.recurrence_rate > 0.5 THEN 'critical' WHEN ca.avg_new_alerts > 1.5 OR ca.recurrence_rate > 0.7 THEN 'high'
             WHEN ca.avg_new_alerts > 1.0 OR ca.recurrence_rate > 0.4 THEN 'medium' WHEN ca.actions_causing_cascades > 0 THEN 'low' ELSE 'none' END::VARCHAR(20)
    FROM cascading_analysis ca WHERE ca.actions_causing_cascades > 0 ORDER BY ca.avg_new_alerts DESC, ca.recurrence_rate DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_action_traces(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_action_type VARCHAR(50) DEFAULT NULL, p_model_used VARCHAR(100) DEFAULT NULL, p_time_start TIMESTAMP WITH TIME ZONE DEFAULT NULL, p_time_end TIMESTAMP WITH TIME ZONE DEFAULT NULL, p_limit INTEGER DEFAULT 50, p_offset INTEGER DEFAULT 0)
RETURNS TABLE (action_id VARCHAR(64), action_timestamp TIMESTAMP WITH TIME ZONE, action_type VARCHAR(50), model_used VARCHAR(100), model_confidence DECIMAL(4,3), execution_status VARCHAR(20), effectiveness_score DECIMAL(4,3), model_reasoning TEXT, action_parameters JSONB, signal_name VARCHAR(200), signal_severity VARCHAR(20)) AS $$
BEGIN
    RETURN QUERY
    SELECT rat.action_id, rat.action_timestamp, rat.action_type, rat.model_used, rat.model_confidence, rat.execution_status, rat.effectiveness_score, rat.model_reasoning, rat.action_parameters, rat.signal_name, rat.signal_severity
    FROM resource_action_traces rat JOIN action_histories ah ON rat.action_history_id = ah.id JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name
    AND (p_action_type IS NULL OR rat.action_type = p_action_type) AND (p_model_used IS NULL OR rat.model_used = p_model_used)
    AND (p_time_start IS NULL OR rat.action_timestamp >= p_time_start) AND (p_time_end IS NULL OR rat.action_timestamp <= p_time_end)
    ORDER BY rat.action_timestamp DESC LIMIT p_limit OFFSET p_offset;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_action_effectiveness(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_action_type VARCHAR(50) DEFAULT NULL, p_time_start TIMESTAMP WITH TIME ZONE DEFAULT NOW() - INTERVAL '7 days', p_time_end TIMESTAMP WITH TIME ZONE DEFAULT NOW())
RETURNS TABLE (action_type VARCHAR(50), sample_size INTEGER, avg_effectiveness DECIMAL(4,3), stddev_effectiveness DECIMAL(4,3), min_effectiveness DECIMAL(4,3), max_effectiveness DECIMAL(4,3), success_rate DECIMAL(4,3)) AS $$
BEGIN
    RETURN QUERY
    SELECT rat.action_type, COUNT(*)::INTEGER, AVG(rat.effectiveness_score)::DECIMAL(4,3), STDDEV(rat.effectiveness_score)::DECIMAL(4,3), MIN(rat.effectiveness_score)::DECIMAL(4,3), MAX(rat.effectiveness_score)::DECIMAL(4,3), AVG(CASE WHEN rat.execution_status = 'completed' THEN 1.0 ELSE 0.0 END)::DECIMAL(4,3)
    FROM resource_action_traces rat JOIN action_histories ah ON rat.action_history_id = ah.id JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name AND rat.effectiveness_score IS NOT NULL AND rat.action_timestamp BETWEEN p_time_start AND p_time_end AND (p_action_type IS NULL OR rat.action_type = p_action_type)
    GROUP BY rat.action_type HAVING COUNT(*) >= 1 ORDER BY avg_effectiveness DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_resource_actions_base(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT NULL)
RETURNS TABLE (trace_id BIGINT, action_timestamp TIMESTAMP WITH TIME ZONE, action_type VARCHAR(50), action_parameters JSONB, effectiveness_score DECIMAL(4,3), model_confidence DECIMAL(4,3), execution_status VARCHAR(20)) AS $$
BEGIN
    RETURN QUERY
    SELECT rat.id as trace_id, rat.action_timestamp, rat.action_type, rat.action_parameters, rat.effectiveness_score, rat.model_confidence, rat.execution_status
    FROM resource_action_traces rat JOIN action_histories ah ON rat.action_history_id = ah.id JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name AND (p_window_minutes IS NULL OR rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes)
    ORDER BY rat.action_timestamp DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_resource_id(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253))
RETURNS INTEGER AS $$
DECLARE v_resource_id INTEGER;
BEGIN
    SELECT id INTO v_resource_id FROM resource_references WHERE namespace = p_namespace AND kind = p_kind AND name = p_name;
    IF v_resource_id IS NULL THEN RAISE EXCEPTION 'Resource not found: namespace=%, kind=%, name=%', p_namespace, p_kind, p_name; END IF;
    RETURN v_resource_id;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION analyze_action_oscillation(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253), p_window_minutes INTEGER DEFAULT 120)
RETURNS TABLE (action_timestamp TIMESTAMP WITH TIME ZONE, action_type VARCHAR(50), effectiveness_score DECIMAL(4,3), prev_timestamp TIMESTAMP WITH TIME ZONE, prev_action_type VARCHAR(50), time_gap_minutes DECIMAL(10,2), action_sequence_position INTEGER) AS $$
BEGIN
    RETURN QUERY
    WITH action_analysis AS (
        SELECT rat.action_timestamp, rat.action_type, rat.effectiveness_score, LAG(rat.action_timestamp) OVER (ORDER BY rat.action_timestamp) as prev_timestamp, LAG(rat.action_type) OVER (ORDER BY rat.action_timestamp) as prev_action_type, ROW_NUMBER() OVER (ORDER BY rat.action_timestamp) as sequence_position
        FROM resource_action_traces rat JOIN action_histories ah ON rat.action_history_id = ah.id JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace AND rr.kind = p_kind AND rr.name = p_name AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    )
    SELECT aa.action_timestamp, aa.action_type, aa.effectiveness_score, aa.prev_timestamp, aa.prev_action_type,
        CASE WHEN aa.prev_timestamp IS NOT NULL THEN EXTRACT(EPOCH FROM (aa.action_timestamp - aa.prev_timestamp))/60 ELSE 0 END::DECIMAL(10,2), aa.sequence_position::INTEGER
    FROM action_analysis aa ORDER BY aa.action_timestamp;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION analyze_cascade_effects(p_days_back INTEGER DEFAULT 7, p_time_window INTERVAL DEFAULT '1 hour'::interval, p_max_signals INTEGER DEFAULT NULL)
RETURNS TABLE (action_type VARCHAR, avg_new_signals NUMERIC, max_signals_triggered INTEGER, actions_causing_cascades INTEGER, total_actions INTEGER, cascade_rate NUMERIC) AS $$
BEGIN
    RETURN QUERY
    WITH action_outcomes AS (
        SELECT rat.action_type, rat.action_id, rat.action_timestamp, rat.signal_name as original_signal, rat.execution_status,
            (SELECT COUNT(DISTINCT rat2.signal_name) FROM resource_action_traces rat2 WHERE rat2.action_timestamp BETWEEN rat.action_timestamp AND rat.action_timestamp + p_time_window AND rat2.signal_name != rat.signal_name) as new_signals_triggered,
            (SELECT COUNT(*) FROM resource_action_traces rat3 WHERE rat3.action_timestamp > rat.action_timestamp AND rat3.action_timestamp <= rat.action_timestamp + INTERVAL '24 hours' AND rat3.signal_name = rat.signal_name) as recurrence_count
        FROM resource_action_traces rat WHERE rat.action_timestamp >= NOW() - (p_days_back || ' days')::INTERVAL AND rat.execution_status = 'completed'
    )
    SELECT ao.action_type::VARCHAR, ROUND(AVG(ao.new_signals_triggered::float), 2) as avg_new_signals, MAX(ao.new_signals_triggered)::INTEGER as max_signals_triggered,
        SUM(CASE WHEN ao.new_signals_triggered > 0 THEN 1 ELSE 0 END)::INTEGER as actions_causing_cascades, COUNT(*)::INTEGER as total_actions,
        ROUND((SUM(CASE WHEN ao.new_signals_triggered > 0 THEN 1 ELSE 0 END)::float / COUNT(*)) * 100, 2) as cascade_rate
    FROM action_outcomes ao GROUP BY ao.action_type HAVING p_max_signals IS NULL OR MAX(ao.new_signals_triggered) <= p_max_signals ORDER BY cascade_rate DESC;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_recent_actions(p_limit INTEGER DEFAULT 100, p_signal_name VARCHAR(200) DEFAULT NULL, p_signal_severity VARCHAR(20) DEFAULT NULL)
RETURNS TABLE (action_id VARCHAR, action_timestamp TIMESTAMP WITH TIME ZONE, signal_name VARCHAR, signal_severity VARCHAR, execution_status VARCHAR) AS $$
BEGIN
    RETURN QUERY
    SELECT rat.action_id::VARCHAR, rat.action_timestamp, rat.signal_name::VARCHAR, rat.signal_severity::VARCHAR, rat.execution_status
    FROM resource_action_traces rat WHERE (p_signal_name IS NULL OR rat.signal_name = p_signal_name) AND (p_signal_severity IS NULL OR rat.signal_severity = p_signal_severity)
    ORDER BY rat.action_timestamp DESC LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION create_assessment_for_action_trace()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.execution_status = 'completed' THEN
        INSERT INTO action_assessments (
            trace_id, action_type, context_hash, alert_name,
            namespace, resource_name, executed_at, scheduled_for
        ) VALUES (
            NEW.id::VARCHAR,
            NEW.action_type,
            encode(sha256(CONCAT(NEW.action_type, ':', COALESCE(NEW.alert_name, 'no-alert'))::bytea), 'hex'),
            COALESCE(NEW.alert_name, 'no-alert'),
            'unknown',
            'unknown',
            COALESCE(NEW.execution_end_time, NEW.action_timestamp),
            NOW() + INTERVAL '5 minutes'
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Note: trigger_create_assessment_for_action_trace (on resource_action_traces)
-- is NOT restored here -- that table was dropped by migration 009 and its
-- own Down block does not restore it either, so there is nothing left to
-- attach this trigger to.
