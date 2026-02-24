## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step.

### Business Requirements

**V1 Defined BRs** (11 BRs with dedicated files):
- **BR-ORCH-001**: Approval notification creation
- **BR-ORCH-025, BR-ORCH-026**: Workflow data pass-through, approval orchestration
- **BR-ORCH-027, BR-ORCH-028**: Global and per-phase timeout management
- **BR-ORCH-029, BR-ORCH-030, BR-ORCH-031**: Notification handling, status tracking, cascade cleanup
- **BR-ORCH-032, BR-ORCH-033, BR-ORCH-034**: WE Skipped phase, duplicate tracking, bulk notification

**See**: [BR_MAPPING.md](./BR_MAPPING.md) for authoritative BR references and test coverage.

**V2 Future** (BRs not yet defined):
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
  - [ ] Sequential service CRD creation (RemediationProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution (DEPRECATED - ADR-025))
  - [ ] Data snapshot pattern for CRD spec population
  - [ ] Owner reference management for cascade deletion

- [ ] **Implement Watch Configuration**
  - [ ] Watch RemediationProcessing status for completion
  - [ ] Watch AIAnalysis status for completion
  - [ ] Watch WorkflowExecution status for completion
  - [ ] Watch KubernetesExecution (DEPRECATED - ADR-025) status for completion
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

