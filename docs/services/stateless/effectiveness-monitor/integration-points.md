# Effectiveness Monitor Service - Integration Points

**Version**: 1.1
**Last Updated**: October 16, 2025
**Service Type**: Stateless HTTP API Service (Assessment & Analysis)
**Port**: 8080 (REST + Health), 9090 (Metrics)
**Prompt Format**: Self-Documenting JSON (DD-HOLMESGPT-009)

**📊 Visual Reference**: [Effectiveness Monitor Sequence Diagrams](../../../architecture/effectiveness-monitor-sequence-diagrams.md) - See complete flows with real examples

---

## 🔗 Upstream Clients (Services Calling Effectiveness Monitor)

### **1. Context API Service** (Port 8080)

**Use Case**: Retrieve effectiveness assessments for historical intelligence

```go
// pkg/context/effectiveness_client.go
package context

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "go.uber.org/zap"
)

func (c *ContextAPIService) GetEffectivenessAssessment(ctx context.Context, actionID string) (*EffectivenessData, error) {
    url := fmt.Sprintf("http://effectiveness-monitor-service:8080/api/v1/assess/effectiveness/%s", actionID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getServiceAccountToken()))

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to call Effectiveness Monitor",
            zap.Error(err),
            zap.String("action_id", actionID),
        )
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("effectiveness monitor returned status %d", resp.StatusCode)
    }

    var assessment EffectivenessData
    if err := json.NewDecoder(resp.Body).Decode(&assessment); err != nil {
        return nil, err
    }

    return &assessment, nil
}
```

---

### **2. Effectiveness Monitor Controller (Internal Trigger)**

**Use Case**: Kubernetes controller watches RemediationRequest CRDs and triggers assessments

**Design Decision**: [DD-EFFECTIVENESS-003](../../../architecture/decisions/DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md) - Watch RR instead of WE for better abstraction and future-proofing

```go
// pkg/monitor/effectiveness_monitor_controller.go
package monitor

import (
    "context"
    "time"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediationrequest/v1alpha1"
)

func (r *EffectivenessMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var rr remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &rr); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Only process completed/failed/timeout remediations
    if rr.Status.OverallPhase != "completed" &&
       rr.Status.OverallPhase != "failed" &&
       rr.Status.OverallPhase != "timeout" {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    // Idempotency check - already assessed?
    traceID := string(rr.UID)
    alreadyAssessed, err := r.db.IsEffectivenessAssessed(ctx, traceID)
    if err != nil {
        return ctrl.Result{RequeueAfter: 5 * time.Minute}, err
    }

    if alreadyAssessed {
        // Already assessed - skip (idempotent)
        return ctrl.Result{}, nil
    }

    // Create assessment record with 5-minute stabilization delay
    assessment := &ActionAssessment{
        TraceID:      traceID,
        ActionType:   rr.Spec.SignalName,
        ExecutedAt:   rr.Status.CompletionTime.Time,
        ScheduledFor: time.Now().Add(5 * time.Minute), // 5-minute stabilization
        Status:       "pending",
    }

    created, err := r.db.CreateAssessmentIfNotExists(ctx, assessment)
    if err != nil {
        return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
    }

    if !created {
        // Another replica already created this assessment
        return ctrl.Result{}, nil
    }

    // Assessment will be performed by background worker after stabilization
    return ctrl.Result{}, nil
}
```

**Trigger Mechanism**:
- **Source**: Kubernetes Watch API on `RemediationRequest` CRDs
- **Filter**: `status.overallPhase IN ("completed", "failed", "timeout")`
- **Delay**: 5-minute stabilization period before assessment begins
- **Idempotency**: Database-backed (RemediationRequest.UID as unique key)

**Accessing WorkflowExecution Details** (Optional - Rare Cases):
```go
// If detailed workflow information is needed (rare)
func (r *EffectivenessMonitorReconciler) getWorkflowDetails(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (*workflowv1.WorkflowExecution, error) {

    if rr.Status.WorkflowExecutionRef == nil {
        return nil, fmt.Errorf("no workflow execution reference")
    }

    we := &workflowv1.WorkflowExecution{}
    weKey := types.NamespacedName{
        Name:      rr.Status.WorkflowExecutionRef.Name,
        Namespace: rr.Status.WorkflowExecutionRef.Namespace,
    }

    if err := r.Get(ctx, weKey, we); err != nil {
        return nil, fmt.Errorf("failed to fetch workflow execution: %w", err)
    }

    return we, nil
}

// Example usage
if r.needsDetailedWorkflowInfo(rr) {
    we, err := r.getWorkflowDetails(ctx, rr)
    if err == nil {
        // Use detailed workflow metrics, step information, etc.
        detailedMetrics := we.Status.ExecutionMetrics
        stepDetails := we.Status.Steps
        // ...
    }
}
```

**See**:
- [DD-EFFECTIVENESS-003](../../../architecture/decisions/DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md) - Watch strategy decision
- [DD-EFFECTIVENESS-002](../../../decisions/DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md) - Restart recovery and idempotency

---

## 🔽 Downstream Dependencies (External Services)

### **1. Data Storage Service** (Port 8085)

**Purpose**: Action history retrieval, assessment result persistence

#### **Action History Retrieval**

```go
// pkg/effectiveness/data_storage_client.go
package effectiveness

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

type DataStorageClient struct {
    baseURL       string
    httpClient    *http.Client
    logger        *zap.Logger
    serviceToken  string
}

func NewDataStorageClient(baseURL string, logger *zap.Logger) *DataStorageClient {
    return &DataStorageClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        logger: logger,
    }
}

func (c *DataStorageClient) GetActionHistory(ctx context.Context, actionType string, window time.Duration) ([]ActionHistory, error) {
    url := fmt.Sprintf("%s/api/v1/audit/actions?action_type=%s&time_range=%s",
        c.baseURL, actionType, window.String())

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to retrieve action history from Data Storage",
            zap.Error(err),
            zap.String("action_type", actionType),
        )
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    var history []ActionHistory
    if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
        return nil, err
    }

    c.logger.Debug("Retrieved action history",
        zap.String("action_type", actionType),
        zap.Int("count", len(history)),
    )

    return history, nil
}

func (c *DataStorageClient) GetOldestAction(ctx context.Context) (*ActionHistory, error) {
    url := fmt.Sprintf("%s/api/v1/audit/actions/oldest", c.baseURL)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    var action ActionHistory
    if err := json.NewDecoder(resp.Body).Decode(&action); err != nil {
        return nil, err
    }

    return &action, nil
}
```

#### **Assessment Result Persistence**

```go
func (c *DataStorageClient) PersistAssessment(ctx context.Context, assessment *EffectivenessScore) error {
    url := fmt.Sprintf("%s/api/v1/audit/effectiveness", c.baseURL)

    payload, err := json.Marshal(assessment)
    if err != nil {
        return err
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to persist assessment to Data Storage",
            zap.Error(err),
            zap.String("assessment_id", assessment.AssessmentID),
        )
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    c.logger.Info("Assessment persisted successfully",
        zap.String("assessment_id", assessment.AssessmentID),
    )

    return nil
}
```

---

### **2. Infrastructure Monitoring Service** (Port 8094)

**Purpose**: Metrics correlation for environmental impact assessment

```go
// pkg/effectiveness/infrastructure_monitoring_client.go
package effectiveness

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

type InfrastructureMonitoringClient struct {
    baseURL      string
    httpClient   *http.Client
    logger       *zap.Logger
    serviceToken string
}

func NewInfrastructureMonitoringClient(baseURL string, logger *zap.Logger) *InfrastructureMonitoringClient {
    return &InfrastructureMonitoringClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        logger: logger,
    }
}

func (c *InfrastructureMonitoringClient) GetMetricsAfterAction(ctx context.Context, actionID string, window time.Duration) (*EnvironmentalMetrics, error) {
    url := fmt.Sprintf("%s/api/v1/metrics/after-action?action_id=%s&window=%s",
        c.baseURL, actionID, window.String())

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Warn("Failed to retrieve metrics from Infrastructure Monitoring",
            zap.Error(err),
            zap.String("action_id", actionID),
        )
        // Graceful degradation: return nil metrics, not an error
        return &EnvironmentalMetrics{}, nil
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        c.logger.Warn("Infrastructure Monitoring returned non-OK status",
            zap.Int("status_code", resp.StatusCode),
            zap.String("action_id", actionID),
        )
        return &EnvironmentalMetrics{}, nil
    }

    var metrics EnvironmentalMetrics
    if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
        c.logger.Error("Failed to decode metrics response",
            zap.Error(err),
        )
        return &EnvironmentalMetrics{}, nil
    }

    c.logger.Debug("Retrieved environmental metrics",
        zap.String("action_id", actionID),
        zap.Float64("memory_improvement", metrics.MemoryImprovement),
        zap.Float64("cpu_impact", metrics.CPUImpact),
    )

    return &metrics, nil
}
```

---

### **3. HolmesGPT API Service** (Port 8080)

**Purpose**: Selective AI analysis for high-value cases (hybrid approach)

**Use Case**: Post-execution analysis for learning and pattern detection

**Call Pattern**: Selective (only ~18,000/year out of ~3.65M actions)

**Decision Logic**: `shouldCallAI()` evaluates whether AI analysis adds value

**Prompt Format**: Self-Documenting JSON (DD-HOLMESGPT-009)
- ✅ **75% token reduction** (~730 → ~180 tokens per analysis)
- ✅ **$1,320/year cost savings** on 18K AI calls ($73/month)
- ✅ **150ms latency improvement** per post-execution analysis
- ✅ **98% parsing accuracy maintained**

**Decision Document**: `docs/architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md`

```go
// pkg/effectiveness/holmesgpt_client.go
package effectiveness

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "go.uber.org/zap"
)

type HolmesGPTClient struct {
    baseURL      string
    httpClient   *http.Client
    logger       *zap.Logger
    serviceToken string
}

func NewHolmesGPTClient(baseURL string, logger *zap.Logger) *HolmesGPTClient {
    return &HolmesGPTClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 60 * time.Second, // AI calls can be slow
        },
        logger: logger,
    }
}

// shouldCallAI determines if AI analysis adds value (hybrid approach)
func (c *HolmesGPTClient) shouldCallAI(
    workflowExecution *WorkflowExecution,
    basicScore float64,
    anomalies []string,
) bool {
    // Decision triggers (from DD-EFFECTIVENESS-001)

    // 1. P0 failures (~50/day) - YES
    if workflowExecution.Priority == "P0" && !workflowExecution.Success {
        return true
    }

    // 2. New action types (~10/day) - YES
    if workflowExecution.IsNewActionType {
        return true
    }

    // 3. Anomalies detected (~5/day) - YES
    if len(anomalies) > 0 {
        return true
    }

    // 4. Oscillation/recurring failures (~5/day) - YES
    if workflowExecution.IsRecurringFailure {
        return true
    }

    // 5. Routine successes (~10,000/day) - NO
    return false
}

func (c *HolmesGPTClient) PostExecutionAnalyze(
    ctx context.Context,
    request *PostExecRequest,
) (*PostExecResponse, error) {
    url := fmt.Sprintf("%s/api/v1/postexec/analyze", c.baseURL)

    // Build ultra-compact JSON context (DD-HOLMESGPT-009)
    encoder := InvestigationContext{}
    compactContext, err := encoder.BuildPostExecCompactContext(request)
    if err != nil {
        return nil, fmt.Errorf("failed to build compact context: %w", err)
    }

    // Post-execution request with ultra-compact context (~180 tokens vs ~730)
    compactRequest := map[string]interface{}{
        "context":      compactContext,  // Ultra-compact JSON
        "llmProvider":  "openai",
        "llmModel":     "gpt-4",
        "analysisType": "post_execution",
    }

    payload, err := json.Marshal(compactRequest)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.serviceToken))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        c.logger.Error("Failed to call HolmesGPT API for post-execution analysis",
            zap.Error(err),
            zap.String("execution_id", request.ExecutionID),
        )
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("holmesgpt API returned status %d", resp.StatusCode)
    }

    var analysis PostExecResponse
    if err := json.NewDecoder(resp.Body).Decode(&analysis); err != nil {
        return nil, err
    }

    c.logger.Info("AI post-execution analysis completed (ultra-compact format)",
        zap.String("execution_id", request.ExecutionID),
        zap.Float64("effectiveness_score", analysis.EffectivenessScore),
        zap.Int("lessons_learned", len(analysis.LessonsLearned)),
        zap.Int("token_count", len(compactContext)/4), // Approximate
    )

    return &analysis, nil
}

// Example assessment flow with selective AI
func (s *EffectivenessMonitorService) performAssessment(
    ctx context.Context,
    assessment *ActionAssessment,
) error {
    // 1. Always perform automated assessment
    basicScore, anomalies := s.calculateBasicEffectiveness(ctx, assessment)

    // 2. Decision: Call AI?
    if s.holmesgptClient.shouldCallAI(assessment.WorkflowExecution, basicScore, anomalies) {
        // Call HolmesGPT API for AI analysis
        aiAnalysis, err := s.holmesgptClient.PostExecutionAnalyze(ctx, &PostExecRequest{
            ExecutionID:         assessment.TraceID,
            ActionType:          assessment.ActionType,
            PreExecutionState:   assessment.PreState,
            PostExecutionState:  assessment.PostState,
            ExecutionSuccess:    assessment.Success,
            // ... other fields
        })

        if err != nil {
            // Graceful degradation - use automated results only
            s.logger.Warn("AI analysis failed, using automated results only",
                zap.Error(err),
                zap.String("trace_id", assessment.TraceID),
            )
            return s.storeBasicResults(ctx, assessment, basicScore)
        }

        // Combine automated + AI results
        return s.storeCombinedResults(ctx, assessment, basicScore, aiAnalysis)
    }

    // No AI call needed - store automated results
    return s.storeBasicResults(ctx, assessment, basicScore)
}
```

**Cost/Benefit**:
- **AI Calls**: ~18,000/year (0.49% of total actions)
- **Cost**: ~$9,000/year ($0.50 per AI call)
- **Value**: 85-90% effectiveness (vs 70% without AI)
- **ROI**: 11x return on investment

**See**: `/docs/decisions/DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md` for complete design

---

## 📊 Data Flow Diagram

```
┌───────────────────────────────────────────────────────────────┐
│  Upstream Triggers                                            │
│  ┌──────────────────────┐    ┌─────────────────────┐         │
│  │ WorkflowExecution    │    │  Context API        │         │
│  │ CRD (Completed)      │    │  Service (8080)     │         │
│  └─────────┬────────────┘    └──────────┬──────────┘         │
│            │ Watch Event              │ HTTP GET              │
│            │ (Kubernetes API)         │ /assess/effectiveness │
└────────────┼──────────────────────────┼────────────────────────┘
             │                          │
             ▼                          ▼
┌────────────────────────────────────────────────────────────────┐
│  Effectiveness Monitor Controller + Service (Port 8080)        │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ CONTROLLER (Kubernetes Reconciliation Loop)              │ │
│  │ 1. Watch WorkflowExecution CRDs                          │ │
│  │ 2. Filter: phase IN ("completed", "failed")              │ │
│  │ 3. Idempotency Check (Database)                          │ │
│  │ 4. Create Assessment Record (scheduled_for: +5min)       │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ ASSESSMENT WORKER (Background Process)                   │ │
│  │ 1. Pick Pending Assessments (scheduled_for <= NOW)       │ │
│  │ 2. Query Action History (Data Storage)                   │ │
│  │ 3. Query Metrics (Infrastructure Monitoring)             │ │
│  │ 4. Calculate Basic Effectiveness Score                   │ │
│  │ 5. Decision: shouldCallAI()? (selective)                 │ │
│  │ 6. [IF YES] Call HolmesGPT API (post-exec analysis)      │ │
│  │ 7. Combine Automated + AI Results                        │ │
│  │ 8. Store Results (Database)                              │ │
│  └──────────────────────────────────────────────────────────┘ │
└──────┬───────────────┬────────────────┬──────────────────┬─────┘
       │               │                │                  │
       │ HTTP GET/POST │ HTTP GET       │ HTTP POST        │
       │ (Bearer Token)│ (Bearer Token) │ (Bearer Token)   │
       ▼               ▼                ▼                  │
┌──────────────┐ ┌─────────────────┐ ┌─────────────────┐ │
│Data Storage  │ │Infrastructure   │ │HolmesGPT API    │ │
│Service (8085)│ │Monitoring (8094)│ │Service (8080)   │ │
│              │ │                 │ │                 │ │
│- Action      │ │- CPU/Memory     │ │- Post-Execution │ │
│  History     │ │  Metrics        │ │  Analysis       │ │
│- Assessment  │ │- Network        │ │- Learning       │ │
│  Persistence │ │  Stability      │ │- Lessons        │ │
│- Idempotency │ │- Side Effects   │ │  Extraction     │ │
│  State       │ │  Detection      │ │  (Selective)    │ │
└──────────────┘ └─────────────────┘ └─────────────────┘ │
                                                          │
     Kubernetes          External                         │
     Cluster (Watch)     Prometheus                       │
          ▲                   ▲                           │
          │                   │                           │
          └───────────────────┴───────────────────────────┘
```

**Key Architectural Points**:
1. **Trigger**: Kubernetes Watch API on WorkflowExecution CRDs (NOT HTTP API call)
2. **Stabilization**: 5-minute delay between completion and assessment
3. **Idempotency**: Database-backed using WorkflowExecution.UID as unique key
4. **Hybrid Approach**: Automated assessment ALWAYS, AI analysis SELECTIVE (~0.49% of actions)
5. **HolmesGPT Direction**: ✅ CORRECTED - Effectiveness Monitor calls HolmesGPT (downstream)

---

## 🔄 Request Flow

### **Complete Assessment Request**

```go
// Example: Complete assessment request flow
func (s *EffectivenessMonitorService) AssessEffectiveness(ctx context.Context, req *AssessmentRequest) (*EffectivenessScore, error) {
    // Step 1: Check data availability (8+ weeks required)
    dataWeeks, sufficient := s.checkDataAvailability(ctx)
    if !sufficient {
        return s.insufficientDataResponse(dataWeeks), nil
    }

    // Step 2: Retrieve action history from Data Storage
    history, err := s.dataStorageClient.GetActionHistory(ctx, req.ActionType, 90*24*time.Hour)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve action history: %w", err)
    }

    // Step 3: Calculate traditional effectiveness score
    traditionalScore := s.calculator.CalculateTraditionalScore(history)

    // Step 4: Query metrics from Infrastructure Monitoring (graceful degradation)
    metrics, err := s.infraMonitorClient.GetMetricsAfterAction(ctx, req.ActionID, 10*time.Minute)
    if err != nil {
        s.logger.Warn("Failed to retrieve environmental metrics, continuing with basic assessment",
            zap.Error(err),
        )
        metrics = &EnvironmentalMetrics{} // Default to zero impact
    }

    // Step 5: Detect side effects
    sideEffects, severity := s.calculator.DetectSideEffects(metrics)

    // Step 6: Analyze trends
    trendDirection := s.calculator.AnalyzeTrend(history)

    // Step 7: Generate pattern insights
    patterns := s.calculator.GeneratePatternInsights(history, req.ActionData)

    // Step 8: Calculate confidence
    confidence := s.calculator.CalculateConfidence(history, dataWeeks)

    // Step 9: Build assessment result
    assessment := &EffectivenessScore{
        AssessmentID:        generateAssessmentID(),
        ActionID:            req.ActionID,
        ActionType:          req.ActionType,
        TraditionalScore:    traditionalScore,
        EnvironmentalImpact: *metrics,
        Confidence:          confidence,
        Status:              "assessed",
        SideEffectsDetected: sideEffects,
        SideEffectSeverity:  severity,
        TrendDirection:      trendDirection,
        PatternInsights:     patterns,
        AssessedAt:          time.Now(),
    }

    // Step 10: Persist assessment to Data Storage (best-effort)
    if err := s.dataStorageClient.PersistAssessment(ctx, assessment); err != nil {
        s.logger.Error("Failed to persist assessment, continuing",
            zap.Error(err),
            zap.String("assessment_id", assessment.AssessmentID),
        )
    }

    return assessment, nil
}
```

---

## 🔄 Circuit Breaker Pattern

### **Graceful Degradation for Infrastructure Monitoring**

```go
package effectiveness

import (
    "context"
    "time"

    "go.uber.org/zap"
)

type CircuitBreaker struct {
    failureCount      int
    lastFailureTime   time.Time
    threshold         int
    resetTimeout      time.Duration
    halfOpenRequests  int
    logger            *zap.Logger
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration, logger *zap.Logger) *CircuitBreaker {
    return &CircuitBreaker{
        threshold:    threshold,
        resetTimeout: resetTimeout,
        logger:       logger,
    }
}

func (cb *CircuitBreaker) Call(ctx context.Context, fn func(context.Context) (*EnvironmentalMetrics, error)) (*EnvironmentalMetrics, error) {
    // If circuit is open, return default metrics immediately
    if cb.isOpen() {
        cb.logger.Warn("Circuit breaker open, returning default metrics")
        return &EnvironmentalMetrics{}, nil
    }

    // Attempt call
    metrics, err := fn(ctx)
    if err != nil {
        cb.recordFailure()
        cb.logger.Warn("Circuit breaker recorded failure",
            zap.Int("failure_count", cb.failureCount),
        )
        return &EnvironmentalMetrics{}, nil
    }

    cb.recordSuccess()
    return metrics, nil
}

func (cb *CircuitBreaker) isOpen() bool {
    if cb.failureCount >= cb.threshold {
        if time.Since(cb.lastFailureTime) < cb.resetTimeout {
            return true
        }
        // Reset to half-open state
        cb.failureCount = 0
        cb.halfOpenRequests = 0
    }
    return false
}

func (cb *CircuitBreaker) recordFailure() {
    cb.failureCount++
    cb.lastFailureTime = time.Now()
}

func (cb *CircuitBreaker) recordSuccess() {
    cb.failureCount = 0
}
```

---

## 📊 Error Handling Strategy

| Dependency | Failure Mode | Handling Strategy |
|-----------|--------------|-------------------|
| **Data Storage** | Unavailable | Return error (critical dependency) |
| **Infrastructure Monitoring** | Unavailable | Graceful degradation (log warning, continue with basic assessment) |
| **Context API** | Not applicable | Effectiveness Monitor does not depend on Context API |

---

## ✅ Integration Checklist

### **Pre-Deployment**

- [ ] Data Storage Service connection tested (action history retrieval)
- [ ] Infrastructure Monitoring Service connection tested (metrics query)
- [ ] Circuit breaker configured for Infrastructure Monitoring
- [ ] Graceful degradation tested (Infrastructure Monitoring unavailable)
- [ ] Assessment persistence tested (Data Storage write)

### **Runtime Integration**

- [ ] All HTTP clients use Bearer token authentication
- [ ] Timeouts configured (10s for Data Storage, 10s for Infrastructure Monitoring)
- [ ] Circuit breaker operational for Infrastructure Monitoring
- [ ] Assessment results persisted to Data Storage (best-effort)
- [ ] Metrics correlation works when Infrastructure Monitoring available

### **Monitoring**

- [ ] Data Storage call duration tracked in metrics
- [ ] Infrastructure Monitoring call duration tracked in metrics
- [ ] Circuit breaker state exposed in metrics
- [ ] Graceful degradation events logged
- [ ] Assessment persistence failures alerted

---

## 🔄 **ARCHITECTURAL CORRECTIONS - October 16, 2025**

### **Critical Fix: HolmesGPT API Direction**

**❌ PREVIOUS (INCORRECT)**:
- HolmesGPT API listed as "Upstream Client" (calling Effectiveness Monitor)
- Implied Effectiveness Monitor was providing effectiveness data TO HolmesGPT

**✅ CORRECTED (October 16, 2025)**:
- HolmesGPT API moved to "Downstream Dependencies" (called BY Effectiveness Monitor)
- Effectiveness Monitor selectively calls HolmesGPT for post-execution analysis
- Hybrid approach: Automated assessment ALWAYS, AI analysis SELECTIVE (~0.49% of actions)

### **Additional Corrections**

1. **Trigger Mechanism Clarified**:
   - Added "Effectiveness Monitor Controller" as internal trigger
   - Documents Kubernetes Watch API pattern on WorkflowExecution CRDs
   - Documents 5-minute stabilization delay

2. **Idempotency Design Added**:
   - Database-backed state using WorkflowExecution.UID
   - Restart recovery mechanisms documented
   - Race condition handling explained

3. **Hybrid Approach Detailed**:
   - `shouldCallAI()` decision logic documented
   - Selective AI analysis pattern (18K/year out of 3.65M actions)
   - Cost/benefit analysis included ($9K/year for 11x ROI)

### **Related Design Decisions**

- **DD-EFFECTIVENESS-001**: Hybrid Automated + AI Analysis Approach
  - Location: `/docs/decisions/DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md`
  - Decision: Automated assessment always, AI analysis selective
  - Rationale: 11x ROI, 85-90% effectiveness vs 70% without AI

- **DD-EFFECTIVENESS-002**: Restart Recovery & Idempotency
  - Location: `/docs/decisions/DD-EFFECTIVENESS-002-Restart-Recovery-Idempotency.md`
  - Decision: Database-backed idempotency with WorkflowExecution.UID
  - Rationale: Zero manual intervention, automatic catch-up, HA support

### **Related Architecture Documents**

- **Restart Recovery Flows**: `/docs/architecture/EFFECTIVENESS_MONITOR_RESTART_RECOVERY_FLOWS.md`
  - Complete operational flows with timing breakdowns
  - 6 recovery scenarios documented
  - Monitoring metrics and alert rules

- **CRD Design Assessment**: `/holmesgpt-api/docs/EFFECTIVENESS_MONITOR_CRD_DESIGN_ASSESSMENT.md`
  - Why NO custom CRD required
  - Watch-and-database pattern rationale
  - CRD vs Database comparison

- **Technical Details**: `/holmesgpt-api/docs/EFFECTIVENESS_MONITOR_RESTART_RECOVERY.md`
  - 5 restart scenarios with complete flows
  - Edge case handling
  - Implementation requirements

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 16, 2025 (Major Architectural Corrections)
**Status**: ✅ Complete Specification - Architecturally Validated

