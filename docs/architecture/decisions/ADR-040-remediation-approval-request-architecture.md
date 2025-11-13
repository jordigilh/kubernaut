# ADR-040: RemediationApprovalRequest CRD Architecture

## Status
**✅ Approved** (2025-11-13)
**Last Reviewed**: 2025-11-13
**Confidence**: 99%

## Context & Problem

When AIAnalysis determines that a remediation recommendation has medium confidence (60-79%) or when Rego policies mandate manual approval, the system must request operator approval before proceeding with remediation execution. This approval workflow requires:

1. **Dedicated CRD**: A Kubernetes-native resource to represent the approval request
2. **Lifecycle Management**: Clear state transitions from requested → approved/rejected/expired
3. **Efficient Linkage**: AIAnalysis controller must efficiently discover and watch approval decisions
4. **Audit Trail**: Complete event tracking for compliance and debugging
5. **Timeout Handling**: Automatic expiration if no decision is made within the deadline

**Key Requirements**:
- Follow Kubernetes `CertificateSigningRequest` pattern (immutable spec, mutable status)
- Maintain flat CRD hierarchy (all service CRDs owned by RemediationRequest)
- Enable efficient AIAnalysis → RemediationApprovalRequest lookup without labels
- Support configurable approval timeouts (per-request, policy, namespace, default)
- Provide complete audit trail for approval lifecycle events

**Related Decisions**:
- **Supersedes**: `AIApprovalRequest` (renamed to `RemediationApprovalRequest`)
- **Builds On**: [ADR-001](ADR-001-crd-microservices-architecture.md) - CRD-based architecture
- **Builds On**: [ADR-034](ADR-034-unified-audit-table-design.md) - Unified audit events
- **Supports**: BR-AI-035, BR-AI-042, BR-AI-043, BR-AI-044, BR-ORCH-020

## Alternatives Considered

### Alternative 1: AIAnalysis Owned RemediationApprovalRequest (Nested Hierarchy)

**Approach**: `RemediationApprovalRequest` owned by `AIAnalysis` (3-level hierarchy)

```yaml
RemediationRequest (Level 1)
  └── AIAnalysis (Level 2)
        └── RemediationApprovalRequest (Level 3)  # Nested ownership
```

**Pros**:
- ✅ Logical grouping: Approval is conceptually part of AI analysis
- ✅ Automatic cleanup when AIAnalysis is deleted

**Cons**:
- ❌ Violates flat hierarchy pattern established in ADR-001
- ❌ Creates 3-level ownership chain (RemediationRequest → AIAnalysis → RemediationApprovalRequest)
- ❌ Complicates cascade deletion (sequential vs parallel)
- ❌ Inconsistent with other service CRDs (SignalProcessing, WorkflowExecution, RemediationExecution)
- ❌ Harder to query all approval requests for a RemediationRequest

**Confidence**: 35% (rejected - violates architectural principles)

---

### Alternative 2: Label-Based Linkage

**Approach**: Use Kubernetes labels for AIAnalysis → RemediationApprovalRequest lookup

```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
metadata:
  labels:
    kubernaut.io/ai-analysis: ai-analysis-123  # Label for reverse lookup
  ownerReferences:
    - kind: RemediationRequest
      name: remediation-request-123
spec:
  aiAnalysisRef:
    name: ai-analysis-123
```

**AIAnalysis Controller Lookup**:
```go
approvalReqList := &remediationv1alpha1.RemediationApprovalRequestList{}
err := r.List(ctx, approvalReqList,
    client.InNamespace(aiAnalysis.Namespace),
    client.MatchingLabels{"kubernaut.io/ai-analysis": aiAnalysis.Name},
)
```

**Pros**:
- ✅ Standard Kubernetes pattern (label selectors)
- ✅ Efficient list operation (scoped by namespace + label)
- ✅ No additional controller-runtime setup required

**Cons**:
- ❌ Label management overhead (must sync labels with spec)
- ❌ Clutters CRD metadata with operational labels
- ❌ Less efficient than field indexing (label cache vs field index cache)
- ❌ Requires list operation in reconcile loop (even if scoped)

**Confidence**: 93% (acceptable but not optimal)

---

### Alternative 3: Field Index on spec.aiAnalysisRef.name (APPROVED)

**Approach**: Use controller-runtime field indexing for efficient spec-based lookup

```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
metadata:
  ownerReferences:
    - kind: RemediationRequest
      name: remediation-request-123
      controller: true
spec:
  remediationRequestRef:
    name: remediation-request-123
  aiAnalysisRef:
    name: ai-analysis-123  # ⭐ Indexed field for efficient lookup
  approvalContext:
    confidence: 0.65
    recommendedPlaybook: "pod-oom-recovery"
    reason: "Confidence below 70% threshold"
  requiredBy: "2025-11-14T10:00:00Z"
status:
  decision: ""  # "Approved", "Rejected", "Expired"
  conditions: [...]
```

**Field Index Registration** (in AIAnalysis controller):
```go
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Register field index for efficient RemediationApprovalRequest lookup
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1alpha1.RemediationApprovalRequest{},
        "spec.aiAnalysisRef.name",
        func(rawObj client.Object) []string {
            approvalReq := rawObj.(*remediationv1alpha1.RemediationApprovalRequest)
            if approvalReq.Spec.AIAnalysisRef.Name == "" {
                return nil
            }
            return []string{approvalReq.Spec.AIAnalysisRef.Name}
        },
    ); err != nil {
        return fmt.Errorf("failed to create field index: %w", err)
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1alpha1.AIAnalysis{}).
        Watches(
            &remediationv1alpha1.RemediationApprovalRequest{},
            handler.EnqueueRequestsFromMapFunc(r.findAIAnalysisForApprovalRequest),
        ).
        Complete(r)
}
```

**AIAnalysis Controller Lookup**:
```go
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
    if err := r.Get(ctx, req.NamespacedName, aiAnalysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    if aiAnalysis.Status.Phase == "Approving" {
        approvalReqList := &remediationv1alpha1.RemediationApprovalRequestList{}

        // ⭐ Use field selector (indexed) for efficient lookup
        if err := r.List(ctx, approvalReqList,
            client.InNamespace(aiAnalysis.Namespace),
            client.MatchingFields{"spec.aiAnalysisRef.name": aiAnalysis.Name},
        ); err != nil {
            return ctrl.Result{}, err
        }

        if len(approvalReqList.Items) == 0 {
            return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
        }

        approvalReq := &approvalReqList.Items[0]

        switch approvalReq.Status.Decision {
        case "Approved":
            aiAnalysis.Status.Phase = "Ready"
            return r.Status().Update(ctx, aiAnalysis)
        case "Rejected", "Expired":
            aiAnalysis.Status.Phase = "Rejected"
            return r.Status().Update(ctx, aiAnalysis)
        }
    }

    return ctrl.Result{}, nil
}
```

**Watch Mapping** (RemediationApprovalRequest → AIAnalysis):
```go
func (r *AIAnalysisReconciler) findAIAnalysisForApprovalRequest(
    ctx context.Context,
    obj client.Object,
) []reconcile.Request {
    approvalReq := obj.(*remediationv1alpha1.RemediationApprovalRequest)

    // Use spec.aiAnalysisRef for efficient lookup (no list operation)
    if approvalReq.Spec.AIAnalysisRef.Name == "" {
        return []reconcile.Request{}
    }

    return []reconcile.Request{{NamespacedName: types.NamespacedName{
        Name:      approvalReq.Spec.AIAnalysisRef.Name,
        Namespace: approvalReq.Namespace,
    }}}
}
```

**Pros**:
- ✅ Standard controller-runtime pattern (field indexing)
- ✅ Most efficient lookup (native cache support, O(1) lookup)
- ✅ Cleaner CRD metadata (no label clutter)
- ✅ Immutable spec-based lookup (no status update needed)
- ✅ Single setup in `SetupWithManager` (one-time registration)
- ✅ Maintains flat hierarchy (RemediationRequest owns all service CRDs)
- ✅ Efficient watch mapping (direct name lookup, no list operation)

**Cons**:
- ⚠️ Requires list operation in reconcile loop (but scoped by namespace + indexed field)
- ⚠️ Slight increase in setup complexity (field index registration)

**Confidence**: 99% (approved - optimal solution)

---

## Decision

**APPROVED: Alternative 3** - Field Index on `spec.aiAnalysisRef.name`

**Rationale**:
1. **Performance**: Field indexing provides O(1) lookup in controller-runtime cache, more efficient than label selectors
2. **Clean Metadata**: No operational labels needed in CRD metadata, keeping it focused on business data
3. **Kubernetes Alignment**: Field indexing is a standard controller-runtime pattern used throughout the ecosystem
4. **Immutable Spec**: Index is based on immutable spec field, not mutable status (no sync issues)
5. **Flat Hierarchy**: Maintains ADR-001 flat hierarchy with RemediationRequest as the single owner
6. **Efficient Watching**: Watch mapping uses direct name lookup without list operations

**Key Insight**: Field indexing combines the efficiency of direct lookups with the flexibility of spec-based filtering, providing the best of both worlds without label management overhead.

## Implementation

### Primary Implementation Files

1. **CRD Definition**: `api/remediation/v1alpha1/remediationapprovalrequest_types.go`
   - Defines `RemediationApprovalRequest` CRD following `CertificateSigningRequest` pattern
   - Immutable `spec` for request details
   - Mutable `status.conditions` for decision tracking

2. **Dedicated Controller**: `internal/controller/remediationapproval/remediationapprovalrequest_controller.go`
   - Manages `RemediationApprovalRequest` lifecycle
   - Detects timeout expiration
   - Emits audit events for all state transitions

3. **AIAnalysis Controller Updates**: `internal/controller/aianalysis/aianalysis_controller.go`
   - Registers field index on `spec.aiAnalysisRef.name` in `SetupWithManager`
   - Watches `RemediationApprovalRequest` changes
   - Uses `client.MatchingFields` for efficient lookup

4. **RemediationOrchestrator Updates**: `internal/controller/remediationorchestrator/remediationrequest_controller.go`
   - Creates `RemediationApprovalRequest` when `AIAnalysis.status.approvalRequired=true`
   - Sets `spec.aiAnalysisRef.name` and `spec.requiredBy` timestamp
   - Updates `AIAnalysis.status.remediationApprovalRequestRef` for reference

### Data Flow

**Approval Request Flow**:
```
1. AIAnalysis determines approval needed (confidence 60-79% or Rego policy)
   ↓
2. AIAnalysis.status.approvalRequired = true
   AIAnalysis.status.approvalContext = {...}
   AIAnalysis.status.phase = "Approving"
   ↓
3. RemediationOrchestrator watches AIAnalysis.status.approvalRequired
   ↓
4. RemediationOrchestrator creates RemediationApprovalRequest:
   - spec.aiAnalysisRef.name = "ai-analysis-123"
   - spec.requiredBy = calculateApprovalDeadline()
   - ownerReferences = [RemediationRequest]
   ↓
5. RemediationOrchestrator updates AIAnalysis.status.remediationApprovalRequestRef
   ↓
6. RemediationApprovalRequest Controller watches for timeout expiration
   ↓
7. Operator reviews and updates RemediationApprovalRequest.status.conditions
   ↓
8. RemediationApprovalRequest Controller updates status.decision based on conditions
   ↓
9. AIAnalysis Controller watches RemediationApprovalRequest (via field index)
   ↓
10. AIAnalysis Controller updates phase based on decision:
    - "Approved" → phase = "Ready"
    - "Rejected" → phase = "Rejected"
    - "Expired" → phase = "Rejected"
```

**Timeout Configuration Hierarchy**:
```go
func (r *RemediationRequestReconciler) calculateApprovalDeadline(
    ctx context.Context,
    aiAnalysis *aianalysisv1alpha1.AIAnalysis,
) time.Time {
    // 1. Check AIAnalysis.spec.approvalTimeout (highest priority)
    if aiAnalysis.Spec.ApprovalTimeout != nil {
        return time.Now().Add(aiAnalysis.Spec.ApprovalTimeout.Duration)
    }

    // 2. Check Rego policy timeout (from Signal Processing categorization)
    if policyTimeout := r.getRegoApprovalTimeout(ctx, aiAnalysis); policyTimeout > 0 {
        return time.Now().Add(policyTimeout)
    }

    // 3. Check ConfigMap (namespace-level default)
    if configTimeout := r.getConfigMapTimeout(ctx, aiAnalysis.Namespace); configTimeout > 0 {
        return time.Now().Add(configTimeout)
    }

    // 4. Hardcoded default (15 minutes)
    return time.Now().Add(15 * time.Minute)
}
```

**Cascade Deletion Flow**:
```
RemediationRequest deleted
    ↓
Kubernetes garbage collector deletes ALL owned CRDs in parallel:
  - SignalProcessing ✓
  - AIAnalysis ✓
  - RemediationApprovalRequest ✓  ⭐ Deleted directly by RemediationRequest
  - WorkflowExecution ✓
  - RemediationExecution ✓
```

### Graceful Degradation

**Timeout Expiration**:
- RemediationApprovalRequest Controller detects `time.Now() > spec.requiredBy`
- Updates `status.decision = "Expired"`
- Emits audit event: `remediationapprovalrequest.expired`
- AIAnalysis Controller transitions to `phase = "Rejected"`

**AIAnalysis Deletion During Approval**:
- RemediationRequest cascade deletion removes RemediationApprovalRequest
- No orphaned approval requests (owner references ensure cleanup)
- Audit events capture deletion for compliance

**RemediationOrchestrator Failure**:
- AIAnalysis remains in `phase = "Approving"`
- RemediationApprovalRequest may not be created
- Timeout detection in AIAnalysis controller (15min default)
- Transitions to `phase = "Rejected"` with reason "ApprovalTimeout"

## Consequences

### Positive

- ✅ **Efficient Lookups**: Field indexing provides O(1) lookup performance in controller-runtime cache
- ✅ **Clean Metadata**: No operational labels cluttering CRD metadata
- ✅ **Flat Hierarchy**: Maintains ADR-001 flat hierarchy pattern with RemediationRequest as single owner
- ✅ **Kubernetes Alignment**: Follows `CertificateSigningRequest` pattern (immutable spec, mutable status)
- ✅ **Complete Audit Trail**: All lifecycle events tracked via ADR-034 unified audit table
- ✅ **Configurable Timeouts**: Supports per-request, policy, namespace, and default timeout configuration
- ✅ **Automatic Cleanup**: Owner references ensure cascade deletion without manual cleanup
- ✅ **Dedicated Controller**: Single Responsibility Principle - one controller per CRD type

### Negative

- ⚠️ **List Operation in Reconcile**: AIAnalysis controller requires list operation (scoped by namespace + indexed field)
  - **Mitigation**: List is highly efficient (O(1) field index lookup + namespace scope)
- ⚠️ **Setup Complexity**: Requires field index registration in `SetupWithManager`
  - **Mitigation**: One-time setup, standard controller-runtime pattern
- ⚠️ **Cross-Controller Coordination**: RemediationOrchestrator creates CRD, AIAnalysis watches it
  - **Mitigation**: Standard Kubernetes pattern, well-documented in ADR-001

### Neutral

- 🔄 **CRD Rename**: `AIApprovalRequest` → `RemediationApprovalRequest`
  - Improves scope clarity (not just AI approvals, any remediation approval)
  - Aligns with Kubernetes naming conventions (e.g., `CertificateSigningRequest`)
- 🔄 **New Controller**: Dedicated `RemediationApprovalRequest` controller
  - Follows Kubernetes best practice (one controller per CRD)
  - Increases controller count (6 → 7 controllers)

## Validation Results

### Confidence Assessment Progression

- Initial assessment: 97% confidence (field index approach)
- After user clarification (RemediationRequest as owner): 98% confidence
- After timeout configuration hierarchy: 99% confidence
- After finalizer cleanup updates: 99% confidence

### Key Validation Points

- ✅ **Owner Reference Pattern**: Verified RemediationRequest owns all service CRDs (flat hierarchy)
- ✅ **Field Index Efficiency**: Confirmed O(1) lookup in controller-runtime cache
- ✅ **Cascade Deletion**: Validated parallel deletion of all child CRDs via owner references
- ✅ **Timeout Configuration**: Defined 4-level hierarchy (per-request → policy → namespace → default)
- ✅ **Audit Integration**: Confirmed event taxonomy aligns with ADR-034 unified audit table

## Related Decisions

- **Index**: [DESIGN_DECISIONS.md](../DESIGN_DECISIONS.md#adr-040) - Architectural decision index
- **Supersedes**: `AIApprovalRequest` CRD (renamed to `RemediationApprovalRequest`)
- **Builds On**: [ADR-001](ADR-001-crd-microservices-architecture.md) - CRD-based microservices architecture
- **Builds On**: [ADR-034](ADR-034-unified-audit-table-design.md) - Unified audit table design
- **Supports**: BR-AI-035 (Approval Request Creation), BR-AI-042 (Manual Review), BR-AI-043 (Approval Timeout), BR-AI-044 (Approval/Rejection Handling), BR-ORCH-020 (Approval Orchestration)

## Service Documentation

- **AIAnalysis Service**: [docs/services/crd-controllers/02-aianalysis/](../../services/crd-controllers/02-aianalysis/) - AI analysis controller that watches RemediationApprovalRequest decisions
- **RemediationOrchestrator Service**: [docs/services/crd-controllers/05-remediationorchestrator/](../../services/crd-controllers/05-remediationorchestrator/) - Orchestrator that creates RemediationApprovalRequest CRDs
- **RemediationApprovalRequest Controller**: [docs/services/crd-controllers/06-remediationapproval/](../../services/crd-controllers/06-remediationapproval/) - Dedicated controller for approval lifecycle management (to be created)

## Review & Evolution

### When to Revisit

- If approval timeout configuration becomes too complex (consider simplification)
- If field index performance degrades (consider alternative lookup strategies)
- If approval workflow requires multi-stage approvals (e.g., L1 → L2 escalation)
- If approval requests need to be queried across namespaces (consider cluster-scoped CRD)

### Success Metrics

- **Approval Latency**: P95 < 100ms for AIAnalysis to detect approval decision
- **Timeout Accuracy**: 99%+ of timeouts detected within 30 seconds of expiration
- **Cascade Deletion**: 100% of RemediationApprovalRequest CRDs deleted with parent RemediationRequest
- **Audit Completeness**: 100% of approval lifecycle events captured in audit table

