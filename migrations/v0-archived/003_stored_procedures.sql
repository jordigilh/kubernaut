-- Migration: 003_stored_procedures.sql
-- Replace hardcoded queries with PostgreSQL stored procedures for better performance and maintainability

-- =============================================================================
-- OSCILLATION DETECTION PROCEDURES
-- =============================================================================

-- 1. Scale Oscillation Detection
CREATE OR REPLACE FUNCTION detect_scale_oscillation(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100), 
    p_name VARCHAR(253),
    p_window_minutes INTEGER DEFAULT 120
)
RETURNS TABLE (
    direction_changes INTEGER,
    first_change TIMESTAMP WITH TIME ZONE,
    last_change TIMESTAMP WITH TIME ZONE,
    avg_effectiveness DECIMAL(4,3),
    duration_minutes DECIMAL(10,2),
    severity VARCHAR(20),
    action_sequence JSONB
) AS $$
BEGIN
    RETURN QUERY
    WITH scale_actions AS (
        SELECT 
            rat.id,
            rat.action_timestamp,
            rat.action_parameters->>'replicas' as replica_count,
            LAG(rat.action_parameters->>'replicas') OVER (
                PARTITION BY ah.resource_id 
                ORDER BY rat.action_timestamp
            ) as prev_replica_count,
            LAG(rat.action_timestamp) OVER (
                PARTITION BY ah.resource_id 
                ORDER BY rat.action_timestamp  
            ) as prev_timestamp,
            COALESCE(rat.effectiveness_score, 0.0) as effectiveness_score
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rat.action_type = 'scale_deployment'
        AND rr.namespace = p_namespace 
        AND rr.kind = p_kind 
        AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    direction_changes AS (
        SELECT 
            id,
            action_timestamp,
            replica_count::int,
            prev_replica_count::int,
            prev_timestamp,
            effectiveness_score,
            CASE 
                WHEN replica_count::int > prev_replica_count::int THEN 'up'
                WHEN replica_count::int < prev_replica_count::int THEN 'down'
                ELSE 'none'
            END as direction,
            LAG(CASE 
                WHEN replica_count::int > prev_replica_count::int THEN 'up'
                WHEN replica_count::int < prev_replica_count::int THEN 'down'
                ELSE 'none'
            END) OVER (ORDER BY action_timestamp) as prev_direction
        FROM scale_actions
        WHERE prev_replica_count IS NOT NULL
    ),
    oscillation_analysis AS (
        SELECT 
            COUNT(*) FILTER (WHERE direction != prev_direction AND direction != 'none' AND prev_direction != 'none') as direction_changes,
            MIN(action_timestamp) as first_change,
            MAX(action_timestamp) as last_change,
            AVG(effectiveness_score) as avg_effectiveness,
            EXTRACT(EPOCH FROM (MAX(action_timestamp) - MIN(action_timestamp)))/60 as duration_minutes,
            array_agg(
                json_build_object(
                    'timestamp', action_timestamp,
                    'replica_count', replica_count,
                    'direction', direction,
                    'effectiveness', effectiveness_score
                ) ORDER BY action_timestamp
            ) as action_sequence
        FROM direction_changes
    )
    SELECT 
        oa.direction_changes::INTEGER,
        oa.first_change,
        oa.last_change,
        oa.avg_effectiveness::DECIMAL(4,3),
        oa.duration_minutes::DECIMAL(10,2),
        CASE 
            WHEN oa.direction_changes >= 4 AND oa.duration_minutes <= 60 AND oa.avg_effectiveness < 0.5 THEN 'critical'
            WHEN oa.direction_changes >= 3 AND oa.duration_minutes <= 120 AND oa.avg_effectiveness < 0.7 THEN 'high'
            WHEN oa.direction_changes >= 2 AND oa.duration_minutes <= 180 THEN 'medium'
            ELSE 'low'
        END::VARCHAR(20) as severity,
        to_jsonb(oa.action_sequence) as action_sequence
    FROM oscillation_analysis oa
    WHERE oa.direction_changes >= 2;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 2. Resource Thrashing Detection
CREATE OR REPLACE FUNCTION detect_resource_thrashing(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253), 
    p_window_minutes INTEGER DEFAULT 120
)
RETURNS TABLE (
    thrashing_transitions INTEGER,
    total_actions INTEGER,
    first_action TIMESTAMP WITH TIME ZONE,
    last_action TIMESTAMP WITH TIME ZONE,
    avg_effectiveness DECIMAL(4,3),
    avg_time_gap_minutes DECIMAL(10,2),
    severity VARCHAR(20)
) AS $$
BEGIN
    RETURN QUERY
    WITH resource_actions AS (
        SELECT 
            rat.action_timestamp,
            rat.action_type,
            rat.action_parameters,
            rat.effectiveness_score,
            LAG(rat.action_type) OVER (
                PARTITION BY ah.resource_id 
                ORDER BY rat.action_timestamp
            ) as prev_action_type,
            LAG(rat.action_timestamp) OVER (
                PARTITION BY ah.resource_id 
                ORDER BY rat.action_timestamp
            ) as prev_timestamp
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rat.action_type IN ('increase_resources', 'scale_deployment')
        AND rr.namespace = p_namespace 
        AND rr.kind = p_kind 
        AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    thrashing_patterns AS (
        SELECT 
            action_timestamp,
            action_type,
            prev_action_type,
            COALESCE(effectiveness_score, 0.0) as effectiveness_score,
            EXTRACT(EPOCH FROM (action_timestamp - prev_timestamp))/60 as time_gap_minutes,
            CASE 
                WHEN (action_type = 'increase_resources' AND prev_action_type = 'scale_deployment') OR
                     (action_type = 'scale_deployment' AND prev_action_type = 'increase_resources')
                THEN 1 ELSE 0
            END as is_thrashing_transition
        FROM resource_actions
        WHERE prev_action_type IS NOT NULL
        AND action_timestamp - prev_timestamp < INTERVAL '45 minutes'
    ),
    thrashing_analysis AS (
        SELECT 
            COUNT(*) FILTER (WHERE is_thrashing_transition = 1) as thrashing_transitions,
            COUNT(*) as total_actions,
            MIN(action_timestamp) as first_action,
            MAX(action_timestamp) as last_action,
            AVG(effectiveness_score) as avg_effectiveness,
            AVG(time_gap_minutes) as avg_time_gap_minutes
        FROM thrashing_patterns
    )
    SELECT 
        ta.thrashing_transitions::INTEGER,
        ta.total_actions::INTEGER,
        ta.first_action,
        ta.last_action,
        ta.avg_effectiveness::DECIMAL(4,3),
        ta.avg_time_gap_minutes::DECIMAL(10,2),
        CASE 
            WHEN ta.thrashing_transitions >= 3 AND ta.avg_effectiveness < 0.6 THEN 'critical'
            WHEN ta.thrashing_transitions >= 2 AND ta.avg_effectiveness < 0.7 THEN 'high'
            WHEN ta.thrashing_transitions >= 1 AND ta.avg_time_gap_minutes < 15 THEN 'medium'
            ELSE 'low'
        END::VARCHAR(20) as severity
    FROM thrashing_analysis ta
    WHERE ta.thrashing_transitions >= 1;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 3. Ineffective Loop Detection
CREATE OR REPLACE FUNCTION detect_ineffective_loops(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_window_minutes INTEGER DEFAULT 120
)
RETURNS TABLE (
    action_type VARCHAR(50),
    repetition_count INTEGER,
    avg_effectiveness DECIMAL(4,3),
    effectiveness_stddev DECIMAL(4,3),
    first_occurrence TIMESTAMP WITH TIME ZONE,
    last_occurrence TIMESTAMP WITH TIME ZONE,
    span_minutes DECIMAL(10,2),
    severity VARCHAR(20),
    effectiveness_trend DECIMAL(6,3),
    effectiveness_scores DECIMAL(4,3)[],
    timestamps TIMESTAMP WITH TIME ZONE[]
) AS $$
BEGIN
    RETURN QUERY
    WITH repeated_actions AS (
        SELECT 
            rat.action_type,
            COUNT(*) as repetition_count,
            AVG(COALESCE(rat.effectiveness_score, 0.0)) as avg_effectiveness,
            STDDEV(COALESCE(rat.effectiveness_score, 0.0)) as effectiveness_stddev,
            MIN(rat.action_timestamp) as first_occurrence,
            MAX(rat.action_timestamp) as last_occurrence,
            EXTRACT(EPOCH FROM (MAX(rat.action_timestamp) - MIN(rat.action_timestamp)))/60 as span_minutes,
            array_agg(COALESCE(rat.effectiveness_score, 0.0) ORDER BY rat.action_timestamp) as effectiveness_scores,
            array_agg(rat.action_timestamp ORDER BY rat.action_timestamp) as timestamps
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace 
        AND rr.kind = p_kind 
        AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
        GROUP BY rat.action_type
    ),
    ineffective_patterns AS (
        SELECT 
            ra.action_type,
            ra.repetition_count,
            ra.avg_effectiveness,
            COALESCE(ra.effectiveness_stddev, 0.0) as effectiveness_stddev,
            ra.first_occurrence,
            ra.last_occurrence,
            ra.span_minutes,
            ra.effectiveness_scores,
            ra.timestamps,
            CASE 
                WHEN ra.repetition_count >= 5 AND ra.avg_effectiveness < 0.3 THEN 'critical'
                WHEN ra.repetition_count >= 4 AND ra.avg_effectiveness < 0.5 THEN 'high'
                WHEN ra.repetition_count >= 3 AND ra.avg_effectiveness < 0.6 THEN 'medium'
                WHEN ra.repetition_count >= 2 AND ra.avg_effectiveness < 0.4 THEN 'low'
                ELSE 'none'
            END as severity,
            CASE 
                WHEN ra.repetition_count >= 3 THEN
                    (ra.effectiveness_scores[array_length(ra.effectiveness_scores, 1)] - ra.effectiveness_scores[1]) / 
                    GREATEST(ra.effectiveness_scores[1], 0.1)
                ELSE 0
            END as effectiveness_trend
        FROM repeated_actions ra
        WHERE ra.repetition_count >= 2
    )
    SELECT 
        ip.action_type,
        ip.repetition_count::INTEGER,
        ip.avg_effectiveness::DECIMAL(4,3),
        ip.effectiveness_stddev::DECIMAL(4,3),
        ip.first_occurrence,
        ip.last_occurrence,
        ip.span_minutes::DECIMAL(10,2),
        ip.severity::VARCHAR(20),
        ip.effectiveness_trend::DECIMAL(6,3),
        ip.effectiveness_scores::DECIMAL(4,3)[],
        ip.timestamps
    FROM ineffective_patterns ip
    WHERE ip.severity != 'none'
    ORDER BY 
        CASE ip.severity 
            WHEN 'critical' THEN 1 
            WHEN 'high' THEN 2 
            WHEN 'medium' THEN 3 
            ELSE 4 
        END,
        ip.avg_effectiveness ASC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 4. Cascading Failure Detection  
CREATE OR REPLACE FUNCTION detect_cascading_failures(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_window_minutes INTEGER DEFAULT 120
)
RETURNS TABLE (
    action_type VARCHAR(50),
    total_actions INTEGER,
    avg_new_alerts DECIMAL(6,2),
    recurrence_rate DECIMAL(4,3),
    avg_effectiveness DECIMAL(4,3),
    actions_causing_cascades INTEGER,
    max_alerts_triggered INTEGER,
    severity VARCHAR(20)
) AS $$
BEGIN
    RETURN QUERY
    WITH action_outcomes AS (
        SELECT 
            rat.id,
            rat.action_timestamp,
            rat.action_type,
            rat.alert_name as original_alert,
            COALESCE(rat.effectiveness_score, 0.0) as effectiveness_score,
            (
                SELECT COUNT(DISTINCT rat2.alert_name)
                FROM resource_action_traces rat2
                JOIN action_histories ah2 ON rat2.action_history_id = ah2.id
                WHERE ah2.resource_id = ah.resource_id
                AND rat2.action_timestamp BETWEEN rat.action_timestamp AND rat.action_timestamp + INTERVAL '30 minutes'
                AND rat2.alert_name != rat.alert_name
            ) as new_alerts_triggered,
            (
                SELECT COUNT(*)
                FROM resource_action_traces rat3
                JOIN action_histories ah3 ON rat3.action_history_id = ah3.id
                WHERE ah3.resource_id = ah.resource_id
                AND rat3.action_timestamp > rat.action_timestamp
                AND rat3.alert_name = rat.alert_name
                LIMIT 1
            ) as original_alert_recurred
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace 
        AND rr.kind = p_kind 
        AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    ),
    cascading_analysis AS (
        SELECT 
            ao.action_type,
            COUNT(*) as total_actions,
            AVG(ao.new_alerts_triggered::float) as avg_new_alerts,
            AVG(CASE WHEN ao.original_alert_recurred > 0 THEN 1.0 ELSE 0.0 END) as recurrence_rate,
            AVG(ao.effectiveness_score) as avg_effectiveness,
            SUM(CASE WHEN ao.new_alerts_triggered > 0 THEN 1 ELSE 0 END) as actions_causing_cascades,
            MAX(ao.new_alerts_triggered) as max_alerts_triggered
        FROM action_outcomes ao
        GROUP BY ao.action_type
    )
    SELECT 
        ca.action_type,
        ca.total_actions::INTEGER,
        ca.avg_new_alerts::DECIMAL(6,2),
        ca.recurrence_rate::DECIMAL(4,3),
        ca.avg_effectiveness::DECIMAL(4,3),
        ca.actions_causing_cascades::INTEGER,
        ca.max_alerts_triggered::INTEGER,
        CASE 
            WHEN ca.avg_new_alerts > 2.0 AND ca.recurrence_rate > 0.5 THEN 'critical'
            WHEN ca.avg_new_alerts > 1.5 OR ca.recurrence_rate > 0.7 THEN 'high'
            WHEN ca.avg_new_alerts > 1.0 OR ca.recurrence_rate > 0.4 THEN 'medium'
            WHEN ca.actions_causing_cascades > 0 THEN 'low'
            ELSE 'none'
        END::VARCHAR(20) as severity
    FROM cascading_analysis ca
    WHERE ca.actions_causing_cascades > 0
    ORDER BY ca.avg_new_alerts DESC, ca.recurrence_rate DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- =============================================================================
-- ACTION HISTORY MANAGEMENT PROCEDURES
-- =============================================================================

-- 5. Get Action History with Filters
CREATE OR REPLACE FUNCTION get_action_traces(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_action_type VARCHAR(50) DEFAULT NULL,
    p_model_used VARCHAR(100) DEFAULT NULL,
    p_time_start TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    p_time_end TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    p_limit INTEGER DEFAULT 50,
    p_offset INTEGER DEFAULT 0
)
RETURNS TABLE (
    action_id VARCHAR(64),
    action_timestamp TIMESTAMP WITH TIME ZONE,
    action_type VARCHAR(50),
    model_used VARCHAR(100),
    model_confidence DECIMAL(4,3),
    execution_status VARCHAR(20),
    effectiveness_score DECIMAL(4,3),
    model_reasoning TEXT,
    action_parameters JSONB,
    alert_name VARCHAR(200),
    alert_severity VARCHAR(20)
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        rat.action_id,
        rat.action_timestamp,
        rat.action_type,
        rat.model_used,
        rat.model_confidence,
        rat.execution_status,
        rat.effectiveness_score,
        rat.model_reasoning,
        rat.action_parameters,
        rat.alert_name,
        rat.alert_severity
    FROM resource_action_traces rat
    JOIN action_histories ah ON rat.action_history_id = ah.id
    JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace 
    AND rr.kind = p_kind 
    AND rr.name = p_name
    AND (p_action_type IS NULL OR rat.action_type = p_action_type)
    AND (p_model_used IS NULL OR rat.model_used = p_model_used)
    AND (p_time_start IS NULL OR rat.action_timestamp >= p_time_start)
    AND (p_time_end IS NULL OR rat.action_timestamp <= p_time_end)
    ORDER BY rat.action_timestamp DESC
    LIMIT p_limit
    OFFSET p_offset;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 6. Get Action Effectiveness Metrics
CREATE OR REPLACE FUNCTION get_action_effectiveness(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_action_type VARCHAR(50) DEFAULT NULL,
    p_time_start TIMESTAMP WITH TIME ZONE DEFAULT NOW() - INTERVAL '7 days',
    p_time_end TIMESTAMP WITH TIME ZONE DEFAULT NOW()
)
RETURNS TABLE (
    action_type VARCHAR(50),
    sample_size INTEGER,
    avg_effectiveness DECIMAL(4,3),
    stddev_effectiveness DECIMAL(4,3),
    min_effectiveness DECIMAL(4,3),
    max_effectiveness DECIMAL(4,3),
    success_rate DECIMAL(4,3)
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        rat.action_type,
        COUNT(*)::INTEGER as sample_size,
        AVG(rat.effectiveness_score)::DECIMAL(4,3) as avg_effectiveness,
        STDDEV(rat.effectiveness_score)::DECIMAL(4,3) as stddev_effectiveness,
        MIN(rat.effectiveness_score)::DECIMAL(4,3) as min_effectiveness,
        MAX(rat.effectiveness_score)::DECIMAL(4,3) as max_effectiveness,
        AVG(CASE WHEN rat.execution_status = 'completed' THEN 1.0 ELSE 0.0 END)::DECIMAL(4,3) as success_rate
    FROM resource_action_traces rat
    JOIN action_histories ah ON rat.action_history_id = ah.id
    JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace 
    AND rr.kind = p_kind 
    AND rr.name = p_name
    AND rat.effectiveness_score IS NOT NULL
    AND rat.action_timestamp BETWEEN p_time_start AND p_time_end
    AND (p_action_type IS NULL OR rat.action_type = p_action_type)
    GROUP BY rat.action_type
    HAVING COUNT(*) >= 1
    ORDER BY avg_effectiveness DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 7. Store Oscillation Detection Result
CREATE OR REPLACE FUNCTION store_oscillation_detection(
    p_pattern_id INTEGER,
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_confidence DECIMAL(4,3),
    p_action_count INTEGER,
    p_time_span_minutes INTEGER,
    p_pattern_evidence JSONB,
    p_prevention_action VARCHAR(50) DEFAULT NULL
)
RETURNS INTEGER AS $$
DECLARE
    v_resource_id INTEGER;
    v_detection_id INTEGER;
BEGIN
    -- Get or create resource reference
    SELECT id INTO v_resource_id
    FROM resource_references 
    WHERE namespace = p_namespace AND kind = p_kind AND name = p_name;
    
    IF v_resource_id IS NULL THEN
        INSERT INTO resource_references (resource_uid, api_version, kind, name, namespace, last_seen)
        VALUES (gen_random_uuid()::text, 'apps/v1', p_kind, p_name, p_namespace, NOW())
        RETURNING id INTO v_resource_id;
    END IF;
    
    -- Insert oscillation detection
    INSERT INTO oscillation_detections (
        pattern_id, resource_id, detected_at, confidence, action_count,
        time_span_minutes, pattern_evidence, prevention_applied,
        prevention_action
    ) VALUES (
        p_pattern_id, v_resource_id, NOW(), p_confidence, p_action_count,
        p_time_span_minutes, p_pattern_evidence, 
        p_prevention_action IS NOT NULL,
        p_prevention_action
    ) RETURNING id INTO v_detection_id;
    
    RETURN v_detection_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- =============================================================================
-- SECURITY AND PERFORMANCE OPTIMIZATIONS
-- =============================================================================

-- Create indexes for procedure performance
CREATE INDEX IF NOT EXISTS idx_rat_resource_action_time 
ON resource_action_traces (action_history_id, action_type, action_timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_rat_effectiveness_analysis
ON resource_action_traces (action_type, effectiveness_score, action_timestamp)
WHERE effectiveness_score IS NOT NULL;

-- Grant execute permissions (adjust as needed for your environment)
GRANT EXECUTE ON FUNCTION detect_scale_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION detect_resource_thrashing(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION detect_ineffective_loops(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION detect_cascading_failures(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION get_action_traces(VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE, INTEGER, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION get_action_effectiveness(VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE) TO slm_user;
GRANT EXECUTE ON FUNCTION store_oscillation_detection(INTEGER, VARCHAR, VARCHAR, VARCHAR, DECIMAL, INTEGER, INTEGER, JSONB, VARCHAR) TO slm_user;

-- Add helpful comments
COMMENT ON FUNCTION detect_scale_oscillation IS 'Detects scale oscillation patterns for a resource within a time window';
COMMENT ON FUNCTION detect_resource_thrashing IS 'Detects resource thrashing between scale and resource adjustment actions';
COMMENT ON FUNCTION detect_ineffective_loops IS 'Identifies repeated actions with low effectiveness scores';
COMMENT ON FUNCTION detect_cascading_failures IS 'Detects actions that trigger more alerts than they resolve';
COMMENT ON FUNCTION get_action_traces IS 'Retrieves filtered action history for a resource';
COMMENT ON FUNCTION get_action_effectiveness IS 'Calculates effectiveness metrics for actions on a resource';
COMMENT ON FUNCTION store_oscillation_detection IS 'Stores oscillation detection results with proper resource management';

-- =============================================================================
-- DETECTOR BASE PROCEDURES
-- =============================================================================

-- 8. Get Resource Actions Base
CREATE OR REPLACE FUNCTION get_resource_actions_base(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_window_minutes INTEGER DEFAULT NULL
)
RETURNS TABLE (
    trace_id BIGINT,
    action_timestamp TIMESTAMP WITH TIME ZONE,
    action_type VARCHAR(50),
    action_parameters JSONB,
    effectiveness_score DECIMAL(4,3),
    model_confidence DECIMAL(4,3),
    execution_status VARCHAR(20)
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        rat.id as trace_id,
        rat.action_timestamp,
        rat.action_type,
        rat.action_parameters,
        rat.effectiveness_score,
        rat.model_confidence,
        rat.execution_status
    FROM resource_action_traces rat
    JOIN action_histories ah ON rat.action_history_id = ah.id
    JOIN resource_references rr ON ah.resource_id = rr.id
    WHERE rr.namespace = p_namespace 
    AND rr.kind = p_kind 
    AND rr.name = p_name
    AND (p_window_minutes IS NULL OR rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes)
    ORDER BY rat.action_timestamp DESC;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 9. Get Resource ID
CREATE OR REPLACE FUNCTION get_resource_id(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253)
)
RETURNS INTEGER AS $$
DECLARE
    v_resource_id INTEGER;
BEGIN
    SELECT id INTO v_resource_id
    FROM resource_references 
    WHERE namespace = p_namespace AND kind = p_kind AND name = p_name;
    
    IF v_resource_id IS NULL THEN
        RAISE EXCEPTION 'Resource not found: namespace=%, kind=%, name=%', p_namespace, p_kind, p_name;
    END IF;
    
    RETURN v_resource_id;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- 10. Action Oscillation Analysis
CREATE OR REPLACE FUNCTION analyze_action_oscillation(
    p_namespace VARCHAR(63),
    p_kind VARCHAR(100),
    p_name VARCHAR(253),
    p_window_minutes INTEGER DEFAULT 120
)
RETURNS TABLE (
    action_timestamp TIMESTAMP WITH TIME ZONE,
    action_type VARCHAR(50),
    effectiveness_score DECIMAL(4,3),
    prev_timestamp TIMESTAMP WITH TIME ZONE,
    prev_action_type VARCHAR(50),
    time_gap_minutes DECIMAL(10,2),
    action_sequence_position INTEGER
) AS $$
BEGIN
    RETURN QUERY
    WITH action_analysis AS (
        SELECT 
            rat.action_timestamp,
            rat.action_type,
            rat.effectiveness_score,
            LAG(rat.action_timestamp) OVER (ORDER BY rat.action_timestamp) as prev_timestamp,
            LAG(rat.action_type) OVER (ORDER BY rat.action_timestamp) as prev_action_type,
            ROW_NUMBER() OVER (ORDER BY rat.action_timestamp) as sequence_position
        FROM resource_action_traces rat
        JOIN action_histories ah ON rat.action_history_id = ah.id
        JOIN resource_references rr ON ah.resource_id = rr.id
        WHERE rr.namespace = p_namespace 
        AND rr.kind = p_kind 
        AND rr.name = p_name
        AND rat.action_timestamp > NOW() - INTERVAL '1 minute' * p_window_minutes
    )
    SELECT 
        aa.action_timestamp,
        aa.action_type,
        aa.effectiveness_score,
        aa.prev_timestamp,
        aa.prev_action_type,
        CASE 
            WHEN aa.prev_timestamp IS NOT NULL THEN
                EXTRACT(EPOCH FROM (aa.action_timestamp - aa.prev_timestamp))/60
            ELSE 0
        END::DECIMAL(10,2) as time_gap_minutes,
        aa.sequence_position::INTEGER as action_sequence_position
    FROM action_analysis aa
    ORDER BY aa.action_timestamp;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

-- Grant execute permissions
GRANT EXECUTE ON FUNCTION get_resource_actions_base(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;
GRANT EXECUTE ON FUNCTION get_resource_id(VARCHAR, VARCHAR, VARCHAR) TO slm_user;
GRANT EXECUTE ON FUNCTION analyze_action_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER) TO slm_user;

-- Add helpful comments
COMMENT ON FUNCTION get_resource_actions_base IS 'Retrieves base resource action data with optional time window filtering';
COMMENT ON FUNCTION get_resource_id IS 'Gets the database ID for a resource reference with error handling';
COMMENT ON FUNCTION analyze_action_oscillation IS 'Analyzes action sequences for oscillation patterns with timing gaps';