# BR-SCOPE-001: Resource Scope Management

**Business Requirement ID**: BR-SCOPE-001
**Title**: Resource Scope Management - Opt-In Model
**Category**: Security / Resource Management
**Priority**: P0 (Critical - Security Boundary)
**Status**: ✅ APPROVED
**Created**: 2026-01-19
**Last Updated**: 2026-01-20
**Owner**: Platform Team

> **Approved pre-GA amendment (2026-11-16, #2593):**
> [DD-FLEET-015](../architecture/decisions/DD-FLEET-015-owned-watch-and-enum-scope.md)
> supersedes older boolean/missing-target fallback examples below. Scope checks
> require observed target presence before namespace inheritance. Missing target
> or namespace evidence is unavailable, never verified unmanaged; invalid labels
> cannot inherit. The enum-only `Check` contract authorizes only
> `verified_managed`; cluster-scoped targets authorize through their own observed
> label (FR-SCOPE-004 Option A, #2670), and only the non-labelable
> `ClusterScope` pseudo-target remains deferred.
> Namespace observations still support namespaced inheritance. The approved
> [26-contract test plan](../tests/2593/TEST_PLAN.md) defines acceptance.

---

## 📋 Executive Summary

Kubernaut MUST provide operators with fine-grained control over which Kubernetes resources can be remediated. This is achieved through an **opt-in model** where operators explicitly mark resources as "managed by Kubernaut" using Kubernetes-native configuration mechanisms (labels, annotations, or CRDs).

**Core Principle**: **Kubernaut only remediates resources that are explicitly opted-in by operators.**

---

## 🎯 Business Need

### Problem Statement

In multi-tenant Kubernetes clusters, not all resources should be automatically remediated by Kubernaut:

1. **Shared Infrastructure**: System namespaces (`kube-system`, `istio-system`) may require manual intervention
2. **Tenant Isolation**: Each tenant may want independent control over auto-remediation
3. **Compliance Requirements**: Some workloads may require manual approval for all changes
4. **Gradual Rollout**: Operators want to enable Kubernaut incrementally (pilot namespaces first)
5. **Safety Boundary**: Prevent accidental remediation of critical resources

### Current Limitation

Without scope management, Kubernaut would:
- ❌ Process ALL signals from ALL namespaces (observability noise)
- ❌ Potentially remediate resources operators don't want automated
- ❌ Create RemediationRequest CRDs for unmanaged resources (wasted processing)
- ❌ Lack clear security boundary for auto-remediation scope

---

## ✅ Requirements

### FR-SCOPE-001: Namespace-Level Opt-In (V1.0)

**Requirement**: Operators MUST be able to mark namespaces as "managed by Kubernaut" using Kubernetes labels.

**Configuration**:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    kubernaut.ai/managed: "true"  # Opt-in to Kubernaut remediation
```

**Behavior**:
- Signals from namespaces with `kubernaut.ai/managed=true` → Processed by Kubernaut
- Signals from namespaces without label or with `kubernaut.ai/managed=false` → Rejected by Kubernaut

**Label Hierarchy** (2-Level):
- **Resource Label** (Priority 1): Check if the signal source resource has `kubernaut.ai/managed` label
  - `kubernaut.ai/managed=true` → Managed (explicit opt-in)
  - `kubernaut.ai/managed=false` → Unmanaged (explicit opt-out)
  - No label → Check namespace (inherit from parent)
- **Namespace Label** (Priority 2): Check if namespace has `kubernaut.ai/managed` label
  - `kubernaut.ai/managed=true` → Managed (inherited)
  - `kubernaut.ai/managed=false` → Unmanaged (inherited)
  - No label → Unmanaged (safe default)

**Rationale**: Namespace-level scope is the most common use case (tenant isolation, environment separation).

---

### FR-SCOPE-002: Signal Filtering at Gateway (V1.0)

**Requirement**: Gateway MUST reject signals from unmanaged resources to avoid unnecessary CRD creation.

**Enforcement Point**: Gateway signal ingestion

**Behavior**:
```
Signal arrives → Gateway validates scope → Unmanaged? → Reject (log, metric)
                                        → Managed? → Create RemediationRequest CRD
```

**Rationale**:
- Fail fast (no downstream processing for unmanaged signals)
- Reduce K8s API load (fewer CRDs created)
- Clear user feedback (immediate rejection with actionable message)

**Response Example**:
```json
{
  "status": "rejected",
  "message": "Resource production/deployment/payment-api is not managed by Kubernaut. Add label 'kubernaut.ai/managed=true' to namespace production to enable remediation."
}
```

**Observability**:
- Log: `INFO: Rejecting signal from unmanaged resource (targetResource: production/deployment/payment-api)`
- Metric: `gateway_signals_rejected_total{reason="unmanaged_resource"}`

---

### FR-SCOPE-003: Routing Filtering at RemediationOrchestrator (V1.0)

**Requirement**: RemediationOrchestrator MUST validate resource scope before creating WorkflowExecution to handle temporal drift.

**Enforcement Point**: RO routing engine (Check 6)

**Behavior**:
```
RR exists (created at T0) → RO validates scope at T60 → Unmanaged? → Block RR (retry)
                                                      → Managed? → Create WorkflowExecution
```

**Rationale**:
- Defense-in-depth (scope may change between Gateway validation and RO routing)
- Temporal drift protection (approval delays, manual label changes)
- Business decision auditing (RO logs routing block with reason)

**Failure Mode**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
status:
  overallPhase: Blocked
  blockReason: UnmanagedResource
  blockMessage: "Resource production/deployment/payment-api is not managed by Kubernaut at routing time. Scope may have changed since signal arrival."
```

**Retry Mechanism** (Automatic Reconciliation):
```
┌─────────────────────────────────────────────────────┐
│ Exponential Backoff Retry (Until RR Timeout)       │
└─────────────────────────────────────────────────────┘

T+0m:     RR blocked (UnmanagedResource)
          └─ Retry scheduled: 5 seconds

T+0m05s:  Retry #1 → Re-validate scope
          ├─ Still blocked? → Retry in 10 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+0m15s:  Retry #2 → Re-validate scope
          ├─ Still blocked? → Retry in 20 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+0m35s:  Retry #3 → Re-validate scope
          ├─ Still blocked? → Retry in 40 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+1m15s:  Retry #4 → Re-validate scope
          ├─ Still blocked? → Retry in 80 seconds
          └─ Unblocked? → Proceed with workflow ✅

...

T+Xm:     Retry #N → Re-validate scope
          ├─ Still blocked? → Retry in 300s (5 min max)
          └─ Unblocked? → Proceed with workflow ✅

T+60m:    RR global timeout reached
          └─ Status: TimedOut (terminal, user action required)
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
- Operators may be labeling resources/namespaces as part of batch operations
- Frequent retries early catch quick fixes (5s, 10s, 20s)
- Longer intervals later reduce API load (capped at 5 min)
- Automatic unblocking without user intervention (Kubernetes-native reconciliation)
- Global timeout (60 min) provides eventual failure for abandoned RRs

**API Call Cost**:
- **Namespaced Resource**: Max 2 GET calls per retry (resource + namespace)
- **Cluster-Scoped Resource**: 1 GET call per retry (resource only)
- **No caching** (V1.0 - rely on controller-runtime metadata-only cache for V2.0)

**Observability**:
- Audit Event: `orchestrator.routing.blocked` (event_action: "blocked", BlockReason: "UnmanagedResource")
- Metric: `remediation_requests_blocked_total{reason="unmanaged_resource"}`
- Log: `INFO: Retrying scope validation for RR rr-xyz (attempt N/12, next retry in 5m)`

---

### FR-SCOPE-004: Cluster-Scoped Resource Support (V2.0 - Future)

**Requirement**: Operators MUST be able to mark cluster-scoped resources (Nodes, PersistentVolumes) as managed by Kubernaut.

**Configuration Options** (TBD - requires design decision):

**Option A: Resource Labels/Annotations**
```yaml
apiVersion: v1
kind: Node
metadata:
  name: worker-node-1
  labels:
    kubernaut.ai/managed: "true"
```

**Option B: RemediationScope CRD**
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationScope
metadata:
  name: cluster-scope-config
spec:
  clusterScoped:
    - resourceType: Node
      selector:
        matchLabels:
          node-role.kubernetes.io/worker: "true"
    - resourceType: PersistentVolume
      selector:
        matchLabels:
          storage.kubernaut.io/managed: "true"
```

**Status**: Deferred to V2.0 (V1.0 focuses on namespace-scoped resources only)

---

### NFR-SCOPE-001: Performance

**Requirement**: Scope validation MUST NOT add significant latency to signal processing.

**Target**: < 10ms per scope validation (namespace label lookup)

**Implementation**: Use controller-runtime cached client for namespace lookups.

---

### NFR-SCOPE-002: Observability

**Requirement**: Operators MUST be able to observe scope validation decisions.

**Implementation**:
- Gateway: Prometheus metric `gateway_signals_rejected_total{reason="unmanaged_resource"}`
- Gateway: Structured logs with `targetResource`, `signalName`, `namespace`, `reason`
- RO: Audit event `orchestrator.routing.blocked` with `BlockReason: UnmanagedResource`
- RO: Prometheus metric `remediation_requests_blocked_total{reason="unmanaged_resource"}`

---

### NFR-SCOPE-003: Temporal Drift Acceptance

**Requirement**: Kubernaut MUST accept that scope can change at any time (eventual consistency).

**Rationale**: Operators can change labels via `kubectl` at any time. We cannot prevent this.

**Mitigation**:
- Multi-stage validation (Gateway + RO)
- Graceful failure with clear error messages
- Audit trail of scope validation at each stage
- Exponential backoff retry for automatic unblocking

**Accepted Risk**:
- Scope changes during PipelineRun execution (< 1 second window between RO validation and execution)
- Natural Kubernetes failures (RBAC, network policies) provide execution-time safety

---

### NFR-SCOPE-004: Notification for Unmanaged Resources (V1.0)

**Requirement**: Operators MUST be notified when remediation is blocked due to unmanaged resource status.

**Rationale**:
- **User Visibility**: Users must know why Kubernaut isn't remediating signals
- **Actionable Feedback**: Notification includes clear instructions (add label)
- **Consistency**: Same notification pattern as approval requests and self-mitigated remediations

**Notification Behavior**:
```
┌─────────────────────────────────────────────────────┐
│ Notification Flow for Unmanaged Resources          │
└─────────────────────────────────────────────────────┘

1. RO blocks RemediationRequest (UnmanagedResource)
2. RO emits audit event: orchestrator.routing.blocked
3. Notification service queries audit events
4. Notification service sends notification:
   - Title: "Remediation Blocked: Unmanaged Resource"
   - Body: "Resource production/deployment/payment-api is not managed by Kubernaut."
   - Action: "Add label 'kubernaut.ai/managed=true' to namespace or resource."
   - Priority: Medium
   - Channel: Configured by operator (Slack, PagerDuty, etc.)
5. User receives notification
6. User adds label (optional)
7. RO auto-retries → Unblocked? → Proceeds to execute
```

**Default Behavior**: **Notify by Default** (Option B)

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

**Success Criteria**:
- 100% of blocked RRs (UnmanagedResource) trigger notification
- Users can discover label requirement without reading documentation
- Opt-out is available but not default

---

## 🔄 Workflow Examples

### Example 1: Signal from Managed Namespace (Happy Path)

```
1. Alert fires: HighMemoryUsage in namespace "production"
2. Gateway receives signal
3. Gateway validates scope (2-level hierarchy):
   - Checks resource (Pod): No kubernaut.ai/managed label
   - Checks namespace "production": kubernaut.ai/managed=true
   - Scope validation: ✅ PASS (inherited from namespace)
4. Gateway creates RemediationRequest CRD
5. RR processes through SignalProcessing, AIAnalysis
6. RO validates scope (Check 6):
   - Re-validates namespace "production"
   - Scope validation: ✅ PASS
7. RO creates WorkflowExecution
8. WE executes remediation → ✅ SUCCESS
```

### Example 2: Signal from Unmanaged Namespace (Early Rejection)

```
1. Alert fires: HighMemoryUsage in namespace "kube-system"
2. Gateway receives signal
3. Gateway validates scope (2-level hierarchy):
   - Checks resource (Pod): No kubernaut.ai/managed label
   - Checks namespace "kube-system": No label or kubernaut.ai/managed=false
   - Scope validation: ❌ FAIL (unmanaged)
4. Gateway rejects signal:
   - HTTP 200 (not an error, a validation decision)
   - Response: "Resource not managed, add label to enable"
   - Log: INFO level
   - Metric: gateway_signals_rejected_total{reason="unmanaged_resource"}
5. No RemediationRequest created
6. No downstream processing
```

### Example 3: Temporal Drift (Scope Changes During Approval)

```
1. Alert fires: HighMemoryUsage in namespace "production" (T0)
2. Gateway validates scope: ✅ PASS (managed at T0)
3. Gateway creates RemediationRequest
4. RR processes through SignalProcessing, AIAnalysis
5. RemediationApprovalRequest created (requires manual approval)
6. Operator reviews for 30 minutes (T0 → T30)
7. Admin removes managed label (T20):
   kubectl label ns production kubernaut.ai/managed-
8. Operator approves (T30)
9. RO validates scope (Check 6, T30):
   - Re-validates namespace "production"
   - Scope validation: ❌ FAIL (unmanaged at T30)
10. RO blocks RemediationRequest:
    - Status.OverallPhase = "Blocked"
    - Status.BlockReason = "UnmanagedResource"
    - Audit: orchestrator.routing.blocked
11. RO begins exponential backoff retry:
    - Retry #1 at T+30m05s → Still unmanaged
    - Retry #2 at T+30m15s → Still unmanaged
    - ...
    - Retry #N at T+60m → RR times out (terminal state)
12. User receives notification (blocked + timeout)
```

### Example 4: Automatic Unblocking (Happy Path)

```
1. Alert fires: HighMemoryUsage in namespace "staging" (unmanaged)
2. Gateway rejects signal
3. Operator sees logs/metrics, realizes mistake
4. Operator labels namespace (T+2 minutes):
   kubectl label ns staging kubernaut.ai/managed=true
5. New signal arrives (T+3 minutes)
6. Gateway validates scope: ✅ PASS (now managed)
7. Gateway creates RemediationRequest
8. RR proceeds normally → ✅ SUCCESS
```

### Example 5: Resource-Level Override (2-Level Hierarchy)

```
1. Namespace "production" is managed (kubernaut.ai/managed=true)
2. Specific Deployment "legacy-app" is excluded (kubernaut.ai/managed=false)
3. Alert fires: HighMemoryUsage for Deployment/production/legacy-app
4. Gateway validates scope:
   - Checks Deployment: kubernaut.ai/managed=false (explicit opt-out)
   - Scope validation: ❌ FAIL (resource override)
5. Gateway rejects signal (respects resource-level label)
6. Other deployments in "production" are still managed
```

---

## 🔗 Dependencies

### Upstream Dependencies

- **Kubernetes API**: Namespace label lookups
- **controller-runtime**: Cached client for performance

### Downstream Impact

- **Gateway**: Add scope validation before CRD creation (FR-SCOPE-002)
- **RemediationOrchestrator**: Add routing check #6 for scope (FR-SCOPE-003)
- **Shared Package**: Create `pkg/shared/scope/manager.go` (reused by Gateway and RO)

---

## 📊 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Scope validation latency | < 10ms | Prometheus histogram |
| False rejections (managed signals rejected) | < 0.1% | `gateway_signals_rejected_total` / `gateway_signals_received_total` |
| Gateway efficiency (CRD reduction) | > 50% fewer CRDs for unmanaged signals | Compare before/after metrics |
| User satisfaction | > 90% operators find scope config intuitive | Survey after 3 months |

---

## 🚫 Out of Scope (V1.0)

The following are explicitly OUT OF SCOPE for V1.0:

1. ❌ **Cluster-Scoped Selection Policies**: `RemediationScope`-style selectors (FR-SCOPE-004 Option B) remain deferred to V2.0; per-object labels (Option A) are implemented (#2670)
2. ❌ **Resource-Level Granularity**: Per-deployment, per-pod scope (namespace-level only)
3. ❌ **Dynamic Scope Policies**: Rego policies for scope decisions (static labels only)
4. ❌ **Scope Inheritance**: Child resources inheriting parent scope (flat namespace model)
5. ❌ **Scope Change Notifications**: Proactive alerts when scope changes (reactive only)

---

## 🎯 Related Business Requirements

| BR ID | Title | Relationship |
|-------|-------|--------------|
| BR-GATEWAY-001 | Signal Ingestion | Gateway scope validation extends signal validation |
| BR-ORCH-001 | Remediation Routing | RO scope check is routing Check #6 |
| BR-SECURITY-001 | Least Privilege | Scope management is security boundary |
| BR-PLATFORM-001 | Kubernetes-Native | Uses native labels (not external config) |

---

## 📝 Implementation References

| Component | Implementation | Status |
|-----------|---------------|--------|
| **Shared Scope Manager** | `pkg/shared/scope/manager.go` | ⚠️ TODO |
| **Gateway Validation** | `pkg/gateway/server.go` (BR-SCOPE-002) | ⚠️ TODO |
| **RO Routing Check** | `pkg/remediationorchestrator/routing/scope_validator.go` (BR-SCOPE-010) | ⚠️ TODO |
| **API Types** | `api/remediation/v1alpha1/remediationrequest_types.go` (BlockReasonUnmanagedResource) | ⚠️ TODO |
| **Documentation** | `docs/user-guide/scope-management.md` | ⚠️ TODO |

---

## ✅ Approval

**Approved By**: Platform Team
**Date**: 2026-01-19
**Confidence**: 95%

**Approval Rationale**:
- ✅ Clear business need (multi-tenant safety)
- ✅ Kubernetes-native approach (labels)
- ✅ Defense-in-depth (Gateway + RO)
- ✅ Graceful failure (clear error messages)
- ✅ Observable (metrics, logs, audit)
- ✅ Extensible (V2.0 can add cluster-scoped, CRD-based config)

**Next Steps**:
1. Implement shared scope manager (`pkg/shared/scope/manager.go`)
2. Implement Gateway validation (FR-SCOPE-002)
3. Implement RO routing check (FR-SCOPE-003)
4. Add unit and integration tests
5. Update user documentation

---

**Document Version**: 1.0
**Last Updated**: 2026-01-19
**Next Review**: 2026-04-19 (3 months)
