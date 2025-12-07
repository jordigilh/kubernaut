# AIAnalysis Error Handling Philosophy

**Version**: v1.0
**Created**: 2025-12-06
**Service**: AIAnalysis Controller
**Status**: ✅ Active

---

## 📋 **Overview**

This document defines the error handling philosophy for the AIAnalysis service, including error classification, recovery strategies, and graceful degradation patterns.

**Design Principles**:
1. **Business continuity** over strict correctness - don't block remediation for non-critical errors
2. **Fail-safe defaults** - when uncertain, require human approval
3. **Observability** - all errors are logged and tracked, even if gracefully degraded
4. **Same-cluster optimization** - leverage co-located services, avoid redundant client-side metrics

---

## 🏷️ **Error Classification**

### **By Recoverability**

| Type | Description | Action | Example |
|------|-------------|--------|---------|
| **Transient** | Temporary failures that may resolve | Retry with backoff | Network timeout, 503 from HAPI |
| **Permanent** | Failures that won't resolve with retry | Fail immediately | 404 workflow not found, invalid input |
| **User** | Requires human intervention | Set `ApprovalRequired=true` | Low confidence, policy violation |

### **By Severity**

| Severity | Impact | Response |
|----------|--------|----------|
| **Critical** | Analysis cannot proceed | Transition to `Failed` phase |
| **Warning** | Analysis can proceed with caveats | Set `DegradedMode=true`, continue |
| **Info** | Minor issue, no impact | Log only |

---

## 📂 **Service-Specific Error Categories**

### **Category A: CRD Not Found**
**Trigger**: `client.IgnoreNotFound(err)` returns non-nil
**Behavior**: Normal during deletion - no action needed
**Recovery**: None (expected behavior)
**Metric**: Not tracked (normal operation)

```go
// Category A: CRD deletion in progress
if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
    return ctrl.Result{}, client.IgnoreNotFound(err) // Normal
}
```

### **Category B: HolmesGPT-API Errors**
**Trigger**: HTTP errors from `/api/v1/incident/analyze`
**Behavior**: Retry with exponential backoff (3 attempts, 1s→2s→4s)
**Recovery**: Requeue with backoff on transient; fail on permanent
**Metric**: `aianalysis_failures_total{reason="APIError", sub_reason="TransientError|PermanentError"}`

| Status Code | Classification | Action |
|-------------|----------------|--------|
| 2xx | Success | Process response |
| 400 | Permanent | Fail immediately (bad request) |
| 401/403 | Permanent | Fail immediately (auth error) |
| 404 | Permanent | Fail immediately (endpoint not found) |
| 429 | Transient | Retry with backoff (rate limited) |
| 500 | Transient | Retry with backoff |
| 502/503/504 | Transient | Retry with backoff |
| Timeout | Transient | Retry with backoff |

```go
// Category B: HolmesGPT-API error handling
resp, err := h.client.AnalyzeIncident(ctx, request)
if err != nil {
    if isTransientError(err) {
        metrics.RecordFailure("APIError", "TransientError")
        return ctrl.Result{RequeueAfter: backoff.Next()}, nil // Retry
    }
    metrics.RecordFailure("APIError", "PermanentError")
    return r.transitionToFailed(ctx, analysis, "APIError", err.Error())
}
```

### **Category C: Rego Policy Errors**
**Trigger**: Policy evaluation fails (syntax error, missing policy, timeout)
**Behavior**: Graceful degradation - default to `ApprovalRequired=true`
**Recovery**: Continue with degraded mode
**Metric**: `aianalysis_rego_evaluations_total{outcome="error", degraded="true"}`

**Graceful Degradation Rationale**: A policy engine failure should not block remediation. Defaulting to human approval is safer than blocking entirely.

```go
// Category C: Rego policy error with graceful degradation
result, err := r.regoEvaluator.Evaluate(ctx, input)
if err != nil {
    log.Error(err, "Rego policy evaluation failed, defaulting to approval required")
    metrics.RecordRegoEvaluation("error", true) // degraded=true
    analysis.Status.ApprovalRequired = true
    analysis.Status.ApprovalReason = "Policy evaluation failed - defaulting to manual review"
    analysis.Status.DegradedMode = true
    // Continue processing - don't fail
}
```

### **Category D: Status Update Conflicts**
**Trigger**: Optimistic locking conflict on status update
**Behavior**: Requeue immediately (controller-runtime handles this)
**Recovery**: Automatic retry on next reconciliation
**Metric**: Not explicitly tracked (controller-runtime manages)

```go
// Category D: Status update conflict
if err := r.Status().Update(ctx, analysis); err != nil {
    if apierrors.IsConflict(err) {
        return ctrl.Result{Requeue: true}, nil // Retry immediately
    }
    return ctrl.Result{}, err
}
```

### **Category E: Audit/Observability Errors**
**Trigger**: Audit store write failure, metrics registration failure
**Behavior**: Fire-and-forget - log error but don't fail reconciliation
**Recovery**: None needed (observability is non-critical)
**Metric**: Not tracked (would create infinite loop)

**Fire-and-Forget Rationale**: Audit failures should never block business operations. The audit system is for compliance/debugging, not for correctness.

```go
// Category E: Audit error with fire-and-forget
if err := c.store.StoreAudit(ctx, event); err != nil {
    c.log.Error(err, "Failed to write audit event",
        "event_type", event.EventType,
        "correlation_id", event.CorrelationID,
    )
    // Don't return error - continue processing
}
```

---

## 🔄 **Graceful Degradation Matrix**

| Component | Failure Mode | Degraded Behavior | User Impact |
|-----------|--------------|-------------------|-------------|
| **HolmesGPT-API** | Timeout | Retry 3x, then fail | Analysis delayed or failed |
| **HolmesGPT-API** | Low confidence | Set `ApprovalRequired=true` | Human review required |
| **Rego Policy Engine** | Evaluation error | Default to approval required | Human review required |
| **Rego Policy Engine** | Policy not found | Default to approval required | Human review required |
| **Audit Store** | Write failure | Log error, continue | Audit trail incomplete |
| **Metrics** | Registration failure | Log error, continue | Metrics may be missing |
| **K8s API** | Status update conflict | Requeue, retry | Brief delay |

---

## 📊 **Error Metrics Strategy**

### **What We Track (Business Value)**

| Metric | When Recorded | Purpose |
|--------|---------------|---------|
| `aianalysis_failures_total` | Any failure | Track failure modes |
| `aianalysis_rego_evaluations_total` | Policy evaluation | Track degradation rate |

### **What We Don't Track (v1.13 Decision)**

| Metric | Reason Not Tracked |
|--------|-------------------|
| Client-side HAPI latency | HAPI tracks server-side; same-cluster deployment |
| Client-side HAPI requests | HAPI tracks server-side; redundant |
| Client-side HAPI retries | Debugging only; not business metric |
| Rego latency | Fast operation (<50ms); debugging only |
| Phase transitions | Debugging only; not business outcome |

**Rationale**: In a same-cluster deployment, client-side metrics for HAPI provide no additional value over HAPI's server-side metrics. Network latency between co-located services is negligible.

---

## 🛡️ **Fail-Safe Defaults**

When in doubt, the AIAnalysis controller defaults to the **safer option**:

| Uncertainty | Default | Rationale |
|-------------|---------|-----------|
| Confidence < 70% | `ApprovalRequired=true` | Low confidence = human review |
| Rego policy fails | `ApprovalRequired=true` | Policy failure = human review |
| HAPI returns warnings | `ApprovalRequired=true` | Warnings = human review |
| Target not in owner chain | `ApprovalRequired=true` | Data quality concern |
| Multiple recovery attempts | `ApprovalRequired=true` | Escalation required |

---

## 🔗 **Integration with BR-HAPI-197**

When HolmesGPT-API returns `needs_human_review=true`:

| `human_review_reason` | `SubReason` | Operator Action |
|-----------------------|-------------|-----------------|
| `workflow_not_found` | `WorkflowNotFound` | Check workflow catalog |
| `image_mismatch` | `ImageMismatch` | Verify OCI image |
| `parameter_validation_failed` | `ParameterValidationFailed` | Check workflow params |
| `no_matching_workflows` | `NoMatchingWorkflows` | Review signal type |
| `low_confidence` | `LowConfidence` | Manual analysis |
| `llm_parsing_error` | `LLMParsingError` | LLM issue (HAPI exhausted retries) |

**Phase Transition**: `Investigating` → `Failed` with `Reason=WorkflowResolutionFailed`

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [DD-AUDIT-002](../../../../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md) | Audit fire-and-forget pattern |
| [BR-HAPI-197](../../../../requirements/BR-HAPI-197-needs-human-review-field.md) | Human review scenarios |
| [reconciliation-phases.md](../../reconciliation-phases.md) | Phase failure handling |
| [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md) | v1.13 metrics rationale |

---

## ✅ **Checklist for Error Handling**

When adding new error handling:

- [ ] Classify error (Transient/Permanent/User)
- [ ] Assign to category (A-E)
- [ ] Implement appropriate recovery strategy
- [ ] Add logging with context
- [ ] Update `aianalysis_failures_total` if business-relevant
- [ ] Consider graceful degradation if non-critical
- [ ] Document in this file if new pattern


