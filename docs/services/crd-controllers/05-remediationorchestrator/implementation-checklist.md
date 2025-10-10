## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step.

### Business Requirements

- **V1 Scope**: BR-AR-001 to BR-AR-067 (25 BRs total)
  - BR-AR-001 to 060: Core orchestration (18 BRs)
    - CRD lifecycle coordination
    - Status aggregation
    - Event-driven phase transitions
  - BR-AR-061 to 067: CRD monitoring (7 BRs, migrated from BR-ALERT-*)
    - Lifecycle monitoring
    - Status aggregation
    - Event coordination
    - Cross-controller integration
- **Reserved for V2**: BR-AR-068 to BR-AR-180
  - Parallel remediation workflows
  - Cross-alert correlation and batch remediation
  - ML-based timeout prediction
  - Multi-cluster orchestration

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("remediationorchestrator")`

---

### Phase 0: Project Setup (30 min) [BEFORE ANALYSIS]

- [ ] **Verify cmd/ structure**: Check [cmd/README.md](../../../../cmd/README.md)
- [ ] **Create service directory**: `mkdir -p cmd/remediationorchestrator` (no hyphens - Go convention)
- [ ] **Copy main.go template**: From existing `cmd/remediationorchestrator/main.go` (reference implementation)
- [ ] **Update package imports**: Verify controller imports (RemediationRequestReconciler)
- [ ] **Verify build**: `go build -o bin/remediation-orchestrator ./cmd/remediationorchestrator` (binary can have hyphens)
- [ ] **Reference documentation**: [cmd/ directory guide](../../../../cmd/README.md)

**Note**: Directory names use Go convention (no hyphens), binaries can use hyphens for readability.

---

### Phase 1: CRD Schema & API (2-3 days)

- [ ] **Define RemediationRequest API types** (`api/v1/alertremediation_types.go`)
  - [ ] RemediationRequestSpec with timeout configuration
  - [ ] RemediationRequestStatus with service CRD references and status summaries
  - [ ] Reference types for all service CRDs
  - [ ] Status summary types for aggregation

- [ ] **Generate CRD manifests**
  ```bash
  kubebuilder create api --group kubernaut --version v1 --kind RemediationRequest
  make manifests
  ```

- [ ] **Install CRD to cluster**
  ```bash
  make install
  kubectl get crds | grep alertremediation
  ```

### Phase 2: Controller Implementation (3-4 days)

- [ ] **Implement RemediationRequestReconciler**
  - [ ] Core reconciliation logic with phase orchestration
  - [ ] Sequential service CRD creation (RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution)
  - [ ] Data snapshot pattern for CRD spec population
  - [ ] Owner reference management for cascade deletion

- [ ] **Implement Watch Configuration**
  - [ ] Watch RemediationProcessing status for completion
  - [ ] Watch AIAnalysis status for completion
  - [ ] Watch WorkflowExecution status for completion
  - [ ] Watch KubernetesExecution status for completion
  - [ ] Mapping functions for all watches

- [ ] **Implement Timeout Detection**
  - [ ] Per-phase timeout calculation
  - [ ] Timeout escalation via Notification Service
  - [ ] Overall workflow timeout detection

- [ ] **Implement Finalizer Pattern**
  - [ ] 24-hour retention timer
  - [ ] Cleanup logic (audit record persistence)
  - [ ] Finalizer removal and CRD deletion

- [ ] **Implement Failure Handling**
  - [ ] Service CRD failure detection
  - [ ] Failure reason propagation
  - [ ] Terminal state management

### Phase 3: External Integrations (1-2 days)

- [ ] **Notification Service Integration**
  - [ ] HTTP client for escalation endpoint
  - [ ] Escalation request payload construction
  - [ ] Channel selection based on severity/environment

- [ ] **Data Storage Service Integration**
  - [ ] HTTP client for audit endpoint
  - [ ] Audit record payload construction
  - [ ] Finalizer cleanup with audit persistence

### Phase 4: Testing (2-3 days)

- [ ] **Unit Tests**
  - [ ] Phase progression logic
  - [ ] Timeout detection
  - [ ] Failure handling
  - [ ] Retention cleanup
  - [ ] Watch mapping functions

- [ ] **Integration Tests**
  - [ ] End-to-end CRD creation flow
  - [ ] Service CRD completion triggers
  - [ ] Cascade deletion
  - [ ] Timeout escalation

- [ ] **E2E Tests**
  - [ ] Complete remediation workflow with live services
  - [ ] Alert storm testing (duplicate handling)

### Phase 5: Metrics & Observability (1 day)

- [ ] **Prometheus Metrics**
  - [ ] Phase transition duration
  - [ ] Timeout counters
  - [ ] Completion duration histogram
  - [ ] Retention cleanup counter
  - [ ] Service CRD creation duration

- [ ] **Event Emission**
  - [ ] Phase transition events
  - [ ] Timeout events
  - [ ] Failure events
  - [ ] Completion events

### Phase 6: Production Readiness (1-2 days)

- [ ] **RBAC Configuration**
  - [ ] ClusterRole for RemediationRequest controller
  - [ ] Service CRD creation permissions
  - [ ] Event emission permissions

- [ ] **Deployment Manifests**
  - [ ] Controller deployment YAML
  - [ ] ServiceAccount, Role, RoleBinding
  - [ ] Prometheus ServiceMonitor

- [ ] **Documentation**
  - [ ] Operator guide
  - [ ] Troubleshooting runbook
  - [ ] Metrics reference

---

