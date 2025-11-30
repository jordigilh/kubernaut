## Implementation Checklist

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.3 | 2025-11-30 | Added OwnerChain, DetectedLabels, CustomLabels tasks (DD-WORKFLOW-001 v1.8) | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) |
> | v1.2 | 2025-11-28 | Gateway migration added, CRD location fixed (kubernaut.io/v1alpha1), parallel testing | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md), [DD-TEST-002](../../../architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Context API removed (deprecated) | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Categorization phase added | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.0 | 2025-01-15 | Initial implementation checklist | - |

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 0: Project Setup (30 min) [BEFORE ANALYSIS]

- [ ] **Verify cmd/ structure**: Check [cmd/README.md](../../../../cmd/README.md)
- [ ] **Create service directory**: `mkdir -p cmd/signalprocessing` (no hyphens - Go convention)
- [ ] **Copy main.go template**: From `cmd/remediationorchestrator/main.go`
- [ ] **Update package imports**: Change to service-specific controller (SignalProcessingReconciler)
- [ ] **Verify build**: `go build -o bin/signal-processing ./cmd/signalprocessing` (binary can have hyphens)
- [ ] **Reference documentation**: [cmd/ directory guide](../../../../cmd/README.md)

**Note**: Directory names use Go convention (no hyphens), binaries can use hyphens for readability.

---

### Phase 1: ANALYSIS & Package Migration (1-2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing implementations (`codebase_search "SignalProcessing implementations"`)
- [ ] **ANALYSIS**: Map business requirements across all V1 BRs:
  - **V1 Scope**: BR-SP-001 to BR-SP-075 (25 BRs total)
    - BR-SP-001 to 050: Core signal processing (16 BRs)
    - BR-SP-051 to 053: Environment classification (3 BRs, migrated from BR-ENV-*)
    - BR-SP-060 to 062: Signal enrichment (3 BRs, migrated from BR-ALERT-*)
    - BR-SP-070 to 075: Priority categorization (6 BRs, consolidated from Gateway per DD-CATEGORIZATION-001)
  - **Reserved for V2**: BR-SP-076 to BR-SP-180 (multi-source context, advanced correlation)

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("signalprocessing")`

---

- [ ] **ANALYSIS**: Identify integration points in cmd/signalprocessing/
- [ ] **Package Migration RED**: Write tests validating type-safe interfaces (fail with map[string]interface{})
- [ ] **Package Migration GREEN**: Implement structured types in `pkg/signalprocessing/types.go`
  - [ ] **Package Location**: `pkg/signalprocessing/`
  - [ ] **Update Package Declarations**: `package signalprocessing`
  - [ ] **Update Imports**: Across relevant files
  - [ ] **Interface Rename**: Define `SignalProcessingService` interface
  - [ ] **Remove Deduplication**: Deduplication is Gateway Service responsibility
- [ ] **Package Migration REFACTOR**: Enhance error handling and validation logic
- [ ] **Test Directory Setup**:
  - [ ] Create `test/unit/signalprocessing/`
  - [ ] Create `test/integration/signalprocessing/`
  - [ ] Create `test/e2e/signalprocessing/`

### Phase 2: CRD Implementation (3-4 days) [RED-GREEN-REFACTOR]

- [ ] **CRD RED**: Write SignalProcessingReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD using Kubebuilder + controller skeleton (tests pass)
  - [ ] Generate SignalProcessing CRD (`api/kubernaut.io/v1alpha1/signalprocessing_types.go`)
  - [ ] Implement SignalProcessingReconciler with 4 phases (enriching, classifying, categorizing, completed)
  - [ ] Add owner references and finalizers for cascade deletion
- [ ] **CRD REFACTOR**: Enhance controller with phase logic and error handling
  - [ ] Implement phase timeout detection and handling
  - [ ] Add Kubernetes event emission for visibility
  - [ ] Implement optimized requeue strategy
- [ ] **Integration RED**: Write tests for owner reference management (fail initially)
- [ ] **Integration GREEN**: Implement owner references to RemediationRequest (tests pass)

### Phase 3: Business Logic Integration (1-2 days) [RED-GREEN-REFACTOR]

- [ ] **Logic RED**: Write tests for environment classification (fail)
- [ ] **Logic GREEN**: Integrate business logic to pass tests
  - [ ] Integrate existing environment classification logic from `pkg/processor/environment/`
  - [ ] Add K8s context enrichment (monitoring + business contexts)
  - [ ] **Add recovery context reading from spec.failureData** (Context API deprecated per DD-CONTEXT-006)
  - [ ] Add status update for RemediationRequest reference
- [ ] **Logic REFACTOR**: Enhance with sophisticated algorithms
  - [ ] Add degraded mode fallback when enrichment services unavailable
  - [ ] **Add fallback recovery context builder (buildDegradedRecoveryContext)**
  - [ ] Optimize classification heuristics and performance
- [ ] **Categorization Integration (DD-CATEGORIZATION-001)**:
  - [ ] **RED**: Write tests for priority categorization based on enriched K8s context
  - [ ] **GREEN**: Implement `reconcileCategorizing()` phase
  - [ ] **GREEN**: Implement priority assignment using namespace labels, workload type, environment
  - [ ] **REFACTOR**: Optimize priority scoring algorithm
- [ ] **Audit Integration**: Integrate Data Storage Service REST API for audit persistence (ADR-032)
- [ ] **Main App Integration**: Verify SignalProcessingReconciler instantiated in cmd/signalprocessing/ (MANDATORY)

### Phase 3.25: Owner Chain & Label Detection (2-3 days) [DD-WORKFLOW-001 v1.8] ⭐ NEW

**Reference**: [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)

#### A. Owner Chain Implementation (4-6 hours)

- [ ] **OwnerChain RED**: Write tests for owner chain building
  - [ ] Test Pod → ReplicaSet → Deployment traversal
  - [ ] Test StatefulSet/DaemonSet ownership
  - [ ] Test cluster-scoped resources (Node has empty namespace)
  - [ ] Test max depth (10 levels) to prevent infinite loops
- [ ] **OwnerChain GREEN**: Implement `buildOwnerChain()` in controller
  - [ ] Create `pkg/signalprocessing/ownerchain/builder.go`
  - [ ] Traverse K8s `ownerReferences` starting from source resource
  - [ ] Use first `controller: true` ownerReference at each level
  - [ ] Inherit namespace from current resource (namespaced) or empty (cluster-scoped)
  - [ ] Stop when no more owners or owner not found
  - [ ] Populate `status.enrichmentResults.ownerChain[]`
- [ ] **OwnerChain REFACTOR**: Add caching, optimize API calls
- [ ] **OwnerChainEntry Type**:
  ```go
  type OwnerChainEntry struct {
      Namespace string `json:"namespace,omitempty"` // Empty for cluster-scoped
      Kind      string `json:"kind"`                // ReplicaSet, Deployment, etc.
      Name      string `json:"name"`
  }
  ```

#### B. DetectedLabels Implementation (4-5 days)

- [ ] **DetectedLabels RED**: Write tests for each detection type
  - [ ] Test GitOps detection (ArgoCD annotation, Flux label)
  - [ ] Test PDB detection (query PDBs matching pod labels)
  - [ ] Test HPA detection (query HPAs targeting deployment)
  - [ ] Test Stateful detection (StatefulSet or PVCs)
  - [ ] Test Helm detection (managed-by label, helm.sh/chart)
  - [ ] Test NetworkPolicy detection (any NP in namespace)
  - [ ] Test PodSecurityLevel detection (namespace label)
  - [ ] Test ServiceMesh detection (Istio/Linkerd sidecar annotations)
- [ ] **DetectedLabels GREEN**: Implement detection logic
  - [ ] Create `pkg/signalprocessing/detection/labels.go`
  - [ ] Create `pkg/signalprocessing/detection/gitops.go`
  - [ ] Create `pkg/signalprocessing/detection/protection.go` (PDB, HPA)
  - [ ] Create `pkg/signalprocessing/detection/workload.go` (StatefulSet, Helm)
  - [ ] Create `pkg/signalprocessing/detection/security.go` (NP, PSS, ServiceMesh)
  - [ ] Implement `LabelDetector` interface in controller
  - [ ] Populate `status.enrichmentResults.detectedLabels`
- [ ] **DetectedLabels REFACTOR**: Parallelize K8s API queries, add caching
- [ ] **DetectedLabels Convention**: Boolean fields only when `true`, omit when `false`
  ```go
  if dl.GitOpsManaged {
      result["gitOpsManaged"] = true
      result["gitOpsTool"] = dl.GitOpsTool
  }
  // Don't add: result["gitOpsManaged"] = false
  ```

#### C. CustomLabels Rego Evaluation (3-4 days)

- [ ] **Rego RED**: Write tests for Rego evaluation
  - [ ] Test simple label extraction (`team` from namespace label)
  - [ ] Test risk-tolerance derivation (environment → risk mapping)
  - [ ] Test constraint labels (`cost-constrained`, `high-availability`)
  - [ ] Test security wrapper (cannot override 5 mandatory labels)
  - [ ] Test empty policy returns empty map
  - [ ] Test policy error is non-fatal (returns empty map)
- [ ] **Rego GREEN**: Implement Rego engine
  - [ ] Create `pkg/signalprocessing/rego/engine.go`
    - [ ] Use OPA/Rego library (`github.com/open-policy-agent/opa/rego`)
    - [ ] Load policy from ConfigMap `signal-processing-policies` in `kubernaut-system`
    - [ ] PrepareForEval with security wrapper
  - [ ] Create `pkg/signalprocessing/rego/security.go`
    - [ ] Implement security wrapper policy that strips 5 mandatory labels
    - [ ] Wrap customer policy with security policy before evaluation
  - [ ] Create `pkg/signalprocessing/rego/input.go`
    - [ ] Build `RegoInput` struct from K8s context + DetectedLabels
    - [ ] Include: namespace, pod, deployment, node, signal, detected_labels
- [ ] **Rego REFACTOR**: Policy hot-reload on ConfigMap change, caching
- [ ] **Output Format**: `map[string][]string` (subdomain → list of values)
  ```go
  // Example output
  customLabels := map[string][]string{
      "kubernaut.io":           {"team=platform", "risk-tolerance=high"},
      "constraint.kubernaut.io": {"cost-constrained"},
  }
  ```
- [ ] **Security Wrapper** (5 mandatory labels blocked):
  ```rego
  system_labels := {
      "kubernaut.io/signal-type",
      "kubernaut.io/severity",
      "kubernaut.io/component",
      "kubernaut.io/environment",
      "kubernaut.io/priority"
  }
  ```

#### D. ConfigMap Setup

- [ ] **Deploy Example Policy ConfigMap**:
  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: signal-processing-policies
    namespace: kubernaut-system
  data:
    labels.rego: |
      package signalprocessing.labels
      # Customer policy goes here
      # See: HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.2
  ```
- [ ] **Document Policy Format**: Update operator documentation
- [ ] **Add RBAC**: Controller needs read access to ConfigMaps in kubernaut-system

#### E. Integration with Controller

- [ ] Update `SignalProcessingReconciler` struct:
  ```go
  LabelDetector *LabelDetector  // V1.0
  RegoEngine    *RegoEngine     // V1.0
  ```
- [ ] Update `reconcileEnriching()`:
  1. Build owner chain (call `buildOwnerChain`)
  2. Detect labels (call `LabelDetector.DetectLabels`)
  3. Evaluate Rego (call `RegoEngine.EvaluatePolicy`)
  4. Populate `status.enrichmentResults.ownerChain`
  5. Populate `status.enrichmentResults.detectedLabels`
  6. Populate `status.enrichmentResults.customLabels`

#### F. Testing

- [ ] **Unit Tests**: `test/unit/signalprocessing/detection/`
- [ ] **Integration Tests**: `test/integration/signalprocessing/labels/`
  - [ ] Real K8s cluster (KIND) with ArgoCD/Flux annotations
  - [ ] Real PDB/HPA resources
  - [ ] Real Rego policy evaluation
- [ ] **E2E Tests**: `test/e2e/signalprocessing/`
  - [ ] Full enrichment flow with labels
  - [ ] Verify labels passed to AIAnalysis

### Phase 3.5: Gateway Code Migration (1 day) [DD-CATEGORIZATION-001]

**Move categorization code from Gateway to Signal Processing**:

- [ ] **Production Code Migration**:
  - [ ] Copy `pkg/gateway/processing/classification.go` → `pkg/signalprocessing/classification.go`
  - [ ] Copy `pkg/gateway/processing/priority.go` → `pkg/signalprocessing/priority.go`
  - [ ] Refactor package names and imports
  - [ ] Update Gateway to remove classification logic (pass through raw values)
- [ ] **Rego Policy Migration**:
  - [ ] Copy `config.app/gateway/policies/priority.rego` → `config.app/signalprocessing/policies/priority.rego`
  - [ ] Update policy paths in configuration
- [ ] **Test Migration**:
  - [ ] Copy `test/unit/gateway/processing/environment_classification_test.go` → `test/unit/signalprocessing/`
  - [ ] Copy `test/unit/gateway/priority_classification_test.go` → `test/unit/signalprocessing/`
  - [ ] Update test imports and package declarations
  - [ ] Update Gateway tests to verify no classification behavior
- [ ] **Integration Test Updates**:
  - [ ] Update `test/integration/gateway/` tests that reference classification
  - [ ] Add Signal Processing integration tests for classification
- [ ] **Deprecate Gateway BRs**: Mark BR-ENV-*, BR-PRIORITY-* as deprecated (moved to BR-SP-*)

**See**: [IMPLEMENTATION_PLAN_V1.11.md - Gateway Migration Section](./IMPLEMENTATION_PLAN_V1.11.md)

### Phase 4: Testing & Validation (1 day) [CHECK Phase]

**Parallel Execution**: Run all tests with 4 concurrent processes per [DD-TEST-002](../../../architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md)

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/signalprocessing/)
  - [ ] Write unit tests for each reconciliation phase (enriching, classifying, categorizing)
  - [ ] Use fake K8s client per [ADR-004](../../../architecture/decisions/ADR-004-fake-kubernetes-client.md), mock external services only
  - [ ] Run: `go test -p 4 ./test/unit/signalprocessing/...`
- [ ] **CHECK**: Run integration tests - >50% coverage target (test/integration/signalprocessing/)
  - [ ] Add integration tests with real enrichment service
  - [ ] Test CRD lifecycle with real K8s API (envtest)
  - [ ] Run: `go test -p 4 ./test/integration/signalprocessing/...`
- [ ] **CHECK**: Execute E2E tests for critical workflows (test/e2e/signalprocessing/)
  - [ ] Add E2E tests for complete signal-to-remediation workflow
  - [ ] Use Kind NodePort (30082) per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)
  - [ ] Run: `ginkgo -procs=4 ./test/e2e/signalprocessing/...`
- [ ] **CHECK**: Validate business requirement coverage (all 25 V1 BRs)
  - [ ] BR-SP-001 to 050: Core signal processing
  - [ ] BR-SP-051 to 053: Environment classification
  - [ ] BR-SP-060 to 062: Signal enrichment
  - [ ] BR-SP-070 to 075: Priority categorization (DD-CATEGORIZATION-001)
- [ ] **CHECK**: Configure RBAC for controller
- [ ] **CHECK**: Add Prometheus metrics for phase durations
- [ ] **CHECK**: Provide confidence assessment (85% - high confidence, see Development Methodology)

## Critical Architectural Patterns (from KUBERNAUT_CRD_ARCHITECTURE.md)

### 1. Owner References & Cascade Deletion
**Pattern**: SignalProcessing CRD owned by RemediationRequest
```go
controllerutil.SetControllerReference(&remediationRequest, &signalProcessing, scheme)
```
**Purpose**: Automatic cleanup when RemediationRequest is deleted (24h retention)

### 2. Finalizers for Cleanup Coordination
**Pattern**: Add finalizer before processing, remove after cleanup
```go
const signalProcessingFinalizer = "signalprocessing.kubernaut.io/finalizer"
```
**Purpose**: Ensure audit data persisted before CRD deletion

### 3. Watch-Based Status Coordination
**Pattern**: Status updates trigger RemediationRequest reconciliation automatically
```go
// Status update here triggers RemediationRequest watch
r.Status().Update(ctx, &signalProcessing)
```
**Purpose**: No manual RemediationRequest updates needed - watch handles aggregation

### 4. Phase Timeout Detection & Escalation
**Pattern**: Per-phase timeout with degraded mode fallback
```go
defaultPhaseTimeout = 5 * time.Minute
```
**Purpose**: Prevent stuck processing, enable degraded mode continuation

### 5. Event Emission for Visibility
**Pattern**: Emit Kubernetes events for operational tracking
```go
r.Recorder.Event(&signalProcessing, "Normal", "PhaseCompleted", message)
```
**Purpose**: Operational visibility in kubectl events and monitoring

### 6. Optimized Requeue Strategy
**Pattern**: Phase-based requeue intervals, no requeue for terminal states
```go
// Completed state: no requeue (watch handles updates)
// Active phases: 10s requeue
// Unknown states: 30s conservative requeue
```
**Purpose**: Efficient reconciliation, reduced API server load

### 7. Cross-CRD Reference Validation
**Pattern**: Validate RemediationRequestRef exists before processing
```go
r.Get(ctx, remediationRequestRef, &remediationRequest)
```
**Purpose**: Ensure parent CRD exists, prevent orphaned processing

### 8. Metrics for Reconciliation Performance
**Pattern**: Track controller performance separately from business metrics
```go
// Controller-specific metrics
ControllerReconciliationDuration
ControllerErrorsTotal
ControllerRequeueTotal
```
**Purpose**: Monitor controller health vs business logic performance

### 9. Recovery Context from Embedded Data (DD-CONTEXT-006)
**Pattern**: Read recovery context from spec.failureData instead of Context API
```go
if sp.Spec.IsRecoveryAttempt && sp.Spec.FailureData != nil {
    recoveryCtx := buildRecoveryContextFromFailureData(sp)
    sp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
}
```
**Purpose**: Simplified architecture, no external Context API dependency

### 10. Data Access Layer Isolation (ADR-032)
**Pattern**: Use Data Storage Service REST API for audit writes
```go
r.DataStorageClient.CreateAuditRecord(ctx, audit)
```
**Purpose**: No direct PostgreSQL access from Signal Processing
