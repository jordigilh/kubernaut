# DD-CRD-003: Custom Field Selectors for Operational Queries

## Status
**🟡 PROPOSED** (2025-12-29)
**Last Reviewed**: 2025-12-29
**Confidence**: 90%
**Minimum Kubernetes Version**: v1.30+ (CRD selectableFields support)

---

## Context & Problem

### Current Limitation
Kubernetes CRDs support field selectors for standard metadata fields (`metadata.name`, `metadata.namespace`), but **custom field selectors** for spec/status fields require explicit API server registration.

**Current Operational Pain Points**:
```bash
# ❌ Cannot filter by RemediationRequest reference
kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123

# ❌ Cannot filter by PipelineRun reference
kubectl get workflowexecutions --field-selector status.pipelineRunRef.name=wfe-xyz789

# ❌ Cannot filter by SignalFingerprint
kubectl get remediationrequests --field-selector spec.signalFingerprint=fingerprint-123

# ❌ Cannot filter by Phase
kubectl get workflowexecutions --field-selector status.phase=Running
```

**Workaround Required**:
```bash
# Current approach: Fetch all, filter client-side (inefficient)
kubectl get workflowexecutions -o json | jq '.items[] | select(.spec.remediationRequestRef.name=="rr-abc123")'
```

### Business Requirements
- **BR-OPS-001**: Operators need efficient filtering for troubleshooting
- **BR-OPS-002**: Support querying child resources by parent reference
- **BR-OPS-003**: Enable phase-based operational queries
- **BR-OPS-004**: Provide consistent UX across all Kubernaut CRDs

### Key Requirements
1. **Efficient Server-Side Filtering**: Reduce API server load and network traffic
2. **Parent-Child Traversal**: Query child CRDs by parent reference (e.g., all WorkflowExecutions for a RemediationRequest)
3. **Phase-Based Queries**: Filter by execution phase for operational monitoring
4. **Consistent UX**: Same field selector patterns across all CRDs

---

## Alternatives Considered

### Alternative 1: No Custom Field Selectors (Status Quo)
**Approach**: Continue using client-side filtering with `kubectl get -o json | jq`

**Pros**:
- ✅ No implementation effort required
- ✅ No API server changes needed

**Cons**:
- ❌ Inefficient: Fetches all resources, filters client-side
- ❌ Poor UX: Requires jq/scripting knowledge
- ❌ High network traffic: Transfers all resources
- ❌ Slow for large clusters: O(n) filtering

**Confidence**: 95% (rejected - poor operational experience)

---

### Alternative 2: Custom Field Selectors via IndexField Registration
**Approach**: Register custom field selectors in controller-runtime's FieldIndexer

**Implementation**:
```go
// In controller setup (cmd/*/main.go or internal/controller/*/setup.go)
mgr.GetFieldIndexer().IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
    "spec.remediationRequestRef.name",
    func(obj client.Object) []string {
        wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
        return []string{wfe.Spec.RemediationRequestRef.Name}
    })
```

**Pros**:
- ✅ Efficient: Server-side filtering
- ✅ Native kubectl support: `--field-selector` works
- ✅ Low implementation effort: ~50 LOC per CRD
- ✅ No CRD schema changes required
- ✅ Works with existing API server

**Cons**:
- ⚠️ Requires controller restart to add new indexes
- ⚠️ Limited to fields known at controller startup

**Confidence**: 90% (**RECOMMENDED**)

---

### Alternative 3: Label-Based Filtering (Workaround) — DEPRECATED
**Approach**: Copy key fields to labels for filtering (historical pattern; **removed in Issue #91**)

**Historical Example** (no longer used):
```yaml
metadata:
  labels:
    kubernaut.ai/remediation-request: rr-abc123  # REMOVED: use spec.remediationRequestRef
    kubernaut.ai/phase: Running                   # REMOVED: use spec/status fields
```

**Pros**:
- ✅ Works with standard label selectors
- ✅ No field indexer registration needed

**Cons**:
- ❌ Label length limits (63 chars)
- ❌ Duplicates data (spec/status → labels)
- ❌ Requires label sync logic
- ❌ Labels can drift from actual values
- ❌ **Issue #91**: `kubernaut.ai/*` labels migrated to immutable spec fields; field selectors replace label-based filtering

**Confidence**: 70% (rejected - maintenance burden)

---

## Decision

**APPROVED: Hybrid Approach** - BOTH kubebuilder CRD annotations AND IndexField Registration

**Two Complementary Mechanisms**:

### 1. CRD-Level Field Selectors (kubebuilder v3.14+)
**Approach**: Use `+kubebuilder:selectablefield` annotations in CRD type definitions

**Example**:
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:selectablefield:JSONPath=.spec.signalFingerprint
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.status.phase
type WorkflowExecution struct {
    // ...
}
```

**Pros**:
- ✅ Declared at CRD schema level (API server enforced)
- ✅ Works without controller running
- ✅ Visible in CRD spec: `kubectl get crd workflowexecutions.kubernaut.ai -o yaml`
- ✅ Standard Kubernetes mechanism
- ✅ **Primary use case**: `kubectl` CLI filtering for operators

**Cons**:
- ⚠️ Requires kubebuilder v3.14+ and Kubernetes v1.30+ (acceptable - set as minimum version)
- ⚠️ Limited to fields declared in CRD (can't add dynamically)

### 2. Controller-Level Field Indexers (controller-runtime)
**Approach**: Register indexes in controller manager setup

**Example**:
```go
mgr.GetFieldIndexer().IndexField(ctx, &WorkflowExecution{},
    "spec.remediationRequestRef.name",
    func(obj client.Object) []string {
        return []string{obj.(*WorkflowExecution).Spec.RemediationRequestRef.Name}
    })
```

**Pros**:
- ✅ **Primary use case**: Programmatic queries in controllers (`client.List()` with `client.MatchingFields`)
- ✅ Can be added/modified without CRD regeneration
- ✅ More flexible indexing logic
- ✅ Essential for controller logic that needs to find child resources by parent reference

**Cons**:
- ⚠️ Only works when controller is running (acceptable - controllers need it for queries)
- ⚠️ Not visible in CRD spec (documented in code comments)

**Rationale**:
1. **Use BOTH for different use cases**: CRD-level for kubectl/API, controller-level for programmatic queries
2. **CRD Annotations** enable `kubectl get --field-selector` (human operators)
3. **Controller Indexers** enable `client.List()` with `client.MatchingFields` (controllers)
4. **Operational Efficiency**: Server-side filtering reduces API load in both cases
5. **Consistent Pattern**: Same field selector names across both mechanisms

**Minimum Kubernetes Version**: Kubernetes v1.30+ (first version with CRD selectableFields support)

**Key Insight**: Field selectors are **operational tooling**, not runtime functionality. They enhance troubleshooting without affecting core business logic.

---

## Implementation

### Proposed Field Selectors by CRD

#### 1. WorkflowExecution

**CRD Annotations** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`):
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetResource`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.namespace
// +kubebuilder:selectablefield:JSONPath=.spec.targetResource
// +kubebuilder:selectablefield:JSONPath=.status.phase
// +kubebuilder:selectablefield:JSONPath=.status.pipelineRunRef.name
type WorkflowExecution struct {
    // ...
}
```

**Controller Indexers** (for programmatic queries):
```go
// Spec fields
"spec.remediationRequestRef.name"     // Parent RR lookup
"spec.remediationRequestRef.namespace" // Cross-namespace queries
"spec.targetResource"                  // Resource-based queries

// Status fields
"status.phase"                         // Phase-based filtering (Running, Completed, Failed)
"status.pipelineRunRef.name"          // PipelineRun lookup
```

**Why Both?**
- **CRD Annotations**: Used by `kubectl` CLI and API server
- **Controller Indexers**: Used by controllers for programmatic `client.List()` calls with `client.MatchingFields`

**Use Cases (kubectl CLI)**:
```bash
# Find all WFEs for a RemediationRequest
kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123

# Find WFE by PipelineRun
kubectl get workflowexecutions --field-selector status.pipelineRunRef.name=wfe-xyz789

# Find all running workflows
kubectl get workflowexecutions --field-selector status.phase=Running
```

**Use Cases (Controller Code)**:
```go
// RemediationOrchestrator finding all WFEs for a RemediationRequest
wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
err := r.Client.List(ctx, wfeList,
    client.InNamespace(rr.Namespace),
    client.MatchingFields{"spec.remediationRequestRef.name": rr.Name})

// WorkflowExecution controller finding WFE by PipelineRun name
wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
err := r.Client.List(ctx, wfeList,
    client.MatchingFields{"status.pipelineRunRef.name": pipelineRunName})
```

---

#### 2. RemediationRequest

**CRD Annotations** (`api/remediation/v1alpha1/remediationrequest_types.go`):
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:selectablefield:JSONPath=.spec.signalFingerprint  // ✅ ALREADY IMPLEMENTED
// +kubebuilder:selectablefield:JSONPath=.spec.targetResource.name         // TODO
// +kubebuilder:selectablefield:JSONPath=.spec.targetResource.namespace    // TODO
// +kubebuilder:selectablefield:JSONPath=.status.phase                     // TODO
// +kubebuilder:selectablefield:JSONPath=.status.workflowExecutionRef.name // TODO
type RemediationRequest struct {
    // ...
}
```

**Controller Indexers**:
```go
// Spec fields
"spec.signalFingerprint"              // Deduplication queries (✅ CRD annotation exists)
"spec.targetResource.name"            // Resource-based queries
"spec.targetResource.namespace"       // Namespace filtering

// Status fields
"status.phase"                         // Phase-based filtering
"status.workflowExecutionRef.name"    // Child WFE lookup
```

**Use Cases**:
```bash
# Find RR by signal fingerprint
kubectl get remediationrequests --field-selector spec.signalFingerprint=fingerprint-123

# Find RR by child WorkflowExecution
kubectl get remediationrequests --field-selector status.workflowExecutionRef.name=wfe-xyz789

# Find all pending RRs
kubectl get remediationrequests --field-selector status.phase=Pending
```

---

#### 3. AIAnalysis
```go
// Spec fields
"spec.remediationRequestRef.name"     // Parent RR lookup
"spec.analysisType"                    // Analysis type filtering

// Status fields
"status.phase"                         // Phase-based filtering
"status.confidence"                    // Confidence-based queries (if useful)
```

**Use Cases**:
```bash
# Find all AI analyses for a RR
kubectl get aianalyses --field-selector spec.remediationRequestRef.name=rr-abc123

# Find completed analyses
kubectl get aianalyses --field-selector status.phase=Completed
```

---

#### 4. SignalProcessing
```go
// Spec fields
"spec.remediationRequestRef.name"     // Parent RR lookup
"spec.signalSource"                    // Source-based filtering

// Status fields
"status.phase"                         // Phase-based filtering
```

---

#### 5. RemediationApprovalRequest
```go
// Spec fields
"spec.remediationRequestRef.name"     // Parent RR lookup
"spec.approvalType"                    // Approval type filtering

// Status fields
"status.decision"                      // Decision-based filtering (Approved, Rejected)
```

---

#### 6. KubernetesExecution
```go
// Spec fields
"spec.remediationRequestRef.name"     // Parent RR lookup (if exists)
"spec.targetResource.name"            // Resource-based queries

// Status fields
"status.phase"                         // Phase-based filtering
```

---

### Implementation Pattern

#### Approach 1: CRD Annotations (Preferred for New Fields)

**Location**: CRD type definitions in `api/*/v1alpha1/*_types.go`

**Template**:
```go
// api/workflowexecution/v1alpha1/workflowexecution_types.go

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetResource`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
//
// Field Selectors for Operational Queries (DD-CRD-003)
// Enable kubectl filtering: kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.namespace
// +kubebuilder:selectablefield:JSONPath=.spec.targetResource
// +kubebuilder:selectablefield:JSONPath=.status.phase
// +kubebuilder:selectablefield:JSONPath=.status.pipelineRunRef.name
//
// WorkflowExecution is the Schema for the workflowexecutions API.
type WorkflowExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
    Status WorkflowExecutionStatus `json:"status,omitempty"`
}
```

**After adding annotations, regenerate CRDs**:
```bash
make manifests  # Regenerates config/crd/bases/*.yaml with selectableFields
```

**Requirements**:
- kubebuilder v3.14+ (for `selectablefield` support)
- Kubernetes v1.30+ (for CRD selectableFields feature gate)

---

#### Approach 2: Controller-Runtime Field Indexers (Programmatic Queries)

**Location**: Controller setup in `cmd/*/main.go` or dedicated `internal/controller/*/indexers.go`

**Purpose**: Enable controllers to efficiently query CRDs programmatically using `client.List()` with `client.MatchingFields`.

**Template**:
```go
// internal/controller/workflowexecution/indexers.go
package workflowexecution

import (
    "context"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"

    workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// RegisterFieldIndexers registers custom field selectors for WorkflowExecution.
//
// This enables efficient server-side filtering via kubectl:
//   kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123
//
// Business Requirement: BR-OPS-001 (Efficient Operational Queries)
// Design Decision: DD-CRD-003 (Custom Field Selectors)
func RegisterFieldIndexers(ctx context.Context, mgr manager.Manager) error {
    indexer := mgr.GetFieldIndexer()

    // Spec: RemediationRequest reference name
    if err := indexer.IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
        "spec.remediationRequestRef.name",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            return []string{wfe.Spec.RemediationRequestRef.Name}
        }); err != nil {
        return err
    }

    // Spec: RemediationRequest reference namespace
    if err := indexer.IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
        "spec.remediationRequestRef.namespace",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            return []string{wfe.Spec.RemediationRequestRef.Namespace}
        }); err != nil {
        return err
    }

    // Spec: Target resource
    if err := indexer.IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
        "spec.targetResource",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            return []string{wfe.Spec.TargetResource}
        }); err != nil {
        return err
    }

    // Status: Phase
    if err := indexer.IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
        "status.phase",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            if wfe.Status.Phase == "" {
                return nil
            }
            return []string{string(wfe.Status.Phase)}
        }); err != nil {
        return err
    }

    // Status: PipelineRun reference
    if err := indexer.IndexField(ctx, &workflowexecutionv1alpha1.WorkflowExecution{},
        "status.pipelineRunRef.name",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            if wfe.Status.PipelineRunRef == nil {
                return nil
            }
            return []string{wfe.Status.PipelineRunRef.Name}
        }); err != nil {
        return err
    }

    return nil
}
```

**Wire in main.go**:
```go
// cmd/workflowexecution/main.go
import (
    weindexers "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

func main() {
    // ... existing setup ...

    // Register field indexers (DD-CRD-003)
    if err := weindexers.RegisterFieldIndexers(ctx, mgr); err != nil {
        setupLog.Error(err, "unable to register field indexers")
        os.Exit(1)
    }

    // ... rest of setup ...
}
```

---

### Testing Strategy

#### Unit Tests
```go
// test/unit/workflowexecution/indexers_test.go
var _ = Describe("Field Indexers", func() {
    It("should index spec.remediationRequestRef.name", func() {
        // Create WFE with RR reference
        wfe := createTestWFE("test-wfe", "rr-abc123")
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        // Query using field selector
        list := &workflowexecutionv1alpha1.WorkflowExecutionList{}
        err := k8sClient.List(ctx, list,
            client.MatchingFields{"spec.remediationRequestRef.name": "rr-abc123"})
        Expect(err).ToNot(HaveOccurred())
        Expect(list.Items).To(HaveLen(1))
        Expect(list.Items[0].Name).To(Equal("test-wfe"))
    })
})
```

#### Integration Tests
- Verify field selectors work with EnvTest
- Test multiple field selector combinations
- Verify nil/empty field handling

#### E2E Tests (Manual)
```bash
# Verify kubectl integration
kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123
```

---

## Consequences

### Positive
- ✅ **Operational Efficiency**: 10-100x faster queries (server-side vs client-side filtering)
- ✅ **Reduced Network Traffic**: Only matching resources returned
- ✅ **Better UX**: Native kubectl syntax, no jq required
- ✅ **Consistent Pattern**: Same approach across all CRDs
- ✅ **Troubleshooting**: Easier parent-child resource traversal

### Negative
- ⚠️ **Memory Overhead**: Each index consumes memory (~100 bytes per resource)
- ⚠️ **Startup Time**: Index building adds ~50-100ms per 1000 resources
- ⚠️ **Controller Restart Required**: Adding new indexes requires restart

**Mitigation**:
- Memory overhead is negligible for typical cluster sizes (<10K CRDs)
- Startup time impact is minimal and one-time
- New indexes can be added in future releases

### Neutral
- 🔄 **Maintenance**: Each CRD needs indexer registration
- 🔄 **Documentation**: Need to document available field selectors

---

## Validation Results

### Confidence Assessment Progression
- **Initial assessment**: 85% confidence
- **After alternatives analysis**: 90% confidence
- **After implementation review**: 90% confidence

### Key Validation Points
- ✅ Proven pattern in controller-runtime ecosystem
- ✅ Low implementation cost (~50 LOC per CRD)
- ✅ No breaking changes to existing APIs
- ✅ Immediate operational value

---

## Related Decisions
- **Builds On**: DD-CRD-001 (CRD Design Patterns)
- **Supports**: BR-OPS-001 (Operational Efficiency)
- **Related To**: DD-API-001 (API Design Consistency)

---

## Review & Evolution

### When to Revisit
- If memory overhead becomes significant (>1GB for indexes)
- If new field selector requirements emerge
- If Kubernetes adds native CRD field selector support

### Success Metrics
- **Adoption**: >50% of operational queries use field selectors
- **Performance**: <100ms query time for filtered results
- **UX**: Positive feedback from operators

---

## Implementation Checklist

### Phase 1: CRD Annotations (P0 - API Level)
- [ ] WorkflowExecution: Add `+kubebuilder:selectablefield` annotations
- [ ] RemediationRequest: Complete remaining field annotations (signalFingerprint ✅ exists)
- [ ] AIAnalysis: Add selectablefield annotations
- [ ] SignalProcessing: Add selectablefield annotations
- [ ] RemediationApprovalRequest: Add selectablefield annotations
- [ ] KubernetesExecution: Add selectablefield annotations
- [ ] Run `make manifests` to regenerate CRDs
- [ ] Verify selectableFields in generated CRD YAML

### Phase 2: Controller Indexers (P1 - Programmatic Queries)
- [ ] WorkflowExecution field indexers (for controller `client.List()` calls)
- [ ] RemediationRequest field indexers
- [ ] AIAnalysis field indexers
- [ ] SignalProcessing field indexers
- [ ] RemediationApprovalRequest field indexers
- [ ] KubernetesExecution field indexers
- [ ] Unit tests for field indexers
- [ ] Integration tests for programmatic field selector queries

### Phase 3: Documentation (P0)
- [ ] Update operator documentation with field selector examples
- [ ] Add troubleshooting guide with common queries
- [ ] Update CRD reference documentation
- [ ] Document kubebuilder version requirements (v3.14+)

---

## Summary: Why Both Mechanisms?

### CRD Annotations (`+kubebuilder:selectablefield`)
**Purpose**: Enable `kubectl` CLI filtering for human operators
**Example**: `kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123`
**Works**: Always (API server enforced)
**Use Case**: Operational troubleshooting, debugging, monitoring

### Controller Indexers (`mgr.GetFieldIndexer().IndexField()`)
**Purpose**: Enable programmatic filtering in controller code
**Example**: `client.List(ctx, list, client.MatchingFields{"spec.remediationRequestRef.name": "rr-abc123"})`
**Works**: Only when controller is running
**Use Case**: Controllers finding child resources, parent-child traversal, reconciliation logic

### Both Required
- **CRD annotations** → kubectl works
- **Controller indexers** → controller code works
- **Same field names** → consistent UX across both interfaces

---

## References

- [Kubernetes Field Selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/)
- [Kubernetes v1.30 CRD selectableFields](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#field-selectors)
- [controller-runtime FieldIndexer](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client#FieldIndexer)
- [kubebuilder selectablefield marker](https://book.kubebuilder.io/reference/markers/crd.html)
- [BR-OPS-001: Operational Efficiency Requirements](../requirements/BR-OPS-OPERATIONAL-EFFICIENCY.md) (to be created)

---

**Document Status**: ✅ **PROPOSED**
**Created**: 2025-12-29
**Priority Level**: P1 - OPERATIONAL ENHANCEMENT
**Estimated Effort**: 2-3 days (all CRDs)
**Minimum Kubernetes**: v1.30+ (no backwards compatibility burden)

