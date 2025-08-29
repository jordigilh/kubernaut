# Database-First Action History Storage Design

**Objective**: Scalable persistent storage for AI action tracking and oscillation detection  
**Primary Storage**: PostgreSQL database for enterprise-scale action history  
**Secondary Integration**: Lightweight CRDs for Kubernetes-native visibility  
**Target**: Handle millions of action records with high performance and advanced analytics

## 🎯 **Database Schema Design**

### **Core Database Tables**

#### **1. resource_references**
**Purpose**: Central registry of Kubernetes resources being tracked

```sql
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
    
    -- Composite index for fast lookups
    UNIQUE(namespace, kind, name),
    INDEX idx_resource_kind (kind),
    INDEX idx_resource_namespace (namespace),
    INDEX idx_resource_last_seen (last_seen)
);
```

#### **2. action_histories**
**Purpose**: Configuration and metadata for action tracking per resource

```sql
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
    
    UNIQUE(resource_id),
    INDEX idx_last_action (last_action_at),
    INDEX idx_next_analysis (next_analysis_at)
);
```

#### **3. resource_action_traces**
**Purpose**: Individual action records with full context and outcomes

```sql
CREATE TABLE resource_action_traces (
    id BIGSERIAL PRIMARY KEY,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,
    
    -- Action identification
    action_id VARCHAR(64) UNIQUE NOT NULL, -- UUID for this specific action
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
    
    -- Indexes for performance
    INDEX idx_action_history (action_history_id),
    INDEX idx_action_timestamp (action_timestamp),
    INDEX idx_action_type (action_type),
    INDEX idx_model_used (model_used),
    INDEX idx_alert_name (alert_name),
    INDEX idx_execution_status (execution_status),
    INDEX idx_effectiveness_score (effectiveness_score),
    INDEX idx_correlation_id (correlation_id),
    
    -- Composite indexes for common queries
    INDEX idx_history_timestamp (action_history_id, action_timestamp),
    INDEX idx_type_timestamp (action_type, action_timestamp),
    INDEX idx_model_effectiveness (model_used, effectiveness_score)
);
```

#### **4. oscillation_patterns**
**Purpose**: Detected oscillation patterns and prevention rules

```sql
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
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    INDEX idx_pattern_type (pattern_type),
    INDEX idx_active_patterns (active),
    INDEX idx_last_detection (last_detection_at)
);
```

#### **5. oscillation_detections**
**Purpose**: Individual instances of pattern detection

```sql
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
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    INDEX idx_pattern_resource (pattern_id, resource_id),
    INDEX idx_detected_at (detected_at),
    INDEX idx_unresolved (resolved) WHERE resolved = false
);
```

#### **6. action_effectiveness_metrics**
**Purpose**: Aggregated effectiveness metrics for continuous learning

```sql
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
    UNIQUE(scope_type, scope_value, metric_period, period_start, action_type),
    INDEX idx_scope_period (scope_type, scope_value, metric_period),
    INDEX idx_period_range (period_start, period_end),
    INDEX idx_action_effectiveness (action_type, average_score)
);
```

#### **7. retention_operations**
**Purpose**: Track data retention and archival operations

```sql
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
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    INDEX idx_action_history_ops (action_history_id),
    INDEX idx_operation_time (operation_start)
);
```

## 🔗 **Database Optimization**

### **Partitioning Strategy**

```sql
-- Partition resource_action_traces by month for better performance
CREATE TABLE resource_action_traces_template (
    LIKE resource_action_traces INCLUDING ALL
);

-- Monthly partitions
CREATE TABLE resource_action_traces_y2025m01 
    PARTITION OF resource_action_traces_template
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE resource_action_traces_y2025m02 
    PARTITION OF resource_action_traces_template
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

-- Automated partition management
CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS void AS $$
DECLARE
    start_date date;
    end_date date;
    table_name text;
BEGIN
    start_date := date_trunc('month', CURRENT_DATE);
    end_date := start_date + interval '1 month';
    table_name := 'resource_action_traces_y' || 
                  to_char(start_date, 'YYYY') || 'm' || 
                  to_char(start_date, 'MM');
    
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I 
                   PARTITION OF resource_action_traces_template
                   FOR VALUES FROM (%L) TO (%L)',
                   table_name, start_date, end_date);
END;
$$ LANGUAGE plpgsql;

-- Schedule monthly partition creation
SELECT cron.schedule('create-partitions', '0 0 1 * *', 'SELECT create_monthly_partitions();');
```

### **Advanced Indexing**

```sql
-- GIN indexes for JSONB queries
CREATE INDEX idx_action_labels_gin ON resource_action_traces USING GIN (alert_labels);
CREATE INDEX idx_action_parameters_gin ON resource_action_traces USING GIN (action_parameters);
CREATE INDEX idx_resource_state_gin ON resource_action_traces USING GIN (resource_state_before);

-- Partial indexes for active data
CREATE INDEX idx_pending_actions ON resource_action_traces (action_timestamp) 
    WHERE execution_status IN ('pending', 'executing');

CREATE INDEX idx_recent_effectiveness ON resource_action_traces (effectiveness_score) 
    WHERE action_timestamp > NOW() - INTERVAL '7 days';

-- Expression indexes for common calculations
CREATE INDEX idx_execution_duration ON resource_action_traces 
    (EXTRACT(EPOCH FROM (execution_end_time - execution_start_time)));

-- Multi-column indexes for complex queries
CREATE INDEX idx_oscillation_detection ON resource_action_traces 
    (action_history_id, action_type, action_timestamp)
    WHERE action_timestamp > NOW() - INTERVAL '24 hours';
```

## 🛠 **Database Access Layer**

### **Repository Pattern Implementation**

```go
// Database repository interfaces
type ActionHistoryRepository interface {
    StoreAction(ctx context.Context, action *ActionRecord) error
    GetActionHistory(ctx context.Context, resourceRef ResourceReference, timeRange TimeRange) ([]ActionTrace, error)
    FindOscillationPatterns(ctx context.Context, resourceRef ResourceReference, windowDuration time.Duration) ([]OscillationDetection, error)
    GetEffectivenessMetrics(ctx context.Context, scope EffectivenessScope) (*EffectivenessMetrics, error)
    ApplyRetention(ctx context.Context, actionHistoryID int64) (*RetentionResult, error)
}

// PostgreSQL implementation
type PostgreSQLActionRepository struct {
    db     *sql.DB
    logger *logrus.Logger
}

func (r *PostgreSQLActionRepository) StoreAction(ctx context.Context, action *ActionRecord) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()
    
    // Ensure resource reference exists
    resourceID, err := r.ensureResourceReference(ctx, tx, action.ResourceReference)
    if err != nil {
        return fmt.Errorf("failed to ensure resource reference: %w", err)
    }
    
    // Ensure action history exists
    actionHistoryID, err := r.ensureActionHistory(ctx, tx, resourceID)
    if err != nil {
        return fmt.Errorf("failed to ensure action history: %w", err)
    }
    
    // Insert action trace
    query := `
        INSERT INTO resource_action_traces (
            action_history_id, action_id, correlation_id, action_timestamp,
            alert_name, alert_severity, alert_labels, alert_annotations,
            model_used, routing_tier, model_confidence, model_reasoning,
            action_type, action_parameters, resource_state_before,
            alternative_actions
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
        ) RETURNING id`
    
    var traceID int64
    err = tx.QueryRowContext(ctx, query,
        actionHistoryID, action.ActionID, action.CorrelationID, action.Timestamp,
        action.Alert.Name, action.Alert.Severity, action.Alert.Labels, action.Alert.Annotations,
        action.ModelUsed, action.RoutingTier, action.Confidence, action.Reasoning,
        action.ActionType, action.Parameters, action.ResourceStateBefore,
        action.AlternativeActions,
    ).Scan(&traceID)
    
    if err != nil {
        return fmt.Errorf("failed to insert action trace: %w", err)
    }
    
    // Update action history counters
    _, err = tx.ExecContext(ctx, `
        UPDATE action_histories 
        SET total_actions = total_actions + 1, 
            last_action_at = $2,
            updated_at = NOW()
        WHERE id = $1`,
        actionHistoryID, action.Timestamp)
    
    if err != nil {
        return fmt.Errorf("failed to update action history: %w", err)
    }
    
    return tx.Commit()
}

func (r *PostgreSQLActionRepository) FindOscillationPatterns(ctx context.Context, resourceRef ResourceReference, windowDuration time.Duration) ([]OscillationDetection, error) {
    query := `
        WITH recent_actions AS (
            SELECT rat.*, rr.namespace, rr.kind, rr.name
            FROM resource_action_traces rat
            JOIN action_histories ah ON rat.action_history_id = ah.id
            JOIN resource_references rr ON ah.resource_id = rr.id
            WHERE rr.namespace = $1 AND rr.kind = $2 AND rr.name = $3
            AND rat.action_timestamp > NOW() - INTERVAL '%d minutes'
            ORDER BY rat.action_timestamp DESC
        ),
        action_sequences AS (
            SELECT 
                action_type,
                action_timestamp,
                LAG(action_type) OVER (ORDER BY action_timestamp) as prev_action,
                LAG(action_timestamp) OVER (ORDER BY action_timestamp) as prev_timestamp,
                effectiveness_score
            FROM recent_actions
        )
        SELECT 
            COUNT(*) as pattern_count,
            action_type,
            MIN(action_timestamp) as first_occurrence,
            MAX(action_timestamp) as last_occurrence,
            AVG(effectiveness_score) as avg_effectiveness
        FROM action_sequences
        WHERE action_type = prev_action
        AND action_timestamp - prev_timestamp < INTERVAL '30 minutes'
        GROUP BY action_type
        HAVING COUNT(*) >= 3`
    
    formattedQuery := fmt.Sprintf(query, int(windowDuration.Minutes()))
    
    rows, err := r.db.QueryContext(ctx, formattedQuery, 
        resourceRef.Namespace, resourceRef.Kind, resourceRef.Name)
    if err != nil {
        return nil, fmt.Errorf("failed to query oscillation patterns: %w", err)
    }
    defer rows.Close()
    
    var detections []OscillationDetection
    for rows.Next() {
        var detection OscillationDetection
        err := rows.Scan(
            &detection.PatternCount,
            &detection.ActionType,
            &detection.FirstOccurrence,
            &detection.LastOccurrence,
            &detection.AvgEffectiveness,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan oscillation detection: %w", err)
        }
        detections = append(detections, detection)
    }
    
    return detections, nil
}
```

## 🔄 **Lightweight CRD Integration**

### **Summary CRDs for Kubernetes Visibility**

```yaml
# Lightweight CRD for action history summaries
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: actionhistorysummaries.ai.prometheus-alerts-slm.io
spec:
  group: ai.prometheus-alerts-slm.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              resourceReference:
                type: object
                properties:
                  apiVersion: { type: string }
                  kind: { type: string }
                  name: { type: string }
                  namespace: { type: string }
                  uid: { type: string }
              databaseRecordId:
                type: integer
                description: "Reference to primary database record"
          status:
            type: object
            properties:
              totalActions: { type: integer }
              lastActionTime: { type: string, format: date-time }
              recentEffectiveness: { type: number }
              activePatterns:
                type: array
                items:
                  type: object
                  properties:
                    patternType: { type: string }
                    severity: { type: string }
                    lastDetected: { type: string, format: date-time }
              quickStats:
                type: object
                properties:
                  last24h: { type: integer }
                  last7d: { type: integer }
                  successRate: { type: number }
  scope: Namespaced
  names:
    plural: actionhistorysummaries
    singular: actionhistorysummary
    kind: ActionHistorySummary
    shortNames: ["ahs"]
```

### **CRD Sync Controller**

```go
// Controller to sync database state to CRDs
type DatabaseCRDSyncController struct {
    client.Client
    actionRepo ActionHistoryRepository
    syncInterval time.Duration
}

func (c *DatabaseCRDSyncController) syncActionHistorySummaries(ctx context.Context) error {
    // Get all action histories that need CRD updates
    summaries, err := c.actionRepo.GetActionHistorySummaries(ctx, time.Hour)
    if err != nil {
        return fmt.Errorf("failed to get action history summaries: %w", err)
    }
    
    for _, summary := range summaries {
        // Create or update CRD
        crd := &aipariv1.ActionHistorySummary{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-%s", summary.ResourceRef.Kind, summary.ResourceRef.Name),
                Namespace: summary.ResourceRef.Namespace,
                Labels: map[string]string{
                    "ai.prometheus-alerts-slm.io/resource-kind": summary.ResourceRef.Kind,
                    "ai.prometheus-alerts-slm.io/managed-by":    "database-sync",
                },
            },
            Spec: aipariv1.ActionHistorySummarySpec{
                ResourceReference: summary.ResourceRef,
                DatabaseRecordId:  summary.DatabaseID,
            },
            Status: aipariv1.ActionHistorySummaryStatus{
                TotalActions:        summary.TotalActions,
                LastActionTime:      metav1.NewTime(summary.LastActionTime),
                RecentEffectiveness: summary.RecentEffectiveness,
                ActivePatterns:      summary.ActivePatterns,
                QuickStats:          summary.QuickStats,
            },
        }
        
        if err := c.CreateOrUpdate(ctx, crd); err != nil {
            return fmt.Errorf("failed to sync CRD for %s: %w", summary.ResourceRef.Name, err)
        }
    }
    
    return nil
}
```

## 📊 **MCP Server Integration**

### **Database-Backed MCP Server**

```go
// MCP Server with database backend
type DatabaseActionHistoryMCPServer struct {
    actionRepo ActionHistoryRepository
    tools      []MCPTool
}

func (server *DatabaseActionHistoryMCPServer) CheckActionHistory(params map[string]interface{}) (interface{}, error) {
    namespace := params["namespace"].(string)
    resourceKind := params["resource_kind"].(string)
    resourceName := params["resource_name"].(string)
    timeRange := params["time_range"].(string)
    
    duration, err := time.ParseDuration(timeRange)
    if err != nil {
        return nil, fmt.Errorf("invalid time range: %w", err)
    }
    
    resourceRef := ResourceReference{
        Namespace: namespace,
        Kind:      resourceKind,
        Name:      resourceName,
    }
    
    actions, err := server.actionRepo.GetActionHistory(context.Background(), resourceRef, TimeRange{
        Start: time.Now().Add(-duration),
        End:   time.Now(),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to query action history: %w", err)
    }
    
    // Perform real-time oscillation analysis
    patterns, err := server.actionRepo.FindOscillationPatterns(context.Background(), resourceRef, duration)
    if err != nil {
        return nil, fmt.Errorf("failed to detect patterns: %w", err)
    }
    
    // Get effectiveness metrics
    effectiveness, err := server.actionRepo.GetEffectivenessMetrics(context.Background(), EffectivenessScope{
        Type:  "resource",
        Value: fmt.Sprintf("%s/%s/%s", namespace, resourceKind, resourceName),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get effectiveness metrics: %w", err)
    }
    
    return map[string]interface{}{
        "resource": map[string]string{
            "namespace": namespace,
            "kind":      resourceKind,
            "name":      resourceName,
        },
        "time_range": map[string]interface{}{
            "duration": timeRange,
            "start":    time.Now().Add(-duration),
            "end":      time.Now(),
        },
        "total_actions":     len(actions),
        "actions":          formatActionsForAI(actions),
        "patterns":         patterns,
        "effectiveness":    effectiveness,
        "recommendations":  generateRecommendations(actions, patterns, effectiveness),
    }, nil
}
```

## 🎯 **Benefits of Database-First Approach**

### **Scalability Benefits**
✅ **Million+ Records**: PostgreSQL handles millions of action records efficiently  
✅ **ETCD Protection**: No pressure on Kubernetes ETCD from action history data  
✅ **Partitioning**: Automatic monthly partitioning for optimal query performance  
✅ **Archival**: Efficient data archival and purging strategies  

### **Performance Benefits**  
✅ **Complex Queries**: Advanced SQL for pattern detection and analytics  
✅ **Indexing**: Optimized indexes for all query patterns  
✅ **Aggregations**: Fast effectiveness calculations and trend analysis  
✅ **Concurrent Access**: High-performance concurrent reads and writes  

### **Operational Benefits**
✅ **Backup/Restore**: Standard database backup and recovery procedures  
✅ **Monitoring**: Existing database monitoring and alerting tools  
✅ **Business Intelligence**: Direct integration with BI tools and reporting  
✅ **Data Export**: Easy data export for compliance and analysis  

### **Development Benefits**
✅ **SQL Familiarity**: Standard SQL for complex queries and analytics  
✅ **ORM Support**: Native support for Go ORMs (GORM, Ent, etc.)  
✅ **Testing**: Standard database testing patterns and tools  
✅ **Migration**: Database schema migrations with tools like Migrate  

This database-first approach provides enterprise-scale storage while maintaining Kubernetes-native visibility through lightweight CRDs, giving us the best of both worlds: scalability and Kubernetes integration.

## 🔄 **Next Steps**

1. **Implement database schema** with migrations and indexes
2. **Create repository layer** with PostgreSQL implementation  
3. **Build oscillation detection algorithms** using SQL and Go
4. **Develop lightweight CRD sync** for Kubernetes visibility
5. **Integrate MCP server** with database backend

Ready to proceed with oscillation detection algorithms that will work with this database backend?