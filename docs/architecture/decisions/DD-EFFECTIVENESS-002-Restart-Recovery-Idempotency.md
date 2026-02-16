# DD-EFFECTIVENESS-002: Effectiveness Monitor Restart Recovery & Idempotency

**Date**: October 16, 2025
**Status**: ✅ APPROVED
**Decision Makers**: Architecture Team
**Priority**: CRITICAL
**Related**: DD-EFFECTIVENESS-001 (Hybrid Approach)

**Note**: Per DD-017 v2.0, this DD applies to EM Level 1 in V1.0 scope.

---

## 🎯 **Context and Problem Statement**

The Effectiveness Monitor service must reliably track and assess completed WorkflowExecution CRDs to evaluate remediation effectiveness. Critical operational challenges:

1. **Restart Recovery**: How does the service recover after crash/restart and know which workflows were already assessed?
2. **Duplicate Prevention**: How does it prevent assessing the same workflow multiple times?
3. **Missed Assessments**: How does it catch workflows that completed while the service was down?
4. **Race Conditions**: How does it handle concurrent processing with multiple replicas (HA deployment)?
5. **State Persistence**: Where is the "already assessed" state stored?

**Without a robust solution**:
- ❌ Duplicate assessments waste resources and skew metrics
- ❌ Missed assessments create learning gaps in the AI system
- ❌ Inconsistent data undermines effectiveness tracking
- ❌ Race conditions cause duplicate work and database conflicts

---

## 🔍 **Decision Drivers**

### Functional Requirements
- **Idempotency**: Each WorkflowExecution must be assessed exactly once
- **Restart Recovery**: Service must automatically resume after restart without manual intervention
- **Missed Assessment Recovery**: Service must catch up on workflows that completed during downtime
- **Race Condition Safety**: Multiple replicas must not create duplicate assessments

### Operational Requirements
- **Zero Manual Intervention**: Fully automated recovery
- **Audit Trail**: Complete history of which workflows were assessed and when
- **Observable**: Clear metrics for duplicate prevention and recovery events
- **Scalable**: Support for multiple replicas (HA deployment)

### Technical Requirements
- **Stateless Service Design**: No local state files or in-memory tracking
- **Database as Source of Truth**: Leverage existing PostgreSQL for state persistence
- **Kubernetes-Native**: Use standard controller patterns and watch API
- **Atomic Operations**: Prevent race conditions at database level

---

## 🎨 **Considered Options**

### Option 1: Local State File (In-Memory + Disk Persistence)

**Approach**: Store assessed WorkflowExecution UIDs in local file/cache

```go
type AssessmentCache struct {
    assessedUIDs map[string]bool
    filePath     string
}

func (c *AssessmentCache) Load() error {
    // Load from disk on startup
}

func (c *AssessmentCache) Save() error {
    // Persist to disk periodically
}
```

**Pros**:
- ✅ Fast lookup (in-memory)
- ✅ Simple implementation

**Cons**:
- ❌ State lost if file corrupted
- ❌ No shared state across multiple replicas
- ❌ Race conditions between replicas
- ❌ No audit trail
- ❌ Manual cleanup required
- ❌ Not cloud-native (violates stateless principle)

**Decision**: ❌ REJECTED - Not suitable for HA deployment

---

### Option 2: Kubernetes ConfigMap/Secret for State

**Approach**: Store assessed UIDs in ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: effectiveness-assessment-state
data:
  assessed-uids: "wf-001-uid,wf-002-uid,wf-003-uid,..."
```

**Pros**:
- ✅ Shared across replicas
- ✅ Kubernetes-native

**Cons**:
- ❌ ConfigMap size limit (1MB)
- ❌ No atomic updates (race conditions)
- ❌ Poor performance (full read/write on every check)
- ❌ No query capability (can't analyze trends)
- ❌ No automatic cleanup

**Decision**: ❌ REJECTED - Scalability and performance issues

---

### Option 3: Custom CRD for Assessment State

**Approach**: Create `EffectivenessAssessment` CRD

```yaml
apiVersion: effectiveness.kubernaut.io/v1alpha1
kind: EffectivenessAssessment
metadata:
  name: wf-001-assessment
spec:
  workflowUID: wf-001-uid
  actionType: restart-pod
status:
  phase: completed
  score: 0.95
  assessedAt: 2025-10-16T10:05:00Z
```

**Pros**:
- ✅ Kubernetes-native
- ✅ Automatic lifecycle management
- ✅ Watch-based coordination

**Cons**:
- ❌ CRDs designed for orchestration, not historical data storage
- ❌ Manual cleanup required (24h TTL?)
- ❌ Poor query capability (no SQL)
- ❌ No long-term audit trail (7+ years)
- ❌ Extra complexity (another CRD to manage)
- ❌ Not designed for ML training data

**Decision**: ❌ REJECTED - CRDs are for orchestration, not data persistence

---

### Option 4: Database-Backed Idempotency (CHOSEN) ✅

**Approach**: Use PostgreSQL with WorkflowExecution.UID as idempotency key

```sql
CREATE TABLE effectiveness_results (
    id UUID PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL UNIQUE,  -- WorkflowExecution.UID
    action_type VARCHAR(100) NOT NULL,
    overall_score FLOAT NOT NULL,
    assessed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    -- ... other fields ...
);
```

**Reconciliation Loop**:
```go
func (r *EffectivenessMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    var wf workflowv1.WorkflowExecution
    r.Get(ctx, req.NamespacedName, &wf)

    // Only process completed/failed
    if wf.Status.Phase != "completed" && wf.Status.Phase != "failed" {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    // Idempotency check
    traceID := string(wf.UID)
    alreadyAssessed, _ := r.db.IsEffectivenessAssessed(ctx, traceID)
    
    if alreadyAssessed {
        return ctrl.Result{}, nil // Skip (idempotent)
    }

    // Atomic insert (prevents race conditions)
    created, _ := r.db.CreateAssessmentIfNotExists(ctx, traceID)
    if !created {
        return ctrl.Result{}, nil // Another replica already processing
    }

    // Perform assessment
    r.performAssessment(ctx, &wf)
    return ctrl.Result{}, nil
}
```

**Pros**:
- ✅ **Idempotency**: UNIQUE constraint on trace_id
- ✅ **Atomic Operations**: INSERT ... ON CONFLICT DO NOTHING
- ✅ **Shared State**: All replicas see same database
- ✅ **Restart Recovery**: Kubernetes Watch API resync + database check
- ✅ **Audit Trail**: Complete history in PostgreSQL
- ✅ **Query Capability**: SQL for trends, ML training data
- ✅ **Long-term Storage**: 7+ years retention
- ✅ **Scalable**: Database handles concurrent access
- ✅ **Observable**: Prometheus metrics for idempotency events

**Cons**:
- ⚠️ Database dependency (mitigated: exponential backoff retry)
- ⚠️ Slightly slower than in-memory (mitigated: indexed queries <10ms)

**Decision**: ✅ **CHOSEN** - Best balance of reliability, scalability, and operational simplicity

---

## 📐 **Decision Details**

### Architecture Components

#### 1. Idempotency Key: WorkflowExecution.UID

**Why UID is Perfect**:

```go
type UID string // Example: "abc-123-def-456-ghi-789-jkl-012"

// Properties:
// ✅ Globally unique across ALL Kubernetes clusters
// ✅ Immutable (never changes for resource lifetime)
// ✅ Generated by Kubernetes API server (no collision risk)
// ✅ Available immediately after CRD creation
// ✅ Survives status updates and spec modifications
```

**Alternatives Considered**:
- ❌ `WorkflowExecution.Name` - Not globally unique (namespace-scoped)
- ❌ `Spec.ActionID` - User-provided (collision risk)
- ❌ Database auto-increment - Doesn't link to CRD
- ❌ Timestamp-based - Race condition risk

---

#### 2. Database Schema

**Table 1: `effectiveness_results`** (Final Assessment Results)

```sql
CREATE TABLE effectiveness_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL UNIQUE,  -- ← IDEMPOTENCY KEY
    action_type VARCHAR(100) NOT NULL,
    overall_score FLOAT NOT NULL CHECK (overall_score >= 0 AND overall_score <= 1),
    alert_resolved BOOLEAN NOT NULL,
    metric_delta JSONB,
    side_effects INTEGER DEFAULT 0,
    confidence FLOAT NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    assessed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    recommended_adjustments JSONB,
    learning_contribution FLOAT NOT NULL DEFAULT 0.5,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_effectiveness_results_trace_id ON effectiveness_results(trace_id);
```

**Purpose**: Stores final assessment results with UNIQUE constraint for idempotency

---

**Table 2: `action_assessments`** (Pending Assessment Queue)

```sql
CREATE TABLE action_assessments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(255) NOT NULL,  -- NOT UNIQUE (allows retries)
    action_type VARCHAR(100) NOT NULL,
    context_hash VARCHAR(64) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255) NOT NULL,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    scheduled_for TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW() + INTERVAL '5 minutes',
    status VARCHAR(50) NOT NULL DEFAULT 'pending' 
        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_action_assessments_trace_id ON action_assessments(trace_id);
CREATE INDEX idx_action_assessments_status_scheduled 
    ON action_assessments(status, scheduled_for) 
    WHERE status = 'pending';
```

**Purpose**: Tracks assessment lifecycle and enables retry logic

**State Transitions**:
- `pending` → Waiting for scheduled time (+5 minutes for stabilization)
- `processing` → Currently being assessed
- `completed` → Assessment finished, result stored in effectiveness_results
- `failed` → Assessment failed (will retry)

---

#### 3. Atomic Database Operations

**Idempotent Assessment Check**:

```go
func (s *EffectivenessStorage) IsEffectivenessAssessed(ctx context.Context, traceID string) (bool, error) {
    var exists bool
    err := s.db.QueryRowContext(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM effectiveness_results 
            WHERE trace_id = $1
        )
    `, traceID).Scan(&exists)
    
    return exists, err
}
```

**Atomic Assessment Creation** (Race Condition Prevention):

```go
func (s *EffectivenessStorage) CreateAssessmentIfNotExists(
    ctx context.Context, 
    assessment *ActionAssessment,
) (created bool, err error) {
    result, err := s.db.ExecContext(ctx, `
        INSERT INTO action_assessments (
            trace_id, action_type, context_hash, alert_name,
            namespace, resource_name, executed_at, scheduled_for, status
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
        ON CONFLICT (trace_id) DO NOTHING
    `, assessment.TraceID, assessment.ActionType, assessment.ContextHash,
       assessment.AlertName, assessment.Namespace, assessment.ResourceName,
       assessment.ExecutedAt, assessment.ScheduledFor)
    
    if err != nil {
        return false, err
    }

    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return false, err
    }

    created = rowsAffected > 0
    return created, nil
}
```

**Idempotent Result Storage**:

```go
func (s *EffectivenessStorage) StoreEffectivenessResults(
    ctx context.Context, 
    result *EffectivenessResult,
) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO effectiveness_results (
            trace_id, action_type, overall_score, alert_resolved,
            metric_delta, side_effects, confidence, assessed_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        ON CONFLICT (trace_id) DO UPDATE SET
            overall_score = EXCLUDED.overall_score,
            alert_resolved = EXCLUDED.alert_resolved,
            metric_delta = EXCLUDED.metric_delta,
            assessed_at = NOW()
    `, result.TraceID, result.ActionType, result.OverallScore, 
       result.AlertResolved, result.MetricDelta, result.SideEffects, 
       result.Confidence)
    
    return err
}
```

---

#### 4. Kubernetes Watch API Integration

**Controller Setup**:

```go
func (r *EffectivenessMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowv1.WorkflowExecution{}).
        WithOptions(controller.Options{
            MaxConcurrentReconciles: 10,
        }).
        Complete(r)
}
```

**Automatic Resync Behavior**:

1. **On Startup (Informer Cache Sync)**:
   - Controller connects to Kubernetes API
   - Performs **LIST** operation: `kubectl get workflowexecutions --all-namespaces`
   - Loads ALL existing WorkflowExecution CRDs into cache
   - Triggers Reconcile() for EVERY CRD
   - Database idempotency check prevents re-assessment

2. **During Normal Operation (Watch Events)**:
   - Controller watches for ADDED/MODIFIED/DELETED events
   - Only triggers Reconcile() for changed CRDs
   - Efficient incremental processing

3. **Periodic Resync (Default: 10 hours)**:
   - Full LIST operation repeated periodically
   - Catches any missed watch events (rare)
   - Database idempotency ensures no duplicates

---

## 🔄 **Operational Flows**

### Flow 1: Normal Operation (No Restart)

```
1. WorkflowExecution "wf-001" completes (phase → "completed")
2. Kubernetes watch event triggers Reconcile(wf-001)
3. Check: wf.Status.Phase == "completed" ✅
4. Database check: IsEffectivenessAssessed("wf-001-uid") → FALSE
5. CreateAssessmentIfNotExists("wf-001-uid") → SUCCESS (created)
6. Wait 5 minutes for system stabilization
7. Perform effectiveness assessment
8. Store in effectiveness_results (trace_id: "wf-001-uid")
9. Update action_assessment (status: "completed")
✅ Assessment complete
```

---

### Flow 2: Restart After Assessment Complete

```
1. WorkflowExecution "wf-002" was assessed yesterday at 09:00 AM
2. Effectiveness Monitor crashes at 10:00 AM (unrelated workflow)
3. Effectiveness Monitor restarts at 10:05 AM
4. Controller starts watch on WorkflowExecution CRDs
5. Kubernetes API sends EXISTING "wf-002" in watch resync
6. Reconcile("wf-002") triggered
7. Check: wf.Status.Phase == "completed" ✅
8. Database check: IsEffectivenessAssessed("wf-002-uid") → TRUE
9. ✅ Skip assessment (already done) - NO duplicate
```

**Result**: Idempotent operation - no duplicate assessment

---

### Flow 3: Restart During Assessment (In-Progress)

```
Timeline:
- 10:00 AM: WorkflowExecution "wf-003" completes
- 10:00 AM: Assessment scheduled for 10:05 AM
- 10:03 AM: Effectiveness Monitor crashes
- 10:04 AM: Effectiveness Monitor restarts

Recovery:
1. Controller starts watch on WorkflowExecution CRDs
2. Reconcile("wf-003") triggered
3. Check: wf.Status.Phase == "completed" ✅
4. Database check: IsEffectivenessAssessed("wf-003-uid") → FALSE
5. Database check: action_assessments WHERE trace_id = "wf-003-uid"
   → EXISTS with status = "pending", scheduled_for = 10:05 AM
6. CreateAssessmentIfNotExists("wf-003-uid") → NOT CREATED (already exists)
7. Assessment worker picks up pending assessment at 10:05 AM
8. ✅ Assessment resumes from scheduled time
```

**Result**: Assessment continues without duplication or loss

---

### Flow 4: Missed Assessments (Extended Downtime)

```
Timeline:
- 09:00 AM: Effectiveness Monitor goes down
- 09:15 AM: WorkflowExecution "wf-004" completes → NOT ASSESSED
- 09:30 AM: WorkflowExecution "wf-005" completes → NOT ASSESSED
- 09:45 AM: WorkflowExecution "wf-006" completes → NOT ASSESSED
- 10:00 AM: Effectiveness Monitor restarts

Recovery:
1. Controller starts watch with LIST operation
2. Kubernetes returns ALL WorkflowExecution CRDs (informer resync)
3. Reconcile("wf-004"), Reconcile("wf-005"), Reconcile("wf-006") triggered
4. For each workflow:
   - Check: wf.Status.Phase == "completed" ✅
   - Database check: IsEffectivenessAssessed() → FALSE
   - CreateAssessmentIfNotExists() → SUCCESS
   - Schedule assessment (+5 minutes from now)
5. ✅ All missed workflows are assessed
```

**Result**: Automatic catch-up for all missed WorkflowExecutions

---

### Flow 5: Race Condition (Multiple Replicas - HA)

```
Environment: 2 Effectiveness Monitor replicas (HA deployment)

1. WorkflowExecution "wf-007" completes
2. Both replicas receive watch event simultaneously
3. Replica A: Reconcile("wf-007") starts
4. Replica B: Reconcile("wf-007") starts (race)
5. Replica A: IsEffectivenessAssessed("wf-007-uid") → FALSE
6. Replica B: IsEffectivenessAssessed("wf-007-uid") → FALSE (race window)
7. Replica A: CreateAssessmentIfNotExists("wf-007-uid") → SUCCESS (first)
8. Replica B: CreateAssessmentIfNotExists("wf-007-uid") → NOT CREATED (conflict)
   - PostgreSQL UNIQUE constraint prevents duplicate
9. Replica A performs assessment
10. Replica B skips (detects duplicate in CreateAssessmentIfNotExists result)
11. ✅ Only ONE assessment performed
```

**Result**: Database UNIQUE constraint prevents duplicate assessments

---

## 🛡️ **Edge Case Handling**

### Edge Case 1: CRD Deleted Before Assessment

**Scenario**:
```
1. WorkflowExecution "wf-008" completes
2. Assessment scheduled for +5 minutes
3. User manually deletes wf-008 CRD (kubectl delete)
4. Assessment worker tries to assess at scheduled time
5. Controller tries to fetch CRD → NotFound error
6. Assessment marked as "failed" with reason: "CRD deleted"
```

**Impact**: ⚠️ Assessment lost (acceptable - user explicitly deleted workflow)

**Mitigation**: Document that manual CRD deletion skips assessment

---

### Edge Case 2: Database Temporarily Unavailable

**Scenario**:
```
1. WorkflowExecution "wf-009" completes
2. Database connection fails during IsEffectivenessAssessed() check
3. Reconcile() returns error with requeue
4. Controller retries after 5 minutes (exponential backoff)
5. Database recovers
6. Retry succeeds, assessment proceeds normally
```

**Impact**: ⏱️ Delayed assessment (NOT lost)

**Mitigation**: Exponential backoff + Prometheus metrics for DB failures

**Retry Schedule**:
- Attempt 1: Immediate
- Attempt 2: 5 minutes
- Attempt 3: 10 minutes
- Attempt 4: 20 minutes
- Attempt 5+: 30 minutes (max)

---

### Edge Case 3: Stale "processing" Assessments

**Scenario**:
```
1. Assessment starts for "wf-010"
2. action_assessments status updated: "pending" → "processing"
3. Controller crashes during assessment
4. Controller restarts
5. Database query finds: trace_id="wf-010-uid", status="processing"
6. Stale check: (created_at + 30min) < NOW() → TRUE
7. Reset status: "processing" → "pending"
8. Assessment retried by worker
```

**Implementation**:

```go
// Background goroutine: Reset stale "processing" assessments
func (r *EffectivenessMonitorReconciler) resetStaleProcessing(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    for {
        select {
        case <-ticker.C:
            _, err := r.db.ExecContext(ctx, `
                UPDATE action_assessments
                SET status = 'pending', completed_at = NULL
                WHERE status = 'processing' 
                  AND created_at < NOW() - INTERVAL '30 minutes'
            `)
            if err != nil {
                r.logger.Error("Failed to reset stale assessments", zap.Error(err))
            }
        case <-ctx.Done():
            return
        }
    }
}
```

**Impact**: ✅ Automatic recovery from crashes during assessment

---

### Edge Case 4: Very Rapid CRD Deletion

**Scenario**:
```
1. WorkflowExecution "wf-011" completes at 10:00:00
2. Assessment scheduled for 10:05:00
3. User deletes CRD at 10:00:01 (before assessment record created)
4. Reconcile() triggered
5. CRD fetch fails → NotFound
6. No assessment record created
```

**Impact**: ⚠️ No assessment (acceptable - CRD deleted immediately)

**Mitigation**: Document expected behavior in operational procedures

---

## 📊 **Monitoring & Observability**

### Prometheus Metrics

```go
var (
    // Idempotency tracking
    assessmentsDuplicateSkipped = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "effectiveness_assessments_duplicate_skipped_total",
            Help: "Total assessments skipped due to existing results (idempotency)",
        },
        []string{"action_type"},
    )

    // Restart recovery
    assessmentsRecovered = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "effectiveness_assessments_recovered_total",
            Help: "Total assessments recovered after restart",
        },
        []string{"action_type"},
    )

    // Database operations
    databaseIdempotencyCheckDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name: "effectiveness_database_idempotency_check_duration_seconds",
            Help: "Time spent checking database for existing assessments",
            Buckets: prometheus.DefBuckets,
        },
    )

    // Race condition detection
    assessmentsRaceConditionDetected = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "effectiveness_assessments_race_condition_detected_total",
            Help: "Total race conditions detected and prevented",
        },
    )

    // Stale assessment cleanup
    assessmentsStaleReset = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "effectiveness_assessments_stale_reset_total",
            Help: "Total stale processing assessments reset to pending",
        },
    )
)
```

### Alert Rules

```yaml
groups:
  - name: effectiveness_monitor_idempotency
    rules:
      - alert: HighDuplicateSkipRate
        expr: rate(effectiveness_assessments_duplicate_skipped_total[5m]) > 10
        for: 10m
        annotations:
          summary: "High duplicate assessment skip rate detected"
          description: "{{ $value }} assessments/sec being skipped (idempotency)"

      - alert: FrequentStaleAssessments
        expr: rate(effectiveness_assessments_stale_reset_total[5m]) > 5
        for: 10m
        annotations:
          summary: "Frequent stale assessment resets"
          description: "Controller may be crashing during assessments"

      - alert: DatabaseIdempotencyCheckSlow
        expr: histogram_quantile(0.95, effectiveness_database_idempotency_check_duration_seconds_bucket) > 0.1
        for: 5m
        annotations:
          summary: "Slow database idempotency checks"
          description: "p95 latency: {{ $value }}s (expected <0.1s)"
```

---

## 🎯 **Consequences**

### Positive Consequences

✅ **Operational Simplicity**
- Zero manual intervention required for restart recovery
- Automatic catch-up for missed assessments
- No state files to manage or corrupt

✅ **Reliability**
- Idempotent operations prevent duplicate assessments
- Database UNIQUE constraint eliminates race conditions
- Kubernetes Watch API handles resync automatically

✅ **Scalability**
- Supports multiple replicas (HA deployment)
- Database handles concurrent access
- No coordination required between replicas

✅ **Audit Trail**
- Complete history in PostgreSQL
- 7+ years retention for compliance
- SQL queries for trend analysis

✅ **Observable**
- Prometheus metrics for all idempotency events
- Alert rules for anomaly detection
- Clear logging for troubleshooting

---

### Negative Consequences

⚠️ **Database Dependency**
- Service requires database availability for idempotency checks
- **Mitigation**: Exponential backoff retry, database HA deployment

⚠️ **Slightly Slower Than In-Memory**
- Database query adds ~5-10ms per assessment check
- **Mitigation**: Indexed queries, connection pooling, acceptable latency

⚠️ **Stale Assessment Cleanup Required**
- Background goroutine must reset stale "processing" assessments
- **Mitigation**: Simple 30-minute timeout, runs every 5 minutes

⚠️ **CRD Deletion Edge Case**
- Manual CRD deletion before assessment loses effectiveness data
- **Mitigation**: Document expected behavior, rare scenario

---

## 📝 **Implementation Requirements**

### Required Components

1. **Database Schema** (Already Exists)
   - Migration 006: effectiveness_results, action_assessments tables
   - UNIQUE constraint on trace_id

2. **Storage Layer** (pkg/storage/effectiveness_storage.go)
   - `IsEffectivenessAssessed(traceID)`
   - `CreateAssessmentIfNotExists(assessment)`
   - `StoreEffectivenessResults(result)`

3. **Controller** (pkg/monitor/effectiveness_monitor_controller.go)
   - Reconcile loop with idempotency checks
   - Kubernetes Watch API integration
   - Background stale assessment cleanup goroutine

4. **Monitoring** (pkg/monitor/metrics.go)
   - Prometheus metrics for idempotency events
   - Alert rules for anomaly detection

---

### Testing Requirements

#### Unit Tests
- Database idempotency operations
- Atomic INSERT ... ON CONFLICT logic
- Stale assessment reset logic

#### Integration Tests
- Restart recovery scenario (mock restart)
- Missed assessment catch-up (simulate downtime)
- Race condition prevention (concurrent reconciles)

#### E2E Tests
- Complete restart recovery flow
- Multi-replica HA deployment
- Database failure and recovery

---

## 📚 **References**

### Internal Documentation
- `/holmesgpt-api/docs/EFFECTIVENESS_MONITOR_RESTART_RECOVERY.md` - Complete technical details
- `/holmesgpt-api/docs/EFFECTIVENESS_MONITOR_CRD_DESIGN_ASSESSMENT.md` - CRD vs Database comparison
- `/docs/architecture/EFFECTIVENESS_MONITOR_RESTART_RECOVERY_FLOWS.md` - Use case flows
- `DD-EFFECTIVENESS-001` - Hybrid approach (automated + selective AI)

### External References
- Kubernetes controller-runtime documentation: Watch API, informer resync
- PostgreSQL documentation: UNIQUE constraints, ON CONFLICT clause
- Go best practices: Exponential backoff, context cancellation

---

## ✅ **Approval and Next Steps**

### Approved By
- Architecture Team: October 16, 2025
- Status: ✅ APPROVED

### Next Steps
1. ✅ Document decision (this document)
2. ✅ Create architecture flow document
3. ⏸️ Implement storage layer functions
4. ⏸️ Implement controller reconciliation loop
5. ⏸️ Add Prometheus metrics
6. ⏸️ Write unit and integration tests
7. ⏸️ Update operational procedures

---

**Document Maintainer**: Kubernaut Architecture Team  
**Last Updated**: October 16, 2025  
**Status**: ✅ APPROVED - Ready for Implementation


