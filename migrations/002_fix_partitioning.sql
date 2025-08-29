-- Fix partitioning issues in resource_action_traces table
-- Migration: 002_fix_partitioning.sql

-- Drop the problematic partitioned table and recreate it properly
DROP TABLE IF EXISTS resource_action_traces CASCADE;

-- 3. Resource Action Traces Table (partitioned by timestamp)
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
    PRIMARY KEY (id, action_timestamp),
    -- Unique constraint must include partition key
    UNIQUE (action_id, action_timestamp)
) PARTITION BY RANGE (action_timestamp);

-- Create initial partitions for resource_action_traces
-- Previous month
CREATE TABLE resource_action_traces_y2025m07 
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');

-- Current month
CREATE TABLE resource_action_traces_y2025m08 
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');

-- Next month
CREATE TABLE resource_action_traces_y2025m09 
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');

-- Future month (for testing edge cases)
CREATE TABLE resource_action_traces_y2025m10 
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

-- Create indexes on the partitioned table (will be inherited by partitions)
CREATE INDEX idx_rat_action_history ON resource_action_traces (action_history_id, action_timestamp);
CREATE INDEX idx_rat_action_type ON resource_action_traces (action_type, action_timestamp);
CREATE INDEX idx_rat_model_used ON resource_action_traces (model_used, action_timestamp);
CREATE INDEX idx_rat_alert_name ON resource_action_traces (alert_name, action_timestamp);
CREATE INDEX idx_rat_execution_status ON resource_action_traces (execution_status) WHERE execution_status IN ('pending', 'executing');
CREATE INDEX idx_rat_effectiveness_score ON resource_action_traces (effectiveness_score) WHERE effectiveness_score IS NOT NULL;
CREATE INDEX idx_rat_correlation_id ON resource_action_traces (correlation_id) WHERE correlation_id IS NOT NULL;

-- GIN indexes for JSONB queries
CREATE INDEX idx_rat_alert_labels_gin ON resource_action_traces USING GIN (alert_labels);
CREATE INDEX idx_rat_action_parameters_gin ON resource_action_traces USING GIN (action_parameters);
CREATE INDEX idx_rat_resource_state_gin ON resource_action_traces USING GIN (resource_state_before);

-- Recreate the views that depend on resource_action_traces
DROP VIEW IF EXISTS action_history_summary;
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

-- Create trigger for updated_at on the new table
CREATE TRIGGER update_resource_action_traces_updated_at
    BEFORE UPDATE ON resource_action_traces
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();