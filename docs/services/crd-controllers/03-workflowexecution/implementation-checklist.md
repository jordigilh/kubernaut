## Implementation Checklist

**Version**: 3.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture

---

## Changelog

### Version 3.0 (2025-12-02)
- ✅ **Updated**: API group references to `.ai`
- ✅ **Updated**: BR prefixes standardized to `BR-WE-*`

---

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 0: Project Setup (30 min) [BEFORE ANALYSIS]

- [ ] **Verify cmd/ structure**: Check [cmd/README.md](../../../../cmd/README.md)
- [ ] **Create service directory**: `mkdir -p cmd/workflowexecution` (no hyphens - Go convention)
- [ ] **Copy main.go template**: From `cmd/remediationorchestrator/main.go`
- [ ] **Update package imports**: Change to service-specific controller (WorkflowExecutionReconciler)
- [ ] **Verify build**: `go build -o bin/workflow-execution ./cmd/workflowexecution` (binary can have hyphens)
- [ ] **Reference documentation**: [cmd/ directory guide](../../../../cmd/README.md)

**Note**: Directory names use Go convention (no hyphens), binaries can use hyphens for readability.

---

### Phase 1: ANALYSIS & CRD Setup (2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing workflow implementations (`codebase_search "workflow execution implementations"`)
- [ ] **ANALYSIS**: Map business requirements across all 4 BR prefixes:
  - BR-WE-001 to BR-WE-011: Core workflow execution and resource locking (see BUSINESS_REQUIREMENTS.md)

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("workflowexecution")`

---

- [ ] **ANALYSIS**: Identify integration points in cmd/workflowexecution/
- [ ] **CRD RED**: Write WorkflowExecutionReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD + controller skeleton (tests pass)
  - [ ] Create WorkflowExecution CRD schema (`api/v1/workflowexecution_types.go`)
  - [ ] Generate Kubebuilder controller scaffold
  - [ ] Implement WorkflowExecutionReconciler with finalizers
  - [ ] Configure owner references to RemediationRequest CRD
- [ ] **CRD REFACTOR**: Enhance controller with error handling
  - [ ] Add controller-specific Prometheus metrics
  - [ ] Implement cross-CRD reference validation
  - [ ] Add phase timeout detection (configurable per phase)

### Phase 2: Planning & Validation Phases (2-3 days) [RED-GREEN-REFACTOR]

- [ ] **Planning RED**: Write tests for planning phase (fail - no planning logic yet)
- [ ] **Planning GREEN**: Implement minimal planning logic (tests pass)
  - [ ] Workflow analysis and dependency resolution
  - [ ] Execution strategy planning
  - [ ] Resource planning and estimation
- [ ] **Planning REFACTOR**: Enhance with sophisticated algorithms
  - [ ] Parallel execution detection
  - [ ] Adaptive optimization based on history
- [ ] **Validation RED**: Write tests for validation phase (fail)
- [ ] **Validation GREEN**: Implement safety validation (tests pass)
  - [ ] RBAC checks
  - [ ] Resource availability validation
  - [ ] Dry-run execution (optional)
  - [ ] Approval validation
- [ ] **Validation REFACTOR**: Add sophisticated safety checks

### Phase 3: Execution & Monitoring Phases (3-4 days) [RED-GREEN-REFACTOR]

- [ ] **Execution RED**: Write tests for step execution (fail - no execution logic yet)
- [ ] **Execution GREEN**: Implement Tekton PipelineRun orchestration (tests pass)
  - [ ] Create PipelineRun with OCI bundle resolver (ADR-044)
  - [ ] Watch-based PipelineRun completion monitoring
  - [ ] Tekton handles step orchestration internally
  - [ ] Failure handling and resource locking (DD-WE-001)
- [ ] **Execution REFACTOR**: Enhance with adaptive adjustments
  - [ ] Runtime optimization
  - [ ] Historical pattern application
- [ ] **Monitoring RED**: Write tests for effectiveness monitoring (fail)
- [ ] **Monitoring GREEN**: Implement monitoring logic (tests pass)
  - [ ] Resource health validation
  - [ ] Success criteria verification
  - [ ] Learning and optimization recording
- [ ] **Main App Integration**: Verify WorkflowExecutionReconciler instantiated in cmd/workflowexecution/ (MANDATORY)

### Phase 4: Rollback & Error Handling (2 days) [RED-GREEN-REFACTOR]

- [ ] **Rollback RED**: Write tests for rollback strategies (fail)
- [ ] **Rollback GREEN**: Implement automatic rollback (tests pass)
  - [ ] Step-by-step rollback execution
  - [ ] State restoration logic
  - [ ] Rollback verification
- [ ] **Rollback REFACTOR**: Add manual rollback support
  - [ ] Rollback approval workflow
  - [ ] Partial rollback capabilities

### Phase 5: Testing & Validation (2 days) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/workflowexecution/)
  - [ ] Core Workflow tests (BR-WE-001 to BR-WE-008): PipelineRun creation, status monitoring
  - [ ] Resource Locking tests (BR-WE-009 to BR-WE-011): Parallel prevention, cooldown
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/workflowexecution/)
  - [ ] Real K8s API (KIND) CRD lifecycle tests
  - [ ] Tekton PipelineRun creation and monitoring
  - [ ] Resource lock verification tests
- [ ] **CHECK**: Execute E2E tests - 10% coverage target (test/e2e/workflowexecution/)
  - [ ] Complete workflow-to-completion scenario
  - [ ] Multi-step workflow with dependencies
  - [ ] Rollback scenario testing
- [ ] **CHECK**: Validate business requirement coverage (BR-WE-001 to BR-WE-011)
  - [ ] BR-WE-001 to BR-WE-008: Core workflow execution
  - [ ] BR-WE-009 to BR-WE-011: Resource locking safety
- [ ] **CHECK**: Performance validation (per-step <5min, total <30min)
- [ ] **CHECK**: Provide confidence assessment (90% high confidence)

### Phase 6: Metrics, Audit & Deployment (1 day)

- [ ] **Metrics**: Define and implement Prometheus metrics
  - [ ] Workflow execution metrics
  - [ ] Phase duration metrics
  - [ ] Step success/failure metrics
  - [ ] Rollback metrics
  - [ ] Setup metrics server on port 9090 (with auth)
- [ ] **Audit**: Database integration for learning
  - [ ] Implement audit client
  - [ ] Record workflow executions to PostgreSQL
  - [ ] Store execution patterns to vector DB
  - [ ] Implement historical success queries
- [ ] **Deployment**: Binary and infrastructure
  - [ ] Create `cmd/workflowexecution/main.go` entry point
  - [ ] Configure Kubebuilder manager with leader election
  - [ ] Add RBAC permissions for CRD operations
  - [ ] Create Kubernetes deployment manifests

### Phase 7: Documentation (1.5 days)

- [ ] Update API documentation with WorkflowExecution CRD
- [ ] Document workflow planning patterns
- [ ] Add troubleshooting guide for workflow execution
- [ ] Create runbook for rollback procedures
- [ ] Document adaptive orchestration mechanisms
- [ ] **Workflow Author's Guide** (`docs/guides/workflow-authoring.md`)
  - [ ] Directory structure (pipeline.yaml, tasks/, workflow-schema.yaml)
  - [ ] workflow-schema.yaml format (reference DD-STORAGE-008)
  - [ ] Building OCI bundles (`tkn bundle push` or Dockerfile)
  - [ ] Signing with Cosign (optional for v1.0)
  - [ ] Registering in Data Storage catalog
  - [ ] Testing locally with `tkn pipeline start`
  - [ ] Example: Complete disk-cleanup workflow

---

