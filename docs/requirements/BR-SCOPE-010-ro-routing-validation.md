# BR-SCOPE-010: RemediationOrchestrator Routing Scope Validation

**Business Requirement ID**: BR-SCOPE-010
**Title**: RemediationOrchestrator Routing Scope Validation with Automatic Retry
**Category**: Routing / Resource Management
**Priority**: P0 (Critical - Defense-in-Depth)
**Status**: ✅ APPROVED
**Created**: 2026-01-20
**Last Updated**: 2026-01-20
**Owner**: RemediationOrchestrator Team
**Parent BR**: BR-SCOPE-001 (Resource Scope Management)

---

## 📋 Executive Summary

RemediationOrchestrator MUST validate resource scope before creating WorkflowExecution to handle temporal drift scenarios where scope changes between Gateway validation and RO routing.

**Core Principle**: **Defense-in-depth with automatic retry - scope can change at any time.**

---

## 🎯 Business Need

### Problem Statement

Without RO-level scope validation:
- ❌ Temporal drift: Namespace/resource labels can change after Gateway validation
- ❌ Manual approval delays: Scope may change during 30-minute approval window
- ❌ Execution on unmanaged resources: No safety net if scope changes between stages
- ❌ Poor user experience: No feedback when scope changes invalidate remediation

### Real-World Scenario

```
T+0m:  Signal arrives, namespace "production" is managed
T+0m:  Gateway validates scope ✅, creates RemediationRequest
T+10m: RemediationApprovalRequest created (requires human approval)
T+20m: Admin accidentally removes label: kubectl label ns production kubernaut.ai/managed-
T+30m: Operator approves remediation
T+30m: ❌ PROBLEM: RO would execute on unmanaged namespace without validation
```

**Solution**: RO re-validates scope before creating WorkflowExecution + automatic retry.

---

## ✅ Requirements

### FR-SCOPE-010-1: Routing Check #6 - Scope Validation (V1.0)

**Requirement**: RemediationOrchestrator MUST add a 6th routing check to validate that the target resource is managed before creating WorkflowExecution.

**Integration Point**: `pkg/remediationorchestrator/routing/blocking.go` (`CheckBlockingConditions()`)

**Check Order**:
```
RO Routing Checks (Sequential):
1. Consecutive Failures Check
2. Resource Busy Check
3. Recently Remediated Check
4. Exponential Backoff Check
5. Duplicate In Progress Check
6. Scope Validation Check (NEW - FR-SCOPE-010-1)
   ├─ Is resource managed?
   ├─ YES → Proceed to create WorkflowExecution
   └─ NO → Block RR (BlockReasonUnmanagedResource) + Retry
```

**Validation Logic** (RCA Target with Fallback):
```go
// pkg/remediationorchestrator/routing/scope_validator.go
import "github.com/jordigilh/kubernaut/pkg/shared/scope"

func (r *Reconciler) CheckManagedResource(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) (bool, error) {
    // Get AIAnalysis CRD to check for RCA-determined target resource (BR-AI-084)
    aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      rr.Status.ChildCRDs.AIAnalysis.Name,
        Namespace: rr.Status.ChildCRDs.AIAnalysis.Namespace,
    }, aiAnalysis)
    if err != nil {
        return false, fmt.Errorf("failed to get AIAnalysis CRD: %w", err)
    }

    // Determine target resource for scope validation (DD-HAPI-006, BR-HAPI-212)
    // Priority: RCA-determined target > Signal source
    var targetNamespace, targetKind, targetAPIVersion, targetName string

    if aiAnalysis.Status.RootCauseAnalysis != nil && aiAnalysis.Status.RootCauseAnalysis.TargetResource != nil {
        // Use RCA-determined target
        // This is the resource HolmesGPT identified as the root cause
        targetResource := aiAnalysis.Status.RootCauseAnalysis.TargetResource
        targetNamespace = targetResource.Namespace
        targetKind = targetResource.Kind
        targetAPIVersion = targetResource.APIVersion  // Optional - may be empty
        targetName = targetResource.Name
    } else {
        // NO FALLBACK: If RCA target is missing, AIAnalysis should have set needs_human_review=true
        // This code path should not be reached (RO should have created NotificationRequest instead of reaching routing)
        return false, fmt.Errorf("RCA target resource missing - AIAnalysis.Status.RootCauseAnalysis.TargetResource is nil (escalation required)")
    }

    // Resolve GVK from Kind + apiVersion (DD-HAPI-006)
    // If apiVersion is empty, use static mapping for core Kubernetes resources
    gvk, err := r.resolveGVK(targetKind, targetAPIVersion)
    if err != nil {
        return false, fmt.Errorf("failed to resolve GVK for kind=%s apiVersion=%s: %w", targetKind, targetAPIVersion, err)
    }

    // Validate scope using shared scope manager (same logic as Gateway)
    isManaged, err := r.scopeManager.IsManaged(ctx, targetNamespace, gvk, targetName)
    if err != nil {
        return false, fmt.Errorf("failed to validate resource scope: %w", err)
    }

    return isManaged, nil
}

// NEW: resolveGVK resolves GroupVersionKind from Kind + apiVersion
// If apiVersion is empty, uses static mapping for core Kubernetes resources
// If apiVersion is provided, parses it to extract Group and Version
func (r *Reconciler) resolveGVK(kind, apiVersion string) (schema.GroupVersionKind, error) {
    if apiVersion != "" {
        // apiVersion provided - parse it
        // Format: "apps/v1" or "v1" (for core resources)
        return parseAPIVersion(apiVersion, kind)
    }

    // apiVersion missing - use static mapping for core resources
    return getGVKForKind(kind), nil
}

// getGVKForKind returns GVK for core Kubernetes resources (static mapping)
func getGVKForKind(kind string) schema.GroupVersionKind {
    // Map of kinds to their API groups (same as pkg/signalprocessing/ownerchain/builder.go)
    kindToGroup := map[string]string{
        "Pod":                "",        // Core API (v1)
        "Node":               "",        // Core API (v1)
        "Service":            "",        // Core API (v1)
        "ConfigMap":          "",        // Core API (v1)
        "Secret":             "",        // Core API (v1)
        "PersistentVolume":   "",        // Core API (v1)
        "PersistentVolumeClaim": "",    // Core API (v1)
        "Deployment":         "apps",    // apps/v1
        "StatefulSet":        "apps",    // apps/v1
        "DaemonSet":          "apps",    // apps/v1
        "ReplicaSet":         "apps",    // apps/v1
        "Job":                "batch",   // batch/v1
        "CronJob":            "batch",   // batch/v1
        "Ingress":            "networking.k8s.io",  // networking.k8s.io/v1
        "ServiceMonitor":     "monitoring.coreos.com",  // monitoring.coreos.com/v1
    }

    group := kindToGroup[kind]
    version := "v1"  // Default version for all core and common resources

    return schema.GroupVersionKind{
        Group:   group,
        Version: version,
        Kind:    kind,
    }
}
```

---

### FR-SCOPE-010-2: Blocked State with Exponential Backoff Retry (V1.0)

**Requirement**: When RO detects an unmanaged resource, it MUST block the RemediationRequest with automatic exponential backoff retry until scope label is added or RR times out.

**State Transition**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
status:
  overallPhase: Blocked  # NON-terminal (allows Gateway deduplication)
  blockReason: UnmanagedResource
  blockMessage: "Resource production/deployment/payment-api is not managed by Kubernaut at routing time (T+30m). Namespace 'production' does not have label 'kubernaut.ai/managed=true'. Add label to unblock."
  blockTimestamp: "2026-01-20T10:30:00Z"
  retryAttempts: 3
  nextRetryTime: "2026-01-20T10:31:00Z"  # Next retry after 80s backoff
```

**Retry Behavior**:
```
┌─────────────────────────────────────────────────────┐
│ Exponential Backoff Retry (Until RR Timeout)       │
└─────────────────────────────────────────────────────┘

T+0m:     RR blocked (UnmanagedResource)
          └─ Schedule retry: 5 seconds
          └─ Metric: remediation_requests_blocked_total{reason="unmanaged_resource"}++

T+0m05s:  Retry #1 → Re-validate scope
          ├─ Still blocked? → Schedule retry in 10s
          └─ Unblocked? → Proceed with workflow ✅

T+0m15s:  Retry #2 → Re-validate scope
          ├─ Still blocked? → Schedule retry in 20s
          └─ Unblocked? → Proceed with workflow ✅

T+0m35s:  Retry #3 → Re-validate scope
          ├─ Still blocked? → Schedule retry in 40s
          └─ Unblocked? → Proceed with workflow ✅

T+1m15s:  Retry #4 → Re-validate scope
          ├─ Still blocked? → Schedule retry in 80s
          └─ Unblocked? → Proceed with workflow ✅

...

T+Xm:     Retry #N → Re-validate scope
          ├─ Still blocked? → Schedule retry in 300s (5 min max)
          └─ Unblocked? → Proceed with workflow ✅

T+60m:    RR global timeout reached
          └─ Status: TimedOut (terminal, user action required)
          └─ Notification: "RR timed out while blocked (UnmanagedResource)"
```

**Configuration** (RemediationOrchestrator Config):
```yaml
retryConfig:
  unmanagedResource:
    initialInterval: 5s         # First retry after 5 seconds
    maxInterval: 300s           # Cap at 5 minutes per retry
    multiplier: 2.0             # Double the interval each retry
```

**Rationale for Exponential Backoff**:
- **Early Retries**: Catch quick fixes (5s, 10s, 20s) when operators are actively labeling
- **Graduated Backoff**: Reduce API load as retries continue (40s, 80s, 160s, 300s)
- **Max Cap**: 5 minutes per retry balances responsiveness and API efficiency
- **Global Timeout**: 60 minutes provides eventual failure (prevents infinite retry)
- **Automatic Unblocking**: No user intervention required (Kubernetes-native reconciliation)

---

### FR-SCOPE-010-3: Audit Event Emission (V1.0)

**Requirement**: RO MUST emit an audit event when blocking a RemediationRequest due to unmanaged resource status.

**Audit Event Type**: `orchestrator.routing.blocked` (reuse existing event type)

**Audit Payload**:
```json
{
  "event_type": "orchestrator.routing.blocked",
  "event_action": "blocked",
  "timestamp": "2026-01-20T10:30:00Z",
  "correlation_id": "rr-a1b2c3d4e5f6-12345678",
  "event_data": {
    "block_reason": "UnmanagedResource",
    "resource": {
      "namespace": "production",
      "kind": "Deployment",
      "name": "payment-api"
    },
    "validation_time": "2026-01-20T10:30:00Z",
    "label_found": false,
    "retry_scheduled": true,
    "next_retry_time": "2026-01-20T10:30:05Z"
  }
}
```

**Rationale**:
- Reuse existing event type (consolidates blocked reasons)
- Clear audit trail for scope validation decisions
- Enables notification service to trigger user alerts

---

### FR-SCOPE-010-4: Notification Integration (V1.0)

**Requirement**: Users MUST be notified when remediation is blocked due to unmanaged resource status.

**Notification Flow**:
```
1. RO blocks RR (UnmanagedResource)
2. RO emits audit event: orchestrator.routing.blocked
3. Notification service queries audit events (polling or webhook)
4. Notification service sends notification:
   - Title: "Remediation Blocked: Unmanaged Resource"
   - Body: "Resource production/deployment/payment-api is not managed by Kubernaut."
   - Action: "Add label 'kubernaut.ai/managed=true' to namespace 'production' or resource to unblock."
   - Priority: Medium
   - Channel: Configured by operator (Slack, PagerDuty, email)
5. RO continues retrying (automatic)
6. If user adds label → RO auto-unblocks → Proceeds to execute ✅
```

**Default Behavior**: **Notify by Default**

**Rationale**:
- Users **MUST** know why remediation isn't happening
- Same visibility as approval requests and self-mitigated remediations
- Configuration limitations should not be hidden
- Users can opt-out via notification configuration if desired

**Opt-Out Configuration** (Notification Service Config):
```yaml
notificationRules:
  - event: orchestrator.routing.blocked
    conditions:
      - blockReason: UnmanagedResource
    channels:
      - slack
      - pagerduty
    enabled: true  # Set to false to opt-out
```

---

### NFR-SCOPE-010-1: Performance (V1.0)

**Requirement**: Scope validation MUST NOT add significant latency to RR routing.

**Target SLA**:
- < 10ms per scope validation (P95)
- < 5ms per cached lookup (P50)

**API Call Cost**:
- **Namespaced Resource**: Max 2 GET calls per retry (resource + namespace)
- **Cluster-Scoped Resource**: 1 GET call per retry (resource only)
- **No caching** (V1.0 - rely on controller-runtime metadata-only cache for V2.0)

**Retry Frequency**:
```
Worst-case API calls (1 blocked RR over 60 minutes):
- Retries: 12 attempts (5s, 10s, 20s, 40s, 80s, 160s, 300s × 7)
- API calls per retry: 2 (resource + namespace)
- Total: 24 GET calls over 60 minutes = 0.4 GET/minute

At scale (100 blocked RRs):
- Total: 2,400 GET calls over 60 minutes = 40 GET/minute = 0.67 GET/second
```

**Rationale**: Acceptable API load for defensive validation.

---

### NFR-SCOPE-010-2: Deduplication Compatibility (V1.0)

**Requirement**: Blocked RRs MUST remain in non-terminal state to enable Gateway deduplication.

**Behavior**:
```
Scenario: User receives 10 duplicate signals while RR is blocked

Without Blocked state (terminal failure):
- Gateway creates 10 new RRs (flood)
- Each RR is blocked by RO (10× validation overhead)

With Blocked state (non-terminal):
- Gateway deduplicates against existing Blocked RR (fingerprint match)
- Only 1 RR in system (efficient)
- Occurrence count incremented (preserves signal frequency data)
```

**Reference**: DD-RO-002-ADDENDUM (Blocked Phase Semantics)

---

## 🔄 Routing Flow with Scope Validation

### Flow Diagram

```
┌─────────────────────────────────────────────────────┐
│ RO Routing with Scope Validation (Check #6)        │
└─────────────────────────────────────────────────────┘

1. RemediationRequest created (Gateway validated scope at T0)
2. RR processes through SignalProcessing, AIAnalysis
3. RO routing engine begins (Check 1-6):

   Check 1: Consecutive Failures?
   ├─ YES → Block (ConsecutiveFailures)
   └─ NO → Continue

   Check 2: Resource Busy?
   ├─ YES → Block (ResourceBusy)
   └─ NO → Continue

   Check 3: Recently Remediated?
   ├─ YES → Block (RecentlyRemediated)
   └─ NO → Continue

   Check 4: Exponential Backoff?
   ├─ YES → Block (ExponentialBackoff)
   └─ NO → Continue

   Check 5: Duplicate In Progress?
   ├─ YES → Block (DuplicateInProgress)
   └─ NO → Continue

  Check 6: Human Review Escalation (NEW)
  ├─ Get AIAnalysis CRD
  │   ├─ NeedsHumanReview = true? (HAPI decision: RCA incomplete)
  │   │   ├─ YES → Create NotificationRequest (NO remediation plan)
  │   │   └─ NO → Continue to Check 7

  Check 7: Approval Requirement (NEW)
  ├─ ApprovalRequired = true? (Rego decision: high-risk remediation)
  │   ├─ YES → Create RemediationApprovalRequest (HAS remediation plan, awaiting approval)
  │   └─ NO → Continue to Check 8

  Check 8: Scope Validation (NEW)
  ├─ Get RCA target resource
  │   ├─ Has RCA target? (AIAnalysis.Status.RootCauseAnalysis.TargetResource)
  │   │   ├─ YES → Use RCA-determined target (BR-AI-084, DD-HAPI-006)
  │   │   └─ NO → ERROR: Escalation required (should not reach this point - HAPI should have set needs_human_review)
   │   │
   │   ├─ Is target resource managed? (re-validate at T60)
   │   │   ├─ Resource label: kubernaut.ai/managed=true → MANAGED
   │   │   ├─ Resource label: kubernaut.ai/managed=false → UNMANAGED
   │   │   ├─ Namespace label: kubernaut.ai/managed=true → MANAGED
   │   │   ├─ Namespace label: kubernaut.ai/managed=false → UNMANAGED
   │   │   └─ No label → UNMANAGED
   │   │
   │   ├─ UNMANAGED → Block (UnmanagedResource) + Schedule Retry
   │   └─ MANAGED → Proceed to 4

4. All routing checks passed → Create WorkflowExecution
5. WE executes remediation → SUCCESS
```

---

## 📊 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Temporal Drift Detection** | 100% unmanaged resources blocked | Audit events |
| **Auto-Unblock Success Rate** | > 80% blocked RRs unblock before timeout | `blocked_duration` histogram |
| **Latency Impact** | < 10ms added latency (P95) | Prometheus histogram |
| **Notification Delivery** | 100% blocked RRs trigger notification | Notification service logs |
| **User Satisfaction** | > 90% operators find retry behavior helpful | Survey after 3 months |

---

## 🔗 Dependencies

### Upstream Dependencies

- **BR-SCOPE-001**: Resource Scope Management (parent BR)
- **BR-SCOPE-002**: Gateway Signal Filtering (first validation layer)
- **BR-AI-084**: AIAnalysis Extract RCA Target Resource (provides RCA-determined target)
- **BR-HAPI-212**: HAPI RCA Target Resource (HAPI returns affectedResource)
- **DD-HAPI-006**: Affected Resource in Root Cause Analysis (design decision)
- **DD-RO-002-ADDENDUM**: Blocked Phase Semantics (non-terminal state)
- **Kubernetes API**: Namespace/resource label lookups

### Downstream Impact

- **WorkflowExecution**: Fewer WE CRDs created for unmanaged resources
- **Notification Service**: New notification type for blocked RRs
- **DataStorage**: Audit events for blocked RRs
- **Prometheus**: New metric `remediation_requests_blocked_total{reason="unmanaged_resource"}`

---

## 🚫 Out of Scope (V1.0)

1. ❌ **Caching**: Use direct K8s API calls (not controller-runtime cache) for V1.0
2. ❌ **Rego Policies**: Dynamic scope decisions (static labels only)
3. ❌ **Scope Inheritance**: Child resources inheriting parent scope (flat 2-level model)
4. ❌ **Proactive Alerts**: Notifications when scope labels change (reactive only)

---

## 🎯 Related Business Requirements

| BR ID | Title | Relationship |
|-------|-------|--------------|
| BR-SCOPE-001 | Resource Scope Management | Parent BR (defines opt-in model) |
| BR-SCOPE-002 | Gateway Signal Filtering | Defense-in-depth Layer 1 (Gateway filters) |
| BR-AI-084 | AIAnalysis Extract RCA Target | Provides RCA-determined target resource |
| BR-HAPI-212 | HAPI RCA Target Resource | HAPI returns affectedResource in RCA |
| DD-HAPI-006 | Affected Resource in RCA | Design decision for RCA target |
| DD-RO-002-ADDENDUM | Blocked Phase Semantics | UnmanagedResource is 6th blocking scenario |
| BR-ORCH-001 | Remediation Routing | RO routing check #6 |

---

## 📝 Implementation References

| Component | Implementation | Status |
|-----------|---------------|--------|
| **Shared Scope Manager** | `pkg/shared/scope/manager.go` | ⚠️ TODO |
| **RO Scope Validator** | `pkg/remediationorchestrator/routing/scope_validator.go` | ⚠️ TODO |
| **RO Routing Integration** | `pkg/remediationorchestrator/routing/blocking.go` (Check #6) | ⚠️ TODO |
| **API Types** | `api/remediation/v1alpha1/remediationrequest_types.go` (BlockReasonUnmanagedResource) | ✅ DONE |
| **DD-RO-002 Update** | `docs/architecture/decisions/DD-RO-002-ADDENDUM.md` | ⚠️ TODO |
| **Unit Tests** | `test/unit/remediationorchestrator/scope_validation_test.go` | ⚠️ TODO |
| **Integration Tests** | `test/integration/remediationorchestrator/scope_blocking_test.go` | ⚠️ TODO |

---

## ✅ Approval

**Approved By**: RemediationOrchestrator Team, Platform Team
**Date**: 2026-01-20
**Confidence**: 95%

**Approval Rationale**:
- ✅ Defense-in-depth (second validation layer after Gateway)
- ✅ Temporal drift protection (scope can change at any time)
- ✅ Automatic retry (Kubernetes-native reconciliation)
- ✅ Clear user feedback (notification + actionable message)
- ✅ Non-terminal blocking (Gateway deduplication works)
- ✅ Observable (metrics, logs, audit events)

**Next Steps**:
1. Implement shared scope manager (`pkg/shared/scope/manager.go`)
2. Implement RO scope validator (`pkg/remediationorchestrator/routing/scope_validator.go`)
3. Integrate scope check into `CheckBlockingConditions()` (Check #6)
4. Add exponential backoff retry logic to RO reconciler
5. Update DD-RO-002-ADDENDUM to document 6th blocking scenario
6. Add unit and integration tests
7. Update RO user documentation

---

**Document Version**: 1.0
**Last Updated**: 2026-01-20
**Next Review**: 2026-04-20 (3 months)
