-- Initial schema for Action History Storage
-- Migration: 001_initial_schema.sql

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Resource References Table
CREATE TABLE resource_references (
    id BIGSERIAL PRIMARY KEY,
    resource_uid VARCHAR(36) UNIQUE NOT NULL, -- Kubernetes UID
    api_version VARCHAR(100) NOT NULL,
    kind VARCHAR(100) NOT NULL,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(63),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE, -- For soft deletion tracking
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    UNIQUE(namespace, kind, name)
);

-- Indexes for resource_references
CREATE INDEX idx_resource_kind ON resource_references (kind);
CREATE INDEX idx_resource_namespace ON resource_references (namespace);
CREATE INDEX idx_resource_last_seen ON resource_references (last_seen);
CREATE INDEX idx_resource_uid ON resource_references (resource_uid);

-- 2. Action Histories Table
CREATE TABLE action_histories (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,

    -- Retention configuration
    max_actions INTEGER DEFAULT 1000,
    max_age_days INTEGER DEFAULT 30,
    compaction_strategy VARCHAR(20) DEFAULT 'pattern-aware', -- oldest-first, effectiveness-based, pattern-aware

    -- Analysis configuration
    oscillation_window_minutes INTEGER DEFAULT 120,
    effectiveness_threshold DECIMAL(3,2) DEFAULT 0.70,
    pattern_min_occurrences INTEGER DEFAULT 3,

    -- Status tracking
    total_actions INTEGER DEFAULT 0,
    last_action_at TIMESTAMP WITH TIME ZONE,
    last_analysis_at TIMESTAMP WITH TIME ZONE,
    next_analysis_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    UNIQUE(resource_id)
);

-- Indexes for action_histories
CREATE INDEX idx_ah_last_action ON action_histories (last_action_at);
CREATE INDEX idx_ah_next_analysis ON action_histories (next_analysis_at);
CREATE INDEX idx_ah_resource_id ON action_histories (resource_id);

-- 3. Resource Action Traces Table (will be partitioned)
CREATE TABLE resource_action_traces (
    id BIGSERIAL,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,

    -- Action identification
    action_id VARCHAR(64) NOT NULL, -- UUID for this specific action
    correlation_id VARCHAR(64), -- For tracing across systems

    -- Timing information
    action_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    execution_start_time TIMESTAMP WITH TIME ZONE,
    execution_end_time TIMESTAMP WITH TIME ZONE,
    execution_duration_ms INTEGER,

    -- Alert context
    alert_name VARCHAR(200) NOT NULL,
    alert_severity VARCHAR(20) NOT NULL, -- info, warning, critical
    alert_labels JSONB,
    alert_annotations JSONB,
    alert_firing_time TIMESTAMP WITH TIME ZONE,

    -- AI model information
    model_used VARCHAR(100) NOT NULL,
    routing_tier VARCHAR(20), -- route1, route2, route3
    model_confidence DECIMAL(4,3) NOT NULL, -- 0.000-1.000
    model_reasoning TEXT,
    alternative_actions JSONB, -- [{"action": "scale_deployment", "confidence": 0.85}]

    -- Action details
    action_type VARCHAR(50) NOT NULL,
    action_parameters JSONB, -- {"replicas": 5, "memory": "2Gi"}

    -- Resource state capture
    resource_state_before JSONB,
    resource_state_after JSONB,

    -- Execution tracking
    execution_status VARCHAR(20) DEFAULT 'pending', -- pending, executing, completed, failed, rolled-back
    execution_error TEXT,
    kubernetes_operations JSONB, -- [{"operation": "patch", "resource": "deployment/webapp", "result": "success"}]

    -- Effectiveness assessment
    effectiveness_score DECIMAL(4,3), -- 0.000-1.000, calculated after execution
    effectiveness_criteria JSONB, -- {"alert_resolved": true, "target_metric_improved": true}
    effectiveness_assessed_at TIMESTAMP WITH TIME ZONE,
    effectiveness_assessment_method VARCHAR(20), -- automated, manual, ml-derived
    effectiveness_notes TEXT,

    -- Follow-up tracking
    follow_up_actions JSONB, -- [{"action_id": "uuid", "relation": "correction"}]

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Primary key includes timestamp for partitioning
    PRIMARY KEY (id, action_timestamp)
) PARTITION BY RANGE (action_timestamp);

-- Indexes for resource_action_traces (will be inherited by partitions)
CREATE INDEX idx_rat_action_history ON resource_action_traces (action_history_id);
CREATE INDEX idx_rat_action_type ON resource_action_traces (action_type);
CREATE INDEX idx_rat_model_used ON resource_action_traces (model_used);
CREATE INDEX idx_rat_alert_name ON resource_action_traces (alert_name);
CREATE INDEX idx_rat_execution_status ON resource_action_traces (execution_status);
CREATE INDEX idx_rat_effectiveness_score ON resource_action_traces (effectiveness_score);
CREATE INDEX idx_rat_correlation_id ON resource_action_traces (correlation_id);

-- Composite indexes for common queries
CREATE INDEX idx_rat_history_timestamp ON resource_action_traces (action_history_id, action_timestamp);
CREATE INDEX idx_rat_type_timestamp ON resource_action_traces (action_type, action_timestamp);
CREATE INDEX idx_rat_model_effectiveness ON resource_action_traces (model_used, effectiveness_score);

-- GIN indexes for JSONB queries
CREATE INDEX idx_rat_alert_labels_gin ON resource_action_traces USING GIN (alert_labels);
CREATE INDEX idx_rat_action_parameters_gin ON resource_action_traces USING GIN (action_parameters);
CREATE INDEX idx_rat_resource_state_gin ON resource_action_traces USING GIN (resource_state_before);

-- Partial indexes for active data
CREATE INDEX idx_rat_pending_actions ON resource_action_traces (action_timestamp)
    WHERE execution_status IN ('pending', 'executing');

-- 4. Create initial partitions for resource_action_traces
-- Extended range: July 2025 - February 2026 (covers development period)
CREATE TABLE resource_action_traces_2025_07
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');

CREATE TABLE resource_action_traces_2025_08
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');

CREATE TABLE resource_action_traces_2025_09
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');

CREATE TABLE resource_action_traces_2025_10
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE TABLE resource_action_traces_2025_11
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE resource_action_traces_2025_12
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

CREATE TABLE resource_action_traces_2026_01
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE resource_action_traces_2026_02
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

-- 5. Oscillation Patterns Table
CREATE TABLE oscillation_patterns (
    id BIGSERIAL PRIMARY KEY,

    -- Pattern definition
    pattern_type VARCHAR(50) NOT NULL, -- scale-oscillation, resource-thrashing, ineffective-loop, cascading-failure
    pattern_name VARCHAR(200) NOT NULL,
    description TEXT,

    -- Detection criteria
    min_occurrences INTEGER NOT NULL DEFAULT 3,
    time_window_minutes INTEGER NOT NULL DEFAULT 120,
    action_sequence JSONB, -- ["scale_deployment", "scale_deployment", "scale_deployment"]
    threshold_config JSONB, -- {"confidence_drop": 0.2, "effectiveness_threshold": 0.3}

    -- Resource scope
    resource_types TEXT[], -- ["Deployment", "StatefulSet"]
    namespaces TEXT[], -- ["production", "staging"]
    label_selectors JSONB, -- {"app": "webapp", "tier": "frontend"}

    -- Prevention strategy
    prevention_strategy VARCHAR(50) NOT NULL, -- block-action, escalate-human, alternative-action, cooling-period
    prevention_parameters JSONB, -- {"cooling_period_minutes": 30, "escalation_webhook": "..."}

    -- Alerting configuration
    alerting_enabled BOOLEAN DEFAULT true,
    alert_severity VARCHAR(20) DEFAULT 'warning',
    alert_channels TEXT[], -- ["slack", "pagerduty"]

    -- Pattern statistics
    total_detections INTEGER DEFAULT 0,
    prevention_success_rate DECIMAL(4,3),
    false_positive_rate DECIMAL(4,3),
    last_detection_at TIMESTAMP WITH TIME ZONE,

    -- Lifecycle
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for oscillation_patterns
CREATE INDEX idx_op_pattern_type ON oscillation_patterns (pattern_type);
CREATE INDEX idx_op_active_patterns ON oscillation_patterns (active);
CREATE INDEX idx_op_last_detection ON oscillation_patterns (last_detection_at);

-- 6. Oscillation Detections Table
CREATE TABLE oscillation_detections (
    id BIGSERIAL PRIMARY KEY,
    pattern_id BIGINT NOT NULL REFERENCES oscillation_patterns(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,

    detected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    confidence DECIMAL(4,3) NOT NULL, -- 0.000-1.000
    action_count INTEGER NOT NULL,
    time_span_minutes INTEGER NOT NULL,

    -- Pattern evidence
    matching_actions BIGINT[], -- Array of action_trace IDs that matched the pattern
    pattern_evidence JSONB, -- Detailed evidence for the detection

    -- Prevention outcome
    prevention_applied BOOLEAN DEFAULT false,
    prevention_action VARCHAR(50), -- blocked, escalated, alternative-suggested
    prevention_details JSONB,
    prevention_successful BOOLEAN,

    -- Resolution tracking
    resolved BOOLEAN DEFAULT false,
    resolved_at TIMESTAMP WITH TIME ZONE,
    resolution_method VARCHAR(50), -- timeout, manual-intervention, automatic
    resolution_notes TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for oscillation_detections
CREATE INDEX idx_od_pattern_resource ON oscillation_detections (pattern_id, resource_id);
CREATE INDEX idx_od_detected_at ON oscillation_detections (detected_at);
CREATE INDEX idx_od_unresolved ON oscillation_detections (resolved) WHERE resolved = false;

-- 7. Action Effectiveness Metrics Table
CREATE TABLE action_effectiveness_metrics (
    id BIGSERIAL PRIMARY KEY,

    -- Scope definition
    scope_type VARCHAR(50) NOT NULL, -- global, namespace, resource-type, alert-type, model
    scope_value VARCHAR(200), -- specific value for the scope
    metric_period VARCHAR(20) NOT NULL, -- 1h, 24h, 7d, 30d

    -- Time range for this metric
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Effectiveness by action type
    action_type VARCHAR(50) NOT NULL,
    sample_size INTEGER NOT NULL,
    average_score DECIMAL(4,3) NOT NULL,
    median_score DECIMAL(4,3),
    std_deviation DECIMAL(4,3),
    confidence_interval_lower DECIMAL(4,3),
    confidence_interval_upper DECIMAL(4,3),

    -- Trend analysis
    trend_direction VARCHAR(20), -- improving, stable, declining
    trend_confidence DECIMAL(4,3),

    -- Statistical significance
    min_sample_size_met BOOLEAN,
    statistical_significance DECIMAL(4,3),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ensure uniqueness and enable efficient queries
    UNIQUE(scope_type, scope_value, metric_period, period_start, action_type)
);

-- Indexes for action_effectiveness_metrics
CREATE INDEX idx_aem_scope_period ON action_effectiveness_metrics (scope_type, scope_value, metric_period);
CREATE INDEX idx_aem_period_range ON action_effectiveness_metrics (period_start, period_end);
CREATE INDEX idx_aem_action_effectiveness ON action_effectiveness_metrics (action_type, average_score);

-- 8. Retention Operations Table
CREATE TABLE retention_operations (
    id BIGSERIAL PRIMARY KEY,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,

    operation_type VARCHAR(30) NOT NULL, -- cleanup, archive, compact
    strategy_used VARCHAR(30) NOT NULL, -- oldest-first, effectiveness-based, pattern-aware

    -- Operation details
    records_before INTEGER NOT NULL,
    records_after INTEGER NOT NULL,
    records_deleted INTEGER NOT NULL,
    records_archived INTEGER,

    -- Criteria used
    retention_criteria JSONB, -- {"max_age_days": 30, "min_effectiveness": 0.1}
    preserved_criteria JSONB, -- {"pattern_examples": 5, "high_effectiveness": 10}

    operation_start TIMESTAMP WITH TIME ZONE NOT NULL,
    operation_end TIMESTAMP WITH TIME ZONE,
    operation_duration_ms INTEGER,
    operation_status VARCHAR(20) DEFAULT 'running', -- running, completed, failed
    error_message TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for retention_operations
CREATE INDEX idx_ro_action_history_ops ON retention_operations (action_history_id);
CREATE INDEX idx_ro_operation_time ON retention_operations (operation_start);

-- 9. Insert default oscillation patterns
INSERT INTO oscillation_patterns (
    pattern_type, pattern_name, description, min_occurrences, time_window_minutes,
    threshold_config, prevention_strategy, prevention_parameters
) VALUES
(
    'scale-oscillation',
    'Scale Up/Down Oscillation',
    'Rapid alternating scale up and scale down operations within a short time window',
    3,
    120,
    '{"min_direction_changes": 2, "max_time_between_actions": 30, "effectiveness_threshold": 0.5}',
    'cooling-period',
    '{"cooling_period_minutes": 30, "escalate_after": 3}'
),
(
    'resource-thrashing',
    'Resource/Scale Thrashing',
    'Alternating between resource adjustments and scaling decisions',
    2,
    90,
    '{"action_types": ["increase_resources", "scale_deployment"], "effectiveness_threshold": 0.6}',
    'alternative-action',
    '{"suggest_alternatives": true, "block_conflicting": true}'
),
(
    'ineffective-loop',
    'Ineffective Action Loop',
    'Repeated actions with consistently low effectiveness scores',
    4,
    180,
    '{"effectiveness_threshold": 0.3, "min_repetitions": 3}',
    'escalate-human',
    '{"escalation_webhook": null, "require_approval": true}'
),
(
    'cascading-failure',
    'Cascading Failure Pattern',
    'Actions that trigger more alerts than they resolve',
    2,
    60,
    '{"new_alerts_threshold": 1.5, "recurrence_rate_threshold": 0.4}',
    'block-action',
    '{"block_duration_minutes": 60, "require_manual_override": true}'
);

-- 10. Create function for automatic partition creation
CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS void AS $$
DECLARE
    start_date date;
    end_date date;
    table_name text;
BEGIN
    -- Create partition for next month
    start_date := date_trunc('month', CURRENT_DATE + interval '1 month');
    end_date := start_date + interval '1 month';
    table_name := 'resource_action_traces_' ||
                  to_char(start_date, 'YYYY') || '_' ||
                  to_char(start_date, 'MM');

    EXECUTE format('CREATE TABLE IF NOT EXISTS %I
                   PARTITION OF resource_action_traces
                   FOR VALUES FROM (%L) TO (%L)',
                   table_name, start_date, end_date);

    RAISE NOTICE 'Created partition: %', table_name;
END;
$$ LANGUAGE plpgsql;

-- 11. Create trigger function to update updated_at columns
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers for updated_at
CREATE TRIGGER update_action_histories_updated_at
    BEFORE UPDATE ON action_histories
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_resource_action_traces_updated_at
    BEFORE UPDATE ON resource_action_traces
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER update_oscillation_patterns_updated_at
    BEFORE UPDATE ON oscillation_patterns
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- 12. Create views for common queries
CREATE VIEW action_history_summary AS
SELECT
    rr.namespace,
    rr.kind,
    rr.name,
    ah.total_actions,
    ah.last_action_at,
    COUNT(rat.id) as recent_actions_24h,
    AVG(rat.effectiveness_score) as avg_effectiveness_24h,
    COUNT(DISTINCT rat.action_type) as action_types_used
FROM resource_references rr
JOIN action_histories ah ON ah.resource_id = rr.id
LEFT JOIN resource_action_traces rat ON rat.action_history_id = ah.id
    AND rat.action_timestamp > NOW() - INTERVAL '24 hours'
GROUP BY rr.id, rr.namespace, rr.kind, rr.name, ah.total_actions, ah.last_action_at;

-- Summary statistics
CREATE VIEW oscillation_detection_summary AS
SELECT
    pattern_type,
    COUNT(*) as total_detections,
    COUNT(*) FILTER (WHERE prevention_applied = true) as preventions_applied,
    COUNT(*) FILTER (WHERE prevention_successful = true) as successful_preventions,
    AVG(confidence) as avg_confidence,
    MAX(detected_at) as last_detection
FROM oscillation_detections od
JOIN oscillation_patterns op ON od.pattern_id = op.id
GROUP BY pattern_type;

-- Grant permissions (for development)
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;