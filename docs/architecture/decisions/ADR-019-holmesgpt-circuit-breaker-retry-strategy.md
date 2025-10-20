# ADR-019: HolmesGPT Circuit Breaker & Retry Strategy

**Status**: ✅ **APPROVED**  
**Date**: 2025-10-17  
**Related**: ADR-018 (Approval Notification Integration)  
**Confidence**: 90%

---

## Context & Problem

HolmesGPT-API is an **external dependency** for AI-powered root cause analysis. If HolmesGPT becomes unavailable (service down, model unavailable, network partition), the AIAnalysis controller cannot proceed.

**Critical Questions**:
1. **What happens when HolmesGPT is down?** Block all remediations? Fail immediately? Retry?
2. **How long should we retry?** Indefinitely? Fixed duration? Exponential backoff?
3. **What should operators see?** Clear status? Actionable error messages?
4. **What is the fallback?** Manual approval? Queue for later? Fail fast?

**Impact**:
- **HolmesGPT unavailability = Kubernaut unavailability** (without retry strategy)
- **No clear operator communication** during transient failures
- **Complete remediation blockage** for extended outages

---

## Decision

**APPROVED: Exponential Backoff Retry with 5-Minute Timeout + Manual Fallback**

**Strategy**:
1. **Retry with exponential backoff** for up to **5 minutes** (configurable)
2. **Update AIAnalysis status and conditions** to reflect retry state
3. **After 5 minutes**: Fail AIAnalysis with reason "HolmesGPT-API unavailable"
4. **Manual fallback**: Create AIApprovalRequest with "AI unavailable - manual review required"

**Rationale**:
- ✅ **Resilient to transient failures**: Network blips, temporary service restarts
- ✅ **Clear observability**: Status reflects retry state, operators know what's happening
- ✅ **Bounded retry time**: 5 minutes prevents indefinite blocking
- ✅ **Manual fallback**: System remains usable even if HolmesGPT down
- ✅ **Configurable timeout**: Can adjust for different environments (staging vs. production)

---

## Design Details

### **Retry Schedule (Exponential Backoff)**

| Attempt | Delay | Cumulative Time | Status Message |
|---|---|---|---|
| **1** | 0s | 0s | "Calling HolmesGPT-API (attempt 1/10)" |
| **2** | 5s | 5s | "HolmesGPT retry (attempt 2/10, next in 10s)" |
| **3** | 10s | 15s | "HolmesGPT retry (attempt 3/10, next in 20s)" |
| **4** | 20s | 35s | "HolmesGPT retry (attempt 4/10, next in 30s)" |
| **5** | 30s | 65s | "HolmesGPT retry (attempt 5/10, next in 30s)" |
| **6** | 30s | 95s | "HolmesGPT retry (attempt 6/10, next in 30s)" |
| **7** | 30s | 125s | "HolmesGPT retry (attempt 7/10, next in 30s)" |
| **8** | 30s | 155s | "HolmesGPT retry (attempt 8/10, next in 30s)" |
| **9** | 30s | 185s | "HolmesGPT retry (attempt 9/10, next in 30s)" |
| **10** | 30s | 215s | "HolmesGPT retry (attempt 10/10, next in 30s)" |
| **11** | 30s | 245s | "HolmesGPT retry (attempt 11/10, next in 30s)" |
| **12** | 30s | 275s | "HolmesGPT retry (attempt 12/10, last attempt)" |
| **13** | — | **305s (5min)** | **"HolmesGPT-API unavailable after 5 minutes"** |

**Backoff Formula**:
```go
delay := min(initialDelay * (2 ^ (attempt - 1)), maxDelay)
// initialDelay = 5s
// maxDelay = 30s
// timeout = 5 minutes (configurable)
```

**Total Attempts**: ~12-13 attempts over 5 minutes

---

### **AIAnalysis Status Updates**

#### **During Retry**

```yaml
status:
  phase: "investigating"
  message: "HolmesGPT retry (attempt 3/12, next in 20s)"
  reason: "HolmesGPTRetrying"
  holmesGPTRetryAttempts: 3
  holmesGPTLastError: "connection timeout"
  holmesGPTLastAttemptTime: "2025-10-17T10:30:15Z"
  holmesGPTNextRetryTime: "2025-10-17T10:30:35Z"
  conditions:
    - type: "HolmesGPTAvailable"
      status: "False"
      reason: "ConnectionTimeout"
      message: "HolmesGPT-API connection timeout (attempt 3/12)"
      lastTransitionTime: "2025-10-17T10:30:15Z"
```

#### **After Timeout (5 minutes)**

```yaml
status:
  phase: "failed"
  message: "HolmesGPT-API unavailable after 5 minutes (12 attempts)"
  reason: "HolmesGPTUnavailable"
  holmesGPTRetryAttempts: 12
  holmesGPTLastError: "connection timeout"
  holmesGPTTotalRetryDuration: "305s"
  requiresApproval: true  # Fall back to manual approval
  approvalContext:
    reason: "AI analysis unavailable - manual review required"
    confidenceLevel: "none"
    investigationSummary: "HolmesGPT-API was unavailable for 5 minutes. Manual root cause analysis required."
    evidenceCollected:
      - "HolmesGPT-API unreachable after 12 retry attempts"
      - "Last error: connection timeout"
      - "Total retry duration: 305 seconds"
    recommendedActions:
      - action: "manual_investigation"
        rationale: "AI analysis unavailable - requires human expertise"
    whyApprovalRequired: "AI service unavailable - fallback to manual review"
  conditions:
    - type: "HolmesGPTAvailable"
      status: "False"
      reason: "Timeout"
      message: "HolmesGPT-API unavailable after 5 minutes (12 attempts)"
      lastTransitionTime: "2025-10-17T10:35:20Z"
```

---

### **AIAnalysis CRD Status Fields**

**New Fields for Retry Tracking**:
```go
type AIAnalysisStatus struct {
    // ... existing fields ...
    
    // HolmesGPT retry tracking
    HolmesGPTRetryAttempts    int        `json:"holmesGPTRetryAttempts,omitempty"`
    HolmesGPTLastError        string     `json:"holmesGPTLastError,omitempty"`
    HolmesGPTLastAttemptTime  *metav1.Time `json:"holmesGPTLastAttemptTime,omitempty"`
    HolmesGPTNextRetryTime    *metav1.Time `json:"holmesGPTNextRetryTime,omitempty"`
    HolmesGPTTotalRetryDuration string   `json:"holmesGPTTotalRetryDuration,omitempty"`
}
```

---

### **Configuration Options**

**ConfigMap: `kubernaut-aianalysis-config`**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-aianalysis-config
  namespace: kubernaut-system
data:
  holmesgpt-retry-timeout: "5m"  # Total retry duration
  holmesgpt-initial-delay: "5s"  # First retry delay
  holmesgpt-max-delay: "30s"     # Maximum delay between retries
  holmesgpt-retry-multiplier: "2" # Exponential backoff multiplier
```

**Environment Variables** (overrides ConfigMap):
```bash
HOLMESGPT_RETRY_TIMEOUT=5m
HOLMESGPT_INITIAL_DELAY=5s
HOLMESGPT_MAX_DELAY=30s
HOLMESGPT_RETRY_MULTIPLIER=2
```

---

## Implementation Details

### **AIAnalysis Controller Retry Logic**

**File**: `internal/controller/aianalysis/aianalysis_controller.go`

```go
package aianalysis

import (
    "context"
    "fmt"
    "time"
    
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    ctrl "sigs.k8s.io/controller-runtime"
)

type AIAnalysisReconciler struct {
    // ... existing fields ...
    
    // Retry configuration
    HolmesGPTRetryTimeout    time.Duration
    HolmesGPTInitialDelay    time.Duration
    HolmesGPTMaxDelay        time.Duration
    HolmesGPTRetryMultiplier int
}

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    aiAnalysis := &aianalysisv1.AIAnalysis{}
    if err := r.Get(ctx, req.NamespacedName, aiAnalysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Handle "investigating" phase with HolmesGPT retry
    if aiAnalysis.Status.Phase == "investigating" {
        return r.handleInvestigating(ctx, aiAnalysis)
    }
    
    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) handleInvestigating(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    
    // Initialize retry tracking
    if aiAnalysis.Status.HolmesGPTRetryAttempts == 0 {
        aiAnalysis.Status.HolmesGPTRetryAttempts = 1
        aiAnalysis.Status.HolmesGPTLastAttemptTime = &metav1.Time{Time: time.Now()}
    }
    
    // Check timeout
    startTime := aiAnalysis.Status.StartedAt.Time
    elapsed := time.Since(startTime)
    if elapsed > r.HolmesGPTRetryTimeout {
        log.Error(nil, "HolmesGPT retry timeout exceeded",
            "attempts", aiAnalysis.Status.HolmesGPTRetryAttempts,
            "duration", elapsed)
        return r.failWithManualFallback(ctx, aiAnalysis)
    }
    
    // Call HolmesGPT
    result, err := r.HolmesGPTClient.AnalyzeAlert(ctx, aiAnalysis.Spec)
    if err != nil {
        // Update retry state
        aiAnalysis.Status.HolmesGPTRetryAttempts++
        aiAnalysis.Status.HolmesGPTLastError = err.Error()
        aiAnalysis.Status.HolmesGPTLastAttemptTime = &metav1.Time{Time: time.Now()}
        
        // Calculate next retry delay (exponential backoff)
        nextDelay := r.calculateBackoff(aiAnalysis.Status.HolmesGPTRetryAttempts)
        aiAnalysis.Status.HolmesGPTNextRetryTime = &metav1.Time{
            Time: time.Now().Add(nextDelay),
        }
        
        // Update status message
        aiAnalysis.Status.Message = fmt.Sprintf(
            "HolmesGPT retry (attempt %d, next in %s)",
            aiAnalysis.Status.HolmesGPTRetryAttempts,
            nextDelay,
        )
        aiAnalysis.Status.Reason = "HolmesGPTRetrying"
        
        // Update condition
        r.setCondition(aiAnalysis, metav1.Condition{
            Type:   "HolmesGPTAvailable",
            Status: metav1.ConditionFalse,
            Reason: "ConnectionTimeout",
            Message: fmt.Sprintf(
                "HolmesGPT-API connection timeout (attempt %d)",
                aiAnalysis.Status.HolmesGPTRetryAttempts,
            ),
        })
        
        if err := r.Status().Update(ctx, aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
        
        // Requeue with backoff delay
        log.Info("HolmesGPT retry scheduled",
            "attempt", aiAnalysis.Status.HolmesGPTRetryAttempts,
            "delay", nextDelay,
            "error", err.Error())
        
        return ctrl.Result{RequeueAfter: nextDelay}, nil
    }
    
    // Success - proceed to next phase
    aiAnalysis.Status.Phase = "analyzing"
    aiAnalysis.Status.RootCause = result.RootCause
    aiAnalysis.Status.Confidence = result.Confidence
    // ... populate other fields ...
    
    // Clear retry tracking
    aiAnalysis.Status.HolmesGPTRetryAttempts = 0
    aiAnalysis.Status.HolmesGPTLastError = ""
    
    // Update condition
    r.setCondition(aiAnalysis, metav1.Condition{
        Type:    "HolmesGPTAvailable",
        Status:  metav1.ConditionTrue,
        Reason:  "Success",
        Message: "HolmesGPT-API analysis successful",
    })
    
    return ctrl.Result{}, r.Status().Update(ctx, aiAnalysis)
}

func (r *AIAnalysisReconciler) calculateBackoff(attempts int) time.Duration {
    // Exponential backoff: initialDelay * (multiplier ^ (attempts - 1))
    delay := r.HolmesGPTInitialDelay * time.Duration(1<<(attempts-1))
    if delay > r.HolmesGPTMaxDelay {
        delay = r.HolmesGPTMaxDelay
    }
    return delay
}

func (r *AIAnalysisReconciler) failWithManualFallback(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    
    // Calculate total retry duration
    elapsed := time.Since(aiAnalysis.Status.StartedAt.Time)
    
    // Update status to failed
    aiAnalysis.Status.Phase = "failed"
    aiAnalysis.Status.Message = fmt.Sprintf(
        "HolmesGPT-API unavailable after %s (%d attempts)",
        elapsed.Round(time.Second),
        aiAnalysis.Status.HolmesGPTRetryAttempts,
    )
    aiAnalysis.Status.Reason = "HolmesGPTUnavailable"
    aiAnalysis.Status.HolmesGPTTotalRetryDuration = elapsed.Round(time.Second).String()
    
    // Enable manual fallback
    aiAnalysis.Status.RequiresApproval = true
    aiAnalysis.Status.ApprovalContext = &aianalysisv1.ApprovalContext{
        Reason:          "AI analysis unavailable - manual review required",
        ConfidenceLevel: "none",
        InvestigationSummary: fmt.Sprintf(
            "HolmesGPT-API was unavailable for %s. Manual root cause analysis required.",
            elapsed.Round(time.Second),
        ),
        EvidenceCollected: []string{
            fmt.Sprintf("HolmesGPT-API unreachable after %d retry attempts", aiAnalysis.Status.HolmesGPTRetryAttempts),
            fmt.Sprintf("Last error: %s", aiAnalysis.Status.HolmesGPTLastError),
            fmt.Sprintf("Total retry duration: %s", elapsed.Round(time.Second)),
        },
        RecommendedActions: []aianalysisv1.RecommendedAction{
            {
                Action:    "manual_investigation",
                Rationale: "AI analysis unavailable - requires human expertise",
            },
        },
        WhyApprovalRequired: "AI service unavailable - fallback to manual review",
    }
    
    // Update condition
    r.setCondition(aiAnalysis, metav1.Condition{
        Type:    "HolmesGPTAvailable",
        Status:  metav1.ConditionFalse,
        Reason:  "Timeout",
        Message: fmt.Sprintf(
            "HolmesGPT-API unavailable after %s (%d attempts)",
            elapsed.Round(time.Second),
            aiAnalysis.Status.HolmesGPTRetryAttempts,
        ),
    })
    
    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }
    
    // Create AIApprovalRequest for manual review
    if err := r.createManualApprovalRequest(ctx, aiAnalysis); err != nil {
        log.Error(err, "Failed to create manual approval request")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }
    
    log.Info("HolmesGPT unavailable - manual fallback triggered",
        "attempts", aiAnalysis.Status.HolmesGPTRetryAttempts,
        "duration", elapsed)
    
    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) createManualApprovalRequest(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {
    approvalRequest := &aiapprovalv1.AIApprovalRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-manual-review", aiAnalysis.Name),
            Namespace: aiAnalysis.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(aiAnalysis, aianalysisv1.GroupVersion.WithKind("AIAnalysis")),
            },
        },
        Spec: aiapprovalv1.AIApprovalRequestSpec{
            AIAnalysisRef: aiAnalysis.Name,
            Reason:        "HolmesGPT-API unavailable - manual analysis required",
            Priority:      "high",
            Timeout:       "30m", // Extended timeout for manual analysis
        },
    }
    
    return r.Create(ctx, approvalRequest)
}
```

---

## Prometheus Metrics

**New Metrics for HolmesGPT Availability**:

```go
// HolmesGPT availability metrics
holmesgpt_requests_total{status="success|failure|timeout"}
holmesgpt_retry_attempts_total
holmesgpt_retry_duration_seconds_bucket
holmesgpt_unavailability_incidents_total
holmesgpt_manual_fallback_total
```

**Example Prometheus Queries**:
```promql
# HolmesGPT success rate (last 5 minutes)
rate(holmesgpt_requests_total{status="success"}[5m]) 
  / 
rate(holmesgpt_requests_total[5m])

# Average retry attempts
avg(holmesgpt_retry_attempts_total)

# Manual fallback rate
rate(holmesgpt_manual_fallback_total[5m])
```

---

## Alerting Rules

**Critical Alert: HolmesGPT Unavailable**
```yaml
groups:
  - name: holmesgpt_availability
    rules:
      - alert: HolmesGPTUnavailable
        expr: rate(holmesgpt_requests_total{status="failure"}[5m]) > 0.5
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "HolmesGPT-API failure rate >50% for 5 minutes"
          description: "HolmesGPT-API may be down. Manual fallback triggered for {{ $value }} remediations."
      
      - alert: HolmesGPTHighRetryRate
        expr: rate(holmesgpt_retry_attempts_total[5m]) > 5
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "HolmesGPT retry rate >5/min"
          description: "HolmesGPT-API may be experiencing issues. Retry rate: {{ $value }}/min"
```

---

## Business Requirements

**New BRs for HolmesGPT Resilience**:

| BR | Description | Priority |
|---|---|---|
| **BR-AI-061** | AIAnalysis MUST retry HolmesGPT calls with exponential backoff for up to 5 minutes (configurable) | P0 |
| **BR-AI-062** | AIAnalysis MUST update status and conditions during retry to reflect HolmesGPT availability | P0 |
| **BR-AI-063** | AIAnalysis MUST fail with manual fallback after 5-minute timeout | P0 |
| **BR-AI-064** | AIAnalysis MUST create AIApprovalRequest with "manual review required" when HolmesGPT unavailable | P0 |
| **BR-AI-065** | AIAnalysis retry timeout MUST be configurable via ConfigMap or environment variable | P1 |

---

## Alternatives Considered

### **Alternative 1: Exponential Backoff with Manual Fallback (APPROVED)**

**Confidence**: **90%** ✅

**Pros**:
- ✅ Resilient to transient failures (network blips, service restarts)
- ✅ Clear observability (status reflects retry state)
- ✅ Bounded retry time (5 minutes)
- ✅ Manual fallback (system usable even if HolmesGPT down)
- ✅ Configurable timeout (adjust for different environments)

**Cons**:
- ⚠️ Adds complexity (~200 lines retry logic)
- ⚠️ Requires manual intervention after timeout
- **Mitigation**: Clear error messages, automated notification

---

### **Alternative 2: Fail Fast (No Retry)**

**Confidence**: **35%** ❌ **REJECTED**

**Pros**:
- ✅ Simple implementation
- ✅ Clear failure signal

**Cons**:
- ❌ No resilience to transient failures
- ❌ Complete outage if HolmesGPT unavailable
- ❌ High false positive rate (network blips)

---

### **Alternative 3: Infinite Retry**

**Confidence**: **25%** ❌ **REJECTED**

**Cons**:
- ❌ Remediation blocks indefinitely
- ❌ No manual fallback
- ❌ Resource exhaustion (goroutines accumulate)

---

### **Alternative 4: Queue for Later**

**Confidence**: **45%** ❌ **REJECTED**

**Cons**:
- ❌ Delayed response (hours if HolmesGPT down)
- ❌ Queue management complexity
- ❌ MTTR target missed (60+ min instead of 5 min)

---

## Testing Strategy

### **Unit Tests**

1. **Retry logic**: Verify exponential backoff calculation
2. **Timeout handling**: Verify manual fallback after 5 minutes
3. **Status updates**: Verify retry state reflected in status
4. **Condition updates**: Verify HolmesGPTAvailable condition

### **Integration Tests**

1. **Transient failure**: HolmesGPT down for 30s, then returns → retry succeeds
2. **Extended outage**: HolmesGPT down for 6 minutes → manual fallback
3. **Network blip**: 1-second timeout → retry succeeds on 2nd attempt
4. **Permanent failure**: HolmesGPT returns 500 error → manual fallback

### **Chaos Tests**

1. **Kill HolmesGPT pod**: Verify retry + recovery
2. **Network partition**: Verify timeout + manual fallback
3. **High latency**: Verify exponential backoff

---

## Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| **Retry Success Rate** | >80% | HolmesGPT calls succeed within 5 minutes |
| **Manual Fallback Rate** | <5% | AIApprovalRequest created due to HolmesGPT unavailability |
| **False Positive Rate** | <2% | Retries that succeed on first attempt |
| **Operator Clarity** | 100% | Status message clearly indicates retry state |

---

## References

1. **ADR-018**: Approval Notification Integration
2. **AIAnalysis Implementation Plan**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md`
3. **Business Requirements**: BR-AI-061 to BR-AI-065
4. **Exponential Backoff Algorithm**: Standard Kubernetes controller pattern

---

**Document Owner**: Platform Architecture Team  
**Last Updated**: 2025-10-17  
**Next Review**: After V1.0 implementation complete

