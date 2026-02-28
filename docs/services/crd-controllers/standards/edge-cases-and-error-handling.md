# CRD Controllers - Edge Cases & Error Handling Guide

**Version**: 1.0
**Date**: 2025-10-13
**Applies To**: Remediation Processor, Workflow Execution, Kubernetes Executor
**Purpose**: Document edge cases and error handling strategies for production resilience

---

## 📋 Table of Contents

1. [Common Edge Cases (All Controllers)](#common-edge-cases-all-controllers)
2. [Remediation Processor Edge Cases](#remediation-processor-edge-cases)
3. [Workflow Execution Edge Cases](#workflow-execution-edge-cases)
4. [Kubernetes Executor Edge Cases](#kubernetes-executor-edge-cases)
5. [Testing Edge Cases](#testing-edge-cases)

---

## 🌐 Common Edge Cases (All Controllers)

### 1. Leader Election Failures

**Scenario**: Multiple replicas fail to elect a leader

**Causes**:
- Network partition between replicas
- etcd/API server unavailable
- Lease duration expired during reconciliation

**Detection**:
```go
// Monitor lease acquisition failures
if err := mgr.Start(ctx); err != nil {
    if strings.Contains(err.Error(), "failed to acquire lease") {
        log.Error(err, "Leader election failed")
        // Exponential backoff retry
    }
}
```

**Handling Strategy**:
1. **Retry with Exponential Backoff**: 1s, 2s, 4s, 8s, 16s (max)
2. **Health Check Failure**: Mark pod as not ready if cannot acquire lease after 5 attempts
3. **Alert**: Trigger alert if no leader elected for >60s
4. **Graceful Degradation**: Stop reconciliation, maintain current state

**Configuration**:
```yaml
--leader-election-lease-duration=15s
--leader-election-renew-deadline=10s
--leader-election-retry-period=2s
```

**Test Coverage**:
```go
// test/integration/common/leader_election_test.go
It("should handle network partition during leader election", func() {
    // Simulate network partition
    // Verify exponential backoff
    // Verify health check fails
})
```

---

### 2. CRD Status Update Conflicts

**Scenario**: Concurrent status updates from multiple reconciliations

**Causes**:
- Watch triggers multiple reconciliations for same resource
- Status update race condition between replicas
- etcd conflict on resource version

**Detection**:
```go
err := r.Status().Update(ctx, crd)
if errors.IsConflict(err) {
    log.Info("Status update conflict, retrying", "resourceVersion", crd.ResourceVersion)
    // Handle conflict
}
```

**Handling Strategy**:
1. **Retry with Fresh Read**: Re-fetch resource and retry update
2. **Optimistic Locking**: Use resource version for conflict detection
3. **Exponential Backoff**: 100ms, 200ms, 400ms, 800ms
4. **Max Retries**: 5 attempts before requeue
5. **Conflict Metrics**: Track conflict rate for tuning

**Implementation**:
```go
func (r *Reconciler) updateStatusWithRetry(ctx context.Context, crd client.Object, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        if err := r.Status().Update(ctx, crd); err != nil {
            if errors.IsConflict(err) && i < maxRetries-1 {
                // Re-fetch latest version
                if fetchErr := r.Get(ctx, client.ObjectKeyFromObject(crd), crd); fetchErr != nil {
                    return fetchErr
                }
                // Exponential backoff
                time.Sleep(time.Duration(100*math.Pow(2, float64(i))) * time.Millisecond)
                continue
            }
            return err
        }
        return nil
    }
    return fmt.Errorf("max retries exceeded for status update")
}
```

**Test Coverage**:
```go
It("should handle concurrent status updates with optimistic locking", func() {
    // Create resource
    // Trigger concurrent updates from 2 goroutines
    // Verify one succeeds, one retries
    // Verify final state is consistent
})
```

---

### 3. Watch Connection Loss

**Scenario**: Kubernetes watch connection drops unexpectedly

**Causes**:
- API server restart
- Network timeout
- etcd disruption
- Long-running reconciliation causing watch timeout

**Detection**:
```go
// controller-runtime automatically handles reconnection
// Monitor watch errors in logs
log.V(1).Info("Watch connection lost, reconnecting", "resource", resource)
```

**Handling Strategy**:
1. **Automatic Reconnection**: controller-runtime handles reconnection automatically
2. **Full Reconciliation**: Trigger full list/reconcile after reconnection
3. **Event Buffer**: Use work queue to buffer events during disconnection
4. **Metrics**: Track watch reconnection frequency
5. **Alert**: Alert if reconnection frequency > 10/hour

**Configuration**:
```go
ctrl.NewControllerManagedBy(mgr).
    For(&v1alpha1.Resource{}).
    WithOptions(controller.Options{
        MaxConcurrentReconciles: 1,  // Prevent concurrent reconciliation storms
    }).
    Complete(r)
```

**Test Coverage**:
```go
It("should recover from watch connection loss", func() {
    // Create resource
    // Simulate API server unavailability
    // Verify watch reconnects
    // Verify no events lost
})
```

---

### 4. Controller Restart During Reconciliation

**Scenario**: Controller pod crashes or restarts mid-reconciliation

**Causes**:
- OOMKilled
- Node failure
- Rolling update
- Manual pod deletion

**Handling Strategy**:
1. **Idempotent Operations**: All reconciliation steps must be idempotent
2. **State Recovery**: Rely on CRD status to determine current state
3. **Requeue**: Re-reconcile from last known state
4. **Graceful Shutdown**: Handle SIGTERM to complete in-flight reconciliation
5. **Status Persistence**: Update status after each significant step

**Implementation**:
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Always fetch fresh state
    var crd v1alpha1.Resource
    if err := r.Get(ctx, req.NamespacedName, &crd); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Resume from current phase (idempotent)
    switch crd.Status.Phase {
    case "":
        return r.handlePending(ctx, &crd)
    case "Processing":
        return r.handleProcessing(ctx, &crd)  // Safe to retry
    case "Completed":
        return ctrl.Result{}, nil  // Already done
    }
}
```

**Graceful Shutdown**:
```go
func main() {
    ctx := ctrl.SetupSignalHandler()

    if err := mgr.Start(ctx); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}

// SetupSignalHandler handles SIGTERM gracefully
// Waits for in-flight reconciliations to complete (up to terminationGracePeriodSeconds)
```

**Test Coverage**:
```go
It("should recover gracefully from controller restart", func() {
    // Create resource
    // Start reconciliation
    // Kill controller pod
    // Wait for new pod
    // Verify reconciliation completes from last state
})
```

---

### 5. API Server Rate Limiting

**Scenario**: Controller hits Kubernetes API rate limits

**Causes**:
- High CRD create/update rate
- Frequent status updates
- Large number of resources
- Aggressive reconciliation loops

**Detection**:
```go
// controller-runtime provides built-in rate limiting
// Monitor 429 (Too Many Requests) errors
if errors.IsRateLimited(err) {
    log.Info("Rate limited by API server, backing off")
}
```

**Handling Strategy**:
1. **Client-Side Rate Limiting**: Configure rate limiter in controller
2. **Exponential Backoff**: Increase backoff on rate limit errors
3. **Batch Operations**: Group related updates
4. **Status Update Optimization**: Update status only when changed
5. **Metrics**: Track rate limit errors

**Configuration**:
```go
cfg, err := ctrl.GetConfig()
cfg.QPS = 20        // Queries per second
cfg.Burst = 30      // Burst capacity

mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    // Rate limiter for work queue
    RateLimiter: workqueue.NewMaxOfRateLimiter(
        workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
        &workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
    ),
})
```

**Test Coverage**:
```go
It("should handle API server rate limiting gracefully", func() {
    // Configure low rate limit
    // Create many resources
    // Verify controller backs off
    // Verify all resources eventually reconciled
})
```

---

## 🔄 Remediation Processor Edge Cases

### 1. PostgreSQL Connection Loss

**Scenario**: Data Storage Service PostgreSQL connection fails during enrichment

**Causes**:
- PostgreSQL pod restart
- Network partition
- Connection pool exhaustion
- Long-running query timeout

**Detection**:
```go
enrichmentContext, err := r.Enricher.EnrichContext(ctx, rp)
if err != nil {
    if strings.Contains(err.Error(), "connection refused") ||
       strings.Contains(err.Error(), "i/o timeout") {
        log.Error(err, "PostgreSQL connection lost")
        // Handle connection loss
    }
}
```

**Handling Strategy**:
1. **Connection Pool**: Use connection pooling with health checks
2. **Retry with Backoff**: 3 retries with 1s, 2s, 4s delays
3. **Circuit Breaker**: Open circuit after 5 consecutive failures
4. **Degraded Mode**: Skip enrichment, proceed with classification using defaults
5. **Metrics**: Track connection failures and circuit breaker state

**Implementation**:
```go
type Enricher struct {
    storageClient storage.Client
    circuitBreaker *CircuitBreaker
    logger *logrus.Logger
}

func (e *Enricher) EnrichContext(ctx context.Context, rp *RemediationProcessing) (*EnrichmentContext, error) {
    // Check circuit breaker
    if !e.circuitBreaker.AllowRequest() {
        log.Warn("Circuit breaker open, using degraded enrichment")
        return e.degradedEnrichment(rp), nil
    }

    // Retry with exponential backoff
    var result *EnrichmentContext
    err := retry.Do(
        func() error {
            var queryErr error
            result, queryErr = e.storageClient.QuerySimilarRemediations(ctx, params)
            return queryErr
        },
        retry.Attempts(3),
        retry.Delay(1*time.Second),
        retry.DelayType(retry.BackOffDelay),
        retry.OnRetry(func(n uint, err error) {
            log.WithField("attempt", n).Warn("Retrying enrichment query")
        }),
    )

    if err != nil {
        e.circuitBreaker.RecordFailure()
        log.Error(err, "Enrichment failed after retries, using degraded mode")
        return e.degradedEnrichment(rp), nil  // Graceful degradation
    }

    e.circuitBreaker.RecordSuccess()
    return result, nil
}

func (e *Enricher) degradedEnrichment(rp *RemediationProcessing) *EnrichmentContext {
    return &EnrichmentContext{
        SimilarRemediationsCount: 0,
        HistoricalSuccessRate:    0.5,  // Conservative default
        AverageResolutionTime:    "unknown",
        CommonRemediationActions: []string{},
        RelatedKnowledgeArticles: []string{},
        DegradedMode:             true,
    }
}
```

**Circuit Breaker Configuration**:
```go
type CircuitBreaker struct {
    maxFailures   int
    resetTimeout  time.Duration
    failureCount  int
    lastFailTime  time.Time
    state         string  // "closed", "open", "half-open"
    mu            sync.Mutex
}

func (cb *CircuitBreaker) AllowRequest() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if cb.state == "open" {
        if time.Since(cb.lastFailTime) > cb.resetTimeout {
            cb.state = "half-open"
            cb.failureCount = 0
            return true
        }
        return false
    }
    return true
}
```

**Test Coverage**:
```go
It("should handle PostgreSQL connection loss with circuit breaker", func() {
    // Start with healthy connection
    // Simulate connection loss
    // Verify 3 retries
    // Verify circuit breaker opens after 5 failures
    // Verify degraded mode enrichment
    // Simulate connection recovery
    // Verify circuit breaker closes
})
```

---

### 2. Semantic Search Timeout

**Scenario**: pgvector semantic search query exceeds timeout

**Causes**:
- Large vector index (millions of embeddings)
- Slow disk I/O
- CPU contention
- Missing index on embedding column

**Detection**:
```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

result, err := e.storageClient.QuerySimilarRemediations(ctx, params)
if err == context.DeadlineExceeded {
    log.Warn("Semantic search timeout, using fallback")
}
```

**Handling Strategy**:
1. **Query Timeout**: 5s default, configurable
2. **Index Optimization**: Ensure IVFFlat index on embedding column
3. **Fallback Query**: Use simple text search without vectors if timeout
4. **Metrics**: Track timeout frequency and query latency
5. **Alert**: Alert if timeout rate > 10%

**Implementation**:
```go
func (c *PostgreSQLClient) QuerySimilarRemediations(ctx context.Context, params QueryParams) (*HistoricalData, error) {
    // Try semantic search with timeout
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    query := `
        SELECT * FROM remediation_audit
        WHERE signal_name = $1 AND severity = $2
        ORDER BY embedding <=> (SELECT embedding FROM remediation_audit WHERE signal_fingerprint = $3 LIMIT 1)
        LIMIT $4
    `

    rows, err := c.db.QueryContext(ctx, query, params.SignalName, params.Severity, params.SignalFingerprint, params.Limit)
    if err == context.DeadlineExceeded {
        log.Warn("Semantic search timeout, falling back to text search")
        return c.fallbackTextSearch(context.Background(), params)
    }
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    return c.parseResults(rows)
}

func (c *PostgreSQLClient) fallbackTextSearch(ctx context.Context, params QueryParams) (*HistoricalData, error) {
    // Simple text-based search without vectors
    query := `
        SELECT * FROM remediation_audit
        WHERE signal_name = $1 AND severity = $2
        ORDER BY created_at DESC
        LIMIT $3
    `
    // ... implementation
}
```

**Test Coverage**:
```go
It("should fallback to text search on semantic search timeout", func() {
    // Configure short timeout (100ms)
    // Create large dataset
    // Verify semantic search times out
    // Verify fallback text search succeeds
    // Verify enrichment completes
})
```

---

### 3. Classification with No Historical Data

**Scenario**: New signal type with zero historical remediations

**Causes**:
- First occurrence of this signal
- Database recently wiped
- New environment/cluster
- Signal name typo

**Detection**:
```go
if rp.Status.EnrichmentContext.SimilarRemediationsCount == 0 {
    log.Info("No historical data found for signal", "signal", rp.Spec.SignalName)
}
```

**Handling Strategy**:
1. **Default to AI Analysis**: Route to AI if no historical data
2. **Conservative Confidence**: Set classification confidence to 0.3 (low)
3. **Explicit Reason**: Document "new signal type" in classification reason
4. **Learning Mode**: Record outcome for future similar signals
5. **Metrics**: Track new signal frequency

**Implementation**:
```go
func (c *Classifier) shouldRequireAI(rp *RemediationProcessing) bool {
    if rp.Status.EnrichmentContext == nil {
        return true  // No context = requires AI
    }

    // No historical data = requires AI
    if rp.Status.EnrichmentContext.SimilarRemediationsCount == 0 {
        log.Info("New signal type detected, routing to AI", "signal", rp.Spec.SignalName)
        return true
    }

    // Low success rate
    if rp.Status.EnrichmentContext.HistoricalSuccessRate < 0.5 {
        return true
    }

    // Few samples (< 3) = requires AI
    if rp.Status.EnrichmentContext.SimilarRemediationsCount < 3 {
        return true
    }

    return false
}

func (c *Classifier) calculateConfidence(rp *RemediationProcessing) float64 {
    if rp.Status.EnrichmentContext == nil || rp.Status.EnrichmentContext.SimilarRemediationsCount == 0 {
        return 0.3  // Low confidence for new signals
    }

    confidence := 0.5  // Base confidence
    // ... rest of calculation
}
```

**Test Coverage**:
```go
It("should route new signal types to AI with low confidence", func() {
    // Create signal with unique name
    // Verify enrichment returns 0 similar results
    // Verify classification requires AI
    // Verify confidence is 0.3
    // Verify reason mentions "new signal type"
})
```

---

### 4. Deduplication Race Conditions

**Scenario**: Two identical alerts arrive simultaneously

**Causes**:
- Multiple Prometheus instances
- Network retry
- Gateway Service race condition

**Detection**:
```go
// Check for duplicate creation
if existingRP, err := r.findExistingByFingerprint(ctx, fingerprint); err == nil && existingRP != nil {
    log.Info("Duplicate alert detected", "fingerprint", fingerprint, "existing", existingRP.Name)
}
```

**Handling Strategy**:
1. **Atomic Check-and-Create**: Use Kubernetes finalizers or labels for coordination
2. **TTL Window**: Only check for duplicates within last 24 hours
3. **Status Update**: Mark duplicate with status="Suppressed"
4. **Metrics**: Track suppression rate
5. **Backlink**: Reference original RemediationProcessing in duplicate

**Implementation**:
```go
func (r *Reconciler) handlePending(ctx context.Context, rp *RemediationProcessing) (ctrl.Result, error) {
    // Generate fingerprint
    fingerprint := r.Fingerprinter.GenerateFingerprint(rp)
    rp.Status.SignalFingerprint = fingerprint

    // Check for duplicates (within TTL window)
    existingRP, err := r.findExistingByFingerprint(ctx, fingerprint, 24*time.Hour)
    if err != nil {
        return ctrl.Result{}, err
    }

    if existingRP != nil && existingRP.Name != rp.Name {
        // Duplicate found
        log.Info("Duplicate signal suppressed",
            "fingerprint", fingerprint,
            "original", existingRP.Name,
            "duplicate", rp.Name)

        rp.Status.Phase = "Suppressed"
        rp.Status.Message = fmt.Sprintf("Duplicate of %s", existingRP.Name)
        rp.Status.OriginalRemediationProcessing = existingRP.Name
        if err := r.Status().Update(ctx, rp); err != nil {
            return ctrl.Result{}, err
        }

        // Increment suppression metric
        suppressionCounter.WithLabelValues(rp.Spec.SignalName).Inc()

        return ctrl.Result{}, nil
    }

    // Not a duplicate, proceed with enrichment
    rp.Status.Phase = "Enriching"
    if err := r.Status().Update(ctx, rp); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}

func (r *Reconciler) findExistingByFingerprint(ctx context.Context, fingerprint string, ttl time.Duration) (*RemediationProcessing, error) {
    var rpList RemediationProcessingList
    if err := r.List(ctx, &rpList); err != nil {
        return nil, err
    }

    cutoff := time.Now().Add(-ttl)
    for _, rp := range rpList.Items {
        if rp.Status.SignalFingerprint == fingerprint &&
           rp.CreationTimestamp.Time.After(cutoff) &&
           rp.Status.Phase != "Suppressed" {
            return &rp, nil
        }
    }

    return nil, nil
}
```

**Test Coverage**:
```go
It("should suppress duplicate alerts within TTL window", func() {
    // Create first alert
    // Wait for fingerprint generation
    // Create identical alert
    // Verify second alert is suppressed
    // Verify backlink to original
    // Wait past TTL window
    // Create identical alert again
    // Verify NOT suppressed (TTL expired)
})
```

---

## 🔀 Workflow Execution Edge Cases

### 1. Circular Dependency Detection

**Scenario**: Workflow definition contains circular step dependencies

**Causes**:
- User error in workflow definition
- Dynamic step injection creating cycle
- Copy-paste error in YAML

**Detection**:
```go
if r.Resolver.hasCycle(graph) {
    log.Error(nil, "Circular dependency detected in workflow")
    return fmt.Errorf("circular dependency detected: step A depends on B, B depends on C, C depends on A")
}
```

**Handling Strategy**:
1. **Fail Fast**: Detect cycle during Planning phase
2. **Clear Error Message**: Identify exact cycle (e.g., "A → B → C → A")
3. **Status Update**: Set phase to "Failed" with reason
4. **Visualization**: Log dependency graph for debugging
5. **Prevention**: Validate workflow definition via webhook (future)

**Implementation**:
```go
func (r *DependencyResolver) ResolveSteps(steps []WorkflowStep) ([]WorkflowStep, error) {
    graph := r.buildDependencyGraph(steps)

    // Detect cycles using DFS
    if cycle := r.findCycle(graph); cycle != nil {
        cycleStr := strings.Join(cycle, " → ")
        return nil, fmt.Errorf("circular dependency detected: %s → %s", cycleStr, cycle[0])
    }

    orderedSteps := r.topologicalSort(steps, graph)
    return orderedSteps, nil
}

func (r *DependencyResolver) findCycle(graph map[string][]string) []string {
    visited := make(map[string]bool)
    recStack := make(map[string]bool)
    parent := make(map[string]string)

    for node := range graph {
        if cycle := r.dfs(node, graph, visited, recStack, parent); cycle != nil {
            return cycle
        }
    }

    return nil
}

func (r *DependencyResolver) dfs(node string, graph map[string][]string, visited, recStack map[string]bool, parent map[string]string) []string {
    if !visited[node] {
        visited[node] = true
        recStack[node] = true

        for _, neighbor := range graph[node] {
            parent[neighbor] = node

            if !visited[neighbor] {
                if cycle := r.dfs(neighbor, graph, visited, recStack, parent); cycle != nil {
                    return cycle
                }
            } else if recStack[neighbor] {
                // Cycle found, reconstruct it
                cycle := []string{neighbor}
                current := node
                for current != neighbor {
                    cycle = append([]string{current}, cycle...)
                    current = parent[current]
                }
                return cycle
            }
        }
    }

    recStack[node] = false
    return nil
}
```

**Error Message Example**:
```
Circular dependency detected: scale-deployment → wait-for-ready → check-metrics → scale-deployment

This creates an infinite loop. Please review your workflow definition and remove the circular dependency.

Dependency graph:
  scale-deployment depends on: []
  wait-for-ready depends on: [scale-deployment]
  check-metrics depends on: [wait-for-ready]
  scale-deployment depends on: [check-metrics]  <-- ERROR: creates cycle
```

**Test Coverage**:
```go
It("should detect and reject circular dependencies", func() {
    // Create workflow with cycle: A → B → C → A
    // Verify Planning phase fails
    // Verify error message identifies cycle
    // Verify workflow phase is "Failed"
    // Verify clear error reason in status
})
```

---

### 2. Step Timeout vs Workflow Timeout

**Scenario**: Individual step timeout vs overall workflow timeout interaction

**Causes**:
- Step takes longer than step timeout
- Sum of step timeouts exceeds workflow timeout
- Long-running validation or dry-run

**Handling Strategy**:
1. **Timeout Hierarchy**: Workflow timeout > sum of step timeouts
2. **Step Timeout**: Per-step timeout (default: 10m, max: 30m)
3. **Workflow Timeout**: Overall workflow timeout (default: 60m)
4. **Timeout Escalation**: Step timeout triggers retry, workflow timeout triggers rollback
5. **Remaining Time**: Calculate remaining workflow time before starting each step

**Implementation**:
```go
func (r *Reconciler) handleExecuting(ctx context.Context, we *WorkflowExecution) (ctrl.Result, error) {
    // Calculate remaining workflow time
    workflowTimeout := we.Spec.Timeout
    if workflowTimeout == 0 {
        workflowTimeout = 60 * time.Minute  // Default
    }

    elapsed := time.Since(we.Status.ExecutionStartTime.Time)
    remaining := workflowTimeout - elapsed

    if remaining <= 0 {
        // Workflow timeout exceeded
        log.Error(nil, "Workflow timeout exceeded",
            "timeout", workflowTimeout,
            "elapsed", elapsed)

        we.Status.Phase = "Failed"
        we.Status.Message = fmt.Sprintf("Workflow timeout exceeded (limit: %s, elapsed: %s)", workflowTimeout, elapsed)
        if err := r.Status().Update(ctx, we); err != nil {
            return ctrl.Result{}, err
        }

        // Trigger rollback if enabled
        if we.Spec.RollbackOnFailure {
            we.Status.Phase = "RollingBack"
            if err := r.Status().Update(ctx, we); err != nil {
                return ctrl.Result{}, err
            }
            return ctrl.Result{Requeue: true}, nil
        }

        return ctrl.Result{}, nil
    }

    // Get ready steps
    readySteps := r.Orchestrator.GetReadySteps(we)

    for _, step := range readySteps {
        // Adjust step timeout to not exceed workflow remaining time
        stepTimeout := step.Timeout
        if stepTimeout == 0 {
            stepTimeout = 10 * time.Minute  // Default
        }

        if stepTimeout > remaining {
            log.Warn("Step timeout exceeds remaining workflow time, adjusting",
                "stepTimeout", stepTimeout,
                "remaining", remaining)
            stepTimeout = remaining
        }

        // Create KubernetesExecution (DEPRECATED - ADR-025) with adjusted timeout
        if err := r.Orchestrator.CreateStepExecution(ctx, we, step, stepTimeout); err != nil {
            log.Error(err, "Failed to create step execution", "step", step.Name)
            continue
        }
    }

    // ... rest of execution logic
}
```

**Test Coverage**:
```go
It("should handle step timeout vs workflow timeout correctly", func() {
    // Create workflow with 30m timeout
    // Add 4 steps with 10m timeout each (total 40m)
    // Verify workflow planning succeeds
    // Verify step timeouts adjusted to fit within workflow timeout
    // Simulate step taking >10m
    // Verify step times out and retries
    // Simulate workflow taking >30m
    // Verify workflow timeout triggers rollback
})
```

---

### 3. Watch-Based Coordination Missed Events

**Scenario**: Controller misses KubernetesExecution (DEPRECATED - ADR-025) completion event

**Causes**:
- Controller restart during step execution
- Watch connection dropped temporarily
- API server event buffer overflow
- Network partition

**Detection**:
```go
// Periodic reconciliation to catch missed events
return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
```

**Handling Strategy**:
1. **Periodic Reconciliation**: Requeue every 30s to check step status
2. **Explicit Status Check**: Query KubernetesExecution (DEPRECATED - ADR-025) directly if watch missed
3. **Event Replay**: Use informer cache to replay missed events
4. **Idempotent Step Creation**: Safe to recreate if not found
5. **Metrics**: Track reconciliation triggers (watch vs periodic)

**Implementation**:
```go
func (r *Reconciler) handleExecuting(ctx context.Context, we *WorkflowExecution) (ctrl.Result, error) {
    // Check step completions (handles missed watch events)
    allCompleted, anyFailed := r.Monitor.CheckStepCompletions(ctx, we)

    if anyFailed {
        we.Status.Phase = "Failed"
        if err := r.Status().Update(ctx, we); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{Requeue: true}, nil
    }

    if allCompleted {
        we.Status.Phase = "Completed"
        if err := r.Status().Update(ctx, we); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // Not complete, requeue for periodic check
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (m *ExecutionMonitor) CheckStepCompletions(ctx context.Context, we *WorkflowExecution) (allCompleted bool, anyFailed bool) {
    // Query all child KubernetesExecution (DEPRECATED - ADR-025) CRDs directly
    var keList KubernetesExecutionList
    if err := m.client.List(ctx, &keList, client.MatchingLabels{
        "workflow": we.Name,
    }); err != nil {
        log.Error(err, "Failed to list KubernetesExecutions (DEPRECATED - ADR-025)")
        return false, false
    }

    // Update workflow status with current step statuses
    stepResults := make(map[string]string)
    for _, ke := range keList.Items {
        stepResults[ke.Labels["step"]] = ke.Status.Phase
    }

    // Analyze results
    completedCount := 0
    failedCount := 0
    for _, phase := range stepResults {
        if phase == "Completed" {
            completedCount++
        } else if phase == "Failed" {
            failedCount++
        }
    }

    allCompleted = (completedCount == we.Status.TotalSteps)
    anyFailed = (failedCount > 0)

    return allCompleted, anyFailed
}
```

**Test Coverage**:
```go
It("should detect step completion even after missed watch event", func() {
    // Create workflow
    // Create steps
    // Manually update KubernetesExecution (DEPRECATED - ADR-025) status (bypass watch)
    // Wait for periodic reconciliation (30s)
    // Verify workflow detects completion
})
```

---

### 4. Step Retry Exhaustion

**Scenario**: Step fails repeatedly and exhausts retry budget

**Causes**:
- Persistent infrastructure problem
- Incorrect action parameters
- Insufficient RBAC permissions
- Resource unavailable

**Handling Strategy**:
1. **Retry Budget**: 3 attempts per step (configurable)
2. **Exponential Backoff**: 30s, 60s, 120s between retries
3. **Retry Reason Tracking**: Log each retry reason
4. **Exhaustion Handling**: Fail workflow after retry exhaustion
5. **Manual Intervention**: Allow manual retry with annotation

**Implementation**:
```go
type StepResult struct {
    StepName           string
    Status             string
    ExecutionStartTime *metav1.Time
    ExecutionEndTime   *metav1.Time
    RetryCount         int
    RetryReasons       []string
    LastError          string
}

func (r *Orchestrator) CreateStepExecution(ctx context.Context, we *WorkflowExecution, step WorkflowStep) error {
    // Check if step already exists
    existingKE, err := r.findExistingStepExecution(ctx, we, step.Name)
    if err != nil {
        return err
    }

    if existingKE != nil {
        // Step exists, check if failed and needs retry
        if existingKE.Status.Phase == "Failed" {
            retryCount := existingKE.Status.RetryCount
            maxRetries := 3  // Configurable

            if retryCount >= maxRetries {
                // Retry budget exhausted
                log.Error(nil, "Step retry budget exhausted",
                    "step", step.Name,
                    "retryCount", retryCount,
                    "maxRetries", maxRetries)

                // Update workflow step result
                r.updateStepResult(ctx, we, step.Name, StepResult{
                    StepName:       step.Name,
                    Status:         "Failed",
                    RetryCount:     retryCount,
                    RetryReasons:   existingKE.Status.RetryReasons,
                    LastError:      fmt.Sprintf("Retry budget exhausted after %d attempts", retryCount),
                })

                return fmt.Errorf("step %s failed after %d retries", step.Name, retryCount)
            }

            // Calculate backoff delay
            backoffDelay := time.Duration(30*math.Pow(2, float64(retryCount))) * time.Second
            log.Info("Retrying failed step after backoff",
                "step", step.Name,
                "retryCount", retryCount+1,
                "backoffDelay", backoffDelay)

            // Delete failed KubernetesExecution (DEPRECATED - ADR-025) and recreate
            if err := r.client.Delete(ctx, existingKE); err != nil {
                return err
            }

            // Wait for backoff
            time.Sleep(backoffDelay)

            // Create new KubernetesExecution (DEPRECATED - ADR-025) with incremented retry count
            newKE := r.buildKubernetesExecution(we, step)
            newKE.Status.RetryCount = retryCount + 1
            newKE.Status.RetryReasons = append(existingKE.Status.RetryReasons, existingKE.Status.LastError)
            return r.client.Create(ctx, newKE)
        }

        // Step exists and not failed, no action needed
        return nil
    }

    // Step doesn't exist, create it
    ke := r.buildKubernetesExecution(we, step) // DEPRECATED - ADR-025
    return r.client.Create(ctx, ke)
}
```

**Manual Retry Annotation**:
```go
// Allow manual retry by adding annotation
if we.Annotations["kubernaut.ai/manual-retry"] == "true" {
    log.Info("Manual retry requested, resetting retry counts")
    // Reset retry counts for failed steps
    delete(we.Annotations, "kubernaut.ai/manual-retry")
    if err := r.Update(ctx, we); err != nil {
        return ctrl.Result{}, err
    }
}
```

**Test Coverage**:
```go
It("should fail workflow after step retry exhaustion", func() {
    // Create workflow with failing step
    // Verify 3 retry attempts with exponential backoff
    // Verify workflow fails after 3rd retry
    // Add manual-retry annotation
    // Verify retry counter resets
})
```

---

## ⚙️ Kubernetes Executor Edge Cases

### 1. Job Failure Categorization

**Scenario**: Kubernetes Job fails, need to determine if retryable or terminal

**Causes**:
- ImagePullBackOff (terminal - wrong image)
- OOMKilled (retryable - increase resources)
- Exit code 1 (depends on reason)
- Pod eviction (retryable)

**Handling Strategy**:
1. **Failure Categories**:
   - **Retryable**: OOMKilled, pod eviction, transient network errors
   - **Terminal**: ImagePullBackOff, invalid RBAC, non-existent resource
2. **Exit Code Interpretation**: 0=success, 1=failure, 2=retryable
3. **Pod Condition Analysis**: Check pod conditions for specific failures
4. **Retry Decision**: Automatic retry for retryable failures only
5. **Metrics**: Track failure categories

**Implementation**:
```go
type FailureCategory string

const (
    FailureRetryable FailureCategory = "Retryable"
    FailureTerminal  FailureCategory = "Terminal"
)

type FailureAnalysis struct {
    Category FailureCategory
    Reason   string
    Retryable bool
}

func (m *JobManager) AnalyzeJobFailure(ctx context.Context, job *batchv1.Job) (*FailureAnalysis, error) {
    // Get pod for this job
    var pods corev1.PodList
    if err := m.client.List(ctx, &pods, client.MatchingLabels{
        "job-name": job.Name,
    }); err != nil {
        return nil, err
    }

    if len(pods.Items) == 0 {
        return &FailureAnalysis{
            Category:  FailureTerminal,
            Reason:    "No pods created for job",
            Retryable: false,
        }, nil
    }

    pod := pods.Items[0]

    // Check pod conditions
    for _, condition := range pod.Status.Conditions {
        if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
            if condition.Reason == "Unschedulable" {
                return &FailureAnalysis{
                    Category:  FailureRetryable,
                    Reason:    "Pod unschedulable - may be transient resource constraint",
                    Retryable: true,
                }, nil
            }
        }
    }

    // Check container statuses
    for _, containerStatus := range pod.Status.ContainerStatuses {
        if containerStatus.State.Waiting != nil {
            waiting := containerStatus.State.Waiting

            switch waiting.Reason {
            case "ImagePullBackOff", "ErrImagePull":
                return &FailureAnalysis{
                    Category:  FailureTerminal,
                    Reason:    fmt.Sprintf("Image pull failed: %s", waiting.Message),
                    Retryable: false,
                }, nil

            case "CrashLoopBackOff":
                // Need to check terminated state for exit code
                if containerStatus.LastTerminationState.Terminated != nil {
                    exitCode := containerStatus.LastTerminationState.Terminated.ExitCode
                    if exitCode == 2 {
                        return &FailureAnalysis{
                            Category:  FailureRetryable,
                            Reason:    "Exit code 2 indicates retryable failure",
                            Retryable: true,
                        }, nil
                    }
                }

                return &FailureAnalysis{
                    Category:  FailureTerminal,
                    Reason:    fmt.Sprintf("CrashLoopBackOff with exit code %d", containerStatus.LastTerminationState.Terminated.ExitCode),
                    Retryable: false,
                }, nil
            }
        }

        if containerStatus.State.Terminated != nil {
            terminated := containerStatus.State.Terminated

            if terminated.Reason == "OOMKilled" {
                return &FailureAnalysis{
                    Category:  FailureRetryable,
                    Reason:    "Pod OOMKilled - retryable with increased memory",
                    Retryable: true,
                }, nil
            }

            if terminated.Reason == "Error" {
                exitCode := terminated.ExitCode
                if exitCode == 2 {
                    return &FailureAnalysis{
                        Category:  FailureRetryable,
                        Reason:    "Exit code 2 indicates retryable failure",
                        Retryable: true,
                    }, nil
                }

                return &FailureAnalysis{
                    Category:  FailureTerminal,
                    Reason:    fmt.Sprintf("Exit code %d: %s", exitCode, terminated.Message),
                    Retryable: false,
                }, nil
            }

            if terminated.Reason == "Evicted" {
                return &FailureAnalysis{
                    Category:  FailureRetryable,
                    Reason:    "Pod evicted - retryable",
                    Retryable: true,
                }, nil
            }
        }
    }

    // Default to terminal if unable to categorize
    return &FailureAnalysis{
        Category:  FailureTerminal,
        Reason:    "Unknown failure reason",
        Retryable: false,
    }, nil
}
```

**Test Coverage**:
```go
Describe("Job Failure Categorization", func() {
    It("should categorize ImagePullBackOff as terminal", func() {
        // Create job with non-existent image
        // Verify failure analysis is Terminal
        // Verify no retry attempted
    })

    It("should categorize OOMKilled as retryable", func() {
        // Create job that uses excessive memory
        // Verify failure analysis is Retryable
        // Verify retry with increased memory
    })

    It("should categorize exit code 2 as retryable", func() {
        // Create job that exits with code 2
        // Verify failure analysis is Retryable
        // Verify retry attempted
    })
})
```

---

### 2. Orphaned Job Cleanup

**Scenario**: Kubernetes Jobs left behind after controller restart or KubernetesExecution (DEPRECATED - ADR-025) deletion

**Causes**:
- Controller crash before Job cleanup
- KubernetesExecution (DEPRECATED - ADR-025) CRD deleted manually
- TTLSecondsAfterFinished not working
- Job stuck in Running state

**Detection**:
```go
// Periodic cleanup reconciliation
return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

// Find orphaned jobs
orphanedJobs := r.findOrphanedJobs(ctx)
```

**Handling Strategy**:
1. **Owner References**: Jobs have owner reference to KubernetesExecution (DEPRECATED - ADR-025)
2. **TTL Controller**: Use TTLSecondsAfterFinished for automatic cleanup
3. **Periodic Scan**: Every 5 minutes, scan for orphaned jobs
4. **Cleanup Criteria**: Job older than 1 hour with no owner
5. **Metrics**: Track orphaned job count

**Implementation**:
```go
func (m *JobManager) CreateJob(ctx context.Context, ke *KubernetesExecution, actionDef *ActionDefinition) (*batchv1.Job, error) { // ke DEPRECATED - ADR-025
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-job", ke.Name),
            Namespace: ke.Namespace,
            Labels: map[string]string{
                "kubernaut.ai/execution": ke.Name,
                "kubernaut.ai/action":    ke.Spec.Action,
                "kubernaut.ai/managed-by": "kubernetes-executor",
            },
            // Owner reference for automatic cleanup
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(ke, schema.GroupVersionKind{
                    Group:   "kubernetesexecution.kubernaut.ai",
                    Version: "v1alpha1",
                    Kind:    "KubernetesExecution", // DEPRECATED - ADR-025
                }),
            },
        },
        Spec: batchv1.JobSpec{
            TTLSecondsAfterFinished: int32Ptr(300),  // Cleanup after 5 minutes
            BackoffLimit:            int32Ptr(0),    // No retries
            // ... rest of spec
        },
    }

    return job, m.client.Create(ctx, job)
}

// Periodic orphaned job cleanup
func (r *Reconciler) cleanupOrphanedJobs(ctx context.Context) error {
    var jobList batchv1.JobList
    if err := r.List(ctx, &jobList, client.MatchingLabels{
        "kubernaut.ai/managed-by": "kubernetes-executor",
    }); err != nil {
        return err
    }

    for _, job := range jobList.Items {
        // Check if owner KubernetesExecution (DEPRECATED - ADR-025) exists
        if len(job.OwnerReferences) == 0 {
            // Orphaned job, check age
            age := time.Since(job.CreationTimestamp.Time)
            if age > 1*time.Hour {
                log.Info("Cleaning up orphaned job", "job", job.Name, "age", age)
                if err := r.Delete(ctx, &job); err != nil {
                    log.Error(err, "Failed to delete orphaned job", "job", job.Name)
                }
                orphanedJobsCleanedTotal.Inc()
            }
        }
    }

    return nil
}
```

**Test Coverage**:
```go
It("should cleanup orphaned jobs after 1 hour", func() {
    // Create KubernetesExecution (DEPRECATED - ADR-025)
    // Create associated Job
    // Delete KubernetesExecution (DEPRECATED - ADR-025) (leaving Job orphaned)
    // Wait for 1 hour (or mock time)
    // Trigger cleanup reconciliation
    // Verify Job is deleted
})
```

---

### 3. Safety Policy Hot-Reload

**Scenario**: Rego policies updated in ConfigMap while controller is running

**Causes**:
- Administrator updates safety policies
- New environment restrictions
- Security incident response

**Handling Strategy**:
1. **ConfigMap Watch**: Watch for ConfigMap changes
2. **Policy Reload**: Reload policies on ConfigMap update
3. **Validation**: Validate new policies before applying
4. **Rollback**: Revert to previous policies if validation fails
5. **Metrics**: Track policy reload events

**Implementation**:
```go
type SafetyEngine struct {
    policies    map[string]*rego.PreparedEvalQuery
    configMap   string
    mu          sync.RWMutex
    client      client.Client
}

func (e *SafetyEngine) Start(ctx context.Context) error {
    // Initial policy load
    if err := e.LoadPolicies(ctx); err != nil {
        return err
    }

    // Watch ConfigMap for changes
    go e.watchPolicies(ctx)

    return nil
}

func (e *SafetyEngine) watchPolicies(ctx context.Context) {
    // Setup watch on ConfigMap
    watcher := &source.Kind{Type: &corev1.ConfigMap{}}

    for {
        select {
        case <-ctx.Done():
            return
        case event := <-watcher.Start(ctx):
            cm, ok := event.Object.(*corev1.ConfigMap)
            if !ok || cm.Name != e.configMap {
                continue
            }

            log.Info("Policy ConfigMap updated, reloading policies")
            if err := e.LoadPolicies(ctx); err != nil {
                log.Error(err, "Failed to reload policies, keeping existing policies")
                policyReloadFailuresTotal.Inc()
                continue
            }

            log.Info("Policies reloaded successfully")
            policyReloadsTotal.Inc()
        }
    }
}

func (e *SafetyEngine) LoadPolicies(ctx context.Context) error {
    // Fetch ConfigMap
    var cm corev1.ConfigMap
    if err := e.client.Get(ctx, client.ObjectKey{
        Namespace: "kubernaut-system",
        Name:      e.configMap,
    }, &cm); err != nil {
        return err
    }

    // Parse and compile policies
    newPolicies := make(map[string]*rego.PreparedEvalQuery)
    for policyName, policyContent := range cm.Data {
        if !strings.HasSuffix(policyName, ".rego") {
            continue
        }

        // Compile policy
        query, err := rego.New(
            rego.Query("data.kubernetesexecution.safety.deny"),
            rego.Module(policyName, policyContent),
        ).PrepareForEval(ctx)
        if err != nil {
            log.Error(err, "Failed to compile policy", "policy", policyName)
            return fmt.Errorf("invalid policy %s: %w", policyName, err)
        }

        newPolicies[policyName] = &query
    }

    // Atomic update
    e.mu.Lock()
    e.policies = newPolicies
    e.mu.Unlock()

    log.Info("Loaded policies", "count", len(newPolicies))
    return nil
}

func (e *SafetyEngine) EvaluateAction(ctx context.Context, ke *KubernetesExecution, actionDef *ActionDefinition) (*PolicyResult, error) { // ke DEPRECATED - ADR-025
    e.mu.RLock()
    policies := e.policies
    e.mu.RUnlock()

    // Evaluate all policies
    input := map[string]interface{}{
        "action":      ke.Spec.Action,
        "parameters":  ke.Spec.ActionParameters,
        "environment": ke.Labels["environment"],
        "target":      ke.Spec.TargetResource,
    }

    for policyName, query := range policies {
        results, err := query.Eval(ctx, rego.EvalInput(input))
        if err != nil {
            return nil, fmt.Errorf("policy evaluation failed: %w", err)
        }

        if len(results) > 0 && len(results[0].Expressions) > 0 {
            denials := results[0].Expressions[0].Value.([]interface{})
            if len(denials) > 0 {
                return &PolicyResult{
                    Allowed: false,
                    Reason:  fmt.Sprintf("Policy %s denied: %v", policyName, denials[0]),
                }, nil
            }
        }
    }

    return &PolicyResult{
        Allowed: true,
        Reason:  "Action approved by all policies",
    }, nil
}
```

**Test Coverage**:
```go
It("should reload policies when ConfigMap is updated", func() {
    // Create controller with initial policies
    // Verify initial policy enforced
    // Update ConfigMap with new policy
    // Verify new policy takes effect
    // Verify policy reload metric incremented
})

It("should keep existing policies if reload fails", func() {
    // Create controller with valid policies
    // Update ConfigMap with invalid Rego syntax
    // Verify policy reload fails
    // Verify existing policies still enforced
    // Verify policy reload failure metric incremented
})
```

---

## 🧪 Testing Edge Cases

### Edge Case Test Structure

**File Organization**:
```
test/
├── unit/
│   ├── common/
│   │   ├── leader_election_test.go
│   │   ├── status_update_conflict_test.go
│   │   └── watch_reconnection_test.go
│   ├── remediationprocessing/
│   │   ├── postgresql_failure_test.go
│   │   ├── semantic_search_timeout_test.go
│   │   └── deduplication_race_test.go
│   ├── workflowexecution/
│   │   ├── circular_dependency_test.go
│   │   ├── timeout_hierarchy_test.go
│   │   └── retry_exhaustion_test.go
│   └── kubernetesexecution/
│       ├── job_failure_categorization_test.go
│       ├── orphaned_job_cleanup_test.go
│       └── policy_hot_reload_test.go
├── integration/
│   └── edge_cases/
│       ├── controller_restart_test.go
│       ├── api_rate_limiting_test.go
│       └── degraded_mode_test.go
└── e2e/
    └── chaos/
        ├── network_partition_test.go
        └── pod_eviction_test.go
```

### Edge Case Test Example

```go
// test/unit/workflowexecution/circular_dependency_test.go
var _ = Describe("Circular Dependency Detection", func() {
    Context("When workflow has circular dependencies", func() {
        It("should detect simple cycle A → B → A", func() {
            steps := []WorkflowStep{
                {Name: "stepA", DependsOn: []string{"stepB"}},
                {Name: "stepB", DependsOn: []string{"stepA"}},
            }

            resolver := NewDependencyResolver()
            _, err := resolver.ResolveSteps(steps)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("circular dependency"))
            Expect(err.Error()).To(ContainSubstring("stepA"))
            Expect(err.Error()).To(ContainSubstring("stepB"))
        })

        It("should detect complex cycle A → B → C → A", func() {
            steps := []WorkflowStep{
                {Name: "stepA", DependsOn: []string{"stepC"}},
                {Name: "stepB", DependsOn: []string{"stepA"}},
                {Name: "stepC", DependsOn: []string{"stepB"}},
            }

            resolver := NewDependencyResolver()
            _, err := resolver.ResolveSteps(steps)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("circular dependency"))
            // Verify cycle is identified in error message
            Expect(err.Error()).To(MatchRegexp("stepA.*stepB.*stepC.*stepA"))
        })

        It("should detect self-dependency", func() {
            steps := []WorkflowStep{
                {Name: "stepA", DependsOn: []string{"stepA"}},
            }

            resolver := NewDependencyResolver()
            _, err := resolver.ResolveSteps(steps)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("circular dependency"))
            Expect(err.Error()).To(ContainSubstring("stepA"))
        })
    })
})
```

---

## 📊 Edge Case Metrics

### Common Metrics

```prometheus
# Leader election failures
controller_leader_election_failures_total

# Status update conflicts
controller_status_update_conflicts_total{resource_type}

# Watch reconnections
controller_watch_reconnections_total{resource_type}

# API rate limit errors
controller_api_rate_limit_errors_total

# Controller restarts
controller_restarts_total{reason}
```

### Service-Specific Metrics

**Remediation Processor**:
```prometheus
# PostgreSQL connection failures
remediation_postgresql_connection_failures_total

# Semantic search timeouts
remediation_semantic_search_timeouts_total

# Circuit breaker state
remediation_circuit_breaker_state{state}  # 0=closed, 1=open, 2=half-open

# Deduplication suppression
remediation_duplicate_signals_suppressed_total
```

**Workflow Execution**:
```prometheus
# Circular dependency detections
workflow_circular_dependency_detected_total

# Workflow timeouts
workflow_timeout_exceeded_total{reason}

# Step retry exhaustion
workflow_step_retry_exhausted_total

# Missed watch events
workflow_watch_events_missed_total
```

**Kubernetes Executor**:
```prometheus
# Job failure categories
kubernetes_job_failures_total{category}  # category=retryable|terminal

# Orphaned jobs cleaned
kubernetes_orphaned_jobs_cleaned_total

# Policy reloads
kubernetes_policy_reloads_total

# Policy reload failures
kubernetes_policy_reload_failures_total
```

---

## ✅ Validation Checklist

Before considering edge case handling complete:

### Common Edge Cases
- [ ] Leader election failure tested and handled
- [ ] Status update conflicts tested with optimistic locking
- [ ] Watch connection loss tested with automatic reconnection
- [ ] Controller restart tested with idempotent reconciliation
- [ ] API rate limiting tested with backoff

### Remediation Processor
- [ ] PostgreSQL connection loss tested with circuit breaker
- [ ] Semantic search timeout tested with fallback
- [ ] No historical data tested with AI routing
- [ ] Deduplication race conditions tested

### Workflow Execution
- [ ] Circular dependency detection tested
- [ ] Timeout hierarchy tested (step vs workflow)
- [ ] Watch-based missed events tested
- [ ] Step retry exhaustion tested

### Kubernetes Executor
- [ ] Job failure categorization tested
- [ ] Orphaned job cleanup tested
- [ ] Safety policy hot-reload tested

---

**Status**: ✅ **Edge Case Documentation Complete**
**Impact**: +1.5% confidence across all services
**Next Action**: Implement edge case handling and tests
**Validation**: Run edge case test suite in integration environment

