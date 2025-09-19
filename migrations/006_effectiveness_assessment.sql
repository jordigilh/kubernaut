-- Migration: Add AI Effectiveness Assessment Tables
-- Created: Phase 1 Implementation - Business Logic Enhancement
-- Purpose: Enable real AI learning from action outcomes

-- Table for pending action assessments
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

-- Indexes for action_assessments
CREATE INDEX IF NOT EXISTS idx_action_assessments_status_scheduled ON action_assessments(status, scheduled_for) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_action_assessments_trace_id ON action_assessments(trace_id);
CREATE INDEX IF NOT EXISTS idx_action_assessments_context ON action_assessments(action_type, context_hash);

-- Table for storing effectiveness assessment results
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

-- Indexes for effectiveness_results
CREATE INDEX IF NOT EXISTS idx_effectiveness_results_action_type ON effectiveness_results(action_type);
CREATE INDEX IF NOT EXISTS idx_effectiveness_results_assessed_at ON effectiveness_results(assessed_at);
CREATE INDEX IF NOT EXISTS idx_effectiveness_results_score ON effectiveness_results(overall_score);

-- Table for action confidence scores (core learning mechanism)
CREATE TABLE IF NOT EXISTS action_confidence_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL, -- Hash of alert context for grouping similar scenarios
    base_confidence FLOAT NOT NULL CHECK (base_confidence >= 0 AND base_confidence <= 1),
    adjusted_confidence FLOAT NOT NULL CHECK (adjusted_confidence >= 0 AND adjusted_confidence <= 1),
    adjustment_reason TEXT,
    effectiveness_samples INTEGER DEFAULT 0,
    last_updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(action_type, context_hash)
);

-- Indexes for action_confidence_scores
CREATE INDEX IF NOT EXISTS idx_action_confidence_context ON action_confidence_scores(action_type, context_hash);
CREATE INDEX IF NOT EXISTS idx_action_confidence_updated ON action_confidence_scores(last_updated);

-- Table for action outcome history (for learning algorithms)
CREATE TABLE IF NOT EXISTS action_outcomes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    success BOOLEAN NOT NULL,
    alert_resolved BOOLEAN NOT NULL,
    side_effects INTEGER DEFAULT 0,
    effectiveness_score FLOAT NOT NULL CHECK (effectiveness_score >= 0 AND effectiveness_score <= 1),
    execution_time BIGINT, -- Duration in nanoseconds
    metrics_before JSONB,
    metrics_after JSONB,
    failure_reason TEXT,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    assessed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for action_outcomes
CREATE INDEX IF NOT EXISTS idx_action_outcomes_context ON action_outcomes(action_type, context_hash);
CREATE INDEX IF NOT EXISTS idx_action_outcomes_executed_at ON action_outcomes(executed_at);
CREATE INDEX IF NOT EXISTS idx_action_outcomes_success ON action_outcomes(success);
CREATE INDEX IF NOT EXISTS idx_action_outcomes_effectiveness ON action_outcomes(effectiveness_score);

-- Table for tracking alternative action recommendations
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

-- Indexes for action_alternatives
CREATE INDEX IF NOT EXISTS idx_action_alternatives_failed ON action_alternatives(failed_action_type, context_hash);
CREATE INDEX IF NOT EXISTS idx_action_alternatives_success_rate ON action_alternatives(success_rate DESC);

-- View for easy querying of effectiveness trends
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

-- View for low-confidence actions requiring attention
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

-- Function to automatically create assessment when action trace is created
CREATE OR REPLACE FUNCTION create_assessment_for_action_trace()
RETURNS TRIGGER AS $$
BEGIN
    -- Only create assessment for completed actions
    IF NEW.execution_status = 'completed' THEN
        INSERT INTO action_assessments (
            trace_id,
            action_type,
            context_hash,
            alert_name,
            namespace,
            resource_name,
            executed_at,
            scheduled_for
        ) VALUES (
            NEW.id::VARCHAR,
            NEW.action_type,
            -- Simple context hash based on action type + alert
            encode(sha256(CONCAT(NEW.action_type, ':', COALESCE(NEW.alert_name, 'no-alert'))::bytea), 'hex'),
            COALESCE(NEW.alert_name, 'no-alert'),
            'unknown', -- Default namespace until we can join with resource_references
            'unknown', -- Default resource name until we can join with resource_references
            COALESCE(NEW.execution_end_time, NEW.action_timestamp),
            NOW() + INTERVAL '5 minutes' -- Schedule assessment 5 minutes after execution
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically create assessments for new action traces
-- Note: This assumes resource_action_traces table exists from previous migrations
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'resource_action_traces') THEN
        DROP TRIGGER IF EXISTS trigger_create_assessment_for_action_trace ON resource_action_traces;
        CREATE TRIGGER trigger_create_assessment_for_action_trace
            AFTER UPDATE ON resource_action_traces
            FOR EACH ROW
            EXECUTE FUNCTION create_assessment_for_action_trace();
    END IF;
END $$;

-- Insert some default confidence scores for common action types
INSERT INTO action_confidence_scores (action_type, context_hash, base_confidence, adjusted_confidence, adjustment_reason)
VALUES
    ('restart_pod', 'default', 0.7, 0.7, 'Default confidence for pod restarts'),
    ('scale_deployment', 'default', 0.75, 0.75, 'Default confidence for deployment scaling'),
    ('delete_pod', 'default', 0.6, 0.6, 'Default confidence for pod deletion'),
    ('rollback_deployment', 'default', 0.8, 0.8, 'Default confidence for deployment rollback')
ON CONFLICT (action_type, context_hash) DO NOTHING;

-- Add comments for documentation
COMMENT ON TABLE action_assessments IS 'Pending effectiveness assessments for completed actions';
COMMENT ON TABLE effectiveness_results IS 'Results of AI effectiveness assessments for learning';
COMMENT ON TABLE action_confidence_scores IS 'Dynamic confidence scores that improve through learning';
COMMENT ON TABLE action_outcomes IS 'Historical outcomes for training ML algorithms';
COMMENT ON TABLE action_alternatives IS 'Alternative actions for failed patterns';
COMMENT ON VIEW effectiveness_trends IS 'Daily trends in action effectiveness for monitoring';
COMMENT ON VIEW low_confidence_actions IS 'Actions requiring attention due to poor performance';

-- Create indexes for performance
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_action_outcomes_learning_query
    ON action_outcomes(action_type, context_hash, executed_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_effectiveness_results_learning_query
    ON effectiveness_results(action_type, assessed_at DESC);

COMMIT;
