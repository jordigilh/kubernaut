# DD-CONTEXT-006: Signal Processor Recovery Data Source Assessment

**Date**: November 11, 2025 (Revised)
**Status**: ✅ Approved
**Decision Makers**: Architecture Team
**Impact**: High - Affects Context API scope and Signal Processor recovery flow
**Related**: DD-001 (Recovery Context Enrichment), DD-CONTEXT-005 (Minimal LLM Response Schema), ADR-035 (Remediation Execution Engine)

---

## 🎯 Decision

**APPROVED: Remediation Orchestrator embeds failure data from WorkflowExecution CRD. Signal Processor does NOT query Context API for historical recovery context.**

**Confidence**: **95%**

**Rationale**: Historical recovery data is circumstantial and cannot be reliably used for LLM decision-making. LLM should reason about the current situation, not historical statistics.

**Note**: Context API endpoints for historical recovery data MAY remain available for future use (human analysis, debugging, observability) but will NOT be used by Signal Processor for LLM prompt enrichment.

---

## 🎯 Original Question

**Should Signal Processor query Context API via REST API for failed remediation history, or should Remediation Orchestrator extract failure data from WorkflowExecution CRD and embed it in the recovery Signal Processor CRD?**

---

## 📋 Context & Problem

### Current Architecture (DD-001: Alternative 2)

**Flow**:
```
WorkflowExecution fails
    ↓
Remediation Orchestrator detects failure
    ↓
Remediation Orchestrator creates NEW SignalProcessing CRD (recovery=true)
    ↓
SignalProcessing Controller enriches with:
    - Fresh monitoring context (from Data Storage)
    - Fresh business context (from Data Storage)
    - Recovery context (from Context API REST call) ← **QUESTION: Is this needed?**
    ↓
Remediation Orchestrator copies enriched data to AIAnalysis CRD
    ↓
AI Analysis → HolmesGPT API → LLM receives recovery context
```

### The Question

**Current Approach**: Signal Processor calls Context API REST endpoint to get historical failure data

**Alternative Approach**: Remediation Orchestrator extracts failure data from WorkflowExecution CRD and embeds it directly in SignalProcessing CRD spec

### What Data is Available in WorkflowExecution CRD?

From `api/workflowexecution/v1alpha1/workflowexecution_types.go`:

```go
type WorkflowExecutionStatus struct {
    Phase            string       `json:"phase"`              // "failed"
    FailedStep       *int         `json:"failedStep"`         // Which step failed (0-based)
    FailedAction     *string      `json:"failedAction"`       // Action type (e.g., "scale-deployment")
    FailureReason    string       `json:"failureReason"`      // Human-readable reason
    ErrorType        *string      `json:"errorType"`          // Classified error ("timeout", "permission_denied")
    ExecutionSnapshot *ExecutionSnapshot `json:"executionSnapshot"` // State at failure time

    StepStatuses     []StepStatus `json:"stepStatuses"`       // Individual step results
    ExecutionMetrics *ExecutionMetrics `json:"executionMetrics"` // Performance data
    CompletionTime   *metav1.Time `json:"completionTime"`     // When it failed
}

type ExecutionSnapshot struct {
    CompletedSteps   []StepResult           `json:"completedSteps"`
    CurrentStep      int                    `json:"currentStep"`
    ClusterState     map[string]interface{} `json:"clusterState"`
    ResourceSnapshot map[string]interface{} `json:"resourceSnapshot"`
    Timestamp        metav1.Time            `json:"timestamp"`
}

type StepStatus struct {
    StepNumber       int                     `json:"stepNumber"`
    Action           string                  `json:"action"`
    Status           string                  `json:"status"` // "completed", "failed"
    StartTime        *metav1.Time            `json:"startTime"`
    EndTime          *metav1.Time            `json:"endTime"`
    ErrorMessage     string                  `json:"errorMessage"`
    RetriesAttempted int                     `json:"retriesAttempted"`
}
```

**Key Insight**: WorkflowExecution CRD contains **complete failure data** for the current attempt.

---

## 🔍 Alternatives Analysis

### Alternative 1: Signal Processor Queries Context API (Current - DD-001)

**Approach**: Signal Processor calls Context API REST endpoint to get historical recovery context

**What Context API Provides**:
- Historical failure patterns across ALL previous attempts
- Related signals/alerts that occurred around the same time
- Success rates of different playbooks for similar incidents
- Aggregated failure reasons and recovery strategies

**Data Flow**:
```go
// In SignalProcessing Controller
if rp.Spec.IsRecoveryAttempt {
    recoveryCtx, err := r.ContextAPIClient.GetRemediationContext(ctx, remediationRequestID)
    if err != nil {
        recoveryCtx = r.buildFallbackRecoveryContext(rp)
    }
    rp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
}
```

**Pros**:
- ✅ Provides **aggregated historical data** across all previous attempts
- ✅ Includes **related signals** and **pattern analysis**
- ✅ Offers **playbook success rates** for similar incidents
- ✅ Enables **trend analysis** (is this getting worse?)
- ✅ Supports **cross-incident learning** (similar failures in other services)

**Cons**:
- ❌ Requires Context API to expose REST endpoint for Signal Processor
- ❌ Adds external dependency to Signal Processor
- ❌ Network call latency (~50-200ms)
- ❌ Requires graceful degradation logic

**Confidence**: 85%

---

### Alternative 2: Remediation Orchestrator Embeds Failure Data (Proposed)

**Approach**: Remediation Orchestrator extracts failure data from WorkflowExecution CRD and embeds it in SignalProcessing CRD spec

**What Remediation Orchestrator Can Provide**:
- Current attempt's failure details (step, action, reason, error type)
- Execution snapshot (cluster state, resource state at failure time)
- Step-by-step execution results
- Performance metrics (duration, success rate of completed steps)

**Data Flow**:
```go
// In Remediation Orchestrator
func (r *RemediationRequestReconciler) initiateRecovery(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {

    // Extract failure data from WorkflowExecution
    failureData := &processingv1.FailureData{
        FailedStep:        *failedWorkflow.Status.FailedStep,
        FailedAction:      *failedWorkflow.Status.FailedAction,
        FailureReason:     failedWorkflow.Status.FailureReason,
        ErrorType:         *failedWorkflow.Status.ErrorType,
        ExecutionSnapshot: failedWorkflow.Status.ExecutionSnapshot,
        StepStatuses:      failedWorkflow.Status.StepStatuses,
        CompletionTime:    failedWorkflow.Status.CompletionTime,
    }

    // Create recovery SignalProcessing with embedded failure data
    recoveryRP := &processingv1.RemediationProcessing{
        Spec: processingv1.RemediationProcessingSpec{
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: remediation.Status.RecoveryAttempts,
            FailedWorkflowRef:     &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailureData:           failureData, // ← NEW: Embedded failure data
        },
    }
}
```

**Pros**:
- ✅ **No external dependency**: Signal Processor doesn't need Context API REST client
- ✅ **No network latency**: Data already in CRD spec
- ✅ **Complete failure details**: All data from WorkflowExecution available
- ✅ **Simpler architecture**: One less REST integration point
- ✅ **Consistent with self-contained CRD pattern**: All data in spec

**Cons**:
- ❌ **No historical aggregation**: Only current attempt's data, not previous attempts
- ❌ **No cross-incident patterns**: Can't learn from similar failures in other services
- ❌ **No playbook success rates**: Can't recommend based on historical effectiveness
- ❌ **No related signals**: Can't correlate with other alerts/incidents
- ❌ **Limited AI context**: LLM doesn't get rich historical patterns

**Confidence**: 45%

---

## 💡 Key Analysis

### What Does the LLM Actually Need for Recovery?

**For effective recovery recommendations, the LLM needs**:

1. **Current Failure Context** (Alternative 2 provides):
   - ✅ What failed (step, action)
   - ✅ Why it failed (error type, reason)
   - ✅ Cluster state at failure time
   - ✅ What succeeded before failure

2. **Historical Context** (Only Alternative 1 provides):
   - ❌ Have we seen this failure before?
   - ❌ What worked in previous recovery attempts?
   - ❌ Are similar incidents happening elsewhere?
   - ❌ Is this failure pattern getting worse over time?
   - ❌ Which playbooks have high success rates for this incident type?

### Critical Question: Is Historical Context Valuable?

**Scenario 1**: First recovery attempt
- **Alternative 1**: Context API returns "no previous attempts" + related signals from other services
- **Alternative 2**: Only current failure data available
- **Winner**: Alternative 1 (cross-incident learning still valuable)

**Scenario 2**: Second recovery attempt (first recovery failed)
- **Alternative 1**: Context API returns "previous attempt failed with X, tried playbook Y, similar incidents suggest Z"
- **Alternative 2**: Only current failure data (no knowledge of first recovery attempt)
- **Winner**: Alternative 1 (historical learning is critical)

**Scenario 3**: Recurring incident (5th time this week)
- **Alternative 1**: Context API returns "this is a pattern, success rate declining, recommend escalation"
- **Alternative 2**: Treats each as isolated incident
- **Winner**: Alternative 1 (trend analysis is critical)

---

---

## 🔍 Alternative 3: Embed Current Failure Data Only (APPROVED)

**Approach**: Remediation Orchestrator extracts failure data from WorkflowExecution CRD and embeds it in SignalProcessing CRD spec. NO historical recovery context from Context API.

**What LLM Receives**:
- ✅ Current failure details (from WorkflowExecution CRD)
- ✅ Current monitoring context (from Data Storage)
- ✅ Current business context (from Data Storage)
- ✅ Available playbooks (from Context API semantic search via HolmesGPT API tool)
- ❌ NO historical recovery context
- ❌ NO previous attempt outcomes
- ❌ NO historical failure patterns

**Data Flow**:
```
Remediation Orchestrator:
  ↓ Extracts failure data from WorkflowExecution CRD
  ↓ Embeds in SignalProcessing CRD spec
  ↓
SignalProcessing Controller:
  ↓ Enriches with monitoring context (current state)
  ↓ Enriches with business context (current ownership)
  ↓ NO Context API call for historical recovery data
  ↓
AIAnalysis/HolmesGPT API:
  ↓ Receives current failure context
  ↓ Queries Context API for playbooks (semantic search)
  ↓ LLM reasons about current situation
  ↓ LLM picks best playbook based on current context
```

**Pros**:
- ✅ **No circumstantial data**: LLM reasons about current situation, not historical statistics
- ✅ **No bias**: Doesn't favor "historically successful" playbooks
- ✅ **Simpler architecture**: No Signal Processor → Context API REST integration
- ✅ **No network latency**: All data in CRD spec
- ✅ **Consistent with DD-CONTEXT-005**: Filter before LLM, don't pre-enrich with conditional data
- ✅ **Semantic search still available**: LLM can query for playbooks via HolmesGPT API tool

**Cons**:
- ⚠️ Historical data not available to LLM (but this is actually a PRO - prevents circumstantial reasoning)

**Confidence**: **95%** (APPROVED)

---

## 📊 Why Historical Data is Not Suitable for LLM Decisions

### Problem 1: Causation vs Correlation (98% Confidence)

**The Issue**: We don't capture all environment data to ensure we're in the same situation.

**Example**:
```
Historical Record:
- Playbook X failed at step 2 with "timeout"
- Cluster had 80% CPU utilization
- 5 pods were pending

Current Situation:
- Same playbook, same step, same "timeout" error
- Cluster has 80% CPU utilization
- 5 pods are pending

Question: Are these the SAME situation?
```

**Missing Context**:
- ❌ Network conditions (latency, packet loss)
- ❌ External dependencies (database load, API availability)
- ❌ Kubernetes version differences
- ❌ Node hardware differences
- ❌ Time-of-day effects (traffic patterns)
- ❌ Recent cluster changes (upgrades, config changes)
- ❌ Concurrent workloads

**Conclusion**: Historical "similarity" is superficial. LLM cannot safely assume "same failure = same cause = same solution."

---

### Problem 2: Historical Data is Circumstantial (96% Confidence)

**Example**:
```
Historical Pattern:
"Playbook pod-restart-v1.2 failed 3 times in production with timeout errors"

LLM Reasoning:
❌ WRONG: "This playbook has low success rate, try different playbook"
✅ RIGHT: "This playbook might have issues, but I need to understand WHY it failed"
```

**The Problem**: Historical success/failure rates don't tell you:
- Why it failed (root cause)
- What changed since then (environment evolution)
- Whether the failure was playbook-related or environmental

**This is the same issue as `success_rate` in DD-CONTEXT-005**:
- Exposing success rates creates bias
- LLM should reason about current situation, not historical statistics
- Historical data useful for humans to evaluate trends, not for LLM decisions

---

### Problem 3: Semantic Search is Sufficient (92% Confidence)

**Key Distinction**:

| Type | Purpose | LLM Use | Human Use |
|------|---------|---------|-----------|
| **Semantic Search** | Find playbooks matching incident description | ✅ YES - Find relevant solutions | ✅ YES - Discover options |
| **Historical Success Rates** | Track playbook performance over time | ❌ NO - Creates bias | ✅ YES - Evaluate effectiveness |
| **Historical Failure Patterns** | Identify recurring issues | ❌ NO - Circumstantial | ✅ YES - Trend analysis |

**Why Semantic Search Works**:
- Matches incident description to playbook descriptions
- Based on semantic similarity (meaning), not statistics
- Doesn't assume "worked before = will work now"
- Helps LLM find relevant options, then LLM reasons about current situation

**Why Historical Data Doesn't Work**:
- Assumes past performance predicts future results
- Doesn't account for environment changes
- Creates bias toward "historically successful" playbooks
- Prevents adoption of new/better playbooks

---

## 📊 Confidence Assessment (Original Alternatives)

### Alternative 1: Signal Processor Queries Context API (REJECTED)

**Confidence**: **15%** that this is the correct approach (REVISED DOWN from 90%)

**Why 90%**:
1. **Historical context is critical for AI** (95% confidence)
   - LLM needs to know "what we tried before" to avoid repeating failures
   - Cross-incident learning improves recommendations
   - Trend analysis enables escalation decisions

2. **Current failure data alone is insufficient** (92% confidence)
   - Treating each recovery as isolated incident misses patterns
   - No way to learn from previous attempts
   - Can't recommend based on historical playbook effectiveness

3. **REST API overhead is acceptable** (85% confidence)
   - 50-200ms latency is acceptable for recovery flow (not time-critical)
   - Graceful degradation already implemented (fallback to current data only)
   - Network call happens once per recovery attempt (not per reconciliation loop)

4. **Architectural consistency** (88% confidence)
   - Context API's purpose is to provide historical intelligence
   - Signal Processor already calls Data Storage for monitoring/business context
   - One more REST call for recovery context is consistent

**Risks**:
- ⚠️ Context API unavailability (Mitigation: Graceful degradation with fallback)
- ⚠️ Additional REST endpoint to maintain (Mitigation: Well-defined interface)

---

### Alternative 2: Remediation Orchestrator Embeds Failure Data

**Confidence**: **45%** that this would be sufficient

**Why Only 45%**:
1. **Missing critical historical context** (90% confidence this is a problem)
   - No knowledge of previous recovery attempts
   - No cross-incident pattern recognition
   - No playbook effectiveness data
   - No trend analysis capabilities

2. **Violates Context API's purpose** (85% confidence this is wrong)
   - Context API exists to provide historical intelligence
   - Not using it for recovery contradicts its design purpose
   - Would need to duplicate historical analysis logic elsewhere

3. **Limited AI effectiveness** (88% confidence)
   - LLM recommendations would be based on incomplete data
   - Higher risk of repeating failed recovery strategies
   - Missed opportunities for cross-incident learning

**When This Might Be Acceptable**:
- ✅ If Context API is unavailable (fallback scenario)
- ✅ If this is the FIRST attempt and no historical data exists yet
- ✅ For very simple incidents where historical context adds no value

---

## 🎯 Implementation Guidance

### Approved Approach: Embed Current Failure Data Only

**Remediation Orchestrator** extracts failure data from WorkflowExecution CRD:

```go
// In Remediation Orchestrator
func (r *RemediationRequestReconciler) initiateRecovery(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {

    // Extract failure data from WorkflowExecution
    failureData := &processingv1.FailureData{
        FailedStep:        *failedWorkflow.Status.FailedStep,
        FailedAction:      *failedWorkflow.Status.FailedAction,
        FailureReason:     failedWorkflow.Status.FailureReason,
        ErrorType:         *failedWorkflow.Status.ErrorType,
        ExecutionSnapshot: failedWorkflow.Status.ExecutionSnapshot,
        StepStatuses:      failedWorkflow.Status.StepStatuses,
        ExecutionMetrics:  failedWorkflow.Status.ExecutionMetrics,
        CompletionTime:    failedWorkflow.Status.CompletionTime,
    }

    // Create recovery SignalProcessing with embedded failure data
    recoveryRP := &processingv1.RemediationProcessing{
        Spec: processingv1.RemediationProcessingSpec{
            Alert:                     originalRP.Spec.Alert,
            EnrichmentConfig:          originalRP.Spec.EnrichmentConfig,
            EnvironmentClassification: originalRP.Spec.EnvironmentClassification,
            RemediationRequestRef:     originalRP.Spec.RemediationRequestRef,

            // Recovery-specific fields
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: remediation.Status.RecoveryAttempts,
            FailedWorkflowRef:     &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailureData:           failureData, // ← Embedded current failure data
            OriginalProcessingRef: &corev1.LocalObjectReference{Name: originalRP.Name},
        },
    }

    return r.Create(ctx, recoveryRP), nil
}
```

**SignalProcessing Controller** does NOT query Context API:

```go
// In SignalProcessing Controller
func (r *RemediationProcessingReconciler) reconcileEnriching(
    ctx context.Context,
    rp *processingv1.RemediationProcessing,
) (ctrl.Result, error) {

    // ALWAYS: Enrich monitoring + business context (gets FRESH data)
    enrichmentResults, err := r.ContextService.GetContext(ctx, rp.Spec.Alert)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    rp.Status.EnrichmentResults = enrichmentResults

    // IF RECOVERY: Use embedded failure data (NO Context API call)
    if rp.Spec.IsRecoveryAttempt {
        log.Info("Recovery attempt detected - using embedded failure data",
            "attemptNumber", rp.Spec.RecoveryAttemptNumber,
            "failedStep", rp.Spec.FailureData.FailedStep,
            "errorType", rp.Spec.FailureData.ErrorType)

        // Failure data already in spec - no external call needed
        rp.Status.EnrichmentResults.FailureContext = rp.Spec.FailureData
    }

    rp.Status.Phase = "classifying"
    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, rp)
}
```

---

## 📋 Context API REST Endpoint Specification (Optional - Not Used by Signal Processor)

**Note**: This endpoint MAY be implemented for human analysis, debugging, or observability purposes, but is NOT used by Signal Processor for LLM prompt enrichment.

### Endpoint

```http
GET /api/v1/remediation/{remediationRequestId}/recovery-context
```

**Usage**: Human analysis, debugging dashboards, observability tools
**NOT Used By**: Signal Processor, AIAnalysis, HolmesGPT API

### Response Schema

```json
{
  "remediation_request_id": "rr-abc123",
  "context_quality": "complete",
  "previous_failures": [
    {
      "attempt_number": 1,
      "failed_step": 2,
      "failure_reason": "Timeout waiting for pod to become ready",
      "error_type": "timeout",
      "playbook_used": "pod-restart-v1.2",
      "timestamp": "2025-11-11T10:30:00Z"
    }
  ],
  "related_signals": [
    {
      "signal_id": "sig-xyz789",
      "incident_type": "pod-oom-killer",
      "correlation_score": 0.85,
      "timestamp": "2025-11-11T10:25:00Z"
    }
  ],
  "historical_patterns": [
    {
      "pattern": "pod-restart-timeout",
      "frequency": 5,
      "last_seen": "2025-11-10T14:20:00Z",
      "successful_recovery_strategies": ["increase-timeout", "scale-up-resources"]
    }
  ],
  "playbook_effectiveness": [
    {
      "playbook_id": "pod-restart-v1.2",
      "success_rate": 0.65,
      "total_executions": 20,
      "avg_duration_ms": 45000
    }
  ]
}
```

---

---

## 💡 Where Historical Data IS Valuable

### For Humans (Effectiveness Monitor Service)

**Use Cases**:
- ✅ Track playbook success rates over time
- ✅ Identify degrading playbooks (success rate declining)
- ✅ Detect recurring failure patterns (need systemic fix)
- ✅ Evaluate playbook effectiveness across environments
- ✅ Trigger alerts when patterns indicate systemic issues

**Implementation**: Effectiveness Monitor Service tracks historical data and presents it via dashboards/alerts for human analysis.

### For System (Effectiveness Monitor → Playbook Status)

**Use Cases**:
- ✅ Mark playbooks as "degraded" when success rate drops below threshold
- ✅ Filter out consistently failing playbooks from Context API semantic search results
- ✅ Trigger human review when recurring patterns detected
- ✅ Update playbook metadata (status, warnings) based on historical performance

**Implementation**: Effectiveness Monitor updates playbook status in database, Context API filters by status when returning semantic search results.

### NOT For LLM Decision-Making

**Anti-Patterns** (DO NOT DO):
- ❌ "This playbook failed before, try different one"
- ❌ "Historical success rate is 60%, prefer other playbook"
- ❌ "Similar failures happened 3 times this week"

**Why**: Historical data is circumstantial and creates bias. LLM should reason about current situation.

---

## 🔗 Related Decisions

- **DD-001**: Recovery Context Enrichment (Alternative 2) - **SUPERSEDED** by this decision (no Context API call for recovery)
- **DD-CONTEXT-005**: Minimal LLM Response Schema - Establishes filtering before LLM pattern (consistent with this decision)
- **ADR-035**: Remediation Execution Engine (Tekton Pipelines) - Defines WorkflowExecution CRD schema with failure data
- **BR-WF-RECOVERY-011**: Context API integration requirement - **MODIFIED** by this decision (no REST call, data embedded in CRD)

---

## 📝 Future Considerations

### V2.0 Enhancements

**Potential optimization** (not for V1.0):
- Cache recent recovery contexts in Signal Processor to reduce Context API calls
- Implement streaming updates for long-running recovery analysis
- Add predictive failure detection based on historical patterns

**Rule**: Only implement if recovery latency becomes critical (<30 seconds requirement)

---

**Document Version**: 1.0
**Last Updated**: November 11, 2025
**Status**: 📊 **ANALYSIS COMPLETE**
**Overall Confidence**: **90%** (Keep Alternative 1 - Signal Processor queries Context API)

