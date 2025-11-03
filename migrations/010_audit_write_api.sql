-- Migration 010: Audit Write API - 6 Audit Tables for Data Storage Service
-- Created: 2025-11-02
-- Purpose: Implement audit trail persistence for ADR-032 v1.1 Data Access Layer Isolation
-- Authority: docs/architecture/decisions/ADR-032-data-access-layer-isolation.md
-- Related: IMPLEMENTATION_PLAN_V4.7.md Phase 0 Day 0.1

-- Enable pgvector extension (idempotent)
CREATE EXTENSION IF NOT EXISTS vector;

-- ============================================================================
-- Table 1: orchestration_audit
-- Service: RemediationOrchestrator (Remediation Orchestrator)
-- Endpoint: POST /api/v1/audit/orchestration
-- Schema Authority: docs/services/crd-controllers/05-remediationorchestrator/database-integration.md
-- ============================================================================

CREATE TABLE IF NOT EXISTS orchestration_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    alert_fingerprint VARCHAR(64) NOT NULL,
    remediation_name VARCHAR(255) NOT NULL UNIQUE,
    
    -- Phase tracking
    overall_phase VARCHAR(50) NOT NULL CHECK (overall_phase IN ('pending', 'processing', 'analyzing', 'executing', 'completed', 'failed', 'timeout')),
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    completion_time TIMESTAMP WITH TIME ZONE,
    
    -- Service CRD references
    remediation_processing_name VARCHAR(255),
    ai_analysis_name VARCHAR(255),
    workflow_execution_name VARCHAR(255),
    
    -- Service CRD statuses (JSONB for flexibility)
    service_crd_statuses JSONB,
    
    -- Timeout/Failure tracking
    timeout_phase VARCHAR(50),
    failure_phase VARCHAR(50),
    failure_reason TEXT,
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for orchestration_audit
CREATE INDEX IF NOT EXISTS idx_orchestration_audit_fingerprint ON orchestration_audit(alert_fingerprint);
CREATE INDEX IF NOT EXISTS idx_orchestration_audit_name ON orchestration_audit(remediation_name);
CREATE INDEX IF NOT EXISTS idx_orchestration_audit_phase ON orchestration_audit(overall_phase);
CREATE INDEX IF NOT EXISTS idx_orchestration_audit_created_at ON orchestration_audit(created_at DESC);

-- ============================================================================
-- Table 2: signal_processing_audit
-- Service: RemediationProcessor (Remediation Processor)
-- Endpoint: POST /api/v1/audit/signal-processing
-- Schema Authority: docs/services/crd-controllers/01-remediationprocessor/database-integration.md
-- Note: Renamed from "alert_processing_audit" to align with generic "signal" terminology
-- ============================================================================

CREATE TABLE IF NOT EXISTS signal_processing_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    remediation_id VARCHAR(255) NOT NULL,
    alert_fingerprint VARCHAR(64) NOT NULL,
    
    -- Processing phases (JSONB for structured phase data)
    processing_phases JSONB NOT NULL,
    
    -- Enrichment results
    enrichment_quality FLOAT CHECK (enrichment_quality BETWEEN 0 AND 1),
    enrichment_sources TEXT[],
    context_size_bytes INTEGER,
    
    -- Classification results
    environment VARCHAR(50) CHECK (environment IN ('prod', 'production', 'staging', 'dev', 'development', 'test')),
    confidence FLOAT CHECK (confidence BETWEEN 0 AND 1),
    business_priority VARCHAR(10) CHECK (business_priority IN ('P0', 'P1', 'P2', 'P3')),
    sla_requirement VARCHAR(50),
    
    -- Routing decision
    routed_to_service VARCHAR(100),
    routing_priority INTEGER,
    
    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL CHECK (status IN ('completed', 'failed', 'degraded')),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for signal_processing_audit
CREATE INDEX IF NOT EXISTS idx_signal_processing_audit_remediation_id ON signal_processing_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_signal_processing_audit_fingerprint ON signal_processing_audit(alert_fingerprint);
CREATE INDEX IF NOT EXISTS idx_signal_processing_audit_environment ON signal_processing_audit(environment);
CREATE INDEX IF NOT EXISTS idx_signal_processing_audit_completed_at ON signal_processing_audit(completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_signal_processing_audit_status ON signal_processing_audit(status);

-- ============================================================================
-- Table 3: ai_analysis_audit
-- Service: AIAnalysis Controller
-- Endpoint: POST /api/v1/audit/ai-decisions
-- Schema Authority: docs/services/crd-controllers/02-aianalysis/database-integration.md
-- Decision 1a: ONLY audit type with pgvector embeddings (1536 dimensions)
-- ============================================================================

CREATE TABLE IF NOT EXISTS ai_analysis_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    crd_name VARCHAR(255) NOT NULL,
    crd_namespace VARCHAR(255) NOT NULL,
    alert_fingerprint VARCHAR(64) NOT NULL,
    environment VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    
    -- Investigation phase
    investigation_start TIMESTAMP WITH TIME ZONE,
    investigation_end TIMESTAMP WITH TIME ZONE,
    investigation_duration_ms INTEGER,
    root_cause_count INTEGER,
    investigation_report TEXT,
    
    -- Analysis phase
    analysis_start TIMESTAMP WITH TIME ZONE,
    analysis_end TIMESTAMP WITH TIME ZONE,
    analysis_duration_ms INTEGER,
    confidence_score DECIMAL(3,2) CHECK (confidence_score BETWEEN 0 AND 1),
    hallucination_detected BOOLEAN DEFAULT FALSE,
    
    -- Recommendation phase
    recommendation_start TIMESTAMP WITH TIME ZONE,
    recommendation_end TIMESTAMP WITH TIME ZONE,
    recommendations JSONB,
    top_recommendation VARCHAR(255),
    effectiveness_probability DECIMAL(3,2) CHECK (effectiveness_probability BETWEEN 0 AND 1),
    historical_success_rate DECIMAL(3,2),
    
    -- Workflow tracking
    workflow_crd_name VARCHAR(255),
    workflow_crd_namespace VARCHAR(255),
    
    -- Semantic search embedding (Decision 1a: AIAnalysis only)
    -- V2.0 RAR generation uses semantic search over AI investigations
    embedding vector(1536),  -- OpenAI text-embedding-3-small or equivalent
    
    -- Metadata
    completion_status VARCHAR(50) CHECK (completion_status IN ('completed', 'failed', 'timeout')),
    failure_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Unique constraint on CRD identity
    UNIQUE (crd_name, crd_namespace)
);

-- Indexes for ai_analysis_audit
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_fingerprint ON ai_analysis_audit(alert_fingerprint);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_environment ON ai_analysis_audit(environment);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_created_at ON ai_analysis_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_completion_status ON ai_analysis_audit(completion_status);

-- pgvector HNSW index for semantic similarity search (V2.0 RAR generation)
-- HNSW provides better performance than IVFFlat for < 1M vectors
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_embedding ON ai_analysis_audit 
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- ============================================================================
-- Table 4: workflow_execution_audit
-- Service: WorkflowExecution Controller
-- Endpoint: POST /api/v1/audit/executions
-- Schema Authority: docs/services/crd-controllers/03-workflowexecution/database-integration.md
-- ============================================================================

CREATE TABLE IF NOT EXISTS workflow_execution_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    remediation_id VARCHAR(255) NOT NULL,
    workflow_name VARCHAR(255) NOT NULL,
    workflow_version VARCHAR(50) NOT NULL,
    
    -- Execution metrics
    total_steps INTEGER NOT NULL,
    steps_completed INTEGER DEFAULT 0,
    steps_failed INTEGER DEFAULT 0,
    total_duration_ms INTEGER,
    
    -- Outcome
    outcome VARCHAR(50) NOT NULL CHECK (outcome IN ('success', 'failed', 'partial', 'timeout')),
    effectiveness_score FLOAT CHECK (effectiveness_score BETWEEN 0 AND 1),
    rollbacks_performed INTEGER DEFAULT 0,
    
    -- Learning data (JSONB for flexibility)
    step_executions JSONB,
    adaptive_adjustments JSONB,
    
    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'timeout')),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for workflow_execution_audit
CREATE INDEX IF NOT EXISTS idx_workflow_execution_audit_remediation_id ON workflow_execution_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_workflow_execution_audit_workflow_name ON workflow_execution_audit(workflow_name);
CREATE INDEX IF NOT EXISTS idx_workflow_execution_audit_outcome ON workflow_execution_audit(outcome);
CREATE INDEX IF NOT EXISTS idx_workflow_execution_audit_created_at ON workflow_execution_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_execution_audit_status ON workflow_execution_audit(status);

-- ============================================================================
-- Table 5: notification_audit
-- Service: Notification Controller
-- Endpoint: POST /api/v1/audit/notifications
-- Schema: Designed for Phase 0 Day 0.1 (GAP #1 resolution)
-- Purpose: Track notification delivery status, retries, and channel usage
-- ============================================================================

CREATE TABLE IF NOT EXISTS notification_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    notification_id VARCHAR(255) NOT NULL UNIQUE,
    remediation_id VARCHAR(255) NOT NULL,
    
    -- Notification details
    channel VARCHAR(50) NOT NULL CHECK (channel IN ('slack', 'pagerduty', 'email', 'webhook', 'teams')),
    recipient_count INTEGER NOT NULL DEFAULT 0,
    recipients JSONB,  -- Array of recipient identifiers
    
    -- Message content
    message_template VARCHAR(255),
    message_priority VARCHAR(10) CHECK (message_priority IN ('P0', 'P1', 'P2', 'P3')),
    notification_type VARCHAR(50) CHECK (notification_type IN ('alert', 'escalation', 'resolution', 'status_update')),
    
    -- Delivery tracking
    status VARCHAR(50) NOT NULL CHECK (status IN ('pending', 'sent', 'delivered', 'failed', 'retrying')),
    delivery_time TIMESTAMP WITH TIME ZONE,
    delivery_duration_ms INTEGER,
    
    -- Retry tracking
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    last_retry_time TIMESTAMP WITH TIME ZONE,
    
    -- Failure tracking
    error_message TEXT,
    error_code VARCHAR(50),
    
    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for notification_audit
CREATE INDEX IF NOT EXISTS idx_notification_audit_notification_id ON notification_audit(notification_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_remediation_id ON notification_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_notification_audit_channel ON notification_audit(channel);
CREATE INDEX IF NOT EXISTS idx_notification_audit_status ON notification_audit(status);
CREATE INDEX IF NOT EXISTS idx_notification_audit_created_at ON notification_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_audit_delivery_time ON notification_audit(delivery_time DESC) WHERE delivery_time IS NOT NULL;

-- ============================================================================
-- Table 6: effectiveness_audit
-- Service: Effectiveness Monitor
-- Endpoint: POST /api/v1/audit/effectiveness
-- Schema: Based on Effectiveness Monitor integration-points.md (GAP #8)
-- Purpose: Track effectiveness assessment results for remediation actions
-- ============================================================================

CREATE TABLE IF NOT EXISTS effectiveness_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    assessment_id VARCHAR(255) NOT NULL UNIQUE,
    remediation_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    
    -- Assessment results
    traditional_score FLOAT NOT NULL CHECK (traditional_score BETWEEN 0 AND 1),
    environmental_impact FLOAT CHECK (environmental_impact BETWEEN -1 AND 1),  -- Negative = adverse effect
    confidence FLOAT NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    
    -- Trend analysis
    trend_direction VARCHAR(20) CHECK (trend_direction IN ('improving', 'declining', 'stable', 'insufficient_data')),
    recent_success_rate FLOAT CHECK (recent_success_rate BETWEEN 0 AND 1),
    historical_success_rate FLOAT CHECK (historical_success_rate BETWEEN 0 AND 1),
    
    -- Data quality
    data_quality VARCHAR(20) CHECK (data_quality IN ('sufficient', 'limited', 'insufficient')),
    sample_size INTEGER,
    data_age_days INTEGER,
    
    -- Pattern recognition
    pattern_detected BOOLEAN DEFAULT FALSE,
    pattern_description TEXT,
    temporal_pattern VARCHAR(50),  -- time_of_day, day_of_week, etc.
    
    -- Side effects
    side_effects_detected BOOLEAN DEFAULT FALSE,
    side_effects_description TEXT,
    
    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for effectiveness_audit
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_assessment_id ON effectiveness_audit(assessment_id);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_remediation_id ON effectiveness_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_action_type ON effectiveness_audit(action_type);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_completed_at ON effectiveness_audit(completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_trend_direction ON effectiveness_audit(trend_direction);

-- ============================================================================
-- Partitioning Strategy (Optional - Enable for production scale)
-- Time-based monthly partitions for all audit tables
-- NOTE: Partitioning is commented out for initial implementation
-- Uncomment when audit data volume reaches >10M rows per table
-- ============================================================================

-- Example partitioning (orchestration_audit):
-- 
-- -- Convert existing table to partitioned table
-- ALTER TABLE orchestration_audit RENAME TO orchestration_audit_old;
-- 
-- CREATE TABLE orchestration_audit (
--     LIKE orchestration_audit_old INCLUDING ALL
-- ) PARTITION BY RANGE (created_at);
-- 
-- -- Create monthly partitions
-- CREATE TABLE orchestration_audit_2025_11 PARTITION OF orchestration_audit
--     FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
-- CREATE TABLE orchestration_audit_2025_12 PARTITION OF orchestration_audit
--     FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
-- 
-- -- Migrate data
-- INSERT INTO orchestration_audit SELECT * FROM orchestration_audit_old;
-- DROP TABLE orchestration_audit_old;

-- ============================================================================
-- Triggers for updated_at timestamp management
-- ============================================================================

CREATE OR REPLACE FUNCTION update_audit_updated_at_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to all 6 audit tables
CREATE TRIGGER trigger_orchestration_audit_updated_at
    BEFORE UPDATE ON orchestration_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

CREATE TRIGGER trigger_signal_processing_audit_updated_at
    BEFORE UPDATE ON signal_processing_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

CREATE TRIGGER trigger_ai_analysis_audit_updated_at
    BEFORE UPDATE ON ai_analysis_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

CREATE TRIGGER trigger_workflow_execution_audit_updated_at
    BEFORE UPDATE ON workflow_execution_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

CREATE TRIGGER trigger_notification_audit_updated_at
    BEFORE UPDATE ON notification_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

CREATE TRIGGER trigger_effectiveness_audit_updated_at
    BEFORE UPDATE ON effectiveness_audit
    FOR EACH ROW
    EXECUTE FUNCTION update_audit_updated_at_timestamp();

-- ============================================================================
-- Retention Policy (Compliance Requirement: 7+ years per ADR-032)
-- ============================================================================

-- NOTE: Automatic retention enforcement deferred to V1.1
-- Manual cleanup query (DO NOT RUN in production without approval):
-- 
-- DELETE FROM orchestration_audit WHERE created_at < NOW() - INTERVAL '7 years';
-- DELETE FROM signal_processing_audit WHERE created_at < NOW() - INTERVAL '7 years';
-- DELETE FROM ai_analysis_audit WHERE created_at < NOW() - INTERVAL '7 years';
-- DELETE FROM workflow_execution_audit WHERE created_at < NOW() - INTERVAL '7 years';
-- DELETE FROM notification_audit WHERE created_at < NOW() - INTERVAL '7 years';
-- DELETE FROM effectiveness_audit WHERE created_at < NOW() - INTERVAL '7 years';

-- ============================================================================
-- Validation Queries (Run after migration to verify success)
-- ============================================================================

-- Verify all 6 tables exist
SELECT 
    tablename,
    schemaname
FROM pg_tables 
WHERE tablename IN (
    'orchestration_audit',
    'signal_processing_audit',
    'ai_analysis_audit',
    'workflow_execution_audit',
    'notification_audit',
    'effectiveness_audit'
)
ORDER BY tablename;

-- Verify pgvector extension and ai_analysis_audit embedding column
SELECT 
    table_name,
    column_name,
    data_type
FROM information_schema.columns
WHERE table_name = 'ai_analysis_audit'
AND column_name = 'embedding';

-- Verify indexes (should show 29 total indexes across 6 tables)
SELECT 
    tablename,
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename IN (
    'orchestration_audit',
    'signal_processing_audit',
    'ai_analysis_audit',
    'workflow_execution_audit',
    'notification_audit',
    'effectiveness_audit'
)
ORDER BY tablename, indexname;

-- ============================================================================
-- Migration Complete
-- ============================================================================
-- Tables Created: 6 (orchestration, signal_processing, ai_analysis, workflow_execution, notification, effectiveness)
-- Indexes Created: 29 (including 1 HNSW pgvector index)
-- Triggers Created: 6 (updated_at timestamp management)
-- Decision 1a Applied: Only ai_analysis_audit has embedding vector(1536) column
-- ADR-032 v1.1 Compliance: All 6 audit endpoints have corresponding tables
-- ============================================================================

