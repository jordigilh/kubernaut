-- Kubernaut v1 schema — single-file squash for clean v1.0.1 start
-- Includes: base schema (27 pre-release), engine_config, content integrity,
-- action_type lifecycle, and effectiveness assessment tables.
--
-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- EXTENSIONS
-- =============================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =============================================================================
-- CORE TABLES (dependency order)
-- =============================================================================

-- 1. Resource References Table
CREATE TABLE resource_references (
    id BIGSERIAL PRIMARY KEY,
    resource_uid VARCHAR(36) UNIQUE NOT NULL,
    api_version VARCHAR(100) NOT NULL,
    kind VARCHAR(100) NOT NULL,
    name VARCHAR(253) NOT NULL,
    namespace VARCHAR(63),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(namespace, kind, name)
);

CREATE INDEX idx_resource_kind ON resource_references (kind);
CREATE INDEX idx_resource_namespace ON resource_references (namespace);
CREATE INDEX idx_resource_last_seen ON resource_references (last_seen);
CREATE INDEX idx_resource_uid ON resource_references (resource_uid);

-- 2. Action Histories Table
CREATE TABLE action_histories (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,
    max_actions INTEGER DEFAULT 1000,
    max_age_days INTEGER DEFAULT 30,
    compaction_strategy VARCHAR(20) DEFAULT 'pattern-aware',
    oscillation_window_minutes INTEGER DEFAULT 120,
    effectiveness_threshold DECIMAL(3,2) DEFAULT 0.70,
    pattern_min_occurrences INTEGER DEFAULT 3,
    total_actions INTEGER DEFAULT 0,
    last_action_at TIMESTAMP WITH TIME ZONE,
    last_analysis_at TIMESTAMP WITH TIME ZONE,
    next_analysis_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(resource_id)
);

CREATE INDEX idx_ah_last_action ON action_histories (last_action_at);
CREATE INDEX idx_ah_next_analysis ON action_histories (next_analysis_at);
CREATE INDEX idx_ah_resource_id ON action_histories (resource_id);

-- 3. Oscillation Patterns Table
CREATE TABLE oscillation_patterns (
    id BIGSERIAL PRIMARY KEY,
    pattern_type VARCHAR(50) NOT NULL,
    pattern_name VARCHAR(200) NOT NULL,
    description TEXT,
    min_occurrences INTEGER NOT NULL DEFAULT 3,
    time_window_minutes INTEGER NOT NULL DEFAULT 120,
    action_sequence JSONB,
    threshold_config JSONB,
    resource_types TEXT[],
    namespaces TEXT[],
    label_selectors JSONB,
    prevention_strategy VARCHAR(50) NOT NULL,
    prevention_parameters JSONB,
    alerting_enabled BOOLEAN DEFAULT true,
    alert_severity VARCHAR(20) DEFAULT 'warning',
    alert_channels TEXT[],
    total_detections INTEGER DEFAULT 0,
    prevention_success_rate DECIMAL(4,3),
    false_positive_rate DECIMAL(4,3),
    last_detection_at TIMESTAMP WITH TIME ZONE,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_op_pattern_type ON oscillation_patterns (pattern_type);
CREATE INDEX idx_op_active_patterns ON oscillation_patterns (active);
CREATE INDEX idx_op_last_detection ON oscillation_patterns (last_detection_at);

-- 4. Oscillation Detections Table
CREATE TABLE oscillation_detections (
    id BIGSERIAL PRIMARY KEY,
    pattern_id BIGINT NOT NULL REFERENCES oscillation_patterns(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES resource_references(id) ON DELETE CASCADE,
    detected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    confidence DECIMAL(4,3) NOT NULL,
    action_count INTEGER NOT NULL,
    time_span_minutes INTEGER NOT NULL,
    matching_actions BIGINT[],
    pattern_evidence JSONB,
    prevention_applied BOOLEAN DEFAULT false,
    prevention_action VARCHAR(50),
    prevention_details JSONB,
    prevention_successful BOOLEAN,
    resolved BOOLEAN DEFAULT false,
    resolved_at TIMESTAMP WITH TIME ZONE,
    resolution_method VARCHAR(50),
    resolution_notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_od_pattern_resource ON oscillation_detections (pattern_id, resource_id);
CREATE INDEX idx_od_detected_at ON oscillation_detections (detected_at);
CREATE INDEX idx_od_unresolved ON oscillation_detections (resolved) WHERE resolved = false;

-- 5. Action Effectiveness Metrics Table
CREATE TABLE action_effectiveness_metrics (
    id BIGSERIAL PRIMARY KEY,
    scope_type VARCHAR(50) NOT NULL,
    scope_value VARCHAR(200),
    metric_period VARCHAR(20) NOT NULL,
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    action_type VARCHAR(50) NOT NULL,
    sample_size INTEGER NOT NULL,
    average_score DECIMAL(4,3) NOT NULL,
    median_score DECIMAL(4,3),
    std_deviation DECIMAL(4,3),
    confidence_interval_lower DECIMAL(4,3),
    confidence_interval_upper DECIMAL(4,3),
    trend_direction VARCHAR(20),
    trend_confidence DECIMAL(4,3),
    min_sample_size_met BOOLEAN,
    statistical_significance DECIMAL(4,3),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(scope_type, scope_value, metric_period, period_start, action_type)
);

CREATE INDEX idx_aem_scope_period ON action_effectiveness_metrics (scope_type, scope_value, metric_period);
CREATE INDEX idx_aem_period_range ON action_effectiveness_metrics (period_start, period_end);
CREATE INDEX idx_aem_action_effectiveness ON action_effectiveness_metrics (action_type, average_score);

-- 6. Retention Operations Table
CREATE TABLE retention_operations (
    id BIGSERIAL PRIMARY KEY,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,
    operation_type VARCHAR(30) NOT NULL,
    strategy_used VARCHAR(30) NOT NULL,
    records_before INTEGER NOT NULL,
    records_after INTEGER NOT NULL,
    records_deleted INTEGER NOT NULL,
    records_archived INTEGER,
    retention_criteria JSONB,
    preserved_criteria JSONB,
    operation_start TIMESTAMP WITH TIME ZONE NOT NULL,
    operation_end TIMESTAMP WITH TIME ZONE,
    operation_duration_ms INTEGER,
    operation_status VARCHAR(20) DEFAULT 'running',
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ro_action_history_ops ON retention_operations (action_history_id);
CREATE INDEX idx_ro_operation_time ON retention_operations (operation_start);

-- 7. Action Type Taxonomy (before workflow catalog for FK)
CREATE TABLE action_type_taxonomy (
    action_type TEXT PRIMARY KEY,
    description JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE action_type_taxonomy IS 'Curated taxonomy of remediation action types (DD-WORKFLOW-016)';
COMMENT ON COLUMN action_type_taxonomy.action_type IS 'Action type identifier (e.g., ScaleReplicas, RestartPod)';
COMMENT ON COLUMN action_type_taxonomy.description IS 'JSONB with camelCase keys: what, whenToUse, whenNotToUse, preconditions';
COMMENT ON COLUMN action_type_taxonomy.status IS 'Lifecycle status: active, disabled, deprecated, archived, or superseded';
COMMENT ON COLUMN action_type_taxonomy.disabled_at IS 'Timestamp when action type was soft-disabled';
COMMENT ON COLUMN action_type_taxonomy.disabled_by IS 'Identity (K8s SA or user) who disabled the action type';

-- 8. Remediation Workflow Catalog (UUID PK, schema_image, execution_bundle, description JSONB)
CREATE TABLE remediation_workflow_catalog (
    workflow_id UUID NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_name VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner VARCHAR(255),
    maintainer VARCHAR(255),
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    parameters JSONB,
    execution_engine VARCHAR(50) NOT NULL DEFAULT 'tekton',
    schema_image TEXT,
    schema_digest VARCHAR(71),
    execution_bundle TEXT,
    execution_bundle_digest VARCHAR(71),
    custom_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    detected_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    engine_config JSONB,
    action_type TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    status_reason TEXT,
    schema_version VARCHAR(10) NOT NULL DEFAULT '1.0',
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,
    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),
    deprecation_notice TEXT,
    version_notes TEXT,
    change_summary TEXT,
    approved_by VARCHAR(255),
    approved_at TIMESTAMP WITH TIME ZONE,
    expected_success_rate DECIMAL(4,3),
    expected_duration_seconds INTEGER,
    actual_success_rate DECIMAL(4,3),
    total_executions INTEGER DEFAULT 0,
    successful_executions INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived', 'superseded')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    CHECK (total_executions >= 0),
    CHECK (successful_executions >= 0 AND successful_executions <= total_executions)
);

CREATE INDEX idx_workflow_catalog_status ON remediation_workflow_catalog(status);
CREATE INDEX idx_workflow_catalog_latest_by_name ON remediation_workflow_catalog(workflow_name, is_latest_version) WHERE is_latest_version = true;
CREATE INDEX idx_workflow_catalog_workflow_name ON remediation_workflow_catalog(workflow_name);
CREATE INDEX idx_workflow_catalog_labels ON remediation_workflow_catalog USING GIN (labels);
CREATE INDEX idx_workflow_catalog_created_at ON remediation_workflow_catalog(created_at DESC);
CREATE INDEX idx_workflow_catalog_success_rate ON remediation_workflow_catalog(actual_success_rate DESC) WHERE status = 'active';
CREATE INDEX idx_workflow_catalog_schema_digest ON remediation_workflow_catalog(schema_digest) WHERE schema_digest IS NOT NULL;
CREATE INDEX idx_workflow_catalog_execution_bundle_digest ON remediation_workflow_catalog(execution_bundle_digest) WHERE execution_bundle_digest IS NOT NULL;
CREATE INDEX idx_workflow_catalog_custom_labels ON remediation_workflow_catalog USING GIN (custom_labels);
CREATE INDEX idx_workflow_catalog_detected_labels ON remediation_workflow_catalog USING GIN (detected_labels);
CREATE INDEX idx_workflow_action_type_status_version ON remediation_workflow_catalog(action_type, status, is_latest_version);
CREATE UNIQUE INDEX uq_workflow_name_version_active ON remediation_workflow_catalog (workflow_name, version) WHERE status = 'active';

COMMENT ON COLUMN remediation_workflow_catalog.description IS 'JSONB with camelCase keys (what, whenToUse, whenNotToUse, preconditions)';
COMMENT ON COLUMN remediation_workflow_catalog.schema_image IS 'OCI image pulled at registration to extract /workflow-schema.yaml';
COMMENT ON COLUMN remediation_workflow_catalog.schema_digest IS 'SHA256 digest of the schema image';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle IS 'OCI execution bundle reference (digest-pinned) for Tekton/Job runtime';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle_digest IS 'SHA256 digest portion of execution_bundle';
COMMENT ON COLUMN remediation_workflow_catalog.labels IS 'JSONB labels use signalName key for semantic signal matching';
COMMENT ON COLUMN remediation_workflow_catalog.engine_config IS 'Engine-specific configuration as JSONB (e.g., ansible playbookPath, inventoryName, jobTemplateName). NULL for tekton/job.';

-- 9. Resource Action Traces Table (partitioned)
CREATE TABLE resource_action_traces (
    id BIGSERIAL,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,
    action_id VARCHAR(64) NOT NULL,
    correlation_id VARCHAR(64),
    action_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    execution_start_time TIMESTAMP WITH TIME ZONE,
    execution_end_time TIMESTAMP WITH TIME ZONE,
    execution_duration_ms INTEGER,
    signal_name VARCHAR(200) NOT NULL,
    signal_severity VARCHAR(20) NOT NULL,
    signal_labels JSONB,
    signal_annotations JSONB,
    signal_firing_time TIMESTAMP WITH TIME ZONE,
    incident_type VARCHAR(100),
    alert_name VARCHAR(255),
    incident_severity VARCHAR(20),
    workflow_id VARCHAR(64),
    workflow_version VARCHAR(20),
    workflow_step_number INT,
    workflow_execution_id VARCHAR(64),
    ai_selected_workflow BOOLEAN DEFAULT false,
    ai_chained_workflows BOOLEAN DEFAULT false,
    ai_manual_escalation BOOLEAN DEFAULT false,
    ai_workflow_customization JSONB,
    model_used VARCHAR(100) NOT NULL,
    routing_tier VARCHAR(20),
    model_confidence DECIMAL(4,3) NOT NULL,
    model_reasoning TEXT,
    alternative_actions JSONB,
    action_type VARCHAR(50) NOT NULL,
    action_parameters JSONB,
    resource_state_before JSONB,
    resource_state_after JSONB,
    execution_status VARCHAR(20) DEFAULT 'pending',
    execution_error TEXT,
    kubernetes_operations JSONB,
    effectiveness_score DECIMAL(4,3),
    effectiveness_criteria JSONB,
    effectiveness_assessed_at TIMESTAMP WITH TIME ZONE,
    effectiveness_assessment_method VARCHAR(20),
    effectiveness_assessment_due TIMESTAMP WITH TIME ZONE,
    effectiveness_notes TEXT,
    follow_up_actions JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, action_timestamp),
    UNIQUE (action_id, action_timestamp)
) PARTITION BY RANGE (action_timestamp);

CREATE TABLE resource_action_traces_default PARTITION OF resource_action_traces DEFAULT;

CREATE TABLE resource_action_traces_2026_03 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE resource_action_traces_2026_04 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE resource_action_traces_2026_05 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE resource_action_traces_2026_06 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE resource_action_traces_2026_07 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE resource_action_traces_2026_08 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE resource_action_traces_2026_09 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE resource_action_traces_2026_10 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE resource_action_traces_2026_11 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE resource_action_traces_2026_12 PARTITION OF resource_action_traces FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE resource_action_traces_2027_01 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
CREATE TABLE resource_action_traces_2027_02 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
CREATE TABLE resource_action_traces_2027_03 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');
CREATE TABLE resource_action_traces_2027_04 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');
CREATE TABLE resource_action_traces_2027_05 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');
CREATE TABLE resource_action_traces_2027_06 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');
CREATE TABLE resource_action_traces_2027_07 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');
CREATE TABLE resource_action_traces_2027_08 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');
CREATE TABLE resource_action_traces_2027_09 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');
CREATE TABLE resource_action_traces_2027_10 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');
CREATE TABLE resource_action_traces_2027_11 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');
CREATE TABLE resource_action_traces_2027_12 PARTITION OF resource_action_traces FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');
CREATE TABLE resource_action_traces_2028_01 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');
CREATE TABLE resource_action_traces_2028_02 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');
CREATE TABLE resource_action_traces_2028_03 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');
CREATE TABLE resource_action_traces_2028_04 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');
CREATE TABLE resource_action_traces_2028_05 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');
CREATE TABLE resource_action_traces_2028_06 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');
CREATE TABLE resource_action_traces_2028_07 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');
CREATE TABLE resource_action_traces_2028_08 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');
CREATE TABLE resource_action_traces_2028_09 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');
CREATE TABLE resource_action_traces_2028_10 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');
CREATE TABLE resource_action_traces_2028_11 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');
CREATE TABLE resource_action_traces_2028_12 PARTITION OF resource_action_traces FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');

CREATE INDEX idx_rat_action_history ON resource_action_traces (action_history_id, action_timestamp);
CREATE INDEX idx_rat_action_type ON resource_action_traces (action_type, action_timestamp);
CREATE INDEX idx_rat_model_used ON resource_action_traces (model_used, action_timestamp);
CREATE INDEX idx_rat_signal_name ON resource_action_traces (signal_name, action_timestamp);
CREATE INDEX idx_rat_signal_labels_gin ON resource_action_traces USING GIN (signal_labels);
CREATE INDEX idx_rat_execution_status ON resource_action_traces (execution_status) WHERE execution_status IN ('pending', 'executing');
CREATE INDEX idx_rat_effectiveness_score ON resource_action_traces (effectiveness_score) WHERE effectiveness_score IS NOT NULL;
CREATE INDEX idx_rat_correlation_id ON resource_action_traces (correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX idx_rat_effectiveness_due ON resource_action_traces (effectiveness_assessment_due);
CREATE INDEX idx_rat_action_parameters_gin ON resource_action_traces USING GIN (action_parameters);
CREATE INDEX idx_rat_resource_state_gin ON resource_action_traces USING GIN (resource_state_before);
CREATE INDEX idx_rat_resource_action_time ON resource_action_traces (action_history_id, action_type, action_timestamp DESC);
CREATE INDEX idx_rat_effectiveness_analysis ON resource_action_traces (action_type, effectiveness_score, action_timestamp) WHERE effectiveness_score IS NOT NULL;
CREATE INDEX idx_incident_type_success ON resource_action_traces(incident_type, execution_status, action_timestamp DESC) WHERE incident_type IS NOT NULL;
CREATE INDEX idx_workflow_success ON resource_action_traces(workflow_id, workflow_version, execution_status, action_timestamp DESC) WHERE workflow_id IS NOT NULL;
CREATE INDEX idx_multidimensional_success ON resource_action_traces(incident_type, workflow_id, action_type, execution_status, action_timestamp DESC) WHERE incident_type IS NOT NULL AND workflow_id IS NOT NULL;
CREATE INDEX idx_workflow_execution ON resource_action_traces(workflow_execution_id, workflow_step_number, action_timestamp DESC) WHERE workflow_execution_id IS NOT NULL;
CREATE INDEX idx_ai_execution_mode ON resource_action_traces(incident_type, ai_selected_workflow, ai_chained_workflows, ai_manual_escalation, action_timestamp DESC) WHERE incident_type IS NOT NULL;
CREATE INDEX idx_alert_name_lookup ON resource_action_traces(alert_name, execution_status, action_timestamp DESC) WHERE alert_name IS NOT NULL;

-- 10. Audit Events Table (partitioned)
CREATE TABLE audit_events (
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_category VARCHAR(50) NOT NULL,
    event_action VARCHAR(50) NOT NULL,
    event_outcome VARCHAR(20) NOT NULL,
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    actor_ip INET,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),
    correlation_id VARCHAR(255) NOT NULL,
    parent_event_id UUID,
    parent_event_date DATE,
    trace_id VARCHAR(255),
    span_id VARCHAR(255),
    namespace VARCHAR(253),
    cluster_name VARCHAR(255),
    event_data JSONB NOT NULL,
    event_metadata JSONB,
    severity VARCHAR(20),
    duration_ms INTEGER,
    error_code VARCHAR(50),
    error_message TEXT,
    retention_days INTEGER DEFAULT 2555,
    is_sensitive BOOLEAN DEFAULT FALSE,
    event_hash TEXT,
    previous_event_hash TEXT,
    legal_hold BOOLEAN DEFAULT FALSE,
    legal_hold_reason TEXT,
    legal_hold_placed_by TEXT,
    legal_hold_placed_at TIMESTAMP,
    PRIMARY KEY (event_id, event_date)
) PARTITION BY RANGE (event_date);

CREATE TABLE audit_events_default PARTITION OF audit_events DEFAULT;

CREATE TABLE audit_events_2026_03 PARTITION OF audit_events FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE audit_events_2026_04 PARTITION OF audit_events FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE audit_events_2026_05 PARTITION OF audit_events FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE audit_events_2026_06 PARTITION OF audit_events FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE audit_events_2026_07 PARTITION OF audit_events FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE audit_events_2026_08 PARTITION OF audit_events FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE audit_events_2026_09 PARTITION OF audit_events FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE audit_events_2026_10 PARTITION OF audit_events FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE audit_events_2026_11 PARTITION OF audit_events FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE audit_events_2026_12 PARTITION OF audit_events FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE audit_events_2027_01 PARTITION OF audit_events FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
CREATE TABLE audit_events_2027_02 PARTITION OF audit_events FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
CREATE TABLE audit_events_2027_03 PARTITION OF audit_events FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');
CREATE TABLE audit_events_2027_04 PARTITION OF audit_events FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');
CREATE TABLE audit_events_2027_05 PARTITION OF audit_events FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');
CREATE TABLE audit_events_2027_06 PARTITION OF audit_events FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');
CREATE TABLE audit_events_2027_07 PARTITION OF audit_events FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');
CREATE TABLE audit_events_2027_08 PARTITION OF audit_events FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');
CREATE TABLE audit_events_2027_09 PARTITION OF audit_events FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');
CREATE TABLE audit_events_2027_10 PARTITION OF audit_events FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');
CREATE TABLE audit_events_2027_11 PARTITION OF audit_events FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');
CREATE TABLE audit_events_2027_12 PARTITION OF audit_events FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');
CREATE TABLE audit_events_2028_01 PARTITION OF audit_events FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');
CREATE TABLE audit_events_2028_02 PARTITION OF audit_events FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');
CREATE TABLE audit_events_2028_03 PARTITION OF audit_events FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');
CREATE TABLE audit_events_2028_04 PARTITION OF audit_events FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');
CREATE TABLE audit_events_2028_05 PARTITION OF audit_events FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');
CREATE TABLE audit_events_2028_06 PARTITION OF audit_events FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');
CREATE TABLE audit_events_2028_07 PARTITION OF audit_events FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');
CREATE TABLE audit_events_2028_08 PARTITION OF audit_events FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');
CREATE TABLE audit_events_2028_09 PARTITION OF audit_events FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');
CREATE TABLE audit_events_2028_10 PARTITION OF audit_events FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');
CREATE TABLE audit_events_2028_11 PARTITION OF audit_events FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');
CREATE TABLE audit_events_2028_12 PARTITION OF audit_events FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');

CREATE INDEX idx_audit_events_event_timestamp ON audit_events (event_timestamp DESC);
CREATE INDEX idx_audit_events_correlation_id ON audit_events (correlation_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_event_type ON audit_events (event_type, event_timestamp DESC);
CREATE INDEX idx_audit_events_resource ON audit_events (resource_type, resource_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_actor ON audit_events (actor_type, actor_id, event_timestamp DESC);
CREATE INDEX idx_audit_events_outcome ON audit_events (event_outcome, event_timestamp DESC);
CREATE INDEX idx_audit_events_event_date ON audit_events (event_date);
CREATE INDEX idx_audit_events_event_data_gin ON audit_events USING GIN (event_data);
CREATE INDEX idx_audit_events_hash ON audit_events(event_hash) WHERE event_hash IS NOT NULL;
CREATE INDEX idx_audit_events_legal_hold ON audit_events(legal_hold) WHERE legal_hold = TRUE;
CREATE INDEX idx_audit_events_target_resource ON audit_events ((event_data->>'target_resource'), event_timestamp DESC) WHERE event_type IN ('remediation.workflow_created', 'effectiveness.assessment.completed');
CREATE INDEX idx_audit_events_pre_remediation_spec_hash ON audit_events ((event_data->>'pre_remediation_spec_hash'), event_timestamp DESC) WHERE event_data->>'pre_remediation_spec_hash' IS NOT NULL;

-- 11. Audit Retention Policies
CREATE TABLE audit_retention_policies (
    policy_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_category TEXT NOT NULL UNIQUE,
    retention_days INTEGER NOT NULL,
    legal_hold_override BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 12. Notification Audit
CREATE TABLE notification_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    notification_id VARCHAR(255) NOT NULL UNIQUE,
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL CHECK (channel IN ('email', 'slack', 'pagerduty', 'sms')),
    message_summary TEXT NOT NULL,
    status VARCHAR(50) NOT NULL CHECK (status IN ('sent', 'failed', 'acknowledged', 'escalated')),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivery_status TEXT,
    error_message TEXT,
    escalation_level INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_audit_notification_id ON notification_audit(notification_id);
CREATE INDEX idx_notification_audit_remediation_id ON notification_audit(remediation_id);
CREATE INDEX idx_notification_audit_channel ON notification_audit(channel);
CREATE INDEX idx_notification_audit_status ON notification_audit(status);
CREATE INDEX idx_notification_audit_created_at ON notification_audit(created_at DESC);

-- =============================================================================
-- FOREIGN KEYS (after tables exist)
-- =============================================================================
ALTER TABLE remediation_workflow_catalog ADD CONSTRAINT fk_workflow_action_type FOREIGN KEY (action_type) REFERENCES action_type_taxonomy(action_type);

ALTER TABLE audit_events ADD CONSTRAINT fk_audit_events_parent
    FOREIGN KEY (parent_event_id, parent_event_date) REFERENCES audit_events(event_id, event_date) ON DELETE RESTRICT;

-- =============================================================================
-- FUNCTIONS
-- =============================================================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_audit_event_date()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_date := NEW.event_timestamp::DATE;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION prevent_legal_hold_deletion()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = TRUE THEN
        RAISE EXCEPTION 'Cannot delete audit event with legal hold: event_id=%, correlation_id=%', OLD.event_id, OLD.correlation_id
            USING HINT = 'Release legal hold before deletion via DELETE /api/v1/audit/legal-hold/{correlation_id}', ERRCODE = '23503';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION audit_event_lock_id(correlation_id_param TEXT)
RETURNS BIGINT AS $$
DECLARE
    hash_bytes BYTEA;
    lock_id BIGINT;
BEGIN
    hash_bytes := digest(correlation_id_param, 'sha256');
    lock_id := ('x' || encode(substring(hash_bytes, 1, 8), 'hex'))::bit(64)::bigint;
    RETURN lock_id;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS void AS $$
DECLARE
    start_date date;
    end_date date;
    table_name text;
BEGIN
    start_date := date_trunc('month', CURRENT_DATE + interval '1 month');
    end_date := start_date + interval '1 month';
    table_name := 'resource_action_traces_' || to_char(start_date, 'YYYY') || '_' || to_char(start_date, 'MM');
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF resource_action_traces FOR VALUES FROM (%L) TO (%L)', table_name, start_date, end_date);
    RAISE NOTICE 'Created partition: %', table_name;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- STORED PROCEDURES (use signal_name, signal_severity)
-- =============================================================================
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

CREATE OR REPLACE FUNCTION get_resource_id(p_namespace VARCHAR(63), p_kind VARCHAR(100), p_name VARCHAR(253))
RETURNS INTEGER AS $$
DECLARE v_resource_id INTEGER;
BEGIN
    SELECT id INTO v_resource_id FROM resource_references WHERE namespace = p_namespace AND kind = p_kind AND name = p_name;
    IF v_resource_id IS NULL THEN RAISE EXCEPTION 'Resource not found: namespace=%, kind=%, name=%', p_namespace, p_kind, p_name; END IF;
    RETURN v_resource_id;
END;
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER;

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

CREATE OR REPLACE FUNCTION get_recent_actions(p_limit INTEGER DEFAULT 100, p_signal_name VARCHAR(200) DEFAULT NULL, p_signal_severity VARCHAR(20) DEFAULT NULL)
RETURNS TABLE (action_id VARCHAR, action_timestamp TIMESTAMP WITH TIME ZONE, signal_name VARCHAR, signal_severity VARCHAR, execution_status VARCHAR) AS $$
BEGIN
    RETURN QUERY
    SELECT rat.action_id::VARCHAR, rat.action_timestamp, rat.signal_name::VARCHAR, rat.signal_severity::VARCHAR, rat.execution_status
    FROM resource_action_traces rat WHERE (p_signal_name IS NULL OR rat.signal_name = p_signal_name) AND (p_signal_severity IS NULL OR rat.signal_severity = p_signal_severity)
    ORDER BY rat.action_timestamp DESC LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- TRIGGERS
-- =============================================================================
CREATE TRIGGER trg_set_audit_event_date BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION set_audit_event_date();
CREATE TRIGGER enforce_legal_hold BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion();
CREATE TRIGGER update_action_histories_updated_at BEFORE UPDATE ON action_histories FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_resource_action_traces_updated_at BEFORE UPDATE ON resource_action_traces FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER update_oscillation_patterns_updated_at BEFORE UPDATE ON oscillation_patterns FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trigger_workflow_catalog_updated_at BEFORE UPDATE ON remediation_workflow_catalog FOR EACH ROW EXECUTE FUNCTION update_workflow_catalog_updated_at();
CREATE TRIGGER trigger_action_type_taxonomy_updated_at BEFORE UPDATE ON action_type_taxonomy FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- =============================================================================
-- VIEWS
-- =============================================================================
CREATE VIEW action_history_summary AS
SELECT rr.namespace, rr.kind, rr.name, ah.total_actions, ah.last_action_at,
    COUNT(rat.id) as recent_actions_24h, AVG(rat.effectiveness_score) as avg_effectiveness_24h, COUNT(DISTINCT rat.action_type) as action_types_used
FROM resource_references rr JOIN action_histories ah ON ah.resource_id = rr.id
LEFT JOIN resource_action_traces rat ON rat.action_history_id = ah.id AND rat.action_timestamp > NOW() - INTERVAL '24 hours'
GROUP BY rr.id, rr.namespace, rr.kind, rr.name, ah.total_actions, ah.last_action_at;

CREATE VIEW oscillation_detection_summary AS
SELECT pattern_type, COUNT(*) as total_detections, COUNT(*) FILTER (WHERE prevention_applied = true) as preventions_applied,
    COUNT(*) FILTER (WHERE prevention_successful = true) as successful_preventions, AVG(confidence) as avg_confidence, MAX(detected_at) as last_detection
FROM oscillation_detections od JOIN oscillation_patterns op ON od.pattern_id = op.id GROUP BY pattern_type;

CREATE VIEW incident_summary_view AS
SELECT signal_severity as severity, COUNT(*) as incident_count FROM resource_action_traces GROUP BY signal_severity
ORDER BY CASE signal_severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END;

-- =============================================================================
-- SEED DATA
-- =============================================================================
INSERT INTO oscillation_patterns (pattern_type, pattern_name, description, min_occurrences, time_window_minutes, threshold_config, prevention_strategy, prevention_parameters) VALUES
('scale-oscillation', 'Scale Up/Down Oscillation', 'Rapid alternating scale up and scale down operations within a short time window', 3, 120, '{"min_direction_changes": 2, "max_time_between_actions": 30, "effectiveness_threshold": 0.5}', 'cooling-period', '{"cooling_period_minutes": 30, "escalate_after": 3}'),
('resource-thrashing', 'Resource/Scale Thrashing', 'Alternating between resource adjustments and scaling decisions', 2, 90, '{"action_types": ["increase_resources", "scale_deployment"], "effectiveness_threshold": 0.6}', 'alternative-action', '{"suggest_alternatives": true, "block_conflicting": true}'),
('ineffective-loop', 'Ineffective Action Loop', 'Repeated actions with consistently low effectiveness scores', 4, 180, '{"effectiveness_threshold": 0.3, "min_repetitions": 3}', 'escalate-human', '{"escalation_webhook": null, "require_approval": true}'),
('cascading-failure', 'Cascading Failure Pattern', 'Actions that trigger more alerts than they resolve', 2, 60, '{"new_alerts_threshold": 1.5, "recurrence_rate_threshold": 0.4}', 'block-action', '{"block_duration_minutes": 60, "require_manual_override": true}');

INSERT INTO audit_retention_policies (event_category, retention_days) VALUES
('gateway', 2555), ('workflow', 2555), ('remediation', 2555), ('analysis', 2555), ('notification', 2555);

-- =============================================================================
-- GRANTS (optional - for slm_user if exists)
-- =============================================================================
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'slm_user') THEN
        GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
        GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
        GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
    END IF;
END $$;

-- =============================================================================
-- EFFECTIVENESS ASSESSMENT TABLES (v1.1 feature, squashed in for clean start)
-- =============================================================================

-- 13. Action Assessments (pending effectiveness assessments)
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

-- 14. Effectiveness Results
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

-- 15. Action Confidence Scores (core learning mechanism)
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

-- 16. Action Outcomes (for learning algorithms)
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

-- 17. Action Alternatives (alternative action recommendations)
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

-- Effectiveness Views
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

-- Effectiveness Function and Trigger
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

CREATE TRIGGER trigger_create_assessment_for_action_trace
    AFTER UPDATE ON resource_action_traces
    FOR EACH ROW
    EXECUTE FUNCTION create_assessment_for_action_trace();

-- Effectiveness Seed Data
INSERT INTO action_confidence_scores (action_type, context_hash, base_confidence, adjusted_confidence, adjustment_reason)
VALUES
    ('restart_pod', 'default', 0.7, 0.7, 'Default confidence for pod restarts'),
    ('scale_deployment', 'default', 0.75, 0.75, 'Default confidence for deployment scaling'),
    ('delete_pod', 'default', 0.6, 0.6, 'Default confidence for pod deletion'),
    ('rollback_deployment', 'default', 0.8, 0.8, 'Default confidence for deployment rollback')
ON CONFLICT (action_type, context_hash) DO NOTHING;

COMMENT ON TABLE action_assessments IS 'Pending effectiveness assessments for completed actions';
COMMENT ON TABLE effectiveness_results IS 'Results of AI effectiveness assessments for learning';
COMMENT ON TABLE action_confidence_scores IS 'Dynamic confidence scores that improve through learning';
COMMENT ON TABLE action_outcomes IS 'Historical outcomes for training ML algorithms';
COMMENT ON TABLE action_alternatives IS 'Alternative actions for failed patterns';
COMMENT ON VIEW effectiveness_trends IS 'Daily trends in action effectiveness for monitoring';
COMMENT ON VIEW low_confidence_actions IS 'Actions requiring attention due to poor performance';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop in reverse dependency order

-- Views
DROP VIEW IF EXISTS low_confidence_actions;
DROP VIEW IF EXISTS effectiveness_trends;
DROP VIEW IF EXISTS incident_summary_view;
DROP VIEW IF EXISTS oscillation_detection_summary;
DROP VIEW IF EXISTS action_history_summary;

-- Triggers (before functions)
DROP TRIGGER IF EXISTS trigger_create_assessment_for_action_trace ON resource_action_traces;
DROP TRIGGER IF EXISTS trigger_action_type_taxonomy_updated_at ON action_type_taxonomy;
DROP TRIGGER IF EXISTS trigger_workflow_catalog_updated_at ON remediation_workflow_catalog;
DROP TRIGGER IF EXISTS update_oscillation_patterns_updated_at ON oscillation_patterns;
DROP TRIGGER IF EXISTS update_resource_action_traces_updated_at ON resource_action_traces;
DROP TRIGGER IF EXISTS update_action_histories_updated_at ON action_histories;
DROP TRIGGER IF EXISTS enforce_legal_hold ON audit_events;
DROP TRIGGER IF EXISTS trg_set_audit_event_date ON audit_events;

-- Effectiveness tables (before resource_action_traces which they reference)
DROP TABLE IF EXISTS action_alternatives;
DROP TABLE IF EXISTS action_outcomes;
DROP TABLE IF EXISTS action_confidence_scores;
DROP TABLE IF EXISTS effectiveness_results;
DROP TABLE IF EXISTS action_assessments;

-- Core tables (CASCADE for partitioned tables drops partitions)
DROP TABLE IF EXISTS notification_audit;
DROP TABLE IF EXISTS audit_retention_policies;
DROP TABLE IF EXISTS audit_events CASCADE;
DROP TABLE IF EXISTS resource_action_traces CASCADE;
DROP TABLE IF EXISTS remediation_workflow_catalog;
DROP TABLE IF EXISTS action_type_taxonomy;
DROP TABLE IF EXISTS retention_operations;
DROP TABLE IF EXISTS action_effectiveness_metrics;
DROP TABLE IF EXISTS oscillation_detections;
DROP TABLE IF EXISTS oscillation_patterns;
DROP TABLE IF EXISTS action_histories;
DROP TABLE IF EXISTS resource_references;

-- Functions
DROP FUNCTION IF EXISTS create_assessment_for_action_trace();
DROP FUNCTION IF EXISTS get_recent_actions(INTEGER, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS analyze_cascade_effects(INTEGER, INTERVAL, INTEGER);
DROP FUNCTION IF EXISTS analyze_action_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS get_resource_id(VARCHAR, VARCHAR, VARCHAR);
DROP FUNCTION IF EXISTS get_resource_actions_base(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS store_oscillation_detection(INTEGER, VARCHAR, VARCHAR, VARCHAR, DECIMAL, INTEGER, INTEGER, JSONB, VARCHAR);
DROP FUNCTION IF EXISTS get_action_effectiveness(VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE);
DROP FUNCTION IF EXISTS get_action_traces(VARCHAR, VARCHAR, VARCHAR, VARCHAR, VARCHAR, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE, INTEGER, INTEGER);
DROP FUNCTION IF EXISTS detect_cascading_failures(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_ineffective_loops(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_resource_thrashing(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS detect_scale_oscillation(VARCHAR, VARCHAR, VARCHAR, INTEGER);
DROP FUNCTION IF EXISTS audit_event_lock_id(TEXT);
DROP FUNCTION IF EXISTS prevent_legal_hold_deletion();
DROP FUNCTION IF EXISTS set_audit_event_date();
DROP FUNCTION IF EXISTS update_workflow_catalog_updated_at();
DROP FUNCTION IF EXISTS create_monthly_partitions();
DROP FUNCTION IF EXISTS update_updated_at();

-- Extensions (optional - may be used by other schemas)
-- DROP EXTENSION IF EXISTS pgcrypto;
-- DROP EXTENSION IF EXISTS "uuid-ossp";

-- +goose StatementEnd
