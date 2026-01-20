# BR-SCOPE-001 Complete Definition - Jan 20, 2026

**Date**: January 20, 2026
**Confidence**: 95%
**Status**: ✅ APPROVED - Ready for Implementation

---

## 📋 Executive Summary

Successfully defined comprehensive Business Requirements for **Resource Scope Management** in Kubernaut. This establishes an opt-in model where operators explicitly control which Kubernetes resources can be remediated using the `kubernaut.ai/managed` label.

---

## ✅ Completed Work

### 1. Architecture Decision Record Created

#### **ADR-053: Resource Scope Management Architecture**
- **File**: `docs/architecture/decisions/ADR-053-resource-scope-management.md`
- **Status**: ✅ APPROVED
- **Confidence**: 95%
- **Key Sections**:
  - ✅ 7 architectural decisions (opt-in model, 2-level hierarchy, defense-in-depth, exponential backoff, metadata cache, non-terminal blocking, no Gateway audit events)
  - ✅ 6 alternatives considered and rejected (owner chain, existence-only labels, Gateway-only, fixed-interval, terminal failure, RemediationScope CRD)
  - ✅ Performance analysis (0 API calls for Gateway, 0.67 GET/second for RO at scale)
  - ✅ Migration plan (4-week phased rollout)
  - ✅ 7 performance metrics with targets

### 2. Business Requirements Created

#### **BR-SCOPE-001: Resource Scope Management**
- **File**: `docs/requirements/BR-SCOPE-001-resource-scope-management.md`
- **Category**: Security / Resource Management
- **Priority**: P0 (Critical)
- **Key Features**:
  - ✅ 2-Level label hierarchy (Resource → Namespace)
  - ✅ Value-based labels (`kubernaut.ai/managed="true"`)
  - ✅ Exponential backoff retry (5s → 5min max)
  - ✅ Notification by default for unmanaged resources
  - ✅ Non-terminal Blocked phase (Gateway deduplication)
  - ✅ Controller-runtime metadata-only cache (V2.0)
  - ✅ Defense-in-depth (Gateway + RO validation)

#### **BR-SCOPE-002: Gateway Signal Filtering**
- **File**: `docs/requirements/BR-SCOPE-002-gateway-signal-filtering.md`
- **Category**: Signal Processing / Resource Management
- **Priority**: P0 (Critical)
- **Key Features**:
  - ✅ Fail-fast signal rejection at Gateway
  - ✅ 2-Level hierarchy validation (Resource → Namespace)
  - ✅ Clear HTTP 200 rejection response with instructions
  - ✅ Prometheus metrics (`gateway_signals_rejected_total`)
  - ✅ Zero API calls (reads from controller-runtime cache)
  - ✅ < 10ms latency target (P95)

#### **BR-SCOPE-010: RO Routing Scope Validation**
- **File**: `docs/requirements/BR-SCOPE-010-ro-routing-validation.md`
- **Category**: Routing / Resource Management
- **Priority**: P0 (Critical)
- **Key Features**:
  - ✅ Routing Check #6 (scope validation)
  - ✅ Exponential backoff retry (5s → 5min → timeout)
  - ✅ Automatic unblocking (no user intervention)
  - ✅ Audit event emission (`orchestrator.routing.blocked`)
  - ✅ Notification integration (Slack, PagerDuty, email)
  - ✅ Max 2 GET calls per retry (resource + namespace)

---

### 2. API Types Updated

#### **RemediationRequest CRD**
- **File**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **Changes**:
  ```go
  // NEW BlockReason constant
  const (
      // ... existing block reasons ...
      BlockReasonUnmanagedResource BlockReason = "UnmanagedResource"
  )

  // Updated PhaseBlocked comment (6 scenarios, was 5)
  PhaseBlocked RemediationPhase = "Blocked"  // 6 scenarios: ..., UnmanagedResource
  ```
- **Reference**: BR-SCOPE-001, FR-SCOPE-003

---

## 🎯 Key Design Decisions

### Decision 1: Label Domain - `kubernaut.ai/managed`

**Approved**: Use `.ai` subdomain to match API group domain

**Rationale**:
```yaml
# CRD API Group
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest

# Label domain should match
metadata:
  labels:
    kubernaut.ai/managed: "true"  # ✅ Consistent
```

---

### Decision 2: Value-Based Labels (Not Existence-Only)

**Approved**: Require explicit `"true"` value, not just label existence

**Rationale**:
- ✅ **Explicit Intent**: `kubernaut.ai/managed="true"` vs `="false"` vs no label
- ✅ **Cluster Tools Compatibility**: Kyverno, OPA, admission webhooks expect values
- ✅ **Audit Trail**: Clear operator decision (true vs false vs unset)
- ✅ **Future Extensibility**: Can add values like `"dry-run"`, `"audit-only"`

---

### Decision 3: 2-Level Hierarchy (Resource → Namespace)

**Approved**: Check resource label first, then namespace label (no owner chain traversal)

**Validation Logic**:
```
1. Is resource cluster-scoped (Node, PV)?
   ├─ YES → Check resource label ONLY
   └─ NO → Check resource label → Check namespace label → Default unmanaged

2. Check resource label:
   ├─ kubernaut.ai/managed="true" → MANAGED (explicit opt-in)
   ├─ kubernaut.ai/managed="false" → UNMANAGED (explicit opt-out)
   └─ No label → Check namespace

3. Check namespace label:
   ├─ kubernaut.ai/managed="true" → MANAGED (inherited)
   ├─ kubernaut.ai/managed="false" → UNMANAGED (inherited)
   └─ No label → UNMANAGED (safe default)
```

**Rationale**:
- ✅ **Simplicity**: Only 2 levels (resource → namespace), not 5 (pod → replicaset → deployment → namespace)
- ✅ **Performance**: Max 2 API calls (resource + namespace)
- ✅ **Operator Control**: Resource-level override for exceptions

---

### Decision 4: Exponential Backoff Retry (Until RR Timeout)

**Approved**: Retry with exponential backoff (5s → 10s → 20s → ... → 5min max) until RR times out (60 min)

**Configuration**:
```yaml
retryConfig:
  unmanagedResource:
    initialInterval: 5s         # First retry after 5 seconds
    maxInterval: 300s           # Cap at 5 minutes per retry
    multiplier: 2.0             # Double the interval each retry
```

**Rationale**:
- ✅ **Early Retries**: Catch quick fixes (5s, 10s, 20s) when operators are actively labeling
- ✅ **Graduated Backoff**: Reduce API load as retries continue (40s, 80s, 160s, 300s)
- ✅ **Max Cap**: 5 minutes per retry balances responsiveness and API efficiency
- ✅ **Global Timeout**: 60 minutes provides eventual failure (prevents infinite retry)
- ✅ **Automatic Unblocking**: No user intervention required (Kubernetes-native reconciliation)

**API Call Cost** (Acceptable):
```
Worst-case (1 blocked RR over 60 minutes):
- Retries: 12 attempts (5s, 10s, 20s, 40s, 80s, 160s, 300s × 7)
- API calls per retry: 2 (resource + namespace)
- Total: 24 GET calls over 60 minutes = 0.4 GET/minute

At scale (100 blocked RRs):
- Total: 2,400 GET calls over 60 minutes = 40 GET/minute = 0.67 GET/second
```

---

### Decision 5: Notification by Default

**Approved**: Notify users by default when remediation is blocked due to unmanaged resource

**Rationale**:
- ✅ **User Visibility**: Users MUST know why Kubernaut isn't remediating
- ✅ **Consistency**: Same pattern as approval requests and self-mitigated remediations
- ✅ **Actionable Feedback**: Notification includes exact label to add
- ✅ **Opt-Out Available**: Users can disable via notification configuration if desired

**Notification Payload**:
```
Title: "Remediation Blocked: Unmanaged Resource"
Body: "Resource production/deployment/payment-api is not managed by Kubernaut."
Action: "Add label 'kubernaut.ai/managed=true' to namespace 'production' or resource."
Priority: Medium
Channel: Configured by operator (Slack, PagerDuty, email)
```

---

### Decision 6: No Gateway Audit Events

**Approved**: Gateway logs + Prometheus metrics only (no audit events for unmanaged signals)

**Rationale**:
- ✅ **Reduce Audit Noise**: Unmanaged signals are expected validation decisions, not business events
- ✅ **Gateway Observability**: Prometheus metrics + structured logs provide sufficient visibility
- ✅ **RO Audit Events**: RO emits `orchestrator.routing.blocked` for blocked RRs (business decision)

---

## 🔄 Example Workflows

### Example 1: Signal from Managed Namespace (Happy Path)

```
1. Namespace "production" has kubernaut.ai/managed=true
2. Alert fires: HighMemoryUsage for Deployment/production/payment-api
3. Gateway validates scope:
   - Checks Deployment: No label
   - Checks Namespace "production": kubernaut.ai/managed=true
   - Scope validation: ✅ PASS (inherited from namespace)
4. Gateway creates RemediationRequest CRD
5. RR processes through SignalProcessing, AIAnalysis
6. RO validates scope (Check #6):
   - Re-validates namespace "production": Still managed
   - Scope validation: ✅ PASS
7. RO creates WorkflowExecution
8. WE executes remediation → ✅ SUCCESS
```

---

### Example 2: Signal from Unmanaged Namespace (Early Rejection)

```
1. Namespace "kube-system" has no kubernaut.ai/managed label
2. Alert fires: HighMemoryUsage for Pod/kube-system/coredns-xyz
3. Gateway validates scope:
   - Checks Pod: No label
   - Checks Namespace "kube-system": No label
   - Scope validation: ❌ FAIL (unmanaged)
4. Gateway rejects signal:
   - HTTP 200 response: "Resource not managed, add label to enable"
   - Log: INFO level
   - Metric: gateway_signals_rejected_total{reason="unmanaged_resource"}++
5. No RemediationRequest created
6. No downstream processing
```

---

### Example 3: Temporal Drift with Automatic Unblocking (Happy Path)

```
1. Alert fires: HighMemoryUsage in namespace "staging" (T0)
2. Gateway validates scope: ✅ PASS (managed at T0)
3. Gateway creates RemediationRequest
4. RR processes through SignalProcessing, AIAnalysis
5. RemediationApprovalRequest created (requires manual approval, T10)
6. Admin removes label (T20):
   kubectl label ns staging kubernaut.ai/managed-
7. Operator approves (T30)
8. RO validates scope (Check #6, T30):
   - Re-validates namespace "staging": No label
   - Scope validation: ❌ FAIL (unmanaged at T30)
9. RO blocks RemediationRequest:
   - Status.OverallPhase = "Blocked"
   - Status.BlockReason = "UnmanagedResource"
   - Audit: orchestrator.routing.blocked
   - Notification: "Remediation blocked: unmanaged resource"
10. RO begins exponential backoff retry:
    - T+30m05s: Retry #1 → Still unmanaged
    - T+30m15s: Retry #2 → Still unmanaged
    - T+35m00s: Admin re-adds label: kubectl label ns staging kubernaut.ai/managed=true
    - T+35m15s: Retry #3 → ✅ NOW MANAGED
11. RO unblocks automatically:
    - Status.OverallPhase = "Processing"
    - Creates WorkflowExecution
12. WE executes remediation → ✅ SUCCESS
```

---

### Example 4: Resource-Level Override (2-Level Hierarchy)

```
1. Namespace "production" has kubernaut.ai/managed=true (all resources managed by default)
2. Specific Deployment "legacy-app" has kubernaut.ai/managed=false (explicit opt-out)
3. Alert fires: HighMemoryUsage for Deployment/production/legacy-app
4. Gateway validates scope:
   - Checks Deployment: kubernaut.ai/managed=false (explicit opt-out)
   - Scope validation: ❌ FAIL (resource override)
5. Gateway rejects signal (respects resource-level label)
6. Other deployments in "production" are still managed ✅
```

---

## 📊 API Call Analysis

### Gateway Scope Validation (Per Signal)

```
Namespaced Resource (e.g., Deployment):
├─ Get resource metadata: 0 API calls (controller-runtime cache)
└─ Get namespace metadata: 0 API calls (controller-runtime cache)
Total: 0 API calls ✅

Cluster-Scoped Resource (e.g., Node):
└─ Get resource metadata: 0 API calls (controller-runtime cache)
Total: 0 API calls ✅
```

**Performance**: Sub-millisecond latency (in-memory map lookup)

---

### RO Scope Validation (Per Retry)

```
Namespaced Resource (e.g., Deployment):
├─ Get resource metadata: 1 GET call (direct API, no cache in V1.0)
└─ Get namespace metadata: 1 GET call (direct API, no cache in V1.0)
Total: 2 GET calls per retry

Cluster-Scoped Resource (e.g., Node):
└─ Get resource metadata: 1 GET call (direct API, no cache in V1.0)
Total: 1 GET call per retry

Retry Frequency (Exponential Backoff):
- 12 retries over 60 minutes (5s, 10s, 20s, 40s, 80s, 160s, 300s × 7)
- Total: 24 GET calls per blocked RR over 60 minutes
- Rate: 0.4 GET/minute per blocked RR

At Scale (100 blocked RRs):
- Total: 2,400 GET calls over 60 minutes
- Rate: 40 GET/minute = 0.67 GET/second
```

**Assessment**: Acceptable API load for defensive validation

---

## 🚀 Implementation Checklist

### Phase 1: Shared Infrastructure

- [ ] Create `pkg/shared/scope/manager.go` (shared by Gateway + RO)
  - [ ] `IsManaged(ctx, namespace, kind, name) bool` method
  - [ ] 2-level hierarchy validation logic
  - [ ] Controller-runtime metadata cache integration (V2.0)

---

### Phase 2: Gateway Integration

- [ ] Update `cmd/gateway/main.go`:
  - [ ] Configure controller-runtime manager
  - [ ] Initialize metadata-only cache for Namespace resources
  - [ ] Initialize `scope.Manager` with cached client
- [ ] Update `pkg/gateway/server.go`:
  - [ ] Add `scopeManager *scope.Manager` field to `Server` struct
  - [ ] Integrate `IsManaged()` check in `ProcessSignal()` (before CRD creation)
  - [ ] Return rejection response for unmanaged signals
  - [ ] Increment `gateway_signals_rejected_total{reason="unmanaged_resource"}`
- [ ] Add Prometheus metric:
  - [ ] `gateway_signals_rejected_total` (counter, labels: `reason`, `namespace`, `signal_type`)
- [ ] Unit tests:
  - [ ] `test/unit/gateway/scope_validation_test.go` (10+ test cases)
- [ ] Integration tests:
  - [ ] `test/integration/gateway/scope_filtering_test.go` (managed, unmanaged, hierarchy)

---

### Phase 3: RO Integration

- [ ] Create `pkg/remediationorchestrator/routing/scope_validator.go`:
  - [ ] `CheckManagedResource(ctx, rr) bool` method
  - [ ] Reuse `scope.Manager` from shared package
- [ ] Update `pkg/remediationorchestrator/routing/blocking.go`:
  - [ ] Add Check #6 (scope validation) to `CheckBlockingConditions()`
  - [ ] Block RR if unmanaged: `Status.BlockReason = UnmanagedResource`
  - [ ] Emit audit event: `orchestrator.routing.blocked`
- [ ] Update `internal/controller/remediationorchestrator/reconciler.go`:
  - [ ] Add exponential backoff retry logic for `BlockReasonUnmanagedResource`
  - [ ] Configuration: `initialInterval=5s`, `maxInterval=300s`, `multiplier=2.0`
  - [ ] Log retry attempts: "Retrying scope validation (attempt N/12)"
- [ ] Add Prometheus metric:
  - [ ] `remediation_requests_blocked_total` (counter, labels: `reason="unmanaged_resource"`)
- [ ] Unit tests:
  - [ ] `test/unit/remediationorchestrator/scope_validation_test.go` (15+ test cases)
- [ ] Integration tests:
  - [ ] `test/integration/remediationorchestrator/scope_blocking_test.go` (temporal drift, retry, unblock)

---

### Phase 4: Documentation Updates

- [ ] Update DD-RO-002-ADDENDUM:
  - [ ] Add UnmanagedResource as 6th blocking scenario
  - [ ] Document exponential backoff retry behavior
- [ ] Create user guide:
  - [ ] `docs/user-guide/scope-management.md`
  - [ ] Labeling instructions (namespace + resource)
  - [ ] Troubleshooting guide (why is my signal rejected?)
- [ ] Update service documentation:
  - [ ] Gateway: Signal rejection behavior
  - [ ] RO: Routing Check #6 and retry logic
- [ ] Update API reference:
  - [ ] RemediationRequest.Status.BlockReason (UnmanagedResource)

---

## 📋 Success Criteria

| Metric | Target | Validation Method |
|--------|--------|-------------------|
| **CRD Reduction** | > 50% fewer RRs for unmanaged signals | Compare RR count before/after |
| **Gateway Latency** | < 10ms added latency (P95) | Prometheus histogram |
| **RO Latency** | < 10ms added latency (P95) | Prometheus histogram |
| **False Rejections** | < 0.1% managed signals rejected | `signals_rejected_total` / `signals_received_total` |
| **Auto-Unblock Rate** | > 80% blocked RRs unblock before timeout | `blocked_duration` histogram |
| **Notification Delivery** | 100% blocked RRs trigger notification | Notification service logs |
| **Temporal Drift Detection** | 100% unmanaged resources blocked at RO | Audit events |

---

## 🔗 Related Documentation

| Document | Purpose |
|----------|---------|
| `docs/requirements/BR-SCOPE-001-resource-scope-management.md` | Parent BR (scope management) |
| `docs/requirements/BR-SCOPE-002-gateway-signal-filtering.md` | Gateway signal filtering BR |
| `docs/requirements/BR-SCOPE-010-ro-routing-validation.md` | RO routing validation BR |
| `docs/architecture/decisions/ADR-053-resource-scope-management.md` | **Architecture Decision Record (alternatives, tradeoffs, rationale)** |
| `api/remediation/v1alpha1/remediationrequest_types.go` | CRD types (BlockReasonUnmanagedResource) |
| `docs/architecture/decisions/DD-RO-002-ADDENDUM.md` | Blocked phase semantics (6 scenarios) |

---

## ✅ Approval Summary

**Approved By**: Platform Team, Gateway Team, RemediationOrchestrator Team
**Date**: January 20, 2026
**Confidence**: 95%

**Key Decisions Approved**:
1. ✅ Label domain: `kubernaut.ai/managed` (matches API group)
2. ✅ Value-based labels: `"true"` (not existence-only)
3. ✅ 2-level hierarchy: Resource → Namespace (no owner chain)
4. ✅ Exponential backoff: 5s → 5min max, until RR timeout (60min)
5. ✅ Notification by default: Users MUST know why remediation isn't happening
6. ✅ No Gateway audit events: Logs + metrics only

**Next Step**: Begin implementation with Phase 1 (Shared Infrastructure)

---

**Document Version**: 1.0
**Last Updated**: January 20, 2026
**Approver**: Platform Team
