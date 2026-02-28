# Next Branch: Field Selectors Implementation

**Status**: 📋 **QUEUED FOR NEXT BRANCH**
**Created**: 2025-12-29
**Design Decision**: [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md)
**Estimated Effort**: 2-3 days (all CRDs)

---

## 🎯 **Objective**

Implement custom field selectors across all Kubernaut CRDs using BOTH mechanisms:
1. **CRD Annotations** (`+kubebuilder:selectablefield`) - For kubectl CLI
2. **Controller Indexers** (FieldIndexer) - For programmatic queries

---

## 📋 **Tasks**

### Phase 1: CRD Annotations (P0)
- [ ] **WorkflowExecution**: Add selectablefield annotations
  - `spec.remediationRequestRef.name`
  - `spec.remediationRequestRef.namespace`
  - `spec.targetResource`
  - `status.phase`
  - `status.pipelineRunRef.name`

- [ ] **RemediationRequest**: Complete remaining annotations
  - ✅ `spec.signalFingerprint` (already exists)
  - `spec.targetResource.name`
  - `spec.targetResource.namespace`
  - `status.phase`
  - `status.workflowExecutionRef.name`

- [ ] **AIAnalysis**: Add selectablefield annotations
  - `spec.remediationRequestRef.name`
  - `spec.analysisType`
  - `status.phase`

- [ ] **SignalProcessing**: Add selectablefield annotations
  - `spec.remediationRequestRef.name`
  - `spec.signalSource`
  - `status.phase`

- [ ] **RemediationApprovalRequest**: Add selectablefield annotations
  - `spec.remediationRequestRef.name`
  - `spec.approvalType`
  - `status.decision`

- [ ] **KubernetesExecution** (DEPRECATED - ADR-025): Add selectablefield annotations
  - `spec.remediationRequestRef.name` (if exists)
  - `spec.targetResource.name`
  - `status.phase`

- [ ] Run `make manifests` to regenerate CRDs
- [ ] Verify selectableFields in generated CRD YAML files

---

### Phase 2: Controller Indexers (P1)
- [ ] Create `internal/controller/workflowexecution/indexers.go`
- [ ] Create `internal/controller/remediationorchestrator/indexers.go`
- [ ] Create `internal/controller/aianalysis/indexers.go`
- [ ] Create `internal/controller/signalprocessing/indexers.go`
- [ ] Create `internal/controller/remediationapprovalrequest/indexers.go`
- [ ] Create `internal/controller/kubernetesexecution/indexers.go`
- [ ] Wire indexers in respective `cmd/*/main.go` files

---

### Phase 3: Testing (P0)
- [ ] Unit tests for each indexer file
- [ ] Integration tests for kubectl field selector queries
- [ ] Integration tests for programmatic field selector queries (`client.MatchingFields`)
- [ ] Verify cross-CRD parent-child traversal queries

---

### Phase 4: Documentation (P0)
- [ ] Update operator troubleshooting guide with field selector examples
- [ ] Add common kubectl queries to operational runbook
- [ ] Update CRD reference documentation
- [ ] Document minimum Kubernetes version requirement (v1.30+)
- [ ] Add examples to each CRD's README

---

## 📝 **Example Implementation (WorkflowExecution)**

### CRD Annotations
```go
// api/workflowexecution/v1alpha1/workflowexecution_types.go

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetResource`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
//
// Field Selectors for Operational Queries (DD-CRD-003)
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.namespace
// +kubebuilder:selectablefield:JSONPath=.spec.targetResource
// +kubebuilder:selectablefield:JSONPath=.status.phase
// +kubebuilder:selectablefield:JSONPath=.status.pipelineRunRef.name
type WorkflowExecution struct {
    // ...
}
```

### Controller Indexers
```go
// internal/controller/workflowexecution/indexers.go
package workflowexecution

import (
    "context"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"

    workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// RegisterFieldIndexers registers field selectors for programmatic queries.
// Per DD-CRD-003: Enable controllers to query WFEs efficiently.
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

    // Add remaining indexes...

    return nil
}
```

### Wiring in main.go
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

## ✅ **Success Criteria**

- [ ] All CRDs have selectablefield annotations
- [ ] All controllers have field indexers registered
- [ ] kubectl field selector queries work for all CRDs
- [ ] Programmatic queries (`client.MatchingFields`) work in controllers
- [ ] Unit tests pass (100% coverage for indexers)
- [ ] Integration tests pass (field selector queries)
- [ ] Documentation updated with examples
- [ ] Minimum Kubernetes version (v1.30+) documented

---

## 🔗 **Related Documents**

- [DD-CRD-003: Field Selectors Design Decision](../architecture/DD-CRD-003-field-selectors-operational-queries.md)
- [BR-OPS-001: Operational Efficiency](../requirements/BR-OPS-001-operational-efficiency.md) (to be created)

---

## 📊 **Impact**

**Operators gain**:
```bash
# Find all WFEs for a RemediationRequest
kubectl get workflowexecutions --field-selector spec.remediationRequestRef.name=rr-abc123

# Find WFE by PipelineRun
kubectl get workflowexecutions --field-selector status.pipelineRunRef.name=wfe-xyz789

# Find all running workflows
kubectl get workflowexecutions --field-selector status.phase=Running
```

**Controllers gain**:
```go
// Efficient parent-child traversal
wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
err := r.Client.List(ctx, wfeList,
    client.MatchingFields{"spec.remediationRequestRef.name": rr.Name})
```

---

**Branch**: TBD (next branch after current test fixes)
**Prerequisites**: Current branch merged (test fixes complete)
**Blocked By**: None
**Blocks**: None (independent enhancement)

